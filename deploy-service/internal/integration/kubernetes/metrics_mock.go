package kubernetes

import (
	"context"
	"deploy-service/internal/domain"
	"deploy-service/internal/service"
)

var _ service.MetricsCollector = (*MetricsCollectorMock)(nil)

func (m *MetricsCollectorMock) CollectProjectUsage(_ context.Context, _ string) (domain.ResourceSnapshot, error) {
	return domain.ResourceSnapshot{
		CPUCores:       0.1,
		MemoryGB:       0.25,
		StorageGB:      1.0,
		EgressGBDelta:  0.02,
		ReplicaCount:   1,
		PodUptimeHours: 4,
	}, nil
}
