//go:build !integration

package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"deploy-service/internal/domain"
)

type projectStoreStub struct {
	projects map[string]domain.Project
}

func newProjectStoreStub(projects ...domain.Project) *projectStoreStub {
	store := &projectStoreStub{projects: make(map[string]domain.Project)}
	for _, p := range projects {
		store.projects[p.ID] = p
	}
	return store
}

func (s *projectStoreStub) Create(_ context.Context, project domain.Project) error {
	s.projects[project.ID] = project
	return nil
}

func (s *projectStoreStub) GetByID(_ context.Context, projectID string) (domain.Project, bool) {
	project, ok := s.projects[projectID]
	return project, ok
}

func (s *projectStoreStub) List(_ context.Context) []domain.Project {
	result := make([]domain.Project, 0, len(s.projects))
	for _, p := range s.projects {
		result = append(result, p)
	}
	return result
}

func (s *projectStoreStub) Update(_ context.Context, project domain.Project) error {
	if existing, ok := s.projects[project.ID]; ok && project.KubeconfigEncrypted == "" {
		project.KubeconfigEncrypted = existing.KubeconfigEncrypted
	}
	s.projects[project.ID] = project
	return nil
}

func (s *projectStoreStub) UpdateKubeconfig(_ context.Context, projectID, encryptedKubeconfig string) error {
	project := s.projects[projectID]
	project.KubeconfigEncrypted = encryptedKubeconfig
	s.projects[projectID] = project
	return nil
}

func (s *projectStoreStub) UpdateGitHubToken(_ context.Context, projectID, encryptedToken string) error {
	project := s.projects[projectID]
	project.GitHubTokenEncrypted = encryptedToken
	s.projects[projectID] = project
	return nil
}

type releaseStoreStub struct {
	releases map[string]domain.Release
}

func newReleaseStoreStub(releases ...domain.Release) *releaseStoreStub {
	store := &releaseStoreStub{releases: make(map[string]domain.Release)}
	for _, r := range releases {
		store.releases[r.ID] = r
	}
	return store
}

func (s *releaseStoreStub) Create(_ context.Context, release domain.Release) error {
	s.releases[release.ID] = release
	return nil
}

func (s *releaseStoreStub) GetByID(_ context.Context, releaseID string) (domain.Release, bool) {
	release, ok := s.releases[releaseID]
	return release, ok
}

func (s *releaseStoreStub) ListByProject(_ context.Context, projectID string) []domain.Release {
	var result []domain.Release
	for _, r := range s.releases {
		if r.ProjectID == projectID {
			result = append(result, r)
		}
	}
	return result
}

func (s *releaseStoreStub) Update(_ context.Context, release domain.Release) error {
	s.releases[release.ID] = release
	return nil
}

func (s *releaseStoreStub) GetByWorkflowRunID(_ context.Context, runID int64) (domain.Release, bool) {
	for _, r := range s.releases {
		if r.WorkflowRunID == runID {
			return r, true
		}
	}
	return domain.Release{}, false
}

type provisionerStub struct {
	createCalled             bool
	createCalls              int
	createProjectErrs        []error
	createProjectAppsDomain  string
	setupImage               string
	kubeconfig               string
	kubeconfigErr            error
	createStageErr           error
	createStageCalls         int
	ensureGrafanaCalls       int
	ensurePublicIngressCalls int
	runtime                  domain.ProjectRuntimeStatus
	stageRuntime             domain.ProjectRuntimeStatus
	suspended                []string
	resumed                  []string
	deleted                  []string
}

func (p *provisionerStub) CreateProjectEnvironment(_ context.Context, _ string) (string, error) {
	p.createCalled = true
	p.createCalls++
	if len(p.createProjectErrs) > 0 {
		err := p.createProjectErrs[0]
		p.createProjectErrs = p.createProjectErrs[1:]
		if err != nil {
			return "", err
		}
	}
	return p.createProjectAppsDomain, nil
}

func (p *provisionerStub) DeleteProjectEnvironment(_ context.Context, projectID string) error {
	p.deleted = append(p.deleted, projectID)
	return nil
}

func (p *provisionerStub) SuspendProjectEnvironment(_ context.Context, projectID string) error {
	p.suspended = append(p.suspended, projectID)
	return nil
}

func (p *provisionerStub) ResumeProjectEnvironment(_ context.Context, projectID string) error {
	p.resumed = append(p.resumed, projectID)
	return nil
}

func (p *provisionerStub) ApplyImage(_ context.Context, _ string, imageTag string) error {
	p.setupImage = imageTag
	return nil
}

func (p *provisionerStub) GetProjectKubeconfig(_ context.Context, _ string) (string, error) {
	if p.kubeconfigErr != nil {
		return "", p.kubeconfigErr
	}
	return p.kubeconfig, nil
}

func (p *provisionerStub) GetProjectRuntimeStatus(_ context.Context, projectID string) (domain.ProjectRuntimeStatus, error) {
	if p.runtime.ProjectID == "" {
		p.runtime.ProjectID = projectID
	}
	return p.runtime, nil
}

func (p *provisionerStub) CreateStageEnvironment(_ context.Context, _, _ string) error {
	p.createStageCalls++
	return p.createStageErr
}
func (p *provisionerStub) DeleteStageEnvironment(_ context.Context, _, _ string) error { return nil }
func (p *provisionerStub) ApplyImageToStage(_ context.Context, _, _, _ string) error   { return nil }
func (p *provisionerStub) GetStageRuntimeStatus(_ context.Context, projectID, stageSlug string) (domain.ProjectRuntimeStatus, error) {
	result := p.stageRuntime
	if result.ProjectID == "" {
		result.ProjectID = projectID
	}
	if result.Namespace == "" {
		result.Namespace = stageSlug
	}
	return result, nil
}

func (p *provisionerStub) EnsureProjectPublicIngress(_ context.Context, _ string) error {
	p.ensurePublicIngressCalls++
	return nil
}

func (p *provisionerStub) EnsureProjectGrafana(_ context.Context, _ string) error {
	p.ensureGrafanaCalls++
	return nil
}

type automationStub struct {
	setupCalled    bool
	bootstrapResp  domain.BootstrapGitHubFlowResponse
	questionsResp  domain.GitHubBootstrapQuestionsResponse
	workflowRun    domain.GitHubWorkflowRunLookupResult
	workflowFound  bool
	lastBootstrap  domain.BootstrapGitHubFlowRequest
	lastQuestions  domain.GitHubBootstrapQuestionsRequest
	bootstrapBlock <-chan struct{}
	onBootstrap    func()
}

func (a *automationStub) SetupProjectAutomation(_ context.Context, _ string) error {
	a.setupCalled = true
	return nil
}

func (a *automationStub) BuildBootstrapQuestions(_ context.Context, _ string, request domain.GitHubBootstrapQuestionsRequest) (domain.GitHubBootstrapQuestionsResponse, error) {
	a.lastQuestions = request
	return a.questionsResp, nil
}

func (a *automationStub) BootstrapRepositoryFlow(_ context.Context, _ string, request domain.BootstrapGitHubFlowRequest) (domain.BootstrapGitHubFlowResponse, error) {
	a.lastBootstrap = request
	if a.onBootstrap != nil {
		a.onBootstrap()
	}
	if a.bootstrapBlock != nil {
		<-a.bootstrapBlock
	}
	return a.bootstrapResp, nil
}

func (a *automationStub) FindLatestDeployWorkflowRun(_ context.Context, _ domain.GitHubWorkflowRunLookupRequest) ([]domain.GitHubWorkflowRunLookupResult, error) {
	if !a.workflowFound {
		return nil, nil
	}
	return []domain.GitHubWorkflowRunLookupResult{a.workflowRun}, nil
}

type monetizationStub struct{}

func (m *monetizationStub) GetProjectCost(_ context.Context, projectID string) (domain.CostBreakdown, error) {
	return domain.CostBreakdown{ProjectID: projectID, Total: 42, Currency: "USD"}, nil
}

func (m *monetizationStub) ComputeUsageCost(_ domain.ResourceUsage) float64 { return 0 }

type stageStoreStub struct {
	stages map[string]domain.Stage
}

func newStageStoreStub() *stageStoreStub {
	return &stageStoreStub{stages: make(map[string]domain.Stage)}
}

func (s *stageStoreStub) Create(_ context.Context, stage domain.Stage) error {
	s.stages[stage.ID] = stage
	return nil
}
func (s *stageStoreStub) GetByID(_ context.Context, stageID string) (domain.Stage, bool) {
	stage, ok := s.stages[stageID]
	return stage, ok
}
func (s *stageStoreStub) GetBySlug(_ context.Context, _, slug string) (domain.Stage, bool) {
	for _, st := range s.stages {
		if st.Slug == slug {
			return st, true
		}
	}
	return domain.Stage{}, false
}
func (s *stageStoreStub) ListByProject(_ context.Context, projectID string) []domain.Stage {
	var result []domain.Stage
	for _, st := range s.stages {
		if st.ProjectID == projectID {
			result = append(result, st)
		}
	}
	return result
}
func (s *stageStoreStub) Update(_ context.Context, stage domain.Stage) error {
	s.stages[stage.ID] = stage
	return nil
}

type txStoreStub struct{}

func (t *txStoreStub) Record(_ context.Context, _ domain.BillingTransaction) error { return nil }
func (t *txStoreStub) ListByUser(_ context.Context, _ string) ([]domain.BillingTransaction, error) {
	return nil, nil
}
func (t *txStoreStub) ListByProject(_ context.Context, _ string) ([]domain.BillingTransaction, error) {
	return nil, nil
}

type cryptoStub struct{}

func (c *cryptoStub) Encrypt(plaintext string) (string, error) {
	return "enc:" + plaintext, nil
}

func (c *cryptoStub) Decrypt(ciphertext string) (string, error) {
	return strings.TrimPrefix(ciphertext, "enc:"), nil
}

func waitForProjectStatus(t *testing.T, store ProjectStore, projectID string, wantStatus domain.ProjectStatus) domain.Project {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if p, ok := store.GetByID(context.Background(), projectID); ok && p.Status == wantStatus {
			return p
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("project %s did not reach status %s within timeout", projectID, wantStatus)
	return domain.Project{}
}

func TestCreateProjectActivatesProjectWithoutFetchingKubeconfig(t *testing.T) {
	t.Parallel()

	projectStore := newProjectStoreStub()
	releaseStore := newReleaseStoreStub()
	provisioner := &provisionerStub{kubeconfig: "apiVersion: v1"}
	automation := &automationStub{}

	svc := NewProjectService(projectStore, releaseStore, newStageStoreStub(), provisioner, automation, &monetizationStub{}, newUserStoreStub(), &txStoreStub{}, &cryptoStub{}, "", "", "")

	project, err := svc.CreateProject(context.Background(), domain.CreateProjectRequest{
		Name:    "demo",
		OwnerID: "usr-1",
	})
	if err != nil {
		t.Fatalf("CreateProject returned error: %v", err)
	}
	if project.Status != domain.ProjectStatusCreating {
		t.Fatalf("expected creating status immediately, got %s", project.Status)
	}

	stored := waitForProjectStatus(t, projectStore, project.ID, domain.ProjectStatusActive)

	if !provisioner.createCalled {
		t.Fatal("expected Kubernetes environment creation to be called")
	}
	if !automation.setupCalled {
		t.Fatal("expected GitHub automation setup to be called")
	}
	if stored.KubeconfigEncrypted != "" {
		t.Fatalf("expected kubeconfig to stay empty until explicit fetch, got %q", stored.KubeconfigEncrypted)
	}
}

func TestCreateProjectRetriesTransientProvisioningFailure(t *testing.T) {
	previousRetryDelays := projectProvisionRetryDelays
	projectProvisionRetryDelays = []time.Duration{0, 0}
	defer func() {
		projectProvisionRetryDelays = previousRetryDelays
	}()

	projectStore := newProjectStoreStub()
	releaseStore := newReleaseStoreStub()
	provisioner := &provisionerStub{
		createProjectErrs: []error{
			errors.New(`failed to wait for kubeconfig secret vc-vcluster-prj-1: not ready yet; retry in a few seconds`),
			errors.New(`failed to wait for kubeconfig secret vc-vcluster-prj-1: not ready yet; retry in a few seconds`),
			nil,
		},
		kubeconfig: "apiVersion: v1",
	}
	automation := &automationStub{}

	svc := NewProjectService(projectStore, releaseStore, newStageStoreStub(), provisioner, automation, &monetizationStub{}, newUserStoreStub(), &txStoreStub{}, &cryptoStub{}, "", "", "")

	project, err := svc.CreateProject(context.Background(), domain.CreateProjectRequest{
		Name:    "demo",
		OwnerID: "usr-1",
	})
	if err != nil {
		t.Fatalf("CreateProject returned error: %v", err)
	}

	stored := waitForProjectStatus(t, projectStore, project.ID, domain.ProjectStatusActive)

	if provisioner.createCalls != 3 {
		t.Fatalf("expected 3 provision attempts, got %d", provisioner.createCalls)
	}
	if !automation.setupCalled {
		t.Fatal("expected automation setup after transient provisioning errors")
	}
	if stored.Status != domain.ProjectStatusActive {
		t.Fatalf("expected project to become active, got %s", stored.Status)
	}
}

func TestCreateProjectSetsGrafanaURLWhenAppsDomainConfigured(t *testing.T) {
	t.Parallel()

	projectStore := newProjectStoreStub()
	svc := NewProjectService(
		projectStore,
		newReleaseStoreStub(),
		newStageStoreStub(),
		&provisionerStub{kubeconfig: "apiVersion: v1"},
		&automationStub{},
		&monetizationStub{},
		newUserStoreStub(),
		&txStoreStub{},
		&cryptoStub{},
		"apps.example.com",
		"http",
		"32080",
	)

	project, err := svc.CreateProject(context.Background(), domain.CreateProjectRequest{
		Name:    "demo",
		OwnerID: "usr-1",
	})
	if err != nil {
		t.Fatalf("CreateProject returned error: %v", err)
	}

	want := "http://grafana-" + project.ID + ".apps.example.com/d/" + domain.ProjectGrafanaDashboardUID(project.ID) + "/project-metrics?orgId=1&refresh=30s"
	if project.GrafanaURL != want {
		t.Fatalf("expected grafana URL %q, got %q", want, project.GrafanaURL)
	}
}

func TestGetProjectKubeconfigReturnsReadableErrorWhenProjectEnvironmentUnavailable(t *testing.T) {
	t.Parallel()

	project := domain.Project{
		ID:        "prj-1",
		Name:      "demo",
		OwnerID:   "usr-1",
		Status:    domain.ProjectStatusActive,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	svc := NewProjectService(
		newProjectStoreStub(project),
		newReleaseStoreStub(),
		newStageStoreStub(),
		&provisionerStub{
			kubeconfigErr: errors.New(`get kubeconfig secret for project prj-1: kubectl --context deploy-context get secret vc-vcluster-prj-1 -n project-prj-1 -o jsonpath={.data.config} failed: exit status 1: Error from server (NotFound): secrets "vc-vcluster-prj-1" not found`),
		},
		&automationStub{},
		&monetizationStub{},
		newUserStoreStub(),
		&txStoreStub{},
		&cryptoStub{},
		"",
		"",
		"",
	)

	_, err := svc.GetProjectKubeconfig(context.Background(), project.ID)
	if err == nil {
		t.Fatal("expected kubeconfig fetch to fail when project environment is unavailable")
	}
	if !errors.Is(err, domain.ErrProjectEnvironmentUnavailable) {
		t.Fatalf("expected ErrProjectEnvironmentUnavailable, got %v", err)
	}
	if strings.Contains(strings.ToLower(err.Error()), "kubectl") || strings.Contains(err.Error(), "vc-vcluster-") {
		t.Fatalf("expected sanitized user-facing error, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "kubeconfig проекта пока недоступен") {
		t.Fatalf("expected kubeconfig availability guidance, got %q", err.Error())
	}
}

func TestBootstrapGitHubFlowSetsPublicURL(t *testing.T) {
	t.Parallel()

	project := domain.Project{
		ID:        "prj-1",
		Name:      "demo",
		OwnerID:   "usr-1",
		Status:    domain.ProjectStatusActive,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	projectStore := newProjectStoreStub(project)
	releaseStore := newReleaseStoreStub()
	automation := &automationStub{
		bootstrapResp: domain.BootstrapGitHubFlowResponse{
			ProjectID:      project.ID,
			PullRequestURL: "https://github.com/example/repo/pull/1",
		},
	}
	users := newUserStoreStub(domain.User{ID: "usr-1", Email: "alice@example.com", BalanceRUB: 100})

	svc := NewProjectService(projectStore, releaseStore, newStageStoreStub(), &provisionerStub{}, automation, &monetizationStub{}, users, &txStoreStub{}, &cryptoStub{}, "apps.example.com", "http", "32080")

	resp, err := svc.BootstrapGitHubFlow(context.Background(), project.ID, domain.BootstrapGitHubFlowRequest{
		RepositoryOwner: "example",
		RepositoryName:  "repo",
		ServiceType:     "LoadBalancer",
	})
	if err != nil {
		t.Fatalf("BootstrapGitHubFlow returned error: %v", err)
	}

	if resp.PullRequestURL == "" {
		t.Fatal("expected pull request URL in response")
	}

	stored, _ := projectStore.GetByID(context.Background(), project.ID)
	wantURL := "http://prj-1.apps.example.com:32080"
	if stored.PublicURL != wantURL {
		t.Fatalf("expected public URL %q, got %q", wantURL, stored.PublicURL)
	}
	if stored.RepositoryOwner != "example" || stored.RepositoryName != "repo" {
		t.Fatalf("expected repository info to be saved, got %q/%q", stored.RepositoryOwner, stored.RepositoryName)
	}
}

func TestBootstrapGitHubFlowFallsBackToStoredRepositoryWhenRequestRepositoryMissing(t *testing.T) {
	t.Parallel()

	project := domain.Project{
		ID:              "prj-1",
		Name:            "demo",
		OwnerID:         "usr-1",
		Status:          domain.ProjectStatusActive,
		RepositoryOwner: "stored-owner",
		RepositoryName:  "stored-repo",
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}
	projectStore := newProjectStoreStub(project)
	automation := &automationStub{
		bootstrapResp: domain.BootstrapGitHubFlowResponse{
			ProjectID:      project.ID,
			PullRequestURL: "https://github.com/example/repo/pull/1",
		},
	}
	users := newUserStoreStub(domain.User{ID: "usr-1", Email: "alice@example.com", BalanceRUB: 100})
	svc := NewProjectService(projectStore, newReleaseStoreStub(), newStageStoreStub(), &provisionerStub{}, automation, &monetizationStub{}, users, &txStoreStub{}, &cryptoStub{}, "", "", "")

	_, err := svc.BootstrapGitHubFlow(context.Background(), project.ID, domain.BootstrapGitHubFlowRequest{
		ServiceType: "LoadBalancer",
	})
	if err != nil {
		t.Fatalf("BootstrapGitHubFlow returned error: %v", err)
	}

	if automation.lastBootstrap.RepositoryOwner != "stored-owner" || automation.lastBootstrap.RepositoryName != "stored-repo" {
		t.Fatalf("expected automation request to use stored repository, got %q/%q", automation.lastBootstrap.RepositoryOwner, automation.lastBootstrap.RepositoryName)
	}
}

func TestBuildGitHubBootstrapQuestionsFallsBackToStoredRepositoryWhenRequestRepositoryMissing(t *testing.T) {
	t.Parallel()

	project := domain.Project{
		ID:              "prj-1",
		Name:            "demo",
		OwnerID:         "usr-1",
		Status:          domain.ProjectStatusActive,
		RepositoryOwner: "stored-owner",
		RepositoryName:  "stored-repo",
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}
	projectStore := newProjectStoreStub(project)
	automation := &automationStub{
		questionsResp: domain.GitHubBootstrapQuestionsResponse{
			RepositoryOwner:       "stored-owner",
			RepositoryName:        "stored-repo",
			BaseBranch:            "main",
			DetectedDockerfile:    "Dockerfile",
			DetectedServiceName:   "service",
			DetectedContainerPort: 8080,
			DetectedServicePort:   80,
			DetectedServiceType:   "LoadBalancer",
		},
	}
	svc := NewProjectService(projectStore, newReleaseStoreStub(), newStageStoreStub(), &provisionerStub{}, automation, &monetizationStub{}, newUserStoreStub(), &txStoreStub{}, &cryptoStub{}, "", "", "")

	_, err := svc.BuildGitHubBootstrapQuestions(context.Background(), project.ID, domain.GitHubBootstrapQuestionsRequest{
		GitHubToken: "ghp_test_token",
	})
	if err != nil {
		t.Fatalf("BuildGitHubBootstrapQuestions returned error: %v", err)
	}

	if automation.lastQuestions.RepositoryOwner != "stored-owner" || automation.lastQuestions.RepositoryName != "stored-repo" {
		t.Fatalf("expected questions request to use stored repository, got %q/%q", automation.lastQuestions.RepositoryOwner, automation.lastQuestions.RepositoryName)
	}
}

func TestUpdateProjectDeploymentSettingsPersistsConfiguration(t *testing.T) {
	t.Parallel()

	project := domain.Project{
		ID:        "prj-1",
		Name:      "demo",
		OwnerID:   "usr-1",
		Status:    domain.ProjectStatusActive,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	projectStore := newProjectStoreStub(project)
	svc := NewProjectService(projectStore, newReleaseStoreStub(), newStageStoreStub(), &provisionerStub{}, &automationStub{}, &monetizationStub{}, newUserStoreStub(), &txStoreStub{}, &cryptoStub{}, "", "", "")

	updated, err := svc.UpdateProjectDeploymentSettings(context.Background(), project.ID, domain.UpdateDeploymentSettingsRequest{
		RepositoryOwner:       "testuser",
		RepositoryName:        "exams",
		BaseBranch:            "main",
		ServiceName:           "example-service",
		DockerfilePath:        "Dockerfile",
		ServiceType:           "LoadBalancer",
		DedicatedLoadBalancer: true,
		ServicePort:           80,
		ContainerPort:         8080,
		ReplicaCount:          3,
		ResourceProfile:       "performance",
	})
	if err != nil {
		t.Fatalf("UpdateProjectDeploymentSettings returned error: %v", err)
	}

	if updated.RepositoryOwner != "testuser" || updated.RepositoryName != "exams" {
		t.Fatalf("expected repository settings to be persisted, got %q/%q", updated.RepositoryOwner, updated.RepositoryName)
	}
	if updated.ServicePort != 80 || updated.ContainerPort != 8080 {
		t.Fatalf("expected ports to be persisted, got %d/%d", updated.ServicePort, updated.ContainerPort)
	}
	if updated.ReplicaCount != 3 || updated.ResourceProfile != "performance" {
		t.Fatalf("expected sizing settings to be persisted, got replicas=%d profile=%q", updated.ReplicaCount, updated.ResourceProfile)
	}
	if !updated.DedicatedLoadBalancer {
		t.Fatal("expected dedicated load balancer flag to be persisted")
	}
}

func TestBootstrapGitHubFlowPreservesExistingDetectedSettingsWhenRequestLeavesThemBlank(t *testing.T) {
	t.Parallel()

	project := domain.Project{
		ID:             "prj-1",
		Name:           "demo",
		OwnerID:        "usr-1",
		Status:         domain.ProjectStatusActive,
		BaseBranch:     "main",
		ServiceName:    "example-service",
		DockerfilePath: "Dockerfile",
		ServiceType:    "LoadBalancer",
		ServicePort:    80,
		ContainerPort:  8080,
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}
	projectStore := newProjectStoreStub(project)
	automation := &automationStub{
		bootstrapResp: domain.BootstrapGitHubFlowResponse{
			ProjectID:      project.ID,
			PullRequestURL: "https://github.com/example/repo/pull/1",
		},
	}
	users := newUserStoreStub(domain.User{ID: "usr-1", Email: "alice@example.com", BalanceRUB: 100})
	svc := NewProjectService(projectStore, newReleaseStoreStub(), newStageStoreStub(), &provisionerStub{}, automation, &monetizationStub{}, users, &txStoreStub{}, &cryptoStub{}, "", "", "")

	_, err := svc.BootstrapGitHubFlow(context.Background(), project.ID, domain.BootstrapGitHubFlowRequest{
		RepositoryOwner: "testuser",
		RepositoryName:  "exams",
	})
	if err != nil {
		t.Fatalf("BootstrapGitHubFlow returned error: %v", err)
	}

	stored, _ := projectStore.GetByID(context.Background(), project.ID)
	if stored.BaseBranch != "main" || stored.ServiceName != "example-service" {
		t.Fatalf("expected existing inferred settings to be preserved, got baseBranch=%q serviceName=%q", stored.BaseBranch, stored.ServiceName)
	}
}

func TestBootstrapGitHubFlowDefaultsStageSlugToProduction(t *testing.T) {
	t.Parallel()

	project := domain.Project{
		ID:        "prj-1",
		Name:      "demo",
		OwnerID:   "usr-1",
		Status:    domain.ProjectStatusActive,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	projectStore := newProjectStoreStub(project)
	stageStore := newStageStoreStub()
	automation := &automationStub{
		bootstrapResp: domain.BootstrapGitHubFlowResponse{
			ProjectID:      project.ID,
			PullRequestURL: "https://github.com/example/repo/pull/1",
		},
	}
	users := newUserStoreStub(domain.User{ID: "usr-1", Email: "alice@example.com", BalanceRUB: 100})
	svc := NewProjectService(projectStore, newReleaseStoreStub(), stageStore, &provisionerStub{}, automation, &monetizationStub{}, users, &txStoreStub{}, &cryptoStub{}, "", "", "")

	_, err := svc.BootstrapGitHubFlow(context.Background(), project.ID, domain.BootstrapGitHubFlowRequest{
		RepositoryOwner: "example",
		RepositoryName:  "repo",
	})
	if err != nil {
		t.Fatalf("BootstrapGitHubFlow returned error: %v", err)
	}

	if automation.lastBootstrap.StageSlug != "production" {
		t.Fatalf("expected production stage slug by default, got %q", automation.lastBootstrap.StageSlug)
	}
	if _, ok := stageStore.GetBySlug(context.Background(), project.ID, "production"); !ok {
		t.Fatal("expected production stage to be created for bootstrap")
	}
}

func TestBootstrapGitHubFlowCreatesPendingReleaseEntry(t *testing.T) {
	t.Parallel()

	project := domain.Project{
		ID:        "prj-1",
		Name:      "demo",
		OwnerID:   "usr-1",
		Status:    domain.ProjectStatusActive,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	projectStore := newProjectStoreStub(project)
	releaseStore := newReleaseStoreStub()
	stageStore := newStageStoreStub()
	automation := &automationStub{
		bootstrapResp: domain.BootstrapGitHubFlowResponse{
			ProjectID:       project.ID,
			RepositoryOwner: "myorg",
			RepositoryName:  "myrepo",
			BranchName:      "deploy-service/demo",
			PullRequestURL:  "https://github.com/myorg/myrepo/pull/42",
		},
	}
	users := newUserStoreStub(domain.User{ID: "usr-1", Email: "alice@example.com", BalanceRUB: 100})
	svc := NewProjectService(projectStore, releaseStore, stageStore, &provisionerStub{}, automation, &monetizationStub{}, users, &txStoreStub{}, &cryptoStub{}, "", "", "")

	_, err := svc.BootstrapGitHubFlow(context.Background(), project.ID, domain.BootstrapGitHubFlowRequest{
		RepositoryOwner: "myorg",
		RepositoryName:  "myrepo",
		BaseBranch:      "main",
		ServiceName:     "api",
		DockerfilePath:  "Dockerfile",
		GitHubToken:     "token",
	})
	if err != nil {
		t.Fatalf("BootstrapGitHubFlow returned error: %v", err)
	}

	releases := releaseStore.ListByProject(context.Background(), project.ID)
	if len(releases) != 1 {
		t.Fatalf("expected exactly 1 pending release entry, got %d", len(releases))
	}
	release := releases[0]
	if release.Status != domain.ReleaseStatusPending {
		t.Fatalf("expected pending release status, got %s", release.Status)
	}
	if release.StageID == "" {
		t.Fatal("expected pending release to be linked to the production stage")
	}
	if release.WorkflowRunID != 0 {
		t.Fatalf("expected pending release without workflowRunID yet, got %d", release.WorkflowRunID)
	}
	if !strings.Contains(release.CommitMessage, "pull/42") {
		t.Fatalf("expected pending release to reference PR url, got %q", release.CommitMessage)
	}
}

func TestBootstrapGitHubFlowRejectsUnknownStageSlug(t *testing.T) {
	t.Parallel()

	project := domain.Project{
		ID:        "prj-1",
		Name:      "demo",
		OwnerID:   "usr-1",
		Status:    domain.ProjectStatusActive,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	projectStore := newProjectStoreStub(project)
	automation := &automationStub{
		bootstrapResp: domain.BootstrapGitHubFlowResponse{
			ProjectID:      project.ID,
			PullRequestURL: "https://github.com/example/repo/pull/1",
		},
	}
	users := newUserStoreStub(domain.User{ID: "usr-1", Email: "alice@example.com", BalanceRUB: 100})
	svc := NewProjectService(projectStore, newReleaseStoreStub(), newStageStoreStub(), &provisionerStub{}, automation, &monetizationStub{}, users, &txStoreStub{}, &cryptoStub{}, "", "", "")

	_, err := svc.BootstrapGitHubFlow(context.Background(), project.ID, domain.BootstrapGitHubFlowRequest{
		RepositoryOwner: "example",
		RepositoryName:  "repo",
		StageSlug:       "preprod",
	})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not found error for unknown stage slug, got %v", err)
	}
}

func TestCreateStageReturnsReadableErrorWhenProjectEnvironmentIsMissing(t *testing.T) {
	t.Parallel()

	project := domain.Project{
		ID:        "prj-1",
		Name:      "demo",
		OwnerID:   "usr-1",
		Status:    domain.ProjectStatusActive,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	projectStore := newProjectStoreStub(project)
	stageStore := newStageStoreStub()
	provisioner := &provisionerStub{
		kubeconfigErr: errors.New(`get kubeconfig secret for project prj-1: kubectl --context deploy-context get secret vc-vcluster-prj-1 -n project-prj-1 -o jsonpath={.data.config} failed: exit status 1: Error from server (NotFound): namespaces "project-prj-1" not found`),
	}

	svc := NewProjectService(
		projectStore,
		newReleaseStoreStub(),
		stageStore,
		provisioner,
		&automationStub{},
		&monetizationStub{},
		newUserStoreStub(),
		&txStoreStub{},
		&cryptoStub{},
		"",
		"",
		"",
	)

	_, err := svc.CreateStage(context.Background(), project.ID, "usr-1", domain.CreateStageRequest{Name: "Preprod"})
	if err == nil {
		t.Fatal("expected stage creation to fail for missing project namespace")
	}
	if !errors.Is(err, domain.ErrProjectEnvironmentUnavailable) {
		t.Fatalf("expected ErrProjectEnvironmentUnavailable, got %v", err)
	}
	if strings.Contains(strings.ToLower(err.Error()), "kubectl") {
		t.Fatalf("expected sanitized user-facing error, got %q", err.Error())
	}
	if len(stageStore.ListByProject(context.Background(), project.ID)) != 0 {
		t.Fatalf("expected no persisted stage when prevalidation fails")
	}
	if provisioner.createStageCalls != 0 {
		t.Fatalf("expected no CreateStageEnvironment calls on failed prevalidation, got %d", provisioner.createStageCalls)
	}
}

func TestCreateStageMarksStageFailedWhenEnvironmentCreationFails(t *testing.T) {
	t.Parallel()

	project := domain.Project{
		ID:        "prj-1",
		Name:      "demo",
		OwnerID:   "usr-1",
		Status:    domain.ProjectStatusActive,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	projectStore := newProjectStoreStub(project)
	stageStore := newStageStoreStub()
	provisioner := &provisionerStub{
		kubeconfig:     "apiVersion: v1",
		createStageErr: errors.New(`get vcluster kubeconfig for project prj-1: get kubeconfig secret for project prj-1: secrets "vc-vcluster-prj-1" not found`),
	}

	svc := NewProjectService(
		projectStore,
		newReleaseStoreStub(),
		stageStore,
		provisioner,
		&automationStub{},
		&monetizationStub{},
		newUserStoreStub(),
		&txStoreStub{},
		&cryptoStub{},
		"",
		"",
		"",
	)

	_, err := svc.CreateStage(context.Background(), project.ID, "usr-1", domain.CreateStageRequest{Name: "Preprod"})
	if err == nil {
		t.Fatal("expected stage creation to fail for missing vcluster secret")
	}
	if !errors.Is(err, domain.ErrProjectEnvironmentUnavailable) {
		t.Fatalf("expected ErrProjectEnvironmentUnavailable, got %v", err)
	}
	if provisioner.createStageCalls != 1 {
		t.Fatalf("expected CreateStageEnvironment to be called once, got %d", provisioner.createStageCalls)
	}

	stage, ok := stageStore.GetBySlug(context.Background(), project.ID, "preprod")
	if !ok {
		t.Fatal("expected failed stage to be present in store")
	}
	if stage.Status != domain.StageStatusFailed {
		t.Fatalf("expected failed stage status, got %s", stage.Status)
	}
}
func TestHandleGitHubWebhookUpdatesReleaseStatus(t *testing.T) {
	t.Parallel()

	releaseStore := newReleaseStoreStub(domain.Release{
		ID:            "rel-1",
		ProjectID:     "prj-1",
		Status:        domain.ReleaseStatusPending,
		WorkflowRunID: 1001,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	})
	svc := NewProjectService(newProjectStoreStub(), releaseStore, newStageStoreStub(), &provisionerStub{}, &automationStub{}, &monetizationStub{}, newUserStoreStub(), &txStoreStub{}, &cryptoStub{}, "", "", "")

	payload := domain.GitHubWorkflowRunPayload{}
	payload.WorkflowRun.ID = 1001
	payload.WorkflowRun.Status = "completed"
	payload.WorkflowRun.Conclusion = "success"

	if err := svc.HandleGitHubWebhook(context.Background(), payload); err != nil {
		t.Fatalf("HandleGitHubWebhook returned error: %v", err)
	}

	updated, _ := releaseStore.GetByID(context.Background(), "rel-1")
	if updated.Status != domain.ReleaseStatusSuccess {
		t.Fatalf("expected success status, got %s", updated.Status)
	}
}

func TestRollbackToReleaseCreatesNewSuccessfulRelease(t *testing.T) {
	t.Parallel()

	project := domain.Project{
		ID:        "prj-1",
		Name:      "demo",
		OwnerID:   "usr-1",
		Status:    domain.ProjectStatusActive,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	target := domain.Release{
		ID:        "rel-success",
		ProjectID: "prj-1",
		ImageTag:  "ghcr.io/example/app:sha",
		CommitSHA: "abc123",
		Status:    domain.ReleaseStatusSuccess,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	releaseStore := newReleaseStoreStub(target)
	provisioner := &provisionerStub{}
	svc := NewProjectService(
		newProjectStoreStub(project),
		releaseStore,
		newStageStoreStub(),
		provisioner,
		&automationStub{},
		&monetizationStub{},
		newUserStoreStub(domain.User{ID: "usr-1", Email: "alice@example.com", BalanceRUB: 100}),
		&txStoreStub{},
		&cryptoStub{},
		"",
		"",
		"",
	)

	rollback, err := svc.RollbackToRelease(context.Background(), "prj-1", "rel-success")
	if err != nil {
		t.Fatalf("RollbackToRelease returned error: %v", err)
	}

	if rollback.Status != domain.ReleaseStatusSuccess {
		t.Fatalf("expected rollback status success, got %s", rollback.Status)
	}
	if rollback.ID == target.ID {
		t.Fatal("expected rollback to create a new release record")
	}
	if provisioner.setupImage != target.ImageTag {
		t.Fatalf("expected image %q to be applied, got %q", target.ImageTag, provisioner.setupImage)
	}
}

func TestRollbackToReleaseRejectsWhenBalanceInsufficient(t *testing.T) {
	t.Parallel()

	project := domain.Project{
		ID:        "prj-1",
		Name:      "demo",
		OwnerID:   "usr-1",
		Status:    domain.ProjectStatusActive,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	target := domain.Release{
		ID:        "rel-success",
		ProjectID: "prj-1",
		ImageTag:  "ghcr.io/example/app:sha",
		CommitSHA: "abc123",
		Status:    domain.ReleaseStatusSuccess,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	svc := NewProjectService(
		newProjectStoreStub(project),
		newReleaseStoreStub(target),
		newStageStoreStub(),
		&provisionerStub{},
		&automationStub{},
		&monetizationStub{},
		newUserStoreStub(domain.User{ID: "usr-1", Email: "alice@example.com", BalanceRUB: 0}),
		&txStoreStub{},
		&cryptoStub{},
		"",
		"",
		"",
	)

	_, err := svc.RollbackToRelease(context.Background(), "prj-1", "rel-success")
	if err == nil || !strings.Contains(err.Error(), domain.ErrInsufficientBalance.Error()) {
		t.Fatalf("expected insufficient balance error, got %v", err)
	}
}

func TestUpdateDeploymentSettingsPersistsToStore(t *testing.T) {
	t.Parallel()

	project := domain.Project{
		ID:        "prj-1",
		Name:      "demo",
		OwnerID:   "usr-1",
		Status:    domain.ProjectStatusActive,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	projectStore := newProjectStoreStub(project)
	svc := NewProjectService(projectStore, newReleaseStoreStub(), newStageStoreStub(), &provisionerStub{}, &automationStub{}, &monetizationStub{}, newUserStoreStub(), &txStoreStub{}, &cryptoStub{}, "", "", "")

	_, err := svc.UpdateProjectDeploymentSettings(context.Background(), "prj-1", domain.UpdateDeploymentSettingsRequest{
		ServiceName:     "mysvc",
		DockerfilePath:  "Dockerfile",
		ContainerPort:   8080,
		ServicePort:     80,
		ServiceType:     "LoadBalancer",
		BaseBranch:      "main",
		RepositoryOwner: "myorg",
		RepositoryName:  "myrepo",
	})
	if err != nil {
		t.Fatalf("UpdateProjectDeploymentSettings returned error: %v", err)
	}

	got, err := svc.GetProject(context.Background(), "prj-1")
	if err != nil {
		t.Fatalf("GetProject returned error: %v", err)
	}

	if got.ServiceName != "mysvc" {
		t.Errorf("ServiceName: got %q, want %q", got.ServiceName, "mysvc")
	}
	if got.DockerfilePath != "Dockerfile" {
		t.Errorf("DockerfilePath: got %q, want %q", got.DockerfilePath, "Dockerfile")
	}
	if got.ContainerPort != 8080 {
		t.Errorf("ContainerPort: got %d, want %d", got.ContainerPort, 8080)
	}
	if got.ServicePort != 80 {
		t.Errorf("ServicePort: got %d, want %d", got.ServicePort, 80)
	}
	if got.ServiceType != "LoadBalancer" {
		t.Errorf("ServiceType: got %q, want %q", got.ServiceType, "LoadBalancer")
	}
	if got.BaseBranch != "main" {
		t.Errorf("BaseBranch: got %q, want %q", got.BaseBranch, "main")
	}
	if got.RepositoryOwner != "myorg" {
		t.Errorf("RepositoryOwner: got %q, want %q", got.RepositoryOwner, "myorg")
	}
	if got.RepositoryName != "myrepo" {
		t.Errorf("RepositoryName: got %q, want %q", got.RepositoryName, "myrepo")
	}
}

func TestGetProjectHydratesPublicAndGrafanaURLsWhenMissing(t *testing.T) {
	t.Parallel()

	project := domain.Project{
		ID:          "prj-1",
		Name:        "demo",
		OwnerID:     "usr-1",
		Status:      domain.ProjectStatusActive,
		ServiceType: "LoadBalancer",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	projectStore := newProjectStoreStub(project)
	svc := NewProjectService(
		projectStore,
		newReleaseStoreStub(),
		newStageStoreStub(),
		&provisionerStub{},
		&automationStub{},
		&monetizationStub{},
		newUserStoreStub(),
		&txStoreStub{},
		&cryptoStub{},
		"apps.example.com",
		"https",
		"",
	)

	got, err := svc.GetProject(context.Background(), project.ID)
	if err != nil {
		t.Fatalf("GetProject returned error: %v", err)
	}

	if got.PublicURL != "https://prj-1.apps.example.com" {
		t.Fatalf("expected hydrated public URL, got %q", got.PublicURL)
	}
	if got.GrafanaURL != "https://grafana-prj-1.apps.example.com/d/"+domain.ProjectGrafanaDashboardUID("prj-1")+"/project-metrics?orgId=1&refresh=30s" {
		t.Fatalf("expected hydrated grafana URL, got %q", got.GrafanaURL)
	}
}

func TestGetProjectHydratesPublicIngressForLoadBalancerProject(t *testing.T) {
	t.Parallel()

	project := domain.Project{
		ID:          "prj-1",
		Name:        "demo",
		OwnerID:     "usr-1",
		Status:      domain.ProjectStatusActive,
		ServiceType: "LoadBalancer",
		PublicURL:   "https://prj-1.apps.example.com",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	projectStore := newProjectStoreStub(project)
	provisioner := &provisionerStub{}
	svc := NewProjectService(
		projectStore,
		newReleaseStoreStub(),
		newStageStoreStub(),
		provisioner,
		&automationStub{},
		&monetizationStub{},
		newUserStoreStub(),
		&txStoreStub{},
		&cryptoStub{},
		"apps.example.com",
		"https",
		"",
	)

	if _, err := svc.GetProject(context.Background(), project.ID); err != nil {
		t.Fatalf("GetProject returned error: %v", err)
	}

	if provisioner.ensurePublicIngressCalls != 0 {
		t.Fatalf("expected GetProject not to call EnsureProjectPublicIngress, got %d", provisioner.ensurePublicIngressCalls)
	}
}

func TestGetProjectHydratesGrafanaForExistingProjectWithoutProvisionerCall(t *testing.T) {
	t.Parallel()

	project := domain.Project{
		ID:         "prj-1",
		Name:       "demo",
		OwnerID:    "usr-1",
		Status:     domain.ProjectStatusActive,
		GrafanaURL: "https://grafana-prj-1.apps.example.com/d/project-prj-1/project-metrics?orgId=1&refresh=30s",
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}
	projectStore := newProjectStoreStub(project)
	provisioner := &provisionerStub{}
	svc := NewProjectService(
		projectStore,
		newReleaseStoreStub(),
		newStageStoreStub(),
		provisioner,
		&automationStub{},
		&monetizationStub{},
		newUserStoreStub(),
		&txStoreStub{},
		&cryptoStub{},
		"apps.example.com",
		"https",
		"",
	)

	if _, err := svc.GetProject(context.Background(), project.ID); err != nil {
		t.Fatalf("GetProject returned error: %v", err)
	}

	if provisioner.ensureGrafanaCalls != 0 {
		t.Fatalf("expected GetProject not to call EnsureProjectGrafana, got %d", provisioner.ensureGrafanaCalls)
	}
}

func TestListProjectsHydratesProjectLinksWhenMissing(t *testing.T) {
	t.Parallel()

	project := domain.Project{
		ID:          "prj-1",
		Name:        "demo",
		OwnerID:     "usr-1",
		Status:      domain.ProjectStatusActive,
		ServiceType: "LoadBalancer",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	projectStore := newProjectStoreStub(project)
	svc := NewProjectService(
		projectStore,
		newReleaseStoreStub(),
		newStageStoreStub(),
		&provisionerStub{},
		&automationStub{},
		&monetizationStub{},
		newUserStoreStub(),
		&txStoreStub{},
		&cryptoStub{},
		"apps.example.com",
		"https",
		"",
	)

	projects := svc.ListProjects(context.Background())
	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}
	if projects[0].PublicURL != "https://prj-1.apps.example.com" {
		t.Fatalf("expected hydrated public URL in list, got %q", projects[0].PublicURL)
	}
	if projects[0].GrafanaURL != "https://grafana-prj-1.apps.example.com/d/"+domain.ProjectGrafanaDashboardUID("prj-1")+"/project-metrics?orgId=1&refresh=30s" {
		t.Fatalf("expected hydrated grafana URL in list, got %q", projects[0].GrafanaURL)
	}
}

func TestBootstrapGitHubFlowRejectsUserWithEmptyBalance(t *testing.T) {
	t.Parallel()

	project := domain.Project{
		ID:        "prj-1",
		Name:      "demo",
		OwnerID:   "usr-1",
		Status:    domain.ProjectStatusActive,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	projectStore := newProjectStoreStub(project)
	users := newUserStoreStub(domain.User{ID: "usr-1", Email: "alice@example.com", BalanceRUB: 0})

	svc := NewProjectService(projectStore, newReleaseStoreStub(), newStageStoreStub(), &provisionerStub{}, &automationStub{}, &monetizationStub{}, users, &txStoreStub{}, &cryptoStub{}, "", "", "")

	_, err := svc.BootstrapGitHubFlow(context.Background(), project.ID, domain.BootstrapGitHubFlowRequest{
		RepositoryOwner: "example",
		RepositoryName:  "repo",
		ServiceType:     "LoadBalancer",
	})
	if err == nil || !strings.Contains(err.Error(), domain.ErrInsufficientBalance.Error()) {
		t.Fatalf("expected insufficient balance error, got %v", err)
	}
}

func TestBootstrapGitHubFlowAllowsExemptOwner(t *testing.T) {
	t.Parallel()

	project := domain.Project{
		ID:        "prj-1",
		Name:      "demo",
		OwnerID:   "usr-1",
		Status:    domain.ProjectStatusActive,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	projectStore := newProjectStoreStub(project)
	automation := &automationStub{
		bootstrapResp: domain.BootstrapGitHubFlowResponse{
			ProjectID:      project.ID,
			PullRequestURL: "https://github.com/example/repo/pull/1",
		},
	}
	users := newUserStoreStub(domain.User{ID: "usr-1", Email: "testuser@yandex.ru", BalanceRUB: 0})

	svc := NewProjectService(projectStore, newReleaseStoreStub(), newStageStoreStub(), &provisionerStub{}, automation, &monetizationStub{}, users, &txStoreStub{}, &cryptoStub{}, "", "", "")

	resp, err := svc.BootstrapGitHubFlow(context.Background(), project.ID, domain.BootstrapGitHubFlowRequest{
		RepositoryOwner: "example",
		RepositoryName:  "repo",
		ServiceType:     "LoadBalancer",
	})
	if err != nil {
		t.Fatalf("expected exempt user to deploy, got %v", err)
	}
	if resp.PullRequestURL == "" {
		t.Fatal("expected pull request URL")
	}
}

func TestBootstrapGitHubFlowReservesPendingBudgetForConcurrentRequests(t *testing.T) {
	t.Parallel()

	project := domain.Project{
		ID:        "prj-1",
		Name:      "demo",
		OwnerID:   "usr-1",
		Status:    domain.ProjectStatusActive,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	projectStore := newProjectStoreStub(project)
	bootstrapGate := make(chan struct{})
	bootstrapStarted := make(chan struct{})
	automation := &automationStub{
		bootstrapResp: domain.BootstrapGitHubFlowResponse{
			ProjectID:      project.ID,
			PullRequestURL: "https://github.com/example/repo/pull/1",
		},
		bootstrapBlock: bootstrapGate,
		onBootstrap: func() {
			select {
			case <-bootstrapStarted:
			default:
				close(bootstrapStarted)
			}
		},
	}
	// monetizationStub возвращает cost.Total=42 для каждого проекта, поэтому spentThisMonth=42.
	// usageReserve = min(3, 42*0.08) = 3.0; baseReserve = 0.45*1.15*1.0 = 0.5175
	// requiredReserve = 3.5175
	// Баланс должен удовлетворять: spentThisMonth + reserve <= balance < spentThisMonth + 2*reserve
	// т.е. 45.5175 <= balance < 49.035 -> берем 46.0
	users := newUserStoreStub(domain.User{ID: "usr-1", Email: "alice@example.com", BalanceRUB: 46.0})
	svc := NewProjectService(projectStore, newReleaseStoreStub(), newStageStoreStub(), &provisionerStub{}, automation, &monetizationStub{}, users, &txStoreStub{}, &cryptoStub{}, "", "", "")

	req := domain.BootstrapGitHubFlowRequest{
		RepositoryOwner: "example",
		RepositoryName:  "repo",
		ServiceType:     "ClusterIP",
	}

	var (
		wg       sync.WaitGroup
		firstErr error
	)
	firstStarted := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, firstErr = svc.BootstrapGitHubFlow(context.Background(), project.ID, req)
		// сигнализируем, если горутина завершилась до закрытия bootstrapStarted (ошибочный путь)
		select {
		case <-bootstrapStarted:
		default:
			close(firstStarted)
		}
	}()

	select {
	case <-bootstrapStarted:
	case <-firstStarted:
		wg.Wait()
		t.Fatalf("first BootstrapGitHubFlow failed before reaching automation: %v", firstErr)
	}

	_, secondErr := svc.BootstrapGitHubFlow(context.Background(), project.ID, req)
	if secondErr == nil || !strings.Contains(secondErr.Error(), domain.ErrInsufficientBalance.Error()) {
		t.Fatalf("expected second concurrent bootstrap to fail with insufficient balance, got %v", secondErr)
	}

	close(bootstrapGate)
	wg.Wait()

	if firstErr != nil {
		t.Fatalf("expected first bootstrap to succeed, got %v", firstErr)
	}
}

func TestGetProjectRuntimeStatusReturnsProvisionerState(t *testing.T) {
	t.Parallel()

	project := domain.Project{
		ID:        "prj-1",
		Name:      "demo",
		OwnerID:   "usr-1",
		Status:    domain.ProjectStatusActive,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	projectStore := newProjectStoreStub(project)
	provisioner := &provisionerStub{
		runtime: domain.ProjectRuntimeStatus{
			Namespace:         "project-prj-1",
			NamespaceExists:   true,
			DeploymentExists:  true,
			ServiceExists:     true,
			ReadyReplicas:     1,
			AvailableReplicas: 1,
		},
	}

	svc := NewProjectService(projectStore, newReleaseStoreStub(), newStageStoreStub(), provisioner, &automationStub{}, &monetizationStub{}, newUserStoreStub(), &txStoreStub{}, &cryptoStub{}, "", "", "")

	status, err := svc.GetProjectRuntimeStatus(context.Background(), project.ID)
	if err != nil {
		t.Fatalf("GetProjectRuntimeStatus returned error: %v", err)
	}
	if !status.DeploymentExists || status.ReadyReplicas != 1 {
		t.Fatalf("unexpected runtime status: %+v", status)
	}
}

func TestGetProjectRuntimeStatusBackfillsPublicURLFromRuntime(t *testing.T) {
	t.Parallel()

	project := domain.Project{
		ID:          "prj-1",
		Name:        "demo",
		OwnerID:     "usr-1",
		Status:      domain.ProjectStatusActive,
		ServiceType: "LoadBalancer",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	projectStore := newProjectStoreStub(project)
	provisioner := &provisionerStub{
		runtime: domain.ProjectRuntimeStatus{
			Namespace:         "project-prj-1",
			NamespaceExists:   true,
			DeploymentExists:  true,
			ServiceExists:     true,
			ReadyReplicas:     1,
			AvailableReplicas: 1,
			PublicURL:         "http://158.160.10.20",
		},
	}
	svc := NewProjectService(projectStore, newReleaseStoreStub(), newStageStoreStub(), provisioner, &automationStub{}, &monetizationStub{}, newUserStoreStub(), &txStoreStub{}, &cryptoStub{}, "", "", "")

	status, err := svc.GetProjectRuntimeStatus(context.Background(), project.ID)
	if err != nil {
		t.Fatalf("GetProjectRuntimeStatus returned error: %v", err)
	}
	if status.PublicURL != "http://158.160.10.20" {
		t.Fatalf("expected runtime public URL, got %q", status.PublicURL)
	}

	stored, ok := projectStore.GetByID(context.Background(), project.ID)
	if !ok {
		t.Fatalf("expected project %s to exist", project.ID)
	}
	if stored.PublicURL != "http://158.160.10.20" {
		t.Fatalf("expected project public URL to be backfilled, got %q", stored.PublicURL)
	}
}

func TestGetStageRuntimeStatusRestoresFailedStageWhenRuntimeReady(t *testing.T) {
	t.Parallel()

	project := domain.Project{
		ID:          "prj-1",
		Name:        "demo",
		OwnerID:     "usr-1",
		Status:      domain.ProjectStatusActive,
		ServiceType: "LoadBalancer",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}

	stage := domain.Stage{
		ID:        "stage-prj-1-production",
		ProjectID: project.ID,
		Name:      "Production",
		Slug:      "production",
		Status:    domain.StageStatusFailed,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	projectStore := newProjectStoreStub(project)
	stageStore := newStageStoreStub()
	if err := stageStore.Create(context.Background(), stage); err != nil {
		t.Fatalf("Create stage returned error: %v", err)
	}

	provisioner := &provisionerStub{
		stageRuntime: domain.ProjectRuntimeStatus{
			NamespaceExists:   true,
			DeploymentExists:  true,
			ServiceExists:     true,
			ReadyReplicas:     1,
			AvailableReplicas: 1,
			PublicURL:         "http://158.160.20.30",
		},
	}
	svc := NewProjectService(projectStore, newReleaseStoreStub(), stageStore, provisioner, &automationStub{}, &monetizationStub{}, newUserStoreStub(), &txStoreStub{}, &cryptoStub{}, "", "", "")

	status, err := svc.GetStageRuntimeStatus(context.Background(), project.ID, stage.ID, project.OwnerID)
	if err != nil {
		t.Fatalf("GetStageRuntimeStatus returned error: %v", err)
	}
	if status.ReadyReplicas != 1 {
		t.Fatalf("expected ready replicas from runtime status, got %+v", status)
	}

	updatedStage, ok := stageStore.GetByID(context.Background(), stage.ID)
	if !ok {
		t.Fatalf("expected stage %s to exist", stage.ID)
	}
	if updatedStage.Status != domain.StageStatusActive {
		t.Fatalf("expected stage status to be restored to active, got %s", updatedStage.Status)
	}
	if updatedStage.PublicURL != "http://158.160.20.30" {
		t.Fatalf("expected stage public URL to be backfilled, got %q", updatedStage.PublicURL)
	}

	updatedProject, ok := projectStore.GetByID(context.Background(), project.ID)
	if !ok {
		t.Fatalf("expected project %s to exist", project.ID)
	}
	if updatedProject.PublicURL != "http://158.160.20.30" {
		t.Fatalf("expected project public URL to be backfilled from stage runtime, got %q", updatedProject.PublicURL)
	}
}

func TestHandleGitHubWebhookCreatesReleaseOnFirstEvent(t *testing.T) {
	t.Parallel()

	project := domain.Project{
		ID:              "prj-1",
		Name:            "demo",
		OwnerID:         "usr-1",
		Status:          domain.ProjectStatusActive,
		RepositoryOwner: "myorg",
		RepositoryName:  "myrepo",
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}
	projectStore := newProjectStoreStub(project)
	releaseStore := newReleaseStoreStub()
	svc := NewProjectService(projectStore, releaseStore, newStageStoreStub(), &provisionerStub{}, &automationStub{}, &monetizationStub{}, newUserStoreStub(), &txStoreStub{}, &cryptoStub{}, "", "", "")

	payload := domain.GitHubWorkflowRunPayload{}
	payload.Action = "requested"
	payload.WorkflowRun.ID = 9999
	payload.WorkflowRun.Status = "queued"
	payload.WorkflowRun.HeadSHA = "abc123"
	payload.WorkflowRun.HeadCommit.Message = "fix: something"
	payload.Repository.Owner.Login = "myorg"
	payload.Repository.Name = "myrepo"

	if err := svc.HandleGitHubWebhook(context.Background(), payload); err != nil {
		t.Fatalf("HandleGitHubWebhook returned error: %v", err)
	}

	release, found := releaseStore.GetByWorkflowRunID(context.Background(), 9999)
	if !found {
		t.Fatal("expected a release to be created for workflowRunID=9999")
	}
	if release.ProjectID != project.ID {
		t.Fatalf("expected release to be associated with project %q, got %q", project.ID, release.ProjectID)
	}
	if release.CommitSHA != "abc123" {
		t.Fatalf("expected commitSHA %q, got %q", "abc123", release.CommitSHA)
	}
	if release.CommitMessage != "fix: something" {
		t.Fatalf("expected commitMessage %q, got %q", "fix: something", release.CommitMessage)
	}
	if release.StageID == "" {
		t.Fatal("expected release to be linked to a stage")
	}
	if release.Status != domain.ReleaseStatusPending {
		t.Fatalf("expected pending status (queued has no special handling), got %s", release.Status)
	}
}

func TestHandleGitHubWebhookTransitionsToBuilding(t *testing.T) {
	t.Parallel()

	project := domain.Project{
		ID:              "prj-1",
		Name:            "demo",
		OwnerID:         "usr-1",
		Status:          domain.ProjectStatusActive,
		RepositoryOwner: "myorg",
		RepositoryName:  "myrepo",
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}
	projectStore := newProjectStoreStub(project)
	releaseStore := newReleaseStoreStub()
	svc := NewProjectService(projectStore, releaseStore, newStageStoreStub(), &provisionerStub{}, &automationStub{}, &monetizationStub{}, newUserStoreStub(), &txStoreStub{}, &cryptoStub{}, "", "", "")

	payload := domain.GitHubWorkflowRunPayload{}
	payload.WorkflowRun.ID = 9999
	payload.WorkflowRun.Status = "in_progress"
	payload.WorkflowRun.HeadSHA = "abc123"
	payload.WorkflowRun.HeadCommit.Message = "fix: something"
	payload.Repository.Owner.Login = "myorg"
	payload.Repository.Name = "myrepo"

	if err := svc.HandleGitHubWebhook(context.Background(), payload); err != nil {
		t.Fatalf("HandleGitHubWebhook returned error: %v", err)
	}

	release, found := releaseStore.GetByWorkflowRunID(context.Background(), 9999)
	if !found {
		t.Fatal("expected a release to be created for workflowRunID=9999")
	}
	if release.Status != domain.ReleaseStatusBuilding {
		t.Fatalf("expected building status, got %s", release.Status)
	}
}

func TestHandleGitHubWebhookAttachesToPendingBootstrapRelease(t *testing.T) {
	t.Parallel()

	project := domain.Project{
		ID:              "prj-1",
		Name:            "demo",
		OwnerID:         "usr-1",
		Status:          domain.ProjectStatusActive,
		RepositoryOwner: "myorg",
		RepositoryName:  "myrepo",
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}
	projectStore := newProjectStoreStub(project)
	releaseStore := newReleaseStoreStub()
	stageStore := newStageStoreStub()
	automation := &automationStub{
		bootstrapResp: domain.BootstrapGitHubFlowResponse{
			ProjectID:       project.ID,
			RepositoryOwner: "myorg",
			RepositoryName:  "myrepo",
			BranchName:      "deploy-service/demo",
			PullRequestURL:  "https://github.com/myorg/myrepo/pull/42",
		},
	}
	users := newUserStoreStub(domain.User{ID: "usr-1", Email: "alice@example.com", BalanceRUB: 100})
	svc := NewProjectService(projectStore, releaseStore, stageStore, &provisionerStub{}, automation, &monetizationStub{}, users, &txStoreStub{}, &cryptoStub{}, "", "", "")

	_, err := svc.BootstrapGitHubFlow(context.Background(), project.ID, domain.BootstrapGitHubFlowRequest{
		RepositoryOwner: "myorg",
		RepositoryName:  "myrepo",
		BaseBranch:      "main",
		ServiceName:     "api",
		DockerfilePath:  "Dockerfile",
		GitHubToken:     "token",
	})
	if err != nil {
		t.Fatalf("BootstrapGitHubFlow returned error: %v", err)
	}

	initialReleases := releaseStore.ListByProject(context.Background(), project.ID)
	if len(initialReleases) != 1 {
		t.Fatalf("expected 1 pending release before webhook, got %d", len(initialReleases))
	}
	pendingID := initialReleases[0].ID

	payload := domain.GitHubWorkflowRunPayload{}
	payload.WorkflowRun.ID = 9999
	payload.WorkflowRun.Status = "in_progress"
	payload.WorkflowRun.HeadSHA = "abc123"
	payload.WorkflowRun.HeadCommit.Message = "feat: deploy"
	payload.Repository.Owner.Login = "myorg"
	payload.Repository.Name = "myrepo"

	if err := svc.HandleGitHubWebhook(context.Background(), payload); err != nil {
		t.Fatalf("HandleGitHubWebhook returned error: %v", err)
	}

	releases := releaseStore.ListByProject(context.Background(), project.ID)
	if len(releases) != 1 {
		t.Fatalf("expected webhook to reuse pending release, got %d records", len(releases))
	}
	release, found := releaseStore.GetByWorkflowRunID(context.Background(), 9999)
	if !found {
		t.Fatal("expected pending release to be linked to workflow run")
	}
	if release.ID != pendingID {
		t.Fatalf("expected webhook to update pending release %q, got %q", pendingID, release.ID)
	}
	if release.Status != domain.ReleaseStatusBuilding {
		t.Fatalf("expected building status, got %s", release.Status)
	}
	if release.CommitSHA != "abc123" {
		t.Fatalf("expected commit sha to be updated from webhook, got %q", release.CommitSHA)
	}
}

func TestHandleGitHubWebhookTransitionsToSuccess(t *testing.T) {
	t.Parallel()

	project := domain.Project{
		ID:              "prj-1",
		Name:            "demo",
		OwnerID:         "usr-1",
		Status:          domain.ProjectStatusActive,
		RepositoryOwner: "myorg",
		RepositoryName:  "myrepo",
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}
	projectStore := newProjectStoreStub(project)
	releaseStore := newReleaseStoreStub()
	svc := NewProjectService(projectStore, releaseStore, newStageStoreStub(), &provisionerStub{}, &automationStub{}, &monetizationStub{}, newUserStoreStub(), &txStoreStub{}, &cryptoStub{}, "", "", "")

	inProgress := domain.GitHubWorkflowRunPayload{}
	inProgress.WorkflowRun.ID = 9999
	inProgress.WorkflowRun.Status = "in_progress"
	inProgress.WorkflowRun.HeadSHA = "abc123"
	inProgress.Repository.Owner.Login = "myorg"
	inProgress.Repository.Name = "myrepo"

	if err := svc.HandleGitHubWebhook(context.Background(), inProgress); err != nil {
		t.Fatalf("HandleGitHubWebhook (in_progress) returned error: %v", err)
	}

	completed := domain.GitHubWorkflowRunPayload{}
	completed.WorkflowRun.ID = 9999
	completed.WorkflowRun.Status = "completed"
	completed.WorkflowRun.Conclusion = "success"
	completed.Repository.Owner.Login = "myorg"
	completed.Repository.Name = "myrepo"

	if err := svc.HandleGitHubWebhook(context.Background(), completed); err != nil {
		t.Fatalf("HandleGitHubWebhook (completed/success) returned error: %v", err)
	}

	release, found := releaseStore.GetByWorkflowRunID(context.Background(), 9999)
	if !found {
		t.Fatal("expected release to exist for workflowRunID=9999")
	}
	if release.Status != domain.ReleaseStatusSuccess {
		t.Fatalf("expected success status, got %s", release.Status)
	}
}

func TestListReleasesSyncsPendingReleaseFromGitHubActionsFallback(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	project := domain.Project{
		ID:              "prj-1",
		Name:            "demo",
		OwnerID:         "usr-1",
		Status:          domain.ProjectStatusActive,
		RepositoryOwner: "myorg",
		RepositoryName:  "myrepo",
		CreatedAt:       now.Add(-2 * time.Hour),
		UpdatedAt:       now.Add(-2 * time.Hour),
	}
	release := domain.Release{
		ID:            "release-pending-1",
		ProjectID:     project.ID,
		Status:        domain.ReleaseStatusPending,
		CommitMessage: "Ожидает запуска деплоя из PR: https://github.com/myorg/myrepo/pull/42",
		CreatedAt:     now.Add(-15 * time.Minute),
		UpdatedAt:     now.Add(-15 * time.Minute),
	}
	stageStore := newStageStoreStub()
	stage := domain.Stage{
		ID:        "stage-prj-1-production",
		ProjectID: project.ID,
		Name:      "Production",
		Slug:      "production",
		Status:    domain.StageStatusCreating,
		CreatedAt: now.Add(-2 * time.Hour),
		UpdatedAt: now.Add(-2 * time.Hour),
	}
	_ = stageStore.Create(context.Background(), stage)
	release.StageID = stage.ID

	automation := &automationStub{
		workflowFound: true,
		workflowRun: domain.GitHubWorkflowRunLookupResult{
			ID:         777,
			Status:     "completed",
			Conclusion: "success",
			HeadSHA:    "deadbeef",
			CreatedAt:  now.Add(-10 * time.Minute),
			UpdatedAt:  now.Add(-8 * time.Minute),
		},
	}
	users := newUserStoreStub(domain.User{
		ID:                   "usr-1",
		Email:                "alice@example.com",
		GitHubTokenEncrypted: "enc:test-token",
	})
	svc := NewProjectService(
		newProjectStoreStub(project),
		newReleaseStoreStub(release),
		stageStore,
		&provisionerStub{},
		automation,
		&monetizationStub{},
		users,
		&txStoreStub{},
		&cryptoStub{},
		"",
		"",
		"",
	)

	releases, err := svc.ListReleases(context.Background(), project.ID)
	if err != nil {
		t.Fatalf("ListReleases returned error: %v", err)
	}
	if len(releases) != 1 {
		t.Fatalf("expected 1 release, got %d", len(releases))
	}
	if releases[0].Status != domain.ReleaseStatusSuccess {
		t.Fatalf("expected synced release status success, got %s", releases[0].Status)
	}
	if releases[0].WorkflowRunID != 777 {
		t.Fatalf("expected workflow run id 777, got %d", releases[0].WorkflowRunID)
	}
	if releases[0].CommitSHA != "deadbeef" {
		t.Fatalf("expected commit sha deadbeef, got %q", releases[0].CommitSHA)
	}

	updatedStage, exists := stageStore.GetByID(context.Background(), stage.ID)
	if !exists {
		t.Fatal("expected stage to remain in store")
	}
	if updatedStage.Status != domain.StageStatusActive {
		t.Fatalf("expected stage status active after synced success, got %s", updatedStage.Status)
	}
}

func TestHandleGitHubWebhookRestoresFailedStageOnSuccessfulRelease(t *testing.T) {
	t.Parallel()

	project := domain.Project{
		ID:              "prj-1",
		Name:            "demo",
		OwnerID:         "usr-1",
		Status:          domain.ProjectStatusActive,
		RepositoryOwner: "myorg",
		RepositoryName:  "myrepo",
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}
	projectStore := newProjectStoreStub(project)
	releaseStore := newReleaseStoreStub()
	stageStore := newStageStoreStub()
	failedStage := domain.Stage{
		ID:        "stage-prj-1-production",
		ProjectID: project.ID,
		Name:      "Production",
		Slug:      "production",
		Status:    domain.StageStatusFailed,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := stageStore.Create(context.Background(), failedStage); err != nil {
		t.Fatalf("Create stage returned error: %v", err)
	}

	svc := NewProjectService(projectStore, releaseStore, stageStore, &provisionerStub{}, &automationStub{}, &monetizationStub{}, newUserStoreStub(), &txStoreStub{}, &cryptoStub{}, "", "", "")

	inProgress := domain.GitHubWorkflowRunPayload{}
	inProgress.WorkflowRun.ID = 9999
	inProgress.WorkflowRun.Status = "in_progress"
	inProgress.WorkflowRun.HeadSHA = "abc123"
	inProgress.Repository.Owner.Login = "myorg"
	inProgress.Repository.Name = "myrepo"

	if err := svc.HandleGitHubWebhook(context.Background(), inProgress); err != nil {
		t.Fatalf("HandleGitHubWebhook (in_progress) returned error: %v", err)
	}

	completed := domain.GitHubWorkflowRunPayload{}
	completed.WorkflowRun.ID = 9999
	completed.WorkflowRun.Status = "completed"
	completed.WorkflowRun.Conclusion = "success"
	completed.Repository.Owner.Login = "myorg"
	completed.Repository.Name = "myrepo"

	if err := svc.HandleGitHubWebhook(context.Background(), completed); err != nil {
		t.Fatalf("HandleGitHubWebhook (completed/success) returned error: %v", err)
	}

	updatedStage, ok := stageStore.GetByID(context.Background(), failedStage.ID)
	if !ok {
		t.Fatalf("expected stage %s to exist", failedStage.ID)
	}
	if updatedStage.Status != domain.StageStatusActive {
		t.Fatalf("expected stage status to become active after successful release, got %s", updatedStage.Status)
	}
}

func TestHandleGitHubWebhookTransitionsToFailed(t *testing.T) {
	t.Parallel()

	project := domain.Project{
		ID:              "prj-1",
		Name:            "demo",
		OwnerID:         "usr-1",
		Status:          domain.ProjectStatusActive,
		RepositoryOwner: "myorg",
		RepositoryName:  "myrepo",
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}
	projectStore := newProjectStoreStub(project)
	releaseStore := newReleaseStoreStub()
	svc := NewProjectService(projectStore, releaseStore, newStageStoreStub(), &provisionerStub{}, &automationStub{}, &monetizationStub{}, newUserStoreStub(), &txStoreStub{}, &cryptoStub{}, "", "", "")

	inProgress := domain.GitHubWorkflowRunPayload{}
	inProgress.WorkflowRun.ID = 9999
	inProgress.WorkflowRun.Status = "in_progress"
	inProgress.WorkflowRun.HeadSHA = "abc123"
	inProgress.Repository.Owner.Login = "myorg"
	inProgress.Repository.Name = "myrepo"

	if err := svc.HandleGitHubWebhook(context.Background(), inProgress); err != nil {
		t.Fatalf("HandleGitHubWebhook (in_progress) returned error: %v", err)
	}

	completed := domain.GitHubWorkflowRunPayload{}
	completed.WorkflowRun.ID = 9999
	completed.WorkflowRun.Status = "completed"
	completed.WorkflowRun.Conclusion = "failure"
	completed.Repository.Owner.Login = "myorg"
	completed.Repository.Name = "myrepo"

	if err := svc.HandleGitHubWebhook(context.Background(), completed); err != nil {
		t.Fatalf("HandleGitHubWebhook (completed/failure) returned error: %v", err)
	}

	release, found := releaseStore.GetByWorkflowRunID(context.Background(), 9999)
	if !found {
		t.Fatal("expected release to exist for workflowRunID=9999")
	}
	if release.Status != domain.ReleaseStatusFailed {
		t.Fatalf("expected failed status, got %s", release.Status)
	}
}

func TestHandleGitHubWebhookIgnoresUnknownRepo(t *testing.T) {
	t.Parallel()

	project := domain.Project{
		ID:              "prj-1",
		Name:            "demo",
		OwnerID:         "usr-1",
		Status:          domain.ProjectStatusActive,
		RepositoryOwner: "myorg",
		RepositoryName:  "myrepo",
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}
	projectStore := newProjectStoreStub(project)
	releaseStore := newReleaseStoreStub()
	svc := NewProjectService(projectStore, releaseStore, newStageStoreStub(), &provisionerStub{}, &automationStub{}, &monetizationStub{}, newUserStoreStub(), &txStoreStub{}, &cryptoStub{}, "", "", "")

	payload := domain.GitHubWorkflowRunPayload{}
	payload.WorkflowRun.ID = 7777
	payload.WorkflowRun.Status = "in_progress"
	payload.Repository.Owner.Login = "unknownorg"
	payload.Repository.Name = "unknownrepo"

	if err := svc.HandleGitHubWebhook(context.Background(), payload); err != nil {
		t.Fatalf("HandleGitHubWebhook returned unexpected error: %v", err)
	}

	releases := releaseStore.ListByProject(context.Background(), project.ID)
	if len(releases) != 0 {
		t.Fatalf("expected no releases to be created, got %d", len(releases))
	}
}

func TestHandleGitHubWebhookMatchesRepositoryCaseInsensitively(t *testing.T) {
	t.Parallel()

	project := domain.Project{
		ID:              "prj-1",
		Name:            "demo",
		OwnerID:         "usr-1",
		Status:          domain.ProjectStatusActive,
		RepositoryOwner: "tasher239",
		RepositoryName:  "soa",
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}
	projectStore := newProjectStoreStub(project)
	releaseStore := newReleaseStoreStub()
	svc := NewProjectService(projectStore, releaseStore, newStageStoreStub(), &provisionerStub{}, &automationStub{}, &monetizationStub{}, newUserStoreStub(), &txStoreStub{}, &cryptoStub{}, "", "", "")

	payload := domain.GitHubWorkflowRunPayload{}
	payload.WorkflowRun.ID = 8801
	payload.WorkflowRun.Status = "completed"
	payload.WorkflowRun.Conclusion = "success"
	payload.WorkflowRun.HeadSHA = "abc123"
	payload.Repository.Owner.Login = "Tasher239"
	payload.Repository.Name = "SOA"

	if err := svc.HandleGitHubWebhook(context.Background(), payload); err != nil {
		t.Fatalf("HandleGitHubWebhook returned error: %v", err)
	}

	release, found := releaseStore.GetByWorkflowRunID(context.Background(), 8801)
	if !found {
		t.Fatal("expected release to be created for case-insensitive repository match")
	}
	if release.ProjectID != "prj-1" {
		t.Fatalf("expected release to be linked to prj-1, got %q", release.ProjectID)
	}
}

func TestHandleGitHubWebhookSkipsDeletedProjects(t *testing.T) {
	t.Parallel()

	deleted := domain.Project{
		ID:              "prj-deleted",
		Name:            "demo",
		OwnerID:         "usr-1",
		Status:          domain.ProjectStatusDeleted,
		RepositoryOwner: "acme",
		RepositoryName:  "app",
		CreatedAt:       time.Now().UTC().Add(-time.Hour),
		UpdatedAt:       time.Now().UTC(),
	}
	active := domain.Project{
		ID:              "prj-active",
		Name:            "demo",
		OwnerID:         "usr-1",
		Status:          domain.ProjectStatusActive,
		RepositoryOwner: "acme",
		RepositoryName:  "app",
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}
	projectStore := newProjectStoreStub(deleted, active)
	releaseStore := newReleaseStoreStub()
	svc := NewProjectService(projectStore, releaseStore, newStageStoreStub(), &provisionerStub{}, &automationStub{}, &monetizationStub{}, newUserStoreStub(), &txStoreStub{}, &cryptoStub{}, "", "", "")

	payload := domain.GitHubWorkflowRunPayload{}
	payload.WorkflowRun.ID = 9901
	payload.WorkflowRun.Status = "in_progress"
	payload.WorkflowRun.HeadSHA = "deadbeef"
	payload.Repository.Owner.Login = "acme"
	payload.Repository.Name = "app"

	if err := svc.HandleGitHubWebhook(context.Background(), payload); err != nil {
		t.Fatalf("HandleGitHubWebhook returned error: %v", err)
	}

	release, found := releaseStore.GetByWorkflowRunID(context.Background(), 9901)
	if !found {
		t.Fatal("expected release to be created")
	}
	if release.ProjectID != "prj-active" {
		t.Fatalf("expected release on prj-active, got %q", release.ProjectID)
	}
}

func TestEnforceBillingGuardStartsGracePeriodBeforeAutoSuspend(t *testing.T) {
	t.Parallel()

	activeProject := domain.Project{
		ID:        "prj-active",
		Name:      "active",
		OwnerID:   "usr-1",
		Status:    domain.ProjectStatusActive,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	alreadySuspended := domain.Project{
		ID:        "prj-suspended",
		Name:      "suspended",
		OwnerID:   "usr-1",
		Status:    domain.ProjectStatusSuspended,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	foreignProject := domain.Project{
		ID:        "prj-foreign",
		Name:      "foreign",
		OwnerID:   "usr-2",
		Status:    domain.ProjectStatusActive,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	projectStore := newProjectStoreStub(activeProject, alreadySuspended, foreignProject)
	provisioner := &provisionerStub{}
	users := newUserStoreStub(domain.User{ID: "usr-1", Email: "alice@example.com", BalanceRUB: 0})

	svc := NewProjectService(projectStore, newReleaseStoreStub(), newStageStoreStub(), provisioner, &automationStub{}, &monetizationStub{}, users, &txStoreStub{}, &cryptoStub{}, "", "", "")
	svc.SetBillingGuardGracePeriod(24 * time.Hour)

	if err := svc.EnforceBillingGuard(context.Background(), "usr-1"); err != nil {
		t.Fatalf("EnforceBillingGuard returned error: %v", err)
	}

	updatedActive, _ := projectStore.GetByID(context.Background(), "prj-active")
	if updatedActive.Status != domain.ProjectStatusActive {
		t.Fatalf("expected active project to stay active during grace period, got %s", updatedActive.Status)
	}
	if len(provisioner.suspended) != 0 {
		t.Fatalf("expected no suspend calls during grace period, got %#v", provisioner.suspended)
	}
	updatedForeign, _ := projectStore.GetByID(context.Background(), "prj-foreign")
	if updatedForeign.Status != domain.ProjectStatusActive {
		t.Fatalf("expected foreign project to stay active, got %s", updatedForeign.Status)
	}

	summary, err := svc.GetBillingSummary(context.Background(), "usr-1")
	if err != nil {
		t.Fatalf("GetBillingSummary returned error: %v", err)
	}
	if summary.GracePeriodEndsAt == nil || summary.GracePeriodRemainingSeconds <= 0 {
		t.Fatalf("expected grace period metadata in summary, got %+v", summary)
	}
}

func TestEnforceBillingGuardSuspendsAfterGracePeriodExpires(t *testing.T) {
	t.Parallel()

	activeProject := domain.Project{
		ID:        "prj-active",
		Name:      "active",
		OwnerID:   "usr-1",
		Status:    domain.ProjectStatusActive,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	projectStore := newProjectStoreStub(activeProject)
	provisioner := &provisionerStub{}
	users := newUserStoreStub(domain.User{ID: "usr-1", Email: "alice@example.com", BalanceRUB: 0})

	svc := NewProjectService(projectStore, newReleaseStoreStub(), newStageStoreStub(), provisioner, &automationStub{}, &monetizationStub{}, users, &txStoreStub{}, &cryptoStub{}, "", "", "")
	svc.SetBillingGuardGracePeriod(30 * time.Minute)

	svc.billingGuardMu.Lock()
	svc.graceStartedAt["usr-1"] = time.Now().UTC().Add(-2 * time.Hour)
	svc.billingGuardMu.Unlock()

	if err := svc.EnforceBillingGuard(context.Background(), "usr-1"); err != nil {
		t.Fatalf("EnforceBillingGuard returned error: %v", err)
	}

	updatedActive, _ := projectStore.GetByID(context.Background(), "prj-active")
	if updatedActive.Status != domain.ProjectStatusSuspended {
		t.Fatalf("expected active project to become suspended after grace period, got %s", updatedActive.Status)
	}
	if updatedActive.SuspendedAt == nil || updatedActive.DeletionDueAt == nil {
		t.Fatalf("expected retention timestamps to be set on auto-suspend, got %+v", updatedActive)
	}
	if len(provisioner.suspended) != 1 || provisioner.suspended[0] != "prj-active" {
		t.Fatalf("expected active project to be suspended once, got %#v", provisioner.suspended)
	}
}

func TestEnforceBillingGuardClearsDeletionScheduleAfterTopUp(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	deletionDueAt := now.Add(6 * 24 * time.Hour)
	suspendedAt := now.Add(-24 * time.Hour)
	suspendedProject := domain.Project{
		ID:            "prj-suspended",
		Name:          "suspended",
		OwnerID:       "usr-1",
		Status:        domain.ProjectStatusSuspended,
		SuspendedAt:   &suspendedAt,
		DeletionDueAt: &deletionDueAt,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	projectStore := newProjectStoreStub(suspendedProject)
	users := newUserStoreStub(domain.User{ID: "usr-1", Email: "alice@example.com", BalanceRUB: 100})

	svc := NewProjectService(projectStore, newReleaseStoreStub(), newStageStoreStub(), &provisionerStub{}, &automationStub{}, &monetizationStub{}, users, &txStoreStub{}, &cryptoStub{}, "", "", "")

	if err := svc.EnforceBillingGuard(context.Background(), "usr-1"); err != nil {
		t.Fatalf("EnforceBillingGuard returned error: %v", err)
	}

	updated, _ := projectStore.GetByID(context.Background(), "prj-suspended")
	if updated.DeletionDueAt != nil || updated.SuspendedAt != nil {
		t.Fatalf("expected deletion schedule to be cleared after top up, got %+v", updated)
	}
}

func TestEnforceBillingGuardDeletesSuspendedProjectAfterRetentionExpires(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	suspendedAt := now.Add(-10 * 24 * time.Hour)
	deletionDueAt := now.Add(-2 * time.Hour)
	suspendedProject := domain.Project{
		ID:            "prj-suspended",
		Name:          "suspended",
		OwnerID:       "usr-1",
		Status:        domain.ProjectStatusSuspended,
		SuspendedAt:   &suspendedAt,
		DeletionDueAt: &deletionDueAt,
		CreatedAt:     now.Add(-20 * 24 * time.Hour),
		UpdatedAt:     now.Add(-2 * time.Hour),
	}
	projectStore := newProjectStoreStub(suspendedProject)
	provisioner := &provisionerStub{}
	users := newUserStoreStub(domain.User{ID: "usr-1", Email: "alice@example.com", BalanceRUB: 0})

	svc := NewProjectService(projectStore, newReleaseStoreStub(), newStageStoreStub(), provisioner, &automationStub{}, &monetizationStub{}, users, &txStoreStub{}, &cryptoStub{}, "", "", "")
	svc.SetBillingGuardGracePeriod(30 * time.Minute)

	svc.billingGuardMu.Lock()
	svc.graceStartedAt["usr-1"] = now.Add(-48 * time.Hour)
	svc.billingGuardMu.Unlock()

	if err := svc.EnforceBillingGuard(context.Background(), "usr-1"); err != nil {
		t.Fatalf("EnforceBillingGuard returned error: %v", err)
	}

	updated, _ := projectStore.GetByID(context.Background(), "prj-suspended")
	if updated.Status != domain.ProjectStatusDeleted {
		t.Fatalf("expected suspended project to be deleted after retention period, got %s", updated.Status)
	}
	if len(provisioner.deleted) != 1 || provisioner.deleted[0] != "prj-suspended" {
		t.Fatalf("expected delete to be called once, got %#v", provisioner.deleted)
	}
}

func TestServiceGitHubTokenLifecycle(t *testing.T) {
	t.Parallel()

	userStore := newUserStoreStub(domain.User{ID: "usr-1", Email: "alice@example.com", BalanceRUB: 100})
	svc := NewProjectService(
		newProjectStoreStub(),
		newReleaseStoreStub(),
		newStageStoreStub(),
		&provisionerStub{},
		&automationStub{},
		&monetizationStub{},
		userStore,
		&txStoreStub{},
		&cryptoStub{},
		"",
		"",
		"",
	)

	status, err := svc.GetServiceGitHubTokenStatus(context.Background(), "usr-1")
	if err != nil {
		t.Fatalf("GetServiceGitHubTokenStatus returned error: %v", err)
	}
	if status.Configured {
		t.Fatal("expected token to be not configured by default")
	}

	status, err = svc.UpsertServiceGitHubToken(context.Background(), "usr-1", "ghp_test_token")
	if err != nil {
		t.Fatalf("UpsertServiceGitHubToken returned error: %v", err)
	}
	if !status.Configured {
		t.Fatal("expected token to be configured after upsert")
	}

	user, ok := userStore.GetByID(context.Background(), "usr-1")
	if !ok {
		t.Fatal("expected user to exist")
	}
	if user.GitHubTokenEncrypted == "" {
		t.Fatal("expected encrypted github token to be stored")
	}

	if err := svc.DeleteServiceGitHubToken(context.Background(), "usr-1"); err != nil {
		t.Fatalf("DeleteServiceGitHubToken returned error: %v", err)
	}
	status, err = svc.GetServiceGitHubTokenStatus(context.Background(), "usr-1")
	if err != nil {
		t.Fatalf("GetServiceGitHubTokenStatus returned error: %v", err)
	}
	if status.Configured {
		t.Fatal("expected token to be removed after delete")
	}
}

func TestBootstrapGitHubFlowUsesStoredServiceTokenWhenRequestTokenIsEmpty(t *testing.T) {
	t.Parallel()

	project := domain.Project{
		ID:        "prj-1",
		Name:      "demo",
		OwnerID:   "usr-1",
		Status:    domain.ProjectStatusActive,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	projectStore := newProjectStoreStub(project)
	automation := &automationStub{
		bootstrapResp: domain.BootstrapGitHubFlowResponse{
			ProjectID:      project.ID,
			PullRequestURL: "https://github.com/example/repo/pull/1",
		},
	}
	users := newUserStoreStub(domain.User{
		ID:                   "usr-1",
		Email:                "alice@example.com",
		BalanceRUB:           100,
		GitHubTokenEncrypted: "enc:ghp_stored_token",
	})
	svc := NewProjectService(projectStore, newReleaseStoreStub(), newStageStoreStub(), &provisionerStub{}, automation, &monetizationStub{}, users, &txStoreStub{}, &cryptoStub{}, "", "", "")

	_, err := svc.BootstrapGitHubFlow(context.Background(), project.ID, domain.BootstrapGitHubFlowRequest{
		RepositoryOwner: "example",
		RepositoryName:  "repo",
	})
	if err != nil {
		t.Fatalf("BootstrapGitHubFlow returned error: %v", err)
	}
	if automation.lastBootstrap.GitHubToken != "ghp_stored_token" {
		t.Fatalf("expected stored token to be used, got %q", automation.lastBootstrap.GitHubToken)
	}
}
