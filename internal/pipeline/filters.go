package pipeline

import "strings"

// Watermark es el carácter invisible que marca los mensajes del bot para no
// reprocesarlos (evita bucles).
const Watermark = "\u200B"

// BotPrefix identifica visualmente los mensajes generados por el bot.
const BotPrefix = "*Asistente Virtual*"

// withBotPrefix antepone el título del bot si el texto aún no lo lleva.
func withBotPrefix(text string) string {
	if strings.TrimSpace(text) == "" || strings.HasPrefix(text, BotPrefix) {
		return text
	}
	return BotPrefix + "\n\n" + text
}

func isGroupOrBroadcast(chat string) bool {
	return strings.HasSuffix(chat, "@g.us") ||
		strings.HasSuffix(chat, "@broadcast") ||
		chat == "status@broadcast"
}

func containsWatermark(s string) bool {
	return strings.Contains(s, Watermark)
}

func phoneFromJID(jid string) string {
	user := jid
	if i := strings.IndexByte(user, '@'); i >= 0 {
		user = user[:i]
	}
	user = strings.TrimPrefix(user, "34")
	return user
}
