// Package repo contiene los repositorios de persistencia.
package repo

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/aibotia/aibotia-go/internal/domain"
)

// UserFilter acota la lista de usuarios del panel.
type UserFilter struct {
	Search string
	Active *bool // nil = todos
}

// Users es el repositorio de usuarios.
type Users struct {
	db *pgxpool.Pool
}

// NewUsers construye el repositorio.
func NewUsers(db *pgxpool.Pool) *Users { return &Users{db: db} }

const userColumns = "id, name, alias, active, COALESCE(topic,''), COALESCE(lang,''), role, buffer_time, temperature, test_mode"

// Get devuelve un usuario por ID.
func (r *Users) Get(ctx context.Context, id string) (domain.User, error) {
	row := r.db.QueryRow(ctx, "SELECT "+userColumns+" FROM users WHERE id = $1", id)
	return scanUser(row)
}

// FindOrCreate devuelve el usuario existente o lo crea.
func (r *Users) FindOrCreate(ctx context.Context, id, name string, activeIfNew bool) (domain.User, error) {
	u, err := r.Get(ctx, id)
	if err == nil {
		return u, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, err
	}
	_, err = r.db.Exec(ctx,
		`INSERT INTO users (id, name, alias, active, role) VALUES ($1, $2, $2, $3, $4)
		 ON CONFLICT (id) DO NOTHING`,
		id, name, activeIfNew, domain.RoleClient)
	if err != nil {
		return domain.User{}, fmt.Errorf("repo: crear usuario: %w", err)
	}
	return r.Get(ctx, id)
}

// SetActive activa o desactiva la IA para el usuario.
func (r *Users) SetActive(ctx context.Context, id string, active bool) error {
	_, err := r.db.Exec(ctx, "UPDATE users SET active = $2, updated_at = now() WHERE id = $1", id, active)
	return err
}

// SetTopic actualiza el tema actual.
func (r *Users) SetTopic(ctx context.Context, id, topic string) error {
	_, err := r.db.Exec(ctx, "UPDATE users SET topic = $2, updated_at = now() WHERE id = $1", id, topic)
	return err
}

// SetTestMode activa o desactiva el modo prueba de un usuario. Al activarlo se
// reactiva la IA para que el bot siga respondiendo durante las pruebas.
func (r *Users) SetTestMode(ctx context.Context, id string, enabled bool) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE users
		SET test_mode = $2,
		    active = CASE WHEN $2 THEN TRUE ELSE active END,
		    updated_at = now()
		WHERE id = $1`, id, enabled)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// UpdateName actualiza el nombre.
func (r *Users) UpdateName(ctx context.Context, id, name string) error {
	_, err := r.db.Exec(ctx, "UPDATE users SET name = $2, updated_at = now() WHERE id = $1", id, name)
	return err
}

// Create inserta (o actualiza) un usuario con todos sus campos editables.
func (r *Users) Create(ctx context.Context, u domain.User) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO users (id, name, alias, active, topic, lang, role, temperature, buffer_time)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (id) DO UPDATE SET
			name=EXCLUDED.name, alias=EXCLUDED.alias, active=EXCLUDED.active,
			topic=EXCLUDED.topic, lang=EXCLUDED.lang, role=EXCLUDED.role,
			temperature=EXCLUDED.temperature, buffer_time=EXCLUDED.buffer_time,
			updated_at=now()`,
		u.ID, u.Name, u.Alias, u.Active, nullText(u.Topic), nullText(u.Lang),
		int(u.Role), u.Temperature, u.BufferMs)
	if err != nil {
		return fmt.Errorf("repo: crear usuario: %w", err)
	}
	return nil
}

// Update actualiza todos los campos editables de un usuario.
func (r *Users) Update(ctx context.Context, id string, u domain.User) error {
	_, err := r.db.Exec(ctx, `
		UPDATE users SET name=$2, alias=$3, active=$4, topic=$5, lang=$6,
			role=$7, temperature=$8, buffer_time=$9, updated_at=now()
		WHERE id=$1`,
		id, u.Name, u.Alias, u.Active, nullText(u.Topic), nullText(u.Lang),
		int(u.Role), u.Temperature, u.BufferMs)
	if err != nil {
		return fmt.Errorf("repo: actualizar usuario: %w", err)
	}
	return nil
}

func nullText(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// DeleteResult resume lo eliminado al borrar un usuario.
type DeleteResult struct {
	Messages     int64 `json:"messages"`
	Appointments int64 `json:"appointments"`
	Contacts     int64 `json:"contacts"`
}

// DeleteCascade elimina el usuario y todos sus datos: conversaciones, citas,
// historial en caché de WhatsApp y ajustes de chat. El historial de Redis y el
// anti-spam se limpian desde la capa HTTP (necesitan el cliente de Redis).
func (r *Users) DeleteCascade(ctx context.Context, id string) (DeleteResult, error) {
	var res DeleteResult

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return res, fmt.Errorf("repo: tx borrado usuario: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM ai_interactions WHERE user_id = $1 OR session_id = $1`, id,
	).Scan(&res.Messages); err != nil {
		return res, fmt.Errorf("repo: contar interacciones: %w", err)
	}
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM appointments WHERE user_id = $1`, id,
	).Scan(&res.Appointments); err != nil {
		return res, fmt.Errorf("repo: contar citas: %w", err)
	}

	jids, err := jidVariants(ctx, tx, id)
	if err != nil {
		return res, err
	}
	var hasContacts bool
	if err := tx.QueryRow(ctx,
		`SELECT to_regclass('public.whatsmeow_contacts') IS NOT NULL`,
	).Scan(&hasContacts); err != nil {
		return res, fmt.Errorf("repo: comprobar tablas de WhatsApp: %w", err)
	}
	if hasContacts {
		tag, err := tx.Exec(ctx,
			`DELETE FROM whatsmeow_contacts WHERE their_jid = ANY($1)`, jids)
		if err != nil {
			return res, fmt.Errorf("repo: borrar contacto cacheado: %w", err)
		}
		res.Contacts = tag.RowsAffected()
		if _, err := tx.Exec(ctx,
			`DELETE FROM whatsmeow_chat_settings WHERE chat_jid = ANY($1)`, jids); err != nil {
			return res, fmt.Errorf("repo: borrar ajustes del chat: %w", err)
		}
	}

	tag, err := tx.Exec(ctx, `DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		return res, fmt.Errorf("repo: borrar usuario: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return DeleteResult{}, pgx.ErrNoRows
	}
	if err := tx.Commit(ctx); err != nil {
		return res, err
	}
	return res, nil
}

// jidVariants devuelve el JID y su equivalente LID/PN para limpiar la caché de
// WhatsApp del contacto.
func jidVariants(ctx context.Context, tx pgx.Tx, id string) ([]string, error) {
	out := []string{id}
	var hasMap bool
	if err := tx.QueryRow(ctx,
		`SELECT to_regclass('public.whatsmeow_lid_map') IS NOT NULL`,
	).Scan(&hasMap); err != nil {
		return nil, fmt.Errorf("repo: comprobar mapa LID: %w", err)
	}
	if !hasMap {
		return out, nil
	}
	local := id
	if i := strings.Index(id, "@"); i > 0 {
		local = id[:i]
	}
	var other string
	switch {
	case strings.HasSuffix(id, "@lid"):
		if err := tx.QueryRow(ctx, `SELECT pn FROM whatsmeow_lid_map WHERE lid = $1`, local).Scan(&other); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("repo: mapear LID: %w", err)
		}
		if other != "" {
			out = append(out, other+"@s.whatsapp.net")
		}
	case strings.HasSuffix(id, "@s.whatsapp.net"):
		if err := tx.QueryRow(ctx, `SELECT lid FROM whatsmeow_lid_map WHERE pn = $1`, local).Scan(&other); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("repo: mapear PN: %w", err)
		}
		if other != "" {
			out = append(out, other+"@lid")
		}
	}
	return out, nil
}

// ImportContacts copia los contactos de WhatsApp (whatsmeow_contacts) a la tabla
// de usuarios, mapeando el teléfono (PN) a su LID cuando esté disponible.
// No sobreescribe usuarios existentes; los nuevos se crean con activeIfNew.
// Devuelve cuántos se insertaron.
func (r *Users) ImportContacts(ctx context.Context, activeIfNew bool) (int, error) {
	const q = `
		WITH src AS (
			SELECT
				CASE
					WHEN c.their_jid LIKE '%@lid' THEN split_part(split_part(c.their_jid,'@',1),':',1) || '@lid'
					WHEN c.their_jid LIKE '%@s.whatsapp.net' AND lm.lid IS NOT NULL THEN lm.lid || '@lid'
					ELSE c.their_jid
				END AS id,
				COALESCE(NULLIF(c.full_name,''), NULLIF(c.first_name,''), NULLIF(c.push_name,''), NULLIF(c.business_name,''), '') AS name
			FROM whatsmeow_contacts c
			LEFT JOIN whatsmeow_lid_map lm ON lm.pn = split_part(c.their_jid, '@', 1)
			WHERE c.their_jid LIKE '%@s.whatsapp.net' OR c.their_jid LIKE '%@lid'
		), uniq AS (
			SELECT DISTINCT ON (id) id, name FROM src WHERE id <> ''
		)
		INSERT INTO users (id, name, alias, active, role)
		SELECT id, name, name, $2, $1 FROM uniq
		ON CONFLICT (id) DO NOTHING`
	tag, err := r.db.Exec(ctx, q, int(domain.RoleClient), activeIfNew)
	if err != nil {
		return 0, fmt.Errorf("repo: importar contactos: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

// List devuelve usuarios paginados y el total.
func (r *Users) List(ctx context.Context, limit, offset int, f UserFilter) ([]domain.User, int, error) {
	pattern := "%" + f.Search + "%"
	var total int
	if err := r.db.QueryRow(ctx, `
		SELECT count(*) FROM users
		WHERE ($1 = '' OR name ILIKE $2 OR id ILIKE $2)
		  AND ($3::boolean IS NULL OR active = $3)`,
		f.Search, pattern, f.Active,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("repo: contar usuarios: %w", err)
	}

	rows, err := r.db.Query(ctx,
		"SELECT "+userColumns+` FROM users
		 WHERE ($1 = '' OR name ILIKE $2 OR id ILIKE $2)
		   AND ($3::boolean IS NULL OR active = $3)
		 ORDER BY updated_at DESC LIMIT $4 OFFSET $5`,
		f.Search, pattern, f.Active, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("repo: listar usuarios: %w", err)
	}
	defer rows.Close()

	users := make([]domain.User, 0)
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, 0, err
		}
		users = append(users, u)
	}
	return users, total, rows.Err()
}

type scannable interface {
	Scan(dest ...any) error
}

func scanUser(row scannable) (domain.User, error) {
	var u domain.User
	if err := row.Scan(&u.ID, &u.Name, &u.Alias, &u.Active, &u.Topic, &u.Lang, &u.Role, &u.BufferMs, &u.Temperature, &u.TestMode); err != nil {
		return domain.User{}, err
	}
	return u, nil
}
