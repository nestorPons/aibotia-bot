package assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/aibotia/aibotia-go/internal/store/repo"
)

var pinCommentRe = regexp.MustCompile(`(?is)<!--\s*pin:.*?-->`)

// stripPin elimina la línea técnica del pin antes de enviar el prompt al modelo.
func stripPin(s string) string {
	return strings.TrimSpace(pinCommentRe.ReplaceAllString(s, ""))
}

// IsCompanyTopic indica si un code corresponde a los datos de empresa. Ese
// contenido ya se inyecta siempre en el system prompt, así que no debe tratarse
// como tema RAG (evitaría duplicarlo).
func IsCompanyTopic(code string) bool {
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

// PromptFactory compone el system prompt a partir de los prompts, del catálogo
// de servicios y de los mensajes fijos almacenados en PostgreSQL. Mantiene una
// instantánea de prompts/servicios en memoria que se invalida desde el panel de
// administración (Reload); los mensajes fijos se leen de content_blocks.
type PromptFactory struct {
	prompts      *repo.Prompts
	services     *repo.Services
	instructions *repo.Instructions
	log          *slog.Logger

	mu      sync.RWMutex
	loaded  bool
	cache   map[string]string
	catalog []repo.Service
}

// NewPromptFactory construye la factoría.
func NewPromptFactory(prompts *repo.Prompts, services *repo.Services, instructions *repo.Instructions, log *slog.Logger) *PromptFactory {
	return &PromptFactory{
		prompts:      prompts,
		services:     services,
		instructions: instructions,
		log:          log,
		cache:        make(map[string]string),
	}
}

// Reload recarga la instantánea de prompts y servicios desde PostgreSQL.
func (p *PromptFactory) Reload(ctx context.Context) error {
	snapshot, err := p.prompts.Snapshot(ctx)
	if err != nil {
		return err
	}
	var catalog []repo.Service
	if p.services != nil {
		catalog, err = p.services.ListActive(ctx)
		if err != nil {
			return err
		}
	}
	p.mu.Lock()
	p.cache = snapshot
	p.catalog = catalog
	p.loaded = true
	p.mu.Unlock()
	return nil
}

// Get devuelve el contenido de un prompt (vacío si no existe).
func (p *PromptFactory) Get(name string) string {
	if strings.TrimSpace(name) == "" {
		return ""
	}
	p.ensureLoaded()
	p.mu.RLock()
	v, ok := p.cache[name]
	p.mu.RUnlock()
	if !ok {
		p.log.Warn("prompt no encontrado", "code", name)
	}
	return v
}

// Compile compone el system prompt base.
func (p *PromptFactory) Compile(ctx context.Context) string {
	p.ensureLoaded()

	base := p.Get("prompt_instructions")
	schema := p.Get("schema_response")
	services := p.servicesSection()
	notes := p.Get("prompt_services_notes")
	company := stripPin(p.Get(repo.CompanyPromptCode))

	parts := []string{base}
	if strings.TrimSpace(company) != "" {
		parts = append(parts, "### DATOS DE EMPRESA\n"+company+
			"\n\nSi el cliente pregunta por la ubicación o cómo llegar, usa la herramienta `send_location` además de responder con la dirección.")
	}
	parts = append(parts,
		"### SCHEMA RESPONSE\n"+schema,
		"### FUNCIONES DISPONIBLES\n"+p.dynamicFunctions(ctx),
		"### SERVICIOS\n"+services,
	)
	if strings.TrimSpace(notes) != "" {
		parts = append(parts, "### NOTAS DE SERVICIOS\n"+notes)
	}
	return strings.Join(parts, "\n---\n")
}

func (p *PromptFactory) servicesSection() string {
	p.ensureLoaded()
	p.mu.RLock()
	catalog := p.catalog
	p.mu.RUnlock()

	return renderServices(catalog)
}

func renderServices(catalog []repo.Service) string {
	var b strings.Builder
	lastCategory := ""
	for _, s := range catalog {
		if s.Category != lastCategory {
			if lastCategory != "" {
				b.WriteString("\n")
			}
			fmt.Fprintf(&b, "## %s\n", s.Category)
			lastCategory = s.Category
		}
		fmt.Fprintf(&b, "- **%s**: %s", s.ID, s.Name)
		if s.Price != nil {
			fmt.Fprintf(&b, ": %s €", strconv.FormatFloat(*s.Price, 'f', -1, 64))
		}
		if s.DurationMin != nil {
			fmt.Fprintf(&b, " · %d min", *s.DurationMin)
		}
		if len(s.Providers) > 0 {
			fmt.Fprintf(&b, " [%s]", strings.Join(s.Providers, ", "))
		}
		if strings.TrimSpace(s.Description) != "" {
			fmt.Fprintf(&b, " — %s", strings.TrimSpace(s.Description))
		}
		if strings.TrimSpace(s.Note) != "" {
			fmt.Fprintf(&b, " (%s)", s.Note)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// GetPrompt devuelve la fila completa de un prompt desde PostgreSQL.
func (p *PromptFactory) GetPrompt(ctx context.Context, code string) (repo.Prompt, error) {
	return p.prompts.Get(ctx, code)
}

func (p *PromptFactory) ensureLoaded() {
	p.mu.RLock()
	loaded := p.loaded
	p.mu.RUnlock()
	if loaded {
		return
	}
	if err := p.Reload(context.Background()); err != nil {
		p.log.Error("no se pudieron cargar los prompts desde la BD", "error", err.Error())
		p.mu.Lock()
		p.loaded = true
		p.mu.Unlock()
	}
}

func (p *PromptFactory) dynamicFunctions(ctx context.Context) string {
	if p.instructions == nil {
		return ""
	}
	blocks, err := p.instructions.List(ctx)
	if err != nil {
		p.log.Warn("no se pudieron cargar los mensajes fijos desde la BD", "error", err.Error())
		return ""
	}
	var lines []string
	for _, b := range blocks {
		if !isFixMessage(b.Type) {
			continue
		}
		desc := strings.TrimSpace(b.Description)
		content := blockContent(b)
		if desc == "" || content == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("- si => %s contesta => %s y ejecuta => %v",
			desc, content, blockActions(b)))
	}
	return strings.Join(lines, "\n")
}

func isFixMessage(t string) bool {
	return t == "fix_message" || t == "message_fixed"
}

func blockContent(b repo.Block) string {
	for _, tr := range b.Translations {
		if strings.TrimSpace(tr.Content) != "" {
			return tr.Content
		}
	}
	return ""
}

func blockActions(b repo.Block) []string {
	var actions []string
	if len(b.Actions) == 0 || string(b.Actions) == "null" {
		return actions
	}
	if err := json.Unmarshal(b.Actions, &actions); err != nil {
		return []string{}
	}
	return actions
}
