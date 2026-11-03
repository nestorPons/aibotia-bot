package tester

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWriteReportMismoCasoSobrescribe(t *testing.T) {
	dir := t.TempDir()

	first := sampleReport()
	json1, md1, html1, client1, err := WriteReport(dir, first)
	if err != nil {
		t.Fatal(err)
	}
	wantBase := filepath.Join(dir, "aibotia-test-clienta_habitual")
	if json1 != wantBase+".json" || md1 != wantBase+".md" || html1 != wantBase+".html" || client1 != wantBase+"-cliente.html" {
		t.Fatalf("rutas inesperadas: %q %q %q %q", json1, md1, html1, client1)
	}

	second := sampleReport()
	second.Started = first.Started.Add(time.Hour)
	second.Finished = first.Finished.Add(time.Hour)
	second.CaseName = "Clienta habitual (segunda)"
	json2, md2, html2, client2, err := WriteReport(dir, second)
	if err != nil {
		t.Fatal(err)
	}
	if json2 != json1 || md2 != md1 || html2 != html1 || client2 != client1 {
		t.Fatalf("el mismo caso debe reutilizar rutas: %q %q %q %q", json2, md2, html2, client2)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 4 {
		t.Fatalf("archivos = %d, quiero 4 (JSON, MD, HTML técnico y HTML cliente)", len(entries))
	}
	raw, err := os.ReadFile(json2)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "Clienta habitual (segunda)") {
		t.Fatal("el reporte no se sobrescribió con la segunda ejecución")
	}
}

func TestReportSlug(t *testing.T) {
	cases := []struct {
		rep  Report
		want string
	}{
		{Report{Case: "ambiguedad_precios"}, "ambiguedad_precios"},
		{Report{Scenario: "clienta habitual"}, "clienta-habitual"},
		{Report{Case: "../../etc/passwd"}, "etc-passwd"},
	}
	for _, tc := range cases {
		if got := reportSlug(tc.rep); got != tc.want {
			t.Errorf("reportSlug(%+v) = %q, quiero %q", tc.rep, got, tc.want)
		}
	}
	fallback := Report{Started: time.Date(2026, 11, 3, 11, 10, 49, 0, time.UTC)}
	if got := reportSlug(fallback); got != "20261103-111049" {
		t.Errorf("fallback = %q", got)
	}
}

func TestReportErrorVerdict(t *testing.T) {
	rep := sampleReport()
	rep.Error = "cliente IA: modelo no disponible"
	rep.Summarize()
	if rep.Verdict != "error" {
		t.Fatalf("veredicto = %q, quiero error", rep.Verdict)
	}
}
