package memory

import (
	"context"
	"deploy-service/internal/domain"
	"sort"
)

func NewStageStore() *StageStore {
	return &StageStore{stages: make(map[string]domain.Stage)}
}

func (s *StageStore) Create(_ context.Context, stage domain.Stage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stages[stage.ID] = stage
	return nil
}

func (s *StageStore) GetByID(_ context.Context, stageID string) (domain.Stage, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	stage, ok := s.stages[stageID]
	return stage, ok
}

func (s *StageStore) GetBySlug(_ context.Context, projectID, slug string) (domain.Stage, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, stage := range s.stages {
		if stage.ProjectID == projectID && stage.Slug == slug {
			return stage, true
		}
	}
	return domain.Stage{}, false
}

func (s *StageStore) ListByProject(_ context.Context, projectID string) []domain.Stage {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []domain.Stage
	for _, stage := range s.stages {
		if stage.ProjectID == projectID {
			result = append(result, stage)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].ID < result[j].ID
		}
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})
	return result
}

func (s *StageStore) Update(_ context.Context, stage domain.Stage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stages[stage.ID] = stage
	return nil
}
