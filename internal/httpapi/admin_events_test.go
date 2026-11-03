package httpapi

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aibotia/aibotia-go/internal/realtime"
)

func TestEventsStreamEnvíaEventos(t *testing.T) {
	hub := realtime.NewHub()
	h := &adminHandlers{log: discardLogger(), events: hub}

	srv := httptest.NewServer(http.HandlerFunc(h.eventsStream))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("content-type = %q", ct)
	}

	reader := bufio.NewReader(resp.Body)
	// El primer frame es el retry del navegador.
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("leyendo retry: %v", err)
	}
	if !strings.HasPrefix(line, "retry:") {
		t.Fatalf("primera línea = %q", line)
	}

	hub.Publish(realtime.Event{Type: realtime.TypeIncoming, UserID: "user@lid", At: time.Now()})

	deadline := time.Now().Add(2 * time.Second)
	var gotEvent, gotData string
	for time.Now().Before(deadline) {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("leyendo stream: %v", err)
		}
		line = strings.TrimRight(line, "\n")
		switch {
		case strings.HasPrefix(line, "event: "):
			gotEvent = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			gotData = strings.TrimPrefix(line, "data: ")
		}
		if gotEvent != "" && gotData != "" {
			break
		}
	}
	if gotEvent != realtime.TypeIncoming {
		t.Fatalf("evento = %q, quiero %q", gotEvent, realtime.TypeIncoming)
	}
	if !strings.Contains(gotData, `"user_id":"user@lid"`) {
		t.Fatalf("data = %q", gotData)
	}
}
