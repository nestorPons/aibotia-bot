-- +goose Up
-- Borrado total de usuario: las interacciones y las citas caen en cascada.
-- Primero se limpian filas huérfanas de borrados anteriores.
DELETE FROM ai_interactions i
WHERE i.user_id IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM users u WHERE u.id = i.user_id);

DELETE FROM appointments a
WHERE NOT EXISTS (SELECT 1 FROM users u WHERE u.id = a.user_id);

ALTER TABLE ai_interactions
    ADD CONSTRAINT ai_interactions_user_fk
    FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE;

ALTER TABLE appointments
    ADD CONSTRAINT appointments_user_fk
    FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE;

CREATE INDEX IF NOT EXISTS idx_ai_interactions_session_id ON ai_interactions (session_id);

-- +goose Down
DROP INDEX IF EXISTS idx_ai_interactions_session_id;
ALTER TABLE appointments DROP CONSTRAINT IF EXISTS appointments_user_fk;
ALTER TABLE ai_interactions DROP CONSTRAINT IF EXISTS ai_interactions_user_fk;
