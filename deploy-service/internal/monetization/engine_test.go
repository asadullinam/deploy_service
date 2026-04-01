//go:build !integration

package monetization

import (
	"testing"
	"time"

	"deploy-service/internal/domain"
)

func TestComputeUsageCostIncludesDedicatedLoadBalancer(t *testing.T) {
	t.Parallel()

	engine := NewPostgresEngine(nil, Tariff{
		CPUCoreHourRUB:               1,
		MemoryGBHourRUB:              1,
		StorageGBMonthRUB:            0,
		EgressGBRUB:                  0,
		DedicatedLoadBalancerHourRUB: 2,
	})

	usage := domain.ResourceUsage{
		CPUCoreHours:               0.5,
		MemoryGBHours:              0.25,
		DedicatedLoadBalancerHours: 1.5,
		PeriodStart:                time.Date(2026, 3, 26, 0, 0, 0, 0, time.UTC),
		PeriodEnd:                  time.Date(2026, 3, 26, 1, 0, 0, 0, time.UTC),
	}

	if got := engine.ComputeUsageCost(usage); got != 3.75 {
		t.Fatalf("expected total cost 3.75, got %.4f", got)
	}
}
