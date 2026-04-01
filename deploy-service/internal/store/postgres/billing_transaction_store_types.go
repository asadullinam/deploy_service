package postgres

import "github.com/jackc/pgx/v5/pgxpool"

type BillingTransactionStore struct {
	pool *pgxpool.Pool
}
