//go:build integration

package github

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"deploy-service/internal/domain"
	"deploy-service/internal/testsupport"
)

func TestBootstrapRepositoryFlowCreatesArtifactsInTemporaryRepository(t *testing.T) {
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

	server := httptest.NewServer(repo)
	defer server.Close()

	automation := NewGitHubAutomation(server.URL, "ghp_test_token", "apps.example.test", "https://platform.example.test", "hook-secret")

	questions, err := automation.BuildBootstrapQuestions(ctx, "prj-123", domain.GitHubBootstrapQuestionsRequest{
		RepositoryOwner: repo.Owner,
		RepositoryName:  repo.Name,
		GitHubToken:     "ghp_request_token",
	})
	if err != nil {
		t.Fatalf("BuildBootstrapQuestions returned error: %v", err)
	}
	if questions.BaseBranch != "main" {
		t.Fatalf("expected base branch main, got %q", questions.BaseBranch)
	}
	if questions.DetectedDockerfile != "Dockerfile" {
		t.Fatalf("expected Dockerfile to be detected, got %q", questions.DetectedDockerfile)
	}
	if questions.DetectedServiceName != "example-service" {
		t.Fatalf("expected service name example-service, got %q", questions.DetectedServiceName)
	}
	if questions.DetectedContainerPort != 9090 {
		t.Fatalf("expected detected container port 9090, got %d", questions.DetectedContainerPort)
	}
	if questions.DetectedServicePort != 9090 {
		t.Fatalf("expected detected service port 9090, got %d", questions.DetectedServicePort)
	}
	if questions.DetectedServiceType != "LoadBalancer" {
		t.Fatalf("expected LoadBalancer service type, got %q", questions.DetectedServiceType)
	}

	response, err := automation.BootstrapRepositoryFlow(ctx, "prj-123", domain.BootstrapGitHubFlowRequest{
		RepositoryOwner: repo.Owner,
		RepositoryName:  repo.Name,
		GitHubToken:     "ghp_request_token",
	})
	if err != nil {
		t.Fatalf("BootstrapRepositoryFlow returned error: %v", err)
	}
	if !strings.HasPrefix(response.BranchName, "deploy-service/prj-123-") {
		t.Fatalf("expected generated branch prefix, got %q", response.BranchName)
	}
	if response.PullRequestURL == "" {
		t.Fatal("expected pull request url to be returned")
	}
	if !repo.BranchExists(ctx, response.BranchName) {
		t.Fatalf("expected branch %q to exist in temp repo", response.BranchName)
	}
	if err := repo.ValidateGeneratedArtifacts(ctx, response.BranchName, "prj-123", "example-service"); err != nil {
		t.Fatalf("ValidateGeneratedArtifacts returned error: %v", err)
	}

	workflow, err := repo.ReadFile(ctx, response.BranchName, ".github/workflows/deploy-service.yml")
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if !strings.Contains(workflow, "Dockerfile") {
		t.Fatal("expected workflow to reference Dockerfile")
	}
	if !strings.Contains(workflow, "ingress.yaml") {
		t.Fatal("expected workflow to include ingress deployment when apps base domain is configured")
	}

	serviceManifest, err := repo.ReadFile(ctx, response.BranchName, "k8s/example-service/service.yaml")
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if !strings.Contains(serviceManifest, "type: ClusterIP") {
		t.Fatal("expected service manifest to use ClusterIP behind shared gateway")
	}

	ingressManifest, err := repo.ReadFile(ctx, response.BranchName, "k8s/example-service/ingress.yaml")
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if !strings.Contains(ingressManifest, "kind: Ingress") || !strings.Contains(ingressManifest, "prj-123.apps.example.test") {
		t.Fatal("expected ingress manifest to expose project host")
	}
}
