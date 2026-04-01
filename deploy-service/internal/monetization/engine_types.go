package monetization

import "deploy-service/internal/service"

type EngineMock struct{}

type PostgresEngine struct {
	usageStore service.UsageStore
	tariff     Tariff
}
