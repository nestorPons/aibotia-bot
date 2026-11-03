package repo

import "testing"

func TestNeedsOperator(t *testing.T) {
	cases := []struct {
		calls []string
		want  bool
	}{
		{[]string{"request_appointment_scheduling"}, true},
		{[]string{"send_location", "appointment_tamara"}, true},
		{[]string{"fix_chat"}, true},
		{[]string{"message_human_request"}, true},
		{[]string{"send_location"}, false},
		{nil, false},
	}
	for _, c := range cases {
		if got := NeedsOperator(c.calls); got != c.want {
			t.Fatalf("NeedsOperator(%v) = %v, quiero %v", c.calls, got, c.want)
		}
	}
}

func TestHandoffReasonPriority(t *testing.T) {
	got := HandoffReason([]string{"fix_chat", "request_appointment_scheduling"})
	if got != "Solicitud de cita pendiente de confirmar disponibilidad" {
		t.Fatalf("motivo = %q", got)
	}
	if got := HandoffReason([]string{"send_location"}); got != "" {
		t.Fatalf("motivo inesperado = %q", got)
	}
}
