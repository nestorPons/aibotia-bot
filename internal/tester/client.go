// Package tester implementa un cliente de WhatsApp de pruebas: se vincula como
// un dispositivo más (QR por consola), conversa con el chatbot de la empresa y
// registra las respuestas para evaluar su calidad.
package tester

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // registra el driver "pgx" para database/sql
	"github.com/mdp/qrterminal/v3"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"

	"github.com/aibotia/aibotia-go/internal/whatsapp"
)

// Incoming es un mensaje recibido en el chat de pruebas.
type Incoming struct {
	ID     string
	Chat   types.JID
	Sender types.JID
	Text   string
	Media  string
	At     time.Time
}

// Client mantiene la sesión de WhatsApp del bot de pruebas.
type Client struct {
	log *slog.Logger
	wa  *whatsmeow.Client
	db  *sql.DB
	in  chan Incoming

	readyOnce sync.Once
	ready     chan struct{}
}

// Open abre (o crea) la sesión de WhatsApp en el PostgreSQL indicado.
// El DSN debe apuntar a una base de datos propia del tester: usar la misma que
// botd sobrescribiría su dispositivo vinculado.
func Open(ctx context.Context, dsn string, log *slog.Logger) (*Client, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, errors.New("tester: TESTER_DATABASE_URL es obligatoria")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("tester: abrir store: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("tester: ping store: %w", err)
	}

	container := sqlstore.NewWithDB(db, "postgres", newWALogger(log))
	if err := container.Upgrade(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("tester: migrar store: %w", err)
	}
	device, err := container.GetFirstDevice(ctx)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("tester: obtener device: %w", err)
	}

	c := &Client{
		log:   log,
		db:    db,
		in:    make(chan Incoming, 64),
		ready: make(chan struct{}),
	}
	c.wa = whatsmeow.NewClient(device, newWALogger(log))
	c.wa.EnableAutoReconnect = true
	c.wa.AddEventHandler(c.handleEvent)
	return c, nil
}

// Login conecta con WhatsApp. Si no hay sesión, imprime el QR en la consola.
func (c *Client) Login(ctx context.Context) error {
	if c.wa.Store.ID != nil {
		fmt.Printf("♻️  Reutilizando sesión de WhatsApp: %s\n", c.wa.Store.ID.User)
		if err := c.wa.Connect(); err != nil {
			return fmt.Errorf("tester: conectar: %w", err)
		}
		return c.waitReady(ctx, 30*time.Second)
	}

	qrChan, err := c.wa.GetQRChannel(ctx)
	if err != nil {
		return fmt.Errorf("tester: canal QR: %w", err)
	}
	fmt.Println("🔗 Vinculando el WhatsApp de pruebas…")
	fmt.Println("   WhatsApp del móvil → Dispositivos vinculados → Vincular dispositivo")
	if err := c.wa.Connect(); err != nil {
		return fmt.Errorf("tester: conectar: %w", err)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case evt, ok := <-qrChan:
			if !ok {
				return errors.New("tester: canal QR cerrado")
			}
			switch evt.Event {
			case "code":
				printQR(evt.Code)
			case "success":
				fmt.Println("\n✅ WhatsApp de pruebas vinculado")
				return c.waitReady(ctx, 30*time.Second)
			case "timeout":
				return errors.New("tester: QR caducado; vuelve a ejecutar el tester")
			case "error":
				return fmt.Errorf("tester: emparejamiento: %w", evt.Error)
			}
		}
	}
}

func (c *Client) waitReady(ctx context.Context, max time.Duration) error {
	timer := time.NewTimer(max)
	defer timer.Stop()
	select {
	case <-c.ready:
		return nil
	case <-timer.C:
		return errors.New("tester: la sesión no llegó a conectarse")
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *Client) handleEvent(evt any) {
	switch v := evt.(type) {
	case *events.Connected:
		c.readyOnce.Do(func() { close(c.ready) })
	case *events.Message:
		if v.Info.IsFromMe {
			return
		}
		text := messageText(v.Message)
		media := mediaType(v.Message)
		if text == "" && media == "" {
			return
		}
		select {
		case c.in <- Incoming{
			ID:     string(v.Info.ID),
			Chat:   v.Info.Chat,
			Sender: v.Info.Sender,
			Text:   text,
			Media:  media,
			At:     v.Info.Timestamp,
		}:
		default:
			c.log.Warn("buzón de entrada lleno; mensaje descartado", "id", v.Info.ID)
		}
	}
}

// Incoming expone los mensajes recibidos (incluye otros chats; el runner filtra).
func (c *Client) Incoming() <-chan Incoming { return c.in }

// Resolve comprueba que el número destino existe en WhatsApp y devuelve su JID.
func (c *Client) Resolve(ctx context.Context, target string) (types.JID, error) {
	phone := normalizePhone(target)
	if phone == "" {
		return types.JID{}, errors.New("tester: número destino vacío (usa -to 34600111222)")
	}
	resp, err := c.wa.IsOnWhatsApp(ctx, []string{phone})
	if err != nil {
		return types.JID{}, fmt.Errorf("tester: consultar número: %w", err)
	}
	if len(resp) == 0 || !resp[0].IsIn {
		return types.JID{}, fmt.Errorf("tester: el número %s no está en WhatsApp", phone)
	}
	return resp[0].JID, nil
}

// SendText envía un mensaje de texto al JID indicado.
func (c *Client) SendText(ctx context.Context, to types.JID, text string) (string, error) {
	resp, err := c.wa.SendMessage(ctx, to, &waE2E.Message{Conversation: proto.String(text)})
	if err != nil {
		return "", fmt.Errorf("tester: enviar: %w", err)
	}
	return resp.ID, nil
}

// OwnNumber devuelve el número vinculado al tester (vacío si aún no hay sesión).
func (c *Client) OwnNumber() string {
	if c.wa.Store.ID == nil {
		return ""
	}
	return c.wa.Store.ID.User
}

// OwnJIDs devuelve los JIDs conocidos del tester (PN y LID), útiles para
// localizar su usuario en la BD del bot.
func (c *Client) OwnJIDs() []types.JID {
	out := make([]types.JID, 0, 2)
	if c.wa.Store.ID != nil {
		out = append(out, *c.wa.Store.ID)
	}
	if c.wa.Store.LID.User != "" {
		out = append(out, c.wa.Store.LID)
	}
	return out
}

// Logout cierra la sesión de WhatsApp y borra el dispositivo del store.
func (c *Client) Logout(ctx context.Context) error {
	if c.wa.Store.ID == nil {
		return nil
	}
	return c.wa.Logout(ctx)
}

// Close desconecta sin borrar la sesión (el próximo arranque la reutiliza).
func (c *Client) Close() {
	if c.wa != nil {
		c.wa.Disconnect()
	}
	if c.db != nil {
		_ = c.db.Close()
	}
}

// Reset borra todos los dispositivos del store de pruebas (fuerza nuevo QR).
func Reset(ctx context.Context, dsn string, log *slog.Logger) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("tester: abrir store: %w", err)
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("tester: ping store: %w", err)
	}
	container := sqlstore.NewWithDB(db, "postgres", newWALogger(log))
	if err := container.Upgrade(ctx); err != nil {
		return fmt.Errorf("tester: migrar store: %w", err)
	}
	devices, err := container.GetAllDevices(ctx)
	if err != nil {
		return fmt.Errorf("tester: listar devices: %w", err)
	}
	for _, d := range devices {
		if err := container.DeleteDevice(ctx, d); err != nil {
			return fmt.Errorf("tester: borrar device: %w", err)
		}
	}
	return nil
}

func printQR(code string) {
	fmt.Println()
	qrterminal.GenerateWithConfig(code, qrterminal.Config{
		Level:     qrterminal.L,
		Writer:    os.Stdout,
		QuietZone: 2,
	})
	fmt.Println("   Esperando escaneo… (el QR caduca en ~60 s)")
}

// normalizePhone deja solo los dígitos del número (admite +34 600 11 22 33).
func normalizePhone(target string) string {
	var b strings.Builder
	for _, r := range target {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// messageText extrae el texto visible de un mensaje.
func messageText(m *waE2E.Message) string {
	if m == nil {
		return ""
	}
	if m.GetConversation() != "" {
		return m.GetConversation()
	}
	if ext := m.GetExtendedTextMessage(); ext != nil {
		return ext.GetText()
	}
	if img := m.GetImageMessage(); img != nil {
		return img.GetCaption()
	}
	if vid := m.GetVideoMessage(); vid != nil {
		return vid.GetCaption()
	}
	if doc := m.GetDocumentMessage(); doc != nil {
		return doc.GetCaption()
	}
	if loc := m.GetLocationMessage(); loc != nil {
		return whatsapp.LocationText(loc)
	}
	return ""
}

// mediaType devuelve el tipo de adjunto del mensaje ("" si es solo texto).
func mediaType(m *waE2E.Message) string {
	if m == nil {
		return ""
	}
	switch {
	case m.GetImageMessage() != nil:
		return "image"
	case m.GetVideoMessage() != nil:
		return "video"
	case m.GetAudioMessage() != nil:
		return "audio"
	case m.GetDocumentMessage() != nil:
		return "document"
	case m.GetStickerMessage() != nil:
		return "sticker"
	case m.GetLocationMessage() != nil:
		return "location"
	default:
		return ""
	}
}
