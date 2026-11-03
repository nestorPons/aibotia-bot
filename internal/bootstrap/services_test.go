package bootstrap

import (
	"strings"
	"testing"

	"github.com/aibotia/aibotia-go/internal/store/repo"
)

func TestParseServicesMarkdown(t *testing.T) {
	md := `# Lista de precios

> Solo están disponibles los servicios incluidos en esta lista.

### Tratamiento realizado por Tamara Freitas
- **nano_tf_cejas**: Cejas: 390 € [TF]

## Servicios faciales
- **fac_hig_90**: Higiene facial: 65 € · 90 min [EM]
- **fac_crio**: Crioterapia: Precios a consultar [EM, LL]
- **las_z_mini**: **ZONAS MINI** - 20 € por zona [LL]

⚠️ **ZONAS NO LISTADAS:**
- Si preguntan por una zona, indica Zonas Individuales.
`
	services, notes := ParseServicesMarkdown(md)

	if len(services) != 4 {
		t.Fatalf("servicios = %d, quiero 4", len(services))
	}

	tf := services[0]
	if tf.ID != "nano_tf_cejas" || tf.Category != "Tratamiento realizado por Tamara Freitas" {
		t.Fatalf("primer servicio inesperado: %+v", tf)
	}
	if tf.Price == nil || *tf.Price != 390 {
		t.Fatalf("precio de %s = %v", tf.ID, tf.Price)
	}
	if len(tf.Providers) != 1 || tf.Providers[0] != "TF" {
		t.Fatalf("providers de %s = %v", tf.ID, tf.Providers)
	}

	hig := services[1]
	if hig.Category != "Servicios faciales" || hig.DurationMin == nil || *hig.DurationMin != 90 {
		t.Fatalf("servicio facial inesperado: %+v", hig)
	}
	if hig.Name != "Higiene facial" {
		t.Fatalf("nombre = %q", hig.Name)
	}
	if hig.Note != "" {
		t.Fatalf("nota inesperada = %q", hig.Note)
	}

	crio := services[2]
	if crio.Price != nil || !strings.Contains(crio.Note, "consultar") {
		t.Fatalf("servicio a consultar inesperado: %+v", crio)
	}
	if len(crio.Providers) != 2 {
		t.Fatalf("providers = %v", crio.Providers)
	}

	zonas := services[3]
	if zonas.Name != "ZONAS MINI" || zonas.Price == nil || *zonas.Price != 20 {
		t.Fatalf("zonas inesperado: %+v", zonas)
	}

	if !strings.Contains(notes, "Solo están disponibles") || !strings.Contains(notes, "ZONAS NO LISTADAS") {
		t.Fatalf("notas incompletas:\n%s", notes)
	}
}

func TestMergeServices(t *testing.T) {
	first := 65.0
	base := []repo.Service{{ID: "fac_hig_90", Name: "Higiene facial", Price: &first}}
	duration := 60
	merged := mergeServices(base, []repo.Service{
		{ID: "fac_hig_90", Description: "Limpieza profunda", DurationMin: &duration, Note: "nota", Providers: []string{"EM"}},
		{ID: "nuevo", Name: "Nuevo servicio", Category: "Extra"},
	})

	if len(merged) != 2 {
		t.Fatalf("merged = %d, quiero 2", len(merged))
	}
	if merged[0].Price == nil || *merged[0].Price != first {
		t.Fatalf("el precio base no debe perderse: %+v", merged[0])
	}
	if merged[0].DurationMin == nil || *merged[0].DurationMin != 60 || merged[0].Note != "nota" {
		t.Fatalf("override incompleto: %+v", merged[0])
	}
	if merged[0].Description != "Limpieza profunda" {
		t.Fatalf("descripción no aplicada: %+v", merged[0])
	}
	if merged[1].ID != "nuevo" || merged[1].SortOrder != 1 {
		t.Fatalf("servicio nuevo inesperado: %+v", merged[1])
	}
}
