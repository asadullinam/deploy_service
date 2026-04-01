package postgres

import "github.com/jackc/pgx/v5/pgxpool"

type YooKassaPaymentStore struct {
	pool *pgxpool.Pool
}
