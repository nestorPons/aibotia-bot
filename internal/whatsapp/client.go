package whatsapp

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // registra el driver "pgx" para database/sql
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

// Options configura el cliente de WhatsApp.
type Options struct {
	// StoreDSN es la cadena de conexión PostgreSQL donde se persiste la sesión.
	StoreDSN string
	Log      *slog.Logger
	// Queue es el tamaño del buffer del canal de mensajes entrantes.
	Queue int
}

// Client implementa Provider sobre whatsmeow.
type Client struct {
	opts Options

	wa *whatsmeow.Client
	db *sql.DB

	incoming chan IncomingMessage
	status   atomic.Value // Status
	qr       atomic.Value // string

	mediaMu    sync.Mutex
	mediaCache map[string]*waE2E.Message
	mediaOrder []string

	mu       sync.Mutex
	started  bool
	stopOnce sync.Once
}

// New construye un Client sin conectarse.
func New(opts Options) *Client {
	if opts.Queue <= 0 {
		opts.Queue = 256
	}
	c := &Client{
		opts:       opts,
		incoming:   make(chan IncomingMessage, opts.Queue),
		mediaCache: make(map[string]*waE2E.Message),
	}
	c.status.Store(StatusDisconnected)
	c.qr.Store("")
	return c
}

// Start inicializa el store y conecta (emparejando por QR si no hay sesión).
func (c *Client) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.started {
		return nil
	}
	log := c.opts.Log

	db, err := sql.Open("pgx", c.opts.StoreDSN)
	if err != nil {
		return fmt.Errorf("whatsapp: abrir store: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return fmt.Errorf("whatsapp: ping store: %w", err)
	}

	container := sqlstore.NewWithDB(db, "postgres", newWALogger(log))
	if err := container.Upgrade(ctx); err != nil {
		_ = db.Close()
		return fmt.Errorf("whatsapp: migrar store: %w", err)
	}

	device, err := container.GetFirstDevice(ctx)
	if err != nil {
		_ = db.Close()
		return fmt.Errorf("whatsapp: obtener device: %w", err)
	}

	c.db = db
	c.wa = whatsmeow.NewClient(device, newWALogger(log))
	c.wa.EnableAutoReconnect = true
	// Si el primer connect falla por red/DNS, whatsmeow reintenta en segundo
	// plano en lugar de dejar el canal muerto hasta el siguiente reinicio.
	c.wa.InitialAutoReconnect = true
	c.wa.AddEventHandler(c.handleEvent)

	if c.wa.Store.ID == nil {
		// Primera ejecución: hay que emparejar mediante QR.
		qrChan, err := c.wa.GetQRChannel(ctx)
		if err != nil {
			_ = db.Close()
			return fmt.Errorf("whatsapp: canal QR: %w", err)
		}
		c.setStatus(StatusWaitingQR)
		if err := c.wa.Connect(); err != nil {
			_ = db.Close()
			return fmt.Errorf("whatsapp: connect: %w", err)
		}
		go c.consumeQR(ctx, qrChan)
	} else {
		if err := c.wa.Connect(); err != nil {
			_ = db.Close()
			return fmt.Errorf("whatsapp: connect: %w", err)
		}
		log.Info("whatsapp: conexión iniciada", "paired", true)
	}

	c.started = true
	log.Info("whatsapp iniciado", "paired", c.wa.Store.ID != nil)
	return nil
}

// Stop desconecta y libera recursos.
func (c *Client) Stop() {
	c.stopOnce.Do(func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.wa != nil {
			c.wa.Disconnect()
		}
		if c.db != nil {
			_ = c.db.Close()
		}
	})
}

// Status devuelve el estado actual.
func (c *Client) Status() Status {
	return c.status.Load().(Status)
}

// QRCode devuelve el último código QR (vacío si no aplica).
func (c *Client) QRCode() string {
	return c.qr.Load().(string)
}

// Incoming expone el canal de mensajes entrantes.
func (c *Client) Incoming() <-chan IncomingMessage {
	return c.incoming
}

func (c *Client) setStatus(s Status) {
	c.status.Store(s)
}

func (c *Client) consumeQR(ctx context.Context, ch <-chan whatsmeow.QRChannelItem) {
	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-ch:
			if !ok {
				return
			}
			switch evt.Event {
			case "code":
				c.qr.Store(evt.Code)
				c.setStatus(StatusWaitingQR)
				c.opts.Log.Info("QR de WhatsApp generado: escanéalo para vincular el dispositivo")
			case "success":
				c.qr.Store("")
				c.setStatus(StatusConnected)
			case "timeout":
				c.qr.Store("")
				c.setStatus(StatusDisconnected)
				c.opts.Log.Warn("QR de WhatsApp caducado; reinicia para generar uno nuevo")
				return
			case "error":
				c.opts.Log.Error("error en emparejamiento WhatsApp", "error", evt.Error)
				return
			}
		}
	}
}

func (c *Client) handleEvent(evt any) {
	switch v := evt.(type) {
	case *events.Connected:
		c.setStatus(StatusConnected)
		c.opts.Log.Info("whatsapp conectado")
	case *events.Disconnected:
		c.setStatus(StatusDisconnected)
		c.opts.Log.Warn("whatsapp desconectado")
	case *events.LoggedOut:
		c.qr.Store("")
		c.setStatus(StatusLoggedOut)
		c.opts.Log.Warn("sesión de whatsapp cerrada", "on_connect", v.OnConnect)
	case *events.Message:
		c.emit(v)
	}
}

func (c *Client) emit(evt *events.Message) {
	mt := mediaType(evt.Message)
	if mt != "" {
		c.storeMedia(string(evt.Info.ID), evt.Message)
	}
	msg := IncomingMessage{
		ID:        string(evt.Info.ID),
		Chat:      evt.Info.Chat.String(),
		Sender:    evt.Info.Sender.String(),
		PushName:  evt.Info.PushName,
		Text:      messageText(evt.Message),
		Timestamp: evt.Info.Timestamp,
		FromMe:    evt.Info.IsFromMe,
		HasMedia:  mt != "",
		MediaType: mt,
	}
	c.opts.Log.Info("mensaje de WhatsApp recibido",
		"id", msg.ID,
		"chat", msg.Chat,
		"from_me", msg.FromMe,
		"media", msg.MediaType,
		"text_len", len(msg.Text),
		"preview", preview(msg.Text, 80),
	)
	select {
	case c.incoming <- msg:
	default:
		c.opts.Log.Warn("canal de entrada lleno; mensaje descartado", "id", msg.ID)
	}
}

// preview recorta el texto para mostrarlo en logs sin volcar contenido largo.
func preview(s string, max int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

// maxMediaCache es el número máximo de mensajes multimedia retenidos en memoria.
const maxMediaCache = 128

func (c *Client) storeMedia(id string, msg *waE2E.Message) {
	c.mediaMu.Lock()
	defer c.mediaMu.Unlock()
	if _, exists := c.mediaCache[id]; !exists {
		c.mediaOrder = append(c.mediaOrder, id)
	}
	c.mediaCache[id] = msg
	for len(c.mediaOrder) > maxMediaCache {
		old := c.mediaOrder[0]
		c.mediaOrder = c.mediaOrder[1:]
		delete(c.mediaCache, old)
	}
}

// DownloadMedia descarga el contenido de un mensaje multimedia por su ID.
func (c *Client) DownloadMedia(ctx context.Context, messageID string) ([]byte, error) {
	if c.wa == nil {
		return nil, ErrNotConnected
	}
	c.mediaMu.Lock()
	msg := c.mediaCache[messageID]
	c.mediaMu.Unlock()
	if msg == nil {
		return nil, fmt.Errorf("whatsapp: media no disponible para %q", messageID)
	}
	data, err := c.wa.DownloadAny(ctx, msg)
	if err != nil {
		return nil, fmt.Errorf("whatsapp: descargar media: %w", err)
	}
	return data, nil
}

// SendText envía un mensaje de texto y devuelve su ID.
func (c *Client) SendText(ctx context.Context, to, text string) (string, error) {
	if c.wa == nil {
		return "", ErrNotConnected
	}
	jid, err := types.ParseJID(to)
	if err != nil {
		return "", fmt.Errorf("whatsapp: jid inválido %q: %w", to, err)
	}
	resp, err := c.wa.SendMessage(ctx, jid, &waE2E.Message{Conversation: proto.String(text)})
	if err != nil {
		return "", fmt.Errorf("whatsapp: enviar texto: %w", err)
	}
	return resp.ID, nil
}

// SendMedia envía una imagen o documento y devuelve su ID.
func (c *Client) SendMedia(ctx context.Context, to string, media OutboundMedia) (string, error) {
	if c.wa == nil {
		return "", ErrNotConnected
	}
	jid, err := types.ParseJID(to)
	if err != nil {
		return "", fmt.Errorf("whatsapp: jid inválido %q: %w", to, err)
	}

	kind := whatsmeow.MediaDocument
	if media.IsImage {
		kind = whatsmeow.MediaImage
	}
	up, err := c.wa.Upload(ctx, media.Data, kind)
	if err != nil {
		return "", fmt.Errorf("whatsapp: subir media: %w", err)
	}

	var msg *waE2E.Message
	if media.IsImage {
		msg = &waE2E.Message{ImageMessage: &waE2E.ImageMessage{
			Caption:       proto.String(media.Caption),
			Mimetype:      proto.String(media.MimeType),
			URL:           proto.String(up.URL),
			DirectPath:    proto.String(up.DirectPath),
			MediaKey:      up.MediaKey,
			FileEncSHA256: up.FileEncSHA256,
			FileSHA256:    up.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(media.Data))),
		}}
	} else {
		msg = &waE2E.Message{DocumentMessage: &waE2E.DocumentMessage{
			Caption:       proto.String(media.Caption),
			Mimetype:      proto.String(media.MimeType),
			FileName:      proto.String(media.FileName),
			Title:         proto.String(media.FileName),
			URL:           proto.String(up.URL),
			DirectPath:    proto.String(up.DirectPath),
			MediaKey:      up.MediaKey,
			FileEncSHA256: up.FileEncSHA256,
			FileSHA256:    up.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(media.Data))),
		}}
	}

	resp, err := c.wa.SendMessage(ctx, jid, msg)
	if err != nil {
		return "", fmt.Errorf("whatsapp: enviar media: %w", err)
	}
	return resp.ID, nil
}

// SendLocation envía una ubicación y devuelve el ID del mensaje.
func (c *Client) SendLocation(ctx context.Context, to string, lat, lng float64, name, address string) (string, error) {
	if c.wa == nil {
		return "", ErrNotConnected
	}
	jid, err := types.ParseJID(to)
	if err != nil {
		return "", fmt.Errorf("whatsapp: jid inválido %q: %w", to, err)
	}
	msg := &waE2E.Message{LocationMessage: &waE2E.LocationMessage{
		DegreesLatitude:  proto.Float64(lat),
		DegreesLongitude: proto.Float64(lng),
		Name:             proto.String(name),
		Address:          proto.String(address),
	}}
	resp, err := c.wa.SendMessage(ctx, jid, msg)
	if err != nil {
		return "", fmt.Errorf("whatsapp: enviar ubicación: %w", err)
	}
	return resp.ID, nil
}

// ListContacts devuelve todos los contactos conocidos.
func (c *Client) ListContacts(ctx context.Context) ([]Contact, error) {
	if c.wa == nil {
		return nil, ErrNotConnected
	}
	all, err := c.wa.Store.Contacts.GetAllContacts(ctx)
	if err != nil {
		return nil, fmt.Errorf("whatsapp: listar contactos: %w", err)
	}
	out := make([]Contact, 0, len(all))
	for jid, info := range all {
		out = append(out, Contact{
			JID:      jid.String(),
			FullName: info.FullName,
			PushName: info.PushName,
			Business: info.BusinessName,
		})
	}
	return out, nil
}

// MarkSeen marca un mensaje como leído.
func (c *Client) MarkSeen(ctx context.Context, chat, sender, messageID string) error {
	if c.wa == nil {
		return ErrNotConnected
	}
	chatJID, err := types.ParseJID(chat)
	if err != nil {
		return fmt.Errorf("whatsapp: chat inválido %q: %w", chat, err)
	}
	senderJID, err := types.ParseJID(sender)
	if err != nil {
		return fmt.Errorf("whatsapp: sender inválido %q: %w", sender, err)
	}
	if err := c.wa.MarkRead(ctx, []types.MessageID{types.MessageID(messageID)}, time.Now(), chatJID, senderJID); err != nil {
		return fmt.Errorf("whatsapp: marcar leído: %w", err)
	}
	return nil
}

// SendTyping envía el estado "escribiendo" o "pausado".
func (c *Client) SendTyping(ctx context.Context, chat string, composing bool) error {
	if c.wa == nil {
		return ErrNotConnected
	}
	jid, err := types.ParseJID(chat)
	if err != nil {
		return fmt.Errorf("whatsapp: jid inválido %q: %w", chat, err)
	}
	state := types.ChatPresencePaused
	if composing {
		state = types.ChatPresenceComposing
	}
	if err := c.wa.SendChatPresence(ctx, jid, state, types.ChatPresenceMediaText); err != nil {
		return fmt.Errorf("whatsapp: enviar presencia: %w", err)
	}
	return nil
}

// GetContact devuelve la información de contacto de un JID.
func (c *Client) GetContact(ctx context.Context, jid string) (Contact, error) {
	if c.wa == nil {
		return Contact{}, ErrNotConnected
	}
	parsed, err := types.ParseJID(jid)
	if err != nil {
		return Contact{}, fmt.Errorf("whatsapp: jid inválido %q: %w", jid, err)
	}
	info, err := c.wa.Store.Contacts.GetContact(ctx, parsed)
	if err != nil {
		return Contact{}, fmt.Errorf("whatsapp: obtener contacto: %w", err)
	}
	return Contact{
		JID:      parsed.String(),
		FullName: info.FullName,
		PushName: info.PushName,
		Business: info.BusinessName,
	}, nil
}

// Logout cierra la sesión (requiere reescanear QR).
func (c *Client) Logout(ctx context.Context) error {
	if c.wa == nil {
		return ErrNotConnected
	}
	if err := c.wa.Logout(ctx); err != nil {
		return fmt.Errorf("whatsapp: logout: %w", err)
	}
	c.setStatus(StatusLoggedOut)
	return nil
}

func messageText(m *waE2E.Message) string {
	if m == nil {
		return ""
	}
	if v := m.GetConversation(); v != "" {
		return v
	}
	if v := m.GetExtendedTextMessage(); v != nil {
		return v.GetText()
	}
	if v := m.GetImageMessage(); v != nil {
		return v.GetCaption()
	}
	if v := m.GetVideoMessage(); v != nil {
		return v.GetCaption()
	}
	if v := m.GetDocumentMessage(); v != nil {
		return v.GetCaption()
	}
	if v := m.GetLocationMessage(); v != nil {
		return LocationText(v)
	}
	return ""
}

// LocationText representa un pin de WhatsApp como texto legible (nombre,
// dirección y enlace a Google Maps). Se usa para logs, historial y pruebas.
func LocationText(v *waE2E.LocationMessage) string {
	if v == nil {
		return ""
	}
	name := strings.TrimSpace(v.GetName())
	address := strings.TrimSpace(v.GetAddress())
	link := fmt.Sprintf("https://maps.google.com/?q=%f,%f", v.GetDegreesLatitude(), v.GetDegreesLongitude())
	switch {
	case name != "" && address != "" && name != address:
		return "📍 " + name + " — " + address + " · " + link
	case address != "":
		return "📍 " + address + " · " + link
	case name != "":
		return "📍 " + name + " · " + link
	default:
		return "📍 " + link
	}
}

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
