package service

import (
	"context"

	"deploy-service/internal/domain"
)

// Port — это входной порт, то есть интерфейс, от которого зависит HTTP-адаптер.
type Port interface {
	CreateProject(ctx context.Context, request domain.CreateProjectRequest) (domain.Project, error)
	ListProjects(ctx context.Context) []domain.Project
	GetProject(ctx context.Context, projectID string) (domain.Project, error)
	DeleteProject(ctx context.Context, projectID string) error
	GetProjectCost(ctx context.Context, projectID string) (domain.CostBreakdown, error)
	UpdateProjectDeploymentSettings(ctx context.Context, projectID string, request domain.UpdateDeploymentSettingsRequest) (domain.Project, error)
	BuildGitHubBootstrapQuestions(ctx context.Context, projectID string, request domain.GitHubBootstrapQuestionsRequest) (domain.GitHubBootstrapQuestionsResponse, error)
	BootstrapGitHubFlow(ctx context.Context, projectID string, request domain.BootstrapGitHubFlowRequest) (domain.BootstrapGitHubFlowResponse, error)
	SuspendProject(ctx context.Context, projectID string) error
	ResumeProject(ctx context.Context, projectID string) error
	ListReleases(ctx context.Context, projectID string) ([]domain.Release, error)
	GetRelease(ctx context.Context, projectID, releaseID string) (domain.Release, error)
	HandleGitHubWebhook(ctx context.Context, payload domain.GitHubWorkflowRunPayload) error
	RollbackToRelease(ctx context.Context, projectID, releaseID string) (domain.Release, error)
	GetProjectKubeconfig(ctx context.Context, projectID string) (string, error)
	RotateProjectKubeconfig(ctx context.Context, projectID string) (string, error)
	GetProjectRuntimeStatus(ctx context.Context, projectID string) (domain.ProjectRuntimeStatus, error)
	GetBillingSummary(ctx context.Context, userID string) (domain.BillingSummary, error)
	EnforceBillingGuard(ctx context.Context, userID string) error
	ListBillingTransactions(ctx context.Context, userID string) ([]domain.BillingTransaction, error)
	ListProjectBillingTransactions(ctx context.Context, projectID, userID string) ([]domain.BillingTransaction, error)
	GetServiceGitHubTokenStatus(ctx context.Context, userID string) (domain.ServiceGitHubTokenStatus, error)
	UpsertServiceGitHubToken(ctx context.Context, userID, token string) (domain.ServiceGitHubTokenStatus, error)
	DeleteServiceGitHubToken(ctx context.Context, userID string) error

	CreateStage(ctx context.Context, projectID, userID string, req domain.CreateStageRequest) (domain.Stage, error)
	ListStages(ctx context.Context, projectID, userID string) ([]domain.Stage, error)
	GetStage(ctx context.Context, projectID, stageID, userID string) (domain.Stage, error)
	DeleteStage(ctx context.Context, projectID, stageID, userID string) error
	GetStageRuntimeStatus(ctx context.Context, projectID, stageID, userID string) (domain.ProjectRuntimeStatus, error)
	ListProjectLogs(ctx context.Context, projectID, userID string, request domain.ProjectLogsRequest) (domain.ProjectLogsResponse, error)
	GetProjectURLs(ctx context.Context, projectID, userID string) (domain.ProjectURLsResponse, error)

	GetProjectGitHubToken(ctx context.Context, projectID, userID string) (domain.ServiceGitHubTokenStatus, error)
	UpsertProjectGitHubToken(ctx context.Context, projectID, userID, token string) (domain.ServiceGitHubTokenStatus, error)
	DeleteProjectGitHubToken(ctx context.Context, projectID, userID string) error
}
