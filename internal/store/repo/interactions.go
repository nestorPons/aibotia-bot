package repo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/aibotia/aibotia-go/internal/domain"
)

// Interactions es el repositorio de interacciones con la IA.
type Interactions struct {
	db    *pgxpool.Pool
	onLog func(InteractionRow)
}

// NewInteractions construye el repositorio.
func NewInteractions(db *pgxpool.Pool) *Interactions { return &Interactions{db: db} }

// OnLog registra un callback que se invoca con cada interacción insertada. Se
// usa para publicar los mensajes en tiempo real (SSE) al panel.
func (r *Interactions) OnLog(fn func(InteractionRow)) { r.onLog = fn }

// Log inserta una interacción.
func (r *Interactions) Log(ctx context.Context, i domain.Interaction) error {
	toolCalls, _ := json.Marshal(i.ToolCalls)
	var (
		id        int64
		createdAt time.Time
	)
	err := r.db.QueryRow(ctx, `
		INSERT INTO ai_interactions
			(user_id, session_id, user_message, ai_response, language_code, prompt_context,
			 tool_calls, model, prompt_tokens, completion_tokens, total_tokens, satisfaction_level, latency_ms)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		RETURNING id, created_at`,
		i.UserID, i.SessionID, i.UserMessage, i.AIResponse, i.LanguageCode, i.PromptContext,
		toolCalls, i.Model, i.PromptTokens, i.CompletionTokens, i.TotalTokens, i.Satisfaction, i.LatencyMs).
		Scan(&id, &createdAt)
	if err != nil {
		return fmt.Errorf("repo: registrar interacción: %w", err)
	}
	if r.onLog != nil {
		row := InteractionRow{
			ID:           id,
			UserID:       i.UserID,
			UserMessage:  i.UserMessage,
			AIResponse:   i.AIResponse,
			LanguageCode: i.LanguageCode,
			Model:        i.Model,
			TotalTokens:  i.TotalTokens,
			Satisfaction: i.Satisfaction,
			ToolCalls:    i.ToolCalls,
			CreatedAt:    createdAt,
		}
		if row.ToolCalls == nil {
			row.ToolCalls = []string{}
		}
		r.onLog(row)
	}
	return nil
}

// InteractionRow es una interacción para listado/API.
type InteractionRow struct {
	ID           int64     `json:"id"`
	UserID       string    `json:"user_id"`
	UserMessage  string    `json:"user_message"`
	AIResponse   string    `json:"ai_response"`
	LanguageCode string    `json:"language_code"`
	Model        string    `json:"model"`
	TotalTokens  int       `json:"total_tokens"`
	Satisfaction float64   `json:"satisfaction_level"`
	ToolCalls    []string  `json:"tool_calls"`
	CreatedAt    time.Time `json:"created_at"`
}

const interactionColumns = `id, COALESCE(user_id,''), COALESCE(user_message,''), COALESCE(ai_response,''),
	COALESCE(language_code,''), COALESCE(model,''), total_tokens, COALESCE(satisfaction_level,0),
	COALESCE(tool_calls,'[]'::jsonb), created_at`

// List devuelve interacciones paginadas.
func (r *Interactions) List(ctx context.Context, limit, offset int) ([]InteractionRow, error) {
	rows, err := r.db.Query(ctx,
		"SELECT "+interactionColumns+" FROM ai_interactions ORDER BY created_at DESC LIMIT $1 OFFSET $2", limit, offset)
	if err != nil {
		return nil, fmt.Errorf("repo: listar interacciones: %w", err)
	}
	return scanInteractions(rows)
}

// ListByUser devuelve las interacciones de un usuario.
func (r *Interactions) ListByUser(ctx context.Context, userID string, limit int) ([]InteractionRow, error) {
	rows, err := r.db.Query(ctx,
		"SELECT "+interactionColumns+" FROM ai_interactions WHERE user_id = $1 ORDER BY created_at DESC LIMIT $2",
		userID, limit)
	if err != nil {
		return nil, fmt.Errorf("repo: interacciones de usuario: %w", err)
	}
	return scanInteractions(rows)
}

// ThreadRow es la última interacción de cada usuario (bandeja de entrada).
type ThreadRow struct {
	InteractionRow
	Name          string `json:"name"`
	Active        bool   `json:"active"`
	NeedsOperator bool   `json:"needs_operator"`
}

// ThreadFilter acota la bandeja de entrada.
type ThreadFilter struct {
	Search        string
	NeedsOperator bool  // solo hilos pendientes de una acción del operador
	AIActive      *bool // nil = todos; true/false filtra por IA activa
}

// operatorActions son las herramientas que dejan el hilo pendiente del equipo
// (valoración, agendado de cita o derivación a atención humana).
const operatorActionsSQL = `ARRAY['request_appointment_scheduling','appointment_tamara','message_human_request','fix_chat']`

// operatorActionReasons explica en lenguaje natural por qué el hilo requiere
// al operador. El orden define la prioridad. Mantener alineado con
// operatorActionsSQL.
var operatorActionReasons = []struct {
	name   string
	reason string
}{
	{"request_appointment_scheduling", "Solicitud de cita pendiente de confirmar disponibilidad"},
	{"appointment_tamara", "Solicitud de cita con Tamara"},
	{"message_human_request", "Derivado a atención humana"},
	{"fix_chat", "Conversación fijada para el equipo"},
}

// NeedsOperator indica si alguna tool de la lista requiere intervención humana.
func NeedsOperator(toolCalls []string) bool {
	return HandoffReason(toolCalls) != ""
}

// HandoffReason devuelve el motivo de derivación (vacío si no aplica).
func HandoffReason(toolCalls []string) string {
	for _, a := range operatorActionReasons {
		for _, call := range toolCalls {
			if call == a.name {
				return a.reason
			}
		}
	}
	return ""
}

const threadColumns = `i.id, COALESCE(i.user_id,''), COALESCE(i.user_message,''), COALESCE(i.ai_response,''),
	COALESCE(i.language_code,''), COALESCE(i.model,''), i.total_tokens, COALESCE(i.satisfaction_level,0), i.created_at,
	COALESCE(NULLIF(u.name,''), u.alias, ''), COALESCE(u.active, false),
	COALESCE(i.tool_calls, '[]'::jsonb) ?| ` + operatorActionsSQL

// lastThreadCTE reduce las interacciones a la última de cada usuario.
const lastThreadCTE = `WITH last_thread AS (
	SELECT DISTINCT ON (user_id) id, user_id, user_message, ai_response, language_code, model,
		total_tokens, satisfaction_level, created_at, tool_calls
	FROM ai_interactions WHERE user_id IS NOT NULL ORDER BY user_id, created_at DESC
)`

// conditions construye el WHERE de la bandeja; los argumentos empiezan en $1.
func (f ThreadFilter) conditions() (string, []any) {
	var conds []string
	var args []any
	if s := strings.TrimSpace(f.Search); s != "" {
		args = append(args, "%"+s+"%")
		n := len(args)
		conds = append(conds, fmt.Sprintf("(COALESCE(u.name,'') ILIKE $%[1]d OR COALESCE(u.alias,'') ILIKE $%[1]d OR i.user_id ILIKE $%[1]d)", n))
	}
	if f.NeedsOperator {
		conds = append(conds, "COALESCE(i.tool_calls, '[]'::jsonb) ?| "+operatorActionsSQL)
	}
	if f.AIActive != nil {
		args = append(args, *f.AIActive)
		conds = append(conds, fmt.Sprintf("COALESCE(u.active, false) = $%d", len(args)))
	}
	if len(conds) == 0 {
		return "", args
	}
	return " AND " + strings.Join(conds, " AND "), args
}

// ListThreads devuelve la última interacción por usuario, ordenada por fecha.
// Filtra por búsqueda (nombre/alias/JID), pendientes de operador y estado de la IA.
func (r *Interactions) ListThreads(ctx context.Context, limit, offset int, f ThreadFilter) ([]ThreadRow, int, error) {
	where, args := f.conditions()

	var total int
	if err := r.db.QueryRow(ctx, lastThreadCTE+`
		SELECT count(*) FROM last_thread i LEFT JOIN users u ON u.id = i.user_id
		WHERE TRUE`+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("repo: contar hilos: %w", err)
	}

	listArgs := append(append([]any{}, args...), limit, offset)
	listQuery := lastThreadCTE + `
		SELECT ` + threadColumns + ` FROM last_thread i LEFT JOIN users u ON u.id = i.user_id
		WHERE TRUE` + where + fmt.Sprintf(` ORDER BY i.created_at DESC LIMIT $%d OFFSET $%d`, len(args)+1, len(args)+2)

	rows, err := r.db.Query(ctx, listQuery, listArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("repo: listar hilos: %w", err)
	}
	defer rows.Close()

	out := make([]ThreadRow, 0)
	for rows.Next() {
		var row ThreadRow
		if err := rows.Scan(&row.ID, &row.UserID, &row.UserMessage, &row.AIResponse,
			&row.LanguageCode, &row.Model, &row.TotalTokens, &row.Satisfaction, &row.CreatedAt,
			&row.Name, &row.Active, &row.NeedsOperator); err != nil {
			return nil, 0, err
		}
		out = append(out, row)
	}
	return out, total, rows.Err()
}

// LastAt devuelve la fecha de la última interacción del usuario (ok=false si no hay).
func (r *Interactions) LastAt(ctx context.Context, userID string) (time.Time, bool, error) {
	var t time.Time
	err := r.db.QueryRow(ctx,
		"SELECT created_at FROM ai_interactions WHERE user_id = $1 ORDER BY created_at DESC LIMIT 1", userID).Scan(&t)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, fmt.Errorf("repo: última interacción: %w", err)
	}
	return t, true, nil
}

func scanInteractions(rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
	Close()
}) ([]InteractionRow, error) {
	defer rows.Close()
	out := make([]InteractionRow, 0)
	for rows.Next() {
		var row InteractionRow
		var toolCalls []byte
		if err := rows.Scan(&row.ID, &row.UserID, &row.UserMessage, &row.AIResponse,
			&row.LanguageCode, &row.Model, &row.TotalTokens, &row.Satisfaction, &toolCalls, &row.CreatedAt); err != nil {
			return nil, err
		}
		if len(toolCalls) > 0 {
			if err := json.Unmarshal(toolCalls, &row.ToolCalls); err != nil {
				return nil, fmt.Errorf("repo: tool_calls de interacción %d: %w", row.ID, err)
			}
		}
		if row.ToolCalls == nil {
			row.ToolCalls = []string{}
		}
		out = append(out, row)
	}
	return out, rows.Err()
}
