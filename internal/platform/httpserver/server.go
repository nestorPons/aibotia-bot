// Package httpserver gestiona el ciclo de vida del servidor HTTP.
package httpserver

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// Serve arranca el servidor y espera a que el contexto se cancele para
// realizar un apagado ordenado. Devuelve error si el listen/shutdown falla.
func Serve(ctx context.Context, log *slog.Logger, srv *http.Server, shutdownTimeout time.Duration) error {
	errCh := make(chan error, 1)

	go func() {
		log.Info("servidor HTTP escuchando", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("httpserver: listen: %w", err)
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	log.Info("apagando servidor HTTP...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("httpserver: shutdown: %w", err)
	}
	log.Info("servidor HTTP detenido")
	return nil
}
