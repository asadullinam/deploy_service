package memory

import (
	"deploy-service/internal/domain"
	"sync"
)

type BillingTransactionStore struct {
	mu      sync.RWMutex
	records []domain.BillingTransaction
}
