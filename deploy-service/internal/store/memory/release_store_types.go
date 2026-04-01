package memory

import (
	"deploy-service/internal/domain"
	"sync"
)

type ReleaseStore struct {
	mu      sync.RWMutex
	records map[string]domain.Release
}
