package postgres

import (
	"context"
	"deploy-service/internal/domain"
	"deploy-service/internal/service"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"strings"
	"time"
)

var _ service.UserStore = (*UserStore)(nil)

func NewUserStore(pool *pgxpool.Pool) *UserStore {
	return &UserStore{pool: pool}
}

func normalizedTelegramUsername(value string) string {
	return strings.TrimPrefix(strings.TrimSpace(value), "@")
}

func (s *UserStore) Create(ctx context.Context, user domain.User) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO users (
			id, email, balance_rub, password_hash, github_token_encrypted,
			telegram_username, telegram_chat_id, telegram_linked_at, telegram_link_code,
			telegram_link_expires_at, telegram_notifications_enabled, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		user.ID, user.Email, user.BalanceRUB, user.PasswordHash, user.GitHubTokenEncrypted,
		user.TelegramUsername, user.TelegramChatID, user.TelegramLinkedAt, user.TelegramLinkCode,
		user.TelegramLinkExpiresAt, user.TelegramNotificationsEnabled, user.CreatedAt,
	)
	return err
}

func (s *UserStore) GetByEmail(ctx context.Context, email string) (domain.User, bool) {
	row := s.pool.QueryRow(ctx,
		`SELECT
			id, email, balance_rub, password_hash, github_token_encrypted,
			telegram_username, telegram_chat_id, telegram_linked_at, telegram_link_code,
			telegram_link_expires_at, telegram_notifications_enabled, created_at
		FROM users WHERE email = $1`, email,
	)
	var u domain.User
	if err := row.Scan(
		&u.ID, &u.Email, &u.BalanceRUB, &u.PasswordHash, &u.GitHubTokenEncrypted,
		&u.TelegramUsername, &u.TelegramChatID, &u.TelegramLinkedAt, &u.TelegramLinkCode,
		&u.TelegramLinkExpiresAt, &u.TelegramNotificationsEnabled, &u.CreatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.User{}, false
		}
		return domain.User{}, false
	}
	return u, true
}

func (s *UserStore) GetByID(ctx context.Context, userID string) (domain.User, bool) {
	row := s.pool.QueryRow(ctx,
		`SELECT
			id, email, balance_rub, password_hash, github_token_encrypted,
			telegram_username, telegram_chat_id, telegram_linked_at, telegram_link_code,
			telegram_link_expires_at, telegram_notifications_enabled, created_at
		FROM users WHERE id = $1`, userID,
	)
	var u domain.User
	if err := row.Scan(
		&u.ID, &u.Email, &u.BalanceRUB, &u.PasswordHash, &u.GitHubTokenEncrypted,
		&u.TelegramUsername, &u.TelegramChatID, &u.TelegramLinkedAt, &u.TelegramLinkCode,
		&u.TelegramLinkExpiresAt, &u.TelegramNotificationsEnabled, &u.CreatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.User{}, false
		}
		return domain.User{}, false
	}
	return u, true
}

func (s *UserStore) GetByTelegramUsername(ctx context.Context, username string) (domain.User, bool) {
	normalized := normalizedTelegramUsername(username)
	if normalized == "" {
		return domain.User{}, false
	}
	row := s.pool.QueryRow(ctx,
		`SELECT
			id, email, balance_rub, password_hash, github_token_encrypted,
			telegram_username, telegram_chat_id, telegram_linked_at, telegram_link_code,
			telegram_link_expires_at, telegram_notifications_enabled, created_at
		FROM users WHERE lower(telegram_username) = lower($1)`, normalized,
	)
	var u domain.User
	if err := row.Scan(
		&u.ID, &u.Email, &u.BalanceRUB, &u.PasswordHash, &u.GitHubTokenEncrypted,
		&u.TelegramUsername, &u.TelegramChatID, &u.TelegramLinkedAt, &u.TelegramLinkCode,
		&u.TelegramLinkExpiresAt, &u.TelegramNotificationsEnabled, &u.CreatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.User{}, false
		}
		return domain.User{}, false
	}
	return u, true
}

func (s *UserStore) GetByTelegramLinkCode(ctx context.Context, code string) (domain.User, bool) {
	row := s.pool.QueryRow(ctx,
		`SELECT
			id, email, balance_rub, password_hash, github_token_encrypted,
			telegram_username, telegram_chat_id, telegram_linked_at, telegram_link_code,
			telegram_link_expires_at, telegram_notifications_enabled, created_at
		FROM users WHERE telegram_link_code = $1`, code,
	)
	var u domain.User
	if err := row.Scan(
		&u.ID, &u.Email, &u.BalanceRUB, &u.PasswordHash, &u.GitHubTokenEncrypted,
		&u.TelegramUsername, &u.TelegramChatID, &u.TelegramLinkedAt, &u.TelegramLinkCode,
		&u.TelegramLinkExpiresAt, &u.TelegramNotificationsEnabled, &u.CreatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.User{}, false
		}
		return domain.User{}, false
	}
	return u, true
}

func (s *UserStore) GetByTelegramChatID(ctx context.Context, chatID int64) (domain.User, bool) {
	row := s.pool.QueryRow(ctx,
		`SELECT
			id, email, balance_rub, password_hash, github_token_encrypted,
			telegram_username, telegram_chat_id, telegram_linked_at, telegram_link_code,
			telegram_link_expires_at, telegram_notifications_enabled, created_at
		FROM users WHERE telegram_chat_id = $1`, chatID,
	)
	var u domain.User
	if err := row.Scan(
		&u.ID, &u.Email, &u.BalanceRUB, &u.PasswordHash, &u.GitHubTokenEncrypted,
		&u.TelegramUsername, &u.TelegramChatID, &u.TelegramLinkedAt, &u.TelegramLinkCode,
		&u.TelegramLinkExpiresAt, &u.TelegramNotificationsEnabled, &u.CreatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.User{}, false
		}
		return domain.User{}, false
	}
	return u, true
}

func (s *UserStore) UpdateBalance(ctx context.Context, userID string, balanceUSD float64) error {
	tag, err := s.pool.Exec(ctx, `UPDATE users SET balance_rub = $1 WHERE id = $2`, balanceUSD, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrUserNotFound
	}
	return nil
}

func (s *UserStore) UpdateGitHubToken(ctx context.Context, userID, encryptedToken string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE users SET github_token_encrypted = $1 WHERE id = $2`, encryptedToken, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrUserNotFound
	}
	return nil
}

func (s *UserStore) UpdateTelegramSettings(ctx context.Context, userID, username, linkCode string, linkExpiresAt *time.Time, enabled bool) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE users
		SET telegram_username = $1,
		    telegram_link_code = $2,
		    telegram_link_expires_at = $3,
		    telegram_notifications_enabled = $4,
		    telegram_chat_id = CASE WHEN $1 = '' THEN 0 ELSE telegram_chat_id END,
		    telegram_linked_at = CASE WHEN $1 = '' THEN NULL ELSE telegram_linked_at END
		WHERE id = $5
	`, username, linkCode, linkExpiresAt, enabled, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrUserNotFound
	}
	return nil
}

func (s *UserStore) LinkTelegramChat(ctx context.Context, userID string, chatID int64, linkedAt time.Time) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE users
		SET telegram_chat_id = $1,
		    telegram_linked_at = $2,
		    telegram_link_code = '',
		    telegram_link_expires_at = NULL,
		    telegram_notifications_enabled = TRUE
		WHERE id = $3
	`, chatID, linkedAt, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrUserNotFound
	}
	return nil
}

func (s *UserStore) DisableTelegramNotifications(ctx context.Context, userID string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE users SET telegram_notifications_enabled = FALSE WHERE id = $1`, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrUserNotFound
	}
	return nil
}

func (s *UserStore) ClearTelegramSettings(ctx context.Context, userID string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE users
		SET telegram_username = '',
		    telegram_chat_id = 0,
		    telegram_linked_at = NULL,
		    telegram_link_code = '',
		    telegram_link_expires_at = NULL,
		    telegram_notifications_enabled = FALSE
		WHERE id = $1
	`, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrUserNotFound
	}
	return nil
}
