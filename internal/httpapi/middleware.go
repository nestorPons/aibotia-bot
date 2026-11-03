package httpapi

import (
	"crypto/subtle"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

// apiKeyAuth exige la cabecera x-api-key con comparación constant-time.
// Las rutas exentas (p. ej. /health, /ready) deben registrarse fuera de este grupo.
func apiKeyAuth(expected string) func(http.Handler) http.Handler {
	want := []byte(expected)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got := []byte(r.Header.Get("x-api-key"))
			if len(got) == 0 || subtle.ConstantTimeCompare(got, want) != 1 {
				writeError(w, http.StatusForbidden, "unauthorized", "No autorizado")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// apiKeyAuthQuery autentica por cabecera x-api-key o por query ?key= (para SSE).
func apiKeyAuthQuery(expected string, next http.HandlerFunc) http.HandlerFunc {
	want := []byte(expected)
	return func(w http.ResponseWriter, r *http.Request) {
		got := []byte(r.Header.Get("x-api-key"))
		if len(got) == 0 {
			got = []byte(r.URL.Query().Get("key"))
		}
		if len(got) == 0 || subtle.ConstantTimeCompare(got, want) != 1 {
			writeError(w, http.StatusForbidden, "unauthorized", "No autorizado")
			return
		}
		next(w, r)
	}
}

// requestTimeout aplica un límite de tiempo a las peticiones normales. Las
// rutas SSE (/events) quedan exentas porque son streams de larga duración.
func requestTimeout(d time.Duration) func(http.Handler) http.Handler {
	timeout := middleware.Timeout(d)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/events") {
				next.ServeHTTP(w, r)
				return
			}
			timeout(next).ServeHTTP(w, r)
		})
	}
}

// requestLogger registra cada petición con slog.
func requestLogger(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			log.Info("http",
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.Status(),
				"bytes", ww.BytesWritten(),
				"duration_ms", time.Since(start).Milliseconds(),
				"request_id", middleware.GetReqID(r.Context()),
			)
		})
	}
}

// recoverer evita que un panic tumbe el servidor.
func recoverer(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					log.Error("panic recuperado",
						"panic", rec,
						"path", r.URL.Path,
						"request_id", middleware.GetReqID(r.Context()),
					)
					writeError(w, http.StatusInternalServerError, "internal_error", "Error interno")
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// cors aplica CORS restringido a los orígenes permitidos.
// Si la lista contiene "*" se permite cualquier origen.
func cors(allowed []string) func(http.Handler) http.Handler {
	allowAll := slices.Contains(allowed, "*")
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" {
				if allowAll {
					w.Header().Set("Access-Control-Allow-Origin", "*")
				} else if slices.Contains(allowed, origin) {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Set("Vary", "Origin")
				}
			}
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, x-api-key")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
