package domain

import "time"

type ResourceUsage struct {
	ID                         string    `json:"id"`
	ProjectID                  string    `json:"projectId"`
	PeriodStart                time.Time `json:"periodStart"`
	PeriodEnd                  time.Time `json:"periodEnd"`
	CPUCores                   float64   `json:"cpuCores"`
	MemoryGB                   float64   `json:"memoryGb"`
	StorageGB                  float64   `json:"storageGb"`
	EgressGB                   float64   `json:"egressGb"`
	ReplicaCount               int32     `json:"replicaCount"`
	PodUptimeHours             float64   `json:"podUptimeHours"`
	CPUCoreHours               float64   `json:"cpuCoreHours"`
	MemoryGBHours              float64   `json:"memoryGbHours"`
	DedicatedLoadBalancerHours float64   `json:"dedicatedLoadBalancerHours"`
	RecordedAt                 time.Time `json:"recordedAt"`
}

type ResourceSnapshot struct {
	CPUCores       float64
	MemoryGB       float64
	StorageGB      float64
	EgressGBDelta  float64
	ReplicaCount   int32
	PodUptimeHours float64
}

type UsageAggregate struct {
	CPUCoreHours               float64
	MemoryGBHours              float64
	StorageGB                  float64
	EgressGB                   float64
	DedicatedLoadBalancerHours float64
}
