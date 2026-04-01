package memory

import (
	"context"
	"deploy-service/internal/domain"
	"deploy-service/internal/service"
)

var _ service.BillingTransactionStore = (*BillingTransactionStore)(nil)

func NewBillingTransactionStore() *BillingTransactionStore {
	return &BillingTransactionStore{}
}

func (s *BillingTransactionStore) Record(_ context.Context, tx domain.BillingTransaction) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = append(s.records, tx)
	return nil
}

func (s *BillingTransactionStore) ListByUser(_ context.Context, userID string) ([]domain.BillingTransaction, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []domain.BillingTransaction
	for _, tx := range s.records {
		if tx.UserID == userID {
			result = append(result, tx)
		}
	}
	return result, nil
}

func (s *BillingTransactionStore) ListByProject(_ context.Context, projectID string) ([]domain.BillingTransaction, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []domain.BillingTransaction
	for _, tx := range s.records {
		if tx.ProjectID == projectID {
			result = append(result, tx)
		}
	}
	return result, nil
}
