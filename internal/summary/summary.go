// Package summary genera resúmenes breves para el operador del panel a partir
// de los últimos mensajes de una conversación y del estado de su cita.
package summary

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	openai "github.com/sashabaranov/go-openai"
)

// Message es un turno de la conversación ya formateado para el modelo.
type Message struct {
	Role string // "Cliente" | "Bot"
	Text string
	At   time.Time
}

// Appointment son los datos de cita conocidos por el sistema.
type Appointment struct {
	Service      string `json:"service"`
	ServiceName  string `json:"service_name"`
	Availability string `json:"availability"`
	Team         bool   `json:"team"`
	Status       string `json:"status"`
	ScheduledAt  string `json:"scheduled_at,omitempty"`
}

// Input es todo lo que recibe el resumen.
type Input struct {
	UserName      string
	Topic         string
	NeedsOperator bool
	HandoffReason string
	Appointment   *Appointment
	Messages      []Message
}

// Result es el resumen listo para pintar.
type Result struct {
	Points []string `json:"points"`
	Source string   `json:"source"` // "ai" | "fallback"
}

type cacheEntry struct {
	points  []string
	source  string
	expires time.Time
}

// Summarizer genera y cachea resúmenes. Puede crearse sin API key: en ese caso
// devuelve un resumen determinista (fallback).
type Summarizer struct {
	client *openai.Client
	model  string
	log    *slog.Logger

	mu    sync.Mutex
	cache map[string]cacheEntry
}

const (
	cacheTTL     = 10 * time.Minute
	cacheFailTTL = time.Minute
	cacheSize    = 250
	maxTurns     = 16
	aiTimeout    = 8 * time.Second
)

// New construye el resumidor. Si apiKey está vacío queda en modo fallback.
func New(apiKey, model string, log *slog.Logger) *Summarizer {
	s := &Summarizer{model: model, log: log, cache: make(map[string]cacheEntry)}
	if strings.TrimSpace(apiKey) != "" {
		s.client = openai.NewClient(apiKey)
	}
	return s
}

// Summarize devuelve los puntos clave del hilo. key identifica la instantánea
// (usuario + última interacción) para reutilizar el resultado.
func (s *Summarizer) Summarize(ctx context.Context, key string, in Input) Result {
	if s == nil {
		return fallback(in)
	}
	if strings.TrimSpace(key) != "" {
		s.mu.Lock()
		if e, ok := s.cache[key]; ok && time.Now().Before(e.expires) {
			s.mu.Unlock()
			return Result{Points: e.points, Source: e.source}
		}
		s.mu.Unlock()
	}

	if s.client == nil {
		return fallback(in)
	}
	points, err := s.generate(ctx, in)
	if err != nil || len(points) == 0 {
		if s.log != nil {
			if err != nil {
				s.log.Warn("no se pudo generar el resumen del hilo", "error", err.Error())
			} else {
				s.log.Warn("resumen del hilo vacío")
			}
		}
		// Cache negativo corto: evita reintentos (y esperas) si OpenAI falla.
		res := fallback(in)
		s.store(key, res.Points, res.Source, cacheFailTTL)
		return res
	}

	res := Result{Points: points, Source: "ai"}
	s.store(key, res.Points, res.Source, cacheTTL)
	return res
}

// Reset descarta las entradas cacheadas de un usuario (clave "<userID>:<id>").
func (s *Summarizer) Reset(userID string) {
	if s == nil || userID == "" {
		return
	}
	prefix := userID + ":"
	s.mu.Lock()
	defer s.mu.Unlock()
	for k := range s.cache {
		if strings.HasPrefix(k, prefix) {
			delete(s.cache, k)
		}
	}
}

func (s *Summarizer) store(key string, points []string, source string, ttl time.Duration) {
	if key == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.cache) >= cacheSize {
		s.cache = make(map[string]cacheEntry, cacheSize)
	}
	s.cache[key] = cacheEntry{points: points, source: source, expires: time.Now().Add(ttl)}
}

// fallback resume lo imprescindible sin llamar al modelo.
func fallback(in Input) Result {
	points := make([]string, 0, 3)
	if t := prettyTopic(in.Topic); t != "" {
		points = append(points, "Tema detectado: "+t)
	}
	if in.Appointment != nil && in.Appointment.Service != "" {
		line := "Servicio de interés: " + in.Appointment.Service
		if in.Appointment.ServiceName != "" {
			line = "Servicio de interés: " + in.Appointment.ServiceName
		}
		points = append(points, line)
	}
	if in.Appointment != nil && strings.TrimSpace(in.Appointment.Availability) != "" {
		points = append(points, "Preferencia horaria: "+in.Appointment.Availability)
	}
	if last := lastClientMessage(in.Messages); last != "" {
		points = append(points, "Último mensaje del cliente: "+last)
	}
	return Result{Points: points, Source: "fallback"}
}

func lastClientMessage(msgs []Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "Cliente" && strings.TrimSpace(msgs[i].Text) != "" {
			t := strings.TrimSpace(msgs[i].Text)
			if len([]rune(t)) > 160 {
				t = string([]rune(t)[:160]) + "…"
			}
			return t
		}
	}
	return ""
}

func (s *Summarizer) generate(ctx context.Context, in Input) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, aiTimeout)
	defer cancel()

	resp, err := s.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model:               s.model,
		MaxCompletionTokens: 400,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: summarySystemPrompt},
			{Role: openai.ChatMessageRoleUser, Content: buildPrompt(in)},
		},
	})
	if err != nil {
		return nil, err
	}
	if len(resp.Choices) == 0 {
		return nil, nil
	}
	return parsePoints(resp.Choices[0].Message.Content), nil
}

const summarySystemPrompt = `Eres el asistente del operador humano de un centro de estética y depilación.
Recibes los últimos mensajes de una conversación de WhatsApp entre cliente y bot, y los datos de cita registrados.
Resume SOLO lo que sea útil para que el operador atienda al cliente.
Devuelve entre 2 y 4 líneas de texto plano, sin markdown, sin numeración y sin viñetas, con este orden de prioridad:
1. De qué está hablando el cliente y qué servicio o zona le interesa.
2. Datos de cita: servicio solicitado, fecha/hora o preferencia horaria que haya dado.
3. Cualquier otro dato relevante: presupuesto, alergias, dudas, quejas, prisa, condiciones especiales.
Máximo 18 palabras por línea. No inventes datos que no aparezcan en los mensajes o en los datos de cita. Si no hay información de algún punto, omítelo.`

func buildPrompt(in Input) string {
	var b strings.Builder
	b.WriteString("Cliente: ")
	if strings.TrimSpace(in.UserName) != "" {
		b.WriteString(in.UserName)
	} else {
		b.WriteString("(sin nombre)")
	}
	b.WriteString("\n")
	if t := prettyTopic(in.Topic); t != "" {
		b.WriteString("Tema detectado por el sistema: " + t + "\n")
	}
	if in.Appointment != nil && (in.Appointment.Service != "" || in.Appointment.Availability != "") {
		b.WriteString("Datos de cita registrados:\n")
		if in.Appointment.Service != "" {
			b.WriteString("- Servicio: " + in.Appointment.Service)
			if in.Appointment.ServiceName != "" {
				b.WriteString(" (" + in.Appointment.ServiceName + ")")
			}
			b.WriteString("\n")
		}
		if strings.TrimSpace(in.Appointment.Availability) != "" {
			b.WriteString("- Preferencia horaria: " + in.Appointment.Availability + "\n")
		}
		if in.Appointment.Team {
			b.WriteString("- Profesional: equipo\n")
		} else {
			b.WriteString("- Profesional: Tamara\n")
		}
		b.WriteString("- Estado: " + in.Appointment.Status + "\n")
	}
	if in.HandoffReason != "" {
		b.WriteString("Motivo de derivación al operador: " + in.HandoffReason + "\n")
	}
	b.WriteString("\nConversación (más antigua primero):\n")
	msgs := in.Messages
	if len(msgs) > maxTurns {
		msgs = msgs[len(msgs)-maxTurns:]
	}
	for _, m := range msgs {
		text := strings.TrimSpace(m.Text)
		if text == "" {
			continue
		}
		if len([]rune(text)) > 400 {
			text = string([]rune(text)[:400]) + "…"
		}
		b.WriteString(m.Role + ": " + text + "\n")
	}
	b.WriteString("\nResumen para el operador:")
	return b.String()
}

func parsePoints(raw string) []string {
	out := make([]string, 0, 4)
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimLeft(line, "-•*·0123456789.) ")
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, line)
		if len(out) == 4 {
			break
		}
	}
	return out
}

// prettyTopic convierte un code RAG (prompt_laser_diodo) en texto legible.
func prettyTopic(code string) string {
	code = strings.TrimSpace(code)
	code = strings.TrimPrefix(code, "prompt_")
	code = strings.ReplaceAll(code, "_", " ")
	return strings.TrimSpace(code)
}
