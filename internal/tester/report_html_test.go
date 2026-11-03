package tester

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func sampleReport() Report {
	at := time.Date(2026, 11, 3, 11, 10, 49, 0, time.UTC)
	rep := Report{
		Started:  at,
		Finished: at.Add(3 * time.Minute),
		Target:   "34600111222",
		Own:      "34600999888",
		Scenario: "clienta_habitual",
		Case:     "clienta_habitual",
		CaseName: "Clienta habitual",
		Goal:     "Retomar su historial sin tratarla como nueva",
		Seed:     42,
		Mode:     "ai",
		Turns: []Turn{
			{
				At:        at,
				Question:  `Hola, <script>alert("x")</script> & quiero retomar mi repaso`,
				Source:    "ia",
				Replies:   []Reply{{At: at.Add(2 * time.Second), Text: "Claro, María & te ayudo con el repaso."}},
				LatencyMs: 2000,
				Evaluation: &Evaluation{
					Score:   4.5,
					Verdict: "correcto",
				},
			},
		},
	}
	rep.Summarize()
	return rep
}

func TestWriteReportIncluyeHTMLCompartible(t *testing.T) {
	dir := t.TempDir()
	rep := sampleReport()

	jsonPath, mdPath, htmlPath, clientPath, err := WriteReport(dir, rep)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{jsonPath, mdPath, htmlPath, clientPath} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("falta %s: %v", p, err)
		}
	}
	raw, err := os.ReadFile(htmlPath)
	if err != nil {
		t.Fatal(err)
	}
	htmlOut := string(raw)

	for _, want := range []string{"Clienta habitual", "Cliente (simulado)", "Asistente", "Nota media"} {
		if !strings.Contains(htmlOut, want) {
			t.Errorf("el HTML no contiene %q", want)
		}
	}
	if strings.Contains(htmlOut, "<script>alert") {
		t.Error("el HTML no debe inyectar contenido sin escapar")
	}
	if !strings.Contains(htmlOut, "&lt;script&gt;") {
		t.Error("el texto del cliente debe escaparse")
	}
	// Datos internos que no deben mostrarse a la clienta.
	for _, priv := range []string{rep.Target, rep.Own, "Semilla"} {
		if strings.Contains(htmlOut, priv) {
			t.Errorf("el HTML de cliente no debe incluir %q", priv)
		}
	}
}

func TestRenderSuiteHTMLIndexaCasos(t *testing.T) {
	a := sampleReport()
	a.CaseName = "Clienta habitual"
	b := sampleReport()
	b.Case = "adversarial"
	b.CaseName = "Adversarial"
	b.Turns[0].Evaluation = &Evaluation{Score: 1, Verdict: "falla", Hallucination: true}
	b.Summarize()

	out := renderSuiteHTML(SuiteReport{
		Started:  a.Started,
		Finished: b.Finished,
		Cases:    []Report{a, b},
	})
	for _, want := range []string{"Suite de pruebas", "Clienta habitual", "Adversarial", "#caso-1", "#caso-2", "1</b> OK", "1</b> con incidencias"} {
		if !strings.Contains(out, want) {
			t.Errorf("la suite HTML no contiene %q", want)
		}
	}
}

func TestReportsHandlerListaYEntregaLogs(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "aibotia-test-20261103-111049-caso.html")
	if err := os.WriteFile(file, []byte("<html><head><title>Prueba visible</title></head><body>hola</body></html>"), 0o644); err != nil {
		t.Fatal(err)
	}

	h := ReportsHandler(dir)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("index status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Prueba visible") {
		t.Error("el índice debe mostrar el título del reporte")
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/aibotia-test-20261103-111049-caso.html", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "hola") {
		t.Fatalf("log status = %d body = %q", rec.Code, rec.Body.String())
	}
}

func TestClientHTMLSinDatosInternos(t *testing.T) {
	dir := t.TempDir()
	rep := sampleReport()
	_, _, _, clientPath, err := WriteReport(dir, rep)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(clientPath)
	if err != nil {
		t.Fatal(err)
	}
	out := string(raw)
	for _, want := range []string{"Conversación de ejemplo", "Cliente", "Asistente", "quiero retomar mi repaso"} {
		if !strings.Contains(out, want) {
			t.Errorf("la página de cliente no contiene %q", want)
		}
	}
	// Nada de métricas, objetivos, evaluaciones ni datos de la sesión.
	for _, priv := range []string{
		rep.Goal, rep.Target, rep.Own, "Semilla", "Veredicto",
		"Nota media", "Evaluación", "Prueba de conversación",
	} {
		if priv != "" && strings.Contains(out, priv) {
			t.Errorf("la página de cliente no debe incluir %q", priv)
		}
	}
}

func TestIndiceEnlazaPaginaCliente(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "aibotia-test-caso")
	for _, f := range []string{base + ".html", base + "-cliente.html"} {
		if err := os.WriteFile(f, []byte("<html><head><title>Caso visible</title></head><body>hola</body></html>"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	index := renderReportsIndex(dir)
	if !strings.Contains(index, `href="aibotia-test-caso-cliente.html"`) {
		t.Error("el índice debe enlazar la página de cliente")
	}
	if !strings.Contains(index, `href="aibotia-test-caso.html">Técnico</a>`) {
		t.Error("el índice debe ofrecer el informe técnico")
	}
	if strings.Count(index, "Caso visible") != 1 {
		t.Error("la página de cliente no debe duplicar la entrada del índice")
	}
}
