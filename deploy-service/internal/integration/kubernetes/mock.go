package kubernetes

import (
	"context"
	"os"
	"strings"
	"time"

	"deploy-service/internal/domain"
	"deploy-service/internal/service"
)

// Проверка на этапе компиляции: ProvisionerMock реализует service.Provisioner.
var _ service.Provisioner = (*ProvisionerMock)(nil)

func (p *ProvisionerMock) CreateProjectEnvironment(_ context.Context, _ string) (string, error) {
	return "", nil
}

func (p *ProvisionerMock) DeleteProjectEnvironment(_ context.Context, _ string) error {
	return nil
}

func (p *ProvisionerMock) SuspendProjectEnvironment(_ context.Context, _ string) error {
	return nil
}

func (p *ProvisionerMock) ResumeProjectEnvironment(_ context.Context, _ string) error {
	return nil
}

func (p *ProvisionerMock) ApplyImage(_ context.Context, _ string, _ string) error {
	return nil
}

func (p *ProvisionerMock) GetProjectKubeconfig(_ context.Context, projectID string) (string, error) {
	return "apiVersion: v1\nclusters: []\ncontexts: []\nkind: Config\npreferences: {}\nusers: []\n", nil
}

func (p *ProvisionerMock) GetProjectRuntimeStatus(_ context.Context, projectID string) (domain.ProjectRuntimeStatus, error) {
	return domain.ProjectRuntimeStatus{
		ProjectID:         projectID,
		Namespace:         NamespaceForProject(projectID),
		NamespaceExists:   true,
		DeploymentExists:  true,
		ServiceExists:     true,
		HTTPRouteExists:   true,
		DesiredReplicas:   1,
		ReadyReplicas:     1,
		AvailableReplicas: 1,
		Pods: []domain.ProjectPodStatus{
			{Name: "mock-pod-0", Phase: "Running", Ready: true},
		},
		LastCheckedAt: time.Now().UTC(),
		Message:       "mock deployment is healthy",
	}, nil
}

func (p *ProvisionerMock) CreateStageEnvironment(_ context.Context, _, _ string) error {
	return nil
}

func (p *ProvisionerMock) DeleteStageEnvironment(_ context.Context, _, _ string) error {
	return nil
}

func (p *ProvisionerMock) ApplyImageToStage(_ context.Context, _, _, _ string) error {
	return nil
}

func (p *ProvisionerMock) GetStageRuntimeStatus(_ context.Context, projectID, stageSlug string) (domain.ProjectRuntimeStatus, error) {
	return domain.ProjectRuntimeStatus{
		ProjectID:         projectID,
		Namespace:         stageSlug,
		NamespaceExists:   true,
		DeploymentExists:  true,
		ServiceExists:     true,
		HTTPRouteExists:   true,
		DesiredReplicas:   1,
		ReadyReplicas:     1,
		AvailableReplicas: 1,
		Pods: []domain.ProjectPodStatus{
			{Name: "mock-pod-0", Phase: "Running", Ready: true},
		},
		LastCheckedAt: time.Now().UTC(),
		Message:       "mock deployment is healthy",
	}, nil
}

// NewProvisionerFromEnvironment возвращает реальный или mock Provisioner в зависимости от конфигурации окружения.
func NewProvisionerFromEnvironment() service.Provisioner {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("KUBERNETES_PROVISIONER")))
	if mode == "" || mode == "kubectl" {
		kubectlPath := strings.TrimSpace(os.Getenv("KUBECTL_PATH"))
		if kubectlPath == "" {
			kubectlPath = "kubectl"
		}

		vclusterPath := strings.TrimSpace(os.Getenv("VCLUSTER_PATH"))
		if vclusterPath == "" {
			vclusterPath = "vcluster"
		}

		contextName := strings.TrimSpace(os.Getenv("KUBECTL_CONTEXT"))
		gatewayControllerNamespace := strings.TrimSpace(os.Getenv("GATEWAY_CONTROLLER_NAMESPACE"))
		appsBaseDomain := strings.TrimSpace(os.Getenv("APPS_BASE_DOMAIN"))
		appsURLScheme := strings.TrimSpace(os.Getenv("APPS_PUBLIC_SCHEME"))
		if appsURLScheme == "" {
			appsURLScheme = "https"
		}
		appsURLPort := strings.TrimSpace(os.Getenv("APPS_PUBLIC_PORT"))
		monitoringNamespace := strings.TrimSpace(os.Getenv("MONITORING_NAMESPACE"))
		if monitoringNamespace == "" {
			monitoringNamespace = "monitoring"
		}
		prometheusBaseURL := strings.TrimSpace(os.Getenv("PROMETHEUS_BASE_URL"))
		ingressNginxManifestURL := strings.TrimSpace(os.Getenv("INGRESS_NGINX_MANIFEST_URL"))
		// Автоустановка включена по умолчанию, если глобальный APPS_BASE_DOMAIN не задан.
		ingressNginxAutoInstall := appsBaseDomain == ""
		if v := strings.ToLower(strings.TrimSpace(os.Getenv("INGRESS_NGINX_AUTO_INSTALL"))); v == "false" || v == "0" {
			ingressNginxAutoInstall = false
		} else if v == "true" || v == "1" {
			ingressNginxAutoInstall = true
		}
		return &KubectlProvisioner{
			kubectlPath:                kubectlPath,
			vclusterPath:               vclusterPath,
			contextName:                contextName,
			gatewayControllerNamespace: gatewayControllerNamespace,
			appsBaseDomain:             appsBaseDomain,
			appsURLScheme:              appsURLScheme,
			appsURLPort:                appsURLPort,
			monitoringNamespace:        monitoringNamespace,
			prometheusBaseURL:          prometheusBaseURL,
			ingressNginxAutoInstall:    ingressNginxAutoInstall,
			ingressNginxManifestURL:    ingressNginxManifestURL,
		}
	}

	return &ProvisionerMock{}
}
