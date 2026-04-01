package postgres

import (
	"context"
	"deploy-service/internal/domain"
	"deploy-service/internal/service"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

var _ service.UsageStore = (*UsageStore)(nil)

func NewUsageStore(pool *pgxpool.Pool) *UsageStore {
	return &UsageStore{pool: pool}
}

func (s *UsageStore) Record(ctx context.Context, usage domain.ResourceUsage) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO resource_usage (
            id, project_id, period_start, period_end,
            cpu_cores, memory_gb, storage_gb, egress_gb,
            replica_count, pod_uptime_hours,
            cpu_core_hours, memory_gb_hours, dedicated_load_balancer_hours, recorded_at
         )
         VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
		usage.ID, usage.ProjectID, usage.PeriodStart, usage.PeriodEnd,
		usage.CPUCores, usage.MemoryGB, usage.StorageGB, usage.EgressGB,
		usage.ReplicaCount, usage.PodUptimeHours,
		usage.CPUCoreHours, usage.MemoryGBHours, usage.DedicatedLoadBalancerHours, usage.RecordedAt,
	)
	return err
}

func (s *UsageStore) AggregateForProject(ctx context.Context, projectID string, from, to time.Time) (domain.UsageAggregate, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT
            COALESCE(SUM(cpu_core_hours), 0),
            COALESCE(SUM(memory_gb_hours), 0),
            COALESCE(MAX(storage_gb), 0),
            COALESCE(SUM(egress_gb), 0),
            COALESCE(SUM(dedicated_load_balancer_hours), 0)
         FROM resource_usage
         WHERE project_id = $1 AND period_start >= $2 AND period_end <= $3`,
		projectID, from, to,
	)
	var agg domain.UsageAggregate
	err := row.Scan(&agg.CPUCoreHours, &agg.MemoryGBHours, &agg.StorageGB, &agg.EgressGB, &agg.DedicatedLoadBalancerHours)
	return agg, err
}
