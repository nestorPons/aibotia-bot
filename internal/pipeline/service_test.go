package pipeline

import "testing"

func TestNormalizeBuffered(t *testing.T) {
	msgs := []BufferedMessage{
		{Text: "Hola"},
		{Text: "gracias"},
		{Text: "quiero cita"},
	}
	text, hasMedia, _ := normalizeBuffered(msgs)
	if text != "Hola\nquiero cita" {
		t.Fatalf("texto = %q", text)
	}
	if hasMedia {
		t.Fatal("hasMedia debería ser false")
	}
}

func TestNormalizeBufferedMedia(t *testing.T) {
	msgs := []BufferedMessage{
		{Text: "mira", HasMedia: true, MediaType: "image"},
	}
	_, hasMedia, mt := normalizeBuffered(msgs)
	if !hasMedia || mt != "image" {
		t.Fatalf("media = %v/%q, quiero true/image", hasMedia, mt)
	}
}

func TestIsGroupOrBroadcast(t *testing.T) {
	if !isGroupOrBroadcast("123@g.us") {
		t.Fatal("grupo no detectado")
	}
	if isGroupOrBroadcast("34600@s.whatsapp.net") {
		t.Fatal("chat privado mal clasificado")
	}
}

func TestPhoneFromJID(t *testing.T) {
	if got := phoneFromJID("34600111222@s.whatsapp.net"); got != "600111222" {
		t.Fatalf("phoneFromJID = %q", got)
	}
}
