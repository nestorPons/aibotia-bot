// Package appointments gestiona el ciclo de vida de las solicitudes de cita
// con una máquina de estados determinista.
package appointments

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// Status es el estado de una solicitud de cita.
type Status string

const (
	StatusIdle                Status = "idle"
	StatusPendingService      Status = "pending_service"
	StatusPendingAvailability Status = "pending_availability"
	StatusReady               Status = "ready"
	StatusFinish              Status = "finish"
	StatusCancel              Status = "cancel"
)

// Event es una transición del flujo de citas.
type Event string

const (
	EventProvideService      Event = "provide_service"
	EventProvideAvailability Event = "provide_availability"
	EventConfirm             Event = "confirm"
	EventFinish              Event = "finish"
	EventCancel              Event = "cancel"
)

// State es el estado efímero de la cita de un usuario.
type State struct {
	UserID       string    `json:"user_id"`
	Service      string    `json:"service"`
	Availability string    `json:"availability"`
	Team         bool      `json:"team"`
	Status       Status    `json:"status"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Service implementa la gestión de citas.
type Service struct {
	db  *pgxpool.Pool
	rdb *redis.Client
	log *slog.Logger
	ttl time.Duration
}

// New construye el servicio de citas.
func New(db *pgxpool.Pool, rdb *redis.Client, log *slog.Logger) *Service {
	return &Service{db: db, rdb: rdb, log: log, ttl: 2 * time.Hour}
}

// NextStatus aplica una transición.
func NextStatus(current Status, ev Event) Status {
	switch ev {
	case EventCancel:
		return StatusCancel
	case EventProvideService:
		if current == StatusIdle || current == StatusPendingService {
			return StatusPendingAvailability
		}
	case EventProvideAvailability:
		if current == StatusPendingService || current == StatusPendingAvailability {
			return StatusReady
		}
	case EventConfirm, EventFinish:
		if current == StatusReady {
			return StatusFinish
		}
	}
	return current
}

// RecordRequest registra una solicitud de cita (invocado por el asistente).
func (s *Service) RecordRequest(ctx context.Context, userID, service, availability string, withTamara bool) error {
	status := StatusPendingService
	switch {
	case service == "":
		status = StatusPendingService
	case availability == "":
		status = StatusPendingAvailability
	default:
		status = StatusReady
	}

	state := State{
		UserID:       userID,
		Service:      service,
		Availability: availability,
		Team:         !withTamara,
		Status:       status,
		UpdatedAt:    time.Now(),
	}
	if err := s.saveState(ctx, state); err != nil {
		s.log.Warn("no se pudo cachear el estado de cita", "error", err.Error())
	}

	if _, err := s.db.Exec(ctx, `
		INSERT INTO appointments (user_id, service, availability, team, status)
		VALUES ($1, $2, $3, $4, $5)`,
		userID, service, availability, !withTamara, status); err != nil {
		return fmt.Errorf("appointments: insertar: %w", err)
	}
	return nil
}

// Get devuelve el estado actual (Redis, con fallback a BD).
func (s *Service) Get(ctx context.Context, userID string) (State, error) {
	if val, err := s.rdb.Get(ctx, stateKey(userID)).Bytes(); err == nil {
		var st State
		if json.Unmarshal(val, &st) == nil {
			return st, nil
		}
	} else if !errors.Is(err, redis.Nil) {
		return State{}, err
	}

	var st State
	st.UserID = userID
	err := s.db.QueryRow(ctx, `
		SELECT service, availability, team, status FROM appointments
		WHERE user_id = $1 ORDER BY created_at DESC LIMIT 1`, userID).
		Scan(&st.Service, &st.Availability, &st.Team, &st.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return State{UserID: userID, Status: StatusIdle, Team: true}, nil
	}
	if err != nil {
		return State{}, fmt.Errorf("appointments: obtener: %w", err)
	}
	return st, nil
}

// DueReminder es una cita próxima que requiere recordatorio.
type DueReminder struct {
	ID          int64
	UserID      string
	Service     string
	ScheduledAt time.Time
}

// DueReminders devuelve las citas listas cuyo recordatorio vence dentro de la ventana.
func (s *Service) DueReminders(ctx context.Context, within time.Duration) ([]DueReminder, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, user_id, service, scheduled_at FROM appointments
		WHERE status = 'ready' AND reminder_sent = FALSE
		  AND scheduled_at IS NOT NULL
		  AND scheduled_at BETWEEN now() AND now() + $1::interval
		ORDER BY scheduled_at`, within.String())
	if err != nil {
		return nil, fmt.Errorf("appointments: recordatorios: %w", err)
	}
	defer rows.Close()

	var out []DueReminder
	for rows.Next() {
		var d DueReminder
		if err := rows.Scan(&d.ID, &d.UserID, &d.Service, &d.ScheduledAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// MarkReminderSent marca una cita como recordada.
func (s *Service) MarkReminderSent(ctx context.Context, id int64) error {
	_, err := s.db.Exec(ctx, "UPDATE appointments SET reminder_sent = TRUE, updated_at = now() WHERE id = $1", id)
	return err
}

func (s *Service) saveState(ctx context.Context, st State) error {
	payload, err := json.Marshal(st)
	if err != nil {
		return err
	}
	return s.rdb.Set(ctx, stateKey(st.UserID), payload, s.ttl).Err()
}

func stateKey(userID string) string { return "appointments:" + userID }
