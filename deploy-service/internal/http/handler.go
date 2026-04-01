package http

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"deploy-service/internal/domain"
	"deploy-service/internal/http/middleware"
	"deploy-service/internal/service"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
)

const invalidCredentialsMessage = "Неверный логин или пароль"
const telegramWebhookSecretHeader = "X-Telegram-Bot-Api-Secret-Token"

func NewHandler(projectService service.Port, authService *service.AuthService, webhookSecret string) *Handler {
	return &Handler{projects: projectService, auth: authService, webhookSecret: webhookSecret}
}

func NewHandlerWithNotifications(projectService service.Port, authService *service.AuthService, notifications *service.NotificationService, webhookSecret string) *Handler {
	return &Handler{projects: projectService, auth: authService, notifications: notifications, webhookSecret: webhookSecret}
}

func (h *Handler) SetTelegramWebhookSecret(secret string) {
	h.telegramWebhookSecret = strings.TrimSpace(secret)
}

func (h *Handler) Health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req domain.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	resp, err := h.auth.Register(r.Context(), req)
	if err != nil {
		if errors.Is(err, domain.ErrEmailTaken) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	setAuthCookie(w, resp.Token)
	writeJSON(w, http.StatusCreated, resp)
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req domain.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	resp, err := h.auth.Login(r.Context(), req)
	if err != nil {
		if errors.Is(err, domain.ErrInvalidCredentials) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": invalidCredentialsMessage})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	setAuthCookie(w, resp.Token)
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) Logout(w http.ResponseWriter, _ *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "deploy_service_token",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged out"})
}

func (h *Handler) GetTelegramSettings(w http.ResponseWriter, r *http.Request) {
	if h.notifications == nil {
		writeJSON(w, http.StatusOK, domain.TelegramSettings{})
		return
	}
	settings, err := h.notifications.GetTelegramSettings(r.Context(), middleware.UserIDFromContext(r.Context()))
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (h *Handler) UpdateTelegramSettings(w http.ResponseWriter, r *http.Request) {
	if h.notifications == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "telegram notifications are not configured"})
		return
	}
	var req domain.UpdateTelegramSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	settings, err := h.notifications.UpdateTelegramSettings(r.Context(), middleware.UserIDFromContext(r.Context()), req)
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (h *Handler) DeleteTelegramSettings(w http.ResponseWriter, r *http.Request) {
	if h.notifications == nil {
		writeJSON(w, http.StatusOK, domain.TelegramSettings{})
		return
	}
	settings, err := h.notifications.ClearTelegramSettings(r.Context(), middleware.UserIDFromContext(r.Context()))
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (h *Handler) TelegramWebhook(w http.ResponseWriter, r *http.Request) {
	if h.telegramWebhookSecret != "" {
		received := strings.TrimSpace(r.Header.Get(telegramWebhookSecretHeader))
		if received == "" || !hmac.Equal([]byte(received), []byte(h.telegramWebhookSecret)) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid telegram webhook secret"})
			return
		}
	}
	if h.notifications == nil {
		writeJSON(w, http.StatusOK, map[string]string{"status": "disabled"})
		return
	}
	var update service.TelegramUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if err := h.notifications.HandleTelegramUpdate(r.Context(), update); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func setAuthCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "deploy_service_token",
		Value:    token,
		Path:     "/",
		MaxAge:   86400, // 24h
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func (h *Handler) BillingSummary(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())
	if err := h.projects.EnforceBillingGuard(r.Context(), userID); err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	summary, err := h.projects.GetBillingSummary(r.Context(), userID)
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func (h *Handler) TopUpBalance(w http.ResponseWriter, r *http.Request) {
	var req domain.TopUpBalanceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if _, err := h.auth.TopUpBalance(r.Context(), middleware.UserIDFromContext(r.Context()), req.AmountRUB); err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	summary, err := h.projects.GetBillingSummary(r.Context(), middleware.UserIDFromContext(r.Context()))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func (h *Handler) InitiateTopUp(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AmountRUB float64 `json:"amountRub"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	userID := middleware.UserIDFromContext(r.Context())
	confirmationURL, err := h.auth.InitiateTopUp(r.Context(), userID, req.AmountRUB)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"confirmationUrl": confirmationURL})
}

func (h *Handler) YooKassaWebhook(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Event  string `json:"event"`
		Object struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"object"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if payload.Event != "payment.succeeded" {
		w.WriteHeader(http.StatusOK)
		return
	}
	if err := h.auth.HandleYooKassaWebhook(r.Context(), payload.Object.ID); err != nil {
		log.Printf("yookassa webhook handler error: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) GetServiceGitHubToken(w http.ResponseWriter, r *http.Request) {
	status, err := h.projects.GetServiceGitHubTokenStatus(r.Context(), middleware.UserIDFromContext(r.Context()))
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (h *Handler) UpsertServiceGitHubToken(w http.ResponseWriter, r *http.Request) {
	var req domain.UpsertServiceGitHubTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	status, err := h.projects.UpsertServiceGitHubToken(r.Context(), middleware.UserIDFromContext(r.Context()), req.Token)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrUserNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (h *Handler) DeleteServiceGitHubToken(w http.ResponseWriter, r *http.Request) {
	err := h.projects.DeleteServiceGitHubToken(r.Context(), middleware.UserIDFromContext(r.Context()))
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, domain.ServiceGitHubTokenStatus{Configured: false})
}

func (h *Handler) GetProjectGitHubToken(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())
	status, err := h.projects.GetProjectGitHubToken(r.Context(), r.PathValue("id"), userID)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrProjectNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		case errors.Is(err, domain.ErrForbidden):
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
		default:
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (h *Handler) UpsertProjectGitHubToken(w http.ResponseWriter, r *http.Request) {
	var req domain.UpsertServiceGitHubTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	userID := middleware.UserIDFromContext(r.Context())
	status, err := h.projects.UpsertProjectGitHubToken(r.Context(), r.PathValue("id"), userID, req.Token)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrProjectNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		case errors.Is(err, domain.ErrForbidden):
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (h *Handler) DeleteProjectGitHubToken(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())
	err := h.projects.DeleteProjectGitHubToken(r.Context(), r.PathValue("id"), userID)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrProjectNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		case errors.Is(err, domain.ErrForbidden):
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
		default:
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return
	}
	writeJSON(w, http.StatusOK, domain.ServiceGitHubTokenStatus{Configured: false})
}

func (h *Handler) ListProjects(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())
	projects := h.projects.ListProjects(r.Context())
	filtered := make([]domain.Project, 0, len(projects))
	for _, project := range projects {
		if project.OwnerID == userID {
			filtered = append(filtered, project)
		}
	}
	writeJSON(w, http.StatusOK, filtered)
}

func (h *Handler) CreateProject(w http.ResponseWriter, r *http.Request) {
	var req domain.CreateProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	req.OwnerID = middleware.UserIDFromContext(r.Context())

	project, err := h.projects.CreateProject(r.Context(), req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusAccepted, project)
}

func (h *Handler) GetProject(w http.ResponseWriter, r *http.Request) {
	project, err := h.requireOwnedProject(r.Context(), r.PathValue("id"), middleware.UserIDFromContext(r.Context()))
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrProjectNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		case errors.Is(err, domain.ErrForbidden):
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
		default:
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return
	}

	writeJSON(w, http.StatusOK, project)
}

func (h *Handler) DeleteProject(w http.ResponseWriter, r *http.Request) {
	if _, err := h.requireOwnedProject(r.Context(), r.PathValue("id"), middleware.UserIDFromContext(r.Context())); err != nil {
		switch {
		case errors.Is(err, domain.ErrProjectNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		case errors.Is(err, domain.ErrForbidden):
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		return
	}

	err := h.projects.DeleteProject(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, domain.ErrProjectNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *Handler) GetProjectCost(w http.ResponseWriter, r *http.Request) {
	if _, err := h.requireOwnedProject(r.Context(), r.PathValue("id"), middleware.UserIDFromContext(r.Context())); err != nil {
		switch {
		case errors.Is(err, domain.ErrProjectNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		case errors.Is(err, domain.ErrForbidden):
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
		default:
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return
	}

	cost, err := h.projects.GetProjectCost(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, domain.ErrProjectNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, cost)
}

func (h *Handler) ListProjectLogs(w http.ResponseWriter, r *http.Request) {
	request := domain.ProjectLogsRequest{
		StageID:   strings.TrimSpace(r.URL.Query().Get("stageId")),
		Pod:       strings.TrimSpace(r.URL.Query().Get("pod")),
		Container: strings.TrimSpace(r.URL.Query().Get("container")),
		Search:    strings.TrimSpace(r.URL.Query().Get("search")),
		Level:     strings.TrimSpace(r.URL.Query().Get("level")),
		Since:     strings.TrimSpace(r.URL.Query().Get("since")),
	}
	if rawLimit := strings.TrimSpace(r.URL.Query().Get("limit")); rawLimit != "" {
		limit, err := strconv.Atoi(rawLimit)
		if err != nil || limit <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid limit query parameter"})
			return
		}
		request.Limit = limit
	}
	response, err := h.projects.ListProjectLogs(r.Context(), r.PathValue("id"), middleware.UserIDFromContext(r.Context()), request)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrProjectNotFound), errors.Is(err, domain.ErrStageNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		case errors.Is(err, domain.ErrForbidden):
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
		case errors.Is(err, domain.ErrLogsUnavailable):
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		default:
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return
	}
	if response.Entries == nil {
		response.Entries = []domain.ProjectLogEntry{}
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) GetProjectRuntimeStatus(w http.ResponseWriter, r *http.Request) {
	if _, err := h.requireOwnedProject(r.Context(), r.PathValue("id"), middleware.UserIDFromContext(r.Context())); err != nil {
		switch {
		case errors.Is(err, domain.ErrProjectNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		case errors.Is(err, domain.ErrForbidden):
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
		default:
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return
	}

	status, err := h.projects.GetProjectRuntimeStatus(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, domain.ErrProjectNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, status)
}

func (h *Handler) CreateStage(w http.ResponseWriter, r *http.Request) {
	var req domain.CreateStageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	stage, err := h.projects.CreateStage(r.Context(), r.PathValue("id"), middleware.UserIDFromContext(r.Context()), req)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrProjectNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		case errors.Is(err, domain.ErrForbidden):
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
		case errors.Is(err, domain.ErrProjectEnvironmentUnavailable):
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		return
	}

	writeJSON(w, http.StatusCreated, stage)
}

func (h *Handler) ListStages(w http.ResponseWriter, r *http.Request) {
	stages, err := h.projects.ListStages(r.Context(), r.PathValue("id"), middleware.UserIDFromContext(r.Context()))
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrProjectNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		case errors.Is(err, domain.ErrForbidden):
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
		default:
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return
	}
	if stages == nil {
		stages = []domain.Stage{}
	}
	writeJSON(w, http.StatusOK, stages)
}

func (h *Handler) GetStage(w http.ResponseWriter, r *http.Request) {
	stage, err := h.projects.GetStage(r.Context(), r.PathValue("id"), r.PathValue("stageId"), middleware.UserIDFromContext(r.Context()))
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrProjectNotFound), errors.Is(err, domain.ErrStageNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		case errors.Is(err, domain.ErrForbidden):
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
		default:
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return
	}
	writeJSON(w, http.StatusOK, stage)
}

func (h *Handler) DeleteStage(w http.ResponseWriter, r *http.Request) {
	err := h.projects.DeleteStage(r.Context(), r.PathValue("id"), r.PathValue("stageId"), middleware.UserIDFromContext(r.Context()))
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrProjectNotFound), errors.Is(err, domain.ErrStageNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		case errors.Is(err, domain.ErrForbidden):
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *Handler) GetStageRuntimeStatus(w http.ResponseWriter, r *http.Request) {
	status, err := h.projects.GetStageRuntimeStatus(r.Context(), r.PathValue("id"), r.PathValue("stageId"), middleware.UserIDFromContext(r.Context()))
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrProjectNotFound), errors.Is(err, domain.ErrStageNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		case errors.Is(err, domain.ErrForbidden):
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
		default:
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (h *Handler) GetProjectURLs(w http.ResponseWriter, r *http.Request) {
	result, err := h.projects.GetProjectURLs(r.Context(), r.PathValue("id"), middleware.UserIDFromContext(r.Context()))
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrProjectNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		case errors.Is(err, domain.ErrForbidden):
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
		default:
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) UpdateDeploymentSettings(w http.ResponseWriter, r *http.Request) {
	if _, err := h.requireOwnedProject(r.Context(), r.PathValue("id"), middleware.UserIDFromContext(r.Context())); err != nil {
		switch {
		case errors.Is(err, domain.ErrProjectNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		case errors.Is(err, domain.ErrForbidden):
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		return
	}

	var req domain.UpdateDeploymentSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	project, err := h.projects.UpdateProjectDeploymentSettings(r.Context(), r.PathValue("id"), req)
	if err != nil {
		if errors.Is(err, domain.ErrProjectNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, project)
}

func (h *Handler) GitHubQuestions(w http.ResponseWriter, r *http.Request) {
	if _, err := h.requireOwnedProject(r.Context(), r.PathValue("id"), middleware.UserIDFromContext(r.Context())); err != nil {
		switch {
		case errors.Is(err, domain.ErrProjectNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		case errors.Is(err, domain.ErrForbidden):
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		return
	}

	var req domain.GitHubBootstrapQuestionsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	response, err := h.projects.BuildGitHubBootstrapQuestions(r.Context(), r.PathValue("id"), req)
	if err != nil {
		if errors.Is(err, domain.ErrProjectNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) GitHubBootstrap(w http.ResponseWriter, r *http.Request) {
	if _, err := h.requireOwnedProject(r.Context(), r.PathValue("id"), middleware.UserIDFromContext(r.Context())); err != nil {
		switch {
		case errors.Is(err, domain.ErrProjectNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		case errors.Is(err, domain.ErrForbidden):
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		return
	}

	var req domain.BootstrapGitHubFlowRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	response, err := h.projects.BootstrapGitHubFlow(r.Context(), r.PathValue("id"), req)
	if err != nil {
		if errors.Is(err, domain.ErrProjectNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		if errors.Is(err, domain.ErrInsufficientBalance) {
			writeJSON(w, http.StatusPaymentRequired, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) SuspendProject(w http.ResponseWriter, r *http.Request) {
	if _, err := h.requireOwnedProject(r.Context(), r.PathValue("id"), middleware.UserIDFromContext(r.Context())); err != nil {
		switch {
		case errors.Is(err, domain.ErrProjectNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		case errors.Is(err, domain.ErrForbidden):
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		return
	}

	err := h.projects.SuspendProject(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, domain.ErrProjectNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "suspended"})
}

func (h *Handler) ResumeProject(w http.ResponseWriter, r *http.Request) {
	if _, err := h.requireOwnedProject(r.Context(), r.PathValue("id"), middleware.UserIDFromContext(r.Context())); err != nil {
		switch {
		case errors.Is(err, domain.ErrProjectNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		case errors.Is(err, domain.ErrForbidden):
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		return
	}

	err := h.projects.ResumeProject(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, domain.ErrProjectNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "active"})
}

func (h *Handler) ListReleases(w http.ResponseWriter, r *http.Request) {
	if _, err := h.requireOwnedProject(r.Context(), r.PathValue("id"), middleware.UserIDFromContext(r.Context())); err != nil {
		switch {
		case errors.Is(err, domain.ErrProjectNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		case errors.Is(err, domain.ErrForbidden):
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
		default:
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return
	}

	releases, err := h.projects.ListReleases(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, domain.ErrProjectNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if stageID := strings.TrimSpace(r.URL.Query().Get("stageId")); stageID != "" {
		filtered := make([]domain.Release, 0, len(releases))
		for _, release := range releases {
			if release.StageID == stageID {
				filtered = append(filtered, release)
			}
		}
		releases = filtered
	}
	if releases == nil {
		releases = []domain.Release{}
	}
	writeJSON(w, http.StatusOK, releases)
}

func (h *Handler) GetRelease(w http.ResponseWriter, r *http.Request) {
	if _, err := h.requireOwnedProject(r.Context(), r.PathValue("id"), middleware.UserIDFromContext(r.Context())); err != nil {
		switch {
		case errors.Is(err, domain.ErrProjectNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		case errors.Is(err, domain.ErrForbidden):
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
		default:
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return
	}

	release, err := h.projects.GetRelease(r.Context(), r.PathValue("id"), r.PathValue("releaseId"))
	if err != nil {
		if errors.Is(err, domain.ErrReleaseNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, release)
}

func (h *Handler) GetKubeconfig(w http.ResponseWriter, r *http.Request) {
	if _, err := h.requireOwnedProject(r.Context(), r.PathValue("id"), middleware.UserIDFromContext(r.Context())); err != nil {
		switch {
		case errors.Is(err, domain.ErrProjectNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		case errors.Is(err, domain.ErrForbidden):
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
		default:
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return
	}

	kubeconfig, err := h.projects.GetProjectKubeconfig(r.Context(), r.PathValue("id"))
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrProjectNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		case errors.Is(err, domain.ErrProjectEnvironmentUnavailable):
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		default:
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return
	}
	w.Header().Set("Content-Type", "application/yaml")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(kubeconfig))
}

func (h *Handler) RotateKubeconfig(w http.ResponseWriter, r *http.Request) {
	if _, err := h.requireOwnedProject(r.Context(), r.PathValue("id"), middleware.UserIDFromContext(r.Context())); err != nil {
		switch {
		case errors.Is(err, domain.ErrProjectNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		case errors.Is(err, domain.ErrForbidden):
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
		default:
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return
	}

	kubeconfig, err := h.projects.RotateProjectKubeconfig(r.Context(), r.PathValue("id"))
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrProjectNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		case errors.Is(err, domain.ErrProjectEnvironmentUnavailable):
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		default:
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return
	}
	w.Header().Set("Content-Type", "application/yaml")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(kubeconfig))
}

func (h *Handler) RollbackRelease(w http.ResponseWriter, r *http.Request) {
	if _, err := h.requireOwnedProject(r.Context(), r.PathValue("id"), middleware.UserIDFromContext(r.Context())); err != nil {
		switch {
		case errors.Is(err, domain.ErrProjectNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		case errors.Is(err, domain.ErrForbidden):
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		return
	}

	release, err := h.projects.RollbackToRelease(r.Context(), r.PathValue("id"), r.PathValue("releaseId"))
	if err != nil {
		if errors.Is(err, domain.ErrReleaseNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		if errors.Is(err, domain.ErrInsufficientBalance) {
			writeJSON(w, http.StatusPaymentRequired, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, release)
}

func (h *Handler) GitHubWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-GitHub-Event") != "workflow_run" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read body"})
		return
	}
	if h.webhookSecret != "" {
		sig := r.Header.Get("X-Hub-Signature-256")
		if !verifyGitHubSignature(body, sig, h.webhookSecret) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid signature"})
			return
		}
	}
	var payload domain.GitHubWorkflowRunPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}
	if err := h.projects.HandleGitHubWebhook(r.Context(), payload); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func verifyGitHubSignature(body []byte, sig, secret string) bool {
	if !strings.HasPrefix(sig, "sha256=") {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(sig), []byte(expected))
}

func (h *Handler) requireOwnedProject(ctx context.Context, projectID string, userID string) (domain.Project, error) {
	project, err := h.projects.GetProject(ctx, projectID)
	if err != nil {
		return domain.Project{}, err
	}
	if project.OwnerID != userID {
		return domain.Project{}, domain.ErrForbidden
	}
	return project, nil
}

func (h *Handler) ListBillingTransactions(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())
	txs, err := h.projects.ListBillingTransactions(r.Context(), userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, txs)
}

func (h *Handler) ListProjectBillingTransactions(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())
	projectID := r.PathValue("id")
	txs, err := h.projects.ListProjectBillingTransactions(r.Context(), projectID, userID)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrProjectNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		case errors.Is(err, domain.ErrForbidden):
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
		default:
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return
	}
	writeJSON(w, http.StatusOK, txs)
}

func writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(payload)
}
