package memory

import (
	"deploy-service/internal/domain"
	"sync"
)

type StageStore struct {
	mu     sync.RWMutex
	stages map[string]domain.Stage
}
