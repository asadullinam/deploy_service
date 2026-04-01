package github

import (
	"bytes"
	"context"
	"deploy-service/internal/domain"
	"deploy-service/internal/service"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Проверка на этапе компиляции: GitHubAutomation реализует service.GitHubAutomation.
var _ service.GitHubAutomation = (*GitHubAutomation)(nil)

func NewGitHubAutomation(baseURL string, token string, baseDomain string, publicURL string, webhookSecret string) *GitHubAutomation {
	return &GitHubAutomation{
		baseURL:       strings.TrimRight(baseURL, "/"),
		token:         token,
		baseDomain:    strings.TrimSpace(baseDomain),
		publicURL:     strings.TrimRight(strings.TrimSpace(publicURL), "/"),
		webhookSecret: strings.TrimSpace(webhookSecret),
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (a *GitHubAutomation) SetupProjectAutomation(_ context.Context, _ string) error {
	return nil
}

func (a *GitHubAutomation) BuildBootstrapQuestions(ctx context.Context, _ string, request domain.GitHubBootstrapQuestionsRequest) (domain.GitHubBootstrapQuestionsResponse, error) {
	client := a.withToken(request.GitHubToken)
	if strings.TrimSpace(request.RepositoryOwner) == "" || strings.TrimSpace(request.RepositoryName) == "" {
		return domain.GitHubBootstrapQuestionsResponse{}, fmt.Errorf("repository owner and repository name are required")
	}

	settings, err := client.inferBootstrapSettings(ctx, request.RepositoryOwner, request.RepositoryName, request.BaseBranch, request.DockerfilePath, request.ServiceName)
	if err != nil {
		return domain.GitHubBootstrapQuestionsResponse{}, err
	}

	return domain.GitHubBootstrapQuestionsResponse{
		RepositoryOwner:       request.RepositoryOwner,
		RepositoryName:        request.RepositoryName,
		BaseBranch:            settings.baseBranch,
		DetectedDockerfile:    settings.dockerfilePath,
		DetectedServiceName:   settings.serviceName,
		DetectedContainerPort: settings.containerPort,
		DetectedServicePort:   settings.servicePort,
		DetectedServiceType:   settings.serviceType,
		Questions: []domain.GitHubBootstrapQuestion{
			{Key: "serviceName", Title: "Имя сервиса", Description: "Имя Deployment и Service", Required: true, DefaultValue: settings.serviceName},
			{Key: "dockerfilePath", Title: "Путь к Dockerfile", Description: "Путь в репозитории к Dockerfile", Required: true, DefaultValue: settings.dockerfilePath},
			{Key: "containerPort", Title: "Порт контейнера", Description: "Порт приложения внутри контейнера", Required: true, DefaultValue: strconv.Itoa(settings.containerPort)},
			{Key: "servicePort", Title: "Порт сервиса", Description: "Порт Kubernetes Service", Required: true, DefaultValue: strconv.Itoa(settings.servicePort)},
			{Key: "serviceType", Title: "Тип сервиса", Description: "LoadBalancer для внешнего доступа, ClusterIP для внутреннего доступа", Required: true, DefaultValue: settings.serviceType, Options: []string{"LoadBalancer", "ClusterIP"}},
		},
	}, nil
}

func (a *GitHubAutomation) BootstrapRepositoryFlow(ctx context.Context, projectID string, request domain.BootstrapGitHubFlowRequest) (domain.BootstrapGitHubFlowResponse, error) {
	client := a.withToken(request.GitHubToken)
	if strings.TrimSpace(request.RepositoryOwner) == "" || strings.TrimSpace(request.RepositoryName) == "" {
		return domain.BootstrapGitHubFlowResponse{}, fmt.Errorf("repository owner and repository name are required")
	}

	settings, err := client.inferBootstrapSettings(ctx, request.RepositoryOwner, request.RepositoryName, request.BaseBranch, request.DockerfilePath, request.ServiceName)
	if err != nil {
		return domain.BootstrapGitHubFlowResponse{}, err
	}

	baseBranch := settings.baseBranch
	serviceName := settings.serviceName
	dockerfilePath := settings.dockerfilePath
	servicePort := settings.servicePort
	containerPort := settings.containerPort
	serviceType := settings.serviceType
	replicaCount := normalizedReplicaCount(request.ReplicaCount)
	resourceProfile := normalizedResourceProfile(request.ResourceProfile)

	if request.ServicePort > 0 {
		servicePort = request.ServicePort
	}
	if request.ContainerPort > 0 {
		containerPort = request.ContainerPort
	}
	if normalized := normalizeServiceType(request.ServiceType); normalized != "" {
		serviceType = normalized
	}

	refSHA, err := client.getBranchSHA(ctx, request.RepositoryOwner, request.RepositoryName, baseBranch)
	if err != nil {
		return domain.BootstrapGitHubFlowResponse{}, err
	}

	branchName := fmt.Sprintf("deploy-service/%s-%d", sanitizeName(projectID), time.Now().Unix())
	if err := client.createBranch(ctx, request.RepositoryOwner, request.RepositoryName, branchName, refSHA); err != nil {
		return domain.BootstrapGitHubFlowResponse{}, err
	}

	stageSlug := strings.TrimSpace(request.StageSlug)
	if stageSlug == "" {
		stageSlug = "production"
	}
	stageSlug = sanitizeName(stageSlug)
	if stageSlug == "" {
		stageSlug = "production"
	}

	workflowPath := stageWorkflowPath(stageSlug)
	manifestBase := fmt.Sprintf("k8s/%s/%s", stageSlug, serviceName)

	effectiveBaseDomain := client.baseDomain
	if strings.TrimSpace(request.AppsBaseDomain) != "" {
		effectiveBaseDomain = request.AppsBaseDomain
	}
	serviceManifestType, needsIngress := resolveServiceExposure(serviceType, request.DedicatedLoadBalancer, effectiveBaseDomain)
	var ingressContent string
	if needsIngress {
		ingressContent = renderIngressYAML(serviceName, projectID, stageSlug, effectiveBaseDomain, servicePort)
	}
	workflowContent := renderWorkflowYAML(projectID, request.RepositoryName, serviceName, dockerfilePath, stageSlug, baseBranch, ingressContent != "")
	deploymentContent := renderDeploymentYAML(serviceName, stageSlug, containerPort, servicePort, replicaCount, resourceProfile)
	serviceContent := renderServiceYAML(serviceName, servicePort, containerPort, serviceManifestType)

	if err := client.upsertFile(ctx, request.RepositoryOwner, request.RepositoryName, branchName, workflowPath, workflowContent, "Add deploy-service GitHub Actions workflow"); err != nil {
		return domain.BootstrapGitHubFlowResponse{}, err
	}
	if err := client.upsertFile(ctx, request.RepositoryOwner, request.RepositoryName, branchName, manifestBase+"/deployment.yaml", deploymentContent, "Add Kubernetes deployment manifest"); err != nil {
		return domain.BootstrapGitHubFlowResponse{}, err
	}
	if err := client.upsertFile(ctx, request.RepositoryOwner, request.RepositoryName, branchName, manifestBase+"/service.yaml", serviceContent, "Add Kubernetes service manifest"); err != nil {
		return domain.BootstrapGitHubFlowResponse{}, err
	}
	if ingressContent != "" {
		if err := client.upsertFile(ctx, request.RepositoryOwner, request.RepositoryName, branchName, manifestBase+"/ingress.yaml", ingressContent, "Add Kubernetes Ingress manifest"); err != nil {
			return domain.BootstrapGitHubFlowResponse{}, err
		}
	}

	prURL, err := client.createPullRequest(
		ctx,
		request.RepositoryOwner,
		request.RepositoryName,
		baseBranch,
		branchName,
		"Setup deploy-service automation",
		"Добавлены workflow и Kubernetes манифесты для автоматической сборки и деплоя.",
	)
	if err != nil {
		if isNoCommitsBetweenBranchesError(err) {
			log.Printf("github bootstrap skipped pr for %s/%s: no commits between %s and %s", request.RepositoryOwner, request.RepositoryName, baseBranch, branchName)
			prURL = ""
		} else {
			return domain.BootstrapGitHubFlowResponse{}, err
		}
	}

	if err := client.ensureWorkflowWebhook(ctx, request.RepositoryOwner, request.RepositoryName); err != nil {
		log.Printf("github webhook registration skipped for %s/%s: %v", request.RepositoryOwner, request.RepositoryName, err)
	}

	return domain.BootstrapGitHubFlowResponse{
		ProjectID:       projectID,
		RepositoryOwner: request.RepositoryOwner,
		RepositoryName:  request.RepositoryName,
		BranchName:      branchName,
		PullRequestURL:  prURL,
		NoChanges:       strings.TrimSpace(prURL) == "",
	}, nil
}

func (a *GitHubAutomation) withToken(override string) *GitHubAutomation {
	token := strings.TrimSpace(override)
	if token == "" {
		token = a.token
	}

	return &GitHubAutomation{
		baseURL:       a.baseURL,
		token:         token,
		baseDomain:    a.baseDomain,
		publicURL:     a.publicURL,
		webhookSecret: a.webhookSecret,
		client:        a.client,
	}
}

func (a *GitHubAutomation) FindLatestDeployWorkflowRun(ctx context.Context, request domain.GitHubWorkflowRunLookupRequest) ([]domain.GitHubWorkflowRunLookupResult, error) {
	client := a.withToken(request.GitHubToken)
	if strings.TrimSpace(request.RepositoryOwner) == "" || strings.TrimSpace(request.RepositoryName) == "" {
		return nil, nil
	}

	body, err := client.requestJSON(
		ctx,
		http.MethodGet,
		fmt.Sprintf("/repos/%s/%s/actions/runs?per_page=20", request.RepositoryOwner, request.RepositoryName),
		nil,
	)
	if err != nil {
		return nil, err
	}

	var payload struct {
		WorkflowRuns []struct {
			ID         int64     `json:"id"`
			Status     string    `json:"status"`
			Conclusion string    `json:"conclusion"`
			HeadSHA    string    `json:"head_sha"`
			Path       string    `json:"path"`
			CreatedAt  time.Time `json:"created_at"`
			UpdatedAt  time.Time `json:"updated_at"`
			HeadCommit struct {
				Message string `json:"message"`
			} `json:"head_commit"`
		} `json:"workflow_runs"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode workflow runs response: %w", err)
	}

	var results []domain.GitHubWorkflowRunLookupResult
	for _, run := range payload.WorkflowRuns {
		if request.WorkflowPath != "" {
			if run.Path != request.WorkflowPath {
				continue
			}
		} else if !isDeployWorkflowPath(run.Path) {
			continue
		}
		if !request.Since.IsZero() && run.CreatedAt.Before(request.Since.Add(-2*time.Minute)) {
			continue
		}
		results = append(results, domain.GitHubWorkflowRunLookupResult{
			ID:            run.ID,
			Status:        run.Status,
			Conclusion:    run.Conclusion,
			HeadSHA:       run.HeadSHA,
			CommitMessage: run.HeadCommit.Message,
			CreatedAt:     run.CreatedAt,
			UpdatedAt:     run.UpdatedAt,
		})
	}

	return results, nil
}

func stageWorkflowPath(stageSlug string) string {
	if stageSlug == "" || stageSlug == "production" {
		return ".github/workflows/deploy-service.yml"
	}
	return fmt.Sprintf(".github/workflows/deploy-service-%s.yml", sanitizeName(stageSlug))
}

func stageSlugFromWorkflowPath(path string) string {
	base := path
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		base = path[idx+1:]
	}
	if base == "deploy-service.yml" || base == "deploy-deploy-service.yml" {
		return "production"
	}
	if strings.HasPrefix(base, "deploy-service-") && strings.HasSuffix(base, ".yml") {
		return strings.TrimSuffix(strings.TrimPrefix(base, "deploy-service-"), ".yml")
	}
	return ""
}

func isDeployWorkflowPath(path string) bool {
	return stageSlugFromWorkflowPath(strings.TrimSpace(path)) != ""
}

func (a *GitHubAutomation) ensureWorkflowWebhook(ctx context.Context, owner string, repo string) error {
	if a.publicURL == "" {
		return fmt.Errorf("PUBLIC_URL is empty, cannot auto-register GitHub workflow_run webhook")
	}

	config := map[string]string{
		"url":          a.publicURL + "/webhooks/github",
		"content_type": "json",
		"insecure_ssl": "0",
	}
	if a.webhookSecret != "" {
		config["secret"] = a.webhookSecret
	}

	payload := map[string]any{
		"name":   "web",
		"active": true,
		"events": []string{"workflow_run"},
		"config": config,
	}

	_, err := a.requestJSON(ctx, http.MethodPost, fmt.Sprintf("/repos/%s/%s/hooks", owner, repo), payload)
	if err != nil {
		// Дубликат webhook не должен блокировать bootstrap.
		if strings.Contains(err.Error(), "status 422") {
			return nil
		}
		return err
	}

	return nil
}

func (a *GitHubAutomation) inferBootstrapSettings(ctx context.Context, owner string, repo string, baseBranchInput string, dockerfilePathInput string, serviceNameInput string) (inferredBootstrapSettings, error) {
	baseBranch := strings.TrimSpace(baseBranchInput)
	if baseBranch == "" {
		defaultBranch, err := a.getDefaultBranch(ctx, owner, repo)
		if err != nil {
			return inferredBootstrapSettings{}, err
		}
		baseBranch = defaultBranch
	}

	dockerfilePath := strings.TrimSpace(dockerfilePathInput)
	if dockerfilePath == "" {
		dockerfilePath = "Dockerfile"
	}

	serviceName := sanitizeName(serviceNameInput)
	if serviceName == "" {
		serviceName = sanitizeName(repo)
	}
	if serviceName == "" {
		serviceName = "service"
	}

	containerPort := 8080
	if dockerfileContent, err := a.getTextFile(ctx, owner, repo, baseBranch, dockerfilePath); err == nil {
		if detectedPort := detectPortFromDockerfile(dockerfileContent); detectedPort > 0 {
			containerPort = detectedPort
		}
	}

	return inferredBootstrapSettings{
		baseBranch:     baseBranch,
		dockerfilePath: dockerfilePath,
		serviceName:    serviceName,
		containerPort:  containerPort,
		servicePort:    containerPort,
		serviceType:    "LoadBalancer",
	}, nil
}

func (a *GitHubAutomation) getDefaultBranch(ctx context.Context, owner string, repo string) (string, error) {
	body, err := a.requestJSON(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/%s", owner, repo), nil)
	if err != nil {
		return "", err
	}

	var payload struct {
		DefaultBranch string `json:"default_branch"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", fmt.Errorf("decode default branch response: %w", err)
	}

	if strings.TrimSpace(payload.DefaultBranch) == "" {
		return "", fmt.Errorf("repository default branch is empty")
	}

	return payload.DefaultBranch, nil
}

func (a *GitHubAutomation) getBranchSHA(ctx context.Context, owner string, repo string, branch string) (string, error) {
	body, err := a.requestJSON(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/%s/git/ref/heads/%s", owner, repo, branch), nil)
	if err != nil {
		return "", err
	}

	var payload struct {
		Object struct {
			SHA string `json:"sha"`
		} `json:"object"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", fmt.Errorf("decode branch ref response: %w", err)
	}

	if payload.Object.SHA == "" {
		return "", fmt.Errorf("branch sha is empty")
	}

	return payload.Object.SHA, nil
}

func (a *GitHubAutomation) createBranch(ctx context.Context, owner string, repo string, branch string, sha string) error {
	payload := map[string]string{
		"ref": fmt.Sprintf("refs/heads/%s", branch),
		"sha": sha,
	}
	_, err := a.requestJSON(ctx, http.MethodPost, fmt.Sprintf("/repos/%s/%s/git/refs", owner, repo), payload)
	if err != nil {
		return fmt.Errorf("create branch %s: %w", branch, err)
	}

	return nil
}

func (a *GitHubAutomation) upsertFile(ctx context.Context, owner string, repo string, branch string, path string, content string, message string) error {
	existingContent, err := a.getTextFile(ctx, owner, repo, branch, path)
	if err == nil && existingContent == content {
		return nil
	}

	sha, _ := a.getFileSHA(ctx, owner, repo, branch, path)

	payload := map[string]string{
		"message": message,
		"content": base64.StdEncoding.EncodeToString([]byte(content)),
		"branch":  branch,
	}
	if sha != "" {
		payload["sha"] = sha
	}

	_, err = a.requestJSON(ctx, http.MethodPut, fmt.Sprintf("/repos/%s/%s/contents/%s", owner, repo, path), payload)
	if err != nil {
		return fmt.Errorf("upsert file %s: %w", path, err)
	}

	return nil
}

func (a *GitHubAutomation) getFileSHA(ctx context.Context, owner string, repo string, branch string, path string) (string, error) {
	body, err := a.requestJSON(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/%s/contents/%s?ref=%s", owner, repo, path, branch), nil)
	if err != nil {
		if strings.Contains(err.Error(), "status 404") {
			return "", nil
		}
		return "", err
	}

	var payload struct {
		SHA string `json:"sha"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", fmt.Errorf("decode content response for %s: %w", path, err)
	}

	return payload.SHA, nil
}

func (a *GitHubAutomation) getTextFile(ctx context.Context, owner string, repo string, branch string, path string) (string, error) {
	body, err := a.requestJSON(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/%s/contents/%s?ref=%s", owner, repo, path, branch), nil)
	if err != nil {
		return "", err
	}

	var payload struct {
		Encoding string `json:"encoding"`
		Content  string `json:"content"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", fmt.Errorf("decode content response for %s: %w", path, err)
	}

	if strings.ToLower(payload.Encoding) != "base64" {
		return "", fmt.Errorf("unsupported content encoding for %s: %s", path, payload.Encoding)
	}

	decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(payload.Content, "\n", ""))
	if err != nil {
		return "", fmt.Errorf("decode base64 content for %s: %w", path, err)
	}

	return string(decoded), nil
}

func (a *GitHubAutomation) createPullRequest(ctx context.Context, owner string, repo string, base string, head string, title string, body string) (string, error) {
	payload := map[string]string{
		"title": title,
		"head":  head,
		"base":  base,
		"body":  body,
	}

	responseBody, err := a.requestJSON(ctx, http.MethodPost, fmt.Sprintf("/repos/%s/%s/pulls", owner, repo), payload)
	if err != nil {
		return "", fmt.Errorf("create pull request: %w", err)
	}

	var response struct {
		HTMLURL string `json:"html_url"`
	}
	if err := json.Unmarshal(responseBody, &response); err != nil {
		return "", fmt.Errorf("decode pull request response: %w", err)
	}

	if response.HTMLURL == "" {
		return "", fmt.Errorf("pull request url is empty")
	}

	return response.HTMLURL, nil
}

func (a *GitHubAutomation) requestJSON(ctx context.Context, method string, path string, payload any) ([]byte, error) {
	if strings.TrimSpace(a.token) == "" {
		return nil, fmt.Errorf("github token is required; needed permissions: Contents=read/write, Pull requests=read/write, Workflows=write, Webhooks=read/write, Metadata=read-only")
	}

	var bodyReader io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("encode request payload: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	request, err := http.NewRequestWithContext(ctx, method, a.baseURL+path, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("build request %s %s: %w", method, path, err)
	}

	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("Authorization", "Bearer "+a.token)
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := a.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("request failed %s %s: %w", method, path, err)
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("read response %s %s: %w", method, path, err)
	}

	if response.StatusCode < 200 || response.StatusCode > 299 {
		message := strings.TrimSpace(string(responseBody))
		if response.StatusCode == http.StatusNotFound && strings.Contains(path, "/contents/.github/workflows/") {
			message = message + "; check token permissions: repository Contents=write and Workflows=write"
		}
		if response.StatusCode == http.StatusForbidden && strings.Contains(path, "/hooks") {
			message = message + "; check token permissions: repository Webhooks=read/write"
		}
		return nil, fmt.Errorf("github api status %d for %s %s: %s", response.StatusCode, method, path, message)
	}

	return responseBody, nil
}

func sanitizeName(value string) string {
	v := strings.ToLower(strings.TrimSpace(value))
	v = strings.ReplaceAll(v, "_", "-")
	re := regexp.MustCompile(`[^a-z0-9-]`)
	v = re.ReplaceAllString(v, "-")
	v = strings.Trim(v, "-")
	if v == "" {
		return ""
	}
	return v
}

func isNoCommitsBetweenBranchesError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "no commits between")
}

func renderWorkflowYAML(projectID string, repositoryName string, serviceName string, dockerfilePath string, stageSlug string, baseBranch string, includeIngress bool) string {
	projectSlug := sanitizeName(projectID)
	if projectSlug == "" {
		projectSlug = "project"
	}
	repoSlug := sanitizeName(repositoryName)
	svcSlug := sanitizeName(serviceName)
	defaultStageSlug := sanitizeName(stageSlug)
	if defaultStageSlug == "" {
		defaultStageSlug = "production"
	}
	manifestPath := fmt.Sprintf("k8s/%s/%s/deployment.yaml", defaultStageSlug, svcSlug)
	servicePath := fmt.Sprintf("k8s/%s/%s/service.yaml", defaultStageSlug, svcSlug)
	ingressPath := fmt.Sprintf("k8s/%s/%s/ingress.yaml", defaultStageSlug, svcSlug)
	buildContext := "."
	if idx := strings.LastIndex(dockerfilePath, "/"); idx > 0 {
		buildContext = dockerfilePath[:idx]
	}

	ingressStep := ""
	if includeIngress {
		ingressStep = fmt.Sprintf(`
          retry_or_fail "ingress apply" kubectl -n "$STAGE_SLUG" apply --validate=false -f %s`, ingressPath)
	}

	return fmt.Sprintf(`name: Deploy Service

on:
  push:
    branches:
      - %s
  workflow_dispatch:

env:
  FORCE_JAVASCRIPT_ACTIONS_TO_NODE24: true
  PROJECT_ID: %s
  STAGE_SLUG: ${{ vars.STAGE_SLUG || '%s' }}

permissions:
  contents: read
  packages: write

jobs:
  build_and_deploy:
    runs-on: ubuntu-latest
    env:
      KUBECONFIG_BASE64: ${{ secrets.KUBECONFIG_BASE64 }}
      YC_SERVICE_ACCOUNT_KEY_JSON: ${{ secrets.YC_SERVICE_ACCOUNT_KEY_JSON }}
      YC_OAUTH_TOKEN: ${{ secrets.YC_OAUTH_TOKEN }}
      YC_MANAGED_K8S_CLUSTER_ID: ${{ secrets.YC_MANAGED_K8S_CLUSTER_ID }}

    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Resolve IMAGE_NAME (lowercase owner)
        run: |
          OWNER_LC=$(echo "${{ github.repository_owner }}" | tr '[:upper:]' '[:lower:]')
          echo "IMAGE_NAME=ghcr.io/${OWNER_LC}/%s-%s:${GITHUB_SHA}" >> "$GITHUB_ENV"

      - name: Log in to GitHub Container Registry
        uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Validate deployment auth source
        env:
          KUBECONFIG_BASE64: ${{ secrets.KUBECONFIG_BASE64 }}
        run: |
          if [ -z "$KUBECONFIG_BASE64" ] && [ -z "$YC_MANAGED_K8S_CLUSTER_ID" ]; then
            echo "Missing deployment auth source: set KUBECONFIG_BASE64 or YC_MANAGED_K8S_CLUSTER_ID"
            exit 1
          fi
          if [ -z "$KUBECONFIG_BASE64" ] && [ -z "$YC_SERVICE_ACCOUNT_KEY_JSON" ] && [ -z "$YC_OAUTH_TOKEN" ]; then
            echo "Missing YC credentials: set YC_SERVICE_ACCOUNT_KEY_JSON or YC_OAUTH_TOKEN to fetch kubeconfig"
            exit 1
          fi

      - name: Build and push image
        uses: docker/build-push-action@v6
        with:
          context: %s
          file: %s
          push: true
          tags: ${{ env.IMAGE_NAME }}

      - name: Configure kubectl
        if: ${{ env.KUBECONFIG_BASE64 != '' }}
        run: |
          mkdir -p $HOME/.kube
          echo "$KUBECONFIG_BASE64" | base64 --decode > $HOME/.kube/config

      - name: Install Yandex Cloud CLI
        run: |
          curl -sSL https://storage.yandexcloud.net/yandexcloud-yc/install.sh | bash -s -- -i $HOME/yandex-cloud -n
          echo "$HOME/yandex-cloud/bin" >> $GITHUB_PATH

      - name: Configure Yandex Cloud profile
        if: ${{ env.YC_SERVICE_ACCOUNT_KEY_JSON != '' || env.YC_OAUTH_TOKEN != '' }}
        run: |
          yc config profile create default || true
          if [ -n "$YC_SERVICE_ACCOUNT_KEY_JSON" ]; then
            printf '%%s' "$YC_SERVICE_ACCOUNT_KEY_JSON" > $HOME/sa-key.json
            yc config set service-account-key $HOME/sa-key.json
          elif [ -n "$YC_OAUTH_TOKEN" ]; then
            yc config set token "$YC_OAUTH_TOKEN"
          fi

      - name: Refresh kubeconfig from Yandex Cloud (optional)
        if: ${{ env.YC_MANAGED_K8S_CLUSTER_ID != '' && (env.YC_SERVICE_ACCOUNT_KEY_JSON != '' || env.YC_OAUTH_TOKEN != '') }}
        run: |
          yc managed-kubernetes cluster get-credentials "$YC_MANAGED_K8S_CLUSTER_ID" --external --force

      - name: Normalize kubeconfig exec command
        run: |
          CURRENT_CONTEXT=$(kubectl config current-context 2>/dev/null || true)
          if [ -z "$CURRENT_CONTEXT" ]; then
            CURRENT_CONTEXT=$(kubectl config view -o jsonpath="{.contexts[0].name}")
            if [ -n "$CURRENT_CONTEXT" ]; then
              kubectl config use-context "$CURRENT_CONTEXT"
            fi
          fi
          CURRENT_USER=""
          if [ -n "$CURRENT_CONTEXT" ]; then
            CURRENT_USER=$(kubectl config view -o jsonpath="{.contexts[?(@.name==\"$CURRENT_CONTEXT\")].context.user}")
          fi
          if [ -n "$CURRENT_USER" ]; then
            kubectl config set-credentials "$CURRENT_USER" \
              --exec-command=yc \
              --exec-api-version=client.authentication.k8s.io/v1beta1 \
              --exec-arg=managed-kubernetes \
              --exec-arg=create-token
          else
            echo "Skipping kubeconfig exec normalization because no current user was found"
          fi

      - name: Verify Kubernetes API connectivity
        run: |
          API_SERVER=$(kubectl config view --minify -o jsonpath="{.clusters[0].cluster.server}")
          echo "Kubernetes API endpoint: $API_SERVER"
          if ! kubectl --request-timeout=20s get --raw=/version > /tmp/k8s-version.json; then
            echo "::error::Не удалось подключиться к Kubernetes API ($API_SERVER). Проверь, что kubeconfig указывает на внешний endpoint кластера, либо запускай деплой на self-hosted runner в той же сети."
            exit 1
          fi
          cat /tmp/k8s-version.json

      - name: Verify kubeconfig targets project vcluster
        run: |
          HOST_PROJECT_NAMESPACE="project-${PROJECT_ID}"
          if kubectl get namespace "$HOST_PROJECT_NAMESPACE" >/dev/null 2>&1; then
            echo "::error::KUBECONFIG указывает на host cluster (найден namespace $HOST_PROJECT_NAMESPACE). Для deploy нужен kubeconfig vcluster этого проекта."
            echo "::error::Открой deploy-service UI -> Проект -> Доступ -> «Скопировать KUBECONFIG_BASE64» и обнови repository secret."
            exit 1
          fi

          if kubectl get namespace -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' | grep -q '^project-prj-'; then
            echo "::error::KUBECONFIG похож на host cluster (видны namespace project-prj-*). Для deploy нужен kubeconfig vcluster проекта."
            echo "::error::Используй секрет KUBECONFIG_BASE64 из кнопки «Скопировать KUBECONFIG_BASE64» в UI deploy-service."
            exit 1
          fi

      - name: Ensure namespace exists
        run: |
          retry_or_fail() {
            description="$1"
            shift
            for attempt in 1 2 3 4 5; do
              if "$@"; then
                return 0
              fi
              echo "$description retry $attempt failed"
              sleep 10
            done
            echo "$description failed after 5 attempts"
            return 1
          }
          retry_or_fail "namespace apply" sh -c 'kubectl create namespace "$STAGE_SLUG" --dry-run=client -o yaml | kubectl apply --validate=false -f -'

      - name: Deploy manifests
        run: |
          retry_or_fail() {
            description="$1"
            shift
            for attempt in 1 2 3 4 5; do
              if "$@"; then
                return 0
              fi
              echo "$description retry $attempt failed"
              sleep 10
            done
            echo "$description failed after 5 attempts"
            return 1
          }
          retry_or_fail "deployment apply" sh -c 'sed "s#IMAGE_PLACEHOLDER#${IMAGE_NAME}#g" %s | kubectl -n "$STAGE_SLUG" apply --validate=false -f -'
          retry_or_fail "service apply" kubectl -n "$STAGE_SLUG" apply --validate=false -f %s%s

      - name: Verify deployment
        run: |
          # Run rollout status in the background; poll for crash states to fail fast.
          kubectl -n "$STAGE_SLUG" rollout status deployment/%s --timeout=180s &
          ROLLOUT_PID=$!
          CRASHED=0
          for _i in $(seq 1 36); do
            sleep 5
            if ! kill -0 "$ROLLOUT_PID" 2>/dev/null; then
              break
            fi
            REASONS=$(kubectl -n "$STAGE_SLUG" get pods -l app=%s \
              -o jsonpath='{range .items[*]}{range .status.containerStatuses[*]}{.state.waiting.reason}{"\n"}{.lastState.terminated.reason}{"\n"}{end}{end}' 2>/dev/null || true)
            if echo "$REASONS" | grep -qE '^(CrashLoopBackOff|OOMKilled|ImagePullBackOff|ErrImagePull|CreateContainerConfigError)$'; then
              CRASHED=1
              kill "$ROLLOUT_PID" 2>/dev/null || true
              wait "$ROLLOUT_PID" 2>/dev/null || true
              break
            fi
          done
          if [ "$CRASHED" = "1" ]; then
            echo "::error::Pod упал (CrashLoopBackOff/OOMKilled/ImagePullBackOff). Собираем диагностику..."
            kubectl -n "$STAGE_SLUG" get pods -l app=%s -o wide || true
            kubectl -n "$STAGE_SLUG" describe pod -l app=%s || true
            kubectl -n "$STAGE_SLUG" logs -l app=%s --tail=100 --previous 2>/dev/null \
              || kubectl -n "$STAGE_SLUG" logs -l app=%s --tail=100 || true
            kubectl -n "$STAGE_SLUG" get events --sort-by=.lastTimestamp | tail -n 30 || true
            exit 1
          fi
          wait "$ROLLOUT_PID"
          ROLLOUT_RC=$?
          if [ "$ROLLOUT_RC" != "0" ]; then
            DESIRED=$(kubectl -n "$STAGE_SLUG" get deployment/%s -o jsonpath='{.spec.replicas}' 2>/dev/null || echo 0)
            UPDATED=$(kubectl -n "$STAGE_SLUG" get deployment/%s -o jsonpath='{.status.updatedReplicas}' 2>/dev/null || echo 0)
            AVAILABLE=$(kubectl -n "$STAGE_SLUG" get deployment/%s -o jsonpath='{.status.availableReplicas}' 2>/dev/null || echo 0)
            if [ "${DESIRED:-0}" -gt 0 ] && [ "${UPDATED:-0}" -ge "${DESIRED:-0}" ] && [ "${AVAILABLE:-0}" -ge "${DESIRED:-0}" ]; then
              echo "::warning::Rollout timed out while old replicas terminate, but new replicas are ready. Continuing."
            else
              echo "::error::Rollout deployment/%s не завершился за 180s. Собираем диагностику..."
              kubectl -n "$STAGE_SLUG" get deployment/%s -o wide || true
              kubectl -n "$STAGE_SLUG" get rs,pods -l app=%s -o wide || true
              kubectl -n "$STAGE_SLUG" describe deployment/%s || true
              kubectl -n "$STAGE_SLUG" describe pod -l app=%s || true
              kubectl -n "$STAGE_SLUG" get events --sort-by=.lastTimestamp | tail -n 50 || true
              exit 1
            fi
          fi
          kubectl -n "$STAGE_SLUG" get pods
`, baseBranch, projectSlug, defaultStageSlug, repoSlug, svcSlug, buildContext, dockerfilePath, manifestPath, servicePath, ingressStep, svcSlug, svcSlug, svcSlug, svcSlug, svcSlug, svcSlug, svcSlug, svcSlug, svcSlug, svcSlug, svcSlug, svcSlug, svcSlug, svcSlug)
}

func renderDeploymentYAML(serviceName string, stageSlug string, containerPort int, servicePort int, replicaCount int, resourceProfile string) string {
	name := sanitizeName(serviceName)
	if name == "" {
		name = "service"
	}
	profile := deploymentResourceProfile(resourceProfile)

	return fmt.Sprintf(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: %s
  labels:
    deploy-service.io/stage: %s
  annotations:
    nginx.ingress.kubernetes.io/force-ssl-redirect: "true"
spec:
  replicas: %d
  selector:
    matchLabels:
      app: %s
  template:
    metadata:
      labels:
        app: %s
        deploy-service.io/stage: %s
    spec:
      containers:
        - name: %s
          image: IMAGE_PLACEHOLDER
          ports:
            - containerPort: %d
          resources:
            requests:
              cpu: %s
              memory: %s
            limits:
              cpu: %s
              memory: %s
          readinessProbe:
            tcpSocket:
              port: %d
            initialDelaySeconds: 10
            periodSeconds: 10
          livenessProbe:
            tcpSocket:
              port: %d
            initialDelaySeconds: 20
            periodSeconds: 15
`, name, sanitizeName(stageSlug), normalizedReplicaCount(replicaCount), name, name, sanitizeName(stageSlug), name, containerPort, profile.cpuRequest, profile.memoryRequest, profile.cpuLimit, profile.memoryLimit, containerPort, containerPort)
}

func renderServiceYAML(serviceName string, servicePort int, containerPort int, serviceType string) string {
	name := sanitizeName(serviceName)
	if name == "" {
		name = "service"
	}

	typeValue := normalizeServiceType(serviceType)
	if typeValue == "" {
		typeValue = "LoadBalancer"
	}

	return fmt.Sprintf(`apiVersion: v1
kind: Service
metadata:
  name: %s
spec:
  selector:
    app: %s
  ports:
    - protocol: TCP
      port: %d
      targetPort: %d
  type: %s
`, name, name, servicePort, containerPort, typeValue)
}

func resolveServiceExposure(serviceType string, dedicatedLoadBalancer bool, baseDomain string) (string, bool) {
	normalizedType := normalizeServiceType(serviceType)
	switch normalizedType {
	case "ClusterIP":
		return "ClusterIP", false
	case "LoadBalancer":
		if dedicatedLoadBalancer {
			return "LoadBalancer", false
		}
		if strings.TrimSpace(baseDomain) != "" {
			return "ClusterIP", true
		}
		return "LoadBalancer", false
	default:
		return "ClusterIP", false
	}
}

func detectPortFromDockerfile(content string) int {
	re := regexp.MustCompile(`(?im)^\s*EXPOSE\s+([0-9]{2,5})`)
	matches := re.FindStringSubmatch(content)
	if len(matches) < 2 {
		return 0
	}

	port, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0
	}
	return port
}

func normalizeServiceType(serviceType string) string {
	value := strings.TrimSpace(serviceType)
	if strings.EqualFold(value, "LoadBalancer") {
		return "LoadBalancer"
	}
	if strings.EqualFold(value, "ClusterIP") {
		return "ClusterIP"
	}
	return ""
}

func deploymentResourceProfile(profile string) resourceProfile {
	switch normalizedResourceProfile(profile) {
	case "starter":
		return resourceProfile{
			name:          "starter",
			cpuRequest:    "50m",
			memoryRequest: "64Mi",
			cpuLimit:      "300m",
			memoryLimit:   "256Mi",
		}
	case "performance":
		return resourceProfile{
			name:          "performance",
			cpuRequest:    "250m",
			memoryRequest: "256Mi",
			cpuLimit:      "1000m",
			memoryLimit:   "1024Mi",
		}
	default:
		return resourceProfile{
			name:          "balanced",
			cpuRequest:    "100m",
			memoryRequest: "128Mi",
			cpuLimit:      "500m",
			memoryLimit:   "512Mi",
		}
	}
}

func normalizedReplicaCount(value int) int {
	switch {
	case value <= 0:
		return 1
	case value > 10:
		return 10
	default:
		return value
	}
}

func normalizedResourceProfile(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "starter":
		return "starter"
	case "performance":
		return "performance"
	case "balanced":
		return "balanced"
	default:
		return "balanced"
	}
}

func renderIngressYAML(serviceName, projectID, stageSlug, baseDomain string, servicePort int) string {
	var host string
	svc := sanitizeName(serviceName)
	proj := sanitizeName(projectID)
	if stageSlug == "" || stageSlug == "production" {
		host = fmt.Sprintf("%s.%s.%s", svc, proj, baseDomain)
	} else {
		host = fmt.Sprintf("%s.%s.%s.%s", svc, proj, sanitizeName(stageSlug), baseDomain)
	}
	name := sanitizeName(serviceName)
	return fmt.Sprintf(`apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: %s
spec:
  ingressClassName: nginx
  rules:
    - host: "%s"
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: %s
                port:
                  number: %d
`, name, host, name, servicePort)
}
