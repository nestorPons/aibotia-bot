package assistant

import (
	"context"
	"encoding/json"
	"fmt"

	openai "github.com/sashabaranov/go-openai"
)

func buildTools() []openai.ResponseTool {
	strProp := func(desc string) map[string]any {
		return map[string]any{"type": "string", "description": desc}
	}
	tool := func(name, desc string, props map[string]any, required []string) openai.ResponseTool {
		params := map[string]any{"type": "object", "properties": props}
		if len(required) > 0 {
			params["required"] = required
		}
		return openai.NewResponseFunctionTool(openai.FunctionDefinition{
			Name:        name,
			Description: desc,
			Parameters:  params,
		})
	}
	return []openai.ResponseTool{
		tool("request_appointment_scheduling",
			"Registra una solicitud de cita para que el equipo revise disponibilidad. Nunca confirma horas ni fechas.",
			map[string]any{
				"code_service": strProp("Código del servicio solicitado"),
				"availability": strProp("Preferencia horaria del cliente (mañana/tarde, días)"),
			}, []string{"code_service"}),
		tool("appointment_tamara",
			"Registra una solicitud de cita con Tamara. Requiere el código del servicio.",
			map[string]any{"code_service": strProp("Código del servicio solicitado")},
			[]string{"code_service"}),
		tool("message_human_request",
			"Deriva la conversación a atención humana del equipo y desactiva la IA para este chat.",
			map[string]any{}, nil),
		tool("fix_chat",
			"Marca la conversación como pendiente para el equipo (desactiva la IA y la fija).",
			map[string]any{}, nil),
		tool("notify_delay",
			"Notifica al equipo un retraso; pide al cliente llamar por teléfono.",
			map[string]any{}, nil),
		tool("send_location",
			"Envía la ubicación del Studio al cliente.",
			map[string]any{}, nil),
		tool("send_attachment",
			"Envía un archivo adjunto al cliente indicando su ruta relativa.",
			map[string]any{"path": strProp("Ruta relativa del archivo a enviar")},
			[]string{"path"}),
	}
}

type toolOutcome struct {
	Result      string
	Action      string
	Attachments []string
}

func (a *Assistant) execTool(ctx context.Context, userID, name, arguments string) toolOutcome {
	var args struct {
		CodeService  string `json:"code_service"`
		Availability string `json:"availability"`
		Path         string `json:"path"`
	}
	if arguments != "" {
		_ = json.Unmarshal([]byte(arguments), &args)
	}

	switch name {
	case "request_appointment_scheduling":
		if a.appts != nil {
			if err := a.appts.RecordRequest(ctx, userID, args.CodeService, args.Availability, false); err != nil {
				a.log.Warn("no se pudo registrar la solicitud de cita", "error", err.Error())
			}
		}
		return toolOutcome{Result: "Solicitud de cita registrada para revisión del equipo.", Action: "request_appointment_scheduling"}

	case "appointment_tamara":
		if args.CodeService == "" {
			return toolOutcome{Result: "Falta el servicio para la cita con Tamara."}
		}
		if a.appts != nil {
			if err := a.appts.RecordRequest(ctx, userID, args.CodeService, "", true); err != nil {
				a.log.Warn("no se pudo registrar la cita con Tamara", "error", err.Error())
			}
		}
		return toolOutcome{Result: "Solicitud de cita con Tamara registrada.", Action: "appointment_tamara"}

	case "message_human_request":
		return toolOutcome{Result: "Conversación derivada a atención humana.", Action: "message_human_request"}

	case "fix_chat":
		return toolOutcome{Result: "Conversación marcada como pendiente.", Action: "fix_chat"}

	case "notify_delay":
		return toolOutcome{Result: "Se notificará el retraso al equipo.", Action: "notify_delay"}

	case "send_location":
		return toolOutcome{Result: "Ubicación solicitada.", Action: "send_location"}

	case "send_attachment":
		if args.Path == "" {
			return toolOutcome{Result: "Falta la ruta del adjunto."}
		}
		return toolOutcome{Result: "Adjunto preparado.", Action: "send_attachment", Attachments: []string{args.Path}}

	default:
		return toolOutcome{Result: fmt.Sprintf("Herramienta desconocida: %s", name)}
	}
}
