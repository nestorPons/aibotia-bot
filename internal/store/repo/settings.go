package repo

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Settings gestiona el estado global del bot (tabla settings, fila única).
type Settings struct {
	db *pgxpool.Pool
}

// NewSettings construye el repositorio.
func NewSettings(db *pgxpool.Pool) *Settings { return &Settings{db: db} }

// IsActive devuelve si el bot está activo globalmente.
func (r *Settings) IsActive(ctx context.Context) (bool, error) {
	var active bool
	err := r.db.QueryRow(ctx, "SELECT is_active FROM settings ORDER BY id LIMIT 1").Scan(&active)
	if err != nil {
		// Si no hay fila, consideramos activo por defecto.
		return true, nil
	}
	return active, nil
}

// SetActive actualiza el estado global del bot.
func (r *Settings) SetActive(ctx context.Context, active bool) error {
	tag, err := r.db.Exec(ctx, "UPDATE settings SET is_active = $1, updated_at = now()", active)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		_, err = r.db.Exec(ctx, "INSERT INTO settings (is_active) VALUES ($1)", active)
	}
	return err
}

// ActivateNewUsers devuelve si la IA se activa por defecto en los usuarios
// nuevos (mensajes entrantes e importación de contactos).
func (r *Settings) ActivateNewUsers(ctx context.Context) (bool, error) {
	var active bool
	err := r.db.QueryRow(ctx, "SELECT activate_new_users FROM settings ORDER BY id LIMIT 1").Scan(&active)
	if err != nil {
		// Si no hay fila, activamos por defecto.
		return true, nil
	}
	return active, nil
}

// SetActivateNewUsers actualiza el interruptor de activación automática.
func (r *Settings) SetActivateNewUsers(ctx context.Context, active bool) error {
	tag, err := r.db.Exec(ctx, "UPDATE settings SET activate_new_users = $1, updated_at = now()", active)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		_, err = r.db.Exec(ctx, "INSERT INTO settings (activate_new_users) VALUES ($1)", active)
	}
	return err
}
