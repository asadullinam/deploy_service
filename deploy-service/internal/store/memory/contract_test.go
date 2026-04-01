//go:build !integration

// Тесты ожидаемого поведения для реализаций хранилища.
// Каждая функция RunXxxStoreContract задает ожидаемое поведение интерфейса хранилища
// и может переиспользоваться для любой реализации (memory, postgres и т. д.).

package memory

import (
	"context"
	"testing"
	"time"

	"deploy-service/internal/domain"
	"deploy-service/internal/service"
)

// ---- ProjectStore: проверка поведения ----

func RunProjectStoreContract(t *testing.T, store service.ProjectStore) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()

	p := domain.Project{
		ID:        "prj-contract-1",
		Name:      "contract-project",
		OwnerID:   "usr-contract-1",
		Status:    domain.ProjectStatusCreating,
		CreatedAt: now,
		UpdatedAt: now,
	}

	t.Run("Create and GetByID", func(t *testing.T) {
		if err := store.Create(ctx, p); err != nil {
			t.Fatalf("Create: %v", err)
		}
		got, ok := store.GetByID(ctx, p.ID)
		if !ok {
			t.Fatal("GetByID: expected project to be found")
		}
		if got.ID != p.ID || got.Name != p.Name || got.OwnerID != p.OwnerID {
			t.Errorf("GetByID: got %+v, want %+v", got, p)
		}
	})

	t.Run("GetByID returns false for missing", func(t *testing.T) {
		_, ok := store.GetByID(ctx, "prj-nonexistent")
		if ok {
			t.Error("GetByID: expected false for nonexistent project")
		}
	})

	t.Run("List returns owned projects", func(t *testing.T) {
		all := store.List(ctx)
		found := false
		for _, item := range all {
			if item.ID == p.ID {
				found = true
				break
			}
		}
		if !found {
			t.Error("List: expected created project in result")
		}
	})

	t.Run("Update persists changes", func(t *testing.T) {
		p.Status = domain.ProjectStatusActive
		p.ServiceName = "updated-svc"
		if err := store.Update(ctx, p); err != nil {
			t.Fatalf("Update: %v", err)
		}
		got, ok := store.GetByID(ctx, p.ID)
		if !ok {
			t.Fatal("GetByID after Update: not found")
		}
		if got.Status != domain.ProjectStatusActive {
			t.Errorf("Status after Update: got %q, want %q", got.Status, domain.ProjectStatusActive)
		}
		if got.ServiceName != "updated-svc" {
			t.Errorf("ServiceName after Update: got %q, want %q", got.ServiceName, "updated-svc")
		}
	})

}

// ---- ReleaseStore: проверка поведения ----

func RunReleaseStoreContract(t *testing.T, store service.ReleaseStore) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()

	r := domain.Release{
		ID:            "rel-contract-1",
		ProjectID:     "prj-contract-1",
		Status:        domain.ReleaseStatusPending,
		WorkflowRunID: 555,
		CommitSHA:     "abc123",
		CommitMessage: "feat: initial",
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	t.Run("Create and GetByID", func(t *testing.T) {
		if err := store.Create(ctx, r); err != nil {
			t.Fatalf("Create: %v", err)
		}
		got, ok := store.GetByID(ctx, r.ID)
		if !ok {
			t.Fatal("GetByID: expected release to be found")
		}
		if got.CommitSHA != r.CommitSHA || got.WorkflowRunID != r.WorkflowRunID {
			t.Errorf("GetByID: got %+v, want %+v", got, r)
		}
	})

	t.Run("GetByID returns false for missing", func(t *testing.T) {
		_, ok := store.GetByID(ctx, "rel-nonexistent")
		if ok {
			t.Error("GetByID: expected false for nonexistent release")
		}
	})

	t.Run("ListByProject returns correct releases", func(t *testing.T) {
		releases := store.ListByProject(ctx, r.ProjectID)
		if len(releases) != 1 {
			t.Fatalf("ListByProject: expected 1, got %d", len(releases))
		}
		if releases[0].ID != r.ID {
			t.Errorf("ListByProject: got ID %q, want %q", releases[0].ID, r.ID)
		}
	})

	t.Run("ListByProject returns empty for unknown project", func(t *testing.T) {
		releases := store.ListByProject(ctx, "prj-unknown")
		if len(releases) != 0 {
			t.Errorf("ListByProject: expected empty, got %d items", len(releases))
		}
	})

	t.Run("GetByWorkflowRunID finds release", func(t *testing.T) {
		got, ok := store.GetByWorkflowRunID(ctx, r.WorkflowRunID)
		if !ok {
			t.Fatal("GetByWorkflowRunID: expected release to be found")
		}
		if got.ID != r.ID {
			t.Errorf("GetByWorkflowRunID: got ID %q, want %q", got.ID, r.ID)
		}
	})

	t.Run("GetByWorkflowRunID returns false for missing", func(t *testing.T) {
		_, ok := store.GetByWorkflowRunID(ctx, 99999)
		if ok {
			t.Error("GetByWorkflowRunID: expected false for nonexistent run ID")
		}
	})

	t.Run("Update persists status change", func(t *testing.T) {
		r.Status = domain.ReleaseStatusSuccess
		if err := store.Update(ctx, r); err != nil {
			t.Fatalf("Update: %v", err)
		}
		got, ok := store.GetByID(ctx, r.ID)
		if !ok {
			t.Fatal("GetByID after Update: not found")
		}
		if got.Status != domain.ReleaseStatusSuccess {
			t.Errorf("Status after Update: got %q, want %q", got.Status, domain.ReleaseStatusSuccess)
		}
	})
}

// ---- Запуск проверок на memory-реализациях ----

func TestMemoryProjectStoreContract(t *testing.T) {
	RunProjectStoreContract(t, NewProjectStore())
}

func TestMemoryReleaseStoreContract(t *testing.T) {
	RunReleaseStoreContract(t, NewReleaseStore())
}
