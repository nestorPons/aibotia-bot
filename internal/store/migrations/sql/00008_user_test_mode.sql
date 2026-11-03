-- +goose Up
-- Modo prueba por usuario: durante las pruebas, el bot no desactiva la IA al
-- derivar a un operador (para que la conversación de test continúe).
ALTER TABLE users ADD COLUMN IF NOT EXISTS test_mode BOOLEAN NOT NULL DEFAULT FALSE;

-- +goose Down
ALTER TABLE users DROP COLUMN IF EXISTS test_mode;
