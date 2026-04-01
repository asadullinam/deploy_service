package domain

import "time"

type ProjectStatus string

const (
	ProjectStatusCreating  ProjectStatus = "creating"
	ProjectStatusFailed    ProjectStatus = "failed"
	ProjectStatusActive    ProjectStatus = "active"
	ProjectStatusSuspended ProjectStatus = "suspended"
	ProjectStatusDeleting  ProjectStatus = "deleting"
	ProjectStatusDeleted   ProjectStatus = "deleted"
)

type Project struct {
	ID                    string        `json:"id"`
	Name                  string        `json:"name"`
	OwnerID               string        `json:"ownerId"`
	Status                ProjectStatus `json:"status"`
	PublicURL             string        `json:"publicUrl,omitempty"`
	GrafanaURL            string        `json:"grafanaUrl,omitempty"`
	RepositoryOwner       string        `json:"repositoryOwner,omitempty"`
	RepositoryName        string        `json:"repositoryName,omitempty"`
	BaseBranch            string        `json:"baseBranch,omitempty"`
	ServiceName           string        `json:"serviceName,omitempty"`
	DockerfilePath        string        `json:"dockerfilePath,omitempty"`
	ServiceType           string        `json:"serviceType,omitempty"`
	ServicePort           int           `json:"servicePort,omitempty"`
	ContainerPort         int           `json:"containerPort,omitempty"`
	ReplicaCount          int           `json:"replicaCount,omitempty"`
	ResourceProfile       string        `json:"resourceProfile,omitempty"`
	DedicatedLoadBalancer bool          `json:"dedicatedLoadBalancer,omitempty"`
	AppsBaseDomain        string        `json:"appsBaseDomain,omitempty"`
	KubeconfigEncrypted   string        `json:"-"`
	GitHubTokenEncrypted  string        `json:"-"`
	SuspendedAt           *time.Time    `json:"suspendedAt,omitempty"`
	DeletionDueAt         *time.Time    `json:"deletionDueAt,omitempty"`
	CreatedAt             time.Time     `json:"createdAt"`
	UpdatedAt             time.Time     `json:"updatedAt"`
}

type CreateProjectRequest struct {
	Name    string `json:"name"`
	OwnerID string `json:"ownerId"`
}

type BootstrapGitHubFlowRequest struct {
	RepositoryOwner       string `json:"repositoryOwner"`
	RepositoryName        string `json:"repositoryName"`
	BaseBranch            string `json:"baseBranch"`
	StageSlug             string `json:"stageSlug,omitempty"`
	ServiceName           string `json:"serviceName"`
	DockerfilePath        string `json:"dockerfilePath"`
	ServicePort           int    `json:"servicePort"`
	ContainerPort         int    `json:"containerPort"`
	ServiceType           string `json:"serviceType"`
	ReplicaCount          int    `json:"replicaCount"`
	ResourceProfile       string `json:"resourceProfile"`
	DedicatedLoadBalancer bool   `json:"dedicatedLoadBalancer,omitempty"`
	GitHubToken           string `json:"githubToken,omitempty"`
	AppsBaseDomain        string `json:"-"` // set by service layer, not from HTTP request
}

type UpdateDeploymentSettingsRequest struct {
	RepositoryOwner       string `json:"repositoryOwner"`
	RepositoryName        string `json:"repositoryName"`
	BaseBranch            string `json:"baseBranch"`
	ServiceName           string `json:"serviceName"`
	DockerfilePath        string `json:"dockerfilePath"`
	ServicePort           int    `json:"servicePort"`
	ContainerPort         int    `json:"containerPort"`
	ServiceType           string `json:"serviceType"`
	ReplicaCount          int    `json:"replicaCount"`
	ResourceProfile       string `json:"resourceProfile"`
	DedicatedLoadBalancer bool   `json:"dedicatedLoadBalancer,omitempty"`
}

type BootstrapGitHubFlowResponse struct {
	ProjectID       string `json:"projectId"`
	RepositoryOwner string `json:"repositoryOwner"`
	RepositoryName  string `json:"repositoryName"`
	BranchName      string `json:"branchName"`
	PullRequestURL  string `json:"pullRequestUrl"`
	NoChanges       bool   `json:"noChanges"`
}

type GitHubBootstrapQuestionsRequest struct {
	RepositoryOwner string `json:"repositoryOwner"`
	RepositoryName  string `json:"repositoryName"`
	BaseBranch      string `json:"baseBranch"`
	DockerfilePath  string `json:"dockerfilePath"`
	ServiceName     string `json:"serviceName"`
	GitHubToken     string `json:"githubToken,omitempty"`
}

type GitHubBootstrapQuestion struct {
	Key          string   `json:"key"`
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	Required     bool     `json:"required"`
	DefaultValue string   `json:"defaultValue"`
	Options      []string `json:"options,omitempty"`
}

type GitHubBootstrapQuestionsResponse struct {
	RepositoryOwner       string                    `json:"repositoryOwner"`
	RepositoryName        string                    `json:"repositoryName"`
	BaseBranch            string                    `json:"baseBranch"`
	DetectedDockerfile    string                    `json:"detectedDockerfile"`
	DetectedServiceName   string                    `json:"detectedServiceName"`
	DetectedContainerPort int                       `json:"detectedContainerPort"`
	DetectedServicePort   int                       `json:"detectedServicePort"`
	DetectedServiceType   string                    `json:"detectedServiceType"`
	Questions             []GitHubBootstrapQuestion `json:"questions"`
}
