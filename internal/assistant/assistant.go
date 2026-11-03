// Package assistant implementa el motor de IA (OpenAI + tool calling) que
// genera las respuestas del bot.
package assistant

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	openai "github.com/sashabaranov/go-openai"

	"github.com/aibotia/aibotia-go/internal/domain"
	"github.com/aibotia/aibotia-go/internal/i18n"
	"github.com/aibotia/aibotia-go/internal/pipeline"
	"github.com/aibotia/aibotia-go/internal/rag"
	"github.com/aibotia/aibotia-go/internal/store/repo"
)

// Config configura el asistente.
type Config struct {
	APIKey       string
	Model        string
	EmbedModel   string
	RAGThreshold float64
}

// AppointmentRecorder registra solicitudes de cita (implementado en la Fase 6).
type AppointmentRecorder interface {
	RecordRequest(ctx context.Context, userID, service, availability string, withTamara bool) error
}

// Assistant implementa pipeline.Assistant.
type Assistant struct {
	cfg     Config
	log     *slog.Logger
	client  *openai.Client
	prompts *PromptFactory
	rag     *rag.Service
	history *repo.History
	users   *repo.Users
	appts   AppointmentRecorder
	tools   []openai.ResponseTool
}

// New construye el asistente. Puede crearse sin API key (modo degradado).
func New(cfg Config, log *slog.Logger, history *repo.History, users *repo.Users, ragSvc *rag.Service, appts AppointmentRecorder, prompts *repo.Prompts, services *repo.Services, instructions *repo.Instructions) *Assistant {
	a := &Assistant{
		cfg:     cfg,
		log:     log,
		prompts: NewPromptFactory(prompts, services, instructions, log),
		rag:     ragSvc,
		history: history,
		users:   users,
		appts:   appts,
	}
	if cfg.APIKey != "" {
		a.client = openai.NewClient(cfg.APIKey)
	}
	a.tools = buildTools()
	return a
}

// ReloadContent invalida la instantánea de prompts/servicios del asistente.
func (a *Assistant) ReloadContent(ctx context.Context) error {
	return a.prompts.Reload(ctx)
}

// ReindexPrompt actualiza (o elimina) el embedding RAG del prompt indicado.
func (a *Assistant) ReindexPrompt(ctx context.Context, code string) error {
	if a.rag == nil || a.client == nil || !strings.HasPrefix(code, "prompt_") {
		return nil
	}
	if IsCompanyTopic(code) {
		// Se inyecta siempre en el system prompt: no necesita embedding.
		return a.rag.Delete(ctx, code)
	}
	p, err := a.prompts.GetPrompt(ctx, code)
	if err != nil || !p.Active || strings.TrimSpace(p.Content) == "" {
		return a.rag.Delete(ctx, code)
	}
	vec, err := a.rag.Embed(ctx, p.Content)
	if err != nil {
		return err
	}
	return a.rag.Upsert(ctx, code, code, p.Content, vec)
}

// DeletePromptEmbedding elimina el embedding RAG de un prompt borrado.
func (a *Assistant) DeletePromptEmbedding(ctx context.Context, code string) error {
	if a.rag == nil {
		return nil
	}
	return a.rag.Delete(ctx, code)
}

// Reply genera la respuesta para un mensaje entrante.
func (a *Assistant) Reply(ctx context.Context, req pipeline.ReplyRequest) (pipeline.Reply, error) {
	lang := i18n.Detect(req.Text)
	reply := pipeline.Reply{Language: lang, PromptContext: req.User.Topic, Model: a.cfg.Model}

	// Detección de tema por similitud semántica.
	if a.rag != nil && strings.TrimSpace(req.Text) != "" {
		code, score, err := a.rag.MatchTopic(ctx, req.Text)
		if err != nil {
			a.log.Warn("rag: error detectando tema", "error", err.Error())
		} else if code != "" && IsCompanyTopic(code) {
			// Los datos de empresa ya viajan siempre en el system prompt.
			code = ""
		} else if code != "" && code != req.User.Topic {
			req.User.LastTopic = req.User.Topic
			req.User.Topic = code
			reply.PromptContext = code
			if err := a.users.SetTopic(ctx, req.User.ID, code); err != nil {
				a.log.Warn("no se pudo persistir el tema", "error", err.Error())
			}
			a.log.Info("tema detectado", "chat", req.User.ID, "topic", code, "score", score)
		}
	}

	if a.client == nil {
		reply.Text = "*Asistente Virtual*\n\n(IA no configurada) He recibido tu mensaje."
		return reply, nil
	}

	contextStr := buildContext(a.prompts, req.User, req.IsNew, "")
	history, err := a.history.Window(ctx, req.User.ID, 20)
	if err != nil {
		a.log.Warn("no se pudo leer el historial", "error", err.Error())
	}

	instructions := a.prompts.Compile(ctx)
	input := a.buildInput(contextStr, history, req.Text)
	text, actions, attachments, toolCalls, usage, err := a.complete(ctx, req.User.ID, instructions, input)
	if err != nil {
		return reply, err
	}

	text, language, satisfaction := parseStructuredReply(text)
	reply.Text = text
	if language != "" {
		reply.Language = language
	}
	if satisfaction > 0 {
		reply.Satisfaction = satisfaction
	}
	reply.Actions = actions
	reply.Attachments = attachments
	reply.ToolCalls = toolCalls
	reply.PromptTokens = usage.PromptTokens
	reply.CompletionTokens = usage.CompletionTokens
	reply.TotalTokens = usage.TotalTokens
	return reply, nil
}

type structuredReply struct {
	Respond           *string `json:"respond"`
	LanguageCode      string  `json:"language_code"`
	SatisfactionLevel float64 `json:"satisfaction_level"`
}

// parseStructuredReply extrae el campo respond cuando el modelo devuelve el
// esquema JSON definido en prompts/schema_response.md. Si no es JSON válido,
// devuelve el texto original.
func parseStructuredReply(raw string) (string, string, float64) {
	trimmed := strings.TrimSpace(raw)
	start := strings.Index(trimmed, "{")
	end := strings.LastIndex(trimmed, "}")
	if start == -1 || end <= start {
		return raw, "", 0
	}
	var sr structuredReply
	if err := json.Unmarshal([]byte(trimmed[start:end+1]), &sr); err != nil || sr.Respond == nil {
		return raw, "", 0
	}
	return strings.TrimSpace(*sr.Respond), sr.LanguageCode, sr.SatisfactionLevel
}

// Transcribe convierte un audio en texto con Whisper.
func (a *Assistant) Transcribe(ctx context.Context, audio []byte) (string, error) {
	if a.client == nil {
		return "", nil
	}
	resp, err := a.client.CreateTranscription(ctx, openai.AudioRequest{
		Model:    openai.Whisper1,
		Reader:   bytes.NewReader(audio),
		FilePath: "audio.ogg",
	})
	if err != nil {
		return "", fmt.Errorf("assistant: transcripción: %w", err)
	}
	return resp.Text, nil
}

func (a *Assistant) buildInput(contextStr string, history []domain.Turn, text string) []any {
	input := make([]any, 0, len(history)+1)
	for _, t := range history {
		role := openai.ChatMessageRoleUser
		if t.Role == "assistant" {
			role = openai.ChatMessageRoleAssistant
		}
		input = append(input, openai.ResponseInputMessage{Type: "message", Role: role, Content: t.Content})
	}
	input = append(input, openai.ResponseInputMessage{
		Type:    "message",
		Role:    openai.ChatMessageRoleUser,
		Content: contextStr + "\n\nUsuario: " + text,
	})
	return input
}

type responseFunctionCall struct {
	ID        string
	Name      string
	Arguments string
}

func extractFunctionCalls(output []any) []responseFunctionCall {
	var calls []responseFunctionCall
	for _, raw := range output {
		data, err := json.Marshal(raw)
		if err != nil {
			continue
		}
		var item openai.ResponseOutputItem
		if err := json.Unmarshal(data, &item); err != nil {
			continue
		}
		if item.Type == "function_call" {
			calls = append(calls, responseFunctionCall{ID: item.CallID, Name: item.Name, Arguments: item.Arguments})
		}
	}
	return calls
}

func (a *Assistant) complete(ctx context.Context, userID, instructions string, input []any) (string, []string, []string, []string, openai.Usage, error) {
	var (
		actions     []string
		attachments []string
		toolNames   []string
		usage       openai.Usage
	)

	for i := 0; i < 5; i++ {
		resp, err := a.client.CreateResponse(ctx, openai.CreateResponseRequest{
			Model:        a.cfg.Model,
			Instructions: instructions,
			Input:        input,
			Tools:        a.tools,
		})
		if err != nil {
			return "", nil, nil, nil, usage, fmt.Errorf("assistant: responses: %w", err)
		}
		if resp.Usage != nil {
			usage = openai.Usage{
				PromptTokens:     resp.Usage.InputTokens,
				CompletionTokens: resp.Usage.OutputTokens,
				TotalTokens:      resp.Usage.TotalTokens,
			}
		}

		calls := extractFunctionCalls(resp.Output)
		if len(calls) == 0 {
			return resp.GetOutputText(), actions, attachments, toolNames, usage, nil
		}

		for _, call := range calls {
			toolNames = append(toolNames, call.Name)
			input = append(input, map[string]any{
				"type":      "function_call",
				"call_id":   call.ID,
				"name":      call.Name,
				"arguments": call.Arguments,
			})
			out := a.execTool(ctx, userID, call.Name, call.Arguments)
			input = append(input, openai.ResponseFunctionCallOutput{
				Type:   "function_call_output",
				CallID: call.ID,
				Output: out.Result,
			})
			if out.Action != "" {
				actions = append(actions, out.Action)
			}
			attachments = append(attachments, out.Attachments...)
		}
	}

	a.log.Warn("se excedió el número de iteraciones de herramientas", "chat", userID)
	return "Disculpa, ¿puedes repetirme tu consulta?", actions, attachments, toolNames, usage, nil
}
