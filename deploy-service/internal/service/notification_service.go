package service

import (
	"context"
	"deploy-service/internal/domain"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

type TelegramSender interface {
	SendMessage(ctx context.Context, chatID int64, text string) error
}

type NotificationService struct {
	users              UserStore
	sender             TelegramSender
	botUsername        string
	adminTelegramUsers []string

	mu             sync.Mutex
	deliveredUntil map[string]time.Time
}

type TelegramUpdate struct {
	Message *TelegramMessage `json:"message,omitempty"`
}

type TelegramMessage struct {
	Text string `json:"text"`
	Chat struct {
		ID int64 `json:"id"`
	} `json:"chat"`
	From struct {
		Username string `json:"username"`
	} `json:"from"`
}

func NewNotificationService(users UserStore, sender TelegramSender, botUsername string) *NotificationService {
	return &NotificationService{
		users:          users,
		sender:         sender,
		botUsername:    strings.TrimPrefix(strings.TrimSpace(botUsername), "@"),
		deliveredUntil: make(map[string]time.Time),
	}
}

func (s *NotificationService) SetAdminTelegramUsernames(usernames []string) {
	normalized := make([]string, 0, len(usernames))
	seen := make(map[string]struct{}, len(usernames))
	for _, username := range usernames {
		value := normalizeTelegramUsername(username)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, value)
	}
	s.adminTelegramUsers = normalized
}

func (s *NotificationService) GetTelegramSettings(ctx context.Context, userID string) (domain.TelegramSettings, error) {
	user, exists := s.users.GetByID(ctx, userID)
	if !exists {
		return domain.TelegramSettings{}, domain.ErrUserNotFound
	}
	return s.buildTelegramSettings(user), nil
}

func (s *NotificationService) UpdateTelegramSettings(ctx context.Context, userID string, req domain.UpdateTelegramSettingsRequest) (domain.TelegramSettings, error) {
	user, exists := s.users.GetByID(ctx, userID)
	if !exists {
		return domain.TelegramSettings{}, domain.ErrUserNotFound
	}

	username := normalizeTelegramUsername(req.Username)
	if username == "" {
		if err := s.users.ClearTelegramSettings(ctx, userID); err != nil {
			return domain.TelegramSettings{}, err
		}
		return domain.TelegramSettings{}, nil
	}

	enabled := req.NotificationsEnabled
	linkCode := user.TelegramLinkCode
	linkExpiresAt := user.TelegramLinkExpiresAt
	usernameChanged := !strings.EqualFold(user.TelegramUsername, username)
	if user.TelegramChatID == 0 || usernameChanged || linkCode == "" || linkExpiresAt == nil || time.Now().UTC().After(*linkExpiresAt) {
		expiresAt := time.Now().UTC().Add(24 * time.Hour)
		linkExpiresAt = &expiresAt
		linkCode = generateTelegramLinkCode()
	}

	if err := s.users.UpdateTelegramSettings(ctx, userID, username, linkCode, linkExpiresAt, enabled); err != nil {
		return domain.TelegramSettings{}, err
	}

	user.TelegramUsername = username
	user.TelegramLinkCode = linkCode
	user.TelegramLinkExpiresAt = linkExpiresAt
	user.TelegramNotificationsEnabled = enabled
	if usernameChanged {
		user.TelegramChatID = 0
		user.TelegramLinkedAt = nil
	}
	return s.buildTelegramSettings(user), nil
}

func (s *NotificationService) ClearTelegramSettings(ctx context.Context, userID string) (domain.TelegramSettings, error) {
	if err := s.users.ClearTelegramSettings(ctx, userID); err != nil {
		return domain.TelegramSettings{}, err
	}
	return domain.TelegramSettings{}, nil
}

func (s *NotificationService) HandleTelegramUpdate(ctx context.Context, update TelegramUpdate) error {
	if update.Message == nil || update.Message.Chat.ID == 0 {
		return nil
	}

	text := strings.TrimSpace(update.Message.Text)
	switch {
	case strings.HasPrefix(text, "/start"):
		return s.handleStart(ctx, update.Message.Chat.ID, update.Message.From.Username, text)
	case strings.HasPrefix(text, "/stop"):
		return s.handleStop(ctx, update.Message.Chat.ID)
	default:
		return s.reply(ctx, update.Message.Chat.ID, s.helpMessage())
	}
}

func (s *NotificationService) SendUserAlert(ctx context.Context, userID, key, text string, ttl time.Duration) error {
	if s.sender == nil {
		return nil
	}
	user, exists := s.users.GetByID(ctx, userID)
	if !exists || user.TelegramChatID == 0 || !user.TelegramNotificationsEnabled {
		return nil
	}

	now := time.Now().UTC()
	cacheKey := userID + ":" + key
	s.mu.Lock()
	if until, ok := s.deliveredUntil[cacheKey]; ok && now.Before(until) {
		s.mu.Unlock()
		return nil
	}
	s.deliveredUntil[cacheKey] = now.Add(ttl)
	s.mu.Unlock()

	if err := s.sender.SendMessage(ctx, user.TelegramChatID, text); err != nil {
		log.Printf("telegram notification failed for user %s: %v", userID, err)
		return err
	}
	return nil
}

func (s *NotificationService) SendSystemAlert(ctx context.Context, key, text string, ttl time.Duration) error {
	if s.sender == nil || len(s.adminTelegramUsers) == 0 {
		return nil
	}
	text = formatSystemAlertText(text)

	var firstErr error
	for _, username := range s.adminTelegramUsers {
		user, exists := s.users.GetByTelegramUsername(ctx, username)
		if !exists || user.TelegramChatID == 0 {
			continue
		}

		now := time.Now().UTC()
		cacheKey := "admin:" + user.ID + ":" + key
		s.mu.Lock()
		if until, ok := s.deliveredUntil[cacheKey]; ok && now.Before(until) {
			s.mu.Unlock()
			continue
		}
		s.deliveredUntil[cacheKey] = now.Add(ttl)
		s.mu.Unlock()

		if err := s.sender.SendMessage(ctx, user.TelegramChatID, text); err != nil {
			log.Printf("telegram system notification failed for admin %s: %v", username, err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func formatSystemAlertText(text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return "[admin]"
	}
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "[admin]") {
		return trimmed
	}
	return "[admin] " + trimmed
}

func (s *NotificationService) buildTelegramSettings(user domain.User) domain.TelegramSettings {
	settings := domain.TelegramSettings{
		Username:             user.TelegramUsername,
		Linked:               user.TelegramChatID != 0,
		NotificationsEnabled: user.TelegramNotificationsEnabled,
		BotUsername:          s.botUsername,
		ConnectedAt:          user.TelegramLinkedAt,
	}
	if user.TelegramChatID == 0 && user.TelegramLinkCode != "" {
		settings.LinkCode = user.TelegramLinkCode
		settings.LinkExpiresAt = user.TelegramLinkExpiresAt
		if s.botUsername != "" {
			settings.DeepLinkURL = fmt.Sprintf("https://t.me/%s?start=%s", s.botUsername, user.TelegramLinkCode)
		}
	}
	return settings
}

func (s *NotificationService) handleStart(ctx context.Context, chatID int64, senderUsername, text string) error {
	code := strings.TrimSpace(strings.TrimPrefix(text, "/start"))
	if code == "" {
		return s.reply(ctx, chatID, s.helpMessage())
	}

	user, exists := s.users.GetByTelegramLinkCode(ctx, code)
	if !exists {
		return s.reply(ctx, chatID, "Код привязки не найден или уже устарел. Открой настройки Telegram в DeployService и запроси новый код.")
	}
	if user.TelegramLinkExpiresAt == nil || time.Now().UTC().After(*user.TelegramLinkExpiresAt) {
		return s.reply(ctx, chatID, "Срок действия кода истек. Открой настройки Telegram в DeployService и сгенерируй новый.")
	}
	if s.isAdminUsername(user.TelegramUsername) {
		normalizedSender := normalizeTelegramUsername(senderUsername)
		if normalizedSender == "" || !s.isAdminUsername(normalizedSender) || !strings.EqualFold(normalizedSender, user.TelegramUsername) {
			return s.reply(ctx, chatID, "Для админского аккаунта нужен тот же Telegram username, который добавлен в конфиг сервиса, и одноразовый код из интерфейса.")
		}
	}
	if err := s.users.LinkTelegramChat(ctx, user.ID, chatID, time.Now().UTC()); err != nil {
		return err
	}
	return s.reply(ctx, chatID, fmt.Sprintf("Подключение завершено. Буду присылать уведомления по аккаунту %s и его проектам.", user.Email))
}

func (s *NotificationService) handleStop(ctx context.Context, chatID int64) error {
	user, exists := s.users.GetByTelegramChatID(ctx, chatID)
	if !exists {
		return s.reply(ctx, chatID, "Этот чат пока не связан ни с одним аккаунтом DeployService.")
	}
	if err := s.users.DisableTelegramNotifications(ctx, user.ID); err != nil {
		return err
	}
	return s.reply(ctx, chatID, "Уведомления остановлены. Включить их снова можно в интерфейсе DeployService.")
}

func (s *NotificationService) reply(ctx context.Context, chatID int64, text string) error {
	if s.sender == nil {
		return nil
	}
	return s.sender.SendMessage(ctx, chatID, text)
}

func (s *NotificationService) helpMessage() string {
	return "Привет. Открой DeployService, укажи Telegram username и перейди по кнопке подключения или отправь команду /start <код>."
}

func (s *NotificationService) isAdminUsername(username string) bool {
	normalized := normalizeTelegramUsername(username)
	if normalized == "" {
		return false
	}
	for _, adminUsername := range s.adminTelegramUsers {
		if strings.EqualFold(adminUsername, normalized) {
			return true
		}
	}
	return false
}
