// Package security implementa controles de abuso (anti-spam).
package security

import (
	"context"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// AntiSpam aplica una ventana deslizante por usuario en Redis.
type AntiSpam struct {
	rdb    *redis.Client
	limit  int
	window time.Duration
}

// New construye el anti-spam.
func New(rdb *redis.Client, limit int, window time.Duration) *AntiSpam {
	if limit <= 0 {
		limit = 5
	}
	if window <= 0 {
		window = time.Minute
	}
	return &AntiSpam{rdb: rdb, limit: limit, window: window}
}

// IsSpamming registra el mensaje y devuelve true si supera el límite de la ventana.
func (a *AntiSpam) IsSpamming(ctx context.Context, userID string) (bool, error) {
	key := "antispam:" + userID
	now := time.Now().UnixMilli()
	cutoff := now - a.window.Milliseconds()

	pipe := a.rdb.TxPipeline()
	pipe.ZRemRangeByScore(ctx, key, "-inf", strconv.FormatInt(cutoff, 10))
	pipe.ZAdd(ctx, key, redis.Z{Score: float64(now), Member: strconv.FormatInt(now, 10)})
	card := pipe.ZCard(ctx, key)
	pipe.Expire(ctx, key, a.window*2)
	if _, err := pipe.Exec(ctx); err != nil {
		return false, err
	}
	return card.Val() > int64(a.limit), nil
}

// Reset borra la ventana deslizante de un usuario (al eliminarlo del panel).
func (a *AntiSpam) Reset(ctx context.Context, userID string) error {
	if a == nil || a.rdb == nil {
		return nil
	}
	return a.rdb.Del(ctx, "antispam:"+userID).Err()
}
