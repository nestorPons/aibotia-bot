package landing

import "time"

func SiteData() Site {
	return Site{
		Name:        "AIBOT",
		Tagline:     "Automatiza tu atención al cliente en WhatsApp",
		Email:       "info@nestorpons.com",
		Phone:       "686 115 433",
		WhatsApp:    "34686115433",
		WhatsAppURL: "https://chat.whatsapp.com/EeZlbPsT7KWBjo8sWRQpBe",
		Location:    "12002 Castellón",
		GitHub:      "https://github.com/nestorPons/wwebjs-bot",
		Web:         "https://nestorpons.com",
		LegalNote:   "AIBOT es un producto de Néstor Pons",
	}
}

func HomePage() PageData {
	site := SiteData()
	return PageData{
		Title:       "AIBOT · Chatbot para WhatsApp",
		Description: "AIBOT responde preguntas, agenda citas y envía recordatorios por WhatsApp. Un sistema de IA instalado en tu propia infraestructura.",
		Canonical:   "https://aibotia.com/",
		Path:        "/",
		Year:        time.Now().Year(),
		Site:        site,
		Features: []Feature{
			{
				Icon:  "chat",
				Title: "Responde consultas",
				Text:  "Automatiza respuestas sobre ubicación, horarios, servicios y cualquier pregunta frecuente de tu negocio.",
			},
			{
				Icon:  "calendar",
				Title: "Agenda citas",
				Text:  "Permite a tus clientes reservar cita sin necesidad de un operador, disponible las 24 horas.",
			},
			{
				Icon:  "bell",
				Title: "Envía recordatorios",
				Text:  "Reduce las ausencias con alertas automáticas antes de cada cita y libera a tu equipo de tareas repetitivas.",
			},
		},
		Plans: []Plan{
			{
				Eyebrow:     "Plan de servicio único",
				Name:        "Instalación personalizada",
				Description: "Incluye la configuración del servidor AWS EC2 o local, la configuración del asistente OpenAI y la carga de los datos de tu negocio.",
				Price:       "Desde 90 €",
				PriceNote:   "* El precio puede variar según la complejidad de la personalización requerida.",
				Features: []string{
					"Configuración del servidor AWS EC2 o local",
					"Configuración del asistente OpenAI",
					"Carga de datos y conocimiento de tu negocio",
					"Puesta en marcha y formación",
				},
				CTA: "Solicitar instalación",
			},
			{
				Eyebrow:     "Servicio recurrente",
				Name:        "Cuota mensual",
				Description: "Mantenimiento, actualizaciones y soporte continuo para que tu asistente siga funcionando sin preocupaciones.",
				Price:       "50 €/mes",
				PriceNote:   "+ Gastos por uso de la API de OpenAI. Precios sin IVA incluido.",
				Features: []string{
					"Mantenimiento y soporte técnico",
					"Actualizaciones del asistente",
					"Supervisión del servicio",
					"Personalización adicional: 25 €/hora",
				},
				Highlight: true,
				CTA:       "Hablar con nosotros",
			},
		},
		Advantages: []Advantage{
			{Icon: "key", Title: "Titularidad", Text: "El sistema es tuyo, lo que te proporciona total control y propiedad."},
			{Icon: "settings", Title: "Adaptación", Text: "El sistema se adapta a tu negocio, no tu negocio al sistema."},
			{Icon: "grid", Title: "Control total", Text: "Gestiona tu sistema de forma autónoma, sin depender de servicios externos para procesar los datos."},
			{Icon: "code", Title: "Personalización profunda", Text: "Incluye tus bases de datos, modelos de IA en local y la integración con otras aplicaciones empresariales."},
			{Icon: "shield", Title: "Privacidad y seguridad", Text: "Los datos se mantienen en tu infraestructura, sin cederlos a terceros."},
			{Icon: "euro", Title: "Costo efectivo", Text: "Un pago único sin tarifas sorpresa, ideal para controlar mejor los gastos a largo plazo."},
		},
		Summary: []SummaryPoint{
			{
				Title: "Integración perfecta",
				Text:  "Se adapta a la infraestructura existente, sin necesidad de ajustes complejos.",
			},
			{
				Title: "Control total y propiedad",
				Text:  "El sistema, desarrollado completamente en código propio, asegura que la instalación y los datos permanezcan bajo tu control.",
			},
			{
				Title: "Flexibilidad",
				Text:  "A medida que tu empresa crece, el sistema es escalable y personalizable para atender nuevas necesidades.",
			},
		},
	}
}
