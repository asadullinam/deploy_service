package postgres

import (
	"context"
	"deploy-service/internal/domain"
	"deploy-service/internal/service"
	"github.com/jackc/pgx/v5/pgxpool"
)

var _ service.BillingTransactionStore = (*BillingTransactionStore)(nil)

func NewBillingTransactionStore(pool *pgxpool.Pool) *BillingTransactionStore {
	return &BillingTransactionStore{pool: pool}
}

func (s *BillingTransactionStore) Record(ctx context.Context, tx domain.BillingTransaction) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO billing_transactions (id, user_id, project_id, type, amount_rub, description, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		tx.ID, tx.UserID, tx.ProjectID, string(tx.Type), tx.AmountRUB, tx.Description, tx.CreatedAt,
	)
	return err
}

func (s *BillingTransactionStore) ListByUser(ctx context.Context, userID string) ([]domain.BillingTransaction, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, user_id, project_id, type, amount_rub, description, created_at
		 FROM billing_transactions WHERE user_id = $1 ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTransactions(rows)
}

func (s *BillingTransactionStore) ListByProject(ctx context.Context, projectID string) ([]domain.BillingTransaction, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, user_id, project_id, type, amount_rub, description, created_at
		 FROM billing_transactions WHERE project_id = $1 ORDER BY created_at DESC`,
		projectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTransactions(rows)
}

func scanTransactions(rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}) ([]domain.BillingTransaction, error) {
	var result []domain.BillingTransaction
	for rows.Next() {
		var tx domain.BillingTransaction
		var txType string
		if err := rows.Scan(&tx.ID, &tx.UserID, &tx.ProjectID, &txType, &tx.AmountRUB, &tx.Description, &tx.CreatedAt); err != nil {
			return nil, err
		}
		tx.Type = domain.TransactionType(txType)
		result = append(result, tx)
	}
	return result, rows.Err()
}
