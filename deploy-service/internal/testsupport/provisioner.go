package testsupport

import (
	"context"
	"deploy-service/internal/domain"
	"deploy-service/internal/integration/kubernetes"
	"fmt"
	"time"
)

func NewRuntimeProvisioner() *RuntimeProvisioner {
	return &RuntimeProvisioner{
		status:  make(map[string]domain.ProjectRuntimeStatus),
		images:  make(map[string]string),
		paused:  make(map[string]bool),
		deleted: make(map[string]bool),
	}
}

func (p *RuntimeProvisioner) CreateProjectEnvironment(_ context.Context, projectID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.status[projectID] = domain.ProjectRuntimeStatus{
		ProjectID:       projectID,
		Namespace:       kubernetes.NamespaceForProject(projectID),
		NamespaceExists: true,
		LastCheckedAt:   time.Now().UTC(),
		Message:         "namespace created, deployment has not been applied yet",
	}
	return nil
}

func (p *RuntimeProvisioner) DeleteProjectEnvironment(_ context.Context, projectID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.status, projectID)
	p.deleted[projectID] = true
	return nil
}

func (p *RuntimeProvisioner) SuspendProjectEnvironment(_ context.Context, projectID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.paused[projectID] = true
	return nil
}

func (p *RuntimeProvisioner) ResumeProjectEnvironment(_ context.Context, projectID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.paused, projectID)
	return nil
}

func (p *RuntimeProvisioner) ApplyImage(_ context.Context, projectID string, imageTag string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.images[projectID] = imageTag
	return nil
}

func (p *RuntimeProvisioner) GetProjectKubeconfig(_ context.Context, projectID string) (string, error) {
	return fmt.Sprintf("apiVersion: v1\nkind: Config\nclusters:\n- name: %s\n", projectID), nil
}

func (p *RuntimeProvisioner) GetProjectRuntimeStatus(_ context.Context, projectID string) (domain.ProjectRuntimeStatus, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	status, ok := p.status[projectID]
	if !ok {
		return domain.ProjectRuntimeStatus{
			ProjectID:     projectID,
			Namespace:     kubernetes.NamespaceForProject(projectID),
			LastCheckedAt: time.Now().UTC(),
			Message:       "project runtime was not created",
		}, nil
	}
	if p.paused[projectID] {
		status.Message = "project is suspended"
	}
	status.LastCheckedAt = time.Now().UTC()
	return status, nil
}

func (p *RuntimeProvisioner) MarkDeployed(projectID string, serviceName string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.status[projectID] = domain.ProjectRuntimeStatus{
		ProjectID:         projectID,
		Namespace:         kubernetes.NamespaceForProject(projectID),
		NamespaceExists:   true,
		DeploymentExists:  true,
		ServiceExists:     true,
		HTTPRouteExists:   true,
		DesiredReplicas:   1,
		ReadyReplicas:     1,
		AvailableReplicas: 1,
		LastCheckedAt:     time.Now().UTC(),
		Message:           fmt.Sprintf("application %s is deployed and ready", serviceName),
		Pods: []domain.ProjectPodStatus{
			{
				Name:     serviceName + "-pod-0",
				Phase:    "Running",
				Ready:    true,
				Restarts: 0,
			},
		},
	}
}

func SimulateGeneratedDeploy(ctx context.Context, repo *TempGitHubRepo, provisioner *RuntimeProvisioner, branch string, projectID string, serviceName string) error {
	if err := repo.ValidateGeneratedArtifacts(ctx, branch, projectID, serviceName); err != nil {
		return err
	}
	if err := repo.MergeBranchIntoDefault(ctx, branch); err != nil {
		return err
	}
	provisioner.MarkDeployed(projectID, serviceName)
	return nil
}
