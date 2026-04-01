package http

import (
	"net/http"

	"deploy-service/internal/http/middleware"
)

func NewRouter(handler *Handler, jwtSecret string) http.Handler {
	mux := http.NewServeMux()
	authMW := middleware.RequireAuth(jwtSecret)

	// Публичные маршруты
	mux.HandleFunc("GET /", handler.HomeUI)
	mux.Handle("GET /ui/", http.StripPrefix("/ui/", handler.UIAssets()))
	mux.HandleFunc("GET /health", handler.Health)
	mux.HandleFunc("POST /auth/register", handler.Register)
	mux.HandleFunc("POST /auth/login", handler.Login)
	mux.HandleFunc("POST /auth/logout", handler.Logout)
	mux.HandleFunc("POST /webhooks/telegram", handler.TelegramWebhook)
	mux.HandleFunc("POST /webhooks/github", handler.GitHubWebhook)
	mux.HandleFunc("POST /webhooks/yookassa", handler.YooKassaWebhook)
	mux.HandleFunc("GET /swagger", handler.SwaggerUI)
	mux.HandleFunc("GET /swagger/", handler.SwaggerUI)
	mux.HandleFunc("GET /swagger/openapi.yaml", handler.SwaggerSpec)

	// Защищенные маршруты — оборачиваются в authMW
	wrap := func(pattern string, h http.HandlerFunc) {
		mux.Handle(pattern, authMW(h))
	}
	wrap("GET /projects", handler.ListProjects)
	wrap("GET /billing/summary", handler.BillingSummary)
	wrap("GET /me/telegram", handler.GetTelegramSettings)
	wrap("PUT /me/telegram", handler.UpdateTelegramSettings)
	wrap("DELETE /me/telegram", handler.DeleteTelegramSettings)
	wrap("POST /billing/top-up", handler.TopUpBalance)
	wrap("POST /billing/top-up/initiate", handler.InitiateTopUp)
	wrap("GET /billing/transactions", handler.ListBillingTransactions)
	wrap("GET /service/github-token", handler.GetServiceGitHubToken)
	wrap("PUT /service/github-token", handler.UpsertServiceGitHubToken)
	wrap("DELETE /service/github-token", handler.DeleteServiceGitHubToken)
	wrap("POST /projects", handler.CreateProject)
	wrap("GET /projects/{id}", handler.GetProject)
	wrap("DELETE /projects/{id}", handler.DeleteProject)
	wrap("GET /projects/{id}/cost", handler.GetProjectCost)
	wrap("GET /projects/{id}/runtime-status", handler.GetProjectRuntimeStatus)
	wrap("GET /projects/{id}/logs", handler.ListProjectLogs)
	wrap("POST /projects/{id}/stages", handler.CreateStage)
	wrap("GET /projects/{id}/stages", handler.ListStages)
	wrap("GET /projects/{id}/stages/{stageId}", handler.GetStage)
	wrap("DELETE /projects/{id}/stages/{stageId}", handler.DeleteStage)
	wrap("GET /projects/{id}/stages/{stageId}/runtime-status", handler.GetStageRuntimeStatus)
	wrap("GET /projects/{id}/urls", handler.GetProjectURLs)
	wrap("PUT /projects/{id}/deployment-settings", handler.UpdateDeploymentSettings)
	wrap("POST /projects/{id}/suspend", handler.SuspendProject)
	wrap("POST /projects/{id}/resume", handler.ResumeProject)
	wrap("POST /projects/{id}/github/questions", handler.GitHubQuestions)
	wrap("POST /projects/{id}/github/bootstrap", handler.GitHubBootstrap)
	wrap("GET /projects/{id}/releases", handler.ListReleases)
	wrap("GET /projects/{id}/releases/{releaseId}", handler.GetRelease)
	wrap("POST /projects/{id}/releases/{releaseId}/rollback", handler.RollbackRelease)
	wrap("GET /projects/{id}/kubeconfig", handler.GetKubeconfig)
	wrap("GET /projects/{id}/billing", handler.ListProjectBillingTransactions)
	wrap("POST /projects/{id}/kubeconfig/rotate", handler.RotateKubeconfig)
	wrap("GET /projects/{id}/github-token", handler.GetProjectGitHubToken)
	wrap("PUT /projects/{id}/github-token", handler.UpsertProjectGitHubToken)
	wrap("DELETE /projects/{id}/github-token", handler.DeleteProjectGitHubToken)

	return middleware.Logging(mux)
}
