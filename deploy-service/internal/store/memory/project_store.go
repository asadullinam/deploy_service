package memory

import (
	"context"
	"deploy-service/internal/domain"
	"deploy-service/internal/service"
	"errors"
)

// Проверка на этапе компиляции: ProjectStore реализует service.ProjectStore.
var _ service.ProjectStore = (*ProjectStore)(nil)

func NewProjectStore() *ProjectStore {
	return &ProjectStore{projects: make(map[string]domain.Project)}
}

func (s *ProjectStore) Create(_ context.Context, project domain.Project) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.projects[project.ID]; exists {
		return errors.New("project already exists")
	}

	s.projects[project.ID] = project
	return nil
}

func (s *ProjectStore) GetByID(_ context.Context, projectID string) (domain.Project, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	project, exists := s.projects[projectID]
	return project, exists
}

func (s *ProjectStore) List(_ context.Context) []domain.Project {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]domain.Project, 0, len(s.projects))
	for _, project := range s.projects {
		result = append(result, project)
	}

	return result
}

func (s *ProjectStore) Update(_ context.Context, project domain.Project) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, exists := s.projects[project.ID]
	if !exists {
		return errors.New("project not found")
	}
	if project.KubeconfigEncrypted == "" {
		project.KubeconfigEncrypted = existing.KubeconfigEncrypted
	}
	if project.RepositoryOwner == "" {
		project.RepositoryOwner = existing.RepositoryOwner
	}
	if project.RepositoryName == "" {
		project.RepositoryName = existing.RepositoryName
	}
	if project.BaseBranch == "" {
		project.BaseBranch = existing.BaseBranch
	}
	if project.ServiceName == "" {
		project.ServiceName = existing.ServiceName
	}
	if project.DockerfilePath == "" {
		project.DockerfilePath = existing.DockerfilePath
	}
	if project.ServiceType == "" {
		project.ServiceType = existing.ServiceType
	}
	if project.GrafanaURL == "" {
		project.GrafanaURL = existing.GrafanaURL
	}
	if project.ServicePort == 0 {
		project.ServicePort = existing.ServicePort
	}
	if project.ContainerPort == 0 {
		project.ContainerPort = existing.ContainerPort
	}
	if project.ReplicaCount == 0 {
		project.ReplicaCount = existing.ReplicaCount
	}
	if project.ResourceProfile == "" {
		project.ResourceProfile = existing.ResourceProfile
	}
	if project.GitHubTokenEncrypted == "" {
		project.GitHubTokenEncrypted = existing.GitHubTokenEncrypted
	}

	s.projects[project.ID] = project
	return nil
}

func (s *ProjectStore) UpdateGitHubToken(_ context.Context, projectID, encryptedToken string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	p, exists := s.projects[projectID]
	if !exists {
		return errors.New("project not found")
	}

	p.GitHubTokenEncrypted = encryptedToken
	s.projects[projectID] = p
	return nil
}

func (s *ProjectStore) UpdateKubeconfig(_ context.Context, projectID, encryptedKubeconfig string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	p, exists := s.projects[projectID]
	if !exists {
		return errors.New("project not found")
	}

	p.KubeconfigEncrypted = encryptedKubeconfig
	s.projects[projectID] = p
	return nil
}
