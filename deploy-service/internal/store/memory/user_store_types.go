package memory

import (
	"deploy-service/internal/domain"
	"sync"
)

type UserStore struct {
	mu      sync.RWMutex
	byID    map[string]domain.User
	byEmail map[string]domain.User
}
