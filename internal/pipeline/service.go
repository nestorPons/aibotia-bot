// Package pipeline procesa los mensajes entrantes: filtros, agrupación
// (debounce), generación de respuesta y envío.
package pipeline

import (
	"context"
	"fmt"
	"log/slog"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/aibotia/aibotia-go/internal/domain"
	"github.com/aibotia/aibotia-go/internal/realtime"
	"github.com/aibotia/aibotia-go/internal/store/repo"
	"github.com/aibotia/aibotia-go/internal/whatsapp"
)

// Options configura el servicio de pipeline.
type Options struct {
	Log           *slog.Logger
	Users         *repo.Users
	Settings      *repo.Settings
	History       *repo.History
	Interactions  *repo.Interactions
	Provider      whatsapp.Provider
	Assistant     Assistant
	Spam          SpamChecker
	Actions       ActionRunner
	Events        *realtime.Hub
	BufferMin     time.Duration
	BufferMax     time.Duration
	HoursToActive time.Duration
}

// Service implementa el pipeline de mensajes.
type Service struct {
	log           *slog.Logger
	users         *repo.Users
	settings      *repo.Settings
	history       *repo.History
	interactions  *repo.Interactions
	provider      whatsapp.Provider
	assistant     Assistant
	spam          SpamChecker
	actions       ActionRunner
	events        *realtime.Hub
	hoursToActive time.Duration
	deb           *debouncer

	dedupMu sync.Mutex
	seen    map[string]time.Time
}

// New construye el servicio de pipeline.
func New(opts Options) *Service {
	s := &Service{
		log:           opts.Log,
		users:         opts.Users,
		settings:      opts.Settings,
		history:       opts.History,
		interactions:  opts.Interactions,
		provider:      opts.Provider,
		assistant:     opts.Assistant,
		spam:          opts.Spam,
		actions:       opts.Actions,
		events:        opts.Events,
		hoursToActive: opts.HoursToActive,
		seen:          make(map[string]time.Time),
	}
	minDelay := opts.BufferMin
	if minDelay <= 0 {
		minDelay = time.Second
	}
	maxDelay := opts.BufferMax
	if maxDelay <= 0 {
		maxDelay = 15 * time.Second
	}
	s.deb = newDebouncer(minDelay, maxDelay, s.flush)
	return s
}

// Run consume mensajes entrantes hasta que se cancele el contexto.
func (s *Service) Run(ctx context.Context, in <-chan whatsapp.IncomingMessage) {
	for {
		select {
		case <-ctx.Done():
			s.deb.stopAll()
			return
		case msg, ok := <-in:
			if !ok {
				return
			}
			s.handle(ctx, msg)
		}
	}
}

func (s *Service) handle(ctx context.Context, msg whatsapp.IncomingMessage) {
	if !s.markSeen(msg.ID) {
		return
	}
	if isGroupOrBroadcast(msg.Chat) {
		return
	}

	// Realtime: el panel ve el mensaje en cuanto llega, antes de que la IA
	// responda o de que se persista la interacción.
	if strings.TrimSpace(msg.Text) != "" || msg.HasMedia {
		s.events.Publish(realtime.Event{
			Type:   realtime.TypeIncoming,
			UserID: msg.Chat,
			At:     time.Now(),
			Data: realtime.Incoming{
				Text:      msg.Text,
				HasMedia:  msg.HasMedia,
				MediaType: msg.MediaType,
				FromMe:    msg.FromMe,
				PushName:  msg.PushName,
			},
		})
	}

	// Intervención del personal: un mensaje propio sin watermark desactiva la IA.
	if msg.FromMe {
		if containsWatermark(msg.Text) {
			return
		}
		if u, err := s.users.Get(ctx, msg.Chat); err == nil && u.Active {
			_ = s.users.SetActive(ctx, msg.Chat, false)
			s.log.Info("IA desactivada por intervención del personal", "chat", msg.Chat)
		}
		return
	}

	active, err := s.settings.IsActive(ctx)
	if err != nil {
		s.log.Error("no se pudo leer el estado del bot", "error", err.Error())
		return
	}
	if !active {
		return
	}

	name := msg.PushName
	if strings.TrimSpace(name) == "" {
		name = phoneFromJID(msg.Chat)
	}
	activateNew := true
	if s.settings != nil {
		if v, serr := s.settings.ActivateNewUsers(ctx); serr == nil {
			activateNew = v
		} else {
			s.log.Warn("no se pudo leer la activación de nuevos usuarios", "error", serr.Error())
		}
	}
	user, err := s.users.FindOrCreate(ctx, msg.Chat, name, activateNew)
	if err != nil {
		s.log.Error("no se pudo cargar/crear el usuario", "chat", msg.Chat, "error", err.Error())
		return
	}
	if !user.Active && !s.tryReactivate(ctx, user) {
		s.log.Info("usuario inactivo; mensaje ignorado", "chat", msg.Chat)
		return
	}

	if s.spam != nil {
		if spamming, serr := s.spam.IsSpamming(ctx, msg.Chat); serr == nil && spamming {
			s.log.Info("spam detectado; mensaje ignorado", "chat", msg.Chat)
			return
		}
	}

	s.deb.add(msg.Chat, BufferedMessage{
		ID:        msg.ID,
		Sender:    msg.Sender,
		Text:      msg.Text,
		HasMedia:  msg.HasMedia,
		MediaType: msg.MediaType,
		Timestamp: msg.Timestamp,
	})
}

func (s *Service) flush(chatID string, msgs []BufferedMessage) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	text, hasMedia, mediaType := normalizeBuffered(msgs)
	last := msgs[len(msgs)-1]

	// Transcripción de audio (si es el único contenido).
	if strings.TrimSpace(text) == "" && hasMedia && mediaType == "audio" {
		if data, derr := s.provider.DownloadMedia(ctx, last.ID); derr == nil {
			if transcribed, terr := s.assistant.Transcribe(ctx, data); terr == nil {
				text = transcribed
			} else {
				s.log.Warn("no se pudo transcribir audio", "chat", chatID, "error", terr.Error())
			}
		}
	}

	user, err := s.users.Get(ctx, chatID)
	if err != nil {
		s.log.Error("usuario no encontrado en flush", "chat", chatID, "error", err.Error())
		return
	}

	isNew := false
	if turns, herr := s.history.Window(ctx, chatID, 1); herr == nil && len(turns) == 0 {
		isNew = true
	}

	start := time.Now()
	s.publishTyping(chatID, true)
	defer s.publishTyping(chatID, false)
	reply, err := s.assistant.Reply(ctx, ReplyRequest{
		User:      user,
		Text:      text,
		HasMedia:  hasMedia,
		MediaType: mediaType,
		IsNew:     isNew,
	})
	latency := time.Since(start)
	if err != nil {
		s.log.Error("error generando respuesta", "chat", chatID, "error", err.Error())
		return
	}

	// Acciones (pueden modificar el texto y enviar ubicación).
	if s.actions != nil {
		if err := s.actions.Run(ctx, user, &reply); err != nil {
			s.log.Warn("error ejecutando acciones", "chat", chatID, "error", err.Error())
		}
	}

	if strings.TrimSpace(reply.Text) == "" && len(reply.Attachments) == 0 {
		s.log.Info("respuesta vacía; nada que enviar", "chat", chatID)
		return
	}

	_ = s.provider.SendTyping(ctx, chatID, true)
	if strings.TrimSpace(reply.Text) != "" {
		if _, err := s.provider.SendText(ctx, chatID, withBotPrefix(reply.Text)+Watermark); err != nil {
			s.log.Error("error enviando respuesta", "chat", chatID, "error", err.Error())
		}
	}
	for _, att := range reply.Attachments {
		if err := s.sendAttachment(ctx, chatID, att); err != nil {
			s.log.Warn("error enviando adjunto", "chat", chatID, "path", att, "error", err.Error())
		}
	}
	_ = s.provider.SendTyping(ctx, chatID, false)
	_ = s.provider.MarkSeen(ctx, chatID, last.Sender, last.ID)

	// Memoria de conversación.
	_ = s.history.Append(ctx, chatID, domain.Turn{Role: "user", Content: text, Timestamp: time.Now()})
	_ = s.history.Append(ctx, chatID, domain.Turn{Role: "assistant", Content: reply.Text, Timestamp: time.Now()})

	// Analítica.
	_ = s.interactions.Log(ctx, domain.Interaction{
		UserID:           chatID,
		SessionID:        chatID,
		UserMessage:      text,
		AIResponse:       reply.Text,
		LanguageCode:     reply.Language,
		PromptContext:    reply.PromptContext,
		ToolCalls:        reply.ToolCalls,
		Model:            reply.Model,
		PromptTokens:     reply.PromptTokens,
		CompletionTokens: reply.CompletionTokens,
		TotalTokens:      reply.TotalTokens,
		Satisfaction:     reply.Satisfaction,
		LatencyMs:        int(latency.Milliseconds()),
	})
}

func (s *Service) publishTyping(chatID string, typing bool) {
	s.events.Publish(realtime.Event{
		Type:   realtime.TypeTyping,
		UserID: chatID,
		At:     time.Now(),
		Data:   realtime.Typing{Typing: typing},
	})
}

func (s *Service) sendAttachment(ctx context.Context, chatID, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	mimeType := mime.TypeByExtension(filepath.Ext(path))
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	_, err = s.provider.SendMedia(ctx, chatID, whatsapp.OutboundMedia{
		Data:     data,
		FileName: filepath.Base(path),
		MimeType: mimeType,
		IsImage:  strings.HasPrefix(mimeType, "image/"),
	})
	return err
}

func (s *Service) tryReactivate(ctx context.Context, user domain.User) bool {
	if s.interactions == nil || s.hoursToActive <= 0 {
		return false
	}
	last, ok, err := s.interactions.LastAt(ctx, user.ID)
	if err != nil || !ok {
		return false
	}
	if time.Since(last) <= s.hoursToActive {
		return false
	}
	if err := s.users.SetActive(ctx, user.ID, true); err != nil {
		s.log.Warn("no se pudo reactivar el usuario", "chat", user.ID, "error", err.Error())
		return false
	}
	s.log.Info("usuario reactivado por inactividad", "chat", user.ID)
	return true
}

// ForceReply genera y envía una respuesta para un usuario (uso administrativo).
func (s *Service) ForceReply(ctx context.Context, userID, text string) (Reply, error) {
	user, err := s.users.Get(ctx, userID)
	if err != nil {
		return Reply{}, fmt.Errorf("pipeline: usuario no encontrado: %w", err)
	}
	if strings.TrimSpace(text) == "" {
		text = "hola"
	}

	reply, err := s.assistant.Reply(ctx, ReplyRequest{User: user, Text: text})
	if err != nil {
		return Reply{}, err
	}
	if s.actions != nil {
		if aerr := s.actions.Run(ctx, user, &reply); aerr != nil {
			s.log.Warn("error ejecutando acciones en force-reply", "error", aerr.Error())
		}
	}
	if strings.TrimSpace(reply.Text) != "" {
		if _, err := s.provider.SendText(ctx, userID, withBotPrefix(reply.Text)+Watermark); err != nil {
			return reply, err
		}
	}
	_ = s.history.Append(ctx, userID, domain.Turn{Role: "assistant", Content: reply.Text, Timestamp: time.Now()})
	return reply, nil
}

// Cancel descarta los mensajes pendientes (debounce) de un chat. Se usa al
// borrar un usuario para que no se envíe nada después.
func (s *Service) Cancel(chatID string) {
	if s == nil || s.deb == nil {
		return
	}
	s.deb.cancel(chatID)
}

func (s *Service) markSeen(id string) bool {
	if id == "" {
		return true
	}
	s.dedupMu.Lock()
	defer s.dedupMu.Unlock()

	now := time.Now()
	for k, t := range s.seen {
		if now.Sub(t) > 10*time.Minute {
			delete(s.seen, k)
		}
	}
	if _, ok := s.seen[id]; ok {
		return false
	}
	s.seen[id] = now
	return true
}

var courtesyWords = map[string]bool{
	"gracias": true, "ok": true, "vale": true, "perfecto": true, "entendido": true,
	"de acuerdo": true, "👍": true, "🙏": true, "😊": true,
	"thanks": true, "thank you": true, "okay": true,
}

// normalizeBuffered une los textos acumulados descartando cortesías cuando hay
// contenido sustantivo.
func normalizeBuffered(msgs []BufferedMessage) (text string, hasMedia bool, mediaType string) {
	var parts []string
	for _, m := range msgs {
		if m.HasMedia {
			hasMedia = true
			if mediaType == "" {
				mediaType = m.MediaType
			}
		}
		if t := strings.TrimSpace(m.Text); t != "" {
			parts = append(parts, t)
		}
	}
	if len(parts) > 1 {
		filtered := make([]string, 0, len(parts))
		for _, p := range parts {
			if courtesyWords[strings.ToLower(p)] {
				continue
			}
			filtered = append(filtered, p)
		}
		if len(filtered) > 0 {
			parts = filtered
		}
	}
	return strings.Join(parts, "\n"), hasMedia, mediaType
}
