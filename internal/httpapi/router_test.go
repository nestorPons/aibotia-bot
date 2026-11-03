package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aibotia/aibotia-go/internal/config"
)

func testRouter() http.Handler {
	return NewRouter(Deps{
		Log:     discardLogger(),
		Config:  &config.Config{APIKey: "k", CORSAllowedOrigins: []string{"*"}},
		Version: "test",
	})
}

func TestHealthIsPublic(t *testing.T) {
	rec := httptest.NewRecorder()
	testRouter().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, quiero 200", rec.Code)
	}
}

func TestProtectedRequiresAPIKey(t *testing.T) {
	rec := httptest.NewRecorder()
	testRouter().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/ping", nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, quiero 403", rec.Code)
	}
}

func TestProtectedWithAPIKey(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ping", nil)
	req.Header.Set("x-api-key", "k")
	rec := httptest.NewRecorder()
	testRouter().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, quiero 200", rec.Code)
	}
}
