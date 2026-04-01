//go:build !integration

package memory

import (
	"context"
	"testing"
	"time"

	"deploy-service/internal/domain"
)

func TestProjectStoreUpdatePreservesKubeconfig(t *testing.T) {
	t.Parallel()

	store := NewProjectStore()
	project := domain.Project{
		ID:                  "prj-1",
		Name:                "demo",
		OwnerID:             "usr-1",
		Status:              domain.ProjectStatusCreating,
		KubeconfigEncrypted: "enc:kubeconfig",
		CreatedAt:           time.Now().UTC(),
		UpdatedAt:           time.Now().UTC(),
	}

	if err := store.Create(context.Background(), project); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	project.Status = domain.ProjectStatusActive
	project.KubeconfigEncrypted = ""
	if err := store.Update(context.Background(), project); err != nil {
		t.Fatalf("Update returned error: %v", err)
	}

	stored, ok := store.GetByID(context.Background(), project.ID)
	if !ok {
		t.Fatalf("expected stored project %s", project.ID)
	}
	if stored.KubeconfigEncrypted != "enc:kubeconfig" {
		t.Fatalf("expected kubeconfig to be preserved, got %q", stored.KubeconfigEncrypted)
	}
}
