package assistant

import (
	"fmt"
	"strings"
	"time"

	"github.com/aibotia/aibotia-go/internal/domain"
)

func buildContext(p *PromptFactory, user domain.User, isNew bool, instructions string) string {
	var b strings.Builder

	fmt.Fprintf(&b, "Nombre del cliente: %s\n", user.Alias)
	fmt.Fprintf(&b, "Idioma del cliente: %s\n", user.Lang)
	b.WriteString("Contesta en el idioma del cliente.\n")
	fmt.Fprintf(&b, "Fecha actual: %s\n", time.Now().Format("Monday, 2 January 2006 15:04"))

	if isNew {
		b.WriteString("Es un usuario nuevo: incluye un saludo inicial y el disclaimer.\n")
		b.WriteString(p.Get("message_initial"))
		b.WriteString("\n")
	}
	if user.Topic != "" {
		fmt.Fprintf(&b, "Tema actual de la conversación: %s\n", user.Topic)
		if !IsCompanyTopic(user.Topic) {
			fmt.Fprintf(&b, "Información relevante del servicio actual:\n%s\n", p.Get(user.Topic))
		}
	}
	if strings.TrimSpace(instructions) != "" {
		fmt.Fprintf(&b, "Instrucciones específicas para esta respuesta:\n%s\n", instructions)
	}
	return b.String()
}
