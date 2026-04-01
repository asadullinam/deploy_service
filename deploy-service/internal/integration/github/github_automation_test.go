//go:build !integration

package github

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRenderWorkflowYAMLIncludesFallbackCurrentContextLogic(t *testing.T) {
	t.Parallel()

	workflow := renderWorkflowYAML("prj-123", "repo", "api", "Dockerfile", "production", "main", true)

	requiredFragments := []string{
		`CURRENT_CONTEXT=$(kubectl config current-context 2>/dev/null || true)`,
		`CURRENT_CONTEXT=$(kubectl config view -o jsonpath="{.contexts[0].name}")`,
		`kubectl config use-context "$CURRENT_CONTEXT"`,
		`Skipping kubeconfig exec normalization because no current user was found`,
		`retry_or_fail() {`,
		`failed after 5 attempts`,
		`PROJECT_ID: prj-123`,
		`STAGE_SLUG: ${{ vars.STAGE_SLUG || 'production' }}`,
		`Verify kubeconfig targets project vcluster`,
		`HOST_PROJECT_NAMESPACE="project-${PROJECT_ID}"`,
		`Открой deploy-service UI -> Проект -> Доступ -> «Скопировать KUBECONFIG_BASE64»`,
		`kubectl -n "$STAGE_SLUG" apply --validate=false -f k8s/production/api/ingress.yaml`,
	}

	for _, fragment := range requiredFragments {
		if !strings.Contains(workflow, fragment) {
			t.Fatalf("expected workflow to contain %q", fragment)
		}
	}
}

func TestRenderWorkflowYAMLCanSkipIngress(t *testing.T) {
	t.Parallel()

	workflow := renderWorkflowYAML("prj-123", "repo", "api", "Dockerfile", "production", "main", false)

	if strings.Contains(workflow, "ingress.yaml") {
		t.Fatal("expected workflow without ingress to omit ingress apply step")
	}
}

func TestResolveServiceExposureUsesSharedIngressByDefault(t *testing.T) {
	t.Parallel()

	serviceType, needsIngress := resolveServiceExposure("LoadBalancer", false, "apps.example.test")
	if serviceType != "ClusterIP" || !needsIngress {
		t.Fatalf("expected shared ingress exposure, got serviceType=%q ingress=%v", serviceType, needsIngress)
	}
}

func TestResolveServiceExposureUsesDedicatedLoadBalancerWhenRequested(t *testing.T) {
	t.Parallel()

	serviceType, needsIngress := resolveServiceExposure("LoadBalancer", true, "apps.example.test")
	if serviceType != "LoadBalancer" || needsIngress {
		t.Fatalf("expected dedicated load balancer exposure, got serviceType=%q ingress=%v", serviceType, needsIngress)
	}
}

func TestRequestJSONRequiresToken(t *testing.T) {
	t.Parallel()

	automation := NewGitHubAutomation("https://api.github.com", "", "", "", "")
	_, err := automation.requestJSON(context.Background(), "GET", "/repos/example/repo", nil)
	if err == nil {
		t.Fatal("expected requestJSON to fail when GitHub token is missing")
	}
	if !strings.Contains(err.Error(), "github token is required") {
		t.Fatalf("expected helpful token error, got %v", err)
	}
}

func TestSanitizeNameNormalizesUnsafeCharacters(t *testing.T) {
	t.Parallel()

	got := sanitizeName(" Example_Service.v1 ")
	if got != "example-service-v1" {
		t.Fatalf("expected normalized name example-service-v1, got %q", got)
	}
}

func TestDetectPortFromDockerfileReadsExpose(t *testing.T) {
	t.Parallel()

	port := detectPortFromDockerfile("FROM golang:1.25\nEXPOSE 9090\nCMD [\"/app\"]\n")
	if port != 9090 {
		t.Fatalf("expected port 9090, got %d", port)
	}
}

func TestRenderDeploymentYAMLUsesReplicaCountAndResourceProfile(t *testing.T) {
	t.Parallel()

	manifest := renderDeploymentYAML("api", "staging", 8080, 80, 3, "performance")

	requiredFragments := []string{
		"replicas: 3",
		"deploy-service.io/stage: staging",
		"cpu: 250m",
		"memory: 256Mi",
		"cpu: 1000m",
		"memory: 1024Mi",
	}
	for _, fragment := range requiredFragments {
		if !strings.Contains(manifest, fragment) {
			t.Fatalf("expected deployment manifest to contain %q, got:\n%s", fragment, manifest)
		}
	}
}

func TestNormalizeServiceTypeRejectsUnknownValues(t *testing.T) {
	t.Parallel()

	if got := normalizeServiceType("NodePort"); got != "" {
		t.Fatalf("expected empty normalized value for unknown service type, got %q", got)
	}
	if got := normalizeServiceType("clusterip"); got != "ClusterIP" {
		t.Fatalf("expected ClusterIP, got %q", got)
	}
}

func TestNormalizedResourceProfileFallsBackToBalanced(t *testing.T) {
	t.Parallel()

	if got := normalizedResourceProfile("unknown"); got != "balanced" {
		t.Fatalf("expected balanced fallback, got %q", got)
	}
}

func TestIsNoCommitsBetweenBranchesError(t *testing.T) {
	t.Parallel()

	if isNoCommitsBetweenBranchesError(nil) {
		t.Fatal("expected nil error not to match")
	}

	err := context.DeadlineExceeded
	if isNoCommitsBetweenBranchesError(err) {
		t.Fatalf("did not expect %q to match", err.Error())
	}

	noCommitsErr := "create pull request: github api status 422 for POST /repos/org/repo/pulls: {\"message\":\"Validation Failed\",\"errors\":[{\"resource\":\"PullRequest\",\"code\":\"custom\",\"message\":\"No commits between master and deploy-service/prj-1-123\"}]}"
	if !isNoCommitsBetweenBranchesError(errors.New(noCommitsErr)) {
		t.Fatal("expected no-commits validation error to match")
	}
}
