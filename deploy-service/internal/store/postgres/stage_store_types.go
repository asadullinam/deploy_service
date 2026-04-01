package postgres

import "github.com/jackc/pgx/v5/pgxpool"

type StageStore struct {
	pool *pgxpool.Pool
}

type stageRowScanner interface {
	Scan(dest ...any) error
}
