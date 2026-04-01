package postgres

import (
	"context"
	"deploy-service/internal/domain"
	"deploy-service/internal/service"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var _ service.ReleaseStore = (*ReleaseStore)(nil)

func NewReleaseStore(pool *pgxpool.Pool) *ReleaseStore {
	return &ReleaseStore{pool: pool}
}

func (s *ReleaseStore) Create(ctx context.Context, r domain.Release) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO releases (id, project_id, stage_id, commit_sha, commit_message, status, workflow_run_id, image_tag, created_at, updated_at)
         VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		r.ID, r.ProjectID, r.StageID, r.CommitSHA, r.CommitMessage, string(r.Status), r.WorkflowRunID, r.ImageTag, r.CreatedAt, r.UpdatedAt,
	)
	return err
}

func (s *ReleaseStore) GetByID(ctx context.Context, releaseID string) (domain.Release, bool) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, project_id, stage_id, commit_sha, commit_message, status, workflow_run_id, image_tag, created_at, updated_at FROM releases WHERE id = $1`,
		releaseID,
	)
	return scanRelease(row)
}

func (s *ReleaseStore) ListByProject(ctx context.Context, projectID string) []domain.Release {
	rows, err := s.pool.Query(ctx,
		`SELECT id, project_id, stage_id, commit_sha, commit_message, status, workflow_run_id, image_tag, created_at, updated_at FROM releases WHERE project_id = $1 ORDER BY created_at DESC`,
		projectID,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var releases []domain.Release
	for rows.Next() {
		r, ok := scanRelease(rows)
		if ok {
			releases = append(releases, r)
		}
	}
	return releases
}

func (s *ReleaseStore) Update(ctx context.Context, r domain.Release) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE releases
         SET stage_id = $1, commit_sha = $2, commit_message = $3, status = $4, workflow_run_id = $5, image_tag = $6, updated_at = $7
         WHERE id = $8`,
		r.StageID, r.CommitSHA, r.CommitMessage, string(r.Status), r.WorkflowRunID, r.ImageTag, r.UpdatedAt, r.ID,
	)
	return err
}

func (s *ReleaseStore) GetByWorkflowRunID(ctx context.Context, runID int64) (domain.Release, bool) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, project_id, stage_id, commit_sha, commit_message, status, workflow_run_id, image_tag, created_at, updated_at FROM releases WHERE workflow_run_id = $1`,
		runID,
	)
	return scanRelease(row)
}

func scanRelease(row scanner) (domain.Release, bool) {
	var r domain.Release
	var status string
	if err := row.Scan(&r.ID, &r.ProjectID, &r.StageID, &r.CommitSHA, &r.CommitMessage, &status, &r.WorkflowRunID, &r.ImageTag, &r.CreatedAt, &r.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Release{}, false
		}
		return domain.Release{}, false
	}
	r.Status = domain.ReleaseStatus(status)
	return r, true
}
