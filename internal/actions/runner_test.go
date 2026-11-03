package actions

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/aibotia/aibotia-go/internal/domain"
	"github.com/aibotia/aibotia-go/internal/pipeline"
	"github.com/aibotia/aibotia-go/internal/whatsapp"
)

func TestParsePin(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		wantLat  float64
		wantLng  float64
		wantName string
		wantNil  bool
	}{
		{
			name:     "completo",
			content:  "<!-- pin: 39.4555556,-0.3583096 | Studio Tamara Freitas | Av. Instituto Obrero, 21 -->\nresto",
			wantLat:  39.4555556,
			wantLng:  -0.3583096,
			wantName: "Studio Tamara Freitas",
		},
		{
			name:     "solo coordenadas",
			content:  "<!--pin: 39.45 , -0.35-->",
			wantLat:  39.45,
			wantLng:  -0.35,
			wantName: "Ubicación",
		},
		{
			name:    "sin pin",
			content: "# DATOS DE EMPRESA\nsin coordenadas",
			wantNil: true,
		},
		{
			name:    "pin inválido",
			content: "<!-- pin: foo,bar -->",
			wantNil: true,
		},
		{
			name:    "cero cero",
			content: "<!-- pin: 0,0 -->",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParsePin(tt.content)
			if tt.wantNil {
				if got != nil {
					t.Fatalf("ParsePin() = %+v, quiero nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("ParsePin() = nil, quiero ubicación")
			}
			if got.Lat != tt.wantLat || got.Lng != tt.wantLng {
				t.Fatalf("coordenadas = %v,%v; quiero %v,%v", got.Lat, got.Lng, tt.wantLat, tt.wantLng)
			}
			if got.Name != tt.wantName {
				t.Fatalf("name = %q; quiero %q", got.Name, tt.wantName)
			}
		})
	}
}

func TestSetLocationThreadSafe(t *testing.T) {
	r := NewRunner(nil, nil, nil, nil)
	if r.Location() != nil {
		t.Fatal("esperaba ubicación nil")
	}
	r.SetLocation(&Location{Lat: 1, Lng: 2})
	got := r.Location()
	if got == nil || got.Lat != 1 || got.Lng != 2 {
		t.Fatalf("Location() = %+v", got)
	}
	if got.Name != "Ubicación" {
		t.Fatalf("name por defecto = %q", got.Name)
	}
}

// stubProvider simula WhatsApp; sólo implementa los métodos que usa el runner.
type stubProvider struct {
	whatsapp.Provider
	failLocation bool
	locations    int
	texts        []string
}

func (s *stubProvider) SendLocation(context.Context, string, float64, float64, string, string) (string, error) {
	if s.failLocation {
		return "", errors.New("pin rechazado")
	}
	s.locations++
	return "loc-1", nil
}

func (s *stubProvider) SendText(_ context.Context, _, text string) (string, error) {
	s.texts = append(s.texts, text)
	return "txt-1", nil
}

func TestSendLocationAction(t *testing.T) {
	p := &stubProvider{}
	r := NewRunner(slog.New(slog.NewTextHandler(io.Discard, nil)), nil, p, &Location{Lat: 39.4, Lng: -0.3, Name: "Studio", Address: "Calle 21"})
	if err := r.Run(context.Background(), domain.User{ID: "u1"}, &pipeline.Reply{Actions: []string{"send_location"}}); err != nil {
		t.Fatal(err)
	}
	if p.locations != 1 || len(p.texts) != 0 {
		t.Fatalf("locations=%d texts=%v", p.locations, p.texts)
	}
}

func TestSendLocationFallback(t *testing.T) {
	p := &stubProvider{failLocation: true}
	r := NewRunner(slog.New(slog.NewTextHandler(io.Discard, nil)), nil, p, &Location{Lat: 39.4, Lng: -0.3, Name: "Studio", Address: "Calle 21"})
	if err := r.Run(context.Background(), domain.User{ID: "u1"}, &pipeline.Reply{Actions: []string{"send_location"}}); err != nil {
		t.Fatal(err)
	}
	if len(p.texts) != 1 || !strings.Contains(p.texts[0], "Calle 21") {
		t.Fatalf("fallback = %v", p.texts)
	}
}

// stubActivator registra las activaciones/desactivaciones de la IA.
type stubActivator struct {
	deactivated []string
	activated   []string
}

func (s *stubActivator) SetActive(_ context.Context, id string, active bool) error {
	if active {
		s.activated = append(s.activated, id)
	} else {
		s.deactivated = append(s.deactivated, id)
	}
	return nil
}

func testRunner(users UserActivator) *Runner {
	return NewRunner(slog.New(slog.NewTextHandler(io.Discard, nil)), users, nil, nil)
}

func TestHandoffNormalDesactivaIA(t *testing.T) {
	users := &stubActivator{}
	reply := &pipeline.Reply{Text: "Te paso con una persona.", Actions: []string{"message_human_request"}}
	if err := testRunner(users).Run(context.Background(), domain.User{ID: "u1"}, reply); err != nil {
		t.Fatal(err)
	}
	if len(users.deactivated) != 1 || users.deactivated[0] != "u1" {
		t.Fatalf("desactivados = %v", users.deactivated)
	}
	if strings.Contains(reply.Text, "Nota de prueba") {
		t.Fatalf("un usuario normal no debe llevar nota de prueba: %q", reply.Text)
	}
}

func TestHandoffEnModoPruebaNoDesactiva(t *testing.T) {
	users := &stubActivator{}
	reply := &pipeline.Reply{Text: "Te paso con una persona.", Actions: []string{"message_human_request"}}
	user := domain.User{ID: "u1", TestMode: true}
	if err := testRunner(users).Run(context.Background(), user, reply); err != nil {
		t.Fatal(err)
	}
	if len(users.deactivated) != 0 {
		t.Fatalf("en modo prueba no debe desactivar: %v", users.deactivated)
	}
	if !strings.Contains(reply.Text, TestModeHandoffNote) {
		t.Fatalf("falta la nota de prueba: %q", reply.Text)
	}
	if !strings.HasPrefix(reply.Text, "Te paso con una persona.\n\n") {
		t.Fatalf("la nota debe ir después del texto: %q", reply.Text)
	}
}

func TestFixChatEnModoPruebaMantieneIA(t *testing.T) {
	users := &stubActivator{}
	reply := &pipeline.Reply{Text: "Hola", Actions: []string{"fix_chat"}}
	user := domain.User{ID: "u1", TestMode: true}
	if err := testRunner(users).Run(context.Background(), user, reply); err != nil {
		t.Fatal(err)
	}
	if len(users.deactivated) != 0 {
		t.Fatalf("en modo prueba no debe desactivar: %v", users.deactivated)
	}
	if !strings.HasPrefix(reply.Text, "📌 ") || !strings.Contains(reply.Text, TestModeHandoffNote) {
		t.Fatalf("respuesta = %q", reply.Text)
	}
}

func TestNotaModoPruebaNoSeDuplica(t *testing.T) {
	reply := &pipeline.Reply{
		Text:    "Hola",
		Actions: []string{"message_human_request", "fix_chat"},
	}
	user := domain.User{ID: "u1", TestMode: true}
	if err := testRunner(&stubActivator{}).Run(context.Background(), user, reply); err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(reply.Text, TestModeHandoffNote); got != 1 {
		t.Fatalf("nota repetida %d veces: %q", got, reply.Text)
	}
}
