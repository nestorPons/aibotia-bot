// Comando migrate-data: migra los datos relacionales desde la base de datos
// legacy (MariaDB/MySQL) a PostgreSQL.
//
// No migra el estado volátil de Redis (historial de conversación y citas):
// es efímero (TTL 7 días / 2 horas) y se regenera en la nueva aplicación.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	"github.com/aibotia/aibotia-go/internal/config"
	"github.com/aibotia/aibotia-go/internal/platform/logger"
	"github.com/aibotia/aibotia-go/internal/store/postgres"
)

func main() {
	if err := run(); err != nil {
		slog.Error("error migrando datos", "error", err.Error())
		os.Exit(1)
	}
}

type migrator struct {
	my       *sql.DB
	pg       *pgxpool.Pool
	log      *slog.Logger
	truncate bool
	dryRun   bool
}

func run() error {
	var (
		mysqlDSN = flag.String("mysql", os.Getenv("LEGACY_DATABASE_URL"), "DSN MySQL legacy (usuario:pass@tcp(host:3306)/db)")
		truncate = flag.Bool("truncate", false, "vacía las tablas destino antes de migrar")
		dryRun   = flag.Bool("dry-run", false, "solo muestra conteos, no escribe")
	)
	flag.Parse()

	_ = godotenv.Load(".env")

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if strings.TrimSpace(*mysqlDSN) == "" {
		return fmt.Errorf("falta -mysql o LEGACY_DATABASE_URL")
	}
	log := logger.New(cfg.LogLevel, cfg.LogFormat)

	my, err := sql.Open("mysql", *mysqlDSN)
	if err != nil {
		return fmt.Errorf("abrir MySQL: %w", err)
	}
	defer my.Close()
	if err := my.Ping(); err != nil {
		return fmt.Errorf("ping MySQL: %w", err)
	}

	ctx := context.Background()
	pool, err := postgres.Connect(ctx, cfg.DatabaseURL, log)
	if err != nil {
		return err
	}
	defer pool.Close()

	m := &migrator{my: my, pg: pool, log: log, truncate: *truncate, dryRun: *dryRun}

	ctx = context.Background()
	if err := m.preflight(ctx); err != nil {
		return err
	}

	steps := []struct {
		name string
		fn   func(context.Context) (int, error)
	}{
		{"settings", m.migrateSettings},
		{"users", m.migrateUsers},
		{"content_blocks", m.migrateContentBlocks},
		{"content_translations", m.migrateTranslations},
		{"ai_interactions", m.migrateInteractions},
		{"system_logs", m.migrateSystemLogs},
	}

	for _, s := range steps {
		n, err := s.fn(ctx)
		if err != nil {
			return fmt.Errorf("migrar %s: %w", s.name, err)
		}
		fmt.Printf("  %-22s %d filas\n", s.name, n)
	}
	fmt.Println("migración completada")
	return nil
}

func (m *migrator) preflight(ctx context.Context) error {
	if m.dryRun {
		m.log.Info("modo dry-run: no se escribirá nada")
		return nil
	}
	if !m.truncate {
		return nil
	}
	if _, err := m.pg.Exec(ctx, `TRUNCATE ai_interactions, content_translations, content_blocks,
		system_logs, users RESTART IDENTITY CASCADE`); err != nil {
		return fmt.Errorf("truncar tablas: %w", err)
	}
	m.log.Info("tablas destino vaciadas")
	return nil
}

func (m *migrator) migrateSettings(ctx context.Context) (int, error) {
	rows, err := m.my.QueryContext(ctx, "SELECT is_active FROM settings ORDER BY id LIMIT 1")
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var active int
		if err := rows.Scan(&active); err != nil {
			return count, err
		}
		if !m.dryRun {
			if _, err := m.pg.Exec(ctx, "UPDATE settings SET is_active = $1, updated_at = now()", active != 0); err != nil {
				return count, err
			}
		}
		count++
	}
	return count, rows.Err()
}

func (m *migrator) migrateUsers(ctx context.Context) (int, error) {
	rows, err := m.my.QueryContext(ctx, `
		SELECT id, name, alias, active, topic, lang, temperature, prompt_last,
		       buffer_time, role, created_at, updated_at FROM users`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var (
			id, name, alias, topic, lang, promptLast sql.NullString
			active                                   sql.NullInt64
			temperature                              sql.NullFloat64
			bufferTime, role                         sql.NullInt64
			createdAt, updatedAt                     sql.NullTime
		)
		if err := rows.Scan(&id, &name, &alias, &active, &topic, &lang, &temperature,
			&promptLast, &bufferTime, &role, &createdAt, &updatedAt); err != nil {
			return count, err
		}
		if m.dryRun {
			count++
			continue
		}
		_, err := m.pg.Exec(ctx, `
			INSERT INTO users (id, name, alias, active, topic, lang, temperature, prompt_last, buffer_time, role, created_at, updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
			ON CONFLICT (id) DO UPDATE SET
				name=EXCLUDED.name, alias=EXCLUDED.alias, active=EXCLUDED.active, topic=EXCLUDED.topic,
				lang=EXCLUDED.lang, temperature=EXCLUDED.temperature, prompt_last=EXCLUDED.prompt_last,
				buffer_time=EXCLUDED.buffer_time, role=EXCLUDED.role, updated_at=EXCLUDED.updated_at`,
			strings.TrimSpace(id.String), name.String, alias.String, active.Int64 != 0,
			nullStr(topic), nullStr(lang), defFloat(temperature, 0.5), defStr(promptLast, "welcome"),
			defInt(bufferTime, 30000), defInt(role, 2), defTime(createdAt), defTime(updatedAt))
		if err != nil {
			return count, err
		}
		count++
	}
	return count, rows.Err()
}

func (m *migrator) migrateContentBlocks(ctx context.Context) (int, error) {
	rows, err := m.my.QueryContext(ctx, `
		SELECT code, name, type, description, actions, attachments, properties, created_at, updated_at
		FROM content_blocks`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var (
			code, name, typ, description sql.NullString
			actions, attachments, props  sql.NullString
			createdAt, updatedAt         sql.NullTime
		)
		if err := rows.Scan(&code, &name, &typ, &description, &actions, &attachments, &props, &createdAt, &updatedAt); err != nil {
			return count, err
		}
		if m.dryRun {
			count++
			continue
		}
		_, err := m.pg.Exec(ctx, `
			INSERT INTO content_blocks (code, name, type, description, actions, attachments, properties, created_at, updated_at)
			VALUES ($1,$2,$3,$4,$5::jsonb,$6::jsonb,$7::jsonb,$8,$9)
			ON CONFLICT (code) DO UPDATE SET
				name=EXCLUDED.name, type=EXCLUDED.type, description=EXCLUDED.description,
				actions=EXCLUDED.actions, attachments=EXCLUDED.attachments, properties=EXCLUDED.properties,
				updated_at=EXCLUDED.updated_at`,
			code.String, name.String, defStr(typ, "message_fixed"), description.String,
			nullJSON(actions), nullJSON(attachments), nullJSON(props), defTime(createdAt), defTime(updatedAt))
		if err != nil {
			return count, err
		}
		count++
	}
	return count, rows.Err()
}

func (m *migrator) migrateTranslations(ctx context.Context) (int, error) {
	column, err := m.translationColumn(ctx)
	if err != nil {
		return 0, err
	}
	query := fmt.Sprintf(`
		SELECT cb.code, ct.language, ct.content
		FROM content_translations ct JOIN content_blocks cb ON cb.id = ct.%s`, column)
	rows, err := m.my.QueryContext(ctx, query)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var code, language, content sql.NullString
		if err := rows.Scan(&code, &language, &content); err != nil {
			return count, err
		}
		if m.dryRun {
			count++
			continue
		}
		_, err := m.pg.Exec(ctx, `
			INSERT INTO content_translations (block_id, language, content)
			SELECT id, $2, $3 FROM content_blocks WHERE code = $1
			ON CONFLICT (block_id, language) DO UPDATE SET content = EXCLUDED.content, updated_at = now()`,
			code.String, language.String, content.String)
		if err != nil {
			return count, err
		}
		count++
	}
	return count, rows.Err()
}

func (m *migrator) translationColumn(ctx context.Context) (string, error) {
	rows, err := m.my.QueryContext(ctx, `
		SELECT column_name FROM information_schema.columns
		WHERE table_schema = DATABASE() AND table_name = 'content_translations'
		  AND column_name IN ('block_id','content_id')`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	for rows.Next() {
		var col string
		if err := rows.Scan(&col); err != nil {
			return "", err
		}
		return col, nil
	}
	return "", fmt.Errorf("no se encontró columna block_id/content_id en content_translations")
}

func (m *migrator) migrateInteractions(ctx context.Context) (int, error) {
	rows, err := m.my.QueryContext(ctx, `
		SELECT user_id, thread_id, user_message, ai_response, language_code, prompt_context,
		       action_required, satisfaction_level, prompt_tokens, completion_tokens, total_tokens, model, created_at
		FROM ai_interactions`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var (
			userID, threadID, userMsg, aiResp, lang, ctxPrompt, action, model sql.NullString
			satisfaction                                                      sql.NullFloat64
			promptTokens, completionTokens, totalTokens                       sql.NullInt64
			createdAt                                                         sql.NullTime
		)
		if err := rows.Scan(&userID, &threadID, &userMsg, &aiResp, &lang, &ctxPrompt,
			&action, &satisfaction, &promptTokens, &completionTokens, &totalTokens, &model, &createdAt); err != nil {
			return count, err
		}
		if m.dryRun {
			count++
			continue
		}
		_, err := m.pg.Exec(ctx, `
			INSERT INTO ai_interactions (user_id, session_id, user_message, ai_response, language_code,
				prompt_context, model, prompt_tokens, completion_tokens, total_tokens, satisfaction_level, created_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
			nullStr(userID), nullStr(threadID), userMsg.String, aiResp.String, nullStr(lang),
			nullStr(ctxPrompt), nullStr(model), int(promptTokens.Int64), int(completionTokens.Int64),
			int(totalTokens.Int64), defFloat(satisfaction, 0), defTime(createdAt))
		if err != nil {
			return count, err
		}
		count++
	}
	return count, rows.Err()
}

func (m *migrator) migrateSystemLogs(ctx context.Context) (int, error) {
	rows, err := m.my.QueryContext(ctx, "SELECT level, message, meta, timestamp FROM system_logs")
	if err != nil {
		// La tabla puede no existir en instalaciones antiguas.
		m.log.Warn("no se pudo leer system_logs; se omite", "error", err.Error())
		return 0, nil
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var level, message string
		var meta sql.NullString
		var ts sql.NullTime
		if err := rows.Scan(&level, &message, &meta, &ts); err != nil {
			return count, err
		}
		if m.dryRun {
			count++
			continue
		}
		_, err := m.pg.Exec(ctx, `
			INSERT INTO system_logs (level, message, meta, created_at) VALUES ($1,$2,$3::jsonb,$4)`,
			level, message, nullJSON(meta), defTime(ts))
		if err != nil {
			return count, err
		}
		count++
	}
	return count, rows.Err()
}

func nullStr(v sql.NullString) any {
	if !v.Valid {
		return nil
	}
	return v.String
}

func nullJSON(v sql.NullString) any {
	if !v.Valid || strings.TrimSpace(v.String) == "" {
		return nil
	}
	if !json.Valid([]byte(v.String)) {
		return nil
	}
	return v.String
}

func defStr(v sql.NullString, def string) string {
	if !v.Valid || strings.TrimSpace(v.String) == "" {
		return def
	}
	return v.String
}

func defInt(v sql.NullInt64, def int) int {
	if !v.Valid {
		return def
	}
	return int(v.Int64)
}

func defFloat(v sql.NullFloat64, def float64) float64 {
	if !v.Valid {
		return def
	}
	return v.Float64
}

func defTime(v sql.NullTime) any {
	if !v.Valid {
		return nil
	}
	return v.Time
}
