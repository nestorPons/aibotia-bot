// Package postgres gestiona la conexión a PostgreSQL mediante pgxpool.
package postgres

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Connect abre un pool de conexiones y verifica conectividad con reintentos.
func Connect(ctx context.Context, url string, log *slog.Logger) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("postgres: parse config: %w", err)
	}
	cfg.MaxConns = 10
	cfg.MinConns = 1
	cfg.MaxConnLifetime = time.Hour
	cfg.MaxConnIdleTime = 30 * time.Minute
	cfg.HealthCheckPeriod = time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("postgres: new pool: %w", err)
	}

	const maxAttempts = 10
	backoff := 500 * time.Millisecond
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err = pool.Ping(pingCtx)
		cancel()
		if err == nil {
			if log != nil {
				log.Info("conexión a PostgreSQL establecida")
			}
			return pool, nil
		}
		if attempt == maxAttempts {
			break
		}
		if log != nil {
			log.Warn("esperando a PostgreSQL", "intento", attempt, "error", err.Error())
		}
		select {
		case <-ctx.Done():
			pool.Close()
			return nil, ctx.Err()
		case <-time.After(backoff):
		}
		if backoff < 8*time.Second {
			backoff *= 2
		}
	}

	pool.Close()
	return nil, fmt.Errorf("postgres: no se pudo conectar tras %d intentos: %w", maxAttempts, err)
}
