package domain

import "time"

// Turn es un turno de conversación (usuario o asistente).
type Turn struct {
	Role      string    `json:"role"` // "user" | "assistant"
	Content   string    `json:"content"`
	Timestamp time.Time `json:"ts"`
}

// Interaction registra una interacción con la IA para analítica.
type Interaction struct {
	UserID           string
	SessionID        string
	UserMessage      string
	AIResponse       string
	LanguageCode     string
	PromptContext    string
	ToolCalls        []string
	Model            string
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	Satisfaction     float64
	LatencyMs        int
}
