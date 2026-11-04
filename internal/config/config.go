// Package config carga y valida la configuración desde variables de entorno.
package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"
)

// Config agrupa toda la configuración de la aplicación.
type Config struct {
	AppEnv     string `env:"APP_ENV" envDefault:"development"`
	InstanceID string `env:"INSTANCE_ID" envDefault:"default"`

	LogLevel  string `env:"LOG_LEVEL" envDefault:"info"`
	LogFormat string `env:"LOG_FORMAT" envDefault:"json"`

	HTTPAddr            string        `env:"HTTP_ADDR" envDefault:":3000"`
	HTTPReadTimeout     time.Duration `env:"HTTP_READ_TIMEOUT" envDefault:"10s"`
	HTTPWriteTimeout    time.Duration `env:"HTTP_WRITE_TIMEOUT" envDefault:"30s"`
	HTTPIdleTimeout     time.Duration `env:"HTTP_IDLE_TIMEOUT" envDefault:"60s"`
	HTTPShutdownTimeout time.Duration `env:"HTTP_SHUTDOWN_TIMEOUT" envDefault:"15s"`

	APIKey             string   `env:"API_KEY,required"`
	APIEnabled         bool     `env:"API_ENABLED" envDefault:"true"`
	CORSAllowedOrigins []string `env:"CORS_ALLOWED_ORIGINS" envSeparator:"," envDefault:"*"`

	// Panel de administración embebido (/admin).
	AdminEnabled       bool          `env:"ADMIN_ENABLED" envDefault:"true"`
	AdminPassword      string        `env:"ADMIN_PASSWORD"`
	AdminOperatorName  string        `env:"ADMIN_OPERATOR_NAME"`
	AdminSessionSecret string        `env:"ADMIN_SESSION_SECRET"`
	AdminSessionTTL    time.Duration `env:"ADMIN_SESSION_TTL" envDefault:"12h"`

	OpenAIAPIKey     string  `env:"OPENAI_API_KEY"`
	OpenAIModel      string  `env:"OPENAI_MODEL" envDefault:"gpt-6.1-sol"`
	OpenAIEmbedModel string  `env:"OPENAI_EMBED_MODEL" envDefault:"text-embedding-3-small"`
	RAGThreshold     float64 `env:"RAG_THRESHOLD" envDefault:"0.78"`

	PromptsDir string `env:"PROMPTS_DIR" envDefault:"data/seeds/prompts"`
	DataDir    string `env:"DATA_DIR" envDefault:"data/seeds"`

	DatabaseURL string `env:"DATABASE_URL,required"`
	RedisURL    string `env:"REDIS_URL" envDefault:"redis://localhost:6379/0"`

	AutoMigrate bool `env:"AUTO_MIGRATE" envDefault:"true"`

	WhatsAppEnabled  bool   `env:"WHATSAPP_ENABLED" envDefault:"true"`
	WhatsAppStoreDSN string `env:"WHATSAPP_STORE_DSN"`

	HoursToActive int `env:"HOURS_TO_ACTIVE" envDefault:"168"`
	BufferMinMs   int `env:"BUFFER_MIN_MS" envDefault:"1000"`
	BufferMaxMs   int `env:"BUFFER_MAX_MS" envDefault:"15000"`

	ReminderCron     string `env:"REMINDER_CRON" envDefault:"*/10 * * * *"`
	ReminderTemplate string `env:"REMINDER_TEMPLATE" envDefault:"📅 Hola {name}, tienes una cita programada el {date}. ¡No olvides asistir!"`

	SpamLimit  int           `env:"SPAM_LIMIT" envDefault:"5"`
	SpamWindow time.Duration `env:"SPAM_WINDOW" envDefault:"60s"`

	LocationName    string  `env:"LOCATION_NAME"`
	LocationAddress string  `env:"LOCATION_ADDRESS"`
	LocationLat     float64 `env:"LOCATION_LAT" envDefault:"0"`
	LocationLng     float64 `env:"LOCATION_LNG" envDefault:"0"`

	// Formulario de contacto de la landing.
	SMTPHost     string `env:"SMTP_HOST"`
	SMTPPort     string `env:"SMTP_PORT" envDefault:"587"`
	SMTPUsername string `env:"SMTP_USERNAME"`
	SMTPPassword string `env:"SMTP_PASSWORD"`
	SMTPFrom     string `env:"SMTP_FROM"`
	SMTPSecure   string `env:"SMTP_SECURE"`
	ContactTo    string `env:"CONTACT_TO" envDefault:"info@nestorpons.com"`

	// CAPTCHA Cloudflare Turnstile del formulario de contacto.
	TurnstileSiteKey string `env:"TURNSTILE_SITE_KEY"`
	TurnstileSecret  string `env:"TURNSTILE_SECRET"`
}

// IsProduction indica si el entorno es producción.
func (c *Config) IsProduction() bool {
	return strings.EqualFold(c.AppEnv, "production")
}

// Load lee la configuración del entorno y la valida.
func Load() (*Config, error) {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	if cfg.WhatsAppStoreDSN == "" {
		cfg.WhatsAppStoreDSN = cfg.DatabaseURL
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) validate() error {
	if strings.TrimSpace(c.APIKey) == "" {
		return fmt.Errorf("config: API_KEY es obligatoria")
	}
	if strings.TrimSpace(c.DatabaseURL) == "" {
		return fmt.Errorf("config: DATABASE_URL es obligatoria")
	}
	if c.BufferMinMs <= 0 || c.BufferMaxMs <= 0 {
		return fmt.Errorf("config: BUFFER_MIN_MS y BUFFER_MAX_MS deben ser > 0")
	}
	if c.BufferMinMs > c.BufferMaxMs {
		return fmt.Errorf("config: BUFFER_MIN_MS (%d) no puede superar BUFFER_MAX_MS (%d)", c.BufferMinMs, c.BufferMaxMs)
	}
	if c.IsProduction() && strings.TrimSpace(c.OpenAIAPIKey) == "" {
		return fmt.Errorf("config: OPENAI_API_KEY es obligatoria en producción")
	}
	if c.APIKey == c.OpenAIAPIKey && c.OpenAIAPIKey != "" {
		return fmt.Errorf("config: API_KEY no debe ser igual a OPENAI_API_KEY")
	}
	return nil
}
