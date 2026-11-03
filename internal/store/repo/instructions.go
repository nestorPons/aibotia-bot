package repo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Translation es una traducción de un bloque de contenido.
type Translation struct {
	Language string `json:"language"`
	Content  string `json:"content"`
}

// Block es un bloque de contenido (fix-message / instrucción).
type Block struct {
	ID           int64           `json:"id"`
	Code         string          `json:"code"`
	Name         string          `json:"name"`
	Type         string          `json:"type"`
	Description  string          `json:"description"`
	Actions      json.RawMessage `json:"actions"`
	Attachments  json.RawMessage `json:"attachments"`
	Properties   json.RawMessage `json:"properties"`
	Translations []Translation   `json:"translations"`
}

// Instructions es el repositorio de bloques de contenido.
type Instructions struct {
	db *pgxpool.Pool
}

// NewInstructions construye el repositorio.
func NewInstructions(db *pgxpool.Pool) *Instructions { return &Instructions{db: db} }

// List devuelve todos los bloques con sus traducciones.
func (r *Instructions) List(ctx context.Context) ([]Block, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, code, COALESCE(name,''), type, COALESCE(description,''),
		       COALESCE(actions,'null'), COALESCE(attachments,'null'), COALESCE(properties,'null')
		FROM content_blocks ORDER BY code`)
	if err != nil {
		return nil, fmt.Errorf("repo: listar bloques: %w", err)
	}
	defer rows.Close()

	blocks := make([]Block, 0)
	for rows.Next() {
		var b Block
		if err := rows.Scan(&b.ID, &b.Code, &b.Name, &b.Type, &b.Description, &b.Actions, &b.Attachments, &b.Properties); err != nil {
			return nil, err
		}
		blocks = append(blocks, b)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range blocks {
		tr, err := r.translations(ctx, blocks[i].ID)
		if err != nil {
			return nil, err
		}
		blocks[i].Translations = tr
	}
	return blocks, nil
}

// GetByCode devuelve un bloque por code.
func (r *Instructions) GetByCode(ctx context.Context, code string) (Block, error) {
	var b Block
	err := r.db.QueryRow(ctx, `
		SELECT id, code, COALESCE(name,''), type, COALESCE(description,''),
		       COALESCE(actions,'null'), COALESCE(attachments,'null'), COALESCE(properties,'null')
		FROM content_blocks WHERE code = $1`, code).
		Scan(&b.ID, &b.Code, &b.Name, &b.Type, &b.Description, &b.Actions, &b.Attachments, &b.Properties)
	if errors.Is(err, pgx.ErrNoRows) {
		return Block{}, pgx.ErrNoRows
	}
	if err != nil {
		return Block{}, fmt.Errorf("repo: obtener bloque: %w", err)
	}
	tr, err := r.translations(ctx, b.ID)
	if err != nil {
		return Block{}, err
	}
	b.Translations = tr
	return b, nil
}

// Update actualiza un bloque y reemplaza sus traducciones.
func (r *Instructions) Update(ctx context.Context, code string, b Block, translations []Translation) (Block, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Block{}, fmt.Errorf("repo: tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx, `
		UPDATE content_blocks
		SET name=$2, type=$3, description=$4, actions=$5, attachments=$6, properties=$7, updated_at=now()
		WHERE code=$1`,
		code, b.Name, defaultType(b.Type), b.Description, jsonOrNull(b.Actions), jsonOrNull(b.Attachments), jsonOrNull(b.Properties))
	if err != nil {
		return Block{}, fmt.Errorf("repo: actualizar bloque: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return Block{}, pgx.ErrNoRows
	}

	if _, err := tx.Exec(ctx, "DELETE FROM content_translations WHERE block_id = (SELECT id FROM content_blocks WHERE code=$1)", code); err != nil {
		return Block{}, fmt.Errorf("repo: borrar traducciones: %w", err)
	}
	for _, t := range translations {
		if _, err := tx.Exec(ctx, `
			INSERT INTO content_translations (block_id, language, content)
			VALUES ((SELECT id FROM content_blocks WHERE code=$1), $2, $3)`, code, t.Language, t.Content); err != nil {
			return Block{}, fmt.Errorf("repo: insertar traducción: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return Block{}, err
	}
	return r.GetByCode(ctx, code)
}

// Upsert inserta o actualiza un bloque y reemplaza sus traducciones.
func (r *Instructions) Upsert(ctx context.Context, code string, b Block, translations []Translation) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("repo: tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		INSERT INTO content_blocks (code, name, type, description, actions, attachments, properties)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (code) DO UPDATE SET
			name = EXCLUDED.name, type = EXCLUDED.type, description = EXCLUDED.description,
			actions = EXCLUDED.actions, attachments = EXCLUDED.attachments,
			properties = EXCLUDED.properties, updated_at = now()`,
		code, b.Name, defaultType(b.Type), b.Description,
		jsonOrNull(b.Actions), jsonOrNull(b.Attachments), jsonOrNull(b.Properties)); err != nil {
		return fmt.Errorf("repo: upsert bloque: %w", err)
	}

	if _, err := tx.Exec(ctx, "DELETE FROM content_translations WHERE block_id = (SELECT id FROM content_blocks WHERE code=$1)", code); err != nil {
		return fmt.Errorf("repo: borrar traducciones: %w", err)
	}
	for _, t := range translations {
		if _, err := tx.Exec(ctx, `
			INSERT INTO content_translations (block_id, language, content)
			VALUES ((SELECT id FROM content_blocks WHERE code=$1), $2, $3)`, code, t.Language, t.Content); err != nil {
			return fmt.Errorf("repo: insertar traducción: %w", err)
		}
	}
	return tx.Commit(ctx)
}

// Delete elimina un bloque (y sus traducciones por CASCADE).
func (r *Instructions) Delete(ctx context.Context, code string) error {
	tag, err := r.db.Exec(ctx, "DELETE FROM content_blocks WHERE code = $1", code)
	if err != nil {
		return fmt.Errorf("repo: borrar bloque: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *Instructions) translations(ctx context.Context, blockID int64) ([]Translation, error) {
	rows, err := r.db.Query(ctx,
		"SELECT language, content FROM content_translations WHERE block_id = $1 ORDER BY language", blockID)
	if err != nil {
		return nil, fmt.Errorf("repo: traducciones: %w", err)
	}
	defer rows.Close()
	out := make([]Translation, 0)
	for rows.Next() {
		var t Translation
		if err := rows.Scan(&t.Language, &t.Content); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func defaultType(t string) string {
	if t == "" {
		return "message_fixed"
	}
	return t
}

func jsonOrNull(raw json.RawMessage) any {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	return raw
}
