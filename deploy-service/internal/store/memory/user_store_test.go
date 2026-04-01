//go:build !integration

package memory

import (
	"context"
	"testing"
	"time"

	"deploy-service/internal/domain"
)

func TestUserStoreRejectsDuplicateEmail(t *testing.T) {
	t.Parallel()

	store := NewUserStore()
	user := domain.User{
		ID:           "usr-1",
		Email:        "alice@example.com",
		PasswordHash: "hash",
		CreatedAt:    time.Now().UTC(),
	}

	if err := store.Create(context.Background(), user); err != nil {
		t.Fatalf("first Create returned error: %v", err)
	}
	if err := store.Create(context.Background(), user); err != domain.ErrEmailTaken {
		t.Fatalf("expected ErrEmailTaken, got %v", err)
	}
}

func TestUserStoreUpdateBalancePersistsValue(t *testing.T) {
	t.Parallel()

	store := NewUserStore()
	user := domain.User{
		ID:           "usr-1",
		Email:        "alice@example.com",
		PasswordHash: "hash",
		BalanceRUB:   5,
		CreatedAt:    time.Now().UTC(),
	}

	if err := store.Create(context.Background(), user); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if err := store.UpdateBalance(context.Background(), "usr-1", 1005); err != nil {
		t.Fatalf("UpdateBalance returned error: %v", err)
	}

	updated, ok := store.GetByID(context.Background(), "usr-1")
	if !ok {
		t.Fatal("expected updated user in store")
	}
	if updated.BalanceRUB != 1005 {
		t.Fatalf("expected balance 1005, got %.2f", updated.BalanceRUB)
	}
}

func TestUserStoreUpdateGitHubTokenPersistsValue(t *testing.T) {
	t.Parallel()

	store := NewUserStore()
	user := domain.User{
		ID:           "usr-1",
		Email:        "alice@example.com",
		PasswordHash: "hash",
		CreatedAt:    time.Now().UTC(),
	}

	if err := store.Create(context.Background(), user); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if err := store.UpdateGitHubToken(context.Background(), "usr-1", "enc:ghp_token"); err != nil {
		t.Fatalf("UpdateGitHubToken returned error: %v", err)
	}

	updated, ok := store.GetByID(context.Background(), "usr-1")
	if !ok {
		t.Fatal("expected updated user in store")
	}
	if updated.GitHubTokenEncrypted != "enc:ghp_token" {
		t.Fatalf("expected encrypted token to be persisted, got %q", updated.GitHubTokenEncrypted)
	}
}
