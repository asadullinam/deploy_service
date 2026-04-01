package monetization

type Tariff struct {
	CPUCoreHourRUB               float64
	MemoryGBHourRUB              float64
	StorageGBMonthRUB            float64
	EgressGBRUB                  float64
	DedicatedLoadBalancerHourRUB float64
}
