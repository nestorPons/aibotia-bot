package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/aibotia/aibotia-go/internal/store/repo"
)

// companyPin es la línea técnica que usa `send_location` para el pin de
// WhatsApp. El texto de la ubicación vive en el resto del prompt.
const companyPin = `<!-- pin: 39.4555556,-0.3583096 | Studio Tamara Freitas | Av. Instituto Obrero de Valencia, Nº 21 – 1ª Planta, Local 4 -->`

// Company crea el prompt único de datos de empresa fusionando los prompts
// antiguos de ubicación y los elimina (junto con sus embeddings). Es
// idempotente: solo actúa mientras existan los codes antiguos.
func Company(ctx context.Context, db *pgxpool.Pool, prompts *repo.Prompts, promptsDir string, log *slog.Logger) (bool, error) {
	var oldContents []string
	hadLegacy := false
	for _, code := range repo.LegacyLocationPrompts {
		p, err := prompts.Get(ctx, code)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return false, fmt.Errorf("bootstrap: leer %s: %w", code, err)
		}
		hadLegacy = true
		if strings.TrimSpace(p.Content) != "" {
			oldContents = append(oldContents, p.Content)
		}
	}

	created := false
	_, err := prompts.Get(ctx, repo.CompanyPromptCode)
	if errors.Is(err, pgx.ErrNoRows) {
		// No resucitar el prompt si se borró desde el panel: solo se crea en
		// instalaciones nuevas o para fusionar los prompts antiguos.
		count, cerr := prompts.Count(ctx)
		if cerr != nil {
			return false, fmt.Errorf("bootstrap: contar prompts: %w", cerr)
		}
		if !hadLegacy && count > 0 {
			return false, nil
		}
		content := mergeCompany(oldContents...)
		if strings.TrimSpace(content) == "" {
			content = readCompanySeed(promptsDir)
		}
		if strings.TrimSpace(content) == "" {
			if hadLegacy {
				return false, fmt.Errorf("bootstrap: no hay contenido para %s", repo.CompanyPromptCode)
			}
			return false, nil
		}
		content = ensurePin(content)
		if _, err := prompts.Create(ctx, repo.Prompt{
			Code:        repo.CompanyPromptCode,
			Name:        "Datos de empresa",
			Description: "Dirección, ubicación, contacto y horarios. Se inyecta siempre en el prompt del sistema.",
			Content:     content,
			Active:      true,
		}); err != nil {
			return false, fmt.Errorf("bootstrap: crear %s: %w", repo.CompanyPromptCode, err)
		}
		created = true
	} else if err != nil {
		return false, fmt.Errorf("bootstrap: leer %s: %w", repo.CompanyPromptCode, err)
	}

	for _, code := range repo.LegacyLocationPrompts {
		if err := prompts.Delete(ctx, code); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return created, fmt.Errorf("bootstrap: borrar %s: %w", code, err)
		}
	}
	if _, err := db.Exec(ctx,
		`DELETE FROM embeddings WHERE code = ANY($1)`, repo.LegacyLocationPrompts); err != nil {
		return created, fmt.Errorf("bootstrap: borrar embeddings de ubicación: %w", err)
	}
	if created && log != nil {
		log.Info("prompt de empresa creado", "code", repo.CompanyPromptCode, "fusionados", repo.LegacyLocationPrompts)
	}
	return created, nil
}

// mergeCompany une los textos antiguos sin duplicar el contenido cuando uno
// contiene al otro.
func mergeCompany(parts ...string) string {
	out := ""
	for _, raw := range parts {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if out == "" {
			out = raw
			continue
		}
		if containsNormalized(out, raw) {
			continue
		}
		if containsNormalized(raw, out) {
			out = raw
			continue
		}
		out += "\n\n" + raw
	}
	return out
}

func containsNormalized(haystack, needle string) bool {
	norm := func(s string) string {
		return strings.Join(strings.Fields(strings.ToLower(s)), " ")
	}
	return strings.Contains(norm(haystack), norm(needle))
}

func readCompanySeed(dir string) string {
	if dir == "" {
		return ""
	}
	if raw, err := os.ReadFile(filepath.Join(dir, "prompt_company.md")); err == nil {
		return string(raw)
	}
	var parts []string
	for _, name := range []string{"prompt_location_contact.md", "prompt_location.md"} {
		if raw, err := os.ReadFile(filepath.Join(dir, name)); err == nil {
			parts = append(parts, string(raw))
		}
	}
	return mergeCompany(parts...)
}

func ensurePin(content string) string {
	if strings.Contains(content, "<!-- pin:") {
		return content
	}
	return companyPin + "\n\n" + content
}
