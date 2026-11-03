package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/aibotia/aibotia-go/internal/store/repo"
)

type messageHandlers struct {
	log          *slog.Logger
	interactions *repo.Interactions
}

func newMessageHandlers(log *slog.Logger, interactions *repo.Interactions) *messageHandlers {
	return &messageHandlers{log: log, interactions: interactions}
}

func (h *messageHandlers) list(w http.ResponseWriter, r *http.Request) {
	page := intQuery(r, "page", 1, 0)
	perPage := intQuery(r, "perPage", 50, 500)
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 50
	}
	rows, err := h.interactions.List(r.Context(), perPage, (page-1)*perPage)
	if err != nil {
		h.log.Error("error listando mensajes", "error", err.Error())
		writeError(w, http.StatusInternalServerError, "list_failed", "Error listando mensajes")
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (h *messageHandlers) byUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	rows, err := h.interactions.ListByUser(r.Context(), id, 100)
	if err != nil {
		h.log.Error("error listando mensajes de usuario", "error", err.Error())
		writeError(w, http.StatusInternalServerError, "list_failed", "Error listando mensajes")
		return
	}
	writeJSON(w, http.StatusOK, rows)
}
