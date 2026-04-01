package monetization

import (
	"context"
	"deploy-service/internal/domain"
	"deploy-service/internal/service"
	"fmt"
	"time"
)

// Проверка на этапе компиляции: EngineMock реализует service.MonetizationEngine.
var _ service.MonetizationEngine = (*EngineMock)(nil)

func NewEngineMock() *EngineMock {
	return &EngineMock{}
}

func (e *EngineMock) GetProjectCost(_ context.Context, projectID string) (domain.CostBreakdown, error) {
	return domain.CostBreakdown{
		ProjectID:                  projectID,
		ProcessorCoreHours:         12.5,
		MemoryGigabyteHours:        48.0,
		PersistentStorageGigabytes: 10,
		OutgoingTrafficGigabytes:   2.1,
		Total:                      13.74,
		Currency:                   "RUB",
	}, nil
}

func (e *EngineMock) ComputeUsageCost(_ domain.ResourceUsage) float64 { return 0 }

// PostgresEngine — реальная реализация движка.
var _ service.MonetizationEngine = (*PostgresEngine)(nil)

func NewPostgresEngine(usageStore service.UsageStore, tariff Tariff) *PostgresEngine {
	return &PostgresEngine{usageStore: usageStore, tariff: tariff}
}

func (e *PostgresEngine) ComputeUsageCost(usage domain.ResourceUsage) float64 {
	return usage.CPUCoreHours*e.tariff.CPUCoreHourRUB +
		usage.MemoryGBHours*e.tariff.MemoryGBHourRUB +
		usage.StorageGB*e.tariff.StorageGBMonthRUB*(usage.PeriodEnd.Sub(usage.PeriodStart).Hours()/(30*24)) +
		usage.EgressGB*e.tariff.EgressGBRUB +
		usage.DedicatedLoadBalancerHours*e.tariff.DedicatedLoadBalancerHourRUB
}

func (e *PostgresEngine) GetProjectCost(ctx context.Context, projectID string) (domain.CostBreakdown, error) {
	now := time.Now().UTC()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)

	usage, err := e.usageStore.AggregateForProject(ctx, projectID, monthStart, now)
	if err != nil {
		return domain.CostBreakdown{}, fmt.Errorf("aggregate usage: %w", err)
	}

	storageMonthFraction := now.Sub(monthStart).Hours() / (30 * 24)

	total := usage.CPUCoreHours*e.tariff.CPUCoreHourRUB +
		usage.MemoryGBHours*e.tariff.MemoryGBHourRUB +
		usage.StorageGB*e.tariff.StorageGBMonthRUB*storageMonthFraction +
		usage.EgressGB*e.tariff.EgressGBRUB +
		usage.DedicatedLoadBalancerHours*e.tariff.DedicatedLoadBalancerHourRUB

	return domain.CostBreakdown{
		ProjectID:                  projectID,
		ProcessorCoreHours:         usage.CPUCoreHours,
		MemoryGigabyteHours:        usage.MemoryGBHours,
		PersistentStorageGigabytes: usage.StorageGB,
		OutgoingTrafficGigabytes:   usage.EgressGB,
		DedicatedLoadBalancerHours: usage.DedicatedLoadBalancerHours,
		Total:                      total,
		Currency:                   "RUB",
	}, nil
}
