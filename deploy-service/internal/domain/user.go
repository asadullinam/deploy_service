package domain

import (
	"errors"
	"time"
)

var ErrUserNotFound = errors.New("user not found")
var ErrEmailTaken = errors.New("email already taken")
var ErrInvalidCredentials = errors.New("invalid credentials")
var ErrForbidden = errors.New("forbidden")
var ErrInsufficientBalance = errors.New("insufficient balance")

type User struct {
	ID                           string     `json:"id"`
	Email                        string     `json:"email"`
	BalanceRUB                   float64    `json:"balanceRub"`
	PasswordHash                 string     `json:"-"`
	GitHubTokenEncrypted         string     `json:"-"`
	TelegramUsername             string     `json:"-"`
	TelegramChatID               int64      `json:"-"`
	TelegramLinkedAt             *time.Time `json:"-"`
	TelegramLinkCode             string     `json:"-"`
	TelegramLinkExpiresAt        *time.Time `json:"-"`
	TelegramNotificationsEnabled bool       `json:"-"`
	CreatedAt                    time.Time  `json:"createdAt"`
}

type RegisterRequest struct {
	Email            string `json:"email"`
	Password         string `json:"password"`
	TelegramUsername string `json:"telegramUsername,omitempty"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type TokenResponse struct {
	Token string `json:"token"`
}

type UpsertServiceGitHubTokenRequest struct {
	Token string `json:"token"`
}

type ServiceGitHubTokenStatus struct {
	Configured bool `json:"configured"`
}

type TelegramSettings struct {
	Username             string     `json:"username"`
	Linked               bool       `json:"linked"`
	NotificationsEnabled bool       `json:"notificationsEnabled"`
	LinkCode             string     `json:"linkCode,omitempty"`
	LinkExpiresAt        *time.Time `json:"linkExpiresAt,omitempty"`
	DeepLinkURL          string     `json:"deepLinkUrl,omitempty"`
	BotUsername          string     `json:"botUsername,omitempty"`
	ConnectedAt          *time.Time `json:"connectedAt,omitempty"`
}

type UpdateTelegramSettingsRequest struct {
	Username             string `json:"username"`
	NotificationsEnabled bool   `json:"notificationsEnabled"`
}
