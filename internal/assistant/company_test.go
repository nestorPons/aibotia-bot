package assistant

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/aibotia/aibotia-go/internal/store/repo"
)

func TestCompileIncludesCompanyBlock(t *testing.T) {
	p := &PromptFactory{
		log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		loaded: true,
		cache: map[string]string{
			"prompt_instructions":  "base",
			"schema_response":      "schema",
			repo.CompanyPromptCode: "<!-- pin: 39.4,-0.3 | Studio | Calle -->\nAv. Instituto Obrero, 21",
		},
	}
	out := p.Compile(context.Background())
	if !strings.Contains(out, "### DATOS DE EMPRESA\nAv. Instituto Obrero, 21") {
		t.Fatalf("falta el bloque de empresa:\n%s", out)
	}
	if strings.Contains(out, "<!-- pin:") {
		t.Fatalf("la línea pin no debe enviarse al modelo:\n%s", out)
	}
	if !strings.Contains(out, "`send_location`") {
		t.Fatalf("falta la instrucción de send_location:\n%s", out)
	}
}

func TestIsCompanyTopic(t *testing.T) {
	if !IsCompanyTopic(repo.CompanyPromptCode) {
		t.Fatal("prompt_company debe ser tema de empresa")
	}
	for _, legacy := range repo.LegacyLocationPrompts {
		if !IsCompanyTopic(legacy) {
			t.Fatalf("%s debe considerarse tema de empresa", legacy)
		}
	}
	if IsCompanyTopic("prompt_laser_diodo") {
		t.Fatal("un prompt de servicio no debe ser tema de empresa")
	}
}
