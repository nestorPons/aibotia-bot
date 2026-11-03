package realtime

import (
	"testing"
	"time"
)

func TestHubDeliversToSubscribers(t *testing.T) {
	h := NewHub()
	ch, unsubscribe := h.Subscribe()
	defer unsubscribe()

	want := Event{Type: TypeMessage, UserID: "user@lid"}
	h.Publish(want)

	select {
	case got := <-ch:
		if got.Type != want.Type || got.UserID != want.UserID {
			t.Fatalf("evento = %+v, quiero %+v", got, want)
		}
		if got.At.IsZero() {
			t.Fatal("el evento debe llevar marca de tiempo")
		}
	case <-time.After(time.Second):
		t.Fatal("no llegó el evento")
	}
}

func TestHubUnsubscribeClosesChannel(t *testing.T) {
	h := NewHub()
	ch, unsubscribe := h.Subscribe()
	unsubscribe()

	if _, open := <-ch; open {
		t.Fatal("el canal debe cerrarse al darse de baja")
	}

	// Publicar sin suscriptores no debe bloquear ni entrar en pánico.
	done := make(chan struct{})
	go func() {
		h.Publish(Event{Type: TypeIncoming, UserID: "x"})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Publish bloqueado sin suscriptores")
	}
}

func TestNilHubDoesNotPanic(t *testing.T) {
	var h *Hub
	ch, unsubscribe := h.Subscribe()
	defer unsubscribe()
	h.Publish(Event{Type: TypeIncoming})
	select {
	case <-ch:
		t.Fatal("un hub nulo no debe entregar eventos")
	case <-time.After(20 * time.Millisecond):
	}
}
