CREATE TABLE IF NOT EXISTS yookassa_payments (
    id            TEXT PRIMARY KEY,
    yookassa_id   TEXT NOT NULL UNIQUE,
    user_id       TEXT NOT NULL,
    amount_rub    DOUBLE PRECISION NOT NULL DEFAULT 0,
    amount_usd    DOUBLE PRECISION NOT NULL DEFAULT 0,
    status        TEXT NOT NULL DEFAULT 'pending',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS yookassa_payments_user_id_idx ON yookassa_payments (user_id);
CREATE INDEX IF NOT EXISTS yookassa_payments_yookassa_id_idx ON yookassa_payments (yookassa_id);
