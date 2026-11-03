package tester

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Reply es una respuesta (burbuja) del chatbot.
type Reply struct {
	At    time.Time `json:"at"`
	Text  string    `json:"text"`
	Media string    `json:"media,omitempty"`
}

// Turn es un intercambio pregunta/respuesta.
type Turn struct {
	At         time.Time   `json:"at"`
	Question   string      `json:"question"`
	Source     string      `json:"source,omitempty"` // "ia" | "guion" | "setup"
	Replies    []Reply     `json:"replies"`
	LatencyMs  int64       `json:"latency_ms"`
	Evaluation *Evaluation `json:"evaluation,omitempty"`
	Error      string      `json:"error,omitempty"`
}

// Summary agrega las métricas de la sesión.
type Summary struct {
	Questions      int     `json:"questions"`
	Replies        int     `json:"replies"`
	NoReply        int     `json:"no_reply"`
	AvgLatency     int64   `json:"avg_latency_ms"`
	AvgScore       float64 `json:"avg_score"`
	Scored         int     `json:"scored"`
	Hallucinations int     `json:"hallucinations"`
	RoleBreaks     int     `json:"role_breaks"`
	Leaks          int     `json:"leaks"`
}

// Report es el resultado completo de una sesión de pruebas.
type Report struct {
	Started  time.Time `json:"started"`
	Finished time.Time `json:"finished"`
	Target   string    `json:"target"`
	Own      string    `json:"own"`
	Scenario string    `json:"scenario"`
	Case     string    `json:"case"`
	CaseName string    `json:"case_name"`
	Goal     string    `json:"goal"`
	Seed     int64     `json:"seed"`
	Mode     string    `json:"mode"`
	Setup    []Turn    `json:"setup,omitempty"`
	Repeated int       `json:"repeated"`
	Turns    []Turn    `json:"turns"`
	Summary  Summary   `json:"summary"`
	Verdict  string    `json:"verdict"`
	Error    string    `json:"error,omitempty"`
}

// Summarize calcula las métricas del reporte.
func (r *Report) Summarize() {
	var totalLatency int64
	var latencyCount int
	var totalScore float64
	for _, t := range r.Turns {
		r.Summary.Questions++
		r.Summary.Replies += len(t.Replies)
		if len(t.Replies) == 0 {
			r.Summary.NoReply++
		} else {
			totalLatency += t.LatencyMs
			latencyCount++
		}
		if t.Evaluation != nil {
			totalScore += t.Evaluation.Score
			r.Summary.Scored++
			if t.Evaluation.Hallucination {
				r.Summary.Hallucinations++
			}
			if t.Evaluation.RoleBreak {
				r.Summary.RoleBreaks++
			}
			if t.Evaluation.Leak {
				r.Summary.Leaks++
			}
		}
	}
	if latencyCount > 0 {
		r.Summary.AvgLatency = totalLatency / int64(latencyCount)
	}
	if r.Summary.Scored > 0 {
		r.Summary.AvgScore = totalScore / float64(r.Summary.Scored)
	}
	r.Verdict = r.computeVerdict()
}

// computeVerdict resume el resultado del caso.
func (r *Report) computeVerdict() string {
	if r.Error != "" {
		return "error"
	}
	if r.Summary.Scored == 0 {
		return "sin evaluar"
	}
	if r.Summary.NoReply > 0 || r.Summary.Hallucinations > 0 ||
		r.Summary.RoleBreaks > 0 || r.Summary.Leaks > 0 || r.Summary.AvgScore < 3 {
		return "fail"
	}
	return "pass"
}

// WriteReport guarda el reporte en JSON, Markdown y dos HTML dentro de dir:
// el informe técnico (chat + métricas/evaluaciones) y una versión "cliente"
// limpia para enseñar o enviar. El nombre es estable por caso: repetir el mismo
// caso sobrescribe los archivos anteriores en lugar de acumular copias.
func WriteReport(dir string, rep Report) (jsonPath, mdPath, htmlPath, clientPath string, err error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", "", "", fmt.Errorf("tester: crear directorio de reportes: %w", err)
	}
	base := filepath.Join(dir, "aibotia-test-"+reportSlug(rep))
	jsonPath, mdPath, htmlPath = base+".json", base+".md", base+".html"
	clientPath = base + "-cliente.html"

	raw, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return "", "", "", "", fmt.Errorf("tester: serializar reporte: %w", err)
	}
	if err := os.WriteFile(jsonPath, raw, 0o644); err != nil {
		return "", "", "", "", fmt.Errorf("tester: escribir JSON: %w", err)
	}
	if err := os.WriteFile(mdPath, []byte(renderMarkdown(rep)), 0o644); err != nil {
		return "", "", "", "", fmt.Errorf("tester: escribir Markdown: %w", err)
	}
	if err := os.WriteFile(htmlPath, []byte(renderHTML(rep)), 0o644); err != nil {
		return "", "", "", "", fmt.Errorf("tester: escribir HTML: %w", err)
	}
	if err := os.WriteFile(clientPath, []byte(renderClientHTML(rep)), 0o644); err != nil {
		return "", "", "", "", fmt.Errorf("tester: escribir HTML de cliente: %w", err)
	}
	return jsonPath, mdPath, htmlPath, clientPath, nil
}

// reportSlug devuelve el nombre estable del reporte: el id del caso (o el
// escenario) para que repetirlo sobrescriba. Si no hay ninguno, usa la hora.
func reportSlug(rep Report) string {
	name := strings.TrimSpace(rep.Case)
	if name == "" {
		name = strings.TrimSpace(rep.Scenario)
	}
	if name == "" {
		name = rep.Started.Format("20060102-150405")
	}
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		return "caso"
	}
	return slug
}

// sourceTag etiqueta en Markdown los mensajes redactados por el cliente IA.
func sourceTag(source string) string {
	if source == "ia" {
		return " (IA)"
	}
	return ""
}

func renderMarkdown(rep Report) string {
	var b strings.Builder
	title := rep.CaseName
	if title == "" {
		title = rep.Scenario
	}
	fmt.Fprintf(&b, "# Prueba de conversación — %s\n\n", title)
	fmt.Fprintf(&b, "| Campo | Valor |\n|---|---|\n")
	fmt.Fprintf(&b, "| Inicio | %s |\n", rep.Started.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(&b, "| Destino | %s |\n", rep.Target)
	fmt.Fprintf(&b, "| Tester | %s |\n", rep.Own)
	fmt.Fprintf(&b, "| Caso | %s (%s) |\n", rep.CaseName, rep.Case)
	if rep.Goal != "" {
		fmt.Fprintf(&b, "| Objetivo | %s |\n", rep.Goal)
	}
	fmt.Fprintf(&b, "| Semilla | %d |\n", rep.Seed)
	fmt.Fprintf(&b, "| Modo | %s |\n", rep.Mode)
	fmt.Fprintf(&b, "| Preguntas | %d |\n", rep.Summary.Questions)
	fmt.Fprintf(&b, "| Respuestas | %d |\n", rep.Summary.Replies)
	fmt.Fprintf(&b, "| Sin respuesta | %d |\n", rep.Summary.NoReply)
	fmt.Fprintf(&b, "| Preguntas repetidas | %d |\n", rep.Repeated)
	fmt.Fprintf(&b, "| Latencia media | %s |\n", time.Duration(rep.Summary.AvgLatency)*time.Millisecond)
	if rep.Summary.Scored > 0 {
		fmt.Fprintf(&b, "| Nota media | %.1f/5 |\n", rep.Summary.AvgScore)
	}
	fmt.Fprintf(&b, "| Invenciones | %d |\n", rep.Summary.Hallucinations)
	fmt.Fprintf(&b, "| Ruptura de rol | %d |\n", rep.Summary.RoleBreaks)
	fmt.Fprintf(&b, "| Fugas de prompt | %d |\n", rep.Summary.Leaks)
	fmt.Fprintf(&b, "| Veredicto | **%s** |\n", rep.Verdict)

	if rep.Error != "" {
		fmt.Fprintf(&b, "\n> ⚠️ **Prueba interrumpida:** %s\n", rep.Error)
	}

	if len(rep.Setup) > 0 {
		b.WriteString("\n## Preparación (contexto previo)\n")
		for i, t := range rep.Setup {
			fmt.Fprintf(&b, "\n### Setup %d. Cliente\n\n%s\n", i+1, t.Question)
			if len(t.Replies) == 0 {
				b.WriteString("\n- _(sin respuesta)_\n")
			}
			for _, r := range t.Replies {
				text := r.Text
				if text == "" && r.Media != "" {
					text = "[" + r.Media + "]"
				}
				b.WriteString("\n- " + strings.ReplaceAll(text, "\n", "\n  ") + "\n")
			}
		}
	}

	b.WriteString("\n## Turnos\n")

	for i, t := range rep.Turns {
		fmt.Fprintf(&b, "\n### %d. Cliente%s\n\n%s\n", i+1, sourceTag(t.Source), t.Question)
		if t.Error != "" {
			fmt.Fprintf(&b, "\n> Error: %s\n", t.Error)
		}
		fmt.Fprintf(&b, "\n**Bot** (%s):\n\n", time.Duration(t.LatencyMs)*time.Millisecond)
		if len(t.Replies) == 0 {
			b.WriteString("- _(sin respuesta)_\n")
		}
		for _, r := range t.Replies {
			text := r.Text
			if text == "" && r.Media != "" {
				text = "[" + r.Media + "]"
			}
			b.WriteString("- " + strings.ReplaceAll(text, "\n", "\n  ") + "\n")
		}
		if t.Evaluation != nil {
			fmt.Fprintf(&b, "\n**Evaluación:** %.1f/5 — %s\n", t.Evaluation.Score, t.Evaluation.Verdict)
			if len(t.Evaluation.Issues) > 0 {
				b.WriteString("\nProblemas: " + strings.Join(t.Evaluation.Issues, "; ") + "\n")
			}
			if t.Evaluation.Hallucination {
				b.WriteString("\n⚠️ Posible invención de datos.\n")
			}
			if t.Evaluation.RoleBreak {
				b.WriteString("\n🚨 Posible ruptura de rol.\n")
			}
			if t.Evaluation.Leak {
				b.WriteString("\n🚨 Posible fuga de prompt.\n")
			}
		}
	}
	return b.String()
}

// SuiteReport agrupa varios casos ejecutados en una misma sesión.
type SuiteReport struct {
	Started  time.Time `json:"started"`
	Finished time.Time `json:"finished"`
	Seed     int64     `json:"seed"`
	Cases    []Report  `json:"cases"`
}

// WriteSuiteReport guarda el resumen comparativo de la suite.
func WriteSuiteReport(dir string, s SuiteReport) (jsonPath, mdPath, htmlPath string, err error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", "", fmt.Errorf("tester: crear directorio de reportes: %w", err)
	}
	stamp := s.Started.Format("20060102-150405")
	base := filepath.Join(dir, "aibotia-suite-"+stamp)
	jsonPath, mdPath, htmlPath = base+".json", base+".md", base+".html"

	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return "", "", "", fmt.Errorf("tester: serializar suite: %w", err)
	}
	if err := os.WriteFile(jsonPath, raw, 0o644); err != nil {
		return "", "", "", fmt.Errorf("tester: escribir JSON: %w", err)
	}
	if err := os.WriteFile(mdPath, []byte(renderSuiteMarkdown(s)), 0o644); err != nil {
		return "", "", "", fmt.Errorf("tester: escribir Markdown: %w", err)
	}
	if err := os.WriteFile(htmlPath, []byte(renderSuiteHTML(s)), 0o644); err != nil {
		return "", "", "", fmt.Errorf("tester: escribir HTML: %w", err)
	}
	return jsonPath, mdPath, htmlPath, nil
}

func renderSuiteMarkdown(s SuiteReport) string {
	var b strings.Builder
	pass, fail := 0, 0
	for _, c := range s.Cases {
		switch c.Verdict {
		case "pass":
			pass++
		case "fail":
			fail++
		}
	}
	fmt.Fprintf(&b, "# Suite de pruebas — %d casos\n\n", len(s.Cases))
	fmt.Fprintf(&b, "| Campo | Valor |\n|---|---|\n")
	fmt.Fprintf(&b, "| Inicio | %s |\n", s.Started.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(&b, "| Fin | %s |\n", s.Finished.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(&b, "| Semilla | %d |\n", s.Seed)
	fmt.Fprintf(&b, "| Pass / Fail | %d / %d |\n", pass, fail)

	b.WriteString("\n## Casos\n\n")
	b.WriteString("| Caso | Veredicto | Nota | Sin respuesta | Invenciones | Rol | Fugas | Latencia |\n")
	b.WriteString("|---|---|---|---|---|---|---|---|\n")
	for _, c := range s.Cases {
		score := "—"
		if c.Summary.Scored > 0 {
			score = fmt.Sprintf("%.1f", c.Summary.AvgScore)
		}
		fmt.Fprintf(&b, "| %s (`%s`) | **%s** | %s | %d | %d | %d | %d | %s |\n",
			c.CaseName, c.Case, c.Verdict, score, c.Summary.NoReply,
			c.Summary.Hallucinations, c.Summary.RoleBreaks, c.Summary.Leaks,
			time.Duration(c.Summary.AvgLatency)*time.Millisecond)
	}
	b.WriteString("\n## Detalle por caso\n")
	for _, c := range s.Cases {
		fmt.Fprintf(&b, "\n### %s\n\n", c.CaseName)
		for i, t := range c.Turns {
			fmt.Fprintf(&b, "\n**%d. Cliente%s:** %s\n", i+1, sourceTag(t.Source), t.Question)
			if len(t.Replies) == 0 {
				b.WriteString("- _(sin respuesta)_\n")
			}
			for _, r := range t.Replies {
				text := r.Text
				if text == "" && r.Media != "" {
					text = "[" + r.Media + "]"
				}
				b.WriteString("- " + strings.ReplaceAll(text, "\n", "\n  ") + "\n")
			}
			if t.Evaluation != nil {
				fmt.Fprintf(&b, "  _Evaluación: %.1f/5 — %s_\n", t.Evaluation.Score, t.Evaluation.Verdict)
			}
		}
	}
	return b.String()
}
