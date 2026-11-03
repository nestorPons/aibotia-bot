package pipeline

import (
	"context"

	"github.com/aibotia/aibotia-go/internal/domain"
)

// ReplyRequest es la entrada para generar una respuesta.
type ReplyRequest struct {
	User      domain.User
	Text      string
	HasMedia  bool
	MediaType string
	IsNew     bool
}

// Reply es el resultado del asistente.
type Reply struct {
	Text             string
	Actions          []string
	Attachments      []string
	Language         string
	PromptContext    string
	ToolCalls        []string
	Model            string
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	Satisfaction     float64
}

// Assistant es el puerto del motor de IA.
type Assistant interface {
	Reply(ctx context.Context, req ReplyRequest) (Reply, error)
	// Transcribe convierte audio en texto.
	Transcribe(ctx context.Context, audio []byte) (string, error)
}

// SpamChecker comprueba si un usuario está enviando mensajes en exceso.
type SpamChecker interface {
	IsSpamming(ctx context.Context, userID string) (bool, error)
}

// ActionRunner ejecuta las acciones devueltas por el asistente.
type ActionRunner interface {
	Run(ctx context.Context, user domain.User, reply *Reply) error
}
