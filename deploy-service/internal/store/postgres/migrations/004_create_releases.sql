CREATE TABLE releases (
  id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES projects (id),
  commit_sha TEXT NOT NULL,
  commit_message TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL,
  workflow_run_id BIGINT NOT NULL DEFAULT 0,
  image_tag TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_releases_project_id ON releases (project_id);
