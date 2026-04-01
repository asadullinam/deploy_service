package postgres

import "github.com/jackc/pgx/v5/pgxpool"

type ReleaseStore struct {
	pool *pgxpool.Pool
}

type scanner interface {
	Scan(dest ...any) error
}
