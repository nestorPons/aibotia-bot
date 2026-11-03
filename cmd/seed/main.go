// Comando seed: carga los bloques de contenido (fix-messages) desde
// data/seeds/messages.json a la base de datos.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"

	"github.com/aibotia/aibotia-go/internal/bootstrap"
	"github.com/aibotia/aibotia-go/internal/config"
	"github.com/aibotia/aibotia-go/internal/platform/logger"
	"github.com/aibotia/aibotia-go/internal/store/postgres"
	"github.com/aibotia/aibotia-go/internal/store/repo"
)

type messageJSON struct {
	Code        string          `json:"code"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Content     string          `json:"content"`
	Actions     json.RawMessage `json:"actions"`
	Attachments json.RawMessage `json:"attachments"`
}

func main() {
	if err := run(); err != nil {
		slog.Error("error cargando contenido", "error", err.Error())
		os.Exit(1)
	}
}

func run() error {
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

	instructions := repo.NewInstructions(pool)
	count, err := seedMessages(ctx, instructions, log, cfg.DataDir)
	if err != nil {
		return err
	}

	prompts := repo.NewPrompts(pool)
	services := repo.NewServices(pool)
	res, err := bootstrap.Content(ctx, prompts, services, cfg.PromptsDir, cfg.DataDir, log)
	if err != nil {
		return err
	}

	fmt.Printf("bloques de contenido cargados: %d\nprompts nuevos: %d\nservicios nuevos: %d\n",
		count, res.Prompts, res.Services)
	return nil
}

func seedMessages(ctx context.Context, instructions *repo.Instructions, log *slog.Logger, dataDir string) (int, error) {
	raw, err := os.ReadFile(filepath.Join(dataDir, "messages.json"))
	if err != nil {
		return 0, fmt.Errorf("leer messages.json: %w", err)
	}
	var messages []messageJSON
	if err := json.Unmarshal(raw, &messages); err != nil {
		return 0, fmt.Errorf("parsear messages.json: %w", err)
	}

	count := 0
	for _, m := range messages {
		if m.Code == "" {
			continue
		}
		block := repo.Block{
			Code:        m.Code,
			Name:        m.Name,
			Type:        "fix_message",
			Description: m.Description,
			Actions:     m.Actions,
			Attachments: m.Attachments,
		}
		translations := []repo.Translation{}
		if m.Content != "" {
			content := m.Content
			if strings.HasPrefix(content, "/data/") {
				if data, err := os.ReadFile(filepath.Join(dataDir, strings.TrimPrefix(content, "/data/"))); err == nil {
					content = string(data)
				}
			}
			translations = append(translations, repo.Translation{Language: "es", Content: content})
		}
		if err := instructions.Upsert(ctx, m.Code, block, translations); err != nil {
			log.Warn("no se pudo cargar bloque", "code", m.Code, "error", err.Error())
			continue
		}
		count++
	}
	return count, nil
}
