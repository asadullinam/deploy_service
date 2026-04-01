package memory

import (
	"deploy-service/internal/domain"
	"sync"
)

type UsageStore struct {
	mu      sync.RWMutex
	records []domain.ResourceUsage
}
