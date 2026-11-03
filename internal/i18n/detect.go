// Package i18n detecta el idioma de un texto de forma heurística.
package i18n

import (
	"regexp"
	"strings"
)

var (
	reSpanish    = regexp.MustCompile(`\b(hola|buenos d[ií]as|buenas tardes|buenas noches|qu[eé] tal|gracias|por favor|cita|precio|servicio|cejas|labios|pesta[nñ]as)\b`)
	reEnglish    = regexp.MustCompile(`\b(hi|hello|good morning|good afternoon|good evening|hey|thanks|thank you|appointment|price|service|eyebrows|lips|lashes)\b`)
	rePortuguese = regexp.MustCompile(`\b(ol[aá]|bom dia|boa tarde|obrigad[oa]|agendamento|pre[cç]o|sobrancelha)\b`)
)

// Detect devuelve el código de idioma ("es", "en", "pt"); por defecto "es".
func Detect(text string) string {
	t := strings.ToLower(strings.TrimSpace(text))
	if t == "" {
		return "es"
	}
	switch {
	case reSpanish.MatchString(t):
		return "es"
	case reEnglish.MatchString(t):
		return "en"
	case rePortuguese.MatchString(t):
		return "pt"
	default:
		return "es"
	}
}
