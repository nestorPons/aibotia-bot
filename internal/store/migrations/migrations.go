// Package migrations aplica las migraciones SQL embebidas mediante goose.
package migrations

import (
	"context"
	"embed"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

//go:embed sql/*.sql
var FS embed.FS

func init() {
	goose.SetBaseFS(FS)
	if err := goose.SetDialect("postgres"); err != nil {
		panic(fmt.Sprintf("migrations: dialecto inválido: %v", err))
	}
}

// Up aplica todas las migraciones pendientes.
func Up(ctx context.Context, pool *pgxpool.Pool) error {
	db := stdlib.OpenDBFromPool(pool)
	// No cerramos db: cerraría el pool compartido con la aplicación.
	if err := goose.UpContext(ctx, db, "sql"); err != nil {
		return fmt.Errorf("migrations: up: %w", err)
	}
	return nil
}

// Down revierte la última migración aplicada.
func Down(ctx context.Context, pool *pgxpool.Pool) error {
	db := stdlib.OpenDBFromPool(pool)
	if err := goose.DownContext(ctx, db, "sql"); err != nil {
		return fmt.Errorf("migrations: down: %w", err)
	}
	return nil
}
