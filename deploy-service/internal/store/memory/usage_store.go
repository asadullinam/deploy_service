package memory

import (
	"context"
	"deploy-service/internal/domain"
	"deploy-service/internal/service"
	"time"
)

var _ service.UsageStore = (*UsageStore)(nil)

func NewUsageStore() *UsageStore {
	return &UsageStore{}
}

func (s *UsageStore) Record(_ context.Context, usage domain.ResourceUsage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = append(s.records, usage)
	return nil
}

func (s *UsageStore) AggregateForProject(_ context.Context, projectID string, from, to time.Time) (domain.UsageAggregate, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var agg domain.UsageAggregate
	for _, r := range s.records {
		if r.ProjectID == projectID && !r.PeriodStart.Before(from) && !r.PeriodEnd.After(to) {
			agg.CPUCoreHours += r.CPUCoreHours
			agg.MemoryGBHours += r.MemoryGBHours
			if r.StorageGB > agg.StorageGB {
				agg.StorageGB = r.StorageGB
			}
			agg.EgressGB += r.EgressGB
		}
	}
	return agg, nil
}
