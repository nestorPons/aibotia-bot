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

const (
	// qrRetryDelay es la pausa entre tandas de QR cuando la anterior caduca
	// sin escanearse (cada tanda dura ~2,5 min).
	qrRetryDelay = 3 * time.Second
	// qrMaxAttempts es el número de tandas seguidas antes de espaciar la
	// regeneración (no se abandona el emparejamiento).
	qrMaxAttempts = 10
	// qrLongRetryDelay es la pausa entre tandas cuando se supera ese número.
	qrLongRetryDelay = 45 * time.Second
)

// Client implementa Provider sobre whatsmeow.
type Client struct {
	opts Options

	db        *sql.DB
	container *sqlstore.Container

	// clientMu protege el puntero wa, que se reemplaza al revincular.
	clientMu sync.RWMutex
	wa       *whatsmeow.Client

	incoming chan IncomingMessage
	status   atomic.Value // Status
	qr       atomic.Value // string

	mediaMu    sync.Mutex
	mediaCache map[string]*waE2E.Message
	mediaOrder []string

	mu         sync.Mutex
	pairMu     sync.Mutex
	pairCancel context.CancelFunc
	started    bool
	stopOnce   sync.Once
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

	c.db = db
	c.container = container
	c.started = true

	if err := c.connectLocked(ctx); err != nil {
		_ = db.Close()
		return err
	}
	log.Info("whatsapp iniciado", "paired", c.paired())
	return nil
}

// connectLocked construye el cliente whatsmeow a partir del store y arranca la
// conexión (o el emparejamiento por QR si no hay dispositivo vinculado).
// Debe llamarse con c.mu tomado.
func (c *Client) connectLocked(ctx context.Context) error {
	device, err := c.container.GetFirstDevice(ctx)
	if err != nil {
		return fmt.Errorf("whatsapp: obtener device: %w", err)
	}

	wa := whatsmeow.NewClient(device, newWALogger(c.opts.Log))
	wa.EnableAutoReconnect = true
	// Si el primer connect falla por red/DNS, whatsmeow reintenta en segundo
	// plano en lugar de dejar el canal muerto hasta el siguiente reinicio.
	wa.InitialAutoReconnect = true
	wa.AddEventHandler(c.handleEvent)
	c.setClient(wa)

	if device.ID == nil {
		// Primera ejecución (o sesión invalidada): emparejar mediante QR.
		c.startPairing(ctx)
		return nil
	}

	if err := wa.Connect(); err != nil {
		return fmt.Errorf("whatsapp: connect: %w", err)
	}
	c.opts.Log.Info("whatsapp: conexión iniciada", "paired", true)
	return nil
}

// Stop desconecta y libera recursos.
func (c *Client) Stop() {
	c.stopOnce.Do(func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.stopPairing()
		if wa := c.current(); wa != nil {
			wa.Disconnect()
		}
		if c.db != nil {
			_ = c.db.Close()
		}
	})
}

// Reconnect reinicia el canal. Si la sesión sigue vinculada solo reconecta; si
// no hay dispositivo (o WhatsApp lo invalidó) vuelve a generar un QR.
func (c *Client) Reconnect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.started || c.container == nil {
		return ErrNotConnected
	}

	c.stopPairing()
	wa := c.current()
	if wa != nil && wa.IsConnected() {
		wa.Disconnect()
	}

	// whatsmeow marca el device como borrado cuando WhatsApp cierra la sesión:
	// hay que construir un cliente nuevo sobre un device vacío.
	if wa == nil || wa.Store.Deleted || wa.Store.ID == nil {
		c.qr.Store("")
		c.setStatus(StatusDisconnected)
		return c.connectLocked(ctx)
	}

	c.setStatus(StatusDisconnected)
	if err := wa.Connect(); err != nil {
		return fmt.Errorf("whatsapp: reconectar: %w", err)
	}
	c.opts.Log.Info("whatsapp: reconexión solicitada")
	return nil
}

// paired indica si hay una sesión de WhatsApp vinculada en el store.
func (c *Client) paired() bool {
	wa := c.current()
	return wa != nil && wa.Store != nil && wa.Store.ID != nil
}

func (c *Client) current() *whatsmeow.Client {
	c.clientMu.RLock()
	defer c.clientMu.RUnlock()
	return c.wa
}

func (c *Client) setClient(wa *whatsmeow.Client) {
	c.clientMu.Lock()
	c.wa = wa
	c.clientMu.Unlock()
}

// startPairing lanza en segundo plano el emparejamiento por QR.
func (c *Client) startPairing(parent context.Context) {
	// El bucle sobrevive al contexto que lo lanzó (p. ej. una petición HTTP
	// del panel): solo se detiene al cancelarlo o al cerrar el proceso.
	ctx, cancel := context.WithCancel(context.WithoutCancel(parent))
	c.pairMu.Lock()
	if c.pairCancel != nil {
		c.pairCancel()
	}
	c.pairCancel = cancel
	c.pairMu.Unlock()

	go c.pairLoop(ctx)
}

func (c *Client) stopPairing() {
	c.pairMu.Lock()
	if c.pairCancel != nil {
		c.pairCancel()
		c.pairCancel = nil
	}
	c.pairMu.Unlock()
}

// pairLoop mantiene vigente un QR mientras no haya sesión: cada vez que una
// tanda caduca genera otra. No se rinde, para que el panel tenga siempre un
// código disponible; a partir de qrMaxAttempts espacia las tandas para no
// martillear los servidores de WhatsApp.
func (c *Client) pairLoop(ctx context.Context) {
	log := c.opts.Log
	for attempt := 1; ; attempt++ {
		if ctx.Err() != nil {
			return
		}
		wa := c.current()
		if wa == nil {
			return
		}

		qrChan, err := wa.GetQRChannel(ctx)
		if err != nil {
			log.Error("whatsapp: no se pudo abrir el canal QR", "error", err.Error())
			c.setStatus(StatusDisconnected)
			return
		}
		if err := wa.Connect(); err != nil {
			log.Error("whatsapp: no se pudo conectar para emparejar", "error", err.Error())
			c.setStatus(StatusDisconnected)
			return
		}
		c.setStatus(StatusWaitingQR)
		log.Info("whatsapp: esperando escaneo del QR", "intento", attempt)

		if !c.consumeQR(ctx, qrChan) {
			return
		}

		// Tanda caducada: la librería desconecta sola, pero nos aseguramos
		// antes de pedir un canal nuevo.
		wa.Disconnect()
		c.qr.Store("")
		c.setStatus(StatusWaitingQR)

		delay := qrRetryDelay
		if attempt >= qrMaxAttempts {
			delay = qrLongRetryDelay
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
	}
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

// consumeQR procesa una tanda de códigos QR. Devuelve true si conviene pedir
// otra tanda (caducó o falló) y false si hay que parar (vinculado o cancelado).
func (c *Client) consumeQR(ctx context.Context, ch <-chan whatsmeow.QRChannelItem) bool {
	for {
		select {
		case <-ctx.Done():
			return false
		case evt, ok := <-ch:
			if !ok {
				return false
			}
			switch evt.Event {
			case "code":
				c.qr.Store(evt.Code)
				c.setStatus(StatusWaitingQR)
				c.opts.Log.Info("QR de WhatsApp generado: escanéalo para vincular el dispositivo")
			case "success":
				c.qr.Store("")
				c.setStatus(StatusConnected)
				c.opts.Log.Info("whatsapp: dispositivo vinculado")
				return false
			case "timeout":
				c.qr.Store("")
				c.opts.Log.Warn("QR de WhatsApp caducado; generando uno nuevo")
				return true
			case "error":
				c.qr.Store("")
				c.opts.Log.Error("error en emparejamiento WhatsApp", "error", errText(evt.Error))
				return true
			default:
				// err-scanned-without-multidevice, err-client-outdated, etc.
				c.qr.Store("")
				c.opts.Log.Warn("aviso de emparejamiento WhatsApp", "evento", evt.Event, "error", errText(evt.Error))
				return true
			}
		}
	}
}

// errText evita imprimir "<nil>" en los logs.
func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
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
		c.stopPairing()
		c.qr.Store("")
		c.setStatus(StatusLoggedOut)
		c.opts.Log.Warn("sesión de whatsapp cerrada; vincula el dispositivo de nuevo desde el panel", "on_connect", v.OnConnect)
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
	wa := c.current()
	if wa == nil {
		return nil, ErrNotConnected
	}
	c.mediaMu.Lock()
	msg := c.mediaCache[messageID]
	c.mediaMu.Unlock()
	if msg == nil {
		return nil, fmt.Errorf("whatsapp: media no disponible para %q", messageID)
	}
	data, err := wa.DownloadAny(ctx, msg)
	if err != nil {
		return nil, fmt.Errorf("whatsapp: descargar media: %w", err)
	}
	return data, nil
}

// SendText envía un mensaje de texto y devuelve su ID.
func (c *Client) SendText(ctx context.Context, to, text string) (string, error) {
	wa := c.current()
	if wa == nil {
		return "", ErrNotConnected
	}
	jid, err := types.ParseJID(to)
	if err != nil {
		return "", fmt.Errorf("whatsapp: jid inválido %q: %w", to, err)
	}
	resp, err := wa.SendMessage(ctx, jid, &waE2E.Message{Conversation: proto.String(text)})
	if err != nil {
		return "", fmt.Errorf("whatsapp: enviar texto: %w", err)
	}
	return resp.ID, nil
}

// SendMedia envía una imagen o documento y devuelve su ID.
func (c *Client) SendMedia(ctx context.Context, to string, media OutboundMedia) (string, error) {
	wa := c.current()
	if wa == nil {
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
	up, err := wa.Upload(ctx, media.Data, kind)
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

	resp, err := wa.SendMessage(ctx, jid, msg)
	if err != nil {
		return "", fmt.Errorf("whatsapp: enviar media: %w", err)
	}
	return resp.ID, nil
}

// SendLocation envía una ubicación y devuelve el ID del mensaje.
func (c *Client) SendLocation(ctx context.Context, to string, lat, lng float64, name, address string) (string, error) {
	wa := c.current()
	if wa == nil {
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
	resp, err := wa.SendMessage(ctx, jid, msg)
	if err != nil {
		return "", fmt.Errorf("whatsapp: enviar ubicación: %w", err)
	}
	return resp.ID, nil
}

// ListContacts devuelve todos los contactos conocidos.
func (c *Client) ListContacts(ctx context.Context) ([]Contact, error) {
	wa := c.current()
	if wa == nil {
		return nil, ErrNotConnected
	}
	all, err := wa.Store.Contacts.GetAllContacts(ctx)
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
	wa := c.current()
	if wa == nil {
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
	if err := wa.MarkRead(ctx, []types.MessageID{types.MessageID(messageID)}, time.Now(), chatJID, senderJID); err != nil {
		return fmt.Errorf("whatsapp: marcar leído: %w", err)
	}
	return nil
}

// SendTyping envía el estado "escribiendo" o "pausado".
func (c *Client) SendTyping(ctx context.Context, chat string, composing bool) error {
	wa := c.current()
	if wa == nil {
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
	if err := wa.SendChatPresence(ctx, jid, state, types.ChatPresenceMediaText); err != nil {
		return fmt.Errorf("whatsapp: enviar presencia: %w", err)
	}
	return nil
}

// GetContact devuelve la información de contacto de un JID.
func (c *Client) GetContact(ctx context.Context, jid string) (Contact, error) {
	wa := c.current()
	if wa == nil {
		return Contact{}, ErrNotConnected
	}
	parsed, err := types.ParseJID(jid)
	if err != nil {
		return Contact{}, fmt.Errorf("whatsapp: jid inválido %q: %w", jid, err)
	}
	info, err := wa.Store.Contacts.GetContact(ctx, parsed)
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
	wa := c.current()
	if wa == nil {
		return ErrNotConnected
	}
	c.stopPairing()
	c.qr.Store("")
	if err := wa.Logout(ctx); err != nil {
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
