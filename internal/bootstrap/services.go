package bootstrap

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/aibotia/aibotia-go/internal/store/repo"
)

var (
	serviceLineRe = regexp.MustCompile(`^\s*-\s+\*\*([A-Za-z0-9_]+)\*\*:\s*(.+?)\s*$`)
	providersRe   = regexp.MustCompile(`\[([A-Z]{2}(?:\s*,\s*[A-Z]{2})*)\]\s*$`)
	priceRe       = regexp.MustCompile(`(\d+(?:[.,]\d+)?)\s*€`)
	durationRe    = regexp.MustCompile(`(\d+)\s*min`)
)

type jsonService struct {
	ID          string          `json:"id"`
	Category    string          `json:"category"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Price       *float64        `json:"price"`
	DurationMin *int            `json:"duration_min"`
	Note        json.RawMessage `json:"note"`
	Providers   []string        `json:"providers"`
}

// loadServices combina el catálogo del markdown (completo) con los metadatos de
// data/services.json y devuelve también las notas globales del catálogo.
func loadServices(promptsDir, dataDir string) ([]repo.Service, string, error) {
	raw, err := os.ReadFile(filepath.Join(promptsDir, "prompt_services_list.md"))
	if err != nil {
		return nil, "", fmt.Errorf("bootstrap: leer prompt_services_list.md: %w", err)
	}
	parsed, notes := ParseServicesMarkdown(string(raw))

	jsonPath := filepath.Join(dataDir, "services.json")
	if extra, err := loadServicesJSON(jsonPath); err == nil {
		parsed = mergeServices(parsed, extra)
	}
	return parsed, notes, nil
}

// ParseServicesMarkdown extrae los servicios de la lista de precios en markdown.
func ParseServicesMarkdown(content string) ([]repo.Service, string) {
	var (
		services   []repo.Service
		notes      []string
		category   string
		topSection bool
		order      int
	)
	for _, rawLine := range strings.Split(content, "\n") {
		line := strings.TrimSpace(rawLine)
		switch {
		case strings.HasPrefix(line, "## "):
			category = strings.TrimSpace(strings.TrimPrefix(line, "## "))
			topSection = true
			continue
		case strings.HasPrefix(line, "### ") && !topSection:
			category = strings.TrimSpace(strings.TrimPrefix(line, "### "))
			continue
		case line == "", line == "---", strings.HasPrefix(line, "#"):
			continue
		}

		m := serviceLineRe.FindStringSubmatch(line)
		if m == nil {
			notes = append(notes, line)
			continue
		}
		s := parseServiceLine(m[1], m[2])
		s.Category = category
		s.SortOrder = order
		s.Active = true
		services = append(services, s)
		order++
	}
	return services, strings.TrimSpace(strings.Join(notes, "\n"))
}

func parseServiceLine(id, desc string) repo.Service {
	s := repo.Service{ID: id, Providers: []string{}}

	if m := providersRe.FindStringSubmatch(desc); m != nil {
		for _, p := range strings.Split(m[1], ",") {
			s.Providers = append(s.Providers, strings.TrimSpace(p))
		}
		desc = strings.TrimSpace(providersRe.ReplaceAllString(desc, ""))
	}

	name, rest := splitName(desc)
	if m := priceRe.FindStringSubmatch(rest); m != nil {
		if v, err := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", "."), 64); err == nil {
			s.Price = &v
		}
		rest = priceRe.ReplaceAllString(rest, "")
	} else if strings.Contains(strings.ToLower(rest), "gratuit") {
		zero := 0.0
		s.Price = &zero
		rest = ""
	}
	if m := durationRe.FindStringSubmatch(rest); m != nil {
		if v, err := strconv.Atoi(m[1]); err == nil {
			s.DurationMin = &v
		}
		rest = durationRe.ReplaceAllString(rest, "")
	}

	s.Name = name
	s.Note = strings.Trim(strings.TrimSpace(rest), " ·-:€")
	return s
}

func splitName(desc string) (name, rest string) {
	if i := strings.Index(desc, ":"); i >= 0 {
		return cleanName(desc[:i]), desc[i+1:]
	}
	if i := strings.Index(desc, " - "); i >= 0 {
		return cleanName(desc[:i]), desc[i+3:]
	}
	return cleanName(desc), desc
}

func cleanName(s string) string {
	s = strings.ReplaceAll(s, "**", "")
	return strings.TrimSpace(strings.Trim(s, "*"))
}

func loadServicesJSON(path string) ([]repo.Service, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var items []jsonService
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, err
	}
	out := make([]repo.Service, 0, len(items))
	for _, it := range items {
		out = append(out, repo.Service{
			ID:          it.ID,
			Category:    it.Category,
			Name:        it.Name,
			Description: it.Description,
			Price:       it.Price,
			DurationMin: it.DurationMin,
			Note:        flattenNote(it.Note),
			Providers:   it.Providers,
			Active:      true,
		})
	}
	return out, nil
}

func flattenNote(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		return single
	}
	var list []string
	if err := json.Unmarshal(raw, &list); err == nil {
		return strings.Join(list, " ")
	}
	return ""
}

func mergeServices(base, extra []repo.Service) []repo.Service {
	index := make(map[string]int, len(base))
	for i, s := range base {
		index[s.ID] = i
	}
	for _, e := range extra {
		i, ok := index[e.ID]
		if !ok {
			e.SortOrder = len(base)
			base = append(base, e)
			index[e.ID] = len(base) - 1
			continue
		}
		if e.Price != nil {
			base[i].Price = e.Price
		}
		if e.DurationMin != nil {
			base[i].DurationMin = e.DurationMin
		}
		if strings.TrimSpace(e.Description) != "" {
			base[i].Description = e.Description
		}
		if strings.TrimSpace(e.Note) != "" {
			base[i].Note = e.Note
		}
		if len(e.Providers) > 0 {
			base[i].Providers = e.Providers
		}
		if e.Name != "" {
			base[i].Name = e.Name
		}
	}
	return base
}
