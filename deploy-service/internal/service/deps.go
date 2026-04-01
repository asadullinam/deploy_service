package service

import (
	"context"
	"time"

	"deploy-service/internal/domain"
)

// ProjectStore — исходящий порт для хранения проектов.
type ProjectStore interface {
	Create(ctx context.Context, project domain.Project) error
	GetByID(ctx context.Context, projectID string) (domain.Project, bool)
	List(ctx context.Context) []domain.Project
	Update(ctx context.Context, project domain.Project) error
	UpdateKubeconfig(ctx context.Context, projectID, encryptedKubeconfig string) error
	UpdateGitHubToken(ctx context.Context, projectID, encryptedToken string) error
}

// Provisioner — исходящий порт для управления окружением Kubernetes.
type Provisioner interface {
	CreateProjectEnvironment(ctx context.Context, projectID string) (string, error)
	DeleteProjectEnvironment(ctx context.Context, projectID string) error
	SuspendProjectEnvironment(ctx context.Context, projectID string) error
	ResumeProjectEnvironment(ctx context.Context, projectID string) error
	ApplyImage(ctx context.Context, projectID string, imageTag string) error
	GetProjectKubeconfig(ctx context.Context, projectID string) (string, error)
	GetProjectRuntimeStatus(ctx context.Context, projectID string) (domain.ProjectRuntimeStatus, error)

	// Методы Stage работают с namespace внутри vcluster проекта.
	CreateStageEnvironment(ctx context.Context, projectID, stageSlug string) error
	DeleteStageEnvironment(ctx context.Context, projectID, stageSlug string) error
	ApplyImageToStage(ctx context.Context, projectID, stageSlug, imageTag string) error
	GetStageRuntimeStatus(ctx context.Context, projectID, stageSlug string) (domain.ProjectRuntimeStatus, error)
}

// StageStore — исходящий порт для хранения stage-окружений.
type StageStore interface {
	Create(ctx context.Context, stage domain.Stage) error
	GetByID(ctx context.Context, stageID string) (domain.Stage, bool)
	GetBySlug(ctx context.Context, projectID, slug string) (domain.Stage, bool)
	ListByProject(ctx context.Context, projectID string) []domain.Stage
	Update(ctx context.Context, stage domain.Stage) error
}

// CryptoService — исходящий порт для симметричного шифрования.
type CryptoService interface {
	Encrypt(plaintext string) (string, error)
	Decrypt(ciphertext string) (string, error)
}

// GitHubAutomation — исходящий порт для настройки GitHub CI/CD.
type GitHubAutomation interface {
	SetupProjectAutomation(ctx context.Context, projectID string) error
	BuildBootstrapQuestions(ctx context.Context, projectID string, request domain.GitHubBootstrapQuestionsRequest) (domain.GitHubBootstrapQuestionsResponse, error)
	BootstrapRepositoryFlow(ctx context.Context, projectID string, request domain.BootstrapGitHubFlowRequest) (domain.BootstrapGitHubFlowResponse, error)
	FindLatestDeployWorkflowRun(ctx context.Context, request domain.GitHubWorkflowRunLookupRequest) ([]domain.GitHubWorkflowRunLookupResult, error)
}

// MonetizationEngine — исходящий порт для расчета стоимости.
type MonetizationEngine interface {
	GetProjectCost(ctx context.Context, projectID string) (domain.CostBreakdown, error)
	// ComputeUsageCost возвращает стоимость в USD для одного периода usage snapshot.
	ComputeUsageCost(usage domain.ResourceUsage) float64
}

// UserStore — исходящий порт для хранения пользователей.
type UserStore interface {
	Create(ctx context.Context, user domain.User) error
	GetByEmail(ctx context.Context, email string) (domain.User, bool)
	GetByID(ctx context.Context, userID string) (domain.User, bool)
	GetByTelegramUsername(ctx context.Context, username string) (domain.User, bool)
	GetByTelegramLinkCode(ctx context.Context, code string) (domain.User, bool)
	GetByTelegramChatID(ctx context.Context, chatID int64) (domain.User, bool)
	UpdateBalance(ctx context.Context, userID string, balanceUSD float64) error
	UpdateGitHubToken(ctx context.Context, userID, encryptedToken string) error
	UpdateTelegramSettings(ctx context.Context, userID, username, linkCode string, linkExpiresAt *time.Time, enabled bool) error
	LinkTelegramChat(ctx context.Context, userID string, chatID int64, linkedAt time.Time) error
	DisableTelegramNotifications(ctx context.Context, userID string) error
	ClearTelegramSettings(ctx context.Context, userID string) error
}

// ReleaseStore — исходящий порт для хранения релизов.
type ReleaseStore interface {
	Create(ctx context.Context, release domain.Release) error
	GetByID(ctx context.Context, releaseID string) (domain.Release, bool)
	ListByProject(ctx context.Context, projectID string) []domain.Release
	Update(ctx context.Context, release domain.Release) error
	GetByWorkflowRunID(ctx context.Context, runID int64) (domain.Release, bool)
}

// UsageStore — исходящий порт для хранения данных об использовании ресурсов.
type UsageStore interface {
	Record(ctx context.Context, usage domain.ResourceUsage) error
	AggregateForProject(ctx context.Context, projectID string, from, to time.Time) (domain.UsageAggregate, error)
}

// MetricsCollector — исходящий порт для сбора метрик ресурсов Kubernetes.
type MetricsCollector interface {
	CollectProjectUsage(ctx context.Context, projectID string) (domain.ResourceSnapshot, error)
}

type LogReader interface {
	ListProjectLogs(ctx context.Context, projectID string, request domain.ProjectLogsRequest) (domain.ProjectLogsResponse, error)
}

// BillingTransactionStore — исходящий порт для журнала биллинговых операций.
type BillingTransactionStore interface {
	Record(ctx context.Context, tx domain.BillingTransaction) error
	ListByUser(ctx context.Context, userID string) ([]domain.BillingTransaction, error)
	ListByProject(ctx context.Context, projectID string) ([]domain.BillingTransaction, error)
}

// YooKassaPaymentStore — исходящий порт для хранения платежей YooKassa.
type YooKassaPaymentStore interface {
	Create(ctx context.Context, p domain.YooKassaPayment) error
	GetByYooKassaID(ctx context.Context, yookassaID string) (domain.YooKassaPayment, bool)
	MarkSucceeded(ctx context.Context, yookassaID string) error
}

// YooKassaClient — исходящий порт для создания платежей через YooKassa API.
type YooKassaClient interface {
	CreatePayment(ctx context.Context, amountRUB float64, idempotenceKey, returnURL, description string, metadata map[string]string) (paymentID, confirmationURL string, err error)
}
