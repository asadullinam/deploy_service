package postgres

import "github.com/jackc/pgx/v5/pgxpool"

type ProjectStore struct {
	pool *pgxpool.Pool
}
