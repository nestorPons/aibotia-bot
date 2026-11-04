package landing

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

var emailRE = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

// Options configura el formulario de contacto de la landing.
type Options struct {
	Env string

	SMTPHost     string
	SMTPPort     string
	SMTPUsername string
	SMTPPassword string
	SMTPFrom     string
	ContactTo    string
	SMTPSecure   string

	TurnstileSiteKey string
	TurnstileSecret  string
}

func (o Options) production() bool {
	return strings.EqualFold(o.Env, "production")
}

func (o Options) smtpEnabled() bool {
	return o.SMTPHost != "" && o.ContactTo != ""
}

// Handler sirve la landing pública de AIBOT.
type Handler struct {
	render   *Renderer
	opts     Options
	mailer   *mailer
	staticFS fs.FS
	limiter  *rateLimiter
}

func New(opts Options) (*Handler, error) {
	r, err := newRenderer(Templates)
	if err != nil {
		return nil, err
	}
	staticFS, err := fs.Sub(Static, "static")
	if err != nil {
		return nil, fmt.Errorf("static fs: %w", err)
	}
	if opts.TurnstileSiteKey != "" && opts.TurnstileSecret == "" {
		slog.Warn("captcha Turnstile sin validar: define TURNSTILE_SECRET para activar la verificación")
	}
	return &Handler{
		render:   r,
		opts:     opts,
		mailer:   newMailer(opts),
		staticFS: staticFS,
		limiter:  newRateLimiter(time.Minute),
	}, nil
}

// Security añade cabeceras defensivas a las respuestas de la landing.
func (h *Handler) Security(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "geolocation=(), camera=(), microphone=()")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self'; script-src 'self' https://challenges.cloudflare.com; connect-src 'self'; font-src 'self'; frame-src https://challenges.cloudflare.com; base-uri 'none'; form-action 'self'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

// Static sirve los assets estáticos embebidos bajo /static/.
func (h *Handler) Static() http.Handler {
	return http.StripPrefix("/static/", http.FileServer(http.FS(h.staticFS)))
}

func (h *Handler) Home(w http.ResponseWriter, r *http.Request) {
	d := HomePage()
	d.TurnstileSiteKey = h.opts.TurnstileSiteKey
	d.Form.OK = r.URL.Query().Get("ok") == "1"
	d.Form.Error = r.URL.Query().Get("error")
	h.renderPage(w, r, "home", d, http.StatusOK)
}

func (h *Handler) ContactForm(w http.ResponseWriter, r *http.Request) {
	h.renderFragment(w, "contact-form", h.formData(FormState{}), http.StatusOK)
}

func (h *Handler) Contact(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := r.ParseForm(); err != nil {
		h.contactResponse(w, r, FormState{Error: "No hemos podido leer el formulario. Inténtalo de nuevo."}, http.StatusBadRequest)
		return
	}

	name := trim(r.FormValue("nombre"), 120)
	email := trim(r.FormValue("email"), 160)
	message := trim(r.FormValue("mensaje"), 2000)
	state := FormState{Name: name, Email: email, Message: message}
	consent := r.FormValue("privacidad") == "on" || r.FormValue("privacidad") == "true" || r.FormValue("privacidad") == "1"

	// Honeypot: los bots rellenan este campo oculto.
	if strings.TrimSpace(r.FormValue("website")) != "" || strings.TrimSpace(r.FormValue("telefono")) != "" {
		h.contactResponse(w, r, FormState{OK: true}, http.StatusCreated)
		return
	}

	if msg := validateContact(name, email, message, consent); msg != "" {
		state.Error = msg
		h.contactResponse(w, r, state, http.StatusUnprocessableEntity)
		return
	}

	if err := h.verifyTurnstile(r.Context(), r.FormValue("cf-turnstile-response"), clientIP(r)); err != nil {
		slog.Warn("contact captcha rejected", "error", err.Error(), "ip", clientIP(r))
		state.Error = "No hemos podido verificar que no eres un robot. Vuelve a intentarlo."
		h.contactResponse(w, r, state, http.StatusUnprocessableEntity)
		return
	}

	if !h.limiter.Allow(clientIP(r)) {
		state.Error = "Espera un minuto antes de volver a enviar el formulario."
		h.contactResponse(w, r, state, http.StatusTooManyRequests)
		return
	}

	if h.opts.smtpEnabled() {
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		if err := h.mailer.Send(ctx, name, email, message); err != nil {
			slog.Error("send contact email", "error", err.Error())
			state.Error = "No hemos podido enviar el mensaje. Inténtalo más tarde o escríbenos por WhatsApp."
			h.contactResponse(w, r, state, http.StatusInternalServerError)
			return
		}
	} else if h.opts.production() {
		slog.Error("contact form disabled: SMTP is not configured")
		state.Error = "El formulario no está disponible en este momento. Escríbenos por WhatsApp o email."
		h.contactResponse(w, r, state, http.StatusServiceUnavailable)
		return
	} else {
		slog.Info("contact form received (SMTP disabled in development)", "name", name, "email", email)
	}

	h.contactResponse(w, r, FormState{OK: true}, http.StatusCreated)
}

func (h *Handler) Robots(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("User-agent: *\nAllow: /\n\nSitemap: https://aibotia.com/sitemap.xml\n"))
}

func (h *Handler) Sitemap(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>https://aibotia.com/</loc><changefreq>monthly</changefreq><priority>1.0</priority></url>
</urlset>`))
}

func (h *Handler) Favicon(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/static/img/favicon.ico", http.StatusMovedPermanently)
}

// NotFound renderiza la 404 de la landing, dejando intactas las rutas de API y panel.
func (h *Handler) NotFound(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/admin") {
		http.NotFound(w, r)
		return
	}
	d := HomePage()
	d.Title = "Página no encontrada · AIBOTIA 2.0"
	d.Description = "La página que buscas no existe."
	d.Path = r.URL.Path
	h.renderPage(w, r, "notfound", d, http.StatusNotFound)
}

func (h *Handler) contactResponse(w http.ResponseWriter, r *http.Request, state FormState, status int) {
	if r.Header.Get("HX-Request") == "true" {
		h.renderFragment(w, "contact-form", h.formData(state), status)
		return
	}
	if state.Error != "" {
		http.Redirect(w, r, "/?error="+url.QueryEscape(state.Error)+"#contacto", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/?ok=1#contacto", http.StatusSeeOther)
}

// formData envuelve el estado del formulario junto a la config del captcha.
func (h *Handler) formData(state FormState) PageData {
	return PageData{Form: state, TurnstileSiteKey: h.opts.TurnstileSiteKey}
}

func (h *Handler) renderPage(w http.ResponseWriter, r *http.Request, page string, d PageData, status int) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := h.render.Render(w, page, d); err != nil {
		slog.Error("render landing page", "page", page, "path", r.URL.Path, "error", err.Error())
	}
}

func (h *Handler) renderFragment(w http.ResponseWriter, name string, d PageData, status int) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := h.render.RenderFragment(w, name, d); err != nil {
		slog.Error("render landing fragment", "name", name, "error", err.Error())
	}
}

type rateLimiter struct {
	mu      sync.Mutex
	window  time.Duration
	entries map[string]time.Time
}

func newRateLimiter(window time.Duration) *rateLimiter {
	return &rateLimiter{window: window, entries: make(map[string]time.Time)}
}

func (l *rateLimiter) Allow(key string) bool {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()

	if len(l.entries) > 5000 {
		for k, seen := range l.entries {
			if now.Sub(seen) > l.window {
				delete(l.entries, k)
			}
		}
	}

	if seen, ok := l.entries[key]; ok && now.Sub(seen) < l.window {
		return false
	}
	l.entries[key] = now
	return true
}

func validateContact(name, email, message string, consent bool) string {
	if len([]rune(name)) < 2 {
		return "Escribe tu nombre."
	}
	if !emailRE.MatchString(email) {
		return "El correo electrónico no tiene un formato válido."
	}
	if len([]rune(message)) < 5 {
		return "Cuéntanos brevemente qué necesitas."
	}
	if !consent {
		return "Acepta el tratamiento de tus datos para enviar el mensaje."
	}
	return ""
}

func clientIP(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		if first, _, ok := strings.Cut(forwarded, ","); ok {
			return strings.TrimSpace(first)
		}
		return strings.TrimSpace(forwarded)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func trim(s string, max int) string {
	s = strings.TrimSpace(s)
	runes := []rune(s)
	if len(runes) > max {
		return string(runes[:max])
	}
	return s
}
