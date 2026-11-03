package repo

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/aibotia/aibotia-go/internal/domain"
)

// History almacena la memoria de conversación de cada usuario en Redis.
type History struct {
	rdb *redis.Client
	ttl time.Duration
	max int
}

// NewHistory construye el repositorio de historial.
func NewHistory(rdb *redis.Client) *History {
	return &History{rdb: rdb, ttl: 7 * 24 * time.Hour, max: 40}
}

func historyKey(sessionID string) string { return "history:" + sessionID }

// Append añade un turno al historial, recorta y renueva TTL.
func (h *History) Append(ctx context.Context, sessionID string, turn domain.Turn) error {
	payload, err := json.Marshal(turn)
	if err != nil {
		return err
	}
	key := historyKey(sessionID)
	pipe := h.rdb.Pipeline()
	pipe.RPush(ctx, key, payload)
	pipe.LTrim(ctx, key, int64(-h.max), -1)
	pipe.Expire(ctx, key, h.ttl)
	_, err = pipe.Exec(ctx)
	return err
}

// Window devuelve los últimos n turnos (orden cronológico).
func (h *History) Window(ctx context.Context, sessionID string, n int) ([]domain.Turn, error) {
	if n <= 0 || n > h.max {
		n = h.max
	}
	vals, err := h.rdb.LRange(ctx, historyKey(sessionID), int64(-n), -1).Result()
	if err != nil {
		return nil, err
	}
	turns := make([]domain.Turn, 0, len(vals))
	for _, v := range vals {
		var t domain.Turn
		if err := json.Unmarshal([]byte(v), &t); err != nil {
			continue
		}
		turns = append(turns, t)
	}
	return turns, nil
}

// Clear borra el historial de una sesión.
func (h *History) Clear(ctx context.Context, sessionID string) error {
	return h.rdb.Del(ctx, historyKey(sessionID)).Err()
}
