//go:build !integration

package kubernetes

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"deploy-service/internal/domain"
)

func TestProjectGrafanaManifestUsesExpectedHostAndPrometheusURL(t *testing.T) {
	t.Parallel()

	provisioner := &KubectlProvisioner{
		appsBaseDomain:      "84.201.145.23.sslip.io",
		monitoringNamespace: "monitoring",
		prometheusBaseURL:   "http://prometheus.monitoring.svc.cluster.local:9090",
		lokiBaseURL:         "http://loki.monitoring.svc.cluster.local:3100",
	}

	manifest := provisioner.projectGrafanaManifest("prj-123", "project-prj-123", provisioner.appsBaseDomain)

	if !strings.Contains(manifest, `host: "grafana-prj-123.84.201.145.23.sslip.io"`) {
		t.Fatalf("expected grafana ingress host in manifest, got:\n%s", manifest)
	}
	if !strings.Contains(manifest, "url: http://prometheus.monitoring.svc.cluster.local:9090") {
		t.Fatalf("expected Prometheus datasource URL in manifest, got:\n%s", manifest)
	}
	if !strings.Contains(manifest, "url: http://loki.monitoring.svc.cluster.local:3100") {
		t.Fatalf("expected Loki datasource URL in manifest, got:\n%s", manifest)
	}
	if !strings.Contains(manifest, "project-prj-123") {
		t.Fatalf("expected dashboard queries to target project namespace, got:\n%s", manifest)
	}
	if !strings.Contains(manifest, "requests:\n              cpu: 25m") || !strings.Contains(manifest, "limits:\n              cpu: 250m") {
		t.Fatalf("expected grafana resource requests and limits in manifest, got:\n%s", manifest)
	}
}

func TestProjectGrafanaManifestFallsBackToDefaultMonitoringService(t *testing.T) {
	t.Parallel()

	provisioner := &KubectlProvisioner{
		appsBaseDomain: "apps.example.com",
	}

	manifest := provisioner.projectGrafanaManifest("prj-123", "project-prj-123", provisioner.appsBaseDomain)

	if !strings.Contains(manifest, "url: http://prometheus.monitoring.svc.cluster.local:9090") {
		t.Fatalf("expected default Prometheus URL in manifest, got:\n%s", manifest)
	}
	if !strings.Contains(manifest, "url: http://loki.monitoring.svc.cluster.local:3100") {
		t.Fatalf("expected default Loki URL in manifest, got:\n%s", manifest)
	}
	if !strings.Contains(manifest, "name: allow-project-grafana-egress") {
		t.Fatalf("expected grafana network policy in manifest, got:\n%s", manifest)
	}
	if !strings.Contains(manifest, "port: 3100") {
		t.Fatalf("expected grafana network policy to allow Loki egress, got:\n%s", manifest)
	}
}

func TestBuildProjectGrafanaDashboardProducesValidJSON(t *testing.T) {
	t.Parallel()

	dashboard := buildProjectGrafanaDashboard("prj-123", "project-prj-123")
	if !json.Valid([]byte(dashboard)) {
		t.Fatalf("expected valid dashboard json, got:\n%s", dashboard)
	}
	if !strings.Contains(dashboard, "project-prj-123") {
		t.Fatalf("expected dashboard queries to target namespace, got:\n%s", dashboard)
	}
	if strings.Contains(dashboard, `\\"`) {
		t.Fatalf("expected dashboard json to avoid over-escaped quotes, got:\n%s", dashboard)
	}
	if !strings.Contains(dashboard, "CPU cores by worker node") {
		t.Fatalf("expected worker cpu panel in dashboard, got:\n%s", dashboard)
	}
	if !strings.Contains(dashboard, "Memory GB by worker node") {
		t.Fatalf("expected worker memory panel in dashboard, got:\n%s", dashboard)
	}
	if !strings.Contains(dashboard, "CPU limit utilization %") {
		t.Fatalf("expected cpu limit utilization panel in dashboard, got:\n%s", dashboard)
	}
	if !strings.Contains(dashboard, "Memory limit utilization %") {
		t.Fatalf("expected memory limit utilization panel in dashboard, got:\n%s", dashboard)
	}
	if !strings.Contains(dashboard, "CPU throttling %") {
		t.Fatalf("expected cpu throttling panel in dashboard, got:\n%s", dashboard)
	}
	if !strings.Contains(dashboard, "Active pods by worker node") {
		t.Fatalf("expected active pods by worker node panel in dashboard, got:\n%s", dashboard)
	}
	if !strings.Contains(dashboard, "Network egress by worker node") {
		t.Fatalf("expected worker network egress panel in dashboard, got:\n%s", dashboard)
	}
	if !strings.Contains(dashboard, "Network ingress by worker node") {
		t.Fatalf("expected worker network ingress panel in dashboard, got:\n%s", dashboard)
	}
	if !strings.Contains(dashboard, "Top pods in project") {
		t.Fatalf("expected top pods table panel in dashboard, got:\n%s", dashboard)
	}
	if !strings.Contains(dashboard, "cpu used vs limit") || !strings.Contains(dashboard, "memory used vs limit") {
		t.Fatalf("expected limit utilization labels in dashboard, got:\n%s", dashboard)
	}
	if !strings.Contains(dashboard, "throttled periods") {
		t.Fatalf("expected throttling label in dashboard, got:\n%s", dashboard)
	}
	if !strings.Contains(dashboard, `worker {{kubernetes_io_hostname}}`) {
		t.Fatalf("expected worker node legend labels in dashboard, got:\n%s", dashboard)
	}
	if !strings.Contains(dashboard, `container_spec_cpu_quota{namespace=\"project-prj-123\"`) {
		t.Fatalf("expected cpu limit panel to use container spec metrics, got:\n%s", dashboard)
	}
	if !strings.Contains(dashboard, `container_spec_memory_limit_bytes{namespace=\"project-prj-123\"`) {
		t.Fatalf("expected memory limit panel to use container spec metrics, got:\n%s", dashboard)
	}
	if strings.Contains(dashboard, "kube_pod_info") || strings.Contains(dashboard, "kube_deployment_spec_replicas") || strings.Contains(dashboard, "kube_pod_container_resource_limits") {
		t.Fatalf("expected dashboard to avoid kube-state-metrics dependencies, got:\n%s", dashboard)
	}
	if strings.Contains(dashboard, "kube_pod_container_status_restarts_total") {
		t.Fatalf("expected top pods table to avoid unavailable restart metric, got:\n%s", dashboard)
	}
	if !strings.Contains(dashboard, "active containers by pod") {
		t.Fatalf("expected available pod activity label in dashboard table, got:\n%s", dashboard)
	}
	if !strings.Contains(dashboard, `"uid": "`+domain.ProjectGrafanaDashboardUID("prj-123")+`"`) {
		t.Fatalf("expected dashboard to use generated grafana uid, got:\n%s", dashboard)
	}
}

func TestGetProjectKubeconfigReconcilesMissingNamespaceBeforeFailing(t *testing.T) {
	t.Parallel()

	projectID := "prj-1"
	namespace := NamespaceForProject(projectID)
	secretName := "vc-" + vclusterNameFromProjectID(projectID)
	secretCalls := 0
	applyCalled := false

	provisioner := &KubectlProvisioner{
		runKubectlOverride: func(_ context.Context, args []string, _ []byte) (string, error) {
			key := strings.Join(args, " ")
			switch key {
			case fmt.Sprintf("get secret %s -n %s -o jsonpath={.data.config}", secretName, namespace):
				secretCalls++
				if secretCalls == 1 {
					return "", errors.New(`Error from server (NotFound): namespaces "` + namespace + `" not found`)
				}
				return "", errors.New(`Error from server (NotFound): secrets "` + secretName + `" not found`)
			case "apply -f - --validate=false":
				applyCalled = true
				return "namespace/" + namespace + " created", nil
			default:
				t.Fatalf("unexpected kubectl call: %s", key)
			}
			return "", nil
		},
		sleepOverride: func(_ time.Duration) {},
	}

	_, err := provisioner.GetProjectKubeconfig(context.Background(), projectID)
	if err == nil {
		t.Fatal("expected kubeconfig fetch to fail after reconcile when secret is still absent")
	}
	if !applyCalled {
		t.Fatal("expected reconcile apply call for missing namespace")
	}
	if secretCalls < 2 {
		t.Fatalf("expected kubeconfig secret read attempts, got %d calls", secretCalls)
	}
}

func TestGetProjectKubeconfigRetriesTransientMissingSecret(t *testing.T) {
	t.Parallel()

	projectID := "prj-1"
	namespace := NamespaceForProject(projectID)
	vclusterName := vclusterNameFromProjectID(projectID)
	secretName := "vc-" + vclusterName
	secretCalls := 0

	provisioner := &KubectlProvisioner{
		runKubectlOverride: func(_ context.Context, args []string, _ []byte) (string, error) {
			key := strings.Join(args, " ")
			switch key {
			case fmt.Sprintf("get secret %s -n %s -o jsonpath={.data.config}", secretName, namespace):
				secretCalls++
				if secretCalls < 3 {
					return "", errors.New(`Error from server (NotFound): secrets "` + secretName + `" not found`)
				}
				return base64.StdEncoding.EncodeToString([]byte(`apiVersion: v1`)), nil
			case fmt.Sprintf("get svc %s -n %s -o jsonpath={.status.loadBalancer.ingress[0].ip}", vclusterName, namespace):
				return "89.169.142.12", nil
			default:
				t.Fatalf("unexpected kubectl call: %s", key)
			}
			return "", nil
		},
		sleepOverride: func(_ time.Duration) {},
	}

	kubeconfig, err := provisioner.GetProjectKubeconfig(context.Background(), projectID)
	if err != nil {
		t.Fatalf("GetProjectKubeconfig returned error: %v", err)
	}
	if kubeconfig != "apiVersion: v1" {
		t.Fatalf("unexpected kubeconfig payload: %q", kubeconfig)
	}
	if secretCalls != 3 {
		t.Fatalf("expected 3 secret reads before success, got %d", secretCalls)
	}
}

func TestGetProjectKubeconfigEnsuresVClusterServiceLoadBalancerAndRewritesServer(t *testing.T) {
	t.Parallel()

	projectID := "prj-1"
	namespace := NamespaceForProject(projectID)
	vclusterName := vclusterNameFromProjectID(projectID)
	secretName := "vc-" + vclusterName
	ipCalls := 0
	patchCalled := false

	rawKubeconfig := `apiVersion: v1
kind: Config
clusters:
- cluster:
    certificate-authority-data: TESTCA
    server: https://localhost:8443
  name: vcluster
`

	provisioner := &KubectlProvisioner{
		runKubectlOverride: func(_ context.Context, args []string, _ []byte) (string, error) {
			key := strings.Join(args, " ")
			switch key {
			case fmt.Sprintf("get secret %s -n %s -o jsonpath={.data.config}", secretName, namespace):
				return base64.StdEncoding.EncodeToString([]byte(rawKubeconfig)), nil
			case fmt.Sprintf("get svc %s -n %s -o jsonpath={.status.loadBalancer.ingress[0].ip}", vclusterName, namespace):
				ipCalls++
				if ipCalls < 3 {
					return "", nil
				}
				return "89.169.142.12", nil
			case fmt.Sprintf("patch svc %s -n %s -p {\"spec\":{\"type\":\"LoadBalancer\"}}", vclusterName, namespace):
				patchCalled = true
				return "service/" + vclusterName + " patched", nil
			default:
				t.Fatalf("unexpected kubectl call: %s", key)
			}
			return "", nil
		},
		sleepOverride: func(_ time.Duration) {},
	}

	kubeconfig, err := provisioner.GetProjectKubeconfig(context.Background(), projectID)
	if err != nil {
		t.Fatalf("GetProjectKubeconfig returned error: %v", err)
	}
	if !patchCalled {
		t.Fatal("expected vcluster service to be patched to LoadBalancer")
	}
	if !strings.Contains(kubeconfig, "server: https://89.169.142.12:443") {
		t.Fatalf("expected kubeconfig server to be rewritten to external endpoint, got:\n%s", kubeconfig)
	}
	if strings.Contains(kubeconfig, "certificate-authority-data:") {
		t.Fatalf("expected certificate-authority-data to be removed after rewrite, got:\n%s", kubeconfig)
	}
}

func TestRunKubectlInVClusterUsesRawKubeconfigWithoutLoadBalancer(t *testing.T) {
	t.Parallel()

	projectID := "prj-1"
	namespace := NamespaceForProject(projectID)
	vclusterName := vclusterNameFromProjectID(projectID)
	secretName := "vc-" + vclusterName
	loadBalancerPatched := false

	rawKubeconfig := `apiVersion: v1
kind: Config
clusters:
- cluster:
    certificate-authority-data: TESTCA
    server: https://localhost:8443
  name: vcluster
users:
- name: user
  user:
    token: test
contexts:
- context:
    cluster: vcluster
    user: user
  name: vcluster
current-context: vcluster
`

	provisioner := &KubectlProvisioner{
		runKubectlOverride: func(_ context.Context, args []string, _ []byte) (string, error) {
			key := strings.Join(args, " ")
			switch {
			case key == fmt.Sprintf("get secret %s -n %s -o jsonpath={.data.config}", secretName, namespace):
				return base64.StdEncoding.EncodeToString([]byte(rawKubeconfig)), nil
			case strings.HasPrefix(key, fmt.Sprintf("patch svc %s -n %s", vclusterName, namespace)):
				loadBalancerPatched = true
				t.Fatalf("unexpected LoadBalancer patch call: %s", key)
			default:
				t.Fatalf("unexpected kubectl call: %s", key)
			}
			return "", nil
		},
		startVClusterPortForwardOverride: func(_ context.Context, gotNamespace, gotName string, localPort int) (func(), error) {
			if gotNamespace != namespace || gotName != vclusterName {
				t.Fatalf("unexpected port-forward target %s/%s", gotNamespace, gotName)
			}
			if localPort <= 0 {
				t.Fatalf("expected reserved local port, got %d", localPort)
			}
			return func() {}, nil
		},
	}

	_, err := provisioner.runKubectlInVCluster(context.Background(), projectID, []string{"get", "ns"}, nil)
	if err == nil {
		t.Fatal("expected kubectl execution to fail without a real kubectl binary")
	}
	if loadBalancerPatched {
		t.Fatal("did not expect vcluster service to be patched to LoadBalancer")
	}
	if !strings.Contains(err.Error(), "--kubeconfig") {
		t.Fatalf("expected kubectl invocation with temp kubeconfig, got: %v", err)
	}
}

func TestRewriteKubeconfigLocalhostServer(t *testing.T) {
	t.Parallel()

	input := `apiVersion: v1
kind: Config
clusters:
- cluster:
    certificate-authority-data: TESTCA
    server: https://localhost:8443
  name: vcluster
`

	output := rewriteKubeconfigLocalhostServer(input, "vcluster-prj-1.project-prj-1.svc:443")
	if strings.Contains(output, "certificate-authority-data:") {
		t.Fatalf("expected certificate-authority-data to be removed, got:\n%s", output)
	}
	if !strings.Contains(output, "insecure-skip-tls-verify: true") {
		t.Fatalf("expected insecure-skip-tls-verify in kubeconfig, got:\n%s", output)
	}
	if !strings.Contains(output, "server: https://vcluster-prj-1.project-prj-1.svc:443") {
		t.Fatalf("expected kubeconfig server to be rewritten, got:\n%s", output)
	}
}

func TestRewriteKubeconfigLocalhostServerNoopWithoutLocalhostEndpoint(t *testing.T) {
	t.Parallel()

	input := `apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://10.10.10.10:443
  name: vcluster
`

	output := rewriteKubeconfigLocalhostServer(input, "vcluster-prj-1.project-prj-1.svc:443")
	if output != input {
		t.Fatalf("expected kubeconfig to stay unchanged, got:\n%s", output)
	}
}

func TestKubeconfigUsesClusterServiceServer(t *testing.T) {
	t.Parallel()

	input := `apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://vcluster-prj-1.project-prj-1.svc:443
  name: vcluster
`

	if !kubeconfigUsesClusterServiceServer(input) {
		t.Fatalf("expected kubeconfig to be detected as cluster-service endpoint, got:\n%s", input)
	}
}

func TestRewriteKubeconfigServerForPortForwardFromClusterService(t *testing.T) {
	t.Parallel()

	input := `apiVersion: v1
kind: Config
clusters:
- cluster:
    certificate-authority-data: TESTCA
    server: https://vcluster-prj-1.project-prj-1.svc:443
  name: vcluster
`

	output := rewriteKubeconfigServerForPortForward(input, 18443)
	if strings.Contains(output, "certificate-authority-data:") {
		t.Fatalf("expected certificate-authority-data to be removed, got:\n%s", output)
	}
	if !strings.Contains(output, "insecure-skip-tls-verify: true") {
		t.Fatalf("expected insecure-skip-tls-verify in kubeconfig, got:\n%s", output)
	}
	if !strings.Contains(output, "server: https://127.0.0.1:18443") {
		t.Fatalf("expected kubeconfig server to be rewritten to local port-forward endpoint, got:\n%s", output)
	}
}

type kubectlCallResult struct {
	output string
	err    error
}

func newKubectlResponder(t *testing.T, results map[string]kubectlCallResult) func(context.Context, []string, []byte) (string, error) {
	t.Helper()

	return func(_ context.Context, args []string, _ []byte) (string, error) {
		key := strings.Join(args, " ")
		result, ok := results[key]
		if !ok {
			t.Fatalf("unexpected kubectl call: %s", key)
		}
		return result.output, result.err
	}
}

func newVClusterKubectlResponder(t *testing.T, results map[string]kubectlCallResult) func(context.Context, string, []string, []byte) (string, error) {
	t.Helper()

	return func(_ context.Context, _ string, args []string, _ []byte) (string, error) {
		key := strings.Join(args, " ")
		result, ok := results[key]
		if !ok {
			t.Fatalf("unexpected kubectl in vcluster call: %s", key)
		}
		return result.output, result.err
	}
}

func TestGetProjectRuntimeStatusNamespaceNotFound(t *testing.T) {
	t.Parallel()

	projectID := "prj-1"
	namespace := NamespaceForProject(projectID)
	provisioner := &KubectlProvisioner{
		runKubectlOverride: newKubectlResponder(t, map[string]kubectlCallResult{
			fmt.Sprintf("get namespace %s -o json", namespace): {
				err: errors.New(`Error from server (NotFound): namespaces "` + namespace + `" not found`),
			},
		}),
	}

	status, err := provisioner.GetProjectRuntimeStatus(context.Background(), projectID)
	if err != nil {
		t.Fatalf("GetProjectRuntimeStatus returned error: %v", err)
	}
	if status.NamespaceExists {
		t.Fatalf("expected namespace to be absent, got %+v", status)
	}
	if status.DeploymentExists || status.ServiceExists || status.HTTPRouteExists {
		t.Fatalf("expected deployment/service/route to be absent, got %+v", status)
	}
	if !strings.Contains(status.Message, "namespace not found") {
		t.Fatalf("expected namespace-not-found message, got %q", status.Message)
	}
}

func TestGetProjectRuntimeStatusNamespaceExistsButDeploymentNotApplied(t *testing.T) {
	t.Parallel()

	projectID := "prj-1"
	namespace := NamespaceForProject(projectID)
	provisioner := &KubectlProvisioner{
		runKubectlOverride: newKubectlResponder(t, map[string]kubectlCallResult{
			fmt.Sprintf("get namespace %s -o json", namespace):     {output: `{}`},
			fmt.Sprintf("get deployment -n %s -o json", namespace): {output: `{"items":[]}`},
			fmt.Sprintf("get service -n %s -o json", namespace): {
				output: `{"items":[{"metadata":{"name":"kubernetes"}}]}`,
			},
			fmt.Sprintf("get ingress -n %s -o json", namespace): {output: `{"items":[]}`},
			fmt.Sprintf("get pods -n %s -o json", namespace):    {output: `{"items":[]}`},
		}),
	}

	status, err := provisioner.GetProjectRuntimeStatus(context.Background(), projectID)
	if err != nil {
		t.Fatalf("GetProjectRuntimeStatus returned error: %v", err)
	}
	if !status.NamespaceExists {
		t.Fatalf("expected namespace to exist, got %+v", status)
	}
	if status.DeploymentExists || status.ServiceExists || status.HTTPRouteExists {
		t.Fatalf("expected deployment/service/route to be absent, got %+v", status)
	}
	if status.Message != "namespace exists, but application manifests have not been applied yet" {
		t.Fatalf("unexpected message: %q", status.Message)
	}
}

func TestGetProjectRuntimeStatusDeploymentExistsButPodsNotReady(t *testing.T) {
	t.Parallel()

	projectID := "prj-1"
	namespace := NamespaceForProject(projectID)
	provisioner := &KubectlProvisioner{
		runKubectlOverride: newKubectlResponder(t, map[string]kubectlCallResult{
			fmt.Sprintf("get namespace %s -o json", namespace): {output: `{}`},
			fmt.Sprintf("get deployment -n %s -o json", namespace): {
				output: `{"items":[{"status":{"replicas":1,"readyReplicas":0,"availableReplicas":0}}]}`,
			},
			fmt.Sprintf("get service -n %s -o json", namespace): {
				output: `{"items":[{"metadata":{"name":"kubernetes"}},{"metadata":{"name":"app"}}]}`,
			},
			fmt.Sprintf("get ingress -n %s -o json", namespace): {output: `{"items":[]}`},
			fmt.Sprintf("get pods -n %s -o json", namespace): {
				output: `{"items":[{"metadata":{"name":"app-abc"},"status":{"phase":"Pending","containerStatuses":[{"ready":false,"restartCount":1}]}}]}`,
			},
		}),
	}

	status, err := provisioner.GetProjectRuntimeStatus(context.Background(), projectID)
	if err != nil {
		t.Fatalf("GetProjectRuntimeStatus returned error: %v", err)
	}
	if !status.NamespaceExists || !status.DeploymentExists || !status.ServiceExists {
		t.Fatalf("expected namespace/deployment/service to exist, got %+v", status)
	}
	if status.HTTPRouteExists {
		t.Fatalf("expected HTTPRoute to be absent, got %+v", status)
	}
	if status.ReadyReplicas != 0 || status.DesiredReplicas != 1 {
		t.Fatalf("unexpected replicas state: %+v", status)
	}
	if len(status.Pods) != 1 || status.Pods[0].Ready || status.Pods[0].Restarts != 1 {
		t.Fatalf("unexpected pod status: %+v", status.Pods)
	}
	if status.Message != "deployment exists, but pods are still starting or unhealthy" {
		t.Fatalf("unexpected message: %q", status.Message)
	}
}

func TestGetProjectRuntimeStatusReadyButRouteMissing(t *testing.T) {
	t.Parallel()

	projectID := "prj-1"
	namespace := NamespaceForProject(projectID)
	provisioner := &KubectlProvisioner{
		runKubectlOverride: newKubectlResponder(t, map[string]kubectlCallResult{
			fmt.Sprintf("get namespace %s -o json", namespace): {output: `{}`},
			fmt.Sprintf("get deployment -n %s -o json", namespace): {
				output: `{"items":[{"status":{"replicas":1,"readyReplicas":1,"availableReplicas":1}}]}`,
			},
			fmt.Sprintf("get service -n %s -o json", namespace): {
				output: `{"items":[{"metadata":{"name":"kubernetes"}},{"metadata":{"name":"app"}}]}`,
			},
			fmt.Sprintf("get ingress -n %s -o json", namespace): {output: `{"items":[]}`},
			fmt.Sprintf("get pods -n %s -o json", namespace): {
				output: `{"items":[{"metadata":{"name":"app-abc"},"status":{"phase":"Running","containerStatuses":[{"ready":true,"restartCount":0}]}}]}`,
			},
		}),
	}

	status, err := provisioner.GetProjectRuntimeStatus(context.Background(), projectID)
	if err != nil {
		t.Fatalf("GetProjectRuntimeStatus returned error: %v", err)
	}
	if !status.DeploymentExists || !status.ServiceExists || !status.NamespaceExists {
		t.Fatalf("expected runtime resources to exist, got %+v", status)
	}
	if status.HTTPRouteExists {
		t.Fatalf("expected missing Ingress, got %+v", status)
	}
	if status.Message != "application has ready replicas, but Ingress was not found" {
		t.Fatalf("unexpected message: %q", status.Message)
	}
}

func TestGetProjectRuntimeStatusReadyWithRoute(t *testing.T) {
	t.Parallel()

	projectID := "prj-1"
	namespace := NamespaceForProject(projectID)
	provisioner := &KubectlProvisioner{
		runKubectlOverride: newKubectlResponder(t, map[string]kubectlCallResult{
			fmt.Sprintf("get namespace %s -o json", namespace): {output: `{}`},
			fmt.Sprintf("get deployment -n %s -o json", namespace): {
				output: `{"items":[{"status":{"replicas":1,"readyReplicas":1,"availableReplicas":1}}]}`,
			},
			fmt.Sprintf("get service -n %s -o json", namespace): {
				output: `{"items":[{"metadata":{"name":"kubernetes"}},{"metadata":{"name":"app"}}]}`,
			},
			fmt.Sprintf("get ingress -n %s -o json", namespace): {
				output: `{"items":[{"spec":{"rules":[{"host":"app.example.com"}]}}]}`,
			},
			fmt.Sprintf("get pods -n %s -o json", namespace): {
				output: `{"items":[{"metadata":{"name":"app-abc"},"status":{"phase":"Running","containerStatuses":[{"ready":true,"restartCount":0}]}}]}`,
			},
		}),
	}

	status, err := provisioner.GetProjectRuntimeStatus(context.Background(), projectID)
	if err != nil {
		t.Fatalf("GetProjectRuntimeStatus returned error: %v", err)
	}
	if !status.HTTPRouteExists {
		t.Fatalf("expected HTTPRoute to exist, got %+v", status)
	}
	if status.Message != "application is deployed and has ready replicas" {
		t.Fatalf("unexpected message: %q", status.Message)
	}
}

func TestApplyImageUsesJSONPatch(t *testing.T) {
	t.Parallel()

	projectID := "prj-1"
	imageTag := "ghcr.io/example/app:abc123"
	expectedPatch := fmt.Sprintf(`[{"op":"replace","path":"/spec/template/spec/containers/0/image","value":"%s"}]`, imageTag)

	provisioner := &KubectlProvisioner{
		runKubectlOverride: func(_ context.Context, args []string, _ []byte) (string, error) {
			got := strings.Join(args, " ")
			want := fmt.Sprintf("patch deployment -n %s --type=json -p %s --all", NamespaceForProject(projectID), expectedPatch)
			if got != want {
				t.Fatalf("unexpected kubectl args:\n got:  %s\n want: %s", got, want)
			}
			return "deployment.apps/my-svc patched", nil
		},
	}

	if err := provisioner.ApplyImage(context.Background(), projectID, imageTag); err != nil {
		t.Fatalf("ApplyImage returned error: %v", err)
	}
}

func TestApplyImageToStageUsesJSONPatch(t *testing.T) {
	t.Parallel()

	projectID := "prj-1"
	stageSlug := "production"
	imageTag := "ghcr.io/example/app:def456"
	expectedPatch := fmt.Sprintf(`[{"op":"replace","path":"/spec/template/spec/containers/0/image","value":"%s"}]`, imageTag)

	provisioner := &KubectlProvisioner{
		runKubectlInVClusterOverride: func(_ context.Context, _ string, args []string, _ []byte) (string, error) {
			got := strings.Join(args, " ")
			want := fmt.Sprintf("patch deployment -n %s --type=json -p %s --all", stageSlug, expectedPatch)
			if got != want {
				t.Fatalf("unexpected kubectl args:\n got:  %s\n want: %s", got, want)
			}
			return "deployment.apps/my-svc patched", nil
		},
	}

	if err := provisioner.ApplyImageToStage(context.Background(), projectID, stageSlug, imageTag); err != nil {
		t.Fatalf("ApplyImageToStage returned error: %v", err)
	}
}

func TestGetStageRuntimeStatusIncludesServiceAndRoute(t *testing.T) {
	t.Parallel()

	stageSlug := "production"
	provisioner := &KubectlProvisioner{
		runKubectlInVClusterOverride: newVClusterKubectlResponder(t, map[string]kubectlCallResult{
			fmt.Sprintf("get deployment -n %s -o json", stageSlug): {
				output: `{"items":[{"status":{"replicas":1,"readyReplicas":1,"availableReplicas":1}}]}`,
			},
			fmt.Sprintf("get service -n %s -o json", stageSlug): {
				output: `{"items":[{"metadata":{"name":"kubernetes"}},{"metadata":{"name":"app"}}]}`,
			},
			fmt.Sprintf("get ingress -n %s -o json", stageSlug): {
				output: `{"items":[{"spec":{"rules":[{"host":"app.example.com"}]}}]}`,
			},
			fmt.Sprintf("get pods -n %s -o json", stageSlug): {
				output: `{"items":[{"metadata":{"name":"app-abc"},"status":{"phase":"Running","containerStatuses":[{"ready":true,"restartCount":0}]}}]}`,
			},
		}),
	}

	status, err := provisioner.GetStageRuntimeStatus(context.Background(), "prj-1", stageSlug)
	if err != nil {
		t.Fatalf("GetStageRuntimeStatus returned error: %v", err)
	}
	if !status.NamespaceExists || !status.ServiceExists || !status.HTTPRouteExists {
		t.Fatalf("expected namespace/service/route in stage runtime, got %+v", status)
	}
	if status.Message != "application is deployed and has ready replicas" {
		t.Fatalf("unexpected message: %q", status.Message)
	}
}
