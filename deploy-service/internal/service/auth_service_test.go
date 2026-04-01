//go:build !integration

package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"deploy-service/internal/auth"
	"deploy-service/internal/domain"
)

type userStoreStub struct {
	usersByEmail map[string]domain.User
	usersByID    map[string]domain.User
}

func newUserStoreStub(users ...domain.User) *userStoreStub {
	store := &userStoreStub{
		usersByEmail: make(map[string]domain.User),
		usersByID:    make(map[string]domain.User),
	}
	for _, user := range users {
		store.usersByEmail[user.Email] = user
		store.usersByID[user.ID] = user
	}
	return store
}

func (s *userStoreStub) Create(_ context.Context, user domain.User) error {
	s.usersByEmail[user.Email] = user
	s.usersByID[user.ID] = user
	return nil
}

func (s *userStoreStub) GetByEmail(_ context.Context, email string) (domain.User, bool) {
	user, ok := s.usersByEmail[email]
	return user, ok
}

func (s *userStoreStub) GetByID(_ context.Context, userID string) (domain.User, bool) {
	user, ok := s.usersByID[userID]
	return user, ok
}

func (s *userStoreStub) GetByTelegramUsername(_ context.Context, username string) (domain.User, bool) {
	normalized := normalizeTelegramUsername(username)
	for _, user := range s.usersByID {
		if strings.EqualFold(user.TelegramUsername, normalized) {
			return user, true
		}
	}
	return domain.User{}, false
}

func (s *userStoreStub) GetByTelegramLinkCode(_ context.Context, code string) (domain.User, bool) {
	for _, user := range s.usersByID {
		if user.TelegramLinkCode == code {
			return user, true
		}
	}
	return domain.User{}, false
}

func (s *userStoreStub) GetByTelegramChatID(_ context.Context, chatID int64) (domain.User, bool) {
	for _, user := range s.usersByID {
		if user.TelegramChatID == chatID {
			return user, true
		}
	}
	return domain.User{}, false
}

func (s *userStoreStub) UpdateBalance(_ context.Context, userID string, balanceRUB float64) error {
	user, ok := s.usersByID[userID]
	if !ok {
		return domain.ErrUserNotFound
	}
	user.BalanceRUB = balanceRUB
	s.usersByID[userID] = user
	s.usersByEmail[user.Email] = user
	return nil
}

func (s *userStoreStub) UpdateGitHubToken(_ context.Context, userID, encryptedToken string) error {
	user, ok := s.usersByID[userID]
	if !ok {
		return domain.ErrUserNotFound
	}
	user.GitHubTokenEncrypted = encryptedToken
	s.usersByID[userID] = user
	s.usersByEmail[user.Email] = user
	return nil
}

func (s *userStoreStub) UpdateTelegramSettings(_ context.Context, userID, username, linkCode string, linkExpiresAt *time.Time, enabled bool) error {
	user, ok := s.usersByID[userID]
	if !ok {
		return domain.ErrUserNotFound
	}
	user.TelegramUsername = username
	user.TelegramLinkCode = linkCode
	user.TelegramLinkExpiresAt = linkExpiresAt
	user.TelegramNotificationsEnabled = enabled
	if username == "" {
		user.TelegramChatID = 0
		user.TelegramLinkedAt = nil
		user.TelegramLinkCode = ""
		user.TelegramLinkExpiresAt = nil
		user.TelegramNotificationsEnabled = false
	}
	s.usersByID[userID] = user
	s.usersByEmail[user.Email] = user
	return nil
}

func (s *userStoreStub) LinkTelegramChat(_ context.Context, userID string, chatID int64, linkedAt time.Time) error {
	user, ok := s.usersByID[userID]
	if !ok {
		return domain.ErrUserNotFound
	}
	user.TelegramChatID = chatID
	user.TelegramLinkedAt = &linkedAt
	user.TelegramLinkCode = ""
	user.TelegramLinkExpiresAt = nil
	user.TelegramNotificationsEnabled = true
	s.usersByID[userID] = user
	s.usersByEmail[user.Email] = user
	return nil
}

func (s *userStoreStub) DisableTelegramNotifications(_ context.Context, userID string) error {
	user, ok := s.usersByID[userID]
	if !ok {
		return domain.ErrUserNotFound
	}
	user.TelegramNotificationsEnabled = false
	s.usersByID[userID] = user
	s.usersByEmail[user.Email] = user
	return nil
}

func (s *userStoreStub) ClearTelegramSettings(_ context.Context, userID string) error {
	user, ok := s.usersByID[userID]
	if !ok {
		return domain.ErrUserNotFound
	}
	user.TelegramUsername = ""
	user.TelegramChatID = 0
	user.TelegramLinkedAt = nil
	user.TelegramLinkCode = ""
	user.TelegramLinkExpiresAt = nil
	user.TelegramNotificationsEnabled = false
	s.usersByID[userID] = user
	s.usersByEmail[user.Email] = user
	return nil
}

func TestRegisterCreatesUserAndReturnsJWT(t *testing.T) {
	t.Parallel()

	users := newUserStoreStub()
	svc := NewAuthService(users, &txStoreStub{}, "secret", time.Hour, 12.5)

	resp, err := svc.Register(context.Background(), domain.RegisterRequest{
		Email:    "alice@example.com",
		Password: "strong-password",
	})
	if err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	stored, ok := users.GetByEmail(context.Background(), "alice@example.com")
	if !ok {
		t.Fatal("expected created user in store")
	}
	if stored.PasswordHash == "" || stored.PasswordHash == "strong-password" {
		t.Fatalf("expected password hash to be stored, got %q", stored.PasswordHash)
	}
	if stored.BalanceRUB != 12.5 {
		t.Fatalf("expected default balance 12.5, got %.2f", stored.BalanceRUB)
	}

	claims, err := auth.ValidateClaims(resp.Token, "secret")
	if err != nil {
		t.Fatalf("expected valid JWT, got error: %v", err)
	}
	if claims.UserID != stored.ID {
		t.Fatalf("expected token user id %q, got %q", stored.ID, claims.UserID)
	}
	if claims.Email != stored.Email {
		t.Fatalf("expected token email %q, got %q", stored.Email, claims.Email)
	}
}

func TestRegisterRejectsDuplicateEmail(t *testing.T) {
	t.Parallel()

	users := newUserStoreStub(domain.User{
		ID:           "usr-1",
		Email:        "alice@example.com",
		PasswordHash: "hash",
		CreatedAt:    time.Now().UTC(),
	})
	svc := NewAuthService(users, &txStoreStub{}, "secret", time.Hour, 0)

	_, err := svc.Register(context.Background(), domain.RegisterRequest{
		Email:    "alice@example.com",
		Password: "strong-password",
	})
	if err != domain.ErrEmailTaken {
		t.Fatalf("expected ErrEmailTaken, got %v", err)
	}
}

func TestLoginRejectsInvalidPassword(t *testing.T) {
	t.Parallel()

	users := newUserStoreStub()
	svc := NewAuthService(users, &txStoreStub{}, "secret", time.Hour, 0)
	registerResp, err := svc.Register(context.Background(), domain.RegisterRequest{
		Email:    "alice@example.com",
		Password: "strong-password",
	})
	if err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	if registerResp.Token == "" {
		t.Fatal("expected register token")
	}

	_, err = svc.Login(context.Background(), domain.LoginRequest{
		Email:    "alice@example.com",
		Password: "wrong-password",
	})
	if err != domain.ErrInvalidCredentials {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestTopUpBalanceUpdatesStoredAmount(t *testing.T) {
	t.Parallel()

	users := newUserStoreStub(domain.User{
		ID:         "usr-1",
		Email:      "alice@example.com",
		BalanceRUB: 10,
	})
	svc := NewAuthService(users, &txStoreStub{}, "secret", time.Hour, 0)

	updated, err := svc.TopUpBalance(context.Background(), "usr-1", 1000)
	if err != nil {
		t.Fatalf("TopUpBalance returned error: %v", err)
	}
	if updated.BalanceRUB != 1010 {
		t.Fatalf("expected balance 1010, got %.2f", updated.BalanceRUB)
	}
}
