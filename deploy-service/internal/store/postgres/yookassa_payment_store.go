package postgres

import (
	"context"
	"deploy-service/internal/domain"
	"deploy-service/internal/service"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

var _ service.YooKassaPaymentStore = (*YooKassaPaymentStore)(nil)

func NewYooKassaPaymentStore(pool *pgxpool.Pool) *YooKassaPaymentStore {
	return &YooKassaPaymentStore{pool: pool}
}

func (s *YooKassaPaymentStore) Create(ctx context.Context, p domain.YooKassaPayment) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO yookassa_payments (id, yookassa_id, user_id, amount_rub, status, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		p.ID, p.YooKassaID, p.UserID, p.AmountRUB, p.Status, p.CreatedAt, p.UpdatedAt,
	)
	return err
}

func (s *YooKassaPaymentStore) GetByYooKassaID(ctx context.Context, yookassaID string) (domain.YooKassaPayment, bool) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, yookassa_id, user_id, amount_rub, status, created_at, updated_at
		 FROM yookassa_payments WHERE yookassa_id = $1`,
		yookassaID,
	)
	var p domain.YooKassaPayment
	err := row.Scan(&p.ID, &p.YooKassaID, &p.UserID, &p.AmountRUB, &p.Status, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return domain.YooKassaPayment{}, false
	}
	return p, true
}

func (s *YooKassaPaymentStore) MarkSucceeded(ctx context.Context, yookassaID string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE yookassa_payments SET status = $1, updated_at = $2 WHERE yookassa_id = $3`,
		domain.YooKassaPaymentStatusSucceeded, time.Now().UTC(), yookassaID,
	)
	return err
}
