package whatsapp

import (
	"strings"
	"testing"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

func TestMessageText(t *testing.T) {
	cases := []struct {
		name string
		msg  *waE2E.Message
		want string
	}{
		{"nil", nil, ""},
		{"conversation", &waE2E.Message{Conversation: proto.String("hola")}, "hola"},
		{"extended", &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{Text: proto.String("ext")}}, "ext"},
		{"image caption", &waE2E.Message{ImageMessage: &waE2E.ImageMessage{Caption: proto.String("foto")}}, "foto"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := messageText(tc.msg); got != tc.want {
				t.Fatalf("messageText() = %q, quiero %q", got, tc.want)
			}
		})
	}
}

func TestMediaType(t *testing.T) {
	cases := []struct {
		name string
		msg  *waE2E.Message
		want string
	}{
		{"nil", nil, ""},
		{"text", &waE2E.Message{Conversation: proto.String("x")}, ""},
		{"image", &waE2E.Message{ImageMessage: &waE2E.ImageMessage{}}, "image"},
		{"audio", &waE2E.Message{AudioMessage: &waE2E.AudioMessage{}}, "audio"},
		{"document", &waE2E.Message{DocumentMessage: &waE2E.DocumentMessage{}}, "document"},
		{"location", &waE2E.Message{LocationMessage: &waE2E.LocationMessage{}}, "location"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := mediaType(tc.msg); got != tc.want {
				t.Fatalf("mediaType() = %q, quiero %q", got, tc.want)
			}
		})
	}
}

func TestNewDefaults(t *testing.T) {
	c := New(Options{})
	if c.Status() != StatusDisconnected {
		t.Fatalf("status inicial = %q, quiero %q", c.Status(), StatusDisconnected)
	}
	if c.QRCode() != "" {
		t.Fatalf("QR inicial debería estar vacío")
	}
	if cap(c.Incoming()) != 256 {
		t.Fatalf("buffer de entrada = %d, quiero 256", cap(c.Incoming()))
	}
}

func TestLocationText(t *testing.T) {
	msg := &waE2E.LocationMessage{
		DegreesLatitude:  proto.Float64(39.4555556),
		DegreesLongitude: proto.Float64(-0.3583096),
		Name:             proto.String("Studio Tamara Freitas"),
		Address:          proto.String("Av. Instituto Obrero, 21"),
	}
	got := LocationText(msg)
	for _, want := range []string{"Studio Tamara Freitas", "Av. Instituto Obrero, 21", "maps.google.com"} {
		if !strings.Contains(got, want) {
			t.Fatalf("LocationText() = %q, falta %q", got, want)
		}
	}
	if got := LocationText(nil); got != "" {
		t.Fatalf("LocationText(nil) = %q", got)
	}
}
