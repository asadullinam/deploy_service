package domain

import (
	"errors"
	"time"
)

var ErrReleaseNotFound = errors.New("release not found")

type ReleaseStatus string

const (
	ReleaseStatusPending   ReleaseStatus = "pending"
	ReleaseStatusBuilding  ReleaseStatus = "building"
	ReleaseStatusDeploying ReleaseStatus = "deploying"
	ReleaseStatusSuccess   ReleaseStatus = "success"
	ReleaseStatusFailed    ReleaseStatus = "failed"
)

type Release struct {
	ID            string        `json:"id"`
	ProjectID     string        `json:"projectId"`
	StageID       string        `json:"stageId"`
	CommitSHA     string        `json:"commitSha"`
	CommitMessage string        `json:"commitMessage"`
	Status        ReleaseStatus `json:"status"`
	WorkflowRunID int64         `json:"workflowRunId"`
	ImageTag      string        `json:"imageTag"`
	CreatedAt     time.Time     `json:"createdAt"`
	UpdatedAt     time.Time     `json:"updatedAt"`
}
