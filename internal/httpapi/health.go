package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

// ReadyChecker comprueba una dependencia. Devuelve error si no está lista.
type ReadyChecker func(ctx context.Context) error

// HealthHandler responde a los endpoints de liveness y readiness.
type HealthHandler struct {
	log     *slog.Logger
	version string
	checks  map[string]ReadyChecker
}

// NewHealthHandler construye el handler de salud.
func NewHealthHandler(log *slog.Logger, version string, checks map[string]ReadyChecker) *HealthHandler {
	return &HealthHandler{log: log, version: version, checks: checks}
}

// Health es el endpoint de liveness: el proceso está vivo.
func (h *HealthHandler) Health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"version": h.version,
	})
}

// Ready es el endpoint de readiness: las dependencias responden.
func (h *HealthHandler) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	result := make(map[string]string, len(h.checks))
	ready := true
	for name, check := range h.checks {
		if err := check(ctx); err != nil {
			ready = false
			result[name] = "error: " + err.Error()
			h.log.Warn("check de readiness falló", "dependency", name, "error", err.Error())
			continue
		}
		result[name] = "ok"
	}

	status := http.StatusOK
	overall := "ok"
	if !ready {
		status = http.StatusServiceUnavailable
		overall = "degraded"
	}
	writeJSON(w, status, map[string]any{
		"status":       overall,
		"dependencies": result,
	})
}
