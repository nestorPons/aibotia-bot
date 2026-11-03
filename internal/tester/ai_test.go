package tester

import (
	"strings"
	"testing"

	openai "github.com/sashabaranov/go-openai"
)

func testCase() Case {
	return Case{
		ID:      "clienta_habitual",
		Name:    "Clienta habitual",
		Persona: "Clienta de 38 años que ya conoce el studio.",
		Goal:    "Pedir hora para el repaso.",
		Tone:    "cercano",
		Questions: []string{
			"¿El repaso tiene coste?",
			"¿Me mandas la dirección?",
		},
		Must:    []string{"usar el contexto previo"},
		MustNot: []string{"tratarla como nueva"},
		Finish:  "Cerrar con una cita.",
	}
}

func TestDialogIncludesBotRepliesAsUserTurns(t *testing.T) {
	c := testCase()
	history := []Turn{
		{
			Question: "Hola, vengo del mes pasado. ¿Cuándo puedo volver?",
			Replies:  []Reply{{Text: "Hola Marta 😊"}, {Text: "El repaso cuesta 40 €."}},
		},
	}

	msgs := buildDialogMessages(c, history, nil, "")

	if len(msgs) != 3 {
		t.Fatalf("mensajes = %d, quiero 3 (sistema + cliente + bot)", len(msgs))
	}
	if msgs[0].Role != openai.ChatMessageRoleSystem {
		t.Fatalf("el primero debe ser system: %q", msgs[0].Role)
	}
	if !strings.Contains(msgs[0].Content, c.Goal) {
		t.Fatalf("el sistema debe incluir el objetivo del caso:\n%s", msgs[0].Content)
	}
	if msgs[1].Role != openai.ChatMessageRoleAssistant || msgs[1].Content != history[0].Question {
		t.Fatalf("el mensaje del cliente debe ser assistant: %+v", msgs[1])
	}
	if msgs[2].Role != openai.ChatMessageRoleUser {
		t.Fatalf("la respuesta del bot debe ser un turno user: %q", msgs[2].Role)
	}
	if !strings.Contains(msgs[2].Content, "El repaso cuesta 40 €.") ||
		!strings.Contains(msgs[2].Content, "Hola Marta") {
		t.Fatalf("el turno del bot debe incluir todas sus burbujas: %q", msgs[2].Content)
	}
}

func TestDialogEndsOnBotReplySoClientReacts(t *testing.T) {
	history := []Turn{
		{Question: "¿Cuánto cuesta?", Replies: []Reply{{Text: "280 €."}}},
		{Question: "¿Y el repaso?", Replies: []Reply{{Text: "40 €."}}},
	}
	msgs := buildDialogMessages(testCase(), history, nil, "")
	last := msgs[len(msgs)-1]
	if last.Role != openai.ChatMessageRoleUser {
		t.Fatalf("el último turno debe ser del bot para que el cliente reaccione: %q", last.Role)
	}
	if !strings.Contains(last.Content, "40 €") {
		t.Fatalf("el último turno debe ser la última respuesta del bot: %q", last.Content)
	}
}

func TestDialogMarksMissingBotReply(t *testing.T) {
	history := []Turn{{Question: "¿Estáis abiertos el domingo?", Replies: nil}}
	msgs := buildDialogMessages(testCase(), history, nil, "")
	last := msgs[len(msgs)-1]
	if !strings.Contains(strings.ToLower(last.Content), "no respondió") {
		t.Fatalf("debe indicar que el bot no respondió: %q", last.Content)
	}
}

func TestDialogSendsCaseTopicsAsHintsNotScript(t *testing.T) {
	c := testCase()
	msgs := buildDialogMessages(c, nil, []string{"Hola, ¿cuánto cuesta?"}, "")
	sys := msgs[0].Content
	if !strings.Contains(sys, "Temas sugeridos") || !strings.Contains(sys, "no es un guion") {
		t.Fatalf("los temas del caso deben ser sugerencias, no un guion:\n%s", sys)
	}
	for _, q := range c.Questions {
		if !strings.Contains(sys, q) {
			t.Fatalf("falta el tema %q en el sistema", q)
		}
	}
	if !strings.Contains(sys, "NO repetir") {
		t.Fatalf("debe prohibir repetir mensajes ya enviados:\n%s", sys)
	}
}

func TestDialogAppendsRetryNote(t *testing.T) {
	note := "Reacciona a la última respuesta del bot."
	sys := buildDialogMessages(testCase(), nil, nil, note)[0].Content
	if !strings.Contains(sys, note) {
		t.Fatalf("el sistema debe incluir el aviso de reintento:\n%s", sys)
	}
}

func TestCleanTextStripsRolePrefix(t *testing.T) {
	if got := cleanText(`"Cliente: ¿Y el repaso?"`); got != "¿Y el repaso?" {
		t.Fatalf("cleanText = %q", got)
	}
}

func TestClarifyNoteWhenBotAsks(t *testing.T) {
	history := []Turn{{
		Question: "En la story vi un 2x1, ¿me lo confirmas?",
		Replies:  []Reply{{Text: "No tengo información de esa promoción. ¿A qué tratamiento se refería la story?"}},
	}}
	note := clarifyNote(history)
	if !strings.Contains(note, "tratamiento") || !strings.Contains(note, "Respóndele") {
		t.Fatalf("clarifyNote = %q", note)
	}
}

func TestClarifyNoteIgnoredWhenBotDoesNotAsk(t *testing.T) {
	history := []Turn{{
		Question: "¿Cuánto cuesta?",
		Replies:  []Reply{{Text: "La micropigmentación cuesta 280 €."}},
	}}
	if note := clarifyNote(history); note != "" {
		t.Fatalf("no debería haber aviso de aclaración: %q", note)
	}
	if note := clarifyNote(nil); note != "" {
		t.Fatalf("historial vacío: %q", note)
	}
}

func TestLastBotQuestionTakesFinalBubble(t *testing.T) {
	history := []Turn{{
		Replies: []Reply{
			{Text: "¡Hola! Soy Carla."},
			{Text: "¿Te refieres a cejas o a labios?"},
		},
	}}
	if got := lastBotQuestion(history); !strings.Contains(got, "cejas o a labios") {
		t.Fatalf("lastBotQuestion = %q", got)
	}
	noQuestion := []Turn{{Replies: []Reply{{Text: "El precio es 280 €."}}}}
	if got := lastBotQuestion(noQuestion); got != "" {
		t.Fatalf("no hay pregunta: %q", got)
	}
}

func TestRolePromptExigeResponderAclaraciones(t *testing.T) {
	if !strings.Contains(roleSystemPrompt, "aclaración") {
		t.Fatal("el prompt del cliente debe cubrir las aclaraciones del bot")
	}
}
