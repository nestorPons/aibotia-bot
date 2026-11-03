package httpapi

import (
	"testing"
	"time"
)

func TestAdminSessionRoundTrip(t *testing.T) {
	s := newAdminSession(AdminOptions{Password: "secret", SessionTTL: time.Hour})
	now := time.Now()

	cookie := s.issue(now, "Lledó")
	if !s.valid(cookie, now.Add(30*time.Minute)) {
		t.Fatal("la sesión debería ser válida dentro del TTL")
	}
	if name, ok := s.parse(cookie, now.Add(30*time.Minute)); !ok || name != "Lledó" {
		t.Fatalf("el nombre del operador debería conservarse, got %q ok=%v", name, ok)
	}
	if s.valid(cookie, now.Add(2*time.Hour)) {
		t.Fatal("la sesión debería caducar")
	}
	if s.valid(cookie+"x", now) {
		t.Fatal("una firma manipulada no debe ser válida")
	}
}

func TestAdminSessionPassword(t *testing.T) {
	s := newAdminSession(AdminOptions{Password: "secret"})
	if !s.checkPassword("secret") {
		t.Fatal("la contraseña correcta debe validar")
	}
	if s.checkPassword("secre") || s.checkPassword("") {
		t.Fatal("una contraseña incorrecta no debe validar")
	}
}

func TestTitleCase(t *testing.T) {
	cases := map[string]string{
		"lledó":        "Lledó",
		"maría josé":   "María José",
		" juan  perez": " Juan  Perez",
		"":             "",
	}
	for in, want := range cases {
		if got := titleCase(in); got != want {
			t.Errorf("titleCase(%q) = %q, want %q", in, got, want)
		}
	}
}
