-- +goose Up
-- Interruptor del panel: activar la IA automáticamente al crear usuarios nuevos
-- (mensajes entrantes, importación de contactos).
ALTER TABLE settings ADD COLUMN IF NOT EXISTS activate_new_users BOOLEAN NOT NULL DEFAULT TRUE;

-- +goose Down
ALTER TABLE settings DROP COLUMN IF EXISTS activate_new_users;
