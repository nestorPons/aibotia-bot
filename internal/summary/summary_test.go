package summary

import (
	"strings"
	"testing"
	"time"
)

func TestFallback(t *testing.T) {
	res := fallback(Input{
		Topic: "prompt_laser_diodo",
		Appointment: &Appointment{
			Service:      "las_z_suel",
			ServiceName:  "ZONAS SUELTAS",
			Availability: "martes por la tarde",
		},
		Messages: []Message{
			{Role: "Cliente", Text: "hola", At: time.Now()},
			{Role: "Bot", Text: "¿en qué te ayudo?", At: time.Now()},
			{Role: "Cliente", Text: "quiero láser en los brazos", At: time.Now()},
		},
	})
	if res.Source != "fallback" {
		t.Fatalf("source = %q", res.Source)
	}
	got := strings.Join(res.Points, " | ")
	for _, want := range []string{"laser diodo", "ZONAS SUELTAS", "martes por la tarde", "quiero láser en los brazos"} {
		if !strings.Contains(got, want) {
			t.Fatalf("falta %q en %q", want, got)
		}
	}
}

func TestParsePoints(t *testing.T) {
	raw := "1. Quiere depilación láser.\n- Prefiere martes por la tarde.\n* Pregunta precio.\n\n• Alergia a la lidocaína.\nSobra esta línea."
	got := parsePoints(raw)
	if len(got) != 4 {
		t.Fatalf("points = %d, quiero 4: %v", len(got), got)
	}
	if got[0] != "Quiere depilación láser." || got[2] != "Pregunta precio." {
		t.Fatalf("limpieza inesperada: %v", got)
	}
}

func TestPrettyTopic(t *testing.T) {
	if got := prettyTopic("prompt_laser_diodo"); got != "laser diodo" {
		t.Fatalf("prettyTopic = %q", got)
	}
	if got := prettyTopic(""); got != "" {
		t.Fatalf("prettyTopic vacío = %q", got)
	}
}

func TestSummarizeWithoutClient(t *testing.T) {
	s := New("", "model", nil)
	res := s.Summarize(t.Context(), "k", Input{Messages: []Message{{Role: "Cliente", Text: "hola"}}})
	if res.Source != "fallback" || len(res.Points) == 0 {
		t.Fatalf("resultado inesperado: %+v", res)
	}
}
