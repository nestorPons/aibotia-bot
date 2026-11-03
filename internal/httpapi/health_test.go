package httpapi

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestHealthLiveness(t *testing.T) {
	h := NewHealthHandler(discardLogger(), "test", nil)
	rec := httptest.NewRecorder()
	h.Health(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, quiero %d", rec.Code, http.StatusOK)
	}
}

func TestReadyDegradedWhenCheckFails(t *testing.T) {
	h := NewHealthHandler(discardLogger(), "test", map[string]ReadyChecker{
		"db": func(context.Context) error { return errors.New("caído") },
	})
	rec := httptest.NewRecorder()
	h.Ready(rec, httptest.NewRequest(http.MethodGet, "/ready", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, quiero %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestReadyOK(t *testing.T) {
	h := NewHealthHandler(discardLogger(), "test", map[string]ReadyChecker{
		"db": func(context.Context) error { return nil },
	})
	rec := httptest.NewRecorder()
	h.Ready(rec, httptest.NewRequest(http.MethodGet, "/ready", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, quiero %d", rec.Code, http.StatusOK)
	}
}
