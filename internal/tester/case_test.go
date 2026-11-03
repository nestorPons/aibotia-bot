package tester

import (
	"math/rand"
	"os"
	"path/filepath"
	"testing"
)

func writeCase(t *testing.T, dir, name, id string) {
	t.Helper()
	body := `{"id":"` + id + `","name":"` + name + `","openers":["hola"],"questions":["uno","dos"],"turns":{"min":1,"max":2}}`
	if err := os.WriteFile(filepath.Join(dir, name+".json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadCasesAndPickAvoidsLast(t *testing.T) {
	dir := t.TempDir()
	writeCase(t, dir, "a", "a")
	writeCase(t, dir, "b", "b")

	cases, err := LoadCases(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 2 || cases[0].ID != "a" || cases[1].ID != "b" {
		t.Fatalf("casos = %+v", cases)
	}
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 20; i++ {
		if got := PickCase(cases, rng, "a"); got.ID != "b" {
			t.Fatalf("PickCase evitando 'a' devolvió %q", got.ID)
		}
	}
}

func TestLastCaseRoundTrip(t *testing.T) {
	dir := t.TempDir()
	SaveLastCase(dir, "adversarial")
	if got := LastCase(dir); got != "adversarial" {
		t.Fatalf("LastCase = %q", got)
	}
}

func TestIsDuplicateNormalizes(t *testing.T) {
	used := []string{"¿Cuánto cuesta?"}
	if !isDuplicate("cuanto   cuesta", used) {
		t.Fatal("debería detectar la repetición")
	}
	if isDuplicate("¿Dónde estáis?", used) {
		t.Fatal("no debería ser duplicada")
	}
}

func TestCaseTurnCountAndBank(t *testing.T) {
	c := Case{Turns: CaseTurns{Min: 3, Max: 5}, Questions: []string{"a", "b", "c"}}
	rng := rand.New(rand.NewSource(7))
	for i := 0; i < 50; i++ {
		if n := c.TurnCount(rng, 0); n < 3 || n > 5 {
			t.Fatalf("TurnCount = %d", n)
		}
	}
	if n := c.TurnCount(rng, 9); n != 9 {
		t.Fatalf("override ignorado: %d", n)
	}
	if got := len(c.ShuffledQuestions(rng)); got != 3 {
		t.Fatalf("banco = %d", got)
	}
}
