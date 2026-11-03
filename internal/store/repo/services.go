package repo

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Service es un servicio del catálogo (fuente de verdad en runtime).
type Service struct {
	ID          string    `json:"id"`
	Category    string    `json:"category"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Price       *float64  `json:"price"`
	DurationMin *int      `json:"duration_min"`
	Note        string    `json:"note"`
	Providers   []string  `json:"providers"`
	Active      bool      `json:"is_active"`
	SortOrder   int       `json:"sort_order"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Services es el repositorio del catálogo de servicios.
type Services struct {
	db *pgxpool.Pool
}

// NewServices construye el repositorio.
func NewServices(db *pgxpool.Pool) *Services { return &Services{db: db} }

// Count devuelve el número de servicios.
func (r *Services) Count(ctx context.Context) (int, error) {
	var n int
	if err := r.db.QueryRow(ctx, "SELECT count(*) FROM services").Scan(&n); err != nil {
		return 0, fmt.Errorf("repo: contar servicios: %w", err)
	}
	return n, nil
}

// List devuelve el catálogo (incluye inactivos si includeInactive).
func (r *Services) List(ctx context.Context, includeInactive bool) ([]Service, error) {
	query := `SELECT id, category, name, description, price, duration_min, note, providers, is_active, sort_order, created_at, updated_at
		FROM services`
	if !includeInactive {
		query += " WHERE is_active"
	}
	query += " ORDER BY category, sort_order, name"
	return r.query(ctx, query)
}

// ListActive devuelve solo los servicios activos.
func (r *Services) ListActive(ctx context.Context) ([]Service, error) {
	return r.List(ctx, false)
}

// Get devuelve un servicio por id.
func (r *Services) Get(ctx context.Context, id string) (Service, error) {
	rows, err := r.query(ctx, `SELECT id, category, name, description, price, duration_min, note, providers, is_active, sort_order, created_at, updated_at
		FROM services WHERE id = $1`, id)
	if err != nil {
		return Service{}, err
	}
	if len(rows) == 0 {
		return Service{}, pgx.ErrNoRows
	}
	return rows[0], nil
}

// Create inserta un servicio.
func (r *Services) Create(ctx context.Context, s Service) (Service, error) {
	providers, err := json.Marshal(s.Providers)
	if err != nil {
		return Service{}, fmt.Errorf("repo: providers: %w", err)
	}
	_, err = r.db.Exec(ctx, `
		INSERT INTO services (id, category, name, description, price, duration_min, note, providers, is_active, sort_order)
		VALUES ($1, $2, $3, $4, $5::numeric, $6, $7, $8::jsonb, $9, $10)`,
		s.ID, s.Category, s.Name, s.Description, priceArg(s.Price), s.DurationMin, s.Note, string(providers), s.Active, s.SortOrder)
	if err != nil {
		return Service{}, fmt.Errorf("repo: crear servicio: %w", err)
	}
	return r.Get(ctx, s.ID)
}

// Update actualiza un servicio por id.
func (r *Services) Update(ctx context.Context, id string, s Service) (Service, error) {
	providers, err := json.Marshal(s.Providers)
	if err != nil {
		return Service{}, fmt.Errorf("repo: providers: %w", err)
	}
	tag, err := r.db.Exec(ctx, `
		UPDATE services SET id=$2, category=$3, name=$4, description=$5, price=$6::numeric, duration_min=$7,
			note=$8, providers=$9::jsonb, is_active=$10, sort_order=$11, updated_at=now()
		WHERE id=$1`,
		id, s.ID, s.Category, s.Name, s.Description, priceArg(s.Price), s.DurationMin, s.Note, string(providers), s.Active, s.SortOrder)
	if err != nil {
		return Service{}, fmt.Errorf("repo: actualizar servicio: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return Service{}, pgx.ErrNoRows
	}
	return r.Get(ctx, s.ID)
}

// Delete elimina un servicio.
func (r *Services) Delete(ctx context.Context, id string) error {
	tag, err := r.db.Exec(ctx, "DELETE FROM services WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("repo: borrar servicio: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// Seed inserta servicios que aún no existen (no sobrescribe ediciones).
func (r *Services) Seed(ctx context.Context, items []Service) (int, error) {
	count := 0
	for _, s := range items {
		providers, err := json.Marshal(s.Providers)
		if err != nil {
			return count, fmt.Errorf("repo: providers: %w", err)
		}
		tag, err := r.db.Exec(ctx, `
			INSERT INTO services (id, category, name, description, price, duration_min, note, providers, is_active, sort_order)
			VALUES ($1, $2, $3, $4, $5::numeric, $6, $7, $8::jsonb, $9, $10) ON CONFLICT (id) DO NOTHING`,
			s.ID, s.Category, s.Name, s.Description, priceArg(s.Price), s.DurationMin, s.Note, string(providers), s.Active, s.SortOrder)
		if err != nil {
			return count, fmt.Errorf("repo: seed servicio %q: %w", s.ID, err)
		}
		if tag.RowsAffected() > 0 {
			count++
		}
	}
	return count, nil
}

func priceArg(p *float64) any {
	if p == nil {
		return nil
	}
	return strconv.FormatFloat(*p, 'f', 2, 64)
}

func (r *Services) query(ctx context.Context, sql string, args ...any) ([]Service, error) {
	rows, err := r.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("repo: consultar servicios: %w", err)
	}
	defer rows.Close()

	out := make([]Service, 0)
	for rows.Next() {
		var (
			s         Service
			price     pgtype.Numeric
			duration  pgtype.Int4
			providers []byte
		)
		if err := rows.Scan(&s.ID, &s.Category, &s.Name, &s.Description, &price, &duration, &s.Note, &providers, &s.Active, &s.SortOrder, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		if price.Valid {
			f, err := price.Float64Value()
			if err == nil && f.Valid {
				v := f.Float64
				s.Price = &v
			}
		}
		if duration.Valid {
			v := int(duration.Int32)
			s.DurationMin = &v
		}
		if len(providers) > 0 {
			if err := json.Unmarshal(providers, &s.Providers); err != nil {
				return nil, fmt.Errorf("repo: providers de %q: %w", s.ID, err)
			}
		}
		if s.Providers == nil {
			s.Providers = []string{}
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
