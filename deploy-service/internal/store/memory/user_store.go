package memory

import (
	"context"
	"deploy-service/internal/domain"
	"deploy-service/internal/service"
	"strings"
	"time"
)

var _ service.UserStore = (*UserStore)(nil)

func NewUserStore() *UserStore {
	return &UserStore{byID: make(map[string]domain.User), byEmail: make(map[string]domain.User)}
}

func (s *UserStore) Create(_ context.Context, user domain.User) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.byEmail[user.Email]; exists {
		return domain.ErrEmailTaken
	}
	s.byID[user.ID] = user
	s.byEmail[user.Email] = user
	return nil
}

func (s *UserStore) GetByEmail(_ context.Context, email string) (domain.User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	u, ok := s.byEmail[email]
	return u, ok
}

func (s *UserStore) GetByID(_ context.Context, userID string) (domain.User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	u, ok := s.byID[userID]
	return u, ok
}

func (s *UserStore) GetByTelegramUsername(_ context.Context, username string) (domain.User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	normalized := strings.TrimPrefix(strings.TrimSpace(username), "@")
	for _, user := range s.byID {
		if strings.EqualFold(user.TelegramUsername, normalized) {
			return user, true
		}
	}
	return domain.User{}, false
}

func (s *UserStore) GetByTelegramLinkCode(_ context.Context, code string) (domain.User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, user := range s.byID {
		if user.TelegramLinkCode == code {
			return user, true
		}
	}
	return domain.User{}, false
}

func (s *UserStore) GetByTelegramChatID(_ context.Context, chatID int64) (domain.User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, user := range s.byID {
		if user.TelegramChatID == chatID {
			return user, true
		}
	}
	return domain.User{}, false
}

func (s *UserStore) UpdateBalance(_ context.Context, userID string, balanceUSD float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.byID[userID]
	if !ok {
		return domain.ErrUserNotFound
	}
	user.BalanceRUB = balanceUSD
	s.byID[userID] = user
	s.byEmail[user.Email] = user
	return nil
}

func (s *UserStore) UpdateGitHubToken(_ context.Context, userID, encryptedToken string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.byID[userID]
	if !ok {
		return domain.ErrUserNotFound
	}
	user.GitHubTokenEncrypted = encryptedToken
	s.byID[userID] = user
	s.byEmail[user.Email] = user
	return nil
}

func (s *UserStore) UpdateTelegramSettings(_ context.Context, userID, username, linkCode string, linkExpiresAt *time.Time, enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.byID[userID]
	if !ok {
		return domain.ErrUserNotFound
	}
	user.TelegramUsername = username
	user.TelegramLinkCode = linkCode
	user.TelegramLinkExpiresAt = linkExpiresAt
	user.TelegramNotificationsEnabled = enabled
	if user.TelegramUsername == "" {
		user.TelegramChatID = 0
		user.TelegramLinkedAt = nil
		user.TelegramLinkCode = ""
		user.TelegramLinkExpiresAt = nil
		user.TelegramNotificationsEnabled = false
	}
	s.byID[userID] = user
	s.byEmail[user.Email] = user
	return nil
}

func (s *UserStore) LinkTelegramChat(_ context.Context, userID string, chatID int64, linkedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.byID[userID]
	if !ok {
		return domain.ErrUserNotFound
	}
	user.TelegramChatID = chatID
	user.TelegramLinkedAt = &linkedAt
	user.TelegramLinkCode = ""
	user.TelegramLinkExpiresAt = nil
	user.TelegramNotificationsEnabled = true
	s.byID[userID] = user
	s.byEmail[user.Email] = user
	return nil
}

func (s *UserStore) DisableTelegramNotifications(_ context.Context, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.byID[userID]
	if !ok {
		return domain.ErrUserNotFound
	}
	user.TelegramNotificationsEnabled = false
	s.byID[userID] = user
	s.byEmail[user.Email] = user
	return nil
}

func (s *UserStore) ClearTelegramSettings(_ context.Context, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.byID[userID]
	if !ok {
		return domain.ErrUserNotFound
	}
	user.TelegramUsername = ""
	user.TelegramChatID = 0
	user.TelegramLinkedAt = nil
	user.TelegramLinkCode = ""
	user.TelegramLinkExpiresAt = nil
	user.TelegramNotificationsEnabled = false
	s.byID[userID] = user
	s.byEmail[user.Email] = user
	return nil
}
