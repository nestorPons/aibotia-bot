package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/aibotia/aibotia-go/internal/store/repo"
)

type assistantHandlers struct {
	log      *slog.Logger
	settings *repo.Settings
}

func newAssistantHandlers(log *slog.Logger, settings *repo.Settings) *assistantHandlers {
	return &assistantHandlers{log: log, settings: settings}
}

func (h *assistantHandlers) status(w http.ResponseWriter, r *http.Request) {
	active, err := h.settings.IsActive(r.Context())
	if err != nil {
		h.log.Error("error leyendo estado del bot", "error", err.Error())
		writeError(w, http.StatusInternalServerError, "status_failed", "Error leyendo estado")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"active": active})
}

func (h *assistantHandlers) setStatus(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Active *bool `json:"active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Active == nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Se requiere 'active' (boolean)")
		return
	}
	if err := h.settings.SetActive(r.Context(), *body.Active); err != nil {
		h.log.Error("error actualizando estado del bot", "error", err.Error())
		writeError(w, http.StatusInternalServerError, "update_failed", "Error actualizando estado")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "active": *body.Active})
}
