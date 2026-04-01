//go:build !integration

package app

import (
	"context"
	"math"
	"testing"
	"time"

	"deploy-service/internal/domain"
	"deploy-service/internal/service"
)

type usageProjectStoreStub struct {
	projects []domain.Project
}

func (s *usageProjectStoreStub) Create(context.Context, domain.Project) error { return nil }
func (s *usageProjectStoreStub) GetByID(context.Context, string) (domain.Project, bool) {
	return domain.Project{}, false
}
func (s *usageProjectStoreStub) List(context.Context) []domain.Project        { return s.projects }
func (s *usageProjectStoreStub) Update(context.Context, domain.Project) error { return nil }
func (s *usageProjectStoreStub) UpdateKubeconfig(context.Context, string, string) error {
	return nil
}

func (s *usageProjectStoreStub) UpdateGitHubToken(context.Context, string, string) error {
	return nil
}

type usageStoreStub struct {
	records []domain.ResourceUsage
}

func (s *usageStoreStub) Record(_ context.Context, usage domain.ResourceUsage) error {
	s.records = append(s.records, usage)
	return nil
}

func (s *usageStoreStub) AggregateForProject(context.Context, string, time.Time, time.Time) (domain.UsageAggregate, error) {
	return domain.UsageAggregate{}, nil
}

type metricsCollectorStub struct {
	snapshot domain.ResourceSnapshot
}

func (m *metricsCollectorStub) CollectProjectUsage(context.Context, string) (domain.ResourceSnapshot, error) {
	return m.snapshot, nil
}

type usageUserStoreStub struct{}

func (s *usageUserStoreStub) Create(_ context.Context, _ domain.User) error { return nil }
func (s *usageUserStoreStub) GetByEmail(_ context.Context, _ string) (domain.User, bool) {
	return domain.User{}, false
}
func (s *usageUserStoreStub) GetByID(_ context.Context, _ string) (domain.User, bool) {
	return domain.User{}, false
}
func (s *usageUserStoreStub) GetByTelegramUsername(_ context.Context, _ string) (domain.User, bool) {
	return domain.User{}, false
}
func (s *usageUserStoreStub) GetByTelegramLinkCode(_ context.Context, _ string) (domain.User, bool) {
	return domain.User{}, false
}
func (s *usageUserStoreStub) GetByTelegramChatID(_ context.Context, _ int64) (domain.User, bool) {
	return domain.User{}, false
}
func (s *usageUserStoreStub) UpdateBalance(_ context.Context, _ string, _ float64) error {
	return nil
}
func (s *usageUserStoreStub) UpdateGitHubToken(_ context.Context, _ string, _ string) error {
	return nil
}
func (s *usageUserStoreStub) UpdateTelegramSettings(_ context.Context, _ string, _ string, _ string, _ *time.Time, _ bool) error {
	return nil
}
func (s *usageUserStoreStub) LinkTelegramChat(_ context.Context, _ string, _ int64, _ time.Time) error {
	return nil
}
func (s *usageUserStoreStub) DisableTelegramNotifications(_ context.Context, _ string) error {
	return nil
}
func (s *usageUserStoreStub) ClearTelegramSettings(_ context.Context, _ string) error {
	return nil
}

type usageTxStoreStub struct{}

func (s *usageTxStoreStub) Record(_ context.Context, _ domain.BillingTransaction) error {
	return nil
}
func (s *usageTxStoreStub) ListByUser(_ context.Context, _ string) ([]domain.BillingTransaction, error) {
	return nil, nil
}
func (s *usageTxStoreStub) ListByProject(_ context.Context, _ string) ([]domain.BillingTransaction, error) {
	return nil, nil
}

type usageMonetizationStub struct{}

func (m *usageMonetizationStub) GetProjectCost(_ context.Context, projectID string) (domain.CostBreakdown, error) {
	return domain.CostBreakdown{ProjectID: projectID}, nil
}
func (m *usageMonetizationStub) ComputeUsageCost(_ domain.ResourceUsage) float64 { return 0 }

var _ service.ProjectStore = (*usageProjectStoreStub)(nil)
var _ service.UsageStore = (*usageStoreStub)(nil)
var _ service.MetricsCollector = (*metricsCollectorStub)(nil)

func TestCollectAndStoreUsageConvertsSnapshotsIntoTimeSeriesAndHourUnits(t *testing.T) {
	t.Parallel()

	projectStore := &usageProjectStoreStub{
		projects: []domain.Project{
			{ID: "prj-1", Status: domain.ProjectStatusActive, ServiceType: "LoadBalancer", DedicatedLoadBalancer: true},
			{ID: "prj-2", Status: domain.ProjectStatusSuspended},
		},
	}
	usageStore := &usageStoreStub{}
	collector := &metricsCollectorStub{
		snapshot: domain.ResourceSnapshot{
			CPUCores:       0.6,
			MemoryGB:       1.2,
			StorageGB:      5,
			EgressGBDelta:  0.25,
			ReplicaCount:   2,
			PodUptimeHours: 6,
		},
	}

	periodEnd := time.Date(2026, 3, 21, 1, 0, 0, 0, time.UTC)
	periodStart := periodEnd.Add(-5 * time.Minute)
	collectAndStoreUsage(context.Background(), periodStart, periodEnd, projectStore, &usageUserStoreStub{}, usageStore, &usageTxStoreStub{}, &usageMonetizationStub{}, collector, nil)

	if len(usageStore.records) != 1 {
		t.Fatalf("expected one usage record, got %d", len(usageStore.records))
	}

	record := usageStore.records[0]
	if record.ProjectID != "prj-1" {
		t.Fatalf("expected active project usage to be recorded, got %q", record.ProjectID)
	}
	if record.CPUCores != 0.6 || record.MemoryGB != 1.2 || record.StorageGB != 5 {
		t.Fatalf("unexpected snapshot fields: %+v", record)
	}
	if record.ReplicaCount != 2 || record.PodUptimeHours != 6 {
		t.Fatalf("unexpected runtime snapshot fields: %+v", record)
	}
	if math.Abs(record.CPUCoreHours-0.05) > 1e-9 {
		t.Fatalf("expected cpu core hours 0.05, got %v", record.CPUCoreHours)
	}
	if math.Abs(record.MemoryGBHours-0.1) > 1e-9 {
		t.Fatalf("expected memory gb hours 0.1, got %v", record.MemoryGBHours)
	}
	if math.Abs(record.DedicatedLoadBalancerHours-(5.0/60.0)) > 1e-9 {
		t.Fatalf("expected dedicated load balancer hours 5m, got %v", record.DedicatedLoadBalancerHours)
	}
	if record.EgressGB != 0.25 {
		t.Fatalf("expected egress delta 0.25, got %v", record.EgressGB)
	}
}
