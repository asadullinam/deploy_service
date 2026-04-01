//go:build !integration

package http

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"deploy-service/internal/auth"
	"deploy-service/internal/domain"
	"deploy-service/internal/service"
)

type projectPortStub struct {
	lastCreateRequest domain.CreateProjectRequest
	lastSettingsReq   domain.UpdateDeploymentSettingsRequest
	webhookPayload    domain.GitHubWorkflowRunPayload
	bootstrapErr      error
	rollbackErr       error
	runtimeStatus     domain.ProjectRuntimeStatus
	stageRuntime      domain.ProjectRuntimeStatus
	project           domain.Project
	projects          []domain.Project
	stages            []domain.Stage
	releases          []domain.Release
	createStageErr    error
	enforcedUserID    string
	billingSummary    domain.BillingSummary
	logsResponse      domain.ProjectLogsResponse
	serviceToken      domain.ServiceGitHubTokenStatus
	kubeconfigErr     error
	rotateKubeErr     error
}

type authUserStoreStub struct {
	usersByID map[string]domain.User
}

func newAuthUserStoreStub(users ...domain.User) *authUserStoreStub {
	store := &authUserStoreStub{usersByID: make(map[string]domain.User)}
	for _, user := range users {
		store.usersByID[user.ID] = user
	}
	return store
}

func (s *authUserStoreStub) Create(_ context.Context, user domain.User) error {
	s.usersByID[user.ID] = user
	return nil
}

func (s *authUserStoreStub) GetByEmail(_ context.Context, email string) (domain.User, bool) {
	for _, user := range s.usersByID {
		if user.Email == email {
			return user, true
		}
	}
	return domain.User{}, false
}

func (s *authUserStoreStub) GetByID(_ context.Context, userID string) (domain.User, bool) {
	user, ok := s.usersByID[userID]
	return user, ok
}

func (s *authUserStoreStub) GetByTelegramUsername(_ context.Context, username string) (domain.User, bool) {
	normalized := strings.TrimPrefix(strings.TrimSpace(username), "@")
	for _, user := range s.usersByID {
		if strings.EqualFold(user.TelegramUsername, normalized) {
			return user, true
		}
	}
	return domain.User{}, false
}

func (s *authUserStoreStub) GetByTelegramLinkCode(_ context.Context, code string) (domain.User, bool) {
	for _, user := range s.usersByID {
		if user.TelegramLinkCode == code {
			return user, true
		}
	}
	return domain.User{}, false
}

func (s *authUserStoreStub) GetByTelegramChatID(_ context.Context, chatID int64) (domain.User, bool) {
	for _, user := range s.usersByID {
		if user.TelegramChatID == chatID {
			return user, true
		}
	}
	return domain.User{}, false
}

func (s *authUserStoreStub) UpdateBalance(_ context.Context, userID string, balanceRUB float64) error {
	user, ok := s.usersByID[userID]
	if !ok {
		return domain.ErrUserNotFound
	}
	user.BalanceRUB = balanceRUB
	s.usersByID[userID] = user
	return nil
}

func (s *authUserStoreStub) UpdateGitHubToken(_ context.Context, userID, encryptedToken string) error {
	user, ok := s.usersByID[userID]
	if !ok {
		return domain.ErrUserNotFound
	}
	user.GitHubTokenEncrypted = encryptedToken
	s.usersByID[userID] = user
	return nil
}

func (s *authUserStoreStub) UpdateTelegramSettings(_ context.Context, userID, username, linkCode string, linkExpiresAt *time.Time, enabled bool) error {
	user, ok := s.usersByID[userID]
	if !ok {
		return domain.ErrUserNotFound
	}
	user.TelegramUsername = username
	user.TelegramLinkCode = linkCode
	user.TelegramLinkExpiresAt = linkExpiresAt
	user.TelegramNotificationsEnabled = enabled
	s.usersByID[userID] = user
	return nil
}

func (s *authUserStoreStub) LinkTelegramChat(_ context.Context, userID string, chatID int64, linkedAt time.Time) error {
	user, ok := s.usersByID[userID]
	if !ok {
		return domain.ErrUserNotFound
	}
	user.TelegramChatID = chatID
	user.TelegramLinkedAt = &linkedAt
	user.TelegramNotificationsEnabled = true
	user.TelegramLinkCode = ""
	user.TelegramLinkExpiresAt = nil
	s.usersByID[userID] = user
	return nil
}

func (s *authUserStoreStub) DisableTelegramNotifications(_ context.Context, userID string) error {
	user, ok := s.usersByID[userID]
	if !ok {
		return domain.ErrUserNotFound
	}
	user.TelegramNotificationsEnabled = false
	s.usersByID[userID] = user
	return nil
}

func (s *authUserStoreStub) ClearTelegramSettings(_ context.Context, userID string) error {
	user, ok := s.usersByID[userID]
	if !ok {
		return domain.ErrUserNotFound
	}
	user.TelegramUsername = ""
	user.TelegramChatID = 0
	user.TelegramLinkedAt = nil
	user.TelegramLinkCode = ""
	user.TelegramLinkExpiresAt = nil
	user.TelegramNotificationsEnabled = false
	s.usersByID[userID] = user
	return nil
}

type authTxStoreStub struct{}

func (t *authTxStoreStub) Record(_ context.Context, _ domain.BillingTransaction) error {
	return nil
}
func (t *authTxStoreStub) ListByUser(_ context.Context, _ string) ([]domain.BillingTransaction, error) {
	return nil, nil
}
func (t *authTxStoreStub) ListByProject(_ context.Context, _ string) ([]domain.BillingTransaction, error) {
	return nil, nil
}

func (p *projectPortStub) CreateProject(_ context.Context, request domain.CreateProjectRequest) (domain.Project, error) {
	p.lastCreateRequest = request
	project := p.project
	project.Name = request.Name
	project.OwnerID = request.OwnerID
	return project, nil
}

func (p *projectPortStub) ListProjects(_ context.Context) []domain.Project {
	if p.projects != nil {
		return p.projects
	}
	return []domain.Project{p.project}
}

func (p *projectPortStub) GetProject(_ context.Context, _ string) (domain.Project, error) {
	return p.project, nil
}

func (p *projectPortStub) DeleteProject(_ context.Context, _ string) error {
	return nil
}

func (p *projectPortStub) GetProjectCost(_ context.Context, projectID string) (domain.CostBreakdown, error) {
	return domain.CostBreakdown{ProjectID: projectID, Total: 1, Currency: "USD"}, nil
}

func (p *projectPortStub) UpdateProjectDeploymentSettings(_ context.Context, _ string, request domain.UpdateDeploymentSettingsRequest) (domain.Project, error) {
	p.lastSettingsReq = request
	project := p.project
	project.RepositoryOwner = request.RepositoryOwner
	project.RepositoryName = request.RepositoryName
	project.BaseBranch = request.BaseBranch
	project.ServiceName = request.ServiceName
	project.DockerfilePath = request.DockerfilePath
	project.ServiceType = request.ServiceType
	project.ServicePort = request.ServicePort
	project.ContainerPort = request.ContainerPort
	project.ReplicaCount = request.ReplicaCount
	project.ResourceProfile = request.ResourceProfile
	project.DedicatedLoadBalancer = request.DedicatedLoadBalancer
	p.project = project
	return project, nil
}

func (p *projectPortStub) BuildGitHubBootstrapQuestions(_ context.Context, _ string, _ domain.GitHubBootstrapQuestionsRequest) (domain.GitHubBootstrapQuestionsResponse, error) {
	return domain.GitHubBootstrapQuestionsResponse{}, nil
}

func (p *projectPortStub) BootstrapGitHubFlow(_ context.Context, projectID string, _ domain.BootstrapGitHubFlowRequest) (domain.BootstrapGitHubFlowResponse, error) {
	if p.bootstrapErr != nil {
		return domain.BootstrapGitHubFlowResponse{}, p.bootstrapErr
	}
	return domain.BootstrapGitHubFlowResponse{ProjectID: projectID, PullRequestURL: "https://github.com/example/repo/pull/1"}, nil
}

func (p *projectPortStub) SuspendProject(_ context.Context, _ string) error {
	return nil
}

func (p *projectPortStub) ResumeProject(_ context.Context, _ string) error {
	return nil
}

func (p *projectPortStub) ListReleases(_ context.Context, _ string) ([]domain.Release, error) {
	if p.releases != nil {
		return p.releases, nil
	}
	return []domain.Release{}, nil
}

func (p *projectPortStub) GetRelease(_ context.Context, _, _ string) (domain.Release, error) {
	return domain.Release{}, nil
}

func (p *projectPortStub) HandleGitHubWebhook(_ context.Context, payload domain.GitHubWorkflowRunPayload) error {
	p.webhookPayload = payload
	return nil
}

func (p *projectPortStub) RollbackToRelease(_ context.Context, _, _ string) (domain.Release, error) {
	if p.rollbackErr != nil {
		return domain.Release{}, p.rollbackErr
	}
	return domain.Release{}, nil
}

func (p *projectPortStub) GetProjectKubeconfig(_ context.Context, _ string) (string, error) {
	if p.kubeconfigErr != nil {
		return "", p.kubeconfigErr
	}
	return "apiVersion: v1", nil
}

func (p *projectPortStub) RotateProjectKubeconfig(_ context.Context, _ string) (string, error) {
	if p.rotateKubeErr != nil {
		return "", p.rotateKubeErr
	}
	return "apiVersion: v1", nil
}

func (p *projectPortStub) GetProjectRuntimeStatus(_ context.Context, projectID string) (domain.ProjectRuntimeStatus, error) {
	status := p.runtimeStatus
	if status.ProjectID == "" {
		status.ProjectID = projectID
	}
	return status, nil
}

func (p *projectPortStub) EnforceBillingGuard(_ context.Context, userID string) error {
	p.enforcedUserID = userID
	return nil
}

func (p *projectPortStub) GetBillingSummary(_ context.Context, userID string) (domain.BillingSummary, error) {
	if p.billingSummary.UserID == "" {
		return domain.BillingSummary{
			UserID:         userID,
			BalanceRUB:     1000,
			SpentThisMonth: 1,
			AvailableRUB:   999,
		}, nil
	}
	return p.billingSummary, nil
}

func (p *projectPortStub) ListBillingTransactions(_ context.Context, _ string) ([]domain.BillingTransaction, error) {
	return nil, nil
}

func (p *projectPortStub) ListProjectBillingTransactions(_ context.Context, _, _ string) ([]domain.BillingTransaction, error) {
	return nil, nil
}

func (p *projectPortStub) GetServiceGitHubTokenStatus(_ context.Context, _ string) (domain.ServiceGitHubTokenStatus, error) {
	return p.serviceToken, nil
}

func (p *projectPortStub) UpsertServiceGitHubToken(_ context.Context, _ string, token string) (domain.ServiceGitHubTokenStatus, error) {
	if strings.TrimSpace(token) == "" {
		return domain.ServiceGitHubTokenStatus{}, errors.New("github token is required")
	}
	p.serviceToken = domain.ServiceGitHubTokenStatus{Configured: true}
	return p.serviceToken, nil
}

func (p *projectPortStub) DeleteServiceGitHubToken(_ context.Context, _ string) error {
	p.serviceToken = domain.ServiceGitHubTokenStatus{Configured: false}
	return nil
}

func (p *projectPortStub) CreateStage(_ context.Context, projectID, _ string, req domain.CreateStageRequest) (domain.Stage, error) {
	if p.createStageErr != nil {
		return domain.Stage{}, p.createStageErr
	}
	slug := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(req.Name), " ", "-"))
	stage := domain.Stage{
		ID:        "stage-" + projectID + "-" + slug,
		ProjectID: projectID,
		Name:      strings.TrimSpace(req.Name),
		Slug:      slug,
		Status:    domain.StageStatusActive,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	p.stages = append(p.stages, stage)
	return stage, nil
}

func (p *projectPortStub) ListStages(_ context.Context, _ string, _ string) ([]domain.Stage, error) {
	if p.stages == nil {
		return []domain.Stage{}, nil
	}
	return p.stages, nil
}

func (p *projectPortStub) GetStage(_ context.Context, _, stageID, _ string) (domain.Stage, error) {
	for _, stage := range p.stages {
		if stage.ID == stageID {
			return stage, nil
		}
	}
	return domain.Stage{}, domain.ErrStageNotFound
}

func (p *projectPortStub) DeleteStage(_ context.Context, _, stageID, _ string) error {
	filtered := make([]domain.Stage, 0, len(p.stages))
	found := false
	for _, stage := range p.stages {
		if stage.ID == stageID {
			found = true
			continue
		}
		filtered = append(filtered, stage)
	}
	if !found {
		return domain.ErrStageNotFound
	}
	p.stages = filtered
	return nil
}

func (p *projectPortStub) GetStageRuntimeStatus(_ context.Context, projectID, stageID, _ string) (domain.ProjectRuntimeStatus, error) {
	if p.stageRuntime.ProjectID != "" {
		return p.stageRuntime, nil
	}
	for _, stage := range p.stages {
		if stage.ID == stageID {
			return domain.ProjectRuntimeStatus{
				ProjectID:       projectID,
				Namespace:       stage.Slug,
				NamespaceExists: true,
			}, nil
		}
	}
	return domain.ProjectRuntimeStatus{}, domain.ErrStageNotFound
}

func (p *projectPortStub) ListProjectLogs(_ context.Context, projectID, _ string, request domain.ProjectLogsRequest) (domain.ProjectLogsResponse, error) {
	response := p.logsResponse
	if response.ProjectID == "" {
		response.ProjectID = projectID
	}
	if response.Namespace == "" {
		response.Namespace = "project-" + projectID
	}
	if response.StageID == "" {
		response.StageID = request.StageID
	}
	if response.StageSlug == "" && request.StageID != "" {
		response.StageSlug = request.StageID
	}
	if response.Entries == nil {
		response.Entries = []domain.ProjectLogEntry{}
	}
	return response, nil
}

func (p *projectPortStub) GetProjectGitHubToken(_ context.Context, projectID, _ string) (domain.ServiceGitHubTokenStatus, error) {
	return domain.ServiceGitHubTokenStatus{Configured: false}, nil
}

func (p *projectPortStub) UpsertProjectGitHubToken(_ context.Context, _, _, token string) (domain.ServiceGitHubTokenStatus, error) {
	if strings.TrimSpace(token) == "" {
		return domain.ServiceGitHubTokenStatus{}, errors.New("token is required")
	}
	return domain.ServiceGitHubTokenStatus{Configured: true}, nil
}

func (p *projectPortStub) DeleteProjectGitHubToken(_ context.Context, _, _ string) error {
	return nil
}

func (p *projectPortStub) GetProjectURLs(_ context.Context, _, _ string) (domain.ProjectURLsResponse, error) {
	return domain.ProjectURLsResponse{Stages: []domain.StageURLs{}}, nil
}

var _ service.Port = (*projectPortStub)(nil)

func TestProtectedRouteRequiresBearerToken(t *testing.T) {
	t.Parallel()

	handler := NewHandler(&projectPortStub{}, nil, "")
	router := NewRouter(handler, "secret")

	req := httptest.NewRequest(http.MethodGet, "/projects", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", rec.Code)
	}
}

func TestCreateProjectUsesAuthenticatedUserID(t *testing.T) {
	t.Parallel()

	projectSvc := &projectPortStub{
		project: domain.Project{
			ID:        "prj-1",
			Status:    domain.ProjectStatusActive,
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		},
	}
	handler := NewHandler(projectSvc, nil, "")
	router := NewRouter(handler, "secret")

	token, err := auth.GenerateToken("usr-real", "alice@example.com", "secret", time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}

	body := bytes.NewBufferString(`{"name":"demo","ownerId":"forged-owner"}`)
	req := httptest.NewRequest(http.MethodPost, "/projects", body)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected status 202, got %d with body %s", rec.Code, rec.Body.String())
	}
	if projectSvc.lastCreateRequest.OwnerID != "usr-real" {
		t.Fatalf("expected owner id from token, got %q", projectSvc.lastCreateRequest.OwnerID)
	}

	var response domain.Project
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.OwnerID != "usr-real" {
		t.Fatalf("expected response owner id %q, got %q", "usr-real", response.OwnerID)
	}
}

func TestGitHubWebhookRejectsInvalidSignature(t *testing.T) {
	t.Parallel()

	handler := NewHandler(&projectPortStub{}, nil, "top-secret")
	body := []byte(`{"workflow_run":{"id":1001,"status":"completed","conclusion":"success"}}`)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "workflow_run")
	req.Header.Set("X-Hub-Signature-256", "sha256=invalid")
	rec := httptest.NewRecorder()

	handler.GitHubWebhook(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", rec.Code)
	}
}

func TestGitHubWebhookAcceptsValidSignature(t *testing.T) {
	t.Parallel()

	projectSvc := &projectPortStub{
		billingSummary: domain.BillingSummary{
			UserID:         "usr-real",
			Email:          "alice@example.com",
			BalanceRUB:     1000,
			SpentThisMonth: 0,
			AvailableRUB:   1000,
		},
	}
	handler := NewHandler(projectSvc, nil, "top-secret")
	body := []byte(`{"workflow_run":{"id":1001,"status":"completed","conclusion":"success"}}`)

	mac := hmac.New(sha256.New, []byte("top-secret"))
	mac.Write(body)
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "workflow_run")
	req.Header.Set("X-Hub-Signature-256", signature)
	rec := httptest.NewRecorder()

	handler.GitHubWebhook(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d with body %s", rec.Code, rec.Body.String())
	}
	if projectSvc.webhookPayload.WorkflowRun.ID != 1001 {
		t.Fatalf("expected workflow run id 1001, got %d", projectSvc.webhookPayload.WorkflowRun.ID)
	}
}

func TestTelegramWebhookRejectsInvalidSecret(t *testing.T) {
	t.Parallel()

	handler := NewHandler(&projectPortStub{}, nil, "")
	handler.SetTelegramWebhookSecret("top-secret")

	req := httptest.NewRequest(http.MethodPost, "/webhooks/telegram", bytes.NewReader([]byte(`{}`)))
	rec := httptest.NewRecorder()

	handler.TelegramWebhook(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", rec.Code)
	}
}

func TestTelegramWebhookAcceptsValidSecret(t *testing.T) {
	t.Parallel()

	handler := NewHandler(&projectPortStub{}, nil, "")
	handler.SetTelegramWebhookSecret("top-secret")

	req := httptest.NewRequest(http.MethodPost, "/webhooks/telegram", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("X-Telegram-Bot-Api-Secret-Token", "top-secret")
	rec := httptest.NewRecorder()

	handler.TelegramWebhook(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
}

func TestGitHubBootstrapReturnsPaymentRequiredOnEmptyBalance(t *testing.T) {
	t.Parallel()

	projectSvc := &projectPortStub{
		project: domain.Project{
			ID:      "prj-1",
			OwnerID: "usr-real",
		},
		bootstrapErr: domain.ErrInsufficientBalance,
	}
	handler := NewHandler(projectSvc, nil, "")
	router := NewRouter(handler, "secret")

	token, err := auth.GenerateToken("usr-real", "alice@example.com", "secret", time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/projects/prj-1/github/bootstrap", bytes.NewBufferString(`{"repositoryOwner":"example","repositoryName":"repo"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusPaymentRequired {
		t.Fatalf("expected status 402, got %d with body %s", rec.Code, rec.Body.String())
	}
}

func TestRollbackReturnsPaymentRequiredOnEmptyBalance(t *testing.T) {
	t.Parallel()

	projectSvc := &projectPortStub{
		project: domain.Project{
			ID:      "prj-1",
			OwnerID: "usr-real",
		},
		rollbackErr: domain.ErrInsufficientBalance,
	}
	handler := NewHandler(projectSvc, nil, "")
	router := NewRouter(handler, "secret")

	token, err := auth.GenerateToken("usr-real", "alice@example.com", "secret", time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/projects/prj-1/releases/rel-1/rollback", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusPaymentRequired {
		t.Fatalf("expected status 402, got %d with body %s", rec.Code, rec.Body.String())
	}
}

func TestGetRuntimeStatusReturnsStatusPayload(t *testing.T) {
	t.Parallel()

	projectSvc := &projectPortStub{
		project: domain.Project{
			ID:      "prj-1",
			OwnerID: "usr-real",
		},
		runtimeStatus: domain.ProjectRuntimeStatus{
			ProjectID:        "prj-1",
			Namespace:        "project-prj-1",
			NamespaceExists:  true,
			DeploymentExists: true,
			ServiceExists:    true,
			ReadyReplicas:    1,
		},
	}
	handler := NewHandler(projectSvc, nil, "")
	router := NewRouter(handler, "secret")

	token, err := auth.GenerateToken("usr-real", "alice@example.com", "secret", time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/projects/prj-1/runtime-status", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d with body %s", rec.Code, rec.Body.String())
	}
	var status domain.ProjectRuntimeStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !status.DeploymentExists || status.ReadyReplicas != 1 {
		t.Fatalf("unexpected runtime status: %+v", status)
	}
}

func TestServiceGitHubTokenEndpointsLifecycle(t *testing.T) {
	t.Parallel()

	projectSvc := &projectPortStub{
		serviceToken: domain.ServiceGitHubTokenStatus{Configured: false},
	}
	handler := NewHandler(projectSvc, nil, "")
	router := NewRouter(handler, "secret")

	token, err := auth.GenerateToken("usr-real", "alice@example.com", "secret", time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}

	var status domain.ServiceGitHubTokenStatus

	getReq := httptest.NewRequest(http.MethodGet, "/service/github-token", nil)
	getReq.Header.Set("Authorization", "Bearer "+token)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d with body %s", getRec.Code, getRec.Body.String())
	}
	if err := json.Unmarshal(getRec.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode GET response: %v", err)
	}
	if status.Configured {
		t.Fatal("expected configured=false before upsert")
	}

	putReq := httptest.NewRequest(http.MethodPut, "/service/github-token", bytes.NewBufferString(`{"token":"ghp_new_token"}`))
	putReq.Header.Set("Authorization", "Bearer "+token)
	putReq.Header.Set("Content-Type", "application/json")
	putRec := httptest.NewRecorder()
	router.ServeHTTP(putRec, putReq)
	if putRec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d with body %s", putRec.Code, putRec.Body.String())
	}
	if err := json.Unmarshal(putRec.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode PUT response: %v", err)
	}
	if !status.Configured {
		t.Fatal("expected configured=true after upsert")
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/service/github-token", nil)
	deleteReq.Header.Set("Authorization", "Bearer "+token)
	deleteRec := httptest.NewRecorder()
	router.ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d with body %s", deleteRec.Code, deleteRec.Body.String())
	}
	if err := json.Unmarshal(deleteRec.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode DELETE response: %v", err)
	}
	if status.Configured {
		t.Fatal("expected configured=false after delete")
	}
}

func TestBillingSummaryReturnsBalanceAndSpentAmount(t *testing.T) {
	t.Parallel()

	userStore := newAuthUserStoreStub(domain.User{
		ID:         "usr-real",
		Email:      "alice@example.com",
		BalanceRUB: 1000,
	})
	authSvc := service.NewAuthService(userStore, &authTxStoreStub{}, "secret", time.Hour, 0)
	projectSvc := &projectPortStub{
		billingSummary: domain.BillingSummary{
			UserID:         "usr-real",
			Email:          "alice@example.com",
			BalanceRUB:     1000,
			SpentThisMonth: 1,
			AvailableRUB:   999,
		},
		projects: []domain.Project{
			{ID: "prj-1", OwnerID: "usr-real", Status: domain.ProjectStatusActive},
			{ID: "prj-2", OwnerID: "usr-other", Status: domain.ProjectStatusActive},
		},
	}
	handler := NewHandler(projectSvc, authSvc, "")
	router := NewRouter(handler, "secret")

	token, err := auth.GenerateToken("usr-real", "alice@example.com", "secret", time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/billing/summary", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d with body %s", rec.Code, rec.Body.String())
	}

	var summary domain.BillingSummary
	if err := json.Unmarshal(rec.Body.Bytes(), &summary); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if summary.BalanceRUB != 1000 || summary.SpentThisMonth != 1 || summary.AvailableRUB != 999 {
		t.Fatalf("unexpected billing summary: %+v", summary)
	}
	if projectSvc.enforcedUserID != "usr-real" {
		t.Fatalf("expected billing guard enforcement for usr-real, got %q", projectSvc.enforcedUserID)
	}
}

func TestTopUpBalanceAddsFunds(t *testing.T) {
	t.Parallel()

	userStore := newAuthUserStoreStub(domain.User{
		ID:         "usr-real",
		Email:      "alice@example.com",
		BalanceRUB: 0,
	})
	authSvc := service.NewAuthService(userStore, &authTxStoreStub{}, "secret", time.Hour, 0)
	projectSvc := &projectPortStub{
		billingSummary: domain.BillingSummary{
			UserID:         "usr-real",
			Email:          "alice@example.com",
			BalanceRUB:     1000,
			SpentThisMonth: 0,
			AvailableRUB:   1000,
		},
	}
	handler := NewHandler(projectSvc, authSvc, "")
	router := NewRouter(handler, "secret")

	token, err := auth.GenerateToken("usr-real", "alice@example.com", "secret", time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/billing/top-up", bytes.NewBufferString(`{"amountRub":1000}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d with body %s", rec.Code, rec.Body.String())
	}

	var summary domain.BillingSummary
	if err := json.Unmarshal(rec.Body.Bytes(), &summary); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if summary.BalanceRUB != 1000 || summary.AvailableRUB != 1000 {
		t.Fatalf("unexpected billing summary after top up: %+v", summary)
	}
}

func TestListProjectsReturnsOnlyOwnedProjects(t *testing.T) {
	t.Parallel()

	projectSvc := &projectPortStub{
		projects: []domain.Project{
			{ID: "prj-1", Name: "mine", OwnerID: "usr-real", Status: domain.ProjectStatusActive},
			{ID: "prj-2", Name: "foreign", OwnerID: "usr-other", Status: domain.ProjectStatusActive},
		},
	}
	handler := NewHandler(projectSvc, nil, "")
	router := NewRouter(handler, "secret")

	token, err := auth.GenerateToken("usr-real", "alice@example.com", "secret", time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/projects", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d with body %s", rec.Code, rec.Body.String())
	}

	var projects []domain.Project
	if err := json.Unmarshal(rec.Body.Bytes(), &projects); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("expected 1 owned project, got %d", len(projects))
	}
	if projects[0].OwnerID != "usr-real" {
		t.Fatalf("expected owned project, got owner %q", projects[0].OwnerID)
	}
}

func TestGetProjectRejectsForeignOwner(t *testing.T) {
	t.Parallel()

	projectSvc := &projectPortStub{
		project: domain.Project{
			ID:        "prj-1",
			Name:      "foreign",
			OwnerID:   "usr-other",
			Status:    domain.ProjectStatusActive,
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		},
	}
	handler := NewHandler(projectSvc, nil, "")
	router := NewRouter(handler, "secret")

	token, err := auth.GenerateToken("usr-real", "alice@example.com", "secret", time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/projects/prj-1", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d with body %s", rec.Code, rec.Body.String())
	}
}

func TestUpdateDeploymentSettingsPersistsNonSecretFields(t *testing.T) {
	t.Parallel()

	projectSvc := &projectPortStub{
		project: domain.Project{
			ID:        "prj-1",
			Name:      "demo",
			OwnerID:   "usr-real",
			Status:    domain.ProjectStatusActive,
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		},
	}
	handler := NewHandler(projectSvc, nil, "")
	router := NewRouter(handler, "secret")

	token, err := auth.GenerateToken("usr-real", "alice@example.com", "secret", time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}

	body := bytes.NewBufferString(`{"repositoryOwner":"testuser","repositoryName":"exams","baseBranch":"main","serviceName":"example-service","dockerfilePath":"Dockerfile","serviceType":"LoadBalancer","dedicatedLoadBalancer":true,"servicePort":80,"containerPort":8080,"replicaCount":3,"resourceProfile":"performance"}`)
	req := httptest.NewRequest(http.MethodPut, "/projects/prj-1/deployment-settings", body)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d with body %s", rec.Code, rec.Body.String())
	}
	if projectSvc.lastSettingsReq.RepositoryOwner != "testuser" {
		t.Fatalf("expected repository owner to be saved, got %q", projectSvc.lastSettingsReq.RepositoryOwner)
	}
	if projectSvc.lastSettingsReq.ServiceName != "example-service" {
		t.Fatalf("expected service name to be saved, got %q", projectSvc.lastSettingsReq.ServiceName)
	}
	if projectSvc.lastSettingsReq.ReplicaCount != 3 || projectSvc.lastSettingsReq.ResourceProfile != "performance" {
		t.Fatalf("expected sizing fields to be saved, got replicas=%d profile=%q", projectSvc.lastSettingsReq.ReplicaCount, projectSvc.lastSettingsReq.ResourceProfile)
	}
	if !projectSvc.lastSettingsReq.DedicatedLoadBalancer {
		t.Fatal("expected dedicated load balancer flag to be saved")
	}
}

func TestCreateStageCreatesStageForOwnedProject(t *testing.T) {
	t.Parallel()

	projectSvc := &projectPortStub{
		project: domain.Project{
			ID:      "prj-1",
			OwnerID: "usr-real",
		},
	}
	handler := NewHandler(projectSvc, nil, "")
	router := NewRouter(handler, "secret")

	token, err := auth.GenerateToken("usr-real", "alice@example.com", "secret", time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/projects/prj-1/stages", bytes.NewBufferString(`{"name":"Preprod"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d with body %s", rec.Code, rec.Body.String())
	}

	var stage domain.Stage
	if err := json.Unmarshal(rec.Body.Bytes(), &stage); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if stage.Slug != "preprod" {
		t.Fatalf("expected stage slug preprod, got %q", stage.Slug)
	}
}

func TestCreateStageReturnsConflictWhenProjectEnvironmentUnavailable(t *testing.T) {
	t.Parallel()

	projectSvc := &projectPortStub{
		project: domain.Project{
			ID:      "prj-1",
			OwnerID: "usr-real",
		},
		createStageErr: domain.ErrProjectEnvironmentUnavailable,
	}
	handler := NewHandler(projectSvc, nil, "")
	router := NewRouter(handler, "secret")

	token, err := auth.GenerateToken("usr-real", "alice@example.com", "secret", time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/projects/prj-1/stages", bytes.NewBufferString(`{"name":"Preprod"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected status 409, got %d with body %s", rec.Code, rec.Body.String())
	}
}

func TestGetKubeconfigReturnsConflictWhenProjectEnvironmentUnavailable(t *testing.T) {
	t.Parallel()

	projectSvc := &projectPortStub{
		project: domain.Project{
			ID:      "prj-1",
			OwnerID: "usr-real",
		},
		kubeconfigErr: domain.ErrProjectEnvironmentUnavailable,
	}
	handler := NewHandler(projectSvc, nil, "")
	router := NewRouter(handler, "secret")

	token, err := auth.GenerateToken("usr-real", "alice@example.com", "secret", time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/projects/prj-1/kubeconfig", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected status 409, got %d with body %s", rec.Code, rec.Body.String())
	}
}

func TestRotateKubeconfigReturnsConflictWhenProjectEnvironmentUnavailable(t *testing.T) {
	t.Parallel()

	projectSvc := &projectPortStub{
		project: domain.Project{
			ID:      "prj-1",
			OwnerID: "usr-real",
		},
		rotateKubeErr: domain.ErrProjectEnvironmentUnavailable,
	}
	handler := NewHandler(projectSvc, nil, "")
	router := NewRouter(handler, "secret")

	token, err := auth.GenerateToken("usr-real", "alice@example.com", "secret", time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/projects/prj-1/kubeconfig/rotate", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected status 409, got %d with body %s", rec.Code, rec.Body.String())
	}
}

func TestListReleasesSupportsStageIDFilter(t *testing.T) {
	t.Parallel()

	projectSvc := &projectPortStub{
		project: domain.Project{
			ID:      "prj-1",
			OwnerID: "usr-real",
		},
		releases: []domain.Release{
			{ID: "rel-1", ProjectID: "prj-1", StageID: "stage-prod"},
			{ID: "rel-2", ProjectID: "prj-1", StageID: "stage-preprod"},
		},
	}
	handler := NewHandler(projectSvc, nil, "")
	router := NewRouter(handler, "secret")

	token, err := auth.GenerateToken("usr-real", "alice@example.com", "secret", time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/projects/prj-1/releases?stageId=stage-preprod", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d with body %s", rec.Code, rec.Body.String())
	}

	var releases []domain.Release
	if err := json.Unmarshal(rec.Body.Bytes(), &releases); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(releases) != 1 {
		t.Fatalf("expected 1 release after stage filter, got %d", len(releases))
	}
	if releases[0].ID != "rel-2" {
		t.Fatalf("expected release rel-2, got %q", releases[0].ID)
	}
}
