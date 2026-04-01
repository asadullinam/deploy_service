//go:build integration

package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"deploy-service/internal/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestProjectStoreContract_PreservesKubeconfigOnUpdate(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run postgres integration tests")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	defer pool.Close()

	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate postgres: %v", err)
	}

	store := NewProjectStore(pool)
	projectID := "prj-it-" + time.Now().UTC().Format("20060102150405.000000000")
	project := domain.Project{
		ID:                  projectID,
		Name:                "demo",
		OwnerID:             "usr-1",
		Status:              domain.ProjectStatusCreating,
		KubeconfigEncrypted: "enc:kubeconfig",
		CreatedAt:           time.Now().UTC(),
		UpdatedAt:           time.Now().UTC(),
	}

	if err := store.Create(ctx, project); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	project.Status = domain.ProjectStatusActive
	project.KubeconfigEncrypted = ""
	project.UpdatedAt = time.Now().UTC()
	if err := store.Update(ctx, project); err != nil {
		t.Fatalf("Update returned error: %v", err)
	}

	stored, ok := store.GetByID(ctx, projectID)
	if !ok {
		t.Fatalf("expected stored project %s", projectID)
	}
	if stored.KubeconfigEncrypted != "enc:kubeconfig" {
		t.Fatalf("expected kubeconfig to be preserved, got %q", stored.KubeconfigEncrypted)
	}
}
