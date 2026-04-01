package kubernetes

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"deploy-service/internal/domain"
	"deploy-service/internal/service"
)

// Проверка на этапе компиляции: KubectlProvisioner реализует service.Provisioner.
var _ service.Provisioner = (*KubectlProvisioner)(nil)

var kubeconfigLocalhostServerRe = regexp.MustCompile(`(?m)^(\s*)server:\s*https://localhost:8443\s*$`)
var kubeconfigClusterServiceServerRe = regexp.MustCompile(`(?m)^(\s*)server:\s*https://[a-z0-9.-]+\.svc(?:\.cluster\.local)?(?::\d+)?\s*$`)
var kubeconfigCADataRe = regexp.MustCompile(`(?m)^\s*certificate-authority-data:.*\n`)

const kubeconfigSecretReadMaxAttempts = 180
const kubeconfigSecretReadRetryDelay = time.Second
const vclusterExternalIPMaxAttempts = 45
const vclusterExternalIPRetryDelay = time.Second
const ingressNginxIPMaxAttempts = 60
const ingressNginxIPRetryDelay = 10 * time.Second
const defaultIngressNginxManifestURL = "https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-v1.11.2/deploy/static/provider/cloud/deploy.yaml"

func rewriteKubeconfigLocalhostServer(kubeconfig, endpoint string) string {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return kubeconfig
	}
	if !strings.Contains(endpoint, "://") {
		endpoint = "https://" + endpoint
	}

	match := kubeconfigLocalhostServerRe.FindStringSubmatch(kubeconfig)
	if len(match) < 2 {
		return kubeconfig
	}

	indent := match[1]
	replacement := fmt.Sprintf("%sinsecure-skip-tls-verify: true\n%sserver: %s", indent, indent, endpoint)
	updated := kubeconfigLocalhostServerRe.ReplaceAllString(kubeconfig, replacement)
	updated = kubeconfigCADataRe.ReplaceAllString(updated, "")
	return updated
}

func kubeconfigUsesLocalhostServer(kubeconfig string) bool {
	return kubeconfigLocalhostServerRe.MatchString(kubeconfig)
}

func kubeconfigUsesClusterServiceServer(kubeconfig string) bool {
	return kubeconfigClusterServiceServerRe.MatchString(kubeconfig)
}

func rewriteKubeconfigServerForPortForward(kubeconfig string, localPort int) string {
	endpoint := fmt.Sprintf("127.0.0.1:%d", localPort)

	rewritten := rewriteKubeconfigLocalhostServer(kubeconfig, endpoint)
	if rewritten != kubeconfig {
		return rewritten
	}

	match := kubeconfigClusterServiceServerRe.FindStringSubmatch(kubeconfig)
	if len(match) < 2 {
		return kubeconfig
	}

	indent := match[1]
	replacement := fmt.Sprintf("%sinsecure-skip-tls-verify: true\n%sserver: https://%s", indent, indent, endpoint)
	rewritten = kubeconfigClusterServiceServerRe.ReplaceAllString(kubeconfig, replacement)
	rewritten = kubeconfigCADataRe.ReplaceAllString(rewritten, "")
	return rewritten
}

func reserveLocalPort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()

	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("unexpected listener addr type %T", listener.Addr())
	}
	return addr.Port, nil
}

func (p *KubectlProvisioner) startVClusterPortForward(
	ctx context.Context,
	namespace string,
	serviceName string,
	localPort int,
) (func(), error) {
	if p.startVClusterPortForwardOverride != nil {
		return p.startVClusterPortForwardOverride(ctx, namespace, serviceName, localPort)
	}

	args := make([]string, 0, 12)
	if p.contextName != "" {
		args = append(args, "--context", p.contextName)
	}
	args = append(args,
		"-n", namespace,
		"port-forward", "service/"+serviceName,
		fmt.Sprintf("%d:443", localPort),
		"--address", "127.0.0.1",
	)

	pfCtx, cancel := context.WithCancel(ctx)
	command := exec.CommandContext(pfCtx, p.kubectlPath, args...)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	command.Stdout = &stderr

	if err := command.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("start vcluster port-forward: %w", err)
	}

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- command.Wait()
	}()

	addr := fmt.Sprintf("127.0.0.1:%d", localPort)
	deadline := time.Now().Add(15 * time.Second)
	for {
		select {
		case err := <-waitCh:
			cancel()
			errText := strings.TrimSpace(stderr.String())
			if err != nil {
				return nil, fmt.Errorf("vcluster port-forward exited early: %w: %s", err, errText)
			}
			return nil, fmt.Errorf("vcluster port-forward exited early: %s", errText)
		default:
		}

		conn, err := net.DialTimeout("tcp", addr, 250*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			stop := func() {
				cancel()
				select {
				case <-waitCh:
				case <-time.After(2 * time.Second):
				}
			}
			return stop, nil
		}

		if time.Now().After(deadline) {
			cancel()
			<-waitCh
			return nil, fmt.Errorf("vcluster port-forward timeout: %w: %s", err, strings.TrimSpace(stderr.String()))
		}
		time.Sleep(150 * time.Millisecond)
	}
}

func (p *KubectlProvisioner) CreateProjectEnvironment(ctx context.Context, projectID string) (string, error) {
	namespace := namespaceFromProjectID(projectID)
	vclusterName := vclusterNameFromProjectID(projectID)

	log.Printf("[k8s][%s] creating namespace %s", projectID, namespace)
	namespaceManifest := fmt.Sprintf(`apiVersion: v1
kind: Namespace
metadata:
  name: %s
`, namespace)

	if err := p.kubectlApply(ctx, namespaceManifest); err != nil {
		return "", fmt.Errorf("failed to create namespace %s: %w", namespace, err)
	}
	log.Printf("[k8s][%s] namespace ready, creating vcluster %s...", projectID, vclusterName)

	if err := p.createVCluster(ctx, vclusterName, namespace); err != nil {
		return "", fmt.Errorf("failed to create vcluster %s in namespace %s: %w", vclusterName, namespace, err)
	}
	log.Printf("[k8s][%s] vcluster created", projectID)

	secretName := "vc-" + vclusterName
	if _, err := p.waitForProjectKubeconfigSecret(ctx, secretName, namespace); err != nil {
		return "", fmt.Errorf("failed to wait for kubeconfig secret %s in namespace %s: %w", secretName, namespace, err)
	}
	log.Printf("[k8s][%s] kubeconfig secret %s is ready", projectID, secretName)

	networkPolicyManifest := fmt.Sprintf(`apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: default-deny-all
  namespace: %s
spec:
  podSelector: {}
  policyTypes:
    - Ingress
    - Egress
`, namespace)

	if err := p.kubectlApply(ctx, networkPolicyManifest); err != nil {
		return "", fmt.Errorf("failed to apply network policy in namespace %s: %w", namespace, err)
	}
	log.Printf("[k8s][%s] default-deny network policy applied", projectID)

	vclusterAllowPolicy := fmt.Sprintf(`apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-vcluster-control-plane
  namespace: %s
spec:
  podSelector:
    matchLabels:
      app: vcluster
  ingress:
    - {}
  egress:
    - {}
  policyTypes:
    - Ingress
    - Egress
`, namespace)

	if err := p.kubectlApply(ctx, vclusterAllowPolicy); err != nil {
		return "", fmt.Errorf("failed to apply vcluster allow policy in namespace %s: %w", namespace, err)
	}
	log.Printf("[k8s][%s] vcluster network policy applied", projectID)

	if err := p.applyResourceQuota(ctx, namespace); err != nil {
		return "", fmt.Errorf("failed to apply resource quota in namespace %s: %w", namespace, err)
	}
	log.Printf("[k8s][%s] resource quota applied", projectID)

	gatewayControllerNS := strings.TrimSpace(p.gatewayControllerNamespace)
	if gatewayControllerNS == "" {
		gatewayControllerNS = "nginx-gateway"
	}
	gatewayAllowPolicy := fmt.Sprintf(`apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-gateway-controller
  namespace: %s
spec:
  podSelector: {}
  ingress:
    - from:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: %s
`, namespace, gatewayControllerNS)
	if err := p.kubectlApply(ctx, gatewayAllowPolicy); err != nil {
		return "", fmt.Errorf("failed to apply gateway allow policy in namespace %s: %w", namespace, err)
	}
	log.Printf("[k8s][%s] gateway network policy applied", projectID)

	// Если глобальный APPS_BASE_DOMAIN не задан и автоустановка включена, ставим nginx
	// ingress controller внутри vcluster, ждем его LoadBalancer IP и используем
	// "{ip}.nip.io" как apps base domain для проекта.
	projectAppsBaseDomain := p.appsBaseDomain
	if p.ingressNginxAutoInstall && strings.TrimSpace(p.appsBaseDomain) == "" {
		log.Printf("[k8s][%s] installing nginx ingress controller in vcluster...", projectID)
		if err := p.installIngressNginxInVCluster(ctx, projectID); err != nil {
			return "", fmt.Errorf("failed to install nginx ingress in vcluster for project %s: %w", projectID, err)
		}
		log.Printf("[k8s][%s] nginx installed, waiting for LoadBalancer IP...", projectID)
		ip, err := p.waitForIngressNginxIP(ctx, projectID)
		if err != nil {
			return "", fmt.Errorf("failed to get nginx ingress IP for project %s: %w", projectID, err)
		}
		projectAppsBaseDomain = ip + ".nip.io"
		log.Printf("[k8s][%s] nginx LoadBalancer IP: %s, appsBaseDomain: %s", projectID, ip, projectAppsBaseDomain)
	}

	if err := p.applyProjectGrafana(ctx, projectID, namespace, projectAppsBaseDomain); err != nil {
		return "", fmt.Errorf("failed to provision project grafana in namespace %s: %w", namespace, err)
	}
	log.Printf("[k8s][%s] environment ready", projectID)

	return projectAppsBaseDomain, nil
}

func (p *KubectlProvisioner) applyResourceQuota(ctx context.Context, namespace string) error {
	manifest := fmt.Sprintf(`apiVersion: v1
kind: ResourceQuota
metadata:
  name: project-quota
  namespace: %s
spec:
  hard:
    requests.cpu:    "4"
    requests.memory: "8Gi"
    limits.cpu:      "16"
    limits.memory:   "32Gi"
    pods:            "50"
---
apiVersion: v1
kind: LimitRange
metadata:
  name: project-default-limits
  namespace: %s
spec:
  limits:
    - type: Container
      default:
        cpu: "1"
        memory: "512Mi"
      defaultRequest:
        cpu: "100m"
        memory: "128Mi"
`, namespace, namespace)
	return p.kubectlApply(ctx, manifest)
}

func (p *KubectlProvisioner) SuspendProjectEnvironment(ctx context.Context, projectID string) error {
	name := vclusterNameFromProjectID(projectID)
	namespace := namespaceFromProjectID(projectID)
	_, err := p.runVCluster(ctx, []string{"pause", name, "--namespace", namespace})
	return err
}

func (p *KubectlProvisioner) ResumeProjectEnvironment(ctx context.Context, projectID string) error {
	name := vclusterNameFromProjectID(projectID)
	namespace := namespaceFromProjectID(projectID)
	_, err := p.runVCluster(ctx, []string{"resume", name, "--namespace", namespace})
	return err
}

func (p *KubectlProvisioner) GetProjectKubeconfig(ctx context.Context, projectID string) (string, error) {
	kubeconfig, err := p.getProjectKubeconfigRaw(ctx, projectID)
	if err != nil {
		return "", err
	}

	name := vclusterNameFromProjectID(projectID)
	namespace := namespaceFromProjectID(projectID)
	if kubeconfigUsesLocalhostServer(kubeconfig) {
		lbIP, err := p.ensureVClusterExternalIP(ctx, name, namespace)
		if err != nil {
			return "", fmt.Errorf("ensure external endpoint for project %s: %w", projectID, err)
		}
		kubeconfig = rewriteKubeconfigLocalhostServer(kubeconfig, lbIP+":443")
	}

	return kubeconfig, nil
}

func (p *KubectlProvisioner) getProjectKubeconfigRaw(ctx context.Context, projectID string) (string, error) {
	name := vclusterNameFromProjectID(projectID)
	namespace := namespaceFromProjectID(projectID)
	// vcluster записывает kubeconfig в secret vc-<name> в хост-namespace.
	// Чтение секрета мгновенное и не требует сетевого доступа к API vcluster.
	secretName := "vc-" + name
	out, err := p.readProjectKubeconfigSecret(ctx, secretName, namespace)
	if err != nil && isNamespaceNotFoundError(err, namespace) {
		log.Printf("[k8s][%s] host namespace %s not found while reading kubeconfig secret, attempting reconcile", projectID, namespace)
		if reconcileErr := p.reconcileProjectNamespace(ctx, namespace); reconcileErr != nil {
			return "", fmt.Errorf("get kubeconfig secret for project %s: %w", projectID, reconcileErr)
		}
		out, err = p.readProjectKubeconfigSecret(ctx, secretName, namespace)
	}
	if err != nil && isSecretNotFoundError(err, secretName) {
		log.Printf("[k8s][%s] kubeconfig secret %s is not ready yet, waiting with retries", projectID, secretName)
		out, err = p.waitForProjectKubeconfigSecret(ctx, secretName, namespace)
	}
	if err != nil {
		return "", fmt.Errorf("get kubeconfig secret for project %s: %w", projectID, err)
	}
	decoded, err := base64.StdEncoding.DecodeString(out)
	if err != nil {
		return "", fmt.Errorf("decode kubeconfig for project %s: %w", projectID, err)
	}
	return string(decoded), nil
}

func (p *KubectlProvisioner) readProjectKubeconfigSecret(ctx context.Context, secretName, namespace string) (string, error) {
	return p.runKubectl(ctx, []string{
		"get", "secret", secretName,
		"-n", namespace,
		"-o", "jsonpath={.data.config}",
	}, nil)
}

func (p *KubectlProvisioner) waitForProjectKubeconfigSecret(ctx context.Context, secretName, namespace string) (string, error) {
	sleepFn := p.sleepOverride
	if sleepFn == nil {
		sleepFn = time.Sleep
	}

	var lastErr error
	for attempt := 1; attempt <= kubeconfigSecretReadMaxAttempts; attempt++ {
		out, err := p.readProjectKubeconfigSecret(ctx, secretName, namespace)
		if err == nil {
			return out, nil
		}
		if !isSecretNotFoundError(err, secretName) {
			return "", err
		}
		lastErr = err
		if attempt == kubeconfigSecretReadMaxAttempts {
			break
		}

		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}
		sleepFn(kubeconfigSecretReadRetryDelay)
	}

	return "", fmt.Errorf("kubeconfig secret %q in namespace %q is not ready yet; retry in a few seconds: %w", secretName, namespace, lastErr)
}

func (p *KubectlProvisioner) reconcileProjectNamespace(ctx context.Context, namespace string) error {
	namespaceManifest := fmt.Sprintf(`apiVersion: v1
kind: Namespace
metadata:
  name: %s
`, namespace)
	if err := p.kubectlApply(ctx, namespaceManifest); err != nil {
		return fmt.Errorf("reconcile missing namespace %s: %w", namespace, err)
	}
	return nil
}

func isNamespaceNotFoundError(err error, namespace string) bool {
	lower := strings.ToLower(err.Error())
	namespaceLower := strings.ToLower(namespace)
	return strings.Contains(lower, `namespaces "`+namespaceLower+`" not found`) ||
		strings.Contains(lower, `namespace "`+namespaceLower+`" not found`)
}

func isSecretNotFoundError(err error, secretName string) bool {
	lower := strings.ToLower(err.Error())
	secretLower := strings.ToLower(secretName)
	return strings.Contains(lower, `secrets "`+secretLower+`" not found`) ||
		strings.Contains(lower, `secret "`+secretLower+`" not found`)
}

func (p *KubectlProvisioner) getVClusterExternalIP(ctx context.Context, name, namespace string) (string, error) {
	out, err := p.runKubectl(ctx, []string{
		"get", "svc", name,
		"-n", namespace,
		"-o", "jsonpath={.status.loadBalancer.ingress[0].ip}",
	}, nil)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func (p *KubectlProvisioner) ensureVClusterExternalIP(ctx context.Context, name, namespace string) (string, error) {
	currentIP, err := p.getVClusterExternalIP(ctx, name, namespace)
	if err != nil {
		return "", err
	}
	if currentIP != "" {
		return currentIP, nil
	}

	_, err = p.runKubectl(ctx, []string{
		"patch", "svc", name,
		"-n", namespace,
		"-p", `{"spec":{"type":"LoadBalancer"}}`,
	}, nil)
	if err != nil {
		return "", fmt.Errorf("patch vcluster service %s/%s to LoadBalancer: %w", namespace, name, err)
	}

	sleepFn := p.sleepOverride
	if sleepFn == nil {
		sleepFn = time.Sleep
	}

	var lastErr error
	for attempt := 1; attempt <= vclusterExternalIPMaxAttempts; attempt++ {
		ip, getErr := p.getVClusterExternalIP(ctx, name, namespace)
		if getErr == nil && ip != "" {
			return ip, nil
		}
		if getErr != nil {
			lastErr = getErr
		}
		if attempt == vclusterExternalIPMaxAttempts {
			break
		}

		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}
		sleepFn(vclusterExternalIPRetryDelay)
	}

	if lastErr != nil {
		return "", lastErr
	}
	return "", fmt.Errorf("external IP for vcluster service %s/%s is still pending; check quota ylb.networkLoadBalancers.count", namespace, name)
}

func (p *KubectlProvisioner) ApplyImage(ctx context.Context, projectID string, imageTag string) error {
	namespace := namespaceFromProjectID(projectID)
	patch := fmt.Sprintf(`[{"op":"replace","path":"/spec/template/spec/containers/0/image","value":"%s"}]`, imageTag)
	_, err := p.runKubectl(ctx, []string{
		"patch", "deployment", "-n", namespace,
		"--type=json", "-p", patch,
		"--all",
	}, nil)
	return err
}

func (p *KubectlProvisioner) DeleteProjectEnvironment(ctx context.Context, projectID string) error {
	namespace := namespaceFromProjectID(projectID)
	vclusterName := vclusterNameFromProjectID(projectID)

	if err := p.deleteVCluster(ctx, vclusterName, namespace); err != nil {
		return fmt.Errorf("failed to delete vcluster %s in namespace %s: %w", vclusterName, namespace, err)
	}

	args := []string{"delete", "namespace", namespace, "--ignore-not-found=true", "--wait=false"}
	if _, err := p.runKubectl(ctx, args, nil); err != nil {
		return fmt.Errorf("failed to cleanup namespace %s after vcluster deletion: %w", namespace, err)
	}

	return nil
}

func (p *KubectlProvisioner) GetProjectRuntimeStatus(ctx context.Context, projectID string) (domain.ProjectRuntimeStatus, error) {
	namespace := namespaceFromProjectID(projectID)
	status := domain.ProjectRuntimeStatus{
		ProjectID:     projectID,
		Namespace:     namespace,
		LastCheckedAt: time.Now().UTC(),
	}

	if _, err := p.runKubectl(ctx, []string{"get", "namespace", namespace, "-o", "json"}, nil); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "notfound") || strings.Contains(strings.ToLower(err.Error()), "not found") {
			status.Message = "namespace not found, deployment has not reached the cluster yet"
			return status, nil
		}
		return domain.ProjectRuntimeStatus{}, err
	}
	status.NamespaceExists = true

	deploymentsJSON, err := p.runKubectl(ctx, []string{"get", "deployment", "-n", namespace, "-o", "json"}, nil)
	if err != nil {
		return domain.ProjectRuntimeStatus{}, err
	}
	var deployments kubectlDeploymentList
	if err := json.Unmarshal([]byte(deploymentsJSON), &deployments); err != nil {
		return domain.ProjectRuntimeStatus{}, fmt.Errorf("decode deployments: %w", err)
	}
	if len(deployments.Items) > 0 {
		status.DeploymentExists = true
		for _, item := range deployments.Items {
			status.DesiredReplicas += item.Status.Replicas
			status.ReadyReplicas += item.Status.ReadyReplicas
			status.AvailableReplicas += item.Status.AvailableReplicas
		}
	}

	servicesJSON, err := p.runKubectl(ctx, []string{"get", "service", "-n", namespace, "-o", "json"}, nil)
	if err != nil {
		return domain.ProjectRuntimeStatus{}, err
	}
	var services kubectlServiceList
	if err := json.Unmarshal([]byte(servicesJSON), &services); err != nil {
		return domain.ProjectRuntimeStatus{}, fmt.Errorf("decode services: %w", err)
	}
	for _, item := range services.Items {
		if item.Metadata.Name != "kubernetes" {
			status.ServiceExists = true
			break
		}
	}
	status.PublicURL = detectPublicURLFromServices(services)

	ingressesHostJSON, err := p.runKubectl(ctx, []string{"get", "ingress", "-n", namespace, "-o", "json"}, nil)
	if err != nil {
		return domain.ProjectRuntimeStatus{}, err
	}
	var hostIngresses kubectlIngressList
	if err := json.Unmarshal([]byte(ingressesHostJSON), &hostIngresses); err != nil {
		return domain.ProjectRuntimeStatus{}, fmt.Errorf("decode ingresses: %w", err)
	}
	status.HTTPRouteExists = len(hostIngresses.Items) > 0
	if status.PublicURL == "" {
		status.PublicURL = detectPublicURLFromIngresses(hostIngresses)
	}
	status.ServiceURLs = detectAllURLsFromIngresses(hostIngresses)

	podsJSON, err := p.runKubectl(ctx, []string{"get", "pods", "-n", namespace, "-o", "json"}, nil)
	if err != nil {
		return domain.ProjectRuntimeStatus{}, err
	}
	var pods kubectlPodList
	if err := json.Unmarshal([]byte(podsJSON), &pods); err != nil {
		return domain.ProjectRuntimeStatus{}, fmt.Errorf("decode pods: %w", err)
	}
	status.Pods = make([]domain.ProjectPodStatus, 0, len(pods.Items))
	for _, item := range pods.Items {
		podStatus := domain.ProjectPodStatus{
			Name:  item.Metadata.Name,
			Phase: item.Status.Phase,
		}
		for _, container := range item.Status.ContainerStatuses {
			podStatus.Restarts += container.RestartCount
			if container.Ready {
				podStatus.Ready = true
			}
		}
		status.Pods = append(status.Pods, podStatus)
	}

	switch {
	case !status.DeploymentExists:
		status.Message = "namespace exists, but application manifests have not been applied yet"
	case status.ReadyReplicas > 0 && !status.HTTPRouteExists:
		status.Message = "application has ready replicas, but Ingress was not found"
	case status.ReadyReplicas > 0:
		status.Message = "application is deployed and has ready replicas"
	default:
		status.Message = "deployment exists, but pods are still starting or unhealthy"
	}

	return status, nil
}

func (p *KubectlProvisioner) EnsureProjectGrafana(ctx context.Context, projectID string) error {
	if strings.TrimSpace(p.appsBaseDomain) == "" {
		return nil
	}
	return p.applyProjectGrafana(ctx, projectID, namespaceFromProjectID(projectID), p.appsBaseDomain)
}

func (p *KubectlProvisioner) EnsureProjectPublicIngress(ctx context.Context, projectID string) error {
	if strings.TrimSpace(p.appsBaseDomain) == "" {
		return nil
	}

	ingressesJSON, err := p.runKubectlInVCluster(ctx, projectID, []string{"get", "ingress", "-n", "production", "-o", "json"}, nil)
	if err != nil {
		return err
	}

	var ingresses kubectlIngressList
	if err := json.Unmarshal([]byte(ingressesJSON), &ingresses); err != nil {
		return fmt.Errorf("decode ingresses: %w", err)
	}

	expectedHost := fmt.Sprintf("%s.%s", projectID, p.appsBaseDomain)
	for _, item := range ingresses.Items {
		if len(item.Spec.Rules) == 0 {
			continue
		}
		if strings.TrimSpace(item.Spec.Rules[0].Host) == expectedHost {
			continue
		}

		patch := fmt.Sprintf(`{"spec":{"rules":[{"host":"%s","http":%s}]}}`, expectedHost, item.Spec.Rules[0].HTTPRaw)
		if _, err := p.runKubectlInVCluster(ctx, projectID, []string{
			"patch", "ingress", item.Metadata.Name, "-n", "production",
			"--type=merge", "-p", patch,
		}, nil); err != nil {
			return fmt.Errorf("patch ingress production/%s host: %w", item.Metadata.Name, err)
		}
	}

	return nil
}

// NamespaceForProject возвращает имя namespace Kubernetes для заданного ID проекта.
// Используется GitHub-адаптером автоматизации, чтобы подставлять правильный namespace в манифестах.
func NamespaceForProject(projectID string) string {
	return namespaceFromProjectID(projectID)
}

// runKubectlInVCluster запускает kubectl с kubeconfig vcluster, чтобы
// команды выполнялись внутри виртуального кластера (а не хост-кластера).
func (p *KubectlProvisioner) runKubectlInVCluster(ctx context.Context, projectID string, args []string, stdin []byte) (string, error) {
	if p.runKubectlInVClusterOverride != nil {
		return p.runKubectlInVClusterOverride(ctx, projectID, args, stdin)
	}

	kubeconfig, err := p.getProjectKubeconfigRaw(ctx, projectID)
	if err != nil {
		return "", fmt.Errorf("get vcluster kubeconfig for project %s: %w", projectID, err)
	}
	stopPortForward := func() {}
	if kubeconfigUsesLocalhostServer(kubeconfig) || kubeconfigUsesClusterServiceServer(kubeconfig) {
		localPort, portErr := reserveLocalPort()
		if portErr != nil {
			return "", fmt.Errorf("reserve local port for vcluster kubeconfig: %w", portErr)
		}
		stop, pfErr := p.startVClusterPortForward(
			ctx,
			namespaceFromProjectID(projectID),
			vclusterNameFromProjectID(projectID),
			localPort,
		)
		if pfErr != nil {
			return "", fmt.Errorf("start port-forward for project %s: %w", projectID, pfErr)
		}
		stopPortForward = stop
		kubeconfig = rewriteKubeconfigServerForPortForward(kubeconfig, localPort)
	}
	defer stopPortForward()

	tmpFile, err := os.CreateTemp("", "vcluster-kubeconfig-*.yaml")
	if err != nil {
		return "", fmt.Errorf("create temp kubeconfig file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(kubeconfig); err != nil {
		tmpFile.Close()
		return "", fmt.Errorf("write temp kubeconfig: %w", err)
	}
	tmpFile.Close()

	fullArgs := append([]string{"--kubeconfig", tmpFile.Name()}, args...)
	command := exec.CommandContext(ctx, p.kubectlPath, fullArgs...)
	if stdin != nil {
		command.Stdin = bytes.NewReader(stdin)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr

	if err := command.Run(); err != nil {
		return "", fmt.Errorf("kubectl (vcluster) %s failed: %w: %s", strings.Join(fullArgs, " "), err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

func (p *KubectlProvisioner) CreateStageEnvironment(ctx context.Context, projectID, stageSlug string) error {
	log.Printf("[k8s][%s] creating stage namespace %s in vcluster", projectID, stageSlug)
	manifest := fmt.Sprintf("apiVersion: v1\nkind: Namespace\nmetadata:\n  name: %s\n", stageSlug)
	_, err := p.runKubectlInVCluster(ctx, projectID, []string{"apply", "-f", "-", "--validate=false"}, []byte(manifest))
	return err
}

func (p *KubectlProvisioner) DeleteStageEnvironment(ctx context.Context, projectID, stageSlug string) error {
	log.Printf("[k8s][%s] deleting stage namespace %s from vcluster", projectID, stageSlug)
	_, err := p.runKubectlInVCluster(ctx, projectID, []string{"delete", "namespace", stageSlug, "--ignore-not-found=true"}, nil)
	return err
}

func (p *KubectlProvisioner) ApplyImageToStage(ctx context.Context, projectID, stageSlug, imageTag string) error {
	patch := fmt.Sprintf(`[{"op":"replace","path":"/spec/template/spec/containers/0/image","value":"%s"}]`, imageTag)
	_, err := p.runKubectlInVCluster(ctx, projectID, []string{
		"patch", "deployment", "-n", stageSlug,
		"--type=json", "-p", patch,
		"--all",
	}, nil)
	return err
}

func (p *KubectlProvisioner) GetStageRuntimeStatus(ctx context.Context, projectID, stageSlug string) (domain.ProjectRuntimeStatus, error) {
	status := domain.ProjectRuntimeStatus{
		ProjectID:     projectID,
		Namespace:     stageSlug,
		LastCheckedAt: time.Now().UTC(),
	}

	deploymentsJSON, err := p.runKubectlInVCluster(ctx, projectID, []string{"get", "deployment", "-n", stageSlug, "-o", "json"}, nil)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "notfound") || strings.Contains(strings.ToLower(err.Error()), "not found") {
			status.Message = "stage namespace not found"
			return status, nil
		}
		return domain.ProjectRuntimeStatus{}, err
	}
	status.NamespaceExists = true

	var deployments kubectlDeploymentList
	if err := json.Unmarshal([]byte(deploymentsJSON), &deployments); err != nil {
		return domain.ProjectRuntimeStatus{}, fmt.Errorf("decode deployments: %w", err)
	}
	if len(deployments.Items) > 0 {
		status.DeploymentExists = true
		for _, item := range deployments.Items {
			status.DesiredReplicas += item.Status.Replicas
			status.ReadyReplicas += item.Status.ReadyReplicas
			status.AvailableReplicas += item.Status.AvailableReplicas
		}
	}

	servicesJSON, err := p.runKubectlInVCluster(ctx, projectID, []string{"get", "service", "-n", stageSlug, "-o", "json"}, nil)
	if err != nil {
		return domain.ProjectRuntimeStatus{}, err
	}
	var services kubectlServiceList
	if err := json.Unmarshal([]byte(servicesJSON), &services); err != nil {
		return domain.ProjectRuntimeStatus{}, fmt.Errorf("decode services: %w", err)
	}
	for _, item := range services.Items {
		if item.Metadata.Name != "kubernetes" {
			status.ServiceExists = true
			break
		}
	}
	status.PublicURL = detectPublicURLFromServices(services)

	ingressesJSON, err := p.runKubectlInVCluster(ctx, projectID, []string{"get", "ingress", "-n", stageSlug, "-o", "json"}, nil)
	if err != nil {
		return domain.ProjectRuntimeStatus{}, err
	}
	var ingresses kubectlIngressList
	if err := json.Unmarshal([]byte(ingressesJSON), &ingresses); err != nil {
		return domain.ProjectRuntimeStatus{}, fmt.Errorf("decode ingresses: %w", err)
	}
	status.HTTPRouteExists = len(ingresses.Items) > 0
	if status.PublicURL == "" {
		status.PublicURL = detectPublicURLFromIngresses(ingresses)
	}
	status.ServiceURLs = detectAllURLsFromIngresses(ingresses)

	podsJSON, err := p.runKubectlInVCluster(ctx, projectID, []string{"get", "pods", "-n", stageSlug, "-o", "json"}, nil)
	if err != nil {
		return domain.ProjectRuntimeStatus{}, err
	}
	var pods kubectlPodList
	if err := json.Unmarshal([]byte(podsJSON), &pods); err != nil {
		return domain.ProjectRuntimeStatus{}, fmt.Errorf("decode pods: %w", err)
	}
	status.Pods = make([]domain.ProjectPodStatus, 0, len(pods.Items))
	for _, item := range pods.Items {
		podStatus := domain.ProjectPodStatus{
			Name:  item.Metadata.Name,
			Phase: item.Status.Phase,
		}
		for _, container := range item.Status.ContainerStatuses {
			podStatus.Restarts += container.RestartCount
			if container.Ready {
				podStatus.Ready = true
			}
		}
		status.Pods = append(status.Pods, podStatus)
	}

	switch {
	case !status.DeploymentExists:
		status.Message = "stage namespace exists, application manifests not yet applied"
	case status.ReadyReplicas > 0 && !status.HTTPRouteExists:
		status.Message = "application has ready replicas, but Ingress was not found"
	case status.ReadyReplicas > 0:
		status.Message = "application is deployed and has ready replicas"
	default:
		status.Message = "deployment exists, but pods are still starting"
	}
	return status, nil
}

func (p *KubectlProvisioner) createVCluster(ctx context.Context, vclusterName string, namespace string) error {
	args := []string{
		"create", vclusterName,
		"--namespace", namespace,
		"--driver", "helm",
		"--connect=false",
		"--background-proxy=false",
		"--create-namespace=false",
		"--set", "controlPlane.statefulSet.resources.requests.memory=128Mi",
		"--set", "controlPlane.statefulSet.resources.requests.cpu=50m",
		"--set", "controlPlane.statefulSet.resources.limits.memory=1Gi",
		"--set", "controlPlane.statefulSet.resources.limits.cpu=500m",
		"--set", "controlPlane.service.spec.type=ClusterIP",
		"--set", "sync.toHost.ingresses.enabled=true",
	}

	if _, err := p.runVCluster(ctx, args); err != nil {
		return err
	}

	return nil
}

func (p *KubectlProvisioner) deleteVCluster(ctx context.Context, vclusterName string, namespace string) error {
	args := []string{
		"delete", vclusterName,
		"--namespace", namespace,
		"--driver", "helm",
		"--ignore-not-found",
		"--wait=false",
		"--delete-namespace",
	}

	if _, err := p.runVCluster(ctx, args); err != nil {
		return err
	}

	return nil
}

func (p *KubectlProvisioner) kubectlApply(ctx context.Context, manifest string) error {
	_, err := p.runKubectl(ctx, []string{"apply", "-f", "-", "--validate=false"}, []byte(manifest))
	return err
}

func (p *KubectlProvisioner) runKubectl(ctx context.Context, args []string, stdin []byte) (string, error) {
	if p.runKubectlOverride != nil {
		return p.runKubectlOverride(ctx, args, stdin)
	}

	fullArgs := make([]string, 0, len(args)+2)
	if p.contextName != "" {
		fullArgs = append(fullArgs, "--context", p.contextName)
	}
	fullArgs = append(fullArgs, args...)

	command := exec.CommandContext(ctx, p.kubectlPath, fullArgs...)
	if stdin != nil {
		command.Stdin = bytes.NewReader(stdin)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr

	if err := command.Run(); err != nil {
		return "", fmt.Errorf("kubectl %s failed: %w: %s", strings.Join(fullArgs, " "), err, strings.TrimSpace(stderr.String()))
	}

	return strings.TrimSpace(stdout.String()), nil
}

func isGatewayAPIUnavailableError(err error) bool {
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "the server doesn't have a resource type") ||
		strings.Contains(lower, "no matches for kind") ||
		strings.Contains(lower, "could not find the requested resource")
}

func (p *KubectlProvisioner) runVCluster(ctx context.Context, args []string) (string, error) {
	fullArgs := make([]string, 0, len(args)+2)
	if p.contextName != "" {
		fullArgs = append(fullArgs, "--context", p.contextName)
	}
	fullArgs = append(fullArgs, args...)

	command := exec.CommandContext(ctx, p.vclusterPath, fullArgs...)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr

	if err := command.Run(); err != nil {
		return "", fmt.Errorf("vcluster %s failed: %w: %s", strings.Join(fullArgs, " "), err, strings.TrimSpace(stderr.String()))
	}

	return strings.TrimSpace(stdout.String()), nil
}

func (p *KubectlProvisioner) applyProjectGrafana(ctx context.Context, projectID, namespace, appsBaseDomain string) error {
	if strings.TrimSpace(appsBaseDomain) == "" {
		return nil
	}

	if err := p.kubectlApply(ctx, p.projectGrafanaManifest(projectID, namespace, appsBaseDomain)); err != nil {
		return err
	}

	_, err := p.runKubectl(ctx, []string{
		"-n", namespace,
		"rollout", "status", "deployment/project-grafana",
		"--timeout=180s",
	}, nil)
	return err
}

func (p *KubectlProvisioner) installIngressNginxInVCluster(ctx context.Context, projectID string) error {
	manifestURL := p.ingressNginxManifestURL
	if manifestURL == "" {
		manifestURL = defaultIngressNginxManifestURL
	}
	if _, err := p.runKubectlInVCluster(ctx, projectID, []string{"apply", "-f", manifestURL}, nil); err != nil {
		return err
	}
	// Ждем готовности deployment admission webhook, чтобы последующие apply
	// Ingress в CI не падали с "connection refused" на webhook.
	_, err := p.runKubectlInVCluster(ctx, projectID, []string{
		"rollout", "status", "deployment/ingress-nginx-controller",
		"-n", "ingress-nginx", "--timeout=300s",
	}, nil)
	return err
}

func (p *KubectlProvisioner) waitForIngressNginxIP(ctx context.Context, projectID string) (string, error) {
	sleepFn := p.sleepOverride
	if sleepFn == nil {
		sleepFn = time.Sleep
	}

	for attempt := 1; attempt <= ingressNginxIPMaxAttempts; attempt++ {
		out, err := p.runKubectlInVCluster(ctx, projectID, []string{
			"get", "svc", "ingress-nginx-controller",
			"-n", "ingress-nginx",
			"-o", "jsonpath={.status.loadBalancer.ingress[0].ip}",
		}, nil)
		if err == nil {
			ip := strings.TrimSpace(out)
			if net.ParseIP(ip) != nil {
				return ip, nil
			}
		}
		if attempt == ingressNginxIPMaxAttempts {
			break
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}
		sleepFn(ingressNginxIPRetryDelay)
	}
	return "", fmt.Errorf("nginx ingress LoadBalancer IP for project %s is still pending after %d attempts", projectID, ingressNginxIPMaxAttempts)
}

func (p *KubectlProvisioner) projectGrafanaManifest(projectID, namespace, appsBaseDomain string) string {
	host := projectGrafanaHost(projectID, appsBaseDomain)
	monitoringNamespace := strings.TrimSpace(p.monitoringNamespace)
	if monitoringNamespace == "" {
		monitoringNamespace = "monitoring"
	}
	prometheusURL := strings.TrimSpace(p.prometheusBaseURL)
	if prometheusURL == "" {
		prometheusURL = fmt.Sprintf("http://prometheus.%s.svc.cluster.local:9090", monitoringNamespace)
	}
	lokiURL := strings.TrimSpace(p.lokiBaseURL)
	if lokiURL == "" {
		lokiURL = fmt.Sprintf("http://loki.%s.svc.cluster.local:3100", monitoringNamespace)
	}
	dashboardJSON := indentYAMLBlock(buildProjectGrafanaDashboard(projectID, namespace), "    ")

	return fmt.Sprintf(`apiVersion: v1
kind: ConfigMap
metadata:
  name: project-grafana-datasource
  namespace: %[1]s
data:
  datasources.yaml: |
    apiVersion: 1
    datasources:
      - name: Prometheus
        type: prometheus
        access: proxy
        url: %[2]s
        isDefault: true
      - name: Loki
        type: loki
        access: proxy
        url: %[7]s
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: project-grafana-dashboard-provider
  namespace: %[1]s
data:
  dashboards.yaml: |
    apiVersion: 1
    providers:
      - name: project
        orgId: 1
        folder: Project
        type: file
        disableDeletion: false
        updateIntervalSeconds: 30
        options:
          path: /var/lib/grafana/dashboards/project
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: project-grafana-dashboard
  namespace: %[1]s
data:
  project-overview.json: |
%[6]s
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: project-grafana
  namespace: %[1]s
  annotations:
    nginx.ingress.kubernetes.io/force-ssl-redirect: "true"
spec:
  replicas: 1
  selector:
    matchLabels:
      app: project-grafana
  template:
    metadata:
      labels:
        app: project-grafana
    spec:
      containers:
        - name: grafana
          image: grafana/grafana:11.1.4
          ports:
            - containerPort: 3000
          env:
            - name: GF_AUTH_ANONYMOUS_ENABLED
              value: "true"
            - name: GF_AUTH_ANONYMOUS_ORG_ROLE
              value: "Viewer"
            - name: GF_AUTH_DISABLE_LOGIN_FORM
              value: "true"
            - name: GF_USERS_ALLOW_SIGN_UP
              value: "false"
            - name: GF_SECURITY_ALLOW_EMBEDDING
              value: "true"
          resources:
            requests:
              cpu: 25m
              memory: 128Mi
            limits:
              cpu: 250m
              memory: 256Mi
          volumeMounts:
            - name: datasource
              mountPath: /etc/grafana/provisioning/datasources
            - name: dashboard-provider
              mountPath: /etc/grafana/provisioning/dashboards
            - name: dashboards
              mountPath: /var/lib/grafana/dashboards/project
          readinessProbe:
            httpGet:
              path: /api/health
              port: 3000
            initialDelaySeconds: 10
            periodSeconds: 10
          livenessProbe:
            httpGet:
              path: /api/health
              port: 3000
            initialDelaySeconds: 20
            periodSeconds: 20
      volumes:
        - name: datasource
          configMap:
            name: project-grafana-datasource
        - name: dashboard-provider
          configMap:
            name: project-grafana-dashboard-provider
        - name: dashboards
          configMap:
            name: project-grafana-dashboard
---
apiVersion: v1
kind: Service
metadata:
  name: project-grafana
  namespace: %[1]s
spec:
  selector:
    app: project-grafana
  ports:
    - name: http
      port: 3000
      targetPort: 3000
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: project-grafana
  namespace: %[1]s
spec:
  ingressClassName: nginx
  rules:
    - host: "%[4]s"
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: project-grafana
                port:
                  number: 3000
---
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-project-grafana-egress
  namespace: %[1]s
spec:
  podSelector:
    matchLabels:
      app: project-grafana
  policyTypes:
    - Egress
  egress:
    - to:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: %[5]s
      ports:
        - protocol: TCP
          port: 9090
        - protocol: TCP
          port: 3100
    - to:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: kube-system
      ports:
        - protocol: UDP
          port: 53
        - protocol: TCP
          port: 53
`, namespace, prometheusURL, projectID, host, monitoringNamespace, dashboardJSON, lokiURL)
}

func buildProjectGrafanaDashboard(projectID, namespace string) string {
	dashboard := map[string]any{
		"title":         "Project metrics",
		"uid":           domain.ProjectGrafanaDashboardUID(projectID),
		"timezone":      "browser",
		"schemaVersion": 39,
		"version":       1,
		"refresh":       "30s",
		"panels": []map[string]any{
			{
				"type":    "timeseries",
				"title":   "Project CPU cores",
				"gridPos": map[string]any{"h": 8, "w": 12, "x": 0, "y": 0},
				"targets": []map[string]any{
					{
						"expr":         fmt.Sprintf(`sum(rate(container_cpu_usage_seconds_total{namespace="%s",container!="",image!=""}[5m]))`, namespace),
						"legendFormat": "project total",
					},
				},
			},
			{
				"type":    "timeseries",
				"title":   "Project memory GB",
				"gridPos": map[string]any{"h": 8, "w": 12, "x": 12, "y": 0},
				"targets": []map[string]any{
					{
						"expr":         fmt.Sprintf(`sum(container_memory_working_set_bytes{namespace="%s",container!="",image!=""}) / 1024 / 1024 / 1024`, namespace),
						"legendFormat": "project total",
					},
				},
			},
			{
				"type":    "timeseries",
				"title":   "CPU cores by worker node",
				"gridPos": map[string]any{"h": 8, "w": 12, "x": 0, "y": 8},
				"targets": []map[string]any{
					{
						"expr":         fmt.Sprintf(`sum by (kubernetes_io_hostname) (rate(container_cpu_usage_seconds_total{namespace="%s",container!="",image!=""}[5m]))`, namespace),
						"legendFormat": "worker {{kubernetes_io_hostname}}",
					},
				},
			},
			{
				"type":    "timeseries",
				"title":   "Memory GB by worker node",
				"gridPos": map[string]any{"h": 8, "w": 12, "x": 12, "y": 8},
				"targets": []map[string]any{
					{
						"expr":         fmt.Sprintf(`sum by (kubernetes_io_hostname) (container_memory_working_set_bytes{namespace="%s",container!="",image!=""}) / 1024 / 1024 / 1024`, namespace),
						"legendFormat": "worker {{kubernetes_io_hostname}}",
					},
				},
			},
			{
				"type":    "timeseries",
				"title":   "CPU limit utilization %",
				"gridPos": map[string]any{"h": 8, "w": 12, "x": 0, "y": 16},
				"targets": []map[string]any{
					{
						"expr":         fmt.Sprintf(`100 * sum(rate(container_cpu_usage_seconds_total{namespace="%s",container!="",image!=""}[5m])) / clamp_min(sum(container_spec_cpu_quota{namespace="%s",container!="",image!=""}) / sum(container_spec_cpu_period{namespace="%s",container!="",image!=""}), 0.001)`, namespace, namespace, namespace),
						"legendFormat": "cpu used vs limit",
					},
				},
			},
			{
				"type":    "timeseries",
				"title":   "Memory limit utilization %",
				"gridPos": map[string]any{"h": 8, "w": 12, "x": 12, "y": 16},
				"targets": []map[string]any{
					{
						"expr":         fmt.Sprintf(`100 * sum(container_memory_working_set_bytes{namespace="%s",container!="",image!=""}) / clamp_min(sum(container_spec_memory_limit_bytes{namespace="%s",container!="",image!=""}), 1)`, namespace, namespace),
						"legendFormat": "memory used vs limit",
					},
				},
			},
			{
				"type":    "timeseries",
				"title":   "CPU throttling %",
				"gridPos": map[string]any{"h": 8, "w": 12, "x": 0, "y": 24},
				"targets": []map[string]any{
					{
						"expr":         fmt.Sprintf(`100 * sum(rate(container_cpu_cfs_throttled_periods_total{namespace="%s",container!="",image!=""}[5m])) / clamp_min(sum(rate(container_cpu_cfs_periods_total{namespace="%s",container!="",image!=""}[5m])), 0.001)`, namespace, namespace),
						"legendFormat": "throttled periods",
					},
				},
			},
			{
				"type":    "timeseries",
				"title":   "Active pods by worker node",
				"gridPos": map[string]any{"h": 8, "w": 12, "x": 12, "y": 24},
				"targets": []map[string]any{
					{
						"expr":         fmt.Sprintf(`count by (kubernetes_io_hostname) (count by (pod, kubernetes_io_hostname) (container_cpu_usage_seconds_total{namespace="%s",container!="",image!=""}))`, namespace),
						"legendFormat": "worker {{kubernetes_io_hostname}}",
					},
				},
			},
			{
				"type":    "timeseries",
				"title":   "Project network egress bytes/s",
				"gridPos": map[string]any{"h": 8, "w": 12, "x": 0, "y": 32},
				"targets": []map[string]any{
					{
						"expr":         fmt.Sprintf(`sum(rate(container_network_transmit_bytes_total{namespace="%s"}[5m]))`, namespace),
						"legendFormat": "project total",
					},
				},
			},
			{
				"type":    "timeseries",
				"title":   "Project network ingress bytes/s",
				"gridPos": map[string]any{"h": 8, "w": 12, "x": 12, "y": 32},
				"targets": []map[string]any{
					{
						"expr":         fmt.Sprintf(`sum(rate(container_network_receive_bytes_total{namespace="%s"}[5m]))`, namespace),
						"legendFormat": "project total",
					},
				},
			},
			{
				"type":    "timeseries",
				"title":   "Network egress by worker node",
				"gridPos": map[string]any{"h": 8, "w": 12, "x": 0, "y": 40},
				"targets": []map[string]any{
					{
						"expr":         fmt.Sprintf(`sum by (kubernetes_io_hostname) (rate(container_network_transmit_bytes_total{namespace="%s",pod!=""}[5m]))`, namespace),
						"legendFormat": "worker {{kubernetes_io_hostname}}",
					},
				},
			},
			{
				"type":    "timeseries",
				"title":   "Network ingress by worker node",
				"gridPos": map[string]any{"h": 8, "w": 12, "x": 12, "y": 40},
				"targets": []map[string]any{
					{
						"expr":         fmt.Sprintf(`sum by (kubernetes_io_hostname) (rate(container_network_receive_bytes_total{namespace="%s",pod!=""}[5m]))`, namespace),
						"legendFormat": "worker {{kubernetes_io_hostname}}",
					},
				},
			},
			{
				"type":    "table",
				"title":   "Top pods in project",
				"gridPos": map[string]any{"h": 10, "w": 24, "x": 0, "y": 48},
				"targets": []map[string]any{
					{
						"expr":         fmt.Sprintf(`topk(10, sum by (pod) (rate(container_cpu_usage_seconds_total{namespace="%s",container!="",image!=""}[5m])))`, namespace),
						"legendFormat": "cpu by pod",
						"format":       "table",
						"instant":      true,
					},
					{
						"expr":         fmt.Sprintf(`topk(10, sum by (pod) (container_memory_working_set_bytes{namespace="%s",container!="",image!=""}) / 1024 / 1024 / 1024)`, namespace),
						"legendFormat": "memory gb by pod",
						"format":       "table",
						"instant":      true,
					},
					{
						"expr":         fmt.Sprintf(`max by (pod,kubernetes_io_hostname) (container_cpu_usage_seconds_total{namespace="%s",container!="",image!=""})`, namespace),
						"legendFormat": "worker node by pod",
						"format":       "table",
						"instant":      true,
					},
					{
						"expr":         fmt.Sprintf(`count by (pod) (container_cpu_usage_seconds_total{namespace="%s",container!="",image!=""})`, namespace),
						"legendFormat": "active containers by pod",
						"format":       "table",
						"instant":      true,
					},
				},
			},
		},
	}

	data, err := json.MarshalIndent(dashboard, "", "  ")
	if err != nil {
		return "{}"
	}

	return string(data)
}

func indentYAMLBlock(value, prefix string) string {
	if value == "" {
		return prefix
	}
	return prefix + strings.ReplaceAll(value, "\n", "\n"+prefix)
}

func projectGrafanaHost(projectID, appsBaseDomain string) string {
	return fmt.Sprintf("grafana-%s.%s", projectID, appsBaseDomain)
}

func namespaceFromProjectID(projectID string) string {
	sanitized := strings.ToLower(projectID)
	sanitized = strings.ReplaceAll(sanitized, "_", "-")
	re := regexp.MustCompile(`[^a-z0-9-]`)
	sanitized = re.ReplaceAllString(sanitized, "-")
	sanitized = strings.Trim(sanitized, "-")
	if sanitized == "" {
		sanitized = "project"
	}

	namespace := "project-" + sanitized
	if len(namespace) > 63 {
		namespace = namespace[:63]
		namespace = strings.Trim(namespace, "-")
	}

	return namespace
}

func vclusterNameFromProjectID(projectID string) string {
	sanitized := strings.ToLower(projectID)
	sanitized = strings.ReplaceAll(sanitized, "_", "-")
	re := regexp.MustCompile(`[^a-z0-9-]`)
	sanitized = re.ReplaceAllString(sanitized, "-")
	sanitized = strings.Trim(sanitized, "-")
	if sanitized == "" {
		sanitized = "project"
	}

	name := "vcluster-" + sanitized
	if len(name) > 63 {
		name = name[:63]
		name = strings.Trim(name, "-")
	}

	return name
}

func detectPublicURLFromServices(services kubectlServiceList) string {
	for _, item := range services.Items {
		if !strings.EqualFold(item.Spec.Type, "LoadBalancer") {
			continue
		}
		endpoint := loadBalancerEndpoint(item.Status.LoadBalancer.Ingress)
		if endpoint == "" {
			continue
		}
		port := servicePublicPort(item.Spec.Ports)
		return formatPublicURL(endpoint, port)
	}
	return ""
}

func detectPublicURLFromIngresses(ingresses kubectlIngressList) string {
	for _, item := range ingresses.Items {
		for _, rule := range item.Spec.Rules {
			if host := strings.TrimSpace(rule.Host); host != "" {
				return "https://" + host
			}
		}
	}
	return ""
}

func detectAllURLsFromIngresses(ingresses kubectlIngressList) []domain.ServiceURL {
	var urls []domain.ServiceURL
	for _, item := range ingresses.Items {
		for _, rule := range item.Spec.Rules {
			if host := strings.TrimSpace(rule.Host); host != "" {
				urls = append(urls, domain.ServiceURL{
					Name: item.Metadata.Name,
					URL:  "https://" + host,
				})
				break
			}
		}
	}
	return urls
}

func detectPublicURLFromHTTPRoutes(routes kubectlHTTPRouteList) string {
	for _, item := range routes.Items {
		for _, host := range item.Spec.Hostnames {
			trimmed := strings.TrimSpace(host)
			if trimmed == "" {
				continue
			}
			return "https://" + trimmed
		}
	}
	return ""
}

func loadBalancerEndpoint(ingress []struct {
	IP       string `json:"ip"`
	Hostname string `json:"hostname"`
}) string {
	for _, point := range ingress {
		if ip := strings.TrimSpace(point.IP); ip != "" {
			return ip
		}
		if host := strings.TrimSpace(point.Hostname); host != "" {
			return host
		}
	}
	return ""
}

func servicePublicPort(ports []struct {
	Port int32 `json:"port"`
}) int32 {
	for _, port := range ports {
		if port.Port > 0 {
			return port.Port
		}
	}
	return 0
}

func formatPublicURL(endpoint string, port int32) string {
	switch port {
	case 443:
		return fmt.Sprintf("https://%s", endpoint)
	case 0, 80:
		return fmt.Sprintf("http://%s", endpoint)
	default:
		return fmt.Sprintf("http://%s:%d", endpoint, port)
	}
}
