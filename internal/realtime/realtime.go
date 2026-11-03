// Package realtime distribuye eventos de conversación en tiempo real hacia los
// clientes SSE (panel de administración). Es un bus en memoria: el proceso que
// escribe las interacciones (botd) es el mismo que sirve el panel.
package realtime

import (
	"sync"
	"time"
)

// Tipos de evento publicados por el sistema.
const (
	TypeIncoming = "incoming" // mensaje entrante del cliente (antes de persistir)
	TypeMessage  = "message"  // interacción persistida (cliente + respuesta IA/operador)
	TypeTyping   = "typing"   // el bot está generando una respuesta
)

// Event es la unidad que se envía por SSE.
type Event struct {
	Type   string    `json:"type"`
	UserID string    `json:"user_id,omitempty"`
	At     time.Time `json:"at"`
	Data   any       `json:"data,omitempty"`
}

// Incoming describe un mensaje recién recibido de WhatsApp.
type Incoming struct {
	Text      string `json:"text"`
	HasMedia  bool   `json:"has_media"`
	MediaType string `json:"media_type,omitempty"`
	FromMe    bool   `json:"from_me"`
	PushName  string `json:"push_name,omitempty"`
}

// Typing indica si el bot está escribiendo en una conversación.
type Typing struct {
	Typing bool `json:"typing"`
}

// Hub hace fan-out de eventos a todos los suscriptores activos.
type Hub struct {
	mu   sync.RWMutex
	subs map[chan Event]struct{}
}

// NewHub construye un hub listo para usar.
func NewHub() *Hub {
	return &Hub{subs: make(map[chan Event]struct{})}
}

// Subscribe devuelve un canal de eventos y una función para darse de baja.
func (h *Hub) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, 64)
	if h == nil {
		return ch, func() {}
	}
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		if _, ok := h.subs[ch]; ok {
			delete(h.subs, ch)
			close(ch)
		}
		h.mu.Unlock()
	}
}

// Publish entrega el evento a todos los suscriptores sin bloquear. Si un
// suscriptor está saturado se descarta el evento para ese cliente; el SSE
// reenviará el estado completo al reconectar.
func (h *Hub) Publish(e Event) {
	if h == nil {
		return
	}
	if e.At.IsZero() {
		e.At = time.Now()
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.subs {
		select {
		case ch <- e:
		default:
		}
	}
}
