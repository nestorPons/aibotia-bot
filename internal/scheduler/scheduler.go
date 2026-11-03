// Package scheduler ejecuta tareas periódicas (recordatorios de cita).
package scheduler

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/aibotia/aibotia-go/internal/appointments"
)

// Sender envía un mensaje de texto.
type Sender interface {
	SendText(ctx context.Context, to, text string) (string, error)
}

// Scheduler programa recordatorios de cita.
type Scheduler struct {
	log      *slog.Logger
	appts    *appointments.Service
	sender   Sender
	cronSpec string
	template string
	within   time.Duration
	cron     *cron.Cron
}

// New construye el scheduler.
func New(log *slog.Logger, appts *appointments.Service, sender Sender, cronSpec, template string) *Scheduler {
	if strings.TrimSpace(cronSpec) == "" {
		cronSpec = "*/10 * * * *"
	}
	if strings.TrimSpace(template) == "" {
		template = "📅 Hola, tienes una cita programada el {date}. ¡No olvides asistir!"
	}
	return &Scheduler{
		log:      log,
		appts:    appts,
		sender:   sender,
		cronSpec: cronSpec,
		template: template,
		within:   24 * time.Hour,
		cron:     cron.New(),
	}
}

// Start registra el trabajo periódico y arranca el cron.
func (s *Scheduler) Start() error {
	if _, err := s.cron.AddFunc(s.cronSpec, s.runReminders); err != nil {
		return err
	}
	s.cron.Start()
	s.log.Info("scheduler de recordatorios iniciado", "cron", s.cronSpec)
	return nil
}

// Stop detiene el scheduler.
func (s *Scheduler) Stop() {
	ctx := s.cron.Stop()
	<-ctx.Done()
}

func (s *Scheduler) runReminders() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	due, err := s.appts.DueReminders(ctx, s.within)
	if err != nil {
		s.log.Error("error consultando recordatorios", "error", err.Error())
		return
	}
	for _, d := range due {
		msg := strings.ReplaceAll(s.template, "{date}", d.ScheduledAt.Format("02/01/2006 15:04"))
		if _, err := s.sender.SendText(ctx, d.UserID, msg); err != nil {
			s.log.Warn("no se pudo enviar recordatorio", "user", d.UserID, "error", err.Error())
			continue
		}
		if err := s.appts.MarkReminderSent(ctx, d.ID); err != nil {
			s.log.Warn("no se pudo marcar recordatorio enviado", "id", d.ID, "error", err.Error())
		}
	}
	if len(due) > 0 {
		s.log.Info("recordatorios procesados", "count", len(due))
	}
}
