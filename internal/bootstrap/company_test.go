package bootstrap

import (
	"strings"
	"testing"
)

func TestMergeCompanyKeepsLongest(t *testing.T) {
	short := "Dirección: Calle 1"
	long := "Dirección: Calle 1\nAcceso: timbre"

	if got := mergeCompany(short, long); got != long {
		t.Fatalf("mergeCompany(corto, largo) = %q", got)
	}
	if got := mergeCompany(long, short); got != long {
		t.Fatalf("mergeCompany(largo, corto) = %q", got)
	}
}

func TestMergeCompanyConcatenatesDifferent(t *testing.T) {
	got := mergeCompany("Uno", "Dos")
	if !strings.Contains(got, "Uno") || !strings.Contains(got, "Dos") {
		t.Fatalf("mergeCompany = %q", got)
	}
}

func TestEnsurePin(t *testing.T) {
	got := ensurePin("Dirección: Calle 1")
	if !strings.HasPrefix(got, "<!-- pin:") {
		t.Fatalf("sin pin: %q", got)
	}
	if again := ensurePin(got); again != got {
		t.Fatalf("ensurePin no es idempotente:\n%s\n%s", got, again)
	}
}
