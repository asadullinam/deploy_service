CREATE TABLE IF NOT EXISTS billing_transactions (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL,
  project_id TEXT NOT NULL DEFAULT '',
  type TEXT NOT NULL,
  amount_usd DOUBLE PRECISION NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW ()
);

CREATE INDEX IF NOT EXISTS billing_transactions_user_id_idx ON billing_transactions (user_id);

CREATE INDEX IF NOT EXISTS billing_transactions_project_id_idx ON billing_transactions (project_id)
WHERE
  project_id != '';
