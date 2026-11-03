package repo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CompanyPromptCode es el prompt único con los datos de empresa. Se inyecta
// siempre en el system prompt y no se indexa en el RAG.
const CompanyPromptCode = "prompt_company"

// LegacyLocationPrompts son los prompts antiguos que se fusionan en
// CompanyPromptCode al arrancar (ver internal/bootstrap/company.go).
var LegacyLocationPrompts = []string{"prompt_location", "prompt_location_contact"}

// Prompt es un prompt del asistente almacenado en base de datos.
type Prompt struct {
	ID          int64     `json:"id"`
	Code        string    `json:"code"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Content     string    `json:"content"`
	Active      bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// PromptVersion es una versión histórica del contenido de un prompt.
type PromptVersion struct {
	ID        int64     `json:"id"`
	PromptID  int64     `json:"prompt_id"`
	Name      string    `json:"name"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

// Prompts es el repositorio de prompts (fuente de verdad en runtime).
type Prompts struct {
	db *pgxpool.Pool
}

// NewPrompts construye el repositorio.
func NewPrompts(db *pgxpool.Pool) *Prompts { return &Prompts{db: db} }

// Count devuelve el número de prompts.
func (r *Prompts) Count(ctx context.Context) (int, error) {
	var n int
	if err := r.db.QueryRow(ctx, "SELECT count(*) FROM prompts").Scan(&n); err != nil {
		return 0, fmt.Errorf("repo: contar prompts: %w", err)
	}
	return n, nil
}

// List devuelve todos los prompts sin contenido (para el panel).
func (r *Prompts) List(ctx context.Context) ([]Prompt, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, code, name, description, is_active, created_at, updated_at
		FROM prompts ORDER BY code`)
	if err != nil {
		return nil, fmt.Errorf("repo: listar prompts: %w", err)
	}
	defer rows.Close()

	out := make([]Prompt, 0)
	for rows.Next() {
		var p Prompt
		if err := rows.Scan(&p.ID, &p.Code, &p.Name, &p.Description, &p.Active, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Get devuelve un prompt por code, con su contenido.
func (r *Prompts) Get(ctx context.Context, code string) (Prompt, error) {
	var p Prompt
	err := r.db.QueryRow(ctx, `
		SELECT id, code, name, description, content, is_active, created_at, updated_at
		FROM prompts WHERE code = $1`, code).
		Scan(&p.ID, &p.Code, &p.Name, &p.Description, &p.Content, &p.Active, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Prompt{}, pgx.ErrNoRows
	}
	if err != nil {
		return Prompt{}, fmt.Errorf("repo: obtener prompt: %w", err)
	}
	return p, nil
}

// Snapshot devuelve code -> content de los prompts activos.
func (r *Prompts) Snapshot(ctx context.Context) (map[string]string, error) {
	rows, err := r.db.Query(ctx, "SELECT code, content FROM prompts WHERE is_active")
	if err != nil {
		return nil, fmt.Errorf("repo: snapshot de prompts: %w", err)
	}
	defer rows.Close()

	out := make(map[string]string)
	for rows.Next() {
		var code, content string
		if err := rows.Scan(&code, &content); err != nil {
			return nil, err
		}
		out[code] = content
	}
	return out, rows.Err()
}

// Create inserta un prompt y guarda su primera versión.
func (r *Prompts) Create(ctx context.Context, p Prompt) (Prompt, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Prompt{}, fmt.Errorf("repo: tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var id int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO prompts (code, name, description, content, is_active)
		VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		p.Code, p.Name, p.Description, p.Content, p.Active).Scan(&id); err != nil {
		return Prompt{}, fmt.Errorf("repo: crear prompt: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO prompt_versions (prompt_id, name, content) VALUES ($1, $2, $3)`,
		id, p.Name, p.Content); err != nil {
		return Prompt{}, fmt.Errorf("repo: versión inicial: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Prompt{}, err
	}
	return r.Get(ctx, p.Code)
}

// Update actualiza un prompt y guarda una versión con el nuevo contenido.
func (r *Prompts) Update(ctx context.Context, code string, p Prompt) (Prompt, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Prompt{}, fmt.Errorf("repo: tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var id int64
	err = tx.QueryRow(ctx, `
		UPDATE prompts SET name=$2, description=$3, content=$4, is_active=$5, updated_at=now()
		WHERE code=$1 RETURNING id`,
		code, p.Name, p.Description, p.Content, p.Active).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Prompt{}, pgx.ErrNoRows
	}
	if err != nil {
		return Prompt{}, fmt.Errorf("repo: actualizar prompt: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO prompt_versions (prompt_id, name, content) VALUES ($1, $2, $3)`,
		id, p.Name, p.Content); err != nil {
		return Prompt{}, fmt.Errorf("repo: versión: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Prompt{}, err
	}
	return r.Get(ctx, code)
}

// Delete elimina un prompt y su historial (CASCADE).
func (r *Prompts) Delete(ctx context.Context, code string) error {
	tag, err := r.db.Exec(ctx, "DELETE FROM prompts WHERE code = $1", code)
	if err != nil {
		return fmt.Errorf("repo: borrar prompt: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// Versions devuelve el historial de un prompt (más reciente primero).
func (r *Prompts) Versions(ctx context.Context, code string) ([]PromptVersion, error) {
	rows, err := r.db.Query(ctx, `
		SELECT v.id, v.prompt_id, v.name, v.content, v.created_at
		FROM prompt_versions v JOIN prompts p ON p.id = v.prompt_id
		WHERE p.code = $1 ORDER BY v.created_at DESC, v.id DESC`, code)
	if err != nil {
		return nil, fmt.Errorf("repo: versiones de prompt: %w", err)
	}
	defer rows.Close()

	out := make([]PromptVersion, 0)
	for rows.Next() {
		var v PromptVersion
		if err := rows.Scan(&v.ID, &v.PromptID, &v.Name, &v.Content, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// Restore restaura una versión concreta creando una nueva versión.
func (r *Prompts) Restore(ctx context.Context, code string, versionID int64) (Prompt, error) {
	var name, content string
	err := r.db.QueryRow(ctx, `
		SELECT v.name, v.content FROM prompt_versions v
		JOIN prompts p ON p.id = v.prompt_id
		WHERE p.code = $1 AND v.id = $2`, code, versionID).Scan(&name, &content)
	if errors.Is(err, pgx.ErrNoRows) {
		return Prompt{}, pgx.ErrNoRows
	}
	if err != nil {
		return Prompt{}, fmt.Errorf("repo: obtener versión: %w", err)
	}
	cur, err := r.Get(ctx, code)
	if err != nil {
		return Prompt{}, err
	}
	return r.Update(ctx, code, Prompt{
		Name:        name,
		Description: cur.Description,
		Content:     content,
		Active:      cur.Active,
	})
}

// Seed inserta prompts que aún no existen (no sobrescribe ediciones).
func (r *Prompts) Seed(ctx context.Context, items []Prompt) (int, error) {
	count := 0
	for _, p := range items {
		var id int64
		err := r.db.QueryRow(ctx, `
			INSERT INTO prompts (code, name, description, content, is_active)
			VALUES ($1, $2, $3, $4, $5) ON CONFLICT (code) DO NOTHING
			RETURNING id`,
			p.Code, p.Name, p.Description, p.Content, p.Active).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return count, fmt.Errorf("repo: seed prompt %q: %w", p.Code, err)
		}
		if _, err := r.db.Exec(ctx, `
			INSERT INTO prompt_versions (prompt_id, name, content) VALUES ($1, $2, $3)`,
			id, p.Name, p.Content); err != nil {
			return count, fmt.Errorf("repo: versión inicial de %q: %w", p.Code, err)
		}
		count++
	}
	return count, nil
}
