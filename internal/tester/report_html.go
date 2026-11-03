package tester

import (
	"fmt"
	"html"
	"strings"
	"time"
)

// reportCSS es el estilo embebido de los reportes HTML. El resultado es un
// archivo autocontenido: se puede enviar por correo/WhatsApp o imprimir a PDF
// sin depender de la aplicación.
const reportCSS = `
:root { color-scheme: light; }
* { box-sizing: border-box; }
body { margin:0; background:#f0f2f5; color:#111b21; font:15px/1.5 system-ui,-apple-system,Segoe UI,Roboto,sans-serif; }
.wrap { max-width:880px; margin:0 auto; padding:28px 16px 64px; }
.top { display:flex; align-items:flex-start; gap:16px; margin-bottom:18px; }
.top h1 { margin:0; font-size:20px; letter-spacing:-.01em; }
.top p { margin:4px 0 0; color:#667781; font-size:13px; }
.brand { margin-left:auto; font-size:12px; color:#667781; text-align:right; white-space:nowrap; }
.chips { display:flex; flex-wrap:wrap; gap:8px; margin:14px 0 18px; }
.chip { display:inline-flex; align-items:center; gap:6px; padding:5px 10px; border-radius:6px; background:#fff; border:1px solid #e1e6ea; color:#3b4a54; font-size:12px; }
.chip b { color:#111b21; font-weight:600; }
.chip.pass { background:#e7fce3; border-color:#b8e6b0; color:#0a6b3d; }
.chip.fail { background:#feecec; border-color:#f5c2c2; color:#a12222; }
.chip.warn { background:#fff6df; border-color:#f2dfa8; color:#7a5a00; }
.goal { background:#fff; border:1px solid #e1e6ea; border-radius:6px; padding:12px 14px; margin:0 0 18px; color:#3b4a54; font-size:13.5px; }
.goal b { color:#111b21; }
.chat { background:#efeae2; border:1px solid #d9d2c7; border-radius:6px; overflow:hidden; box-shadow:0 2px 10px rgba(11,20,26,.08); }
.chat-head { display:flex; align-items:center; gap:12px; padding:13px 16px; background:#008069; color:#fff; }
.chat-head .avatar { width:38px; height:38px; border-radius:50%; background:#fff; color:#008069; display:grid; place-items:center; font-size:20px; flex:none; }
.chat-head h2 { margin:0; font-size:15px; font-weight:600; }
.chat-head p { margin:2px 0 0; font-size:12px; opacity:.85; }
.chat-body { padding:18px 14px 22px; display:flex; flex-direction:column; gap:10px; background-image:radial-gradient(rgba(11,20,26,.045) 1px, transparent 1px); background-size:20px 20px; }
.row { display:flex; }
.row.in { justify-content:flex-start; }
.row.out { justify-content:flex-end; }
.bubble { max-width:78%; padding:8px 11px 6px; border-radius:6px; box-shadow:0 1px .5px rgba(11,20,26,.18); white-space:pre-wrap; word-wrap:break-word; }
.bubble.in { background:#fff; border-top-left-radius:0; }
.bubble.out { background:#d9fdd3; border-top-right-radius:0; }
.bubble .who { font-size:11px; font-weight:600; color:#008069; margin-bottom:3px; }
.bubble.out .who { color:#0a6b3d; }
.bubble .text { font-size:14px; }
.bubble .meta { margin-top:4px; text-align:right; font-size:10.5px; color:#667781; }
.bubble.muted { color:#667781; font-style:italic; }
.eval { max-width:78%; margin-top:-4px; font-size:11.5px; color:#667781; text-align:right; }
.eval.ok { color:#0a6b3d; }
.eval.bad { color:#a12222; }
.eval .warn { color:#8a6100; }
.setup { margin:0 0 18px; background:#fff; border:1px solid #e1e6ea; border-radius:6px; padding:10px 14px; }
.setup summary { cursor:pointer; font-weight:600; font-size:13px; color:#3b4a54; }
.setup .chat { margin-top:12px; box-shadow:none; }
.error { color:#a12222; font-size:12px; margin-top:4px; }
.index { background:#fff; border:1px solid #e1e6ea; border-radius:6px; padding:6px 0; margin:0 0 22px; }
.index a { display:flex; align-items:center; gap:10px; padding:9px 15px; color:#111b21; text-decoration:none; font-size:13.5px; border-bottom:1px solid #f0f2f5; }
.index a:last-child { border-bottom:0; }
.index a:hover { background:#f5f7f8; }
.index .num { color:#667781; min-width:22px; }
.index .spacer { flex:1; }
.case { margin-bottom:26px; }
.case-title { font-size:15px; margin:0 0 8px; }
.case-title .chip { margin-left:6px; vertical-align:middle; }
footer { margin-top:28px; text-align:center; color:#8696a0; font-size:11.5px; }
.print { position:fixed; right:18px; bottom:18px; background:#008069; color:#fff; border:0; border-radius:6px; padding:11px 16px; font:inherit; font-weight:600; cursor:pointer; box-shadow:0 6px 18px rgba(0,128,105,.3); }
.print:hover { background:#006e5b; }
@media print {
  body { background:#fff; }
  .wrap { max-width:none; padding:0; }
  .print, .brand { display:none; }
  .chat, .goal, .setup, .index { box-shadow:none; border-color:#ddd; }
  .chat-body { background:#fff; }
  .case { break-inside:avoid-page; }
}
`

func verdictClass(v string) string {
	switch v {
	case "pass":
		return "pass"
	case "fail", "error":
		return "fail"
	default:
		return "warn"
	}
}

func verdictLabel(v string) string {
	if v == "" {
		return "sin evaluar"
	}
	return v
}

// htmlDocument abre el documento HTML con el estilo embebido.
func htmlDocument(title string) string {
	return "<!doctype html>\n<html lang=\"es\">\n<head>\n<meta charset=\"utf-8\">\n" +
		"<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n" +
		"<title>" + html.EscapeString(title) + "</title>\n<style>" + reportCSS + "</style>\n" +
		"</head>\n<body>\n<div class=\"wrap\">\n"
}

func htmlClose() string {
	return "<footer>Registro generado automáticamente · aibotia</footer>\n</div>\n" +
		"<button class=\"print\" type=\"button\" onclick=\"window.print()\">Imprimir / guardar PDF</button>\n" +
		"</body>\n</html>\n"
}

func replyText(r Reply) string {
	text := strings.TrimSpace(r.Text)
	if text == "" && r.Media != "" {
		text = "[" + r.Media + "]"
	}
	if text == "" {
		text = "(mensaje vacío)"
	}
	return text
}

func replyTime(t Turn, r Reply) string {
	at := r.At
	if at.IsZero() {
		at = t.At
	}
	if at.IsZero() {
		return ""
	}
	return at.Format("15:04")
}

// htmlTurn pinta un intercambio pregunta/respuesta como burbujas.
func htmlTurn(t Turn) string {
	var b strings.Builder
	who := "Cliente"
	if t.Source == "ia" {
		who = "Cliente (simulado)"
	}
	// Mensaje del cliente.
	b.WriteString("<div class=\"row in\"><div class=\"bubble in\"><div class=\"who\">")
	b.WriteString(html.EscapeString(who))
	b.WriteString("</div><div class=\"text\">")
	b.WriteString(html.EscapeString(strings.TrimSpace(t.Question)))
	b.WriteString("</div>")
	if ts := htmlTime(t.At); ts != "" {
		b.WriteString("<div class=\"meta\">" + ts + "</div>")
	}
	b.WriteString("</div></div>")

	if t.Error != "" {
		b.WriteString("<div class=\"row in\"><div class=\"error\">No se pudo enviar: ")
		b.WriteString(html.EscapeString(t.Error))
		b.WriteString("</div></div>")
	}

	if len(t.Replies) == 0 {
		b.WriteString("<div class=\"row out\"><div class=\"bubble out muted\">(sin respuesta)</div></div>")
	}
	for _, r := range t.Replies {
		b.WriteString("<div class=\"row out\"><div class=\"bubble out\"><div class=\"who\">Asistente</div><div class=\"text\">")
		b.WriteString(html.EscapeString(replyText(r)))
		b.WriteString("</div>")
		if ts := replyTime(t, r); ts != "" {
			b.WriteString("<div class=\"meta\">" + ts + "</div>")
		}
		b.WriteString("</div></div>")
	}

	if ev := t.Evaluation; ev != nil {
		class := "ok"
		if ev.Score < 3 || ev.Hallucination || ev.RoleBreak || ev.Leak {
			class = "bad"
		}
		b.WriteString("<div class=\"row out\"><div class=\"eval " + class + "\">Nota " +
			fmt.Sprintf("%.1f/5", ev.Score) + " · " + html.EscapeString(ev.Verdict))
		if len(ev.Issues) > 0 {
			b.WriteString(` · <span class="warn">` + html.EscapeString(strings.Join(ev.Issues, "; ")) + "</span>")
		}
		var flags []string
		if ev.Hallucination {
			flags = append(flags, "posible invención")
		}
		if ev.RoleBreak {
			flags = append(flags, "ruptura de rol")
		}
		if ev.Leak {
			flags = append(flags, "fuga de prompt")
		}
		if len(flags) > 0 {
			b.WriteString(` · <span class="warn">⚠ ` + html.EscapeString(strings.Join(flags, ", ")) + "</span>")
		}
		b.WriteString("</div></div>")
	}
	return b.String()
}

func htmlTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("15:04")
}

// htmlCaseChat pinta la cabecera de la conversación y sus turnos.
func htmlCaseChat(rep Report) string {
	var b strings.Builder
	b.WriteString("<div class=\"chat\">\n<div class=\"chat-head\"><div class=\"avatar\">🤖</div><div><h2>")
	title := rep.CaseName
	if title == "" {
		title = rep.Scenario
	}
	b.WriteString(html.EscapeString(title))
	b.WriteString("</h2><p>Conversación de prueba")
	if !rep.Started.IsZero() {
		b.WriteString(" · " + rep.Started.Format("2006-01-02 15:04"))
	}
	b.WriteString("</p></div></div>\n<div class=\"chat-body\">\n")

	if len(rep.Setup) > 0 {
		b.WriteString("<div class=\"row in\"><div class=\"bubble in muted\">Contexto previo (no forma parte de la prueba)</div></div>")
		for _, t := range rep.Setup {
			b.WriteString(htmlTurn(t))
		}
	}

	for _, t := range rep.Turns {
		b.WriteString(htmlTurn(t))
	}
	b.WriteString("\n</div>\n</div>\n")
	return b.String()
}

// renderHTML genera el log de una sesión listo para enseñar o imprimir.
func renderHTML(rep Report) string {
	title := rep.CaseName
	if title == "" {
		title = rep.Scenario
	}
	var b strings.Builder
	b.WriteString(htmlDocument("Prueba de conversación · " + title))
	b.WriteString("<div class=\"top\"><div><h1>Prueba de conversación</h1><p>")
	b.WriteString(html.EscapeString(title))
	if !rep.Started.IsZero() {
		b.WriteString(" · " + rep.Started.Format("2006-01-02 15:04"))
	}
	b.WriteString("</p></div><div class=\"brand\">aibotia</div></div>\n")

	if rep.Goal != "" {
		b.WriteString("<p class=\"goal\"><b>Objetivo:</b> " + html.EscapeString(rep.Goal) + "</p>\n")
	}
	if rep.Error != "" {
		b.WriteString("<p class=\"goal\"><b>Prueba interrumpida:</b> " + html.EscapeString(rep.Error) + "</p>\n")
	}

	b.WriteString("<div class=\"chips\">")
	b.WriteString("<span class=\"chip\"><b>" + fmt.Sprintf("%d", rep.Summary.Questions) + "</b> turnos</span>")
	b.WriteString("<span class=\"chip\"><b>" + fmt.Sprintf("%d", rep.Summary.Replies) + "</b> respuestas</span>")
	if rep.Summary.NoReply > 0 {
		b.WriteString("<span class=\"chip warn\"><b>" + fmt.Sprintf("%d", rep.Summary.NoReply) + "</b> sin respuesta</span>")
	}
	if rep.Summary.Scored > 0 {
		b.WriteString("<span class=\"chip\"><b>Nota media</b> " + fmt.Sprintf("%.1f/5", rep.Summary.AvgScore) + "</span>")
	}
	b.WriteString("<span class=\"chip " + verdictClass(rep.Verdict) + "\"><b>Veredicto</b> " + html.EscapeString(verdictLabel(rep.Verdict)) + "</span>")
	b.WriteString("</div>\n")

	b.WriteString(htmlCaseChat(rep))
	b.WriteString(htmlClose())
	return b.String()
}

// clientCSS añade el marco de presentación a la hoja de estilos del reporte.
const clientCSS = `
body.client { background:#eef2f4; }
.client-wrap { max-width:760px; margin:0 auto; padding:36px 16px 64px; }
.client-cover { text-align:center; margin:0 0 22px; }
.client-cover .badge { display:inline-block; padding:5px 11px; border-radius:6px; background:#e7f5f2; color:#006e5b; font-size:12px; font-weight:600; }
.client-cover h1 { margin:12px 0 4px; font-size:23px; letter-spacing:-.02em; }
.client-cover p { margin:0; color:#667781; font-size:13px; }
.client-chat { max-width:620px; margin:0 auto; }
.client-chat .chat-head { background:#075e54; }
.client-chat .chat-head .avatar { color:#075e54; font-size:17px; }
.client-chat .bubble .who { color:#075e54; }
.client-chat .bubble.out .who { color:#0a6b3d; }
.client-chat .meta { color:#8696a0; }
.client-foot { max-width:620px; margin:16px auto 0; text-align:center; color:#8696a0; font-size:11.5px; }
@media print { body.client { background:#fff; } .client-cover { margin-bottom:14px; } }
`

// htmlClientDocument abre el HTML de presentación (sin datos internos).
func htmlClientDocument(title string) string {
	return "<!doctype html>\n<html lang=\"es\">\n<head>\n<meta charset=\"utf-8\">\n" +
		"<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n" +
		"<title>" + html.EscapeString(title) + "</title>\n<style>" + reportCSS + clientCSS + "</style>\n" +
		"</head>\n<body class=\"client\">\n<div class=\"client-wrap\">\n"
}

// htmlClientTurn pinta un turno sin evaluación ni datos técnicos.
func htmlClientTurn(t Turn) string {
	var b strings.Builder
	if q := strings.TrimSpace(t.Question); q != "" {
		b.WriteString("<div class=\"row in\"><div class=\"bubble in\"><div class=\"who\">Cliente</div><div class=\"text\">")
		b.WriteString(html.EscapeString(q))
		b.WriteString("</div>")
		if ts := htmlTime(t.At); ts != "" {
			b.WriteString("<div class=\"meta\">" + ts + "</div>")
		}
		b.WriteString("</div></div>")
	}
	for _, r := range t.Replies {
		if strings.TrimSpace(r.Text) == "" && r.Media == "" {
			continue
		}
		b.WriteString("<div class=\"row out\"><div class=\"bubble out\"><div class=\"who\">Asistente</div><div class=\"text\">")
		b.WriteString(html.EscapeString(replyText(r)))
		b.WriteString("</div>")
		if ts := replyTime(t, r); ts != "" {
			b.WriteString("<div class=\"meta\">" + ts + "</div>")
		}
		b.WriteString("</div></div>")
	}
	return b.String()
}

// htmlClientChat pinta el marco tipo WhatsApp con la conversación limpia.
func htmlClientChat(rep Report) string {
	var b strings.Builder
	b.WriteString("<div class=\"chat client-chat\">\n")
	b.WriteString("<div class=\"chat-head\"><div class=\"avatar\">🤖</div><div><h2>Asistente virtual</h2><p>Conversación de ejemplo</p></div></div>\n")
	b.WriteString("<div class=\"chat-body\">\n")
	for _, t := range rep.Setup {
		b.WriteString(htmlClientTurn(t))
	}
	for _, t := range rep.Turns {
		b.WriteString(htmlClientTurn(t))
	}
	b.WriteString("</div>\n</div>\n")
	return b.String()
}

// renderClientHTML genera una página limpia para enseñar o enviar a un cliente:
// misma conversación en burbujas, sin objetivo, métricas, notas ni errores.
func renderClientHTML(rep Report) string {
	var b strings.Builder
	b.WriteString(htmlClientDocument("Conversación de ejemplo · Asistente virtual"))
	b.WriteString("<header class=\"client-cover\"><span class=\"badge\">CONVERSACIÓN DE EJEMPLO</span>")
	b.WriteString("<h1>Atención por WhatsApp</h1>")
	if !rep.Started.IsZero() {
		b.WriteString("<p>" + rep.Started.Format("02/01/2006 · 15:04") + "</p>")
	} else {
		b.WriteString("<p>Asistente virtual</p>")
	}
	b.WriteString("</header>\n")
	b.WriteString(htmlClientChat(rep))
	b.WriteString("<p class=\"client-foot\">Conversación de demostración generada automáticamente.</p>\n")
	b.WriteString("</div>\n")
	b.WriteString("<button class=\"print\" type=\"button\" onclick=\"window.print()\">Imprimir / guardar PDF</button>\n")
	b.WriteString("</body>\n</html>\n")
	return b.String()
}

// renderSuiteHTML genera un único documento con todos los casos de la suite.
func renderSuiteHTML(s SuiteReport) string {
	var b strings.Builder
	b.WriteString(htmlDocument("Suite de pruebas · aibotia"))
	b.WriteString("<div class=\"top\"><div><h1>Suite de pruebas</h1><p>")
	b.WriteString(fmt.Sprintf("%d casos", len(s.Cases)))
	if !s.Started.IsZero() {
		b.WriteString(" · " + s.Started.Format("2006-01-02 15:04"))
	}
	b.WriteString("</p></div><div class=\"brand\">aibotia</div></div>\n")

	pass, fail := 0, 0
	for _, c := range s.Cases {
		switch c.Verdict {
		case "pass":
			pass++
		case "fail":
			fail++
		}
	}
	b.WriteString("<div class=\"chips\">")
	b.WriteString("<span class=\"chip\"><b>" + fmt.Sprintf("%d", len(s.Cases)) + "</b> casos</span>")
	b.WriteString("<span class=\"chip pass\"><b>" + fmt.Sprintf("%d", pass) + "</b> OK</span>")
	b.WriteString("<span class=\"chip fail\"><b>" + fmt.Sprintf("%d", fail) + "</b> con incidencias</span>")
	b.WriteString("</div>\n")

	b.WriteString("<nav class=\"index\">\n")
	for i, c := range s.Cases {
		name := c.CaseName
		if name == "" {
			name = c.Case
		}
		b.WriteString(fmt.Sprintf("<a href=\"#caso-%d\"><span class=\"num\">%d.</span><span>%s</span><span class=\"spacer\"></span><span class=\"chip %s\">%s</span></a>\n",
			i+1, i+1, html.EscapeString(name), verdictClass(c.Verdict), html.EscapeString(verdictLabel(c.Verdict))))
	}
	b.WriteString("</nav>\n")

	for i, c := range s.Cases {
		name := c.CaseName
		if name == "" {
			name = c.Case
		}
		b.WriteString(fmt.Sprintf("<section class=\"case\" id=\"caso-%d\">\n<h2 class=\"case-title\">%d. %s <span class=\"chip %s\">%s</span></h2>\n",
			i+1, i+1, html.EscapeString(name), verdictClass(c.Verdict), html.EscapeString(verdictLabel(c.Verdict))))
		b.WriteString(htmlCaseChat(c))
		b.WriteString("</section>\n")
	}
	b.WriteString(htmlClose())
	return b.String()
}
