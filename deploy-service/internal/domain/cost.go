package domain

type CostBreakdown struct {
	ProjectID                  string  `json:"projectId"`
	ProcessorCoreHours         float64 `json:"processorCoreHours"`
	MemoryGigabyteHours        float64 `json:"memoryGigabyteHours"`
	PersistentStorageGigabytes float64 `json:"persistentStorageGigabytes"`
	OutgoingTrafficGigabytes   float64 `json:"outgoingTrafficGigabytes"`
	DedicatedLoadBalancerHours float64 `json:"dedicatedLoadBalancerHours,omitempty"`
	Total                      float64 `json:"total"`
	Currency                   string  `json:"currency"`
}
