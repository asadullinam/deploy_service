package domain

import "time"

type GitHubWorkflowRunPayload struct {
	Action      string `json:"action"`
	WorkflowRun struct {
		ID         int64  `json:"id"`
		Status     string `json:"status"`
		Conclusion string `json:"conclusion"`
		HeadSHA    string `json:"head_sha"`
		Path       string `json:"path"`
		StageSlug  string `json:"stage_slug"`
		ImageTag   string `json:"image_tag"`
		HeadCommit struct {
			Message string `json:"message"`
		} `json:"head_commit"`
	} `json:"workflow_run"`
	Repository struct {
		Name  string `json:"name"`
		Owner struct {
			Login string `json:"login"`
		} `json:"owner"`
	} `json:"repository"`
}

type GitHubWorkflowRunLookupRequest struct {
	RepositoryOwner string
	RepositoryName  string
	GitHubToken     string
	Since           time.Time
	WorkflowPath    string // optional: filter to a specific workflow file path
}

type GitHubWorkflowRunLookupResult struct {
	ID            int64
	Status        string
	Conclusion    string
	HeadSHA       string
	CommitMessage string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}
