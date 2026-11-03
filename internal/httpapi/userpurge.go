package httpapi

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/aibotia/aibotia-go/internal/pipeline"
	"github.com/aibotia/aibotia-go/internal/security"
	"github.com/aibotia/aibotia-go/internal/store/repo"
	"github.com/aibotia/aibotia-go/internal/summary"
)

// parseUserAIFilter traduce ?ai=active|inactive a *bool. Escribe el 400 si el
// valor no es válido y devuelve ok=false.
func parseUserAIFilter(w http.ResponseWriter, r *http.Request) (*bool, bool) {
	var active *bool
	switch r.URL.Query().Get("ai") {
	case "":
		return nil, true
	case "active":
		v := true
		active = &v
	case "inactive":
		v := false
		active = &v
	default:
		writeError(w, http.StatusBadRequest, "invalid_filter", "El filtro ai debe ser active o inactive")
		return nil, false
	}
	return active, true
}

// purgeUserRuntime limpia los datos en memoria y en Redis de un usuario que se
// acaba de borrar de PostgreSQL.
func purgeUserRuntime(ctx context.Context, log *slog.Logger, id string, history *repo.History, spam *security.AntiSpam, pl *pipeline.Service, sum *summary.Summarizer) {
	if history != nil {
		if err := history.Clear(ctx, id); err != nil {
			log.Warn("no se pudo borrar el historial de Redis", "user", id, "error", err.Error())
		}
	}
	if spam != nil {
		if err := spam.Reset(ctx, id); err != nil {
			log.Warn("no se pudo borrar el anti-spam de Redis", "user", id, "error", err.Error())
		}
	}
	if pl != nil {
		pl.Cancel(id)
	}
	if sum != nil {
		sum.Reset(id)
	}
}
