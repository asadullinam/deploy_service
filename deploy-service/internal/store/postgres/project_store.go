package postgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"deploy-service/internal/domain"
	"deploy-service/internal/service"
)

// Проверка на этапе компиляции: ProjectStore реализует service.ProjectStore.
var _ service.ProjectStore = (*ProjectStore)(nil)

func NewProjectStore(pool *pgxpool.Pool) *ProjectStore {
	return &ProjectStore{pool: pool}
}

func (s *ProjectStore) Create(ctx context.Context, project domain.Project) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO projects (
			id, name, owner_id, status, public_url, grafana_url,
			repository_owner, repository_name, base_branch, service_name, dockerfile_path, service_type, service_port, container_port,
			replica_count, resource_profile, dedicated_load_balancer, apps_base_domain,
			kubeconfig_encrypted, created_at, updated_at
		)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21)`,
		project.ID,
		project.Name,
		project.OwnerID,
		string(project.Status),
		project.PublicURL,
		project.GrafanaURL,
		project.RepositoryOwner,
		project.RepositoryName,
		project.BaseBranch,
		project.ServiceName,
		project.DockerfilePath,
		project.ServiceType,
		project.ServicePort,
		project.ContainerPort,
		project.ReplicaCount,
		project.ResourceProfile,
		project.DedicatedLoadBalancer,
		project.AppsBaseDomain,
		project.KubeconfigEncrypted,
		project.CreatedAt,
		project.UpdatedAt,
	)
	return err
}

func (s *ProjectStore) GetByID(ctx context.Context, projectID string) (domain.Project, bool) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, name, owner_id, status, public_url, grafana_url,
		        repository_owner, repository_name, base_branch, service_name, dockerfile_path, service_type, service_port, container_port,
		        replica_count, resource_profile, dedicated_load_balancer, apps_base_domain,
		        kubeconfig_encrypted, github_token_encrypted, created_at, updated_at
		 FROM projects WHERE id = $1`,
		projectID,
	)

	var p domain.Project
	var status string
	var kubeconfigEncrypted sql.NullString
	var githubTokenEncrypted sql.NullString
	err := row.Scan(
		&p.ID, &p.Name, &p.OwnerID, &status, &p.PublicURL, &p.GrafanaURL,
		&p.RepositoryOwner, &p.RepositoryName, &p.BaseBranch, &p.ServiceName, &p.DockerfilePath, &p.ServiceType, &p.ServicePort, &p.ContainerPort,
		&p.ReplicaCount, &p.ResourceProfile, &p.DedicatedLoadBalancer, &p.AppsBaseDomain,
		&kubeconfigEncrypted, &githubTokenEncrypted, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Project{}, false
		}
		return domain.Project{}, false
	}

	p.Status = domain.ProjectStatus(status)
	p.KubeconfigEncrypted = nullString(kubeconfigEncrypted)
	p.GitHubTokenEncrypted = nullString(githubTokenEncrypted)
	return p, true
}

func (s *ProjectStore) List(ctx context.Context) []domain.Project {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, owner_id, status, public_url, grafana_url,
		        repository_owner, repository_name, base_branch, service_name, dockerfile_path, service_type, service_port, container_port,
		        replica_count, resource_profile, dedicated_load_balancer, apps_base_domain,
		        kubeconfig_encrypted, github_token_encrypted, created_at, updated_at
		 FROM projects ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var projects []domain.Project
	for rows.Next() {
		var p domain.Project
		var status string
		var kubeconfigEncrypted sql.NullString
		var githubTokenEncrypted sql.NullString
		if err := rows.Scan(
			&p.ID, &p.Name, &p.OwnerID, &status, &p.PublicURL, &p.GrafanaURL,
			&p.RepositoryOwner, &p.RepositoryName, &p.BaseBranch, &p.ServiceName, &p.DockerfilePath, &p.ServiceType, &p.ServicePort, &p.ContainerPort,
			&p.ReplicaCount, &p.ResourceProfile, &p.DedicatedLoadBalancer, &p.AppsBaseDomain,
			&kubeconfigEncrypted, &githubTokenEncrypted, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			continue
		}
		p.Status = domain.ProjectStatus(status)
		p.KubeconfigEncrypted = nullString(kubeconfigEncrypted)
		p.GitHubTokenEncrypted = nullString(githubTokenEncrypted)
		projects = append(projects, p)
	}

	return projects
}

func nullString(value sql.NullString) string {
	if value.Valid {
		return value.String
	}
	return ""
}

func (s *ProjectStore) Update(ctx context.Context, project domain.Project) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE projects
		 SET name = $1, owner_id = $2, status = $3, public_url = $4, grafana_url = $5,
		     repository_owner = $6, repository_name = $7, base_branch = $8, service_name = $9, dockerfile_path = $10,
		     service_type = $11, service_port = $12, container_port = $13, replica_count = $14, resource_profile = $15,
		     dedicated_load_balancer = $16, apps_base_domain = $17, updated_at = $18
		 WHERE id = $19`,
		project.Name,
		project.OwnerID,
		string(project.Status),
		project.PublicURL,
		project.GrafanaURL,
		project.RepositoryOwner,
		project.RepositoryName,
		project.BaseBranch,
		project.ServiceName,
		project.DockerfilePath,
		project.ServiceType,
		project.ServicePort,
		project.ContainerPort,
		project.ReplicaCount,
		project.ResourceProfile,
		project.DedicatedLoadBalancer,
		project.AppsBaseDomain,
		project.UpdatedAt,
		project.ID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("project not found")
	}
	return nil
}

func (s *ProjectStore) UpdateGitHubToken(ctx context.Context, projectID, encryptedToken string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE projects SET github_token_encrypted = $1 WHERE id = $2`,
		encryptedToken,
		projectID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("project not found")
	}
	return nil
}

func (s *ProjectStore) UpdateKubeconfig(ctx context.Context, projectID, encryptedKubeconfig string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE projects SET kubeconfig_encrypted = $1 WHERE id = $2`,
		encryptedKubeconfig,
		projectID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("project not found")
	}
	return nil
}
