package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/aibotia/aibotia-go/internal/store/repo"
)

type instructionHandlers struct {
	log          *slog.Logger
	instructions *repo.Instructions
}

func newInstructionHandlers(log *slog.Logger, instructions *repo.Instructions) *instructionHandlers {
	return &instructionHandlers{log: log, instructions: instructions}
}

func (h *instructionHandlers) list(w http.ResponseWriter, r *http.Request) {
	blocks, err := h.instructions.List(r.Context())
	if err != nil {
		h.log.Error("error listando instrucciones", "error", err.Error())
		writeError(w, http.StatusInternalServerError, "list_failed", "Error listando instrucciones")
		return
	}
	writeJSON(w, http.StatusOK, blocks)
}

func (h *instructionHandlers) get(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	block, err := h.instructions.GetByCode(r.Context(), code)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not_found", "Bloque no encontrado")
		return
	}
	if err != nil {
		h.log.Error("error obteniendo instrucción", "error", err.Error())
		writeError(w, http.StatusInternalServerError, "get_failed", "Error obteniendo instrucción")
		return
	}
	writeJSON(w, http.StatusOK, block)
}

func (h *instructionHandlers) update(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")

	var body struct {
		Block        repo.Block         `json:"block"`
		Translations []repo.Translation `json:"translations"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "JSON inválido")
		return
	}

	block, err := h.instructions.Update(r.Context(), code, body.Block, body.Translations)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not_found", "Bloque no encontrado")
		return
	}
	if err != nil {
		h.log.Error("error actualizando instrucción", "error", err.Error())
		writeError(w, http.StatusInternalServerError, "update_failed", "Error actualizando instrucción")
		return
	}
	writeJSON(w, http.StatusOK, block)
}

func (h *instructionHandlers) delete(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	err := h.instructions.Delete(r.Context(), code)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not_found", "Bloque no encontrado")
		return
	}
	if err != nil {
		h.log.Error("error borrando instrucción", "error", err.Error())
		writeError(w, http.StatusInternalServerError, "delete_failed", "Error borrando instrucción")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "bloque eliminado"})
}
