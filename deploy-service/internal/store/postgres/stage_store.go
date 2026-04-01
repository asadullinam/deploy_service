package postgres

import (
	"context"
	"deploy-service/internal/domain"
	"deploy-service/internal/service"
	"github.com/jackc/pgx/v5/pgxpool"
)

var _ service.StageStore = (*StageStore)(nil)

func NewStageStore(pool *pgxpool.Pool) *StageStore {
	return &StageStore{pool: pool}
}

func (s *StageStore) Create(ctx context.Context, stage domain.Stage) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO stages (id, project_id, name, slug, status, public_url, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		stage.ID, stage.ProjectID, stage.Name, stage.Slug, string(stage.Status), stage.PublicURL, stage.CreatedAt, stage.UpdatedAt,
	)
	return err
}

func (s *StageStore) GetByID(ctx context.Context, stageID string) (domain.Stage, bool) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, project_id, name, slug, status, public_url, created_at, updated_at FROM stages WHERE id = $1`,
		stageID,
	)
	stage, err := scanStage(row)
	if err != nil {
		return domain.Stage{}, false
	}
	return stage, true
}

func (s *StageStore) GetBySlug(ctx context.Context, projectID, slug string) (domain.Stage, bool) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, project_id, name, slug, status, public_url, created_at, updated_at FROM stages WHERE project_id = $1 AND slug = $2`,
		projectID, slug,
	)
	stage, err := scanStage(row)
	if err != nil {
		return domain.Stage{}, false
	}
	return stage, true
}

func (s *StageStore) ListByProject(ctx context.Context, projectID string) []domain.Stage {
	rows, err := s.pool.Query(ctx,
		`SELECT id, project_id, name, slug, status, public_url, created_at, updated_at FROM stages WHERE project_id = $1 ORDER BY created_at ASC`,
		projectID,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var result []domain.Stage
	for rows.Next() {
		stage, err := scanStage(rows)
		if err != nil {
			continue
		}
		result = append(result, stage)
	}
	return result
}

func (s *StageStore) Update(ctx context.Context, stage domain.Stage) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE stages SET name = $1, status = $2, public_url = $3, updated_at = $4 WHERE id = $5`,
		stage.Name, string(stage.Status), stage.PublicURL, stage.UpdatedAt, stage.ID,
	)
	return err
}

func scanStage(row stageRowScanner) (domain.Stage, error) {
	var stage domain.Stage
	var status string
	err := row.Scan(&stage.ID, &stage.ProjectID, &stage.Name, &stage.Slug, &status, &stage.PublicURL, &stage.CreatedAt, &stage.UpdatedAt)
	if err != nil {
		return domain.Stage{}, err
	}
	stage.Status = domain.StageStatus(status)
	return stage, nil
}
