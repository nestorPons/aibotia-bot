// Comando embed: genera/actualiza los embeddings del RAG a partir de los
// prompts y del catálogo de temas (data/seeds/embeddings.json).
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
	openai "github.com/sashabaranov/go-openai"

	"github.com/aibotia/aibotia-go/internal/config"
	"github.com/aibotia/aibotia-go/internal/platform/logger"
	"github.com/aibotia/aibotia-go/internal/rag"
	"github.com/aibotia/aibotia-go/internal/store/postgres"
	"github.com/aibotia/aibotia-go/internal/store/repo"
)

type topicEntry struct {
	Name              string `json:"name"`
	Keywords          string `json:"keywords"`
	AdditionalContent string `json:"additional_content"`
	Code              string `json:"code"`
	ContextNotes      string `json:"context_notes"`
}

func main() {
	if err := run(); err != nil {
		slog.Error("error generando embeddings", "error", err.Error())
		os.Exit(1)
	}
}

func run() error {
	_ = godotenv.Load(".env")

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.OpenAIAPIKey == "" {
		return fmt.Errorf("OPENAI_API_KEY es obligatoria para generar embeddings")
	}
	log := logger.New(cfg.LogLevel, cfg.LogFormat)

	ctx := context.Background()
	pool, err := postgres.Connect(ctx, cfg.DatabaseURL, log)
	if err != nil {
		return err
	}
	defer pool.Close()

	client := openai.NewClient(cfg.OpenAIAPIKey)
	svc := rag.New(pool, client, cfg.OpenAIEmbedModel, cfg.RAGThreshold, log)

	promptCount, err := ingestPrompts(ctx, svc, repo.NewPrompts(pool))
	if err != nil {
		return err
	}
	topicCount, err := ingestTopics(ctx, svc, log, filepath.Join(cfg.DataDir, "embeddings.json"))
	if err != nil {
		return err
	}

	fmt.Printf("embeddings generados: %d prompts, %d temas\n", promptCount, topicCount)
	return nil
}

func ingestPrompts(ctx context.Context, svc *rag.Service, prompts *repo.Prompts) (int, error) {
	snapshot, err := prompts.Snapshot(ctx)
	if err != nil {
		return 0, fmt.Errorf("leer prompts de BD: %w", err)
	}
	count := 0
	for code, content := range snapshot {
		if !strings.HasPrefix(code, "prompt_") || strings.TrimSpace(content) == "" {
			continue
		}
		if ragExcluded(code) {
			continue
		}
		vec, err := svc.Embed(ctx, content)
		if err != nil {
			return count, fmt.Errorf("embed %s: %w", code, err)
		}
		if err := svc.Upsert(ctx, code, code, content, vec); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func ingestTopics(ctx context.Context, svc *rag.Service, log *slog.Logger, path string) (int, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		log.Warn("no se encontró catálogo de temas; se omite", "path", path, "error", err.Error())
		return 0, nil
	}
	var topics []topicEntry
	if err := json.Unmarshal(raw, &topics); err != nil {
		return 0, fmt.Errorf("parsear embeddings.json: %w", err)
	}
	count := 0
	for _, t := range topics {
		if t.Code == "" || ragExcluded(t.Code) {
			continue
		}
		full := fmt.Sprintf("Nombre: %s\nPalabras clave: %s\n%s\nNotas de contexto: %s",
			t.Name, t.Keywords, t.AdditionalContent, t.ContextNotes)
		vec, err := svc.Embed(ctx, full)
		if err != nil {
			return count, fmt.Errorf("embed %s: %w", t.Code, err)
		}
		if err := svc.Upsert(ctx, t.Code, t.Code, full, vec); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

// ragExcluded indica los codes que no se indexan en el RAG: los datos de
// empresa van siempre en el system prompt y los antiguos ya no existen.
func ragExcluded(code string) bool {
	if code == repo.CompanyPromptCode {
		return true
	}
	for _, legacy := range repo.LegacyLocationPrompts {
		if code == legacy {
			return true
		}
	}
	return false
}
