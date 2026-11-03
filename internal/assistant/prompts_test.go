package assistant

import (
	"strings"
	"testing"

	"github.com/aibotia/aibotia-go/internal/store/repo"
)

func TestRenderServices(t *testing.T) {
	price := 65.0
	duration := 90
	out := renderServices([]repo.Service{
		{ID: "fac_hig_90", Category: "Facial", Name: "Higiene facial", Description: "Limpieza profunda", Price: &price, DurationMin: &duration, Providers: []string{"EM"}, Note: "nota"},
		{ID: "fac_otro", Category: "Facial", Name: "Otro"},
		{ID: "corp_otro", Category: "Corporal", Name: "Corporal"},
	})

	if !strings.Contains(out, "## Facial\n") || !strings.Contains(out, "## Corporal\n") {
		t.Fatalf("faltan categorías:\n%s", out)
	}
	if !strings.Contains(out, "- **fac_hig_90**: Higiene facial: 65 € · 90 min [EM] — Limpieza profunda (nota)") {
		t.Fatalf("formato inesperado:\n%s", out)
	}
	if !strings.Contains(out, "- **fac_otro**: Otro\n") {
		t.Fatalf("servicio sin extras inesperado:\n%s", out)
	}
}
