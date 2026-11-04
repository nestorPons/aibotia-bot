package summary

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aibotia/aibotia-go/internal/appointments"
	"github.com/aibotia/aibotia-go/internal/domain"
	"github.com/aibotia/aibotia-go/internal/store/repo"
)

// threadTurns es el número de interacciones que se tienen en cuenta para el
// resumen (mismo valor que usa el panel).
const threadTurns = 24

// ThreadSources agrupa las fuentes de datos que alimentan el resumen del hilo.
// Son las mismas para el panel y para el bot en modo prueba.
type ThreadSources struct {
	Interactions ThreadInteractions
	Users        ThreadUsers
	Appointments ThreadAppointments
	Services     ThreadServices
}

// ThreadInteractions lista las interacciones de un usuario.
type ThreadInteractions interface {
	ListByUser(ctx context.Context, userID string, limit int) ([]repo.InteractionRow, error)
}

// ThreadUsers obtiene los datos del usuario (nombre, tema, IA activa).
type ThreadUsers interface {
	Get(ctx context.Context, id string) (domain.User, error)
}

// ThreadAppointments obtiene el estado de cita del usuario.
type ThreadAppointments interface {
	Get(ctx context.Context, userID string) (appointments.State, error)
}

// ThreadServices resuelve el nombre legible de un servicio.
type ThreadServices interface {
	Get(ctx context.Context, id string) (repo.Service, error)
}

// Thread es el resumen del hilo: los mismos datos que pinta el panel.
type Thread struct {
	UserID        string
	UserName      string
	Topic         string
	AIEnabled     bool
	NeedsOperator bool
	HandoffReason string
	Appointment   *Appointment
	Points        []string
	Source        string
	Messages      int
	LastMessageAt *time.Time
}

// Text formatea los puntos como los ve el operador (uno por línea).
func (t Thread) Text() string {
	points := make([]string, 0, len(t.Points))
	for _, p := range t.Points {
		if p = strings.TrimSpace(p); p != "" {
			points = append(points, p)
		}
	}
	return strings.Join(points, "\n")
}

// Reader construye resúmenes de hilo con fuentes fijas.
type Reader struct {
	Sources    ThreadSources
	Summarizer *Summarizer
}

// NewReader construye un lector de resúmenes de hilo.
func NewReader(sources ThreadSources, summarizer *Summarizer) *Reader {
	return &Reader{Sources: sources, Summarizer: summarizer}
}

// Thread devuelve el resumen del hilo de un usuario.
func (r *Reader) Thread(ctx context.Context, userID string) (Thread, error) {
	return BuildThread(ctx, r.Sources, r.Summarizer, userID)
}

// BuildThread reúne la conversación, el usuario y la cita, y genera el resumen
// con el mismo criterio que el panel del operador.
func BuildThread(ctx context.Context, src ThreadSources, s *Summarizer, userID string) (Thread, error) {
	th := Thread{UserID: userID, Points: []string{}, Source: "fallback"}
	if src.Interactions == nil {
		return th, fmt.Errorf("summary: faltan las interacciones")
	}
	rows, err := src.Interactions.ListByUser(ctx, userID, threadTurns)
	if err != nil {
		return th, err
	}

	if src.Users != nil {
		if u, err := src.Users.Get(ctx, userID); err == nil {
			th.UserName = u.Name
			th.Topic = u.Topic
			th.AIEnabled = u.Active
		}
	}

	if len(rows) > 0 {
		latest := rows[0]
		th.NeedsOperator = repo.NeedsOperator(latest.ToolCalls)
		th.HandoffReason = repo.HandoffReason(latest.ToolCalls)
		th.LastMessageAt = &latest.CreatedAt
		th.Messages = len(rows)
	}

	if src.Appointments != nil {
		if st, err := src.Appointments.Get(ctx, userID); err == nil &&
			(st.Status != appointments.StatusIdle || st.Service != "" || st.Availability != "") {
			appt := &Appointment{
				Service:      st.Service,
				Availability: st.Availability,
				Team:         st.Team,
				Status:       string(st.Status),
			}
			if st.Service != "" && src.Services != nil {
				if sv, err := src.Services.Get(ctx, st.Service); err == nil {
					appt.ServiceName = sv.Name
				}
			}
			th.Appointment = appt
		}
	}

	messages := make([]Message, 0, len(rows)*2)
	for i := len(rows) - 1; i >= 0; i-- {
		row := rows[i]
		if strings.TrimSpace(row.UserMessage) != "" {
			messages = append(messages, Message{Role: "Cliente", Text: row.UserMessage, At: row.CreatedAt})
		}
		if strings.TrimSpace(row.AIResponse) != "" {
			role := "Bot"
			if row.Model == "manual" {
				role = "Operador"
			}
			messages = append(messages, Message{Role: role, Text: row.AIResponse, At: row.CreatedAt})
		}
	}

	key := ""
	if len(rows) > 0 {
		key = fmt.Sprintf("%s:%d", userID, rows[0].ID)
	}
	res := s.Summarize(ctx, key, Input{
		UserName:      th.UserName,
		Topic:         th.Topic,
		NeedsOperator: th.NeedsOperator,
		HandoffReason: th.HandoffReason,
		Appointment:   th.Appointment,
		Messages:      messages,
	})
	if res.Points != nil {
		th.Points = res.Points
	}
	th.Source = res.Source
	return th, nil
}
