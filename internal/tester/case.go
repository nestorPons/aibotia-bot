package tester

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// CaseTurns es el rango de turnos recomendado para un caso.
type CaseTurns struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

// Case describe un caso de uso de prueba: quién escribe, con qué objetivo y
// qué se espera del bot.
type Case struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Persona   string    `json:"persona"`
	Goal      string    `json:"goal"`
	Tone      string    `json:"tone"`
	Setup     []string  `json:"setup"`
	Openers   []string  `json:"openers"`
	Questions []string  `json:"questions"`
	Turns     CaseTurns `json:"turns"`
	Must      []string  `json:"must"`
	MustNot   []string  `json:"must_not"`
	Finish    string    `json:"finish"`
}

// LoadCases lee todos los casos JSON de un directorio.
func LoadCases(dir string) ([]Case, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("tester: leer casos: %w", err)
	}
	out := make([]Case, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("tester: leer %s: %w", e.Name(), err)
		}
		var c Case
		if err := json.Unmarshal(raw, &c); err != nil {
			return nil, fmt.Errorf("tester: caso inválido %s: %w", e.Name(), err)
		}
		if strings.TrimSpace(c.ID) == "" {
			return nil, fmt.Errorf("tester: el caso %s no tiene id", e.Name())
		}
		if c.Name == "" {
			c.Name = c.ID
		}
		if c.Turns.Min <= 0 {
			c.Turns.Min = 6
		}
		if c.Turns.Max < c.Turns.Min {
			c.Turns.Max = c.Turns.Min
		}
		out = append(out, c)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("tester: no hay casos en %s", dir)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// FindCase busca un caso por id.
func FindCase(cases []Case, id string) (Case, error) {
	for _, c := range cases {
		if c.ID == id {
			return c, nil
		}
	}
	ids := make([]string, 0, len(cases))
	for _, c := range cases {
		ids = append(ids, c.ID)
	}
	return Case{}, fmt.Errorf("tester: caso %q no encontrado (disponibles: %s)", id, strings.Join(ids, ", "))
}

// PickCase elige un caso aleatorio evitando el último usado cuando sea posible.
func PickCase(cases []Case, rng *rand.Rand, exclude string) Case {
	pool := make([]Case, 0, len(cases))
	for _, c := range cases {
		if c.ID != exclude {
			pool = append(pool, c)
		}
	}
	if len(pool) == 0 {
		pool = cases
	}
	return pool[rng.Intn(len(pool))]
}

// Opener devuelve un saludo inicial aleatorio del caso.
func (c Case) Opener(rng *rand.Rand) string {
	if len(c.Openers) == 0 {
		return "Hola, quería información sobre los servicios."
	}
	return c.Openers[rng.Intn(len(c.Openers))]
}

// TurnCount devuelve los turnos a realizar (override manda si es > 0).
func (c Case) TurnCount(rng *rand.Rand, override int) int {
	if override > 0 {
		return override
	}
	span := c.Turns.Max - c.Turns.Min
	if span <= 0 {
		return c.Turns.Min
	}
	return c.Turns.Min + rng.Intn(span+1)
}

// ShuffledQuestions devuelve una copia barajada del banco de preguntas.
func (c Case) ShuffledQuestions(rng *rand.Rand) []string {
	out := append([]string(nil), c.Questions...)
	rng.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out
}

// statePath guarda el último caso usado junto al directorio de casos.
func statePath(casesDir string) string {
	return filepath.Join(filepath.Dir(casesDir), ".last-case")
}

// LastCase devuelve el último caso ejecutado (vacío si no hay).
func LastCase(casesDir string) string {
	raw, err := os.ReadFile(statePath(casesDir))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

// SaveLastCase registra el último caso ejecutado para no repetirlo.
func SaveLastCase(casesDir, id string) {
	_ = os.MkdirAll(filepath.Dir(statePath(casesDir)), 0o755)
	_ = os.WriteFile(statePath(casesDir), []byte(id+"\n"), 0o644)
}

// PrintCases muestra el catálogo de casos disponibles y cómo ejecutarlos.
func PrintCases(cases []Case, casesDir string) {
	last := LastCase(casesDir)
	fmt.Printf("Casos disponibles (%d) en %s\n\n", len(cases), casesDir)
	fmt.Printf("%-24s %-32s %-8s %-6s %s\n", "ID", "NOMBRE", "TURNOS", "SETUP", "OBJETIVO")
	for _, c := range cases {
		setup := "—"
		if len(c.Setup) > 0 {
			setup = fmt.Sprintf("%d", len(c.Setup))
		}
		mark := ""
		if c.ID == last {
			mark = "  ← último"
		}
		fmt.Printf("%-24s %-32s %-8s %-6s %s%s\n",
			c.ID, truncate(c.Name, 32),
			fmt.Sprintf("%d-%d", c.Turns.Min, c.Turns.Max),
			setup, truncate(c.Goal, 58), mark)
	}
	fmt.Println()
	fmt.Println("Ejecutar uno:   make tester-case CASE=<id>")
	fmt.Println("Ejecutar todos: make tester-suite SUITE=all")
	fmt.Println("Aleatorio:      make tester")
}

func truncate(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max <= 1 {
		return string(r[:max])
	}
	return string(r[:max-1]) + "…"
}
