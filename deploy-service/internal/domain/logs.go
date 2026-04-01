package domain

import "time"

type ProjectLogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Pod       string    `json:"pod,omitempty"`
	Container string    `json:"container,omitempty"`
	Stage     string    `json:"stage,omitempty"`
	Level     string    `json:"level,omitempty"`
	Message   string    `json:"message"`
}

type ProjectLogsRequest struct {
	StageID   string `json:"stageId,omitempty"`
	StageSlug string `json:"-"`
	Pod       string `json:"pod,omitempty"`
	Container string `json:"container,omitempty"`
	Search    string `json:"search,omitempty"`
	Level     string `json:"level,omitempty"`
	Since     string `json:"since,omitempty"`
	Limit     int    `json:"limit,omitempty"`
}

type ProjectLogsResponse struct {
	ProjectID string            `json:"projectId"`
	Namespace string            `json:"namespace"`
	StageID   string            `json:"stageId,omitempty"`
	StageSlug string            `json:"stageSlug,omitempty"`
	Query     string            `json:"query"`
	Entries   []ProjectLogEntry `json:"entries"`
}
