package postgres

import "github.com/jackc/pgx/v5/pgxpool"

type UsageStore struct {
	pool *pgxpool.Pool
}
