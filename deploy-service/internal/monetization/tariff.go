package monetization

import (
	"os"
	"strconv"
	"strings"
)

var DefaultTariff = Tariff{
	CPUCoreHourRUB:               0.72,
	MemoryGBHourRUB:              0.09,
	StorageGBMonthRUB:            45.0,
	EgressGBRUB:                  1.08,
	DedicatedLoadBalancerHourRUB: 0.75,
}

// TariffFromEnvironment возвращает Tariff, заполненный из переменных окружения, с откатом
// к значениям DefaultTariff для каждой отсутствующей или некорректной переменной.
//
//	TARIFF_CPU_CORE_HOUR_RUB
//	TARIFF_MEMORY_GB_HOUR_RUB
//	TARIFF_STORAGE_GB_MONTH_RUB
//	TARIFF_EGRESS_GB_RUB
//	TARIFF_DEDICATED_LOAD_BALANCER_HOUR_RUB
func TariffFromEnvironment() Tariff {
	t := DefaultTariff
	if v := parseEnvFloat("TARIFF_CPU_CORE_HOUR_RUB"); v > 0 {
		t.CPUCoreHourRUB = v
	}
	if v := parseEnvFloat("TARIFF_MEMORY_GB_HOUR_RUB"); v > 0 {
		t.MemoryGBHourRUB = v
	}
	if v := parseEnvFloat("TARIFF_STORAGE_GB_MONTH_RUB"); v > 0 {
		t.StorageGBMonthRUB = v
	}
	if v := parseEnvFloat("TARIFF_EGRESS_GB_RUB"); v > 0 {
		t.EgressGBRUB = v
	}
	if v := parseEnvFloat("TARIFF_DEDICATED_LOAD_BALANCER_HOUR_RUB"); v > 0 {
		t.DedicatedLoadBalancerHourRUB = v
	}
	return t
}

func parseEnvFloat(key string) float64 {
	s := strings.TrimSpace(os.Getenv(key))
	if s == "" {
		return 0
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}
