//go:build !integration

package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"deploy-service/internal/domain"
	integrationtelegram "deploy-service/internal/integration/telegram"
)

type telegramSenderStub struct {
	chatID int64
	text   string
	calls  int
}

func (s *telegramSenderStub) SendMessage(_ context.Context, chatID int64, text string) error {
	s.chatID = chatID
	s.text = text
	s.calls++
	return nil
}

func TestNotificationServiceUpdateTelegramSettingsGeneratesLinkCode(t *testing.T) {
	t.Parallel()

	users := newUserStoreStub(domain.User{
		ID:        "usr-1",
		Email:     "alice@example.com",
		CreatedAt: time.Now().UTC(),
	})
	svc := NewNotificationService(users, &telegramSenderStub{}, "deploysvc_bot")

	settings, err := svc.UpdateTelegramSettings(context.Background(), "usr-1", domain.UpdateTelegramSettingsRequest{
		Username:             "@alice_dev",
		NotificationsEnabled: true,
	})
	if err != nil {
		t.Fatalf("UpdateTelegramSettings returned error: %v", err)
	}
	if settings.Username != "alice_dev" {
		t.Fatalf("expected normalized username, got %q", settings.Username)
	}
	if settings.LinkCode == "" {
		t.Fatal("expected link code to be generated")
	}
	if settings.DeepLinkURL == "" {
		t.Fatal("expected deep link url to be generated")
	}
	if settings.Linked {
		t.Fatal("expected telegram to remain unlinked until /start")
	}
}

func TestNotificationServiceHandleTelegramUpdateLinksChat(t *testing.T) {
	t.Parallel()

	expiresAt := time.Now().UTC().Add(time.Hour)
	users := newUserStoreStub(domain.User{
		ID:                           "usr-1",
		Email:                        "alice@example.com",
		TelegramUsername:             "alice_dev",
		TelegramLinkCode:             "link-code-123",
		TelegramLinkExpiresAt:        &expiresAt,
		TelegramNotificationsEnabled: true,
		CreatedAt:                    time.Now().UTC(),
	})
	sender := &telegramSenderStub{}
	svc := NewNotificationService(users, sender, "deploysvc_bot")

	err := svc.HandleTelegramUpdate(context.Background(), TelegramUpdate{
		Message: &TelegramMessage{
			Text: "/start link-code-123",
			Chat: struct {
				ID int64 `json:"id"`
			}{ID: 777},
			From: struct {
				Username string `json:"username"`
			}{Username: "alice_dev"},
		},
	})
	if err != nil {
		t.Fatalf("HandleTelegramUpdate returned error: %v", err)
	}

	user, ok := users.GetByID(context.Background(), "usr-1")
	if !ok {
		t.Fatal("expected user to remain in store")
	}
	if user.TelegramChatID != 777 {
		t.Fatalf("expected telegram chat id 777, got %d", user.TelegramChatID)
	}
	if user.TelegramLinkedAt == nil {
		t.Fatal("expected linked timestamp to be set")
	}
	if sender.calls != 1 {
		t.Fatalf("expected one confirmation message, got %d", sender.calls)
	}
}

func TestNotificationServiceHandleTelegramUpdateStartWithoutCodeShowsHelp(t *testing.T) {
	t.Parallel()

	users := newUserStoreStub()
	sender := &telegramSenderStub{}
	svc := NewNotificationService(users, sender, "deploysvc_bot")

	err := svc.HandleTelegramUpdate(context.Background(), TelegramUpdate{
		Message: &TelegramMessage{
			Text: "/start",
			Chat: struct {
				ID int64 `json:"id"`
			}{ID: 701},
			From: struct {
				Username string `json:"username"`
			}{Username: "alice_dev"},
		},
	})
	if err != nil {
		t.Fatalf("HandleTelegramUpdate returned error: %v", err)
	}
	if sender.calls != 1 {
		t.Fatalf("expected one help message, got %d", sender.calls)
	}
	if sender.chatID != 701 {
		t.Fatalf("expected reply to chat 701, got %d", sender.chatID)
	}
	if sender.text != svc.helpMessage() {
		t.Fatalf("expected help message, got %q", sender.text)
	}
}

func TestNotificationServiceHandleTelegramUpdateInvalidCode(t *testing.T) {
	t.Parallel()

	users := newUserStoreStub()
	sender := &telegramSenderStub{}
	svc := NewNotificationService(users, sender, "deploysvc_bot")

	err := svc.HandleTelegramUpdate(context.Background(), TelegramUpdate{
		Message: &TelegramMessage{
			Text: "/start missing-code",
			Chat: struct {
				ID int64 `json:"id"`
			}{ID: 702},
			From: struct {
				Username string `json:"username"`
			}{Username: "alice_dev"},
		},
	})
	if err != nil {
		t.Fatalf("HandleTelegramUpdate returned error: %v", err)
	}
	if sender.calls != 1 {
		t.Fatalf("expected one reply, got %d", sender.calls)
	}
	if sender.text != "Код привязки не найден или уже устарел. Открой настройки Telegram в DeployService и запроси новый код." {
		t.Fatalf("unexpected response: %q", sender.text)
	}
}

func TestNotificationServiceHandleTelegramUpdateExpiredCode(t *testing.T) {
	t.Parallel()

	expiresAt := time.Now().UTC().Add(-time.Minute)
	users := newUserStoreStub(domain.User{
		ID:                    "usr-1",
		Email:                 "alice@example.com",
		TelegramUsername:      "alice_dev",
		TelegramLinkCode:      "expired-code",
		TelegramLinkExpiresAt: &expiresAt,
		CreatedAt:             time.Now().UTC(),
	})
	sender := &telegramSenderStub{}
	svc := NewNotificationService(users, sender, "deploysvc_bot")

	err := svc.HandleTelegramUpdate(context.Background(), TelegramUpdate{
		Message: &TelegramMessage{
			Text: "/start expired-code",
			Chat: struct {
				ID int64 `json:"id"`
			}{ID: 703},
			From: struct {
				Username string `json:"username"`
			}{Username: "alice_dev"},
		},
	})
	if err != nil {
		t.Fatalf("HandleTelegramUpdate returned error: %v", err)
	}
	if sender.calls != 1 {
		t.Fatalf("expected one reply, got %d", sender.calls)
	}
	if sender.text != "Срок действия кода истек. Открой настройки Telegram в DeployService и сгенерируй новый." {
		t.Fatalf("unexpected response: %q", sender.text)
	}
}

func TestNotificationServiceHandleTelegramUpdateStopWithoutLinkedChat(t *testing.T) {
	t.Parallel()

	users := newUserStoreStub()
	sender := &telegramSenderStub{}
	svc := NewNotificationService(users, sender, "deploysvc_bot")

	err := svc.HandleTelegramUpdate(context.Background(), TelegramUpdate{
		Message: &TelegramMessage{
			Text: "/stop",
			Chat: struct {
				ID int64 `json:"id"`
			}{ID: 704},
			From: struct {
				Username string `json:"username"`
			}{Username: "alice_dev"},
		},
	})
	if err != nil {
		t.Fatalf("HandleTelegramUpdate returned error: %v", err)
	}
	if sender.calls != 1 {
		t.Fatalf("expected one reply, got %d", sender.calls)
	}
	if sender.text != "Этот чат пока не связан ни с одним аккаунтом DeployService." {
		t.Fatalf("unexpected response: %q", sender.text)
	}
}

func TestNotificationServiceHandleTelegramUpdateStopDisablesNotifications(t *testing.T) {
	t.Parallel()

	linkedAt := time.Now().UTC().Add(-time.Hour)
	users := newUserStoreStub(domain.User{
		ID:                           "usr-1",
		Email:                        "alice@example.com",
		TelegramUsername:             "alice_dev",
		TelegramChatID:               705,
		TelegramLinkedAt:             &linkedAt,
		TelegramNotificationsEnabled: true,
		CreatedAt:                    time.Now().UTC(),
	})
	sender := &telegramSenderStub{}
	svc := NewNotificationService(users, sender, "deploysvc_bot")

	err := svc.HandleTelegramUpdate(context.Background(), TelegramUpdate{
		Message: &TelegramMessage{
			Text: "/stop",
			Chat: struct {
				ID int64 `json:"id"`
			}{ID: 705},
			From: struct {
				Username string `json:"username"`
			}{Username: "alice_dev"},
		},
	})
	if err != nil {
		t.Fatalf("HandleTelegramUpdate returned error: %v", err)
	}
	user, ok := users.GetByID(context.Background(), "usr-1")
	if !ok {
		t.Fatal("expected user to remain in store")
	}
	if user.TelegramNotificationsEnabled {
		t.Fatal("expected notifications to be disabled")
	}
	if sender.calls != 1 {
		t.Fatalf("expected one confirmation message, got %d", sender.calls)
	}
	if sender.text != "Уведомления остановлены. Включить их снова можно в интерфейсе DeployService." {
		t.Fatalf("unexpected response: %q", sender.text)
	}
}

func TestNotificationServiceSendUserAlertWithTypedNilTelegramSenderDoesNotPanic(t *testing.T) {
	t.Parallel()

	users := newUserStoreStub(domain.User{
		ID:                           "usr-1",
		Email:                        "alice@example.com",
		TelegramUsername:             "alice_dev",
		TelegramChatID:               777,
		TelegramNotificationsEnabled: true,
		CreatedAt:                    time.Now().UTC(),
	})

	var sender TelegramSender = (*integrationtelegram.Client)(nil)
	svc := NewNotificationService(users, sender, "deploysvc_bot")

	if err := svc.SendUserAlert(context.Background(), "usr-1", "cpu-warning:prj-1", "cpu high", time.Minute); err != nil {
		t.Fatalf("SendUserAlert returned error: %v", err)
	}
}

func TestNotificationServiceHandleTelegramUpdateUnknownCommandShowsHelp(t *testing.T) {
	t.Parallel()

	users := newUserStoreStub()
	sender := &telegramSenderStub{}
	svc := NewNotificationService(users, sender, "deploysvc_bot")

	err := svc.HandleTelegramUpdate(context.Background(), TelegramUpdate{
		Message: &TelegramMessage{
			Text: "привет",
			Chat: struct {
				ID int64 `json:"id"`
			}{ID: 706},
			From: struct {
				Username string `json:"username"`
			}{Username: "alice_dev"},
		},
	})
	if err != nil {
		t.Fatalf("HandleTelegramUpdate returned error: %v", err)
	}
	if sender.calls != 1 {
		t.Fatalf("expected one help message, got %d", sender.calls)
	}
	if sender.text != svc.helpMessage() {
		t.Fatalf("expected help message, got %q", sender.text)
	}
}

func TestNotificationServiceSendUserAlertDeduplicatesByTTL(t *testing.T) {
	t.Parallel()

	linkedAt := time.Now().UTC().Add(-time.Hour)
	users := newUserStoreStub(domain.User{
		ID:                           "usr-1",
		Email:                        "alice@example.com",
		TelegramUsername:             "alice_dev",
		TelegramChatID:               777,
		TelegramLinkedAt:             &linkedAt,
		TelegramNotificationsEnabled: true,
		CreatedAt:                    time.Now().UTC(),
	})
	sender := &telegramSenderStub{}
	svc := NewNotificationService(users, sender, "deploysvc_bot")

	if err := svc.SendUserAlert(context.Background(), "usr-1", "billing-low", "balance low", time.Minute); err != nil {
		t.Fatalf("SendUserAlert returned error: %v", err)
	}
	if err := svc.SendUserAlert(context.Background(), "usr-1", "billing-low", "balance low", time.Minute); err != nil {
		t.Fatalf("SendUserAlert returned error on duplicate call: %v", err)
	}
	if sender.calls != 1 {
		t.Fatalf("expected one delivered alert because of TTL dedup, got %d", sender.calls)
	}
	if sender.chatID != 777 {
		t.Fatalf("expected alert for chat 777, got %d", sender.chatID)
	}
}

func TestNotificationServiceSendSystemAlertTargetsOnlyConfiguredAdmins(t *testing.T) {
	t.Parallel()

	users := newUserStoreStub(
		domain.User{
			ID:                           "usr-admin-1",
			Email:                        "admin1@example.com",
			TelegramUsername:             "dastroo",
			TelegramChatID:               101,
			TelegramNotificationsEnabled: false,
			CreatedAt:                    time.Now().UTC(),
		},
		domain.User{
			ID:                           "usr-admin-2",
			Email:                        "admin2@example.com",
			TelegramUsername:             "asaidar",
			TelegramChatID:               202,
			TelegramNotificationsEnabled: true,
			CreatedAt:                    time.Now().UTC(),
		},
		domain.User{
			ID:                           "usr-user",
			Email:                        "user@example.com",
			TelegramUsername:             "regular_user",
			TelegramChatID:               303,
			TelegramNotificationsEnabled: true,
			CreatedAt:                    time.Now().UTC(),
		},
	)
	sender := &telegramSenderStub{}
	svc := NewNotificationService(users, sender, "deploysvc_bot")
	svc.SetAdminTelegramUsernames([]string{"@dastroo", "asaidar"})

	if err := svc.SendSystemAlert(context.Background(), "system:test", "[critical] system check", time.Minute); err != nil {
		t.Fatalf("SendSystemAlert returned error: %v", err)
	}

	if sender.calls != 2 {
		t.Fatalf("expected two admin messages, got %d", sender.calls)
	}
	if !strings.HasPrefix(sender.text, "[admin] ") {
		t.Fatalf("expected admin prefix in system alert text, got %q", sender.text)
	}
}

func TestNotificationServiceHandleTelegramUpdateLinksAdminChatWhenUsernameMatches(t *testing.T) {
	t.Parallel()

	expiresAt := time.Now().UTC().Add(time.Hour)
	users := newUserStoreStub(domain.User{
		ID:                    "usr-admin-1",
		Email:                 "admin@example.com",
		TelegramUsername:      "dastroo",
		TelegramLinkCode:      "admin-link-code",
		TelegramLinkExpiresAt: &expiresAt,
		CreatedAt:             time.Now().UTC(),
	})
	sender := &telegramSenderStub{}
	svc := NewNotificationService(users, sender, "deploysvc_bot")
	svc.SetAdminTelegramUsernames([]string{"dastroo"})

	err := svc.HandleTelegramUpdate(context.Background(), TelegramUpdate{
		Message: &TelegramMessage{
			Text: "/start admin-link-code",
			Chat: struct {
				ID int64 `json:"id"`
			}{ID: 808},
			From: struct {
				Username string `json:"username"`
			}{Username: "dastroo"},
		},
	})
	if err != nil {
		t.Fatalf("HandleTelegramUpdate returned error: %v", err)
	}

	user, ok := users.GetByID(context.Background(), "usr-admin-1")
	if !ok {
		t.Fatal("expected admin user to remain in store")
	}
	if user.TelegramChatID != 808 {
		t.Fatalf("expected telegram chat id 808, got %d", user.TelegramChatID)
	}
}

func TestNotificationServiceHandleTelegramUpdateRejectsAdminCodeWhenSenderUsernameDoesNotMatch(t *testing.T) {
	t.Parallel()

	expiresAt := time.Now().UTC().Add(time.Hour)
	users := newUserStoreStub(domain.User{
		ID:                    "usr-admin-1",
		Email:                 "admin@example.com",
		TelegramUsername:      "dastroo",
		TelegramLinkCode:      "admin-link-code",
		TelegramLinkExpiresAt: &expiresAt,
		CreatedAt:             time.Now().UTC(),
	})
	sender := &telegramSenderStub{}
	svc := NewNotificationService(users, sender, "deploysvc_bot")
	svc.SetAdminTelegramUsernames([]string{"dastroo"})

	err := svc.HandleTelegramUpdate(context.Background(), TelegramUpdate{
		Message: &TelegramMessage{
			Text: "/start admin-link-code",
			Chat: struct {
				ID int64 `json:"id"`
			}{ID: 809},
			From: struct {
				Username string `json:"username"`
			}{Username: "not-dastroo"},
		},
	})
	if err != nil {
		t.Fatalf("HandleTelegramUpdate returned error: %v", err)
	}
	if sender.calls != 1 {
		t.Fatalf("expected one reply, got %d", sender.calls)
	}
	if sender.text != "Для админского аккаунта нужен тот же Telegram username, который добавлен в конфиг сервиса, и одноразовый код из интерфейса." {
		t.Fatalf("unexpected response: %q", sender.text)
	}

	user, ok := users.GetByID(context.Background(), "usr-admin-1")
	if !ok {
		t.Fatal("expected admin user to remain in store")
	}
	if user.TelegramChatID != 0 {
		t.Fatalf("expected admin chat to remain unlinked, got %d", user.TelegramChatID)
	}
}

func TestFormatSystemAlertTextPreservesExistingAdminPrefix(t *testing.T) {
	t.Parallel()

	got := formatSystemAlertText("[admin] [critical] deploy failed")
	if got != "[admin] [critical] deploy failed" {
		t.Fatalf("expected existing admin prefix to stay intact, got %q", got)
	}
}
