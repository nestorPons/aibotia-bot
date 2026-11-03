// Comando tester: cliente de WhatsApp de pruebas que conversa con el chatbot de
// la empresa con casos rotativos (clienta habitual, casual, adversarial,
// maleducado, muchas dudas, otro sector, urgente, inglés) y evalúa la calidad.
//
// Uso:
//
//	go run ./cmd/tester -to 34600111222
//	go run ./cmd/tester -to 34600111222 -case adversarial -eval
//	go run ./cmd/tester -to 34600111222 -suite all -eval
//	go run ./cmd/tester -serve 127.0.0.1:8091   (ver logs guardados)
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"github.com/aibotia/aibotia-go/internal/platform/logger"
	"github.com/aibotia/aibotia-go/internal/tester"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "\n❌", err)
		os.Exit(1)
	}
}

func run() error {
	_ = godotenv.Load(".env")

	var (
		to        = flag.String("to", env("TEST_TARGET"), "número del chatbot destino (E.164, p. ej. 34600111222)")
		mode      = flag.String("mode", env("TESTER_MODE"), "modo: ai (dos LLM: cliente IA ↔ bot, reacciona a sus respuestas; por defecto) | scenario (banco fijo, sin reacción)")
		caseID    = flag.String("case", env("TESTER_CASE"), "id del caso (vacío = aleatorio, evita el último)")
		casesDir  = flag.String("cases-dir", envDefault("TESTER_CASES_DIR", "data/test/cases"), "directorio de casos JSON")
		suite     = flag.String("suite", "", "casos a ejecutar en secuencia: 'all' o ids separados por coma")
		seed      = flag.Int64("seed", 0, "semilla aleatoria (0 = hora actual)")
		fresh     = flag.Bool("fresh", true, "limpia el historial del bot y reactiva la IA antes de cada caso")
		turns     = flag.Int("turns", 0, "turnos por caso (0 = rango aleatorio del caso)")
		delay     = flag.Duration("delay", 15*time.Second, "espera entre preguntas (anti-spam)")
		idle      = flag.Duration("idle", 10*time.Second, "silencio para dar por terminada una respuesta")
		timeout   = flag.Duration("timeout", 2*time.Minute, "espera máxima por turno")
		evalAI    = flag.Bool("eval", false, "puntuar cada respuesta con IA (requiere OPENAI_API_KEY)")
		list      = flag.Bool("list", false, "listar los casos disponibles y salir")
		reset     = flag.Bool("reset", false, "borrar la sesión local del tester (fuerza un QR nuevo)")
		logout    = flag.Bool("logout", false, "cerrar la sesión de WhatsApp del tester y salir")
		reportDir = flag.String("report-dir", envDefault("TESTER_REPORT_DIR", "data/test/reports"), "directorio de reportes")
		serve     = flag.String("serve", "", "sirve los logs guardados en la dirección indicada y sale (p. ej. 127.0.0.1:8091)")
		dsn       = flag.String("dsn", firstEnv("TESTER_DATABASE_URL", "TEST_DSN"), "PostgreSQL para la sesión del tester")
		model     = flag.String("model", envDefault("OPENAI_MODEL", "gpt-6.1-sol"), "modelo de OpenAI")
		apiBase   = flag.String("api-base", firstEnv("BOT_API_BASE", "API_BASE", "http://localhost:3000"), "URL base de la API del bot (reset de sesión)")
		apiKey    = flag.String("api-key", firstEnv("BOT_API_KEY", "API_KEY"), "API key del bot (reset de sesión)")
		verbose   = flag.Bool("verbose", false, "logs de depuración (incluye whatsmeow)")
	)
	flag.Parse()

	level := "info"
	if *verbose {
		level = "debug"
	}
	log := logger.New(level, "text")

	if *list {
		cases, err := tester.LoadCases(*casesDir)
		if err != nil {
			return err
		}
		tester.PrintCases(cases, *casesDir)
		return nil
	}

	if *serve != "" {
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer cancel()
		return tester.ServeReports(ctx, *serve, *reportDir, log)
	}

	if *dsn == "" {
		return fmt.Errorf("falta TESTER_DATABASE_URL (PostgreSQL propio para la sesión del tester)\n" +
			"   ejemplo: postgres://aibotia:PASS@postgres:5432/aibotia_tester?sslmode=disable")
	}
	if *to == "" && !*reset {
		return fmt.Errorf("falta el número del chatbot: -to 34600111222 (o TEST_TARGET)")
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if *reset {
		if err := tester.Reset(ctx, *dsn, log); err != nil {
			return err
		}
		fmt.Println("🧹 Sesión local del tester borrada. Vuelve a ejecutar para escanear un QR nuevo.")
		return nil
	}

	openaiKey := os.Getenv("OPENAI_API_KEY")
	resolvedMode := strings.ToLower(strings.TrimSpace(*mode))
	if resolvedMode == "" {
		// Por defecto el test es una conversación entre dos LLM: el cliente
		// simulado reacciona a cada respuesta del bot. Sin API key el runner
		// falla con un error claro (no degrada en silencio a un guion fijo).
		resolvedMode = "ai"
	}

	opts := tester.Options{
		Target:     *to,
		Mode:       resolvedMode,
		CaseID:     strings.TrimSpace(*caseID),
		CasesDir:   *casesDir,
		Suite:      parseSuite(*suite),
		Seed:       *seed,
		Fresh:      *fresh,
		Turns:      *turns,
		Delay:      *delay,
		Idle:       *idle,
		Timeout:    *timeout,
		Eval:       *evalAI,
		ReportDir:  *reportDir,
		DSN:        *dsn,
		APIKey:     openaiKey,
		BotAPIBase: *apiBase,
		BotAPIKey:  *apiKey,
		Model:      *model,
	}

	if *logout {
		cli, err := tester.Open(ctx, opts.DSN, log)
		if err != nil {
			return err
		}
		defer cli.Close()
		if err := cli.Login(ctx); err != nil {
			return err
		}
		if err := cli.Logout(ctx); err != nil {
			return err
		}
		fmt.Println("✅ Sesión del tester cerrada")
		return nil
	}

	if opts.Fresh && opts.BotAPIKey == "" {
		fmt.Println("⚠️  Sin API key del bot: no se limpiará el historial entre ejecuciones.")
	}

	return tester.NewRunner(opts, log).Run(ctx)
}

func parseSuite(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if strings.EqualFold(raw, "all") {
		return []string{"all"}
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func env(key string) string { return os.Getenv(key) }

func envDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}
