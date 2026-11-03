package config

import "testing"

func TestLoadValid(t *testing.T) {
	t.Setenv("API_KEY", "api-secret")
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/db?sslmode=disable")
	t.Setenv("OPENAI_API_KEY", "openai-secret")
	t.Setenv("BUFFER_MIN_MS", "1000")
	t.Setenv("BUFFER_MAX_MS", "15000")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error inesperado: %v", err)
	}
	if cfg.APIKey != "api-secret" {
		t.Fatalf("APIKey = %q, quiero %q", cfg.APIKey, "api-secret")
	}
	if cfg.BufferMaxMs != 15000 {
		t.Fatalf("BufferMaxMs = %d, quiero 15000", cfg.BufferMaxMs)
	}
}

func TestLoadRejectsEqualKeys(t *testing.T) {
	t.Setenv("API_KEY", "same-secret")
	t.Setenv("OPENAI_API_KEY", "same-secret")
	t.Setenv("DATABASE_URL", "postgres://x")

	if _, err := Load(); err == nil {
		t.Fatal("Load() debería fallar si API_KEY == OPENAI_API_KEY")
	}
}

func TestLoadRejectsBadBufferRange(t *testing.T) {
	t.Setenv("API_KEY", "api-secret")
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("BUFFER_MIN_MS", "20000")
	t.Setenv("BUFFER_MAX_MS", "1000")

	if _, err := Load(); err == nil {
		t.Fatal("Load() debería fallar si BUFFER_MIN_MS > BUFFER_MAX_MS")
	}
}
