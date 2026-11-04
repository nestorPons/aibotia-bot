// Package httpapi expone la API HTTP (router, middlewares y handlers).
package httpapi

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/aibotia/aibotia-go/internal/appointments"
	"github.com/aibotia/aibotia-go/internal/config"
	"github.com/aibotia/aibotia-go/internal/landing"
	"github.com/aibotia/aibotia-go/internal/pipeline"
	"github.com/aibotia/aibotia-go/internal/realtime"
	"github.com/aibotia/aibotia-go/internal/security"
	"github.com/aibotia/aibotia-go/internal/store/repo"
	"github.com/aibotia/aibotia-go/internal/summary"
	"github.com/aibotia/aibotia-go/internal/whatsapp"
)

// Deps son las dependencias necesarias para construir el router.
type Deps struct {
	Log          *slog.Logger
	Config       *config.Config
	Version      string
	Checks       map[string]ReadyChecker
	WhatsApp     whatsapp.Provider
	Pipeline     *pipeline.Service
	AntiSpam     *security.AntiSpam
	Users        *repo.Users
	Settings     *repo.Settings
	Interactions *repo.Interactions
	Appointments *appointments.Service
	Summarizer   *summary.Summarizer
	History      *repo.History
	Instructions *repo.Instructions
	Prompts      *repo.Prompts
	Services     *repo.Services
	Content      ContentReloader
	Admin        AdminOptions
	Landing      *landing.Handler
	Events       *realtime.Hub
}

// NewRouter construye el router HTTP con middlewares y rutas.
func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(requestLogger(d.Log))
	r.Use(recoverer(d.Log))
	r.Use(cors(d.Config.CORSAllowedOrigins))
	r.Use(requestTimeout(60 * time.Second))

	health := NewHealthHandler(d.Log, d.Version, d.Checks)

	// Endpoints públicos (sin API key).
	r.Get("/health", health.Health)
	r.Get("/ready", health.Ready)

	// Landing pública en la raíz.
	if d.Landing != nil {
		r.Group(func(lr chi.Router) {
			lr.Use(d.Landing.Security)
			lr.Get("/", d.Landing.Home)
			lr.Get("/contacto/formulario", d.Landing.ContactForm)
			lr.Post("/contacto", d.Landing.Contact)
			lr.Get("/robots.txt", d.Landing.Robots)
			lr.Get("/sitemap.xml", d.Landing.Sitemap)
			lr.Get("/favicon.ico", d.Landing.Favicon)
		})
		r.Handle("/static/*", d.Landing.Security(d.Landing.Static()))
		r.NotFound(d.Landing.NotFound)
	}

	// Rutas protegidas por API key (desactivables con API_ENABLED=false).
	if d.Config.APIEnabled {
		r.Group(func(pr chi.Router) {
			pr.Use(apiKeyAuth(d.Config.APIKey))

			wa := newWhatsAppHandlers(d.Log, d.WhatsApp)
			users := newUserHandlers(d.Log, d.Users, d.Settings, d.History, d.Interactions, d.WhatsApp, d.Pipeline, d.AntiSpam, d.Summarizer)
			messages := newMessageHandlers(d.Log, d.Interactions)
			instructions := newInstructionHandlers(d.Log, d.Instructions)
			assistant := newAssistantHandlers(d.Log, d.Settings)

			pr.Route("/api/v1", func(api chi.Router) {
				api.Get("/ping", func(w http.ResponseWriter, _ *http.Request) {
					writeJSON(w, http.StatusOK, map[string]string{"status": "protected ok"})
				})

				api.Route("/whatsapp", func(wr chi.Router) {
					wr.Get("/status", wa.status)
					wr.Get("/qr", wa.qr)
					wr.Get("/contact", wa.contact)
					wr.Post("/logout", wa.logout)
				})

				api.Get("/assistant/status", assistant.status)
				api.Put("/assistant/status", assistant.setStatus)

				api.Get("/users", users.list)
				api.Post("/users/import-users", users.importUsers)
				api.Get("/users/{id}", users.get)
				api.Put("/users/{id}", users.update)
				api.Delete("/users/{id}", users.delete)
				api.Delete("/users/{id}/history", users.clearHistory)
				api.Get("/users/{id}/conversation", users.conversation)
				api.Post("/users/{id}/force-reply", users.forceReply)
				api.Post("/users/{id}/test-mode", users.setTestMode)

				api.Get("/messages", messages.list)
				api.Get("/messages/{id}", messages.byUser)

				api.Get("/instructions", instructions.list)
				api.Get("/instructions/{code}", instructions.get)
				api.Put("/instructions/{code}", instructions.update)
				api.Delete("/instructions/{code}", instructions.delete)
			})
		})
	}

	// Panel embebido (auth por sesión propia, fuera de la API key).
	if d.Admin.Enabled && d.Admin.Password != "" {
		r.Route("/admin", func(ar chi.Router) {
			newAdminHandlers(d.Log, d.Admin, d.Prompts, d.Services, d.Users, d.Settings, d.History, d.Interactions, d.Appointments, d.Summarizer, d.WhatsApp, d.Content, d.Pipeline, d.AntiSpam, d.Events).mount(ar)
		})
	}

	// Realtime (SSE): requiere API key por cabecera o query (?key=).
	r.Get("/api/v1/events", apiKeyAuthQuery(d.Config.APIKey, newEventHandlers(d.Log, d.WhatsApp).stream))

	return r
}
