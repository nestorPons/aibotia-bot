package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/aibotia/aibotia-go/internal/pipeline"
	"github.com/aibotia/aibotia-go/internal/security"
	"github.com/aibotia/aibotia-go/internal/store/repo"
	"github.com/aibotia/aibotia-go/internal/summary"
	"github.com/aibotia/aibotia-go/internal/whatsapp"
)

type userHandlers struct {
	log          *slog.Logger
	users        *repo.Users
	settings     *repo.Settings
	history      *repo.History
	interactions *repo.Interactions
	provider     whatsapp.Provider
	pipeline     *pipeline.Service
	spam         *security.AntiSpam
	summarizer   *summary.Summarizer
}

func newUserHandlers(log *slog.Logger, users *repo.Users, settings *repo.Settings, history *repo.History, interactions *repo.Interactions, provider whatsapp.Provider, pl *pipeline.Service, spam *security.AntiSpam, summarizer *summary.Summarizer) *userHandlers {
	return &userHandlers{log: log, users: users, settings: settings, history: history, interactions: interactions, provider: provider, pipeline: pl, spam: spam, summarizer: summarizer}
}

func (h *userHandlers) forceReply(w http.ResponseWriter, r *http.Request) {
	if h.pipeline == nil {
		writeError(w, http.StatusServiceUnavailable, "pipeline_unavailable", "El pipeline no está disponible")
		return
	}
	id := pathParam(r, "id")

	var body struct {
		Text string `json:"text"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	reply, err := h.pipeline.ForceReply(r.Context(), id, body.Text)
	if err != nil {
		h.log.Error("error en force-reply", "user", id, "error", err.Error())
		writeError(w, http.StatusBadGateway, "force_reply_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"text":    reply.Text,
		"actions": reply.Actions,
	})
}

func (h *userHandlers) importUsers(w http.ResponseWriter, r *http.Request) {
	if h.provider == nil {
		writeError(w, http.StatusServiceUnavailable, "whatsapp_disabled", "WhatsApp no está habilitado")
		return
	}
	ctx := r.Context()
	contacts, err := h.provider.ListContacts(ctx)
	if err != nil {
		h.log.Error("error listando contactos", "error", err.Error())
		writeError(w, http.StatusBadGateway, "list_contacts_failed", err.Error())
		return
	}

	imported := make([]string, 0, len(contacts))
	activateNew := true
	if h.settings != nil {
		if v, serr := h.settings.ActivateNewUsers(ctx); serr == nil {
			activateNew = v
		}
	}
	for _, c := range contacts {
		if strings.TrimSpace(c.JID) == "" {
			continue
		}
		name := c.FullName
		if strings.TrimSpace(name) == "" {
			name = c.PushName
		}
		if strings.TrimSpace(name) == "" {
			name = jidToPhone(c.JID)
		}
		if _, err := h.users.FindOrCreate(ctx, c.JID, name, activateNew); err != nil {
			h.log.Warn("no se pudo importar contacto", "jid", c.JID, "error", err.Error())
			continue
		}
		imported = append(imported, c.JID)
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "count": len(imported), "imported": imported})
}

func jidToPhone(jid string) string {
	user := jid
	if i := strings.IndexByte(user, '@'); i >= 0 {
		user = user[:i]
	}
	return strings.TrimPrefix(user, "34")
}

func (h *userHandlers) list(w http.ResponseWriter, r *http.Request) {
	page := intQuery(r, "page", 1, 0)
	perPage := intQuery(r, "perPage", 20, 200)
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 20
	}
	filter := repo.UserFilter{Search: strings.TrimSpace(r.URL.Query().Get("search"))}
	active, ok := parseUserAIFilter(w, r)
	if !ok {
		return
	}
	filter.Active = active

	users, total, err := h.users.List(r.Context(), perPage, (page-1)*perPage, filter)
	if err != nil {
		h.log.Error("error listando usuarios", "error", err.Error())
		writeError(w, http.StatusInternalServerError, "list_failed", "No se pudieron listar los usuarios")
		return
	}
	lastPage := (total + perPage - 1) / perPage
	writeJSON(w, http.StatusOK, map[string]any{
		"data": users,
		"meta": map[string]any{"total": total, "page": page, "perPage": perPage, "lastPage": lastPage},
	})
}

func (h *userHandlers) get(w http.ResponseWriter, r *http.Request) {
	id := pathParam(r, "id")
	user, err := h.users.Get(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not_found", "Usuario no encontrado")
		return
	}
	if err != nil {
		h.log.Error("error obteniendo usuario", "error", err.Error())
		writeError(w, http.StatusInternalServerError, "get_failed", "Error obteniendo usuario")
		return
	}
	writeJSON(w, http.StatusOK, user)
}

func (h *userHandlers) update(w http.ResponseWriter, r *http.Request) {
	id := pathParam(r, "id")

	var body struct {
		Active *bool   `json:"active"`
		Name   *string `json:"name"`
		Topic  *string `json:"topic"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "JSON inválido")
		return
	}

	ctx := r.Context()
	if body.Active != nil {
		if err := h.users.SetActive(ctx, id, *body.Active); err != nil {
			writeError(w, http.StatusInternalServerError, "update_failed", "Error actualizando usuario")
			return
		}
	}
	if body.Name != nil {
		_ = h.users.UpdateName(ctx, id, *body.Name)
	}
	if body.Topic != nil {
		_ = h.users.SetTopic(ctx, id, *body.Topic)
	}

	user, err := h.users.Get(ctx, id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Usuario no encontrado")
		return
	}
	writeJSON(w, http.StatusOK, user)
}

// setTestMode activa o desactiva el modo prueba de un usuario. Durante una
// conversación de pruebas el bot no desactiva la IA al derivar a un operador.
func (h *userHandlers) setTestMode(w http.ResponseWriter, r *http.Request) {
	id := pathParam(r, "id")

	var body struct {
		Enabled *bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "JSON inválido")
		return
	}
	if body.Enabled == nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "El campo enabled es obligatorio")
		return
	}
	if err := h.users.SetTestMode(r.Context(), id, *body.Enabled); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "not_found", "Usuario no encontrado")
			return
		}
		h.log.Error("error actualizando el modo prueba", "user", id, "error", err.Error())
		writeError(w, http.StatusInternalServerError, "update_failed", "Error actualizando el modo prueba")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "test_mode": *body.Enabled})
}

func (h *userHandlers) delete(w http.ResponseWriter, r *http.Request) {
	id := pathParam(r, "id")
	res, err := h.users.DeleteCascade(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not_found", "Usuario no encontrado")
		return
	}
	if err != nil {
		h.log.Error("error eliminando usuario", "error", err.Error())
		writeError(w, http.StatusInternalServerError, "delete_failed", "Error eliminando usuario")
		return
	}
	purgeUserRuntime(r.Context(), h.log, id, h.history, h.spam, h.pipeline, h.summarizer)
	writeJSON(w, http.StatusOK, map[string]any{
		"success":      true,
		"message":      "usuario eliminado",
		"messages":     res.Messages,
		"appointments": res.Appointments,
		"contacts":     res.Contacts,
	})
}

func (h *userHandlers) clearHistory(w http.ResponseWriter, r *http.Request) {
	id := pathParam(r, "id")
	if err := h.history.Clear(r.Context(), id); err != nil {
		h.log.Error("error limpiando historial", "error", err.Error())
		writeError(w, http.StatusInternalServerError, "clear_failed", "Error limpiando historial")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (h *userHandlers) conversation(w http.ResponseWriter, r *http.Request) {
	id := pathParam(r, "id")
	user, err := h.users.Get(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not_found", "Usuario no encontrado")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "get_failed", "Error obteniendo usuario")
		return
	}
	rows, err := h.interactions.ListByUser(r.Context(), id, 50)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "conversation_failed", "Error obteniendo conversación")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": user, "conversation": rows})
}
