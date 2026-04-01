//go:build !integration

package github

import (
	"strings"
	"testing"
)

// Тесты шаблонов для сгенерированных манифестов Kubernetes и workflow GitHub Actions.
// Они ловят непреднамеренные регрессии в шаблонах до того, как изменения попадут в GitHub Actions.

func TestSnapshotDeploymentYAML(t *testing.T) {
	t.Parallel()

	got := renderDeploymentYAML("my-svc", "production", 8080, 80, 1, "balanced")

	checks := []struct {
		desc        string
		mustContain string
	}{
		{"apiVersion", "apiVersion: apps/v1"},
		{"kind", "kind: Deployment"},
		{"name", "name: my-svc"},
		{"image placeholder", "image: IMAGE_PLACEHOLDER"},
		{"containerPort", "containerPort: 8080"},
		{"readinessProbe port", "port: 8080"},
		{"livenessProbe present", "livenessProbe:"},
		{"resource requests cpu", "cpu: 100m"},
		{"resource requests memory", "memory: 128Mi"},
		{"resource limits cpu", "cpu: 500m"},
		{"resource limits memory", "memory: 512Mi"},
		{"selector matchLabels", "app: my-svc"},
		{"stage label", "deploy-service.io/stage: production"},
	}
	for _, c := range checks {
		if !strings.Contains(got, c.mustContain) {
			t.Errorf("deployment.yaml missing %s: %q not found", c.desc, c.mustContain)
		}
	}
}

func TestSnapshotServiceYAML_LoadBalancer(t *testing.T) {
	t.Parallel()

	got := renderServiceYAML("my-svc", 80, 8080, "LoadBalancer")

	checks := []struct {
		desc        string
		mustContain string
	}{
		{"apiVersion", "apiVersion: v1"},
		{"kind", "kind: Service"},
		{"name", "name: my-svc"},
		{"selector app", "app: my-svc"},
		{"service port", "port: 80"},
		{"target port", "targetPort: 8080"},
		{"type LoadBalancer", "type: LoadBalancer"},
	}
	for _, c := range checks {
		if !strings.Contains(got, c.mustContain) {
			t.Errorf("service.yaml (LoadBalancer) missing %s: %q not found", c.desc, c.mustContain)
		}
	}
}

func TestSnapshotServiceYAML_ClusterIP(t *testing.T) {
	t.Parallel()

	got := renderServiceYAML("my-svc", 80, 8080, "ClusterIP")

	if !strings.Contains(got, "type: ClusterIP") {
		t.Error("service.yaml (ClusterIP) missing type: ClusterIP")
	}
	if strings.Contains(got, "LoadBalancer") {
		t.Error("service.yaml (ClusterIP) must not contain LoadBalancer")
	}
}

func TestSnapshotIngressYAML(t *testing.T) {
	t.Parallel()

	got := renderIngressYAML("my-svc", "prj-abc123", "production", "apps.example.com", 80)

	checks := []struct {
		desc        string
		mustContain string
	}{
		{"apiVersion", "apiVersion: networking.k8s.io/v1"},
		{"kind", "kind: Ingress"},
		{"name", "name: my-svc"},
		{"ingressClassName", "ingressClassName: nginx"},
		{"hostname", `host: "my-svc.prj-abc123.apps.example.com"`},
		{"service name", "name: my-svc"},
		{"service port", "number: 80"},
		{"path prefix", "pathType: Prefix"},
		{"path value", "path: /"},
	}
	for _, c := range checks {
		if !strings.Contains(got, c.mustContain) {
			t.Errorf("ingress.yaml missing %s: %q not found", c.desc, c.mustContain)
		}
	}
}

func TestSnapshotIngressYAML_NonProductionStage(t *testing.T) {
	t.Parallel()

	got := renderIngressYAML("my-svc", "prj-abc123", "staging", "apps.example.com", 80)

	if !strings.Contains(got, `host: "my-svc.prj-abc123.staging.apps.example.com"`) {
		t.Errorf("ingress.yaml for staging stage must use service.project.stage host, got:\n%s", got)
	}
	if strings.Contains(got, `host: "prj-abc123.staging.apps.example.com"`) {
		t.Error("ingress.yaml for staging stage must not use project-only host")
	}
}

func TestSnapshotWorkflowYAML_WithIngress(t *testing.T) {
	t.Parallel()

	got := renderWorkflowYAML("prj-abc123", "my-repo", "my-svc", "Dockerfile", "production", "main", true)

	checks := []struct {
		desc        string
		mustContain string
	}{
		{"workflow name", "name: Deploy Service"},
		{"branch trigger", "- main"},
		{"checkout step", "actions/checkout@v4"},
		{"docker login step", "docker/login-action@v3"},
		{"build-push step", "docker/build-push-action@v6"},
		{"dockerfile ref", "file: Dockerfile"},
		{"image name contains slug", "my-repo-my-svc"},
		{"kubeconfig setup", "KUBECONFIG_BASE64"},
		{"project id env", "PROJECT_ID: prj-abc123"},
		{"yc install", "yandex-cloud"},
		{"stage slug env", "STAGE_SLUG: ${{ vars.STAGE_SLUG || 'production' }}"},
		{"kubeconfig vcluster guard", "Verify kubeconfig targets project vcluster"},
		{"retry_or_fail helper", "retry_or_fail()"},
		{"namespace apply", "dry-run=client"},
		{"deployment apply", "k8s/production/my-svc/deployment.yaml"},
		{"service apply", "k8s/production/my-svc/service.yaml"},
		{"ingress apply", "k8s/production/my-svc/ingress.yaml"},
		{"verify step", "Verify deployment"},
		{"rollout status", "rollout status deployment/"},
		{"timeout flag", "--timeout=180s"},
		{"get pods", `kubectl -n "$STAGE_SLUG" get pods`},
	}
	for _, c := range checks {
		if !strings.Contains(got, c.mustContain) {
			t.Errorf("workflow.yml (with Ingress) missing %s: %q not found", c.desc, c.mustContain)
		}
	}
}

func TestSnapshotWorkflowYAML_WithoutIngress(t *testing.T) {
	t.Parallel()

	got := renderWorkflowYAML("prj-abc123", "my-repo", "my-svc", "Dockerfile", "production", "main", false)

	if strings.Contains(got, "ingress.yaml") {
		t.Error("workflow.yml (no Ingress) must not reference ingress.yaml")
	}
	// Проверяем, что шаг Verify все еще присутствует
	if !strings.Contains(got, "Verify deployment") {
		t.Error("workflow.yml must always include the Verify deployment step")
	}
	if !strings.Contains(got, "rollout status") {
		t.Error("workflow.yml must always include rollout status check")
	}
}

func TestSnapshotWorkflowYAML_DockerfileInSubdir(t *testing.T) {
	t.Parallel()

	got := renderWorkflowYAML("prj-abc123", "my-repo", "my-svc", "backend/Dockerfile", "production", "main", false)

	if !strings.Contains(got, "context: backend") {
		t.Error("build context should be set to Dockerfile's parent directory")
	}
	if !strings.Contains(got, "file: backend/Dockerfile") {
		t.Error("file field should contain full Dockerfile path")
	}
}
