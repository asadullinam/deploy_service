package http

import "deploy-service/internal/service"

type Handler struct {
	projects              service.Port
	auth                  *service.AuthService
	notifications         *service.NotificationService
	webhookSecret         string
	telegramWebhookSecret string
}
