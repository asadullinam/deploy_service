package memory

import (
	"context"
	"deploy-service/internal/domain"
	"deploy-service/internal/service"
)

var _ service.ReleaseStore = (*ReleaseStore)(nil)

func NewReleaseStore() *ReleaseStore {
	return &ReleaseStore{records: make(map[string]domain.Release)}
}

func (s *ReleaseStore) Create(_ context.Context, r domain.Release) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records[r.ID] = r
	return nil
}

func (s *ReleaseStore) GetByID(_ context.Context, id string) (domain.Release, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.records[id]
	return r, ok
}

func (s *ReleaseStore) ListByProject(_ context.Context, projectID string) []domain.Release {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []domain.Release
	for _, r := range s.records {
		if r.ProjectID == projectID {
			result = append(result, r)
		}
	}
	return result
}

func (s *ReleaseStore) Update(_ context.Context, r domain.Release) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records[r.ID] = r
	return nil
}

func (s *ReleaseStore) GetByWorkflowRunID(_ context.Context, runID int64) (domain.Release, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, r := range s.records {
		if r.WorkflowRunID == runID {
			return r, true
		}
	}
	return domain.Release{}, false
}
