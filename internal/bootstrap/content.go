// Package bootstrap siembra en PostgreSQL los assets iniciales (prompts y
// catálogo de servicios) cuando aún no existen. A partir de ahí, la BD es la
// fuente de verdad y los .md/.json solo actúan como seed/export.
package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/aibotia/aibotia-go/internal/store/repo"
)

// Result resume lo insertado por el bootstrap.
type Result struct {
	Prompts  int
	Services int
}

// Content siembra prompts desde promptsDir y servicios desde los assets.
// No sobrescribe filas existentes, por lo que es seguro ejecutarlo en cada arranque.
func Content(ctx context.Context, prompts *repo.Prompts, services *repo.Services, promptsDir, dataDir string, log *slog.Logger) (Result, error) {
	var res Result

	// Solo se siembra en la primera instalación (tablas vacías): así un
	// elemento borrado desde el panel no reaparece en el siguiente arranque.
	promptCount, err := prompts.Count(ctx)
	if err != nil {
		return res, err
	}
	serviceCount, err := services.Count(ctx)
	if err != nil {
		return res, err
	}
	needPrompts := promptCount == 0
	needServices := serviceCount == 0
	if !needPrompts && !needServices {
		return res, nil
	}

	parsed, notes, err := loadServices(promptsDir, dataDir)
	if err != nil {
		return res, err
	}
	if needPrompts && notes != "" {
		if _, err := prompts.Seed(ctx, []repo.Prompt{{
			Code:        "prompt_services_notes",
			Name:        "Notas del catálogo de servicios",
			Description: "Preamble y avisos extraídos de prompt_services_list.md",
			Content:     notes,
			Active:      true,
		}}); err != nil {
			return res, err
		}
	}
	if needPrompts {
		promptItems, err := loadPrompts(promptsDir)
		if err != nil {
			return res, err
		}
		res.Prompts, err = prompts.Seed(ctx, promptItems)
		if err != nil {
			return res, err
		}
	}
	if needServices {
		res.Services, err = services.Seed(ctx, parsed)
		if err != nil {
			return res, err
		}
	}

	if log != nil && (res.Prompts > 0 || res.Services > 0) {
		log.Info("bootstrap de contenido", "prompts", res.Prompts, "servicios", res.Services)
	}
	return res, nil
}

func loadPrompts(dir string) ([]repo.Prompt, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("bootstrap: leer prompts: %w", err)
	}
	out := make([]repo.Prompt, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("bootstrap: leer %s: %w", path, err)
		}
		if e.Name() == "prompt_services_list.md" {
			// Se representa como catálogo estructurado + prompt_services_notes.
			continue
		}
		code := strings.TrimSuffix(e.Name(), ".md")
		out = append(out, repo.Prompt{
			Code:        code,
			Name:        code,
			Description: "Importado desde " + e.Name(),
			Content:     string(raw),
			Active:      true,
		})
	}
	return out, nil
}
