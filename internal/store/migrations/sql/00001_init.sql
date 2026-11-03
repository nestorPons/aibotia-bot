-- +goose Up
CREATE EXTENSION IF NOT EXISTS vector;

-- Usuarios de WhatsApp (id = JID, p. ej. 34600...@c.us)
CREATE TABLE users (
    id            TEXT PRIMARY KEY,
    name          TEXT NOT NULL DEFAULT '',
    alias         TEXT NOT NULL DEFAULT '',
    active        BOOLEAN NOT NULL DEFAULT FALSE,
    topic         TEXT,
    lang          VARCHAR(5),
    temperature   REAL NOT NULL DEFAULT 0.5,
    prompt_last   TEXT NOT NULL DEFAULT 'welcome',
    buffer_time   INTEGER NOT NULL DEFAULT 30000,
    role          SMALLINT NOT NULL DEFAULT 2,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_users_active ON users (active);

-- Estado global del bot (fila única)
CREATE TABLE settings (
    id          BIGSERIAL PRIMARY KEY,
    is_active   BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO settings (is_active) VALUES (TRUE);

-- Registro de interacciones con la IA
CREATE TABLE ai_interactions (
    id                  BIGSERIAL PRIMARY KEY,
    user_id             TEXT,
    session_id          TEXT,
    user_message        TEXT,
    ai_response         TEXT,
    language_code       VARCHAR(5),
    prompt_context      TEXT,
    tool_calls          JSONB,
    model               TEXT,
    prompt_tokens       INTEGER NOT NULL DEFAULT 0,
    completion_tokens   INTEGER NOT NULL DEFAULT 0,
    total_tokens        INTEGER NOT NULL DEFAULT 0,
    satisfaction_level  REAL,
    latency_ms          INTEGER NOT NULL DEFAULT 0,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_ai_interactions_user_id ON ai_interactions (user_id);
CREATE INDEX idx_ai_interactions_created_at ON ai_interactions (created_at DESC);

-- Bloques de contenido (fix-messages / instrucciones)
CREATE TABLE content_blocks (
    id           BIGSERIAL PRIMARY KEY,
    code         TEXT UNIQUE NOT NULL,
    name         TEXT,
    type         TEXT NOT NULL,
    description  TEXT,
    actions      JSONB,
    attachments  JSONB,
    properties   JSONB,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Traducciones por idioma de los bloques
CREATE TABLE content_translations (
    id          BIGSERIAL PRIMARY KEY,
    block_id    BIGINT NOT NULL REFERENCES content_blocks (id) ON DELETE CASCADE,
    language    VARCHAR(5) NOT NULL,
    content     TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (block_id, language)
);

-- Logs de sistema
CREATE TABLE system_logs (
    id          BIGSERIAL PRIMARY KEY,
    level       TEXT NOT NULL,
    message     TEXT NOT NULL,
    meta        JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Embeddings para el RAG (pgvector)
CREATE TABLE embeddings (
    id          BIGSERIAL PRIMARY KEY,
    code        TEXT UNIQUE NOT NULL,
    topic       TEXT,
    content     TEXT NOT NULL,
    embedding   VECTOR(1536) NOT NULL,
    model       TEXT NOT NULL DEFAULT 'text-embedding-3-small',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_embeddings_embedding ON embeddings USING hnsw (embedding vector_cosine_ops);

-- +goose Down
DROP TABLE IF EXISTS embeddings;
DROP TABLE IF EXISTS system_logs;
DROP TABLE IF EXISTS content_translations;
DROP TABLE IF EXISTS content_blocks;
DROP TABLE IF EXISTS ai_interactions;
DROP TABLE IF EXISTS settings;
DROP TABLE IF EXISTS users;
