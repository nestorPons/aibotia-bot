// Comando botd: binario principal (API HTTP + workers).
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/joho/godotenv"
	openai "github.com/sashabaranov/go-openai"

	"github.com/aibotia/aibotia-go/internal/actions"
	"github.com/aibotia/aibotia-go/internal/appointments"
	"github.com/aibotia/aibotia-go/internal/assistant"
	"github.com/aibotia/aibotia-go/internal/bootstrap"
	"github.com/aibotia/aibotia-go/internal/config"
	"github.com/aibotia/aibotia-go/internal/httpapi"
	"github.com/aibotia/aibotia-go/internal/landing"
	"github.com/aibotia/aibotia-go/internal/pipeline"
	"github.com/aibotia/aibotia-go/internal/platform/httpserver"
	"github.com/aibotia/aibotia-go/internal/platform/logger"
	"github.com/aibotia/aibotia-go/internal/rag"
	"github.com/aibotia/aibotia-go/internal/realtime"
	"github.com/aibotia/aibotia-go/internal/scheduler"
	"github.com/aibotia/aibotia-go/internal/security"
	"github.com/aibotia/aibotia-go/internal/store/migrations"
	"github.com/aibotia/aibotia-go/internal/store/postgres"
	redisstore "github.com/aibotia/aibotia-go/internal/store/redis"
	"github.com/aibotia/aibotia-go/internal/store/repo"
	"github.com/aibotia/aibotia-go/internal/summary"
	"github.com/aibotia/aibotia-go/internal/version"
	"github.com/aibotia/aibotia-go/internal/whatsapp"
)

func main() {
	if err := run(); err != nil {
		slog.Error("error fatal", "error", err.Error())
		os.Exit(1)
	}
}

func run() error {
	_ = godotenv.Load(".env")

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	log := logger.New(cfg.LogLevel, cfg.LogFormat)
	log.Info("arrancando aibotia",
		"version", version.Version,
		"commit", version.Commit,
		"env", cfg.AppEnv,
		"instance", cfg.InstanceID,
	)
	if cfg.OpenAIAPIKey == "" {
		log.Warn("OPENAI_API_KEY no definida; las funciones de IA no estarán disponibles")
	}
	if !cfg.APIEnabled {
		log.Warn("API_ENABLED=false; /api/v1 no se monta (solo landing, /health, /ready y /admin)")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// PostgreSQL.
	pool, err := postgres.Connect(ctx, cfg.DatabaseURL, log)
	if err != nil {
		return err
	}
	defer pool.Close()

	if cfg.AutoMigrate {
		if err := migrations.Up(ctx, pool); err != nil {
			return err
		}
		log.Info("migraciones aplicadas")
	}

	// Redis.
	rdb, err := redisstore.Connect(ctx, cfg.RedisURL, log)
	if err != nil {
		return err
	}
	defer func() { _ = rdb.Close() }()

	// Repositorios.
	users := repo.NewUsers(pool)
	settings := repo.NewSettings(pool)
	history := repo.NewHistory(rdb)
	interactions := repo.NewInteractions(pool)
	instructions := repo.NewInstructions(pool)
	prompts := repo.NewPrompts(pool)
	services := repo.NewServices(pool)

	// Bus de eventos en tiempo real (SSE del panel).
	events := realtime.NewHub()
	interactions.OnLog(func(row repo.InteractionRow) {
		events.Publish(realtime.Event{
			Type:   realtime.TypeMessage,
			UserID: row.UserID,
			At:     row.CreatedAt,
			Data:   row,
		})
	})

	// La BD es la fuente de verdad: los .md/.json solo siembran filas ausentes.
	if res, err := bootstrap.Content(ctx, prompts, services, cfg.PromptsDir, cfg.DataDir, log); err != nil {
		log.Error("no se pudo sembrar el contenido inicial", "error", err.Error())
	} else {
		log.Info("contenido en BD", "prompts_nuevos", res.Prompts, "servicios_nuevos", res.Services)
	}
	// Datos de empresa: prompt único (fusiona los antiguos de ubicación).
	if created, err := bootstrap.Company(ctx, pool, prompts, cfg.PromptsDir, log); err != nil {
		log.Error("no se pudo preparar el prompt de empresa", "error", err.Error())
	} else if created {
		log.Info("prompt de empresa disponible", "code", repo.CompanyPromptCode)
	}

	// Canal WhatsApp (whatsmeow).
	var waProvider whatsapp.Provider
	if cfg.WhatsAppEnabled {
		waClient := whatsapp.New(whatsapp.Options{StoreDSN: cfg.WhatsAppStoreDSN, Log: log})
		waProvider = waClient
		defer waClient.Stop()

		if err := waClient.Start(ctx); err != nil {
			log.Error("no se pudo iniciar WhatsApp", "error", err.Error())
		} else {
			log.Info("canal WhatsApp iniciado")
		}
	} else {
		log.Info("WhatsApp deshabilitado por configuración")
	}

	// Anti-spam.
	antispam := security.New(rdb, cfg.SpamLimit, cfg.SpamWindow)

	// Motor de IA (OpenAI) + RAG.
	var oaClient *openai.Client
	if cfg.OpenAIAPIKey != "" {
		oaClient = openai.NewClient(cfg.OpenAIAPIKey)
	} else {
		log.Warn("OpenAI no configurado: el asistente responderá en modo degradado")
	}
	ragSvc := rag.New(pool, oaClient, cfg.OpenAIEmbedModel, cfg.RAGThreshold, log)
	apptSvc := appointments.New(pool, rdb, log)
	ai := assistant.New(assistant.Config{
		APIKey:       cfg.OpenAIAPIKey,
		Model:        cfg.OpenAIModel,
		EmbedModel:   cfg.OpenAIEmbedModel,
		RAGThreshold: cfg.RAGThreshold,
	}, log, history, users, ragSvc, apptSvc, prompts, services, instructions)

	// Ubicación del Studio (fallback por entorno; el panel manda).
	var location *actions.Location
	if strings.TrimSpace(cfg.LocationName) != "" {
		location = &actions.Location{
			Lat:     cfg.LocationLat,
			Lng:     cfg.LocationLng,
			Name:    cfg.LocationName,
			Address: cfg.LocationAddress,
		}
	}
	actionRunner := actions.NewRunner(log, users, waProvider, location)

	// Recarga de contenido + ubicación del prompt de empresa.
	reloader := &contentReloader{ai: ai, prompts: prompts, actions: actionRunner, log: log}
	if err := reloader.RefreshCompany(ctx); err != nil {
		log.Warn("no se pudo aplicar la ubicación del prompt de empresa", "error", err.Error())
	} else if loc := actionRunner.Location(); loc != nil {
		log.Info("ubicación configurada", "name", loc.Name, "lat", loc.Lat, "lng", loc.Lng)
	} else {
		log.Warn("sin ubicación configurada: send_location no enviará nada")
	}

	// Pipeline de mensajes.
	var pipelineSvc *pipeline.Service
	if waProvider != nil {
		pipelineSvc = pipeline.New(pipeline.Options{
			Log:           log,
			Users:         users,
			Settings:      settings,
			History:       history,
			Interactions:  interactions,
			Provider:      waProvider,
			Assistant:     ai,
			Spam:          antispam,
			Actions:       actionRunner,
			Events:        events,
			BufferMin:     time.Duration(cfg.BufferMinMs) * time.Millisecond,
			BufferMax:     time.Duration(cfg.BufferMaxMs) * time.Millisecond,
			HoursToActive: time.Duration(cfg.HoursToActive) * time.Hour,
		})
		go pipelineSvc.Run(ctx, waProvider.Incoming())

		// Recordatorios de cita.
		sched := scheduler.New(log, apptSvc, waProvider, cfg.ReminderCron, cfg.ReminderTemplate)
		if err := sched.Start(); err != nil {
			log.Error("no se pudo iniciar el scheduler", "error", err.Error())
		} else {
			defer sched.Stop()
		}
	}

	// Panel de administración embebido.
	adminEnabled := cfg.AdminEnabled
	adminPassword := cfg.AdminPassword
	if adminEnabled && adminPassword == "" {
		if cfg.IsProduction() {
			adminEnabled = false
			log.Warn("panel /admin deshabilitado: define ADMIN_PASSWORD en producción")
		} else {
			adminPassword = cfg.APIKey
			log.Warn("panel /admin usando API_KEY como contraseña; define ADMIN_PASSWORD")
		}
	}

	// Landing pública (Go + htmx) servida en la raíz.
	landingHandler, err := landing.New(landing.Options{
		Env:          cfg.AppEnv,
		SMTPHost:     cfg.SMTPHost,
		SMTPPort:     cfg.SMTPPort,
		SMTPUsername: cfg.SMTPUsername,
		SMTPPassword: cfg.SMTPPassword,
		SMTPFrom:     cfg.SMTPFrom,
		ContactTo:    cfg.ContactTo,
		SMTPSecure:   cfg.SMTPSecure,

		TurnstileSiteKey: cfg.TurnstileSiteKey,
		TurnstileSecret:  cfg.TurnstileSecret,
	})
	if err != nil {
		return err
	}

	// Router HTTP.
	threadSummarizer := summary.New(cfg.OpenAIAPIKey, cfg.OpenAIModel, log)
	router := httpapi.NewRouter(httpapi.Deps{
		Log:          log,
		Config:       cfg,
		Version:      version.Version,
		WhatsApp:     waProvider,
		Pipeline:     pipelineSvc,
		Users:        users,
		Settings:     settings,
		Interactions: interactions,
		Appointments: apptSvc,
		Summarizer:   threadSummarizer,
		History:      history,
		Instructions: instructions,
		Prompts:      prompts,
		Services:     services,
		AntiSpam:     antispam,
		Content:      reloader,
		Landing:      landingHandler,
		Events:       events,
		Admin: httpapi.AdminOptions{
			Enabled:       adminEnabled,
			Password:      adminPassword,
			OperatorName:  cfg.AdminOperatorName,
			SessionSecret: cfg.AdminSessionSecret,
			SessionTTL:    cfg.AdminSessionTTL,
			SecureCookie:  cfg.IsProduction(),
		},
		Checks: map[string]httpapi.ReadyChecker{
			"postgres": func(ctx context.Context) error { return pool.Ping(ctx) },
			"redis":    func(ctx context.Context) error { return rdb.Ping(ctx).Err() },
		},
	})

	srv := &http.Server{
		Addr:         cfg.HTTPAddr,
		Handler:      router,
		ReadTimeout:  cfg.HTTPReadTimeout,
		WriteTimeout: cfg.HTTPWriteTimeout,
		IdleTimeout:  cfg.HTTPIdleTimeout,
	}

	log.Info("sistema listo")
	if err := httpserver.Serve(ctx, log, srv, cfg.HTTPShutdownTimeout); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	log.Info("apagado completado")
	return nil
}

// contentReloader coordina la recarga del asistente y la ubicación del runner
// de acciones cuando se edita contenido desde el panel.
type contentReloader struct {
	ai      *assistant.Assistant
	prompts *repo.Prompts
	actions *actions.Runner
	log     *slog.Logger
}

func (c *contentReloader) ReloadContent(ctx context.Context) error {
	return c.ai.ReloadContent(ctx)
}

func (c *contentReloader) ReindexPrompt(ctx context.Context, code string) error {
	return c.ai.ReindexPrompt(ctx, code)
}

func (c *contentReloader) DeletePromptEmbedding(ctx context.Context, code string) error {
	return c.ai.DeletePromptEmbedding(ctx, code)
}

// RefreshCompany lee el prompt de empresa y aplica su línea `pin` al runner.
func (c *contentReloader) RefreshCompany(ctx context.Context) error {
	p, err := c.prompts.Get(ctx, repo.CompanyPromptCode)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	loc := actions.ParsePin(p.Content)
	if loc == nil {
		c.log.Warn("prompt de empresa sin línea pin; se mantiene la ubicación anterior", "code", repo.CompanyPromptCode)
		return nil
	}
	c.actions.SetLocation(loc)
	c.log.Info("ubicación de empresa actualizada", "lat", loc.Lat, "lng", loc.Lng, "name", loc.Name)
	return nil
}
