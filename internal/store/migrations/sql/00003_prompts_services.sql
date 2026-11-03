-- +goose Up
-- Prompts del asistente: fuente de verdad en runtime (los .md son solo seed/export).
CREATE TABLE prompts (
    id          BIGSERIAL PRIMARY KEY,
    code        TEXT UNIQUE NOT NULL,
    name        TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    content     TEXT NOT NULL DEFAULT '',
    is_active   BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_prompts_active ON prompts (is_active);

-- Historial de versiones de cada prompt (rollback desde el panel).
CREATE TABLE prompt_versions (
    id          BIGSERIAL PRIMARY KEY,
    prompt_id   BIGINT NOT NULL REFERENCES prompts (id) ON DELETE CASCADE,
    name        TEXT NOT NULL DEFAULT '',
    content     TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_prompt_versions_prompt ON prompt_versions (prompt_id, created_at DESC);

-- Catálogo estructurado de servicios (sustituye a data/services.json en runtime).
CREATE TABLE services (
    id           TEXT PRIMARY KEY,
    category     TEXT NOT NULL DEFAULT '',
    name         TEXT NOT NULL DEFAULT '',
    price        NUMERIC(10, 2),
    duration_min INTEGER,
    note         TEXT NOT NULL DEFAULT '',
    providers    JSONB NOT NULL DEFAULT '[]'::jsonb,
    is_active    BOOLEAN NOT NULL DEFAULT TRUE,
    sort_order   INTEGER NOT NULL DEFAULT 0,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_services_category ON services (category, sort_order);

-- +goose Down
DROP TABLE IF EXISTS services;
DROP TABLE IF EXISTS prompt_versions;
DROP TABLE IF EXISTS prompts;
