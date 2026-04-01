CREATE TABLE IF NOT EXISTS stages (
    id         TEXT PRIMARY KEY,
    project_id TEXT NOT NULL,
    name       TEXT NOT NULL,
    slug       TEXT NOT NULL,
    status     TEXT NOT NULL DEFAULT 'creating',
    public_url TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (project_id, slug)
);
CREATE INDEX IF NOT EXISTS stages_project_id_idx ON stages (project_id);

-- Backfill production stage for existing active projects.
INSERT INTO stages (id, project_id, name, slug, status, public_url, created_at, updated_at)
SELECT 'stage-' || id || '-production', id, 'Production', 'production', 'active', '', created_at, updated_at
FROM projects
WHERE status NOT IN ('deleted', 'deleting')
ON CONFLICT DO NOTHING;
