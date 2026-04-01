package app

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
)

type Config struct {
	Address string
}

type Application struct {
	Config Config
	Router http.Handler
	// pool не равен nil только при использовании PostgreSQL; закрывается в Close().
	pool *pgxpool.Pool
}

type billingGuardEnforcer interface {
	EnforceAllBillingGuards(ctx context.Context) error
}
