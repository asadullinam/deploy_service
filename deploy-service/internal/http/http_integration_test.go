//go:build integration

package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	stdhttp "net/http"
	"net/http/httptest"
	"testing"
	"time"

	cryptosvc "deploy-service/internal/crypto"
	"deploy-service/internal/domain"
	githubintegration "deploy-service/internal/integration/github"
	"deploy-service/internal/monetization"
	"deploy-service/internal/service"
	"deploy-service/internal/store/memory"
	"deploy-service/internal/testsupport"
)

func TestEndToEndBootstrapFlowUsesTemporaryRepositoryAndCleansUpThroughTempDir(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo, err := testsupport.NewTempGitHubRepo(
		ctx,
		t.TempDir(),
		"test-owner",
		"example-service",
		"main",
		map[string]string{
			"Dockerfile": "FROM golang:1.22\nEXPOSE 9090\nCMD [\"/app\"]\n",
			"go.mod":     "module example-service\n\ngo 1.22\n",
		},
	)
	if err != nil {
		t.Fatalf("NewTempGitHubRepo returned error: %v", err)
	}

	fakeGitHub := httptest.NewServer(repo)
	defer fakeGitHub.Close()

	projectStore := memory.NewProjectStore()
	releaseStore := memory.NewReleaseStore()
	userStore := memory.NewUserStore()
	provisioner := testsupport.NewRuntimeProvisioner()
	cryptoService, err := cryptosvc.NewService(bytes.Repeat([]byte{1}, 32))
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}
	projectService := service.NewProjectService(
		projectStore,
		releaseStore,
		provisioner,
		githubintegration.NewGitHubAutomation(fakeGitHub.URL, "ghp_server_token", "apps.example.test", "https://platform.example.test", "hook-secret"),
		monetization.NewEngineMock(),
		userStore,
		cryptoService,
		"apps.example.test",
		"http",
		"32080",
	)
	authService := service.NewAuthService(userStore, "test-secret", time.Hour, 0)

	handler := NewHandler(projectService, authService, "hook-secret")
	server := httptest.NewServer(NewRouter(handler, "test-secret"))
	defer server.Close()

	token := registerUserForE2E(t, server.URL, "alice@example.com", "passw0rd123")

	project := createProjectForE2E(t, server.URL, token, "demo")

	questions := requestGitHubQuestionsForE2E(t, server.URL, token, project.ID, domain.GitHubBootstrapQuestionsRequest{
		RepositoryOwner: repo.Owner,
		RepositoryName:  repo.Name,
		GitHubToken:     "ghp_request_token",
	})
	if questions.DetectedContainerPort != 9090 {
		t.Fatalf("expected detected container port 9090, got %d", questions.DetectedContainerPort)
	}
	if questions.DetectedServiceName != "example-service" {
		t.Fatalf("expected detected service name example-service, got %q", questions.DetectedServiceName)
	}

	var insufficient map[string]string
	doJSONRequestForE2E(t, stdhttp.MethodPost, server.URL+"/projects/"+project.ID+"/github/bootstrap", token, domain.BootstrapGitHubFlowRequest{
		RepositoryOwner: repo.Owner,
		RepositoryName:  repo.Name,
		ServiceName:     "example-service",
		DockerfilePath:  "Dockerfile",
		ServiceType:     "LoadBalancer",
		ServicePort:     80,
		ContainerPort:   9090,
		GitHubToken:     "ghp_request_token",
	}, stdhttp.StatusPaymentRequired, &insufficient)
	if insufficient["error"] == "" {
		t.Fatal("expected payment required error body")
	}

	summary := topUpForE2E(t, server.URL, token, 1000)
	if summary.BalanceRUB != 1000 {
		t.Fatalf("expected balance 1000 after top up, got %.2f", summary.BalanceRUB)
	}
	if summary.AvailableRUB <= 0 {
		t.Fatalf("expected positive available balance, got %.2f", summary.AvailableRUB)
	}

	settings := updateDeploymentSettingsForE2E(t, server.URL, token, project.ID, domain.UpdateDeploymentSettingsRequest{
		RepositoryOwner: repo.Owner,
		RepositoryName:  repo.Name,
		BaseBranch:      "main",
		ServiceName:     "example-service",
		DockerfilePath:  "Dockerfile",
		ServiceType:     "LoadBalancer",
		ServicePort:     80,
		ContainerPort:   9090,
	})
	if settings.RepositoryOwner != repo.Owner || settings.RepositoryName != repo.Name {
		t.Fatalf("expected deployment settings to persist repo identity, got %#v", settings)
	}

	bootstrapResponse := bootstrapRepoForE2E(t, server.URL, token, project.ID, domain.BootstrapGitHubFlowRequest{
		RepositoryOwner: repo.Owner,
		RepositoryName:  repo.Name,
		BaseBranch:      "main",
		ServiceName:     "example-service",
		DockerfilePath:  "Dockerfile",
		ServiceType:     "LoadBalancer",
		ServicePort:     80,
		ContainerPort:   9090,
		GitHubToken:     "ghp_request_token",
	})
	if bootstrapResponse.PullRequestURL == "" {
		t.Fatal("expected pull request url")
	}

	if err := testsupport.SimulateGeneratedDeploy(ctx, repo, provisioner, bootstrapResponse.BranchName, project.ID, "example-service"); err != nil {
		t.Fatalf("SimulateGeneratedDeploy returned error: %v", err)
	}

	runtimeStatus := getRuntimeStatusForE2E(t, server.URL, token, project.ID)
	if !runtimeStatus.NamespaceExists || !runtimeStatus.DeploymentExists || !runtimeStatus.ServiceExists {
		t.Fatalf("expected runtime status to report deployed resources, got %#v", runtimeStatus)
	}
	if runtimeStatus.ReadyReplicas != 1 {
		t.Fatalf("expected one ready replica, got %d", runtimeStatus.ReadyReplicas)
	}
	if len(runtimeStatus.Pods) != 1 || !runtimeStatus.Pods[0].Ready {
		t.Fatalf("expected one ready pod, got %#v", runtimeStatus.Pods)
	}

	stages := listStagesForE2E(t, server.URL, token, project.ID)
	if len(stages) == 0 {
		t.Fatal("expected at least one stage after bootstrap")
	}
	stageRuntime := getStageRuntimeStatusForE2E(t, server.URL, token, project.ID, stages[0].ID)
	if !stageRuntime.NamespaceExists || !stageRuntime.DeploymentExists || !stageRuntime.ServiceExists {
		t.Fatalf("expected stage runtime to report deployed resources, got %#v", stageRuntime)
	}
	if len(stageRuntime.Pods) == 0 || !stageRuntime.Pods[0].Ready {
		t.Fatalf("expected at least one ready stage pod, got %#v", stageRuntime.Pods)
	}

	storedProject := getProjectForE2E(t, server.URL, token, project.ID)
	if storedProject.ServiceName != "example-service" {
		t.Fatalf("expected stored project service name to survive flow, got %q", storedProject.ServiceName)
	}
	if storedProject.RepositoryOwner != repo.Owner || storedProject.RepositoryName != repo.Name {
		t.Fatalf("expected stored project repo settings, got %#v", storedProject)
	}
}

func TestBillingSummaryAutoSuspendsProjectsWhenBalanceIsExhausted(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo, err := testsupport.NewTempGitHubRepo(
		ctx,
		t.TempDir(),
		"test-owner",
		"example-service",
		"main",
		map[string]string{
			"Dockerfile": "FROM golang:1.22\nEXPOSE 9090\nCMD [\"/app\"]\n",
		},
	)
	if err != nil {
		t.Fatalf("NewTempGitHubRepo returned error: %v", err)
	}

	fakeGitHub := httptest.NewServer(repo)
	defer fakeGitHub.Close()

	projectStore := memory.NewProjectStore()
	releaseStore := memory.NewReleaseStore()
	userStore := memory.NewUserStore()
	provisioner := testsupport.NewRuntimeProvisioner()
	cryptoService, err := cryptosvc.NewService(bytes.Repeat([]byte{2}, 32))
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}
	projectService := service.NewProjectService(
		projectStore,
		releaseStore,
		provisioner,
		githubintegration.NewGitHubAutomation(fakeGitHub.URL, "ghp_server_token", "apps.example.test", "https://platform.example.test", "hook-secret"),
		monetization.NewEngineMock(),
		userStore,
		cryptoService,
		"apps.example.test",
		"http",
		"32080",
	)
	authService := service.NewAuthService(userStore, "test-secret", time.Hour, 0)

	handler := NewHandler(projectService, authService, "hook-secret")
	server := httptest.NewServer(NewRouter(handler, "test-secret"))
	defer server.Close()

	token := registerUserForE2E(t, server.URL, "bob@example.com", "passw0rd123")
	project := createProjectForE2E(t, server.URL, token, "budget-demo")

	summary := getBillingSummaryForE2E(t, server.URL, token)
	if summary.AvailableRUB >= 0 {
		t.Fatalf("expected negative available balance with zero funds and mock usage, got %+v", summary)
	}

	updatedProject := getProjectForE2E(t, server.URL, token, project.ID)
	if updatedProject.Status != domain.ProjectStatusSuspended {
		t.Fatalf("expected project to be auto-suspended, got %s", updatedProject.Status)
	}

	runtimeStatus := getRuntimeStatusForE2E(t, server.URL, token, project.ID)
	if runtimeStatus.Message != "project is suspended" {
		t.Fatalf("expected suspended runtime message, got %+v", runtimeStatus)
	}
}

func registerUserForE2E(t *testing.T, baseURL string, email string, password string) string {
	t.Helper()

	var response domain.TokenResponse
	doJSONRequestForE2E(t, stdhttp.MethodPost, baseURL+"/auth/register", "", domain.RegisterRequest{
		Email:    email,
		Password: password,
	}, stdhttp.StatusCreated, &response)
	if response.Token == "" {
		t.Fatal("expected jwt token after registration")
	}
	return response.Token
}

func createProjectForE2E(t *testing.T, baseURL string, token string, name string) domain.Project {
	t.Helper()

	var response domain.Project
	doJSONRequestForE2E(t, stdhttp.MethodPost, baseURL+"/projects", token, domain.CreateProjectRequest{
		Name: name,
	}, stdhttp.StatusCreated, &response)
	if response.ID == "" {
		t.Fatal("expected project id")
	}
	return response
}

func requestGitHubQuestionsForE2E(t *testing.T, baseURL string, token string, projectID string, payload domain.GitHubBootstrapQuestionsRequest) domain.GitHubBootstrapQuestionsResponse {
	t.Helper()

	var response domain.GitHubBootstrapQuestionsResponse
	doJSONRequestForE2E(t, stdhttp.MethodPost, fmt.Sprintf("%s/projects/%s/github/questions", baseURL, projectID), token, payload, stdhttp.StatusOK, &response)
	return response
}

func topUpForE2E(t *testing.T, baseURL string, token string, amount float64) domain.BillingSummary {
	t.Helper()

	var response domain.BillingSummary
	doJSONRequestForE2E(t, stdhttp.MethodPost, baseURL+"/billing/top-up", token, domain.TopUpBalanceRequest{
		AmountRUB: amount,
	}, stdhttp.StatusOK, &response)
	return response
}

func getBillingSummaryForE2E(t *testing.T, baseURL string, token string) domain.BillingSummary {
	t.Helper()

	var response domain.BillingSummary
	doJSONRequestForE2E(t, stdhttp.MethodGet, baseURL+"/billing/summary", token, nil, stdhttp.StatusOK, &response)
	return response
}

func updateDeploymentSettingsForE2E(t *testing.T, baseURL string, token string, projectID string, payload domain.UpdateDeploymentSettingsRequest) domain.Project {
	t.Helper()

	var response domain.Project
	doJSONRequestForE2E(t, stdhttp.MethodPut, fmt.Sprintf("%s/projects/%s/deployment-settings", baseURL, projectID), token, payload, stdhttp.StatusOK, &response)
	return response
}

func bootstrapRepoForE2E(t *testing.T, baseURL string, token string, projectID string, payload domain.BootstrapGitHubFlowRequest) domain.BootstrapGitHubFlowResponse {
	t.Helper()

	var response domain.BootstrapGitHubFlowResponse
	doJSONRequestForE2E(t, stdhttp.MethodPost, fmt.Sprintf("%s/projects/%s/github/bootstrap", baseURL, projectID), token, payload, stdhttp.StatusOK, &response)
	return response
}

func getRuntimeStatusForE2E(t *testing.T, baseURL string, token string, projectID string) domain.ProjectRuntimeStatus {
	t.Helper()

	var response domain.ProjectRuntimeStatus
	doJSONRequestForE2E(t, stdhttp.MethodGet, fmt.Sprintf("%s/projects/%s/runtime-status", baseURL, projectID), token, nil, stdhttp.StatusOK, &response)
	return response
}

func listStagesForE2E(t *testing.T, baseURL string, token string, projectID string) []domain.Stage {
	t.Helper()

	var response []domain.Stage
	doJSONRequestForE2E(t, stdhttp.MethodGet, fmt.Sprintf("%s/projects/%s/stages", baseURL, projectID), token, nil, stdhttp.StatusOK, &response)
	return response
}

func getStageRuntimeStatusForE2E(t *testing.T, baseURL string, token string, projectID string, stageID string) domain.ProjectRuntimeStatus {
	t.Helper()

	var response domain.ProjectRuntimeStatus
	doJSONRequestForE2E(t, stdhttp.MethodGet, fmt.Sprintf("%s/projects/%s/stages/%s/runtime-status", baseURL, projectID, stageID), token, nil, stdhttp.StatusOK, &response)
	return response
}

func getProjectForE2E(t *testing.T, baseURL string, token string, projectID string) domain.Project {
	t.Helper()

	var response domain.Project
	doJSONRequestForE2E(t, stdhttp.MethodGet, fmt.Sprintf("%s/projects/%s", baseURL, projectID), token, nil, stdhttp.StatusOK, &response)
	return response
}

func doJSONRequestForE2E(t *testing.T, method string, url string, token string, payload any, expectedStatus int, out any) {
	t.Helper()

	var body *bytes.Reader
	if payload == nil {
		body = bytes.NewReader(nil)
	} else {
		data, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("json.Marshal returned error: %v", err)
		}
		body = bytes.NewReader(data)
	}

	req, err := stdhttp.NewRequest(method, url, body)
	if err != nil {
		t.Fatalf("NewRequest returned error: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := stdhttp.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("http request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != expectedStatus {
		var errorBody map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&errorBody)
		t.Fatalf("expected status %d, got %d for %s %s, body=%v", expectedStatus, resp.StatusCode, method, url, errorBody)
	}

	if out == nil {
		return
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
}
