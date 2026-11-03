package tester

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// Options configura una sesión de pruebas.
type Options struct {
	Target     string        // número del chatbot (E.164 sin + o con +)
	Mode       string        // "ai" | "scenario"
	CaseID     string        // caso concreto (vacío = aleatorio)
	CasesDir   string        // directorio de casos JSON
	Suite      []string      // ids de casos a ejecutar en secuencia ("all" se resuelve antes)
	Seed       int64         // semilla (0 = time.Now)
	Fresh      bool          // limpiar historial y reactivar la IA en el bot antes de cada caso
	Turns      int           // 0 = turnos automáticos del caso
	Delay      time.Duration // espera entre preguntas (anti-spam)
	Idle       time.Duration // silencio tras la última respuesta
	Timeout    time.Duration // espera máxima por turno
	Eval       bool          // puntuar respuestas con IA
	ReportDir  string        // directorio de reportes
	DSN        string        // PostgreSQL de la sesión del tester
	APIKey     string        // API key de OpenAI (generación/evaluación)
	BotAPIBase string        // URL base de la API del bot (reset de sesión)
	BotAPIKey  string        // API key del bot
	Model      string        // modelo de OpenAI
}

// Runner ejecuta la conversación de prueba.
type Runner struct {
	opts Options
	log  *slog.Logger
	ai   *AI

	// testModeReady evita repetir las llamadas al activar el modo prueba.
	testModeReady bool
}

// NewRunner crea un runner. Si opts.APIKey está vacío, la IA queda deshabilitada
// y solo se podrá ejecutar el modo "scenario" (banco fijo, sin reacción al bot).
func NewRunner(opts Options, log *slog.Logger) *Runner {
	if opts.Mode == "" {
		opts.Mode = "ai"
	}
	opts.Mode = strings.ToLower(strings.TrimSpace(opts.Mode))
	if opts.CasesDir == "" {
		opts.CasesDir = "data/test/cases"
	}
	if opts.Turns < 0 {
		opts.Turns = 0
	}
	if opts.Delay <= 0 {
		opts.Delay = 15 * time.Second
	}
	if opts.Idle <= 0 {
		opts.Idle = 10 * time.Second
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 2 * time.Minute
	}
	if opts.ReportDir == "" {
		opts.ReportDir = "data/test/reports"
	}
	if opts.BotAPIBase == "" {
		opts.BotAPIBase = "http://localhost:3000"
	}
	return &Runner{opts: opts, log: log, ai: NewAI(opts.APIKey, opts.Model, log)}
}

// Run vincula WhatsApp (si hace falta), conversa y escribe los reportes.
func (r *Runner) Run(ctx context.Context) error {
	if strings.TrimSpace(r.opts.Target) == "" {
		return errors.New("tester: falta el número del chatbot (-to 34600111222 o TEST_TARGET)")
	}
	switch r.opts.Mode {
	case "ai":
		if r.ai == nil {
			return errors.New("tester: el modo ai (conversación entre dos LLM) requiere OPENAI_API_KEY; " +
				"usa -mode scenario solo si quieres el banco fijo sin reacción al bot")
		}
	case "scenario":
		r.log.Warn("modo scenario: el cliente NO reacciona a las respuestas del bot (usa -mode ai)")
	default:
		return fmt.Errorf("tester: modo %q no válido (ai | scenario)", r.opts.Mode)
	}
	cases, err := LoadCases(r.opts.CasesDir)
	if err != nil {
		return err
	}

	seed := r.opts.Seed
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	rng := rand.New(rand.NewSource(seed))

	cli, err := Open(ctx, r.opts.DSN, r.log)
	if err != nil {
		return err
	}
	defer cli.Close()

	printHeader(r.opts, cases, seed)
	if err := cli.Login(ctx); err != nil {
		return err
	}
	if own := cli.OwnNumber(); own != "" {
		fmt.Printf("📱 Tester vinculado como +%s\n", own)
	}
	// El modo prueba evita que el bot desactive la IA al derivar a un operador
	// y añade una nota explicando el comportamiento real.
	r.ensureTestMode(ctx, cli)
	defer r.disableTestMode(cli)
	jid, err := cli.Resolve(ctx, r.opts.Target)
	if err != nil {
		return err
	}
	fmt.Printf("🎯 Chatbot destino: %s\n", jid.String())

	if len(r.opts.Suite) > 0 {
		return r.runSuite(ctx, cli, jid, cases, rng, seed)
	}

	c, err := r.pickOne(cases, rng)
	if err != nil {
		return err
	}
	rep := r.runCase(ctx, cli, jid, c, rng, seed)
	SaveLastCase(r.opts.CasesDir, c.ID)

	jsonPath, mdPath, htmlPath, clientPath, err := WriteReport(r.opts.ReportDir, rep)
	if err != nil {
		return err
	}
	fmt.Printf("\n📄 Reporte JSON: %s\n📄 Reporte Markdown: %s\n🌐 Log HTML (para compartir): %s\n🌐 Página para cliente: %s\n",
		jsonPath, mdPath, htmlPath, clientPath)
	return nil
}

func printHeader(opts Options, cases []Case, seed int64) {
	fmt.Println("════════════════════════════════════════════════════════════")
	fmt.Println(" Prueba de conversación con el chatbot")
	fmt.Printf(" Destino   : %s\n", opts.Target)
	fmt.Printf(" Modo      : %s", opts.Mode)
	switch opts.Mode {
	case "ai":
		fmt.Print(" · cliente IA ↔ bot real (reacciona a cada respuesta)")
	case "scenario":
		fmt.Print(" · banco fijo (NO reacciona al bot)")
	}
	if opts.Turns > 0 {
		fmt.Printf(" (%d turnos)", opts.Turns)
	} else {
		fmt.Print(" (turnos por caso)")
	}
	if len(opts.Suite) > 0 {
		fmt.Printf(" · suite: %s", strings.Join(opts.Suite, ", "))
	} else if opts.CaseID != "" {
		fmt.Printf(" · caso: %s", opts.CaseID)
	} else {
		fmt.Printf(" · casos disponibles: %d (aleatorio)", len(cases))
	}
	fmt.Println()
	fmt.Printf(" Semilla   : %d\n", seed)
	fmt.Printf(" Pausa     : %s entre preguntas (anti-spam del bot)\n", opts.Delay)
	if !opts.Fresh {
		fmt.Println(" Reset     : desactivado (se conserva el historial del bot)")
	} else if opts.BotAPIKey == "" {
		fmt.Println(" Reset     : ⚠️ sin API key del bot; no se limpiará el historial")
	} else {
		fmt.Printf(" Reset     : historial limpio vía %s\n", opts.BotAPIBase)
	}
	fmt.Println("════════════════════════════════════════════════════════════")
}

func (r *Runner) pickOne(cases []Case, rng *rand.Rand) (Case, error) {
	if r.opts.CaseID != "" {
		return FindCase(cases, r.opts.CaseID)
	}
	prev := LastCase(r.opts.CasesDir)
	if prev != "" {
		fmt.Printf("🔁 Evitando el último caso (%s)\n", prev)
	}
	return PickCase(cases, rng, prev), nil
}

func (r *Runner) runSuite(ctx context.Context, cli *Client, jid types.JID, cases []Case, rng *rand.Rand, seed int64) error {
	list := cases
	if !(len(r.opts.Suite) == 1 && strings.EqualFold(r.opts.Suite[0], "all")) {
		list = make([]Case, 0, len(r.opts.Suite))
		for _, id := range r.opts.Suite {
			c, err := FindCase(cases, id)
			if err != nil {
				return err
			}
			list = append(list, c)
		}
	}

	suite := SuiteReport{Started: time.Now(), Seed: seed}
	for i, c := range list {
		if ctx.Err() != nil {
			break
		}
		fmt.Printf("\n▶️  Caso %d/%d: %s (%s)\n", i+1, len(list), c.Name, c.ID)
		rep := r.runCase(ctx, cli, jid, c, rng, seed)
		suite.Cases = append(suite.Cases, rep)
		if jsonPath, mdPath, htmlPath, clientPath, err := WriteReport(r.opts.ReportDir, rep); err != nil {
			r.log.Warn("no se pudo escribir el reporte del caso", "case", c.ID, "error", err.Error())
		} else {
			fmt.Printf("📄 %s · %s · %s\n🌐 cliente: %s\n", jsonPath, mdPath, htmlPath, clientPath)
		}
		if i < len(list)-1 && ctx.Err() == nil {
			sleep(ctx, r.opts.Delay)
		}
	}
	suite.Finished = time.Now()
	if len(suite.Cases) == 0 {
		return errors.New("tester: la suite no ejecutó ningún caso")
	}

	jsonPath, mdPath, htmlPath, err := WriteSuiteReport(r.opts.ReportDir, suite)
	if err != nil {
		return err
	}
	printSuiteSummary(suite)
	fmt.Printf("\n📄 Resumen suite JSON: %s\n📄 Resumen suite Markdown: %s\n🌐 Log suite HTML (para compartir): %s\n", jsonPath, mdPath, htmlPath)
	return nil
}

func (r *Runner) runCase(ctx context.Context, cli *Client, jid types.JID, c Case, rng *rand.Rand, seed int64) Report {
	rep := Report{
		Started:  time.Now(),
		Target:   jid.String(),
		Own:      cli.OwnNumber(),
		Scenario: c.ID,
		Case:     c.ID,
		CaseName: c.Name,
		Goal:     c.Goal,
		Seed:     seed,
		Mode:     r.opts.Mode,
	}

	if r.opts.Fresh {
		r.resetSession(ctx, cli)
	}

	// Mensajes de preparación (contexto previo del caso, p. ej. clienta habitual).
	for _, q := range c.Setup {
		if ctx.Err() != nil {
			break
		}
		turn := r.exchange(ctx, cli, jid, q, 0, 0)
		turn.Source = "setup"
		rep.Setup = append(rep.Setup, turn)
		sleep(ctx, r.opts.Delay)
	}
	// Si el usuario del tester se acaba de crear con el primer mensaje, activa
	// el modo prueba ahora (también reactiva la IA si el bot ya lo desactivó).
	r.ensureTestMode(ctx, cli)

	history := append([]Turn(nil), rep.Setup...)
	used := make([]string, 0, len(c.Setup)+len(c.Questions))
	used = append(used, c.Setup...)

	limit := c.TurnCount(rng, r.opts.Turns)
	if r.opts.Mode != "ai" {
		limit = len(c.Questions)
	}
	bank := c.ShuffledQuestions(rng)

	for i := 0; i < limit; i++ {
		if ctx.Err() != nil {
			fmt.Println("\n⏹  Prueba interrumpida")
			break
		}

		question := ""
		source := "guion"
		if r.opts.Mode == "ai" {
			if i == 0 {
				question = c.Opener(rng)
			} else {
				q, err := r.aiNext(ctx, c, history, used, clarifyNote(history))
				if err != nil {
					// Sin cliente IA no hay conversación real: es mejor detener
					// el caso que degradar a preguntas sueltas del banco.
					r.log.Error("el cliente IA no pudo continuar; se detiene el caso", "error", err.Error())
					rep.Error = "cliente IA: " + err.Error()
					break
				}
				question = q
				source = "ia"
			}
		} else if i < len(bank) {
			question = bank[i]
		}

		if strings.TrimSpace(question) == "" {
			question = nextUnused(&bank, used)
		}
		if strings.TrimSpace(question) == "" {
			break
		}
		if isDuplicate(question, used) {
			rep.Repeated++
			if alt := nextUnused(&bank, used); alt != "" {
				question = alt
				source = "guion"
			}
		}
		used = append(used, question)

		turn := r.exchange(ctx, cli, jid, question, i+1, limit)
		turn.Source = source
		r.ensureTestMode(ctx, cli)
		if r.ai != nil && r.opts.Eval && len(turn.Replies) > 0 {
			ev, err := r.ai.Evaluate(ctx, c, history, question, turn.Replies)
			if err != nil {
				r.log.Warn("no se pudo evaluar la respuesta", "error", err.Error())
			} else {
				turn.Evaluation = &ev
				printEvaluation(ev)
			}
		}
		rep.Turns = append(rep.Turns, turn)
		history = append(history, turn)

		if i < limit-1 && ctx.Err() == nil {
			sleep(ctx, r.opts.Delay)
		}
	}

	rep.Finished = time.Now()
	rep.Summarize()
	printSummary(rep)
	return rep
}

// aiNext pide el siguiente mensaje teniendo en cuenta la conversación (incluidas
// las respuestas del bot) y evita repetir mensajes ya enviados. note añade una
// instrucción puntual (p. ej. responder a una aclaración del bot).
func (r *Runner) aiNext(ctx context.Context, c Case, history []Turn, used []string, note string) (string, error) {
	q, err := r.ai.Next(ctx, c, history, used, note)
	if err != nil {
		return "", err
	}
	if !isDuplicate(q, used) {
		return q, nil
	}

	retry := "El mensaje que ibas a enviar repite algo ya dicho. Reacciona a la última respuesta del bot y avanza con algo distinto."
	if strings.TrimSpace(note) != "" {
		retry = strings.TrimSpace(note) + " " + retry
	}
	q, err = r.ai.Next(ctx, c, history, used, retry)
	if err != nil {
		return "", err
	}
	if !isDuplicate(q, used) {
		return q, nil
	}
	return "", errors.New("la IA repitió una pregunta")
}

func isDuplicate(q string, used []string) bool {
	norm := func(s string) string {
		s = strings.ToLower(strings.TrimSpace(s))
		s = strings.Trim(s, "¿?¡!.,;:\"'()[]{}")
		s = accentFolder.Replace(s)
		return strings.Join(strings.Fields(s), " ")
	}
	qn := norm(q)
	if qn == "" {
		return true
	}
	for _, u := range used {
		if norm(u) == qn {
			return true
		}
	}
	return false
}

var accentFolder = strings.NewReplacer(
	"á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u", "ñ", "n",
	"à", "a", "è", "e", "ì", "i", "ò", "o", "ù", "u",
)

func nextUnused(bank *[]string, used []string) string {
	for len(*bank) > 0 {
		q := (*bank)[0]
		*bank = (*bank)[1:]
		if !isDuplicate(q, used) {
			return q
		}
	}
	return ""
}

// --- Reset de sesión contra la API del bot ---------------------------------

var errBotNotFound = errors.New("tester: recurso no encontrado en el bot")

// resetSession limpia el historial del tester y reactiva la IA para el usuario.
// Sin esto el bot arrastra la conversación anterior y las respuestas se repiten.
func (r *Runner) resetSession(ctx context.Context, cli *Client) {
	if r.opts.BotAPIKey == "" {
		return
	}
	ids := r.userIDs(ctx, cli)
	if len(ids) == 0 {
		r.log.Warn("no se localizó el usuario del tester en el bot; no se limpia el historial")
		return
	}
	for _, id := range ids {
		if err := r.botRequest(ctx, http.MethodDelete, "/api/v1/users/"+url.PathEscape(id)+"/history", nil, nil); err == nil {
			r.log.Info("historial del tester borrado", "user", id)
		}
		body := map[string]any{"active": true, "topic": ""}
		if err := r.botRequest(ctx, http.MethodPut, "/api/v1/users/"+url.PathEscape(id), body, nil); err == nil {
			r.log.Info("IA reactivada para el tester", "user", id)
		}
	}
}

// setTestMode activa o desactiva el modo prueba del usuario del tester. Al
// activarlo, el bot también reactiva la IA. Devuelve true si actualizó alguna
// variante del usuario (LID/PN).
func (r *Runner) setTestMode(ctx context.Context, cli *Client, enabled bool) bool {
	if r.opts.BotAPIKey == "" {
		return false
	}
	applied := false
	for _, id := range r.userIDs(ctx, cli) {
		body := map[string]any{"enabled": enabled}
		if err := r.botRequest(ctx, http.MethodPost, "/api/v1/users/"+url.PathEscape(id)+"/test-mode", body, nil); err != nil {
			if errors.Is(err, errBotNotFound) {
				continue
			}
			r.log.Warn("no se pudo actualizar el modo prueba", "user", id, "enabled", enabled, "error", err.Error())
			continue
		}
		applied = true
	}
	return applied
}

// ensureTestMode activa el modo prueba una sola vez, en cuanto el usuario del
// tester existe en la BD del bot.
func (r *Runner) ensureTestMode(ctx context.Context, cli *Client) {
	if r.testModeReady {
		return
	}
	if r.setTestMode(ctx, cli, true) {
		r.testModeReady = true
		r.log.Info("modo prueba activado: la IA no se desactivará al derivar")
	}
}

// disableTestMode deshace el modo prueba al terminar (aunque el contexto de la
// prueba se haya cancelado).
func (r *Runner) disableTestMode(cli *Client) {
	if !r.testModeReady {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if r.setTestMode(ctx, cli, false) {
		r.log.Info("modo prueba desactivado")
	}
}

// userIDs devuelve los ids de usuario del bot que corresponden al tester.
func (r *Runner) userIDs(ctx context.Context, cli *Client) []string {
	seen := map[string]bool{}
	out := make([]string, 0, 4)
	add := func(id string) {
		if id == "" || !strings.Contains(id, "@") || seen[id] {
			return
		}
		seen[id] = true
		out = append(out, id)
	}

	terms := make([]string, 0, 6)
	addTerm := func(t string) {
		t = strings.TrimSpace(t)
		if t == "" {
			return
		}
		for _, existing := range terms {
			if existing == t {
				return
			}
		}
		terms = append(terms, t)
	}

	for _, j := range cli.OwnJIDs() {
		add(j.String())
		addTerm(j.User)
	}
	if n := cli.OwnNumber(); n != "" {
		add(n + "@s.whatsapp.net")
		addTerm(n)
	}

	for _, term := range terms {
		var res struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		path := "/api/v1/users?perPage=50&search=" + url.QueryEscape(term)
		if err := r.botRequest(ctx, http.MethodGet, path, nil, &res); err != nil {
			continue
		}
		for _, u := range res.Data {
			add(u.ID)
		}
	}
	return out
}

func (r *Runner) botRequest(ctx context.Context, method, path string, body any, out any) error {
	if r.opts.BotAPIKey == "" {
		return errors.New("tester: falta la API key del bot")
	}
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(r.opts.BotAPIBase, "/")+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("x-api-key", r.opts.BotAPIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return errBotNotFound
	}
	if resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return fmt.Errorf("%s %s: %d %s", method, path, resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

// --- Intercambio de mensajes ----------------------------------------------

func (r *Runner) exchange(ctx context.Context, cli *Client, jid types.JID, question string, n, total int) Turn {
	turn := Turn{At: time.Now(), Question: question}
	if total > 0 {
		fmt.Printf("\n[%d/%d] 💬 CLIENTE: %s\n", n, total, question)
	} else {
		fmt.Printf("\n💬 CLIENTE (setup): %s\n", question)
	}

	sentAt := time.Now()
	id, err := cli.SendText(ctx, jid, question)
	if err != nil {
		turn.Error = err.Error()
		fmt.Printf("   ⚠️  no se pudo enviar: %v\n", err)
		return turn
	}
	r.log.Debug("mensaje enviado", "id", id, "chat", jid.String())

	deadline := time.NewTimer(r.opts.Timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	var lastReply time.Time
loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		case <-deadline.C:
			if len(turn.Replies) == 0 {
				fmt.Println("   ⏱️  sin respuesta (timeout)")
			}
			break loop
		case msg := <-cli.Incoming():
			if msg.Chat.User != jid.User || msg.Chat.Server != jid.Server {
				continue
			}
			if msg.At.Before(sentAt.Add(-5 * time.Second)) {
				continue
			}
			turn.Replies = append(turn.Replies, Reply{At: msg.At, Text: msg.Text, Media: msg.Media})
			if turn.LatencyMs == 0 {
				turn.LatencyMs = time.Since(sentAt).Milliseconds()
			}
			lastReply = time.Now()
			printReply(msg, turn.LatencyMs)
		case <-ticker.C:
			if len(turn.Replies) > 0 && time.Since(lastReply) >= r.opts.Idle {
				break loop
			}
		}
	}
	return turn
}

func printReply(msg Incoming, latencyMs int64) {
	text := msg.Text
	if text == "" && msg.Media != "" {
		text = "[" + msg.Media + "]"
	}
	if latencyMs > 0 {
		fmt.Printf("   🤖 BOT (%s): %s\n", time.Duration(latencyMs)*time.Millisecond, text)
	} else {
		fmt.Printf("   🤖 BOT: %s\n", text)
	}
}

func printEvaluation(ev Evaluation) {
	fmt.Printf("   ⭐ %.1f/5 — %s", ev.Score, ev.Verdict)
	warns := make([]string, 0, 4)
	if len(ev.Issues) > 0 {
		warns = append(warns, strings.Join(ev.Issues, "; "))
	}
	if ev.Hallucination {
		warns = append(warns, "posible invención")
	}
	if ev.RoleBreak {
		warns = append(warns, "ruptura de rol")
	}
	if ev.Leak {
		warns = append(warns, "fuga de prompt")
	}
	if len(warns) > 0 {
		fmt.Printf(" (%s)", strings.Join(warns, " · "))
	}
	fmt.Println()
}

func printSummary(rep Report) {
	s := rep.Summary
	fmt.Println("\n════════════════════════════════════════════════════════════")
	fmt.Printf(" Resumen · %s\n", rep.CaseName)
	fmt.Printf("   Preguntas      : %d (repetidas: %d)\n", s.Questions, rep.Repeated)
	fmt.Printf("   Respuestas     : %d (sin respuesta: %d)\n", s.Replies, s.NoReply)
	fmt.Printf("   Latencia media : %s\n", time.Duration(s.AvgLatency)*time.Millisecond)
	if s.Scored > 0 {
		fmt.Printf("   Nota media IA  : %.1f/5 (%d evaluadas)\n", s.AvgScore, s.Scored)
	}
	fmt.Printf("   Invenciones    : %d · ruptura de rol: %d · fugas: %d\n",
		s.Hallucinations, s.RoleBreaks, s.Leaks)
	fmt.Printf("   Veredicto      : %s\n", rep.Verdict)
	fmt.Printf("   Duración       : %s\n", rep.Finished.Sub(rep.Started).Round(time.Second))
	fmt.Println("════════════════════════════════════════════════════════════")
}

func printSuiteSummary(s SuiteReport) {
	pass, fail := 0, 0
	fmt.Println("\n════════════════════════════════════════════════════════════")
	fmt.Printf(" Suite · %d casos (semilla %d)\n", len(s.Cases), s.Seed)
	for _, c := range s.Cases {
		score := "—"
		if c.Summary.Scored > 0 {
			score = fmt.Sprintf("%.1f", c.Summary.AvgScore)
		}
		switch c.Verdict {
		case "pass":
			pass++
		case "fail":
			fail++
		}
		fmt.Printf("   %-18s %-8s nota %-4s sin-resp %d\n", c.Case, c.Verdict, score, c.Summary.NoReply)
	}
	fmt.Printf("   Pass / Fail: %d / %d\n", pass, fail)
	fmt.Println("════════════════════════════════════════════════════════════")
}

func sleep(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}
