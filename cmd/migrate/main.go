// Comando migrate: aplica o revierte migraciones SQL.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/joho/godotenv"

	"github.com/aibotia/aibotia-go/internal/config"
	"github.com/aibotia/aibotia-go/internal/platform/logger"
	"github.com/aibotia/aibotia-go/internal/store/migrations"
	"github.com/aibotia/aibotia-go/internal/store/postgres"
)

func main() {
	down := flag.Bool("down", false, "revierte la última migración")
	flag.Parse()

	if err := run(*down); err != nil {
		slog.Error("error en migraciones", "error", err.Error())
		os.Exit(1)
	}
}

func run(down bool) error {
	_ = godotenv.Load(".env")

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := logger.New(cfg.LogLevel, cfg.LogFormat)

	ctx := context.Background()
	pool, err := postgres.Connect(ctx, cfg.DatabaseURL, log)
	if err != nil {
		return err
	}
	defer pool.Close()

	if down {
		if err := migrations.Down(ctx, pool); err != nil {
			return err
		}
		fmt.Println("última migración revertida")
		return nil
	}

	if err := migrations.Up(ctx, pool); err != nil {
		return err
	}
	fmt.Println("migraciones aplicadas")
	return nil
}
