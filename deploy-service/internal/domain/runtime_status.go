package domain

import "time"

type ProjectPodStatus struct {
	Name     string `json:"name"`
	Phase    string `json:"phase"`
	Ready    bool   `json:"ready"`
	Restarts int32  `json:"restarts"`
}

type ServiceURL struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type StageURLs struct {
	StageID  string       `json:"stageId"`
	StageName string      `json:"stageName"`
	Slug     string       `json:"slug"`
	Services []ServiceURL `json:"services"`
}

type ProjectURLsResponse struct {
	Stages []StageURLs `json:"stages"`
}

type ProjectRuntimeStatus struct {
	ProjectID         string             `json:"projectId"`
	Namespace         string             `json:"namespace"`
	NamespaceExists   bool               `json:"namespaceExists"`
	DeploymentExists  bool               `json:"deploymentExists"`
	ServiceExists     bool               `json:"serviceExists"`
	HTTPRouteExists   bool               `json:"httpRouteExists"`
	PublicURL         string             `json:"publicUrl,omitempty"`
	ServiceURLs       []ServiceURL       `json:"serviceUrls,omitempty"`
	DesiredReplicas   int32              `json:"desiredReplicas"`
	ReadyReplicas     int32              `json:"readyReplicas"`
	AvailableReplicas int32              `json:"availableReplicas"`
	Pods              []ProjectPodStatus `json:"pods"`
	LastCheckedAt     time.Time          `json:"lastCheckedAt"`
	Message           string             `json:"message,omitempty"`
}
