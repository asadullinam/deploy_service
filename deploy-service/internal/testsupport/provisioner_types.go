package testsupport

import (
	"deploy-service/internal/domain"
	"sync"
)

type RuntimeProvisioner struct {
	mu      sync.RWMutex
	status  map[string]domain.ProjectRuntimeStatus
	images  map[string]string
	paused  map[string]bool
	deleted map[string]bool
}
