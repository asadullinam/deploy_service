package domain

import (
	"errors"
	"time"
)

var ErrStageNotFound = errors.New("stage not found")

type StageStatus string

const (
	StageStatusCreating StageStatus = "creating"
	StageStatusActive   StageStatus = "active"
	StageStatusDeleting StageStatus = "deleting"
	StageStatusDeleted  StageStatus = "deleted"
	StageStatusFailed   StageStatus = "failed"
)

type Stage struct {
	ID        string      `json:"id"`
	ProjectID string      `json:"projectId"`
	Name      string      `json:"name"`
	Slug      string      `json:"slug"`
	Status    StageStatus `json:"status"`
	PublicURL string      `json:"publicUrl,omitempty"`
	CreatedAt time.Time   `json:"createdAt"`
	UpdatedAt time.Time   `json:"updatedAt"`
}

type CreateStageRequest struct {
	Name string `json:"name"`
}
