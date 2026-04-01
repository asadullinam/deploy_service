package memory

import (
	"deploy-service/internal/domain"
	"sync"
)

type ProjectStore struct {
	mu       sync.RWMutex
	projects map[string]domain.Project
}
