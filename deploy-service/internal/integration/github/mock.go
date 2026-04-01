package github

import (
	"context"
	"deploy-service/internal/domain"
	"deploy-service/internal/service"
	"fmt"
	"log"
	"os"
	"strings"
)

// Проверка на этапе компиляции: AutomationMock реализует service.GitHubAutomation.
var _ service.GitHubAutomation = (*AutomationMock)(nil)

func (a *AutomationMock) SetupProjectAutomation(_ context.Context, _ string) error {
	return nil
}

func (a *AutomationMock) BuildBootstrapQuestions(_ context.Context, _ string, request domain.GitHubBootstrapQuestionsRequest) (domain.GitHubBootstrapQuestionsResponse, error) {
	baseBranch := strings.TrimSpace(request.BaseBranch)
	if baseBranch == "" {
		baseBranch = "main"
	}

	serviceName := strings.TrimSpace(request.ServiceName)
	if serviceName == "" {
		serviceName = "service"
	}

	dockerfilePath := strings.TrimSpace(request.DockerfilePath)
	if dockerfilePath == "" {
		dockerfilePath = "Dockerfile"
	}

	return domain.GitHubBootstrapQuestionsResponse{
		RepositoryOwner:       request.RepositoryOwner,
		RepositoryName:        request.RepositoryName,
		BaseBranch:            baseBranch,
		DetectedDockerfile:    dockerfilePath,
		DetectedServiceName:   serviceName,
		DetectedContainerPort: 8080,
		DetectedServicePort:   8080,
		DetectedServiceType:   "LoadBalancer",
		Questions: []domain.GitHubBootstrapQuestion{
			{Key: "serviceName", Title: "Имя сервиса", Description: "Используется в Kubernetes манифестах", Required: true, DefaultValue: serviceName},
			{Key: "dockerfilePath", Title: "Путь к Dockerfile", Description: "Путь в репозитории к Dockerfile", Required: true, DefaultValue: dockerfilePath},
			{Key: "containerPort", Title: "Порт контейнера", Description: "Порт, на котором слушает приложение", Required: true, DefaultValue: "8080"},
			{Key: "servicePort", Title: "Порт сервиса", Description: "Внешний порт Kubernetes Service", Required: true, DefaultValue: "8080"},
			{Key: "serviceType", Title: "Тип сервиса", Description: "ClusterIP для внутреннего доступа, LoadBalancer для внешнего доступа", Required: true, DefaultValue: "LoadBalancer", Options: []string{"ClusterIP", "LoadBalancer"}},
		},
	}, nil
}

func (a *AutomationMock) BootstrapRepositoryFlow(_ context.Context, projectID string, request domain.BootstrapGitHubFlowRequest) (domain.BootstrapGitHubFlowResponse, error) {
	branchName := fmt.Sprintf("deploy-service/%s/bootstrap", projectID)
	return domain.BootstrapGitHubFlowResponse{
		ProjectID:       projectID,
		RepositoryOwner: request.RepositoryOwner,
		RepositoryName:  request.RepositoryName,
		BranchName:      branchName,
		PullRequestURL:  fmt.Sprintf("https://github.com/%s/%s/pull/new/%s", request.RepositoryOwner, request.RepositoryName, branchName),
		NoChanges:       false,
	}, nil
}

func (a *AutomationMock) FindLatestDeployWorkflowRun(_ context.Context, _ domain.GitHubWorkflowRunLookupRequest) ([]domain.GitHubWorkflowRunLookupResult, error) {
	return nil, nil
}

// NewAutomationFromEnvironment возвращает реальный или mock GitHubAutomation в зависимости от конфигурации окружения.
func NewAutomationFromEnvironment() service.GitHubAutomation {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("GITHUB_AUTOMATION_MODE")))
	if mode == "mock" {
		return &AutomationMock{}
	}

	token := strings.TrimSpace(os.Getenv("GITHUB_TOKEN"))
	baseURL := strings.TrimSpace(os.Getenv("GITHUB_API_BASE_URL"))
	if baseURL == "" {
		baseURL = "https://api.github.com"
	}
	baseDomain := strings.TrimSpace(os.Getenv("APPS_BASE_DOMAIN"))
	publicURL := strings.TrimSpace(os.Getenv("PUBLIC_URL"))
	webhookSecret := strings.TrimSpace(os.Getenv("GITHUB_WEBHOOK_SECRET"))
	if publicURL == "" {
		log.Println("WARNING: PUBLIC_URL is not set; GitHub workflow_run webhooks cannot be auto-registered, release history may remain empty")
	}
	if webhookSecret == "" {
		log.Println("WARNING: GITHUB_WEBHOOK_SECRET is not set; webhook signature verification is disabled")
	}
	return NewGitHubAutomation(baseURL, token, baseDomain, publicURL, webhookSecret)
}
