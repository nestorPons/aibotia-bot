package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/aibotia/aibotia-go/internal/whatsapp"
)

type whatsAppHandlers struct {
	log *slog.Logger
	wa  whatsapp.Provider
}

func newWhatsAppHandlers(log *slog.Logger, wa whatsapp.Provider) *whatsAppHandlers {
	return &whatsAppHandlers{log: log, wa: wa}
}

func (h *whatsAppHandlers) status(w http.ResponseWriter, _ *http.Request) {
	if h.wa == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"enabled": false,
			"status":  string(whatsapp.StatusDisconnected),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled":      true,
		"status":       string(h.wa.Status()),
		"qr_available": h.wa.QRCode() != "",
	})
}

func (h *whatsAppHandlers) qr(w http.ResponseWriter, _ *http.Request) {
	if h.wa == nil {
		writeError(w, http.StatusServiceUnavailable, "whatsapp_disabled", "WhatsApp no está habilitado")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status": string(h.wa.Status()),
		"qr":     h.wa.QRCode(),
	})
}

func (h *whatsAppHandlers) contact(w http.ResponseWriter, r *http.Request) {
	if h.wa == nil {
		writeError(w, http.StatusServiceUnavailable, "whatsapp_disabled", "WhatsApp no está habilitado")
		return
	}
	jid := r.URL.Query().Get("jid")
	if jid == "" {
		writeError(w, http.StatusBadRequest, "missing_jid", "El parámetro 'jid' es obligatorio")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	contact, err := h.wa.GetContact(ctx, jid)
	if err != nil {
		h.log.Warn("error obteniendo contacto", "jid", jid, "error", err.Error())
		writeError(w, http.StatusBadGateway, "contact_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, contact)
}

func (h *whatsAppHandlers) logout(w http.ResponseWriter, r *http.Request) {
	if h.wa == nil {
		writeError(w, http.StatusServiceUnavailable, "whatsapp_disabled", "WhatsApp no está habilitado")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	if err := h.wa.Logout(ctx); err != nil {
		h.log.Warn("error cerrando sesión de WhatsApp", "error", err.Error())
		writeError(w, http.StatusBadGateway, "logout_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": string(h.wa.Status())})
}
