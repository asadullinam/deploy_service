CREATE TABLE resource_usage (
  id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES projects (id),
  period_start TIMESTAMPTZ NOT NULL,
  period_end TIMESTAMPTZ NOT NULL,
  cpu_core_hours FLOAT NOT NULL DEFAULT 0,
  memory_gb_hours FLOAT NOT NULL DEFAULT 0,
  storage_gb FLOAT NOT NULL DEFAULT 0,
  egress_gb FLOAT NOT NULL DEFAULT 0,
  recorded_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_resource_usage_project_period ON resource_usage (project_id, period_start);
