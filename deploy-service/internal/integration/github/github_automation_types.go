package github

import "net/http"

type GitHubAutomation struct {
	baseURL       string
	token         string
	baseDomain    string
	publicURL     string
	webhookSecret string
	client        *http.Client
}

type inferredBootstrapSettings struct {
	baseBranch     string
	dockerfilePath string
	serviceName    string
	containerPort  int
	servicePort    int
	serviceType    string
}

type resourceProfile struct {
	name          string
	cpuRequest    string
	memoryRequest string
	cpuLimit      string
	memoryLimit   string
}
