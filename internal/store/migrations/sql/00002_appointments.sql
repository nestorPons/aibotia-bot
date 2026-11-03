-- +goose Up
CREATE TABLE appointments (
    id              BIGSERIAL PRIMARY KEY,
    user_id         TEXT NOT NULL,
    service         TEXT NOT NULL DEFAULT '',
    availability    TEXT NOT NULL DEFAULT '',
    team            BOOLEAN NOT NULL DEFAULT TRUE,
    status          TEXT NOT NULL DEFAULT 'pending_service',
    scheduled_at    TIMESTAMPTZ,
    reminder_sent   BOOLEAN NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT appointments_status_check
        CHECK (status IN ('idle','pending_service','pending_availability','ready','finish','cancel'))
);
CREATE INDEX idx_appointments_user ON appointments (user_id, created_at DESC);
CREATE INDEX idx_appointments_pending_reminder
    ON appointments (scheduled_at)
    WHERE reminder_sent = FALSE AND status = 'ready';

-- +goose Down
DROP TABLE IF EXISTS appointments;
