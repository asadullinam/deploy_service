package postgres

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migrate применяет все ожидающие SQL-миграции из каталога migrations/.
// Функция создает таблицу schema_migrations, чтобы отслеживать уже примененные файлы.
// Миграции применяются в лексикографическом порядке (001_..., 002_... и т.д.).
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	if err := createMigrationsTable(ctx, pool); err != nil {
		return fmt.Errorf("create migrations table: %w", err)
	}

	files, err := listMigrationFiles()
	if err != nil {
		return fmt.Errorf("list migration files: %w", err)
	}

	applied, err := appliedMigrations(ctx, pool)
	if err != nil {
		return fmt.Errorf("fetch applied migrations: %w", err)
	}

	for _, name := range files {
		if applied[name] {
			continue
		}
		if err := applyMigration(ctx, pool, name); err != nil {
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
	}

	return nil
}

func createMigrationsTable(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			name       TEXT        PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`)
	return err
}

func listMigrationFiles() ([]string, error) {
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return nil, err
	}

	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}

	sort.Strings(names)
	return names, nil
}

func appliedMigrations(ctx context.Context, pool *pgxpool.Pool) (map[string]bool, error) {
	rows, err := pool.Query(ctx, `SELECT name FROM schema_migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applied := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		applied[name] = true
	}

	return applied, rows.Err()
}

func applyMigration(ctx context.Context, pool *pgxpool.Pool, name string) error {
	sql, err := migrationsFS.ReadFile("migrations/" + name)
	if err != nil {
		return err
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if _, err := tx.Exec(ctx, string(sql)); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (name) VALUES ($1)`, name); err != nil {
		return err
	}

	return tx.Commit(ctx)
}
