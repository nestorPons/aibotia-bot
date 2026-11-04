// Package actions ejecuta las acciones solicitadas por el modelo tras generar
// una respuesta (fix_chat, derivar a humano, enviar ubicación, ...).
package actions

import (
	"context"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aibotia/aibotia-go/internal/domain"
	"github.com/aibotia/aibotia-go/internal/pipeline"
	"github.com/aibotia/aibotia-go/internal/summary"
	"github.com/aibotia/aibotia-go/internal/whatsapp"
)

// UserActivator permite activar o desactivar la IA de un usuario.
type UserActivator interface {
	SetActive(ctx context.Context, id string, active bool) error
}

// ThreadReader genera el resumen del hilo que vería el operador en el panel.
type ThreadReader interface {
	Thread(ctx context.Context, userID string) (summary.Thread, error)
}

// TestModeHandoffNote se añade a la respuesta cuando una acción de derivación
// ocurre en modo prueba: el bot sigue activo, pero se explica qué pasaría en
// producción.
const TestModeHandoffNote = "🧪 Nota de prueba: en producción, el bot se desactivaría en este punto para que atendiera un operador."

// TestModeSummaryHeading precede el resumen del panel que se adjunta en modo
// prueba.
const TestModeSummaryHeading = "📋 Resumen que vería el operador en el panel:"

// testModeSummaryTimeout acota la generación del resumen para no retrasar la
// respuesta al cliente si OpenAI tarda.
const testModeSummaryTimeout = 12 * time.Second

// Location es la ubicación del Studio.
type Location struct {
	Lat     float64
	Lng     float64
	Name    string
	Address string
}

// pinRe reconoce la línea técnica del prompt de empresa:
//
//	<!-- pin: 39.4555556,-0.3583096 | Studio Tamara Freitas | Av. Instituto Obrero, 21 -->
var pinRe = regexp.MustCompile(`(?is)<!--\s*pin:\s*(-?\d+(?:[.,]\d+)?)\s*,\s*(-?\d+(?:[.,]\d+)?)\s*(?:\|\s*([^|]*?)\s*)?(?:\|\s*(.*?)\s*)?-->`)

// ParsePin extrae la ubicación de la línea `<!-- pin: ... -->` de un prompt.
// Devuelve nil si no hay una ubicación válida.
func ParsePin(content string) *Location {
	m := pinRe.FindStringSubmatch(content)
	if m == nil {
		return nil
	}
	lat, err := strconv.ParseFloat(strings.Replace(m[1], ",", ".", 1), 64)
	if err != nil {
		return nil
	}
	lng, err := strconv.ParseFloat(strings.Replace(m[2], ",", ".", 1), 64)
	if err != nil {
		return nil
	}
	if lat == 0 && lng == 0 {
		return nil
	}
	loc := &Location{
		Lat:     lat,
		Lng:     lng,
		Name:    strings.TrimSpace(m[3]),
		Address: strings.TrimSpace(m[4]),
	}
	if loc.Name == "" {
		loc.Name = "Ubicación"
	}
	return loc
}

// Runner implementa pipeline.ActionRunner.
type Runner struct {
	log      *slog.Logger
	users    UserActivator
	provider whatsapp.Provider
	threads  ThreadReader

	mu       sync.RWMutex
	location *Location
}

// NewRunner construye el ejecutor de acciones. threads puede ser nil: en ese
// caso el modo prueba solo añade la nota, sin resumen.
func NewRunner(log *slog.Logger, users UserActivator, provider whatsapp.Provider, location *Location, threads ThreadReader) *Runner {
	r := &Runner{log: log, users: users, provider: provider, threads: threads}
	r.SetLocation(location)
	return r
}

// SetLocation actualiza la ubicación en caliente (sin reiniciar).
func (r *Runner) SetLocation(location *Location) {
	if location != nil && strings.TrimSpace(location.Name) == "" {
		location.Name = "Ubicación"
	}
	r.mu.Lock()
	r.location = location
	r.mu.Unlock()
}

// Location devuelve la ubicación configurada (nil si no hay).
func (r *Runner) Location() *Location {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.location
}

// Run ejecuta las acciones de una respuesta.
func (r *Runner) Run(ctx context.Context, user domain.User, reply *pipeline.Reply) error {
	for _, name := range reply.Actions {
		switch name {
		case "fix_chat":
			r.fixChat(ctx, user, reply)
		case "message_human_request":
			r.handoff(ctx, user, reply)
		case "send_location":
			r.sendLocation(ctx, user.ID)
		case "notify_delay":
			r.log.Info("acción notify_delay ejecutada", "user", user.ID)
		case "send_attachment":
			// Los adjuntos ya viajan en reply.Attachments.
		default:
			r.log.Debug("acción no reconocida", "action", name, "user", user.ID)
		}
	}
	return nil
}

func (r *Runner) fixChat(ctx context.Context, user domain.User, reply *pipeline.Reply) {
	if reply.Text != "" && !strings.HasPrefix(reply.Text, "📌") {
		reply.Text = "📌 " + reply.Text
	}
	// whatsmeow no expone etiquetas/pin a alto nivel (son app-state); se aplica
	// el efecto funcional (fijar en cola de "Pendientes" desactivando la IA).
	if user.TestMode {
		r.addTestModeHandoff(ctx, user, reply)
		r.log.Info("fix_chat aplicado en modo prueba: mensaje marcado, IA activa", "user", user.ID)
		return
	}
	r.deactivate(ctx, user.ID)
	r.log.Info("fix_chat aplicado: IA desactivada y mensaje marcado", "user", user.ID)
}

// handoff deriva la conversación a una persona. En modo prueba no desactiva la
// IA (la prueba debe continuar) y añade una nota explicando el comportamiento
// real.
func (r *Runner) handoff(ctx context.Context, user domain.User, reply *pipeline.Reply) {
	if user.TestMode {
		r.addTestModeHandoff(ctx, user, reply)
		r.log.Info("handoff en modo prueba: la IA sigue activa", "user", user.ID)
		return
	}
	r.deactivate(ctx, user.ID)
}

// addTestModeNote añade una sola vez la nota de modo prueba a la respuesta.
func addTestModeNote(reply *pipeline.Reply) {
	if reply == nil || strings.Contains(reply.Text, TestModeHandoffNote) {
		return
	}
	text := strings.TrimRight(reply.Text, "\n")
	if text == "" {
		reply.Text = TestModeHandoffNote
		return
	}
	reply.Text = text + "\n\n" + TestModeHandoffNote
}

// addTestModeHandoff añade la nota de derivación y, a continuación, el resumen
// que vería el operador en el panel.
func (r *Runner) addTestModeHandoff(ctx context.Context, user domain.User, reply *pipeline.Reply) {
	addTestModeNote(reply)
	r.appendThreadSummary(ctx, user, reply)
}

// appendThreadSummary añade los puntos del resumen del hilo a la respuesta.
func (r *Runner) appendThreadSummary(ctx context.Context, user domain.User, reply *pipeline.Reply) {
	if r == nil || r.threads == nil || reply == nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, testModeSummaryTimeout)
	defer cancel()

	th, err := r.threads.Thread(ctx, user.ID)
	if err != nil {
		r.log.Warn("no se pudo generar el resumen del hilo en modo prueba", "user", user.ID, "error", err.Error())
		return
	}
	points := th.Text()
	if points == "" || strings.Contains(reply.Text, TestModeSummaryHeading) {
		return
	}
	lines := strings.Split(points, "\n")
	for i := range lines {
		lines[i] = "• " + lines[i]
	}
	reply.Text = strings.TrimRight(reply.Text, "\n") + "\n\n" + TestModeSummaryHeading + "\n" + strings.Join(lines, "\n")
}

func (r *Runner) deactivate(ctx context.Context, userID string) {
	if r.users == nil {
		return
	}
	if err := r.users.SetActive(ctx, userID, false); err != nil {
		r.log.Warn("no se pudo desactivar la IA", "user", userID, "error", err.Error())
	}
}

func (r *Runner) sendLocation(ctx context.Context, userID string) {
	loc := r.Location()
	if r.provider == nil || loc == nil {
		r.log.Warn("ubicación no configurada; se omite send_location", "user", userID)
		return
	}
	id, err := r.provider.SendLocation(ctx, userID, loc.Lat, loc.Lng, loc.Name, loc.Address)
	if err != nil {
		r.log.Warn("error enviando ubicación", "user", userID, "error", err.Error())
		r.sendLocationFallback(ctx, userID, loc)
		return
	}
	r.log.Info("ubicación enviada",
		"user", userID, "message", id, "lat", loc.Lat, "lng", loc.Lng, "name", loc.Name)
}

// sendLocationFallback envía la dirección como texto si WhatsApp rechaza el
// pin, para que el cliente reciba al menos la ubicación.
func (r *Runner) sendLocationFallback(ctx context.Context, userID string, loc *Location) {
	parts := make([]string, 0, 2)
	if name := strings.TrimSpace(loc.Name); name != "" && name != "Ubicación" {
		parts = append(parts, name)
	}
	if address := strings.TrimSpace(loc.Address); address != "" {
		parts = append(parts, address)
	}
	if len(parts) == 0 {
		return
	}
	text := "📍 " + strings.Join(parts, " — ")
	if _, err := r.provider.SendText(ctx, userID, text); err != nil {
		r.log.Warn("tampoco se pudo enviar la ubicación como texto", "user", userID, "error", err.Error())
		return
	}
	r.log.Info("ubicación enviada como texto (fallback)", "user", userID)
}
