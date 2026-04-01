package memory

import (
	"context"
	"deploy-service/internal/domain"
	"deploy-service/internal/service"
	"sync"
	"time"
)

var _ service.YooKassaPaymentStore = (*YooKassaPaymentStore)(nil)

type YooKassaPaymentStore struct {
	mu       sync.RWMutex
	payments []domain.YooKassaPayment
}

func NewYooKassaPaymentStore() *YooKassaPaymentStore {
	return &YooKassaPaymentStore{}
}

func (s *YooKassaPaymentStore) Create(_ context.Context, p domain.YooKassaPayment) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.payments = append(s.payments, p)
	return nil
}

func (s *YooKassaPaymentStore) GetByYooKassaID(_ context.Context, yookassaID string) (domain.YooKassaPayment, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, p := range s.payments {
		if p.YooKassaID == yookassaID {
			return p, true
		}
	}
	return domain.YooKassaPayment{}, false
}

func (s *YooKassaPaymentStore) MarkSucceeded(_ context.Context, yookassaID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, p := range s.payments {
		if p.YooKassaID == yookassaID {
			s.payments[i].Status = domain.YooKassaPaymentStatusSucceeded
			s.payments[i].UpdatedAt = time.Now().UTC()
			return nil
		}
	}
	return nil
}
