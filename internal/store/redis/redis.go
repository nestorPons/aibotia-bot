// Package redisstore gestiona la conexión a Redis.
package redisstore

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

// Connect abre y verifica una conexión a Redis con reintentos.
func Connect(ctx context.Context, url string, log *slog.Logger) (*redis.Client, error) {
	opt, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("redis: URL inválida: %w", err)
	}
	client := redis.NewClient(opt)

	const maxAttempts = 10
	backoff := 500 * time.Millisecond
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		err = client.Ping(pingCtx).Err()
		cancel()
		if err == nil {
			if log != nil {
				log.Info("conexión a Redis establecida")
			}
			return client, nil
		}
		if attempt == maxAttempts {
			break
		}
		if log != nil {
			log.Warn("esperando a Redis", "intento", attempt, "error", err.Error())
		}
		select {
		case <-ctx.Done():
			_ = client.Close()
			return nil, ctx.Err()
		case <-time.After(backoff):
		}
		if backoff < 5*time.Second {
			backoff *= 2
		}
	}

	_ = client.Close()
	return nil, fmt.Errorf("redis: no se pudo conectar tras %d intentos: %w", maxAttempts, err)
}
