package httpapi

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/aibotia/aibotia-go/internal/whatsapp"
)

type eventHandlers struct {
	log *slog.Logger
	wa  whatsapp.Provider
}

func newEventHandlers(log *slog.Logger, wa whatsapp.Provider) *eventHandlers {
	return &eventHandlers{log: log, wa: wa}
}

// stream envía por SSE los cambios de estado/QR de WhatsApp.
func (h *eventHandlers) stream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "sse_unsupported", "SSE no soportado")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	send := func(event string, payload any) {
		data, _ := json.Marshal(payload)
		_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
		flusher.Flush()
	}

	var lastStatus, lastQR string
	emit := func() {
		if h.wa == nil {
			return
		}
		if status := string(h.wa.Status()); status != lastStatus {
			send("status", map[string]string{"status": status})
			lastStatus = status
		}
		qr := h.wa.QRCode()
		if qr != lastQR {
			if qr != "" {
				send("qr", map[string]string{"qr": qr})
			}
			lastQR = qr
		}
	}

	emit()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			emit()
		}
	}
}
