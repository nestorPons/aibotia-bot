package httpapi

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/skip2/go-qrcode"

	"github.com/aibotia/aibotia-go/internal/appointments"
	"github.com/aibotia/aibotia-go/internal/domain"
	"github.com/aibotia/aibotia-go/internal/pipeline"
	"github.com/aibotia/aibotia-go/internal/realtime"
	"github.com/aibotia/aibotia-go/internal/security"
	"github.com/aibotia/aibotia-go/internal/store/repo"
	"github.com/aibotia/aibotia-go/internal/summary"
	"github.com/aibotia/aibotia-go/internal/whatsapp"
)

//go:embed admin/index.html admin/login.html
var adminUI embed.FS

const adminCookieName = "aibotia_admin"

var (
	adminCodeRe = regexp.MustCompile(`^[a-z0-9_]{2,80}$`)
	adminIDRe   = regexp.MustCompile(`^[a-z0-9_]{2,80}$`)
)

// AdminOptions configura el panel embebido.
type AdminOptions struct {
	Enabled       bool
	Password      string
	OperatorName  string
	SessionSecret string
	SessionTTL    time.Duration
	SecureCookie  bool
}

// ContentReloader refresca la instantánea del asistente y su índice RAG.
type ContentReloader interface {
	ReloadContent(ctx context.Context) error
	ReindexPrompt(ctx context.Context, code string) error
	DeletePromptEmbedding(ctx context.Context, code string) error
}

type adminSession struct {
	password []byte
	secret   []byte
	ttl      time.Duration
	secure   bool
}

func newAdminSession(opts AdminOptions) *adminSession {
	secret := opts.SessionSecret
	if strings.TrimSpace(secret) == "" {
		secret = opts.Password
	}
	return &adminSession{
		password: []byte(opts.Password),
		secret:   []byte(secret),
		ttl:      opts.SessionTTL,
		secure:   opts.SecureCookie,
	}
}

func (s *adminSession) checkPassword(got string) bool {
	return subtle.ConstantTimeCompare([]byte(got), s.password) == 1
}

func (s *adminSession) issue(now time.Time, name string) string {
	payload := strconv.FormatInt(now.Add(s.ttl).Unix(), 10)
	if name = sanitizeOperator(name); name != "" {
		payload += "|" + base64.RawURLEncoding.EncodeToString([]byte(name))
	}
	return payload + "." + s.sign(payload)
}

// parse valida el token y devuelve el nombre del operador (vacío si no hay).
func (s *adminSession) parse(raw string, now time.Time) (string, bool) {
	payload, sig, ok := strings.Cut(raw, ".")
	if !ok || subtle.ConstantTimeCompare([]byte(sig), []byte(s.sign(payload))) != 1 {
		return "", false
	}
	expPart, namePart, _ := strings.Cut(payload, "|")
	exp, err := strconv.ParseInt(expPart, 10, 64)
	if err != nil || now.Unix() >= exp {
		return "", false
	}
	var name string
	if namePart != "" {
		if b, err := base64.RawURLEncoding.DecodeString(namePart); err == nil {
			name = string(b)
		}
	}
	return name, true
}

func (s *adminSession) valid(raw string, now time.Time) bool {
	_, ok := s.parse(raw, now)
	return ok
}

// sanitizeOperator limita el nombre a 40 caracteres y elimina saltos de línea.
func sanitizeOperator(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, "\n", " ")
	name = strings.ReplaceAll(name, "\r", " ")
	if r := []rune(name); len(r) > 40 {
		name = string(r[:40])
	}
	return name
}

// titleCase pone en mayúscula la primera letra de cada palabra sin alterar el
// resto (p. ej. "maría josé" → "María José").
func titleCase(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	start := true
	for _, r := range s {
		if unicode.IsSpace(r) {
			start = true
			b.WriteRune(r)
			continue
		}
		if start {
			b.WriteRune(unicode.ToUpper(r))
			start = false
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (s *adminSession) sign(payload string) string {
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

type adminHandlers struct {
	log          *slog.Logger
	opts         AdminOptions
	prompts      *repo.Prompts
	services     *repo.Services
	users        *repo.Users
	settings     *repo.Settings
	history      *repo.History
	interactions *repo.Interactions
	appointments *appointments.Service
	summarizer   *summary.Summarizer
	wa           whatsapp.Provider
	content      ContentReloader
	pipeline     *pipeline.Service
	antispam     *security.AntiSpam
	session      *adminSession
	events       *realtime.Hub
}

func newAdminHandlers(log *slog.Logger, opts AdminOptions, prompts *repo.Prompts, services *repo.Services, users *repo.Users, settings *repo.Settings, history *repo.History, interactions *repo.Interactions, appts *appointments.Service, summarizer *summary.Summarizer, wa whatsapp.Provider, content ContentReloader, pl *pipeline.Service, spam *security.AntiSpam, events *realtime.Hub) *adminHandlers {
	if events == nil {
		events = realtime.NewHub()
	}
	return &adminHandlers{
		log:          log,
		opts:         opts,
		prompts:      prompts,
		services:     services,
		users:        users,
		settings:     settings,
		history:      history,
		interactions: interactions,
		appointments: appts,
		summarizer:   summarizer,
		wa:           wa,
		content:      content,
		pipeline:     pl,
		antispam:     spam,
		session:      newAdminSession(opts),
		events:       events,
	}
}

// mount registra las rutas del panel (UI + API).
func (h *adminHandlers) mount(r chi.Router) {
	r.Get("/login", h.loginPage)
	r.Post("/login", h.login)
	r.Post("/logout", h.logout)

	r.Group(func(pr chi.Router) {
		pr.Use(h.requireAuth)
		pr.Get("/", h.index)

		pr.Route("/api", func(api chi.Router) {
			api.Use(h.requireHeader)

			api.Get("/services", h.listServices)
			api.Post("/services", h.createService)
			api.Get("/services/{id}", h.getService)
			api.Put("/services/{id}", h.updateService)
			api.Delete("/services/{id}", h.deleteService)

			api.Get("/prompts", h.listPrompts)
			api.Post("/prompts", h.createPrompt)
			api.Get("/prompts/{code}", h.getPrompt)
			api.Put("/prompts/{code}", h.updatePrompt)
			api.Delete("/prompts/{code}", h.deletePrompt)
			api.Get("/prompts/{code}/versions", h.listPromptVersions)
			api.Post("/prompts/{code}/versions/{version}", h.restorePrompt)

			api.Get("/users", h.listUsers)
			api.Post("/users", h.createUser)
			api.Post("/users/import", h.importContacts)
			api.Get("/users/{id}", h.getUser)
			api.Put("/users/{id}", h.updateUser)
			api.Post("/users/{id}/active", h.setUserActive)
			api.Delete("/users/{id}", h.deleteUser)

			api.Get("/settings", h.getSettings)
			api.Put("/settings", h.updateSettings)

			api.Get("/messages", h.listMessages)
			api.Get("/messages/{id}", h.conversation)
			api.Get("/messages/{id}/summary", h.messageSummary)
			api.Post("/messages/{id}/send", h.sendMessage)

			api.Get("/events", h.eventsStream)

			api.Get("/whatsapp", h.whatsappStatus)
			api.Get("/whatsapp/qr.png", h.whatsappQR)
		})
	})
}

func (h *adminHandlers) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(adminCookieName)
		if err == nil && h.session.valid(cookie.Value, time.Now()) {
			next.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/admin/api/") {
			writeError(w, http.StatusUnauthorized, "unauthorized", "Sesión no válida")
			return
		}
		http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
	})
}

func (h *adminHandlers) requireHeader(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Header.Get("X-Admin-Request") != "1" {
			writeError(w, http.StatusForbidden, "csrf", "Cabecera X-Admin-Request requerida")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *adminHandlers) loginPage(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(adminCookieName); err == nil && h.session.valid(cookie.Value, time.Now()) {
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}
	h.serveFile(w, "admin/login.html", http.StatusOK)
}

func (h *adminHandlers) login(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.serveFile(w, "admin/login.html", http.StatusBadRequest)
		return
	}
	if !h.session.checkPassword(r.FormValue("password")) {
		h.log.Warn("intento de login fallido en /admin", "ip", r.RemoteAddr)
		h.serveFile(w, "admin/login.html", http.StatusUnauthorized)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     adminCookieName,
		Value:    h.session.issue(time.Now(), r.FormValue("name")),
		Path:     "/admin",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   h.session.secure,
		MaxAge:   int(h.session.ttl.Seconds()),
	})
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

func (h *adminHandlers) logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     adminCookieName,
		Value:    "",
		Path:     "/admin",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   h.session.secure,
		MaxAge:   -1,
	})
	http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
}

func (h *adminHandlers) index(w http.ResponseWriter, _ *http.Request) {
	h.serveFile(w, "admin/index.html", http.StatusOK)
}

func (h *adminHandlers) serveFile(w http.ResponseWriter, name string, status int) {
	data, err := adminUI.ReadFile(name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "UI no disponible")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write(data)
}

// --- Servicios -------------------------------------------------------------

func (h *adminHandlers) listServices(w http.ResponseWriter, r *http.Request) {
	items, err := h.services.List(r.Context(), true)
	if err != nil {
		h.fail(w, "list_services_failed", "Error listando servicios", err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *adminHandlers) getService(w http.ResponseWriter, r *http.Request) {
	item, err := h.services.Get(r.Context(), pathParam(r, "id"))
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not_found", "Servicio no encontrado")
		return
	}
	if err != nil {
		h.fail(w, "get_service_failed", "Error obteniendo servicio", err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *adminHandlers) createService(w http.ResponseWriter, r *http.Request) {
	var s repo.Service
	if !h.decode(w, r, &s) {
		return
	}
	if !adminIDRe.MatchString(s.ID) {
		writeError(w, http.StatusBadRequest, "invalid_id", "ID inválido (minúsculas, números y _)")
		return
	}
	if strings.TrimSpace(s.Name) == "" {
		writeError(w, http.StatusBadRequest, "invalid_name", "El nombre es obligatorio")
		return
	}
	item, err := h.services.Create(r.Context(), s)
	if err != nil {
		h.fail(w, "create_service_failed", "Error creando servicio", err)
		return
	}
	h.changed(r.Context(), "")
	writeJSON(w, http.StatusCreated, item)
}

func (h *adminHandlers) updateService(w http.ResponseWriter, r *http.Request) {
	id := pathParam(r, "id")
	var s repo.Service
	if !h.decode(w, r, &s) {
		return
	}
	if s.ID == "" {
		s.ID = id
	}
	if !adminIDRe.MatchString(s.ID) {
		writeError(w, http.StatusBadRequest, "invalid_id", "ID inválido")
		return
	}
	item, err := h.services.Update(r.Context(), id, s)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not_found", "Servicio no encontrado")
		return
	}
	if err != nil {
		h.fail(w, "update_service_failed", "Error actualizando servicio", err)
		return
	}
	h.changed(r.Context(), "")
	writeJSON(w, http.StatusOK, item)
}

func (h *adminHandlers) deleteService(w http.ResponseWriter, r *http.Request) {
	err := h.services.Delete(r.Context(), pathParam(r, "id"))
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not_found", "Servicio no encontrado")
		return
	}
	if err != nil {
		h.fail(w, "delete_service_failed", "Error borrando servicio", err)
		return
	}
	h.changed(r.Context(), "")
	writeJSON(w, http.StatusOK, map[string]string{"message": "servicio eliminado"})
}

// --- Prompts ---------------------------------------------------------------

func (h *adminHandlers) listPrompts(w http.ResponseWriter, r *http.Request) {
	items, err := h.prompts.List(r.Context())
	if err != nil {
		h.fail(w, "list_prompts_failed", "Error listando prompts", err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *adminHandlers) getPrompt(w http.ResponseWriter, r *http.Request) {
	item, err := h.prompts.Get(r.Context(), pathParam(r, "code"))
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not_found", "Prompt no encontrado")
		return
	}
	if err != nil {
		h.fail(w, "get_prompt_failed", "Error obteniendo prompt", err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *adminHandlers) createPrompt(w http.ResponseWriter, r *http.Request) {
	var p repo.Prompt
	if !h.decode(w, r, &p) {
		return
	}
	if !adminCodeRe.MatchString(p.Code) {
		writeError(w, http.StatusBadRequest, "invalid_code", "Código inválido (minúsculas, números y _)")
		return
	}
	if p.Name == "" {
		p.Name = p.Code
	}
	item, err := h.prompts.Create(r.Context(), p)
	if err != nil {
		h.fail(w, "create_prompt_failed", "Error creando prompt", err)
		return
	}
	h.changed(r.Context(), item.Code)
	writeJSON(w, http.StatusCreated, item)
}

func (h *adminHandlers) updatePrompt(w http.ResponseWriter, r *http.Request) {
	code := pathParam(r, "code")
	var p repo.Prompt
	if !h.decode(w, r, &p) {
		return
	}
	item, err := h.prompts.Update(r.Context(), code, p)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not_found", "Prompt no encontrado")
		return
	}
	if err != nil {
		h.fail(w, "update_prompt_failed", "Error actualizando prompt", err)
		return
	}
	h.changed(r.Context(), code)
	writeJSON(w, http.StatusOK, item)
}

func (h *adminHandlers) deletePrompt(w http.ResponseWriter, r *http.Request) {
	code := pathParam(r, "code")
	err := h.prompts.Delete(r.Context(), code)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not_found", "Prompt no encontrado")
		return
	}
	if err != nil {
		h.fail(w, "delete_prompt_failed", "Error borrando prompt", err)
		return
	}
	if h.content != nil {
		if err := h.content.DeletePromptEmbedding(r.Context(), code); err != nil {
			h.log.Warn("no se pudo borrar el embedding del prompt", "code", code, "error", err.Error())
		}
	}
	h.changed(r.Context(), "")
	writeJSON(w, http.StatusOK, map[string]string{"message": "prompt eliminado"})
}

func (h *adminHandlers) listPromptVersions(w http.ResponseWriter, r *http.Request) {
	items, err := h.prompts.Versions(r.Context(), pathParam(r, "code"))
	if err != nil {
		h.fail(w, "versions_failed", "Error listando versiones", err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *adminHandlers) restorePrompt(w http.ResponseWriter, r *http.Request) {
	code := pathParam(r, "code")
	version, err := strconv.ParseInt(chi.URLParam(r, "version"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_version", "Versión inválida")
		return
	}
	item, err := h.prompts.Restore(r.Context(), code, version)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not_found", "Versión no encontrada")
		return
	}
	if err != nil {
		h.fail(w, "restore_failed", "Error restaurando versión", err)
		return
	}
	h.changed(r.Context(), code)
	writeJSON(w, http.StatusOK, item)
}

// --- Ajustes ---------------------------------------------------------------

type adminSettings struct {
	ActivateNewUsers bool `json:"activate_new_users"`
}

func (h *adminHandlers) getSettings(w http.ResponseWriter, r *http.Request) {
	active, err := h.settings.ActivateNewUsers(r.Context())
	if err != nil {
		h.fail(w, "get_settings_failed", "Error leyendo los ajustes", err)
		return
	}
	writeJSON(w, http.StatusOK, adminSettings{ActivateNewUsers: active})
}

func (h *adminHandlers) updateSettings(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ActivateNewUsers *bool `json:"activate_new_users"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	if body.ActivateNewUsers == nil {
		writeError(w, http.StatusBadRequest, "invalid_settings", "El campo activate_new_users es obligatorio")
		return
	}
	if err := h.settings.SetActivateNewUsers(r.Context(), *body.ActivateNewUsers); err != nil {
		h.fail(w, "update_settings_failed", "Error guardando los ajustes", err)
		return
	}
	h.log.Info("ajuste del panel actualizado", "activate_new_users", *body.ActivateNewUsers)
	writeJSON(w, http.StatusOK, adminSettings{ActivateNewUsers: *body.ActivateNewUsers})
}

// --- Usuarios --------------------------------------------------------------

type adminUser struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Alias       string  `json:"alias"`
	Active      bool    `json:"active"`
	Topic       string  `json:"topic"`
	Lang        string  `json:"lang"`
	Role        int     `json:"role"`
	BufferTime  int     `json:"buffer_time"`
	Temperature float64 `json:"temperature"`
	TestMode    bool    `json:"test_mode"`
}

func toAdminUser(u domain.User) adminUser {
	return adminUser{
		ID:          u.ID,
		Name:        u.Name,
		Alias:       u.Alias,
		Active:      u.Active,
		Topic:       u.Topic,
		Lang:        u.Lang,
		Role:        int(u.Role),
		BufferTime:  u.BufferMs,
		Temperature: u.Temperature,
		TestMode:    u.TestMode,
	}
}

func (h *adminHandlers) listUsers(w http.ResponseWriter, r *http.Request) {
	page := intQuery(r, "page", 1, 0)
	perPage := intQuery(r, "perPage", 20, 200)
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 20
	}
	search := strings.TrimSpace(r.URL.Query().Get("search"))
	active, ok := parseUserAIFilter(w, r)
	if !ok {
		return
	}

	users, total, err := h.users.List(r.Context(), perPage, (page-1)*perPage, repo.UserFilter{Search: search, Active: active})
	if err != nil {
		h.fail(w, "list_users_failed", "Error listando usuarios", err)
		return
	}
	out := make([]adminUser, 0, len(users))
	for _, u := range users {
		out = append(out, toAdminUser(u))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data": out,
		"meta": map[string]any{"total": total, "page": page, "perPage": perPage, "lastPage": (total + perPage - 1) / perPage},
	})
}

func (h *adminHandlers) getUser(w http.ResponseWriter, r *http.Request) {
	u, err := h.users.Get(r.Context(), pathParam(r, "id"))
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not_found", "Usuario no encontrado")
		return
	}
	if err != nil {
		h.fail(w, "get_user_failed", "Error obteniendo usuario", err)
		return
	}
	writeJSON(w, http.StatusOK, toAdminUser(u))
}

func (h *adminHandlers) decodeUser(w http.ResponseWriter, r *http.Request) (domain.User, bool) {
	var body struct {
		ID          string   `json:"id"`
		Name        string   `json:"name"`
		Alias       string   `json:"alias"`
		Active      *bool    `json:"active"`
		Topic       *string  `json:"topic"`
		Lang        *string  `json:"lang"`
		Role        *int     `json:"role"`
		BufferTime  *int     `json:"buffer_time"`
		Temperature *float64 `json:"temperature"`
	}
	if !h.decode(w, r, &body) {
		return domain.User{}, false
	}
	u := domain.User{ID: body.ID, Name: body.Name, Alias: body.Alias, Role: domain.RoleClient}
	if body.Active != nil {
		u.Active = *body.Active
	}
	if body.Topic != nil {
		u.Topic = *body.Topic
	}
	if body.Lang != nil {
		u.Lang = *body.Lang
	}
	if body.Role != nil {
		u.Role = domain.Role(*body.Role)
	}
	if body.BufferTime != nil {
		u.BufferMs = *body.BufferTime
	}
	if body.Temperature != nil {
		u.Temperature = *body.Temperature
	}
	return u, true
}

func (h *adminHandlers) createUser(w http.ResponseWriter, r *http.Request) {
	u, ok := h.decodeUser(w, r)
	if !ok {
		return
	}
	if strings.TrimSpace(u.ID) == "" {
		writeError(w, http.StatusBadRequest, "invalid_id", "El ID (JID) es obligatorio")
		return
	}
	if err := h.users.Create(r.Context(), u); err != nil {
		h.fail(w, "create_user_failed", "Error creando usuario", err)
		return
	}
	writeJSON(w, http.StatusCreated, toAdminUser(u))
}

func (h *adminHandlers) updateUser(w http.ResponseWriter, r *http.Request) {
	id := pathParam(r, "id")
	u, ok := h.decodeUser(w, r)
	if !ok {
		return
	}
	u.ID = id
	if err := h.users.Update(r.Context(), id, u); err != nil {
		h.fail(w, "update_user_failed", "Error actualizando usuario", err)
		return
	}
	got, err := h.users.Get(r.Context(), id)
	if err != nil {
		h.fail(w, "get_user_failed", "Error recargando usuario", err)
		return
	}
	writeJSON(w, http.StatusOK, toAdminUser(got))
}

func (h *adminHandlers) setUserActive(w http.ResponseWriter, r *http.Request) {
	id := pathParam(r, "id")
	var body struct {
		Active *bool `json:"active"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	if body.Active == nil {
		writeError(w, http.StatusBadRequest, "invalid_active", "El campo active es obligatorio")
		return
	}
	if err := h.users.SetActive(r.Context(), id, *body.Active); err != nil {
		h.fail(w, "set_active_failed", "Error actualizando la IA del usuario", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"active": *body.Active})
}

func (h *adminHandlers) deleteUser(w http.ResponseWriter, r *http.Request) {
	id := pathParam(r, "id")
	res, err := h.users.DeleteCascade(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not_found", "Usuario no encontrado")
		return
	}
	if err != nil {
		h.fail(w, "delete_user_failed", "Error eliminando usuario", err)
		return
	}
	purgeUserRuntime(r.Context(), h.log, id, h.history, h.antispam, h.pipeline, h.summarizer)
	h.log.Info("usuario eliminado en cascada", "user", id, "messages", res.Messages, "appointments", res.Appointments, "contacts", res.Contacts)
	writeJSON(w, http.StatusOK, map[string]any{
		"message":      "usuario eliminado",
		"messages":     res.Messages,
		"appointments": res.Appointments,
		"contacts":     res.Contacts,
	})
}

func (h *adminHandlers) importContacts(w http.ResponseWriter, r *http.Request) {
	active, err := h.settings.ActivateNewUsers(r.Context())
	if err != nil {
		h.fail(w, "get_settings_failed", "Error leyendo los ajustes", err)
		return
	}
	n, err := h.users.ImportContacts(r.Context(), active)
	if err != nil {
		h.fail(w, "import_contacts_failed", "Error importando contactos", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"imported": n})
}

// --- Mensajes --------------------------------------------------------------

func (h *adminHandlers) listMessages(w http.ResponseWriter, r *http.Request) {
	page := intQuery(r, "page", 1, 0)
	perPage := intQuery(r, "perPage", 25, 200)
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 25
	}
	search := strings.TrimSpace(r.URL.Query().Get("search"))
	filter := repo.ThreadFilter{
		Search:        search,
		NeedsOperator: r.URL.Query().Get("needs_operator") == "1",
	}
	switch r.URL.Query().Get("ai") {
	case "active":
		v := true
		filter.AIActive = &v
	case "inactive":
		v := false
		filter.AIActive = &v
	}
	rows, total, err := h.interactions.ListThreads(r.Context(), perPage, (page-1)*perPage, filter)
	if err != nil {
		h.fail(w, "list_messages_failed", "Error listando mensajes", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data": rows,
		"meta": map[string]any{"total": total, "page": page, "perPage": perPage, "lastPage": (total + perPage - 1) / perPage},
	})
}

func (h *adminHandlers) conversation(w http.ResponseWriter, r *http.Request) {
	id := pathParam(r, "id")
	rows, err := h.interactions.ListByUser(r.Context(), id, 100)
	if err != nil {
		h.fail(w, "conversation_failed", "Error obteniendo conversación", err)
		return
	}
	var user any
	if h.users != nil {
		if u, err := h.users.Get(r.Context(), id); err == nil {
			user = toAdminUser(u)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": user, "conversation": rows})
}

// operatorSummary es el resumen que ve el operador al abrir un hilo.
type operatorSummary struct {
	UserID        string               `json:"user_id"`
	Topic         string               `json:"topic"`
	AIEnabled     bool                 `json:"ai_enabled"`
	NeedsOperator bool                 `json:"needs_operator"`
	HandoffReason string               `json:"handoff_reason"`
	Appointment   *summary.Appointment `json:"appointment,omitempty"`
	Points        []string             `json:"points"`
	Source        string               `json:"source"`
	Messages      int                  `json:"messages"`
	LastMessageAt *time.Time           `json:"last_message_at,omitempty"`
}

func (h *adminHandlers) messageSummary(w http.ResponseWriter, r *http.Request) {
	id := pathParam(r, "id")
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	rows, err := h.interactions.ListByUser(ctx, id, 24)
	if err != nil {
		h.fail(w, "summary_failed", "Error obteniendo el resumen", err)
		return
	}

	resp := operatorSummary{UserID: id, Points: []string{}, Source: "fallback"}

	var user domain.User
	if h.users != nil {
		if u, err := h.users.Get(ctx, id); err == nil {
			user = u
			resp.AIEnabled = u.Active
			resp.Topic = u.Topic
		}
	}

	if len(rows) > 0 {
		latest := rows[0]
		resp.NeedsOperator = repo.NeedsOperator(latest.ToolCalls)
		resp.HandoffReason = repo.HandoffReason(latest.ToolCalls)
		resp.LastMessageAt = &latest.CreatedAt
		resp.Messages = len(rows)
	}

	if h.appointments != nil {
		if st, err := h.appointments.Get(ctx, id); err == nil &&
			(st.Status != appointments.StatusIdle || st.Service != "" || st.Availability != "") {
			appt := &summary.Appointment{
				Service:      st.Service,
				Availability: st.Availability,
				Team:         st.Team,
				Status:       string(st.Status),
			}
			if st.Service != "" && h.services != nil {
				if sv, err := h.services.Get(ctx, st.Service); err == nil {
					appt.ServiceName = sv.Name
				}
			}
			resp.Appointment = appt
		}
	}

	messages := make([]summary.Message, 0, len(rows)*2)
	for i := len(rows) - 1; i >= 0; i-- {
		row := rows[i]
		if strings.TrimSpace(row.UserMessage) != "" {
			messages = append(messages, summary.Message{Role: "Cliente", Text: row.UserMessage, At: row.CreatedAt})
		}
		if strings.TrimSpace(row.AIResponse) != "" {
			role := "Bot"
			if row.Model == "manual" {
				role = "Operador"
			}
			messages = append(messages, summary.Message{Role: role, Text: row.AIResponse, At: row.CreatedAt})
		}
	}

	key := ""
	if len(rows) > 0 {
		key = fmt.Sprintf("%s:%d", id, rows[0].ID)
	}
	res := h.summarizer.Summarize(ctx, key, summary.Input{
		UserName:      user.Name,
		Topic:         user.Topic,
		NeedsOperator: resp.NeedsOperator,
		HandoffReason: resp.HandoffReason,
		Appointment:   resp.Appointment,
		Messages:      messages,
	})
	if res.Points != nil {
		resp.Points = res.Points
	}
	resp.Source = res.Source

	writeJSON(w, http.StatusOK, resp)
}

func (h *adminHandlers) sendMessage(w http.ResponseWriter, r *http.Request) {
	id := pathParam(r, "id")
	var body struct {
		Text string `json:"text"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	text := strings.TrimSpace(body.Text)
	if text == "" {
		writeError(w, http.StatusBadRequest, "invalid_text", "El texto es obligatorio")
		return
	}
	if h.wa == nil {
		writeError(w, http.StatusServiceUnavailable, "whatsapp_disabled", "WhatsApp no está habilitado")
		return
	}
	operator := sanitizeOperator(h.opts.OperatorName)
	if cookie, err := r.Cookie(adminCookieName); err == nil {
		if name, ok := h.session.parse(cookie.Value, time.Now()); ok && name != "" {
			operator = name
		}
	}
	out := text
	if operator != "" {
		out = "*" + titleCase(operator) + "*\n\n" + text
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	if _, err := h.wa.SendText(ctx, id, out); err != nil {
		h.log.Error("error enviando mensaje", "user", id, "error", err.Error())
		writeError(w, http.StatusBadGateway, "send_failed", err.Error())
		return
	}
	if h.users != nil {
		if err := h.users.SetActive(ctx, id, false); err != nil {
			h.log.Warn("no se pudo desactivar la IA tras la respuesta manual", "user", id, "error", err.Error())
		}
	}
	if h.interactions != nil {
		_ = h.interactions.Log(ctx, domain.Interaction{
			UserID:      id,
			SessionID:   id,
			UserMessage: "",
			AIResponse:  out,
			Model:       "manual",
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

// --- Realtime (SSE) ---------------------------------------------------------

// eventsStream envía por SSE los eventos de conversación (mensajes entrantes,
// respuestas persistidas y estado de "escribiendo"). Requiere sesión de panel.
func (h *adminHandlers) eventsStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "sse_unsupported", "SSE no soportado")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	rc := http.NewResponseController(w)
	extend := func() { _ = rc.SetWriteDeadline(time.Now().Add(60 * time.Second)) }

	// Reintento del navegador tras una caída.
	extend()
	_, _ = fmt.Fprint(w, "retry: 3000\n\n")
	flusher.Flush()

	ch, unsubscribe := h.events.Subscribe()
	defer unsubscribe()

	heartbeat := time.NewTicker(20 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case ev, open := <-ch:
			if !open {
				return
			}
			payload, err := json.Marshal(ev)
			if err != nil {
				continue
			}
			extend()
			if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, payload); err != nil {
				return
			}
			flusher.Flush()
		case <-heartbeat.C:
			extend()
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// --- WhatsApp --------------------------------------------------------------

func (h *adminHandlers) whatsappStatus(w http.ResponseWriter, _ *http.Request) {
	if h.wa == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"enabled": false, "status": string(whatsapp.StatusDisconnected), "qr_available": false, "paired": false,
		})
		return
	}
	st := h.wa.Status()
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled":      true,
		"status":       string(st),
		"qr_available": h.wa.QRCode() != "",
		"paired":       st == whatsapp.StatusConnected,
	})
}

func (h *adminHandlers) whatsappQR(w http.ResponseWriter, _ *http.Request) {
	if h.wa == nil || h.wa.QRCode() == "" {
		writeError(w, http.StatusNotFound, "no_qr", "No hay código QR disponible")
		return
	}
	png, err := qrcode.Encode(h.wa.QRCode(), qrcode.Medium, 320)
	if err != nil {
		h.fail(w, "qr_failed", "No se pudo generar el QR", err)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(png)
}

// --- helpers ---------------------------------------------------------------

// companyRefresher implementa el refresco en caliente de la ubicación del
// runner de acciones a partir del prompt de empresa.
type companyRefresher interface {
	RefreshCompany(ctx context.Context) error
}

func (h *adminHandlers) changed(ctx context.Context, promptCode string) {
	if h.content == nil {
		return
	}
	if err := h.content.ReloadContent(ctx); err != nil {
		h.log.Error("no se pudo recargar el contenido del asistente", "error", err.Error())
	}
	if promptCode != "" {
		if err := h.content.ReindexPrompt(ctx, promptCode); err != nil {
			h.log.Warn("no se pudo reindexar el prompt", "code", promptCode, "error", err.Error())
		}
	}
	if promptCode == "" || promptCode == repo.CompanyPromptCode {
		if cr, ok := h.content.(companyRefresher); ok {
			if err := cr.RefreshCompany(ctx); err != nil {
				h.log.Warn("no se pudo refrescar la ubicación de empresa", "error", err.Error())
			}
		}
	}
}

// pathParam devuelve un parámetro de ruta decodificado. chi usa RawPath, por lo
// que los valores llegan con escapes (p. ej. `%40` en los JID `@lid`).
func pathParam(r *http.Request, name string) string {
	v := chi.URLParam(r, name)
	if dec, err := url.PathUnescape(v); err == nil {
		return dec
	}
	return v
}

func (h *adminHandlers) decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "JSON inválido")
		return false
	}
	return true
}

func (h *adminHandlers) fail(w http.ResponseWriter, code, msg string, err error) {
	h.log.Error(msg, "error", err.Error())
	writeError(w, http.StatusInternalServerError, code, msg)
}
