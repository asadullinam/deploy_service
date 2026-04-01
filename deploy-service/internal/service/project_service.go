package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"deploy-service/internal/domain"
)

// Проверка на этапе компиляции: ProjectService реализует Port.
var _ Port = (*ProjectService)(nil)

const (
	defaultReplicaCount       = 1
	defaultResourceProfile    = "balanced"
	defaultBillingGracePeriod = 24 * time.Hour

	stageEnvironmentUnavailableMessage = "не удалось создать контур: инфраструктура проекта недоступна. Проверьте, что проект активен, затем нажмите «Безопасно возобновить» и повторите попытку. Если проблема сохраняется, пересоздайте проект."
	kubeconfigUnavailableMessage       = "kubeconfig проекта пока недоступен: vcluster ещё запускается или недоступен. Подождите 20–60 секунд и попробуйте снова. Если проект был остановлен, возобновите его и повторите попытку."
)

func NewProjectService(
	store ProjectStore,
	releaseStore ReleaseStore,
	stageStore StageStore,
	provisioner Provisioner,
	automation GitHubAutomation,
	monetization MonetizationEngine,
	users UserStore,
	txStore BillingTransactionStore,
	crypto CryptoService,
	appsBaseDomain string,
	appsURLScheme string,
	appsURLPort string,
) *ProjectService {
	scheme := strings.TrimSpace(appsURLScheme)
	if scheme == "" {
		scheme = "https"
	}
	return &ProjectService{
		store:          store,
		releaseStore:   releaseStore,
		stageStore:     stageStore,
		provisioner:    provisioner,
		automation:     automation,
		monetization:   monetization,
		users:          users,
		txStore:        txStore,
		crypto:         crypto,
		appsBaseDomain: strings.TrimSpace(appsBaseDomain),
		appsURLScheme:  scheme,
		appsURLPort:    strings.TrimSpace(appsURLPort),

		pendingCharges:     make(map[string]float64),
		graceStartedAt:     make(map[string]time.Time),
		billingGracePeriod: defaultBillingGracePeriod,
		urlsCache:          make(map[string]cachedProjectURLs),
	}
}

func (s *ProjectService) SetBillingGuardGracePeriod(period time.Duration) {
	if period < 0 {
		return
	}
	s.billingGuardMu.Lock()
	s.billingGracePeriod = period
	s.billingGuardMu.Unlock()
}

func (s *ProjectService) SetBillingGuardRetentionPeriod(period time.Duration) {
	if period < 0 {
		return
	}
	s.billingGuardMu.Lock()
	s.billingRetentionPeriod = period
	s.billingGuardMu.Unlock()
}

func (s *ProjectService) SetLogReader(reader LogReader) {
	s.logReader = reader
}

func (s *ProjectService) SetNotificationService(notifications *NotificationService) {
	s.notifications = notifications
}

const createProjectTimeout = 15 * time.Minute

var projectProvisionRetryDelays = []time.Duration{
	5 * time.Second,
	10 * time.Second,
	20 * time.Second,
}

func (s *ProjectService) CreateProject(ctx context.Context, request domain.CreateProjectRequest) (domain.Project, error) {
	if strings.TrimSpace(request.Name) == "" {
		return domain.Project{}, errors.New("project name is required")
	}
	if strings.TrimSpace(request.OwnerID) == "" {
		return domain.Project{}, errors.New("owner identifier is required")
	}

	now := time.Now().UTC()
	projectID := domain.NewID()
	log.Printf("[%s] creating project name=%q owner=%s", projectID, request.Name, request.OwnerID)

	project := domain.Project{
		ID:              projectID,
		Name:            request.Name,
		OwnerID:         request.OwnerID,
		Status:          domain.ProjectStatusCreating,
		GrafanaURL:      s.projectGrafanaURL(domain.Project{ID: projectID}),
		ReplicaCount:    defaultReplicaCount,
		ResourceProfile: defaultResourceProfile,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	if err := s.store.Create(ctx, project); err != nil {
		return domain.Project{}, err
	}
	log.Printf("[%s] saved to store, provisioning in background...", projectID)

	go s.provisionProject(project)

	return project, nil
}

func (s *ProjectService) provisionProject(project domain.Project) {
	projectID := project.ID
	ctx, cancel := context.WithTimeout(context.Background(), createProjectTimeout)
	defer cancel()

	markFailed := func(reason string, err error) {
		log.Printf("[%s] %s: %v", projectID, reason, err)
		project.Status = domain.ProjectStatusFailed
		project.UpdatedAt = time.Now().UTC()
		if updateErr := s.store.Update(context.Background(), project); updateErr != nil {
			log.Printf("[%s] failed to mark project as failed: %v", projectID, updateErr)
		}
		s.notifyUser(context.Background(), project.OwnerID, "project-failed:"+project.ID, fmt.Sprintf("[critical] Проект %s не был подготовлен\nПричина: %s.\nПроверь настройки деплоя и статус инфраструктуры.", project.Name, reason), 30*time.Minute)
	}

	log.Printf("[%s] provisioning k8s environment...", projectID)
	projectAppsBaseDomain, err := s.createProjectEnvironmentWithRetry(ctx, projectID)
	if err != nil {
		markFailed("provisioner failed", err)
		return
	}
	if projectAppsBaseDomain != "" {
		project.AppsBaseDomain = projectAppsBaseDomain
		_ = s.store.Update(context.Background(), project)
	}
	log.Printf("[%s] k8s environment ready, setting up automation...", projectID)

	if err := s.automation.SetupProjectAutomation(ctx, projectID); err != nil {
		markFailed("automation failed", err)
		return
	}
	log.Printf("[%s] automation done", projectID)

	// Повторно читаем из хранилища, чтобы не перезаписать параллельные обновления полей
	// (например, PublicURL заполняется в GetProjectRuntimeStatus во время подготовки).
	if current, ok := s.store.GetByID(context.Background(), projectID); ok {
		current.Status = domain.ProjectStatusActive
		current.UpdatedAt = time.Now().UTC()
		project = current
	} else {
		project.Status = domain.ProjectStatusActive
		project.UpdatedAt = time.Now().UTC()
	}
	if err := s.store.Update(context.Background(), project); err != nil {
		log.Printf("[%s] failed to mark project as active: %v", projectID, err)
		return
	}

	log.Printf("[%s] project active, creating default production stage...", projectID)
	if _, stageErr := s.createStage(context.Background(), projectID, "Production", "production"); stageErr != nil {
		log.Printf("[%s] failed to create production stage (non-fatal): %v", projectID, stageErr)
	}

	log.Printf("[%s] project active", projectID)
	s.notifyUser(context.Background(), project.OwnerID, "project-active:"+project.ID, fmt.Sprintf("[info] Проект %s готов\nСреда создана, можно настраивать деплой и подключать репозиторий.", project.Name), 30*time.Minute)
}

func (s *ProjectService) createProjectEnvironmentWithRetry(ctx context.Context, projectID string) (string, error) {
	attempts := len(projectProvisionRetryDelays) + 1
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		if attempt > 1 {
			delay := projectProvisionRetryDelays[attempt-2]
			log.Printf("[%s] retrying project provisioning in %s (attempt %d/%d)", projectID, delay, attempt, attempts)
			if err := sleepWithContext(ctx, delay); err != nil {
				return "", err
			}
		}

		appsBaseDomain, err := s.provisioner.CreateProjectEnvironment(ctx, projectID)
		if err == nil {
			return appsBaseDomain, nil
		}

		lastErr = err
		if !isRetryableProjectProvisionError(err) || attempt == attempts {
			break
		}
		log.Printf("[%s] transient project provisioning error on attempt %d/%d: %v", projectID, attempt, attempts, err)
	}
	return "", lastErr
}

func isRetryableProjectProvisionError(err error) bool {
	if err == nil {
		return false
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "retry in a few seconds") ||
		strings.Contains(lower, "not ready yet") ||
		strings.Contains(lower, "failed to wait for kubeconfig secret") ||
		strings.Contains(lower, "context deadline exceeded") ||
		strings.Contains(lower, "timeout") ||
		strings.Contains(lower, "connection refused") ||
		strings.Contains(lower, "temporary failure") ||
		strings.Contains(lower, "temporarily unavailable") ||
		strings.Contains(lower, "untolerated taint") ||
		strings.Contains(lower, "failedscheduling")
}

func sleepWithContext(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (s *ProjectService) ListProjects(ctx context.Context) []domain.Project {
	projects := s.store.List(ctx)
	for i := range projects {
		projects[i] = s.hydrateProjectLinks(projects[i])
	}
	return projects
}

func (s *ProjectService) GetProject(ctx context.Context, projectID string) (domain.Project, error) {
	project, exists := s.store.GetByID(ctx, projectID)
	if !exists {
		return domain.Project{}, domain.ErrProjectNotFound
	}

	return s.hydrateProjectLinks(project), nil
}

func (s *ProjectService) DeleteProject(ctx context.Context, projectID string) error {
	project, exists := s.store.GetByID(ctx, projectID)
	if !exists {
		return domain.ErrProjectNotFound
	}

	project.Status = domain.ProjectStatusDeleting
	project.UpdatedAt = time.Now().UTC()

	if err := s.store.Update(ctx, project); err != nil {
		return err
	}

	if err := s.provisioner.DeleteProjectEnvironment(ctx, project.ID); err != nil {
		return err
	}

	project.Status = domain.ProjectStatusDeleted
	project.UpdatedAt = time.Now().UTC()

	if err := s.store.Update(ctx, project); err != nil {
		return err
	}
	s.invalidateProjectCaches(projectID)
	s.notifyUser(ctx, project.OwnerID, "project-deleted:"+project.ID, fmt.Sprintf("[critical] Проект %s удален\nОкружение и записи проекта удалены из платформы.", project.Name), 30*time.Minute)
	return nil
}

func (s *ProjectService) GetProjectCost(ctx context.Context, projectID string) (domain.CostBreakdown, error) {
	if _, exists := s.store.GetByID(ctx, projectID); !exists {
		return domain.CostBreakdown{}, domain.ErrProjectNotFound
	}

	return s.monetization.GetProjectCost(ctx, projectID)
}

func (s *ProjectService) UpdateProjectDeploymentSettings(ctx context.Context, projectID string, request domain.UpdateDeploymentSettingsRequest) (domain.Project, error) {
	project, exists := s.store.GetByID(ctx, projectID)
	if !exists {
		return domain.Project{}, domain.ErrProjectNotFound
	}

	project.RepositoryOwner = strings.TrimSpace(request.RepositoryOwner)
	project.RepositoryName = strings.TrimSpace(request.RepositoryName)
	project.BaseBranch = strings.TrimSpace(request.BaseBranch)
	project.ServiceName = strings.TrimSpace(request.ServiceName)
	project.DockerfilePath = strings.TrimSpace(request.DockerfilePath)
	project.ServiceType = strings.TrimSpace(request.ServiceType)
	project.ServicePort = request.ServicePort
	project.ContainerPort = request.ContainerPort
	project.DedicatedLoadBalancer = request.DedicatedLoadBalancer
	if request.ReplicaCount > 0 {
		project.ReplicaCount = normalizedReplicaCount(request.ReplicaCount)
	} else {
		project.ReplicaCount = normalizedReplicaCount(project.ReplicaCount)
	}
	if strings.TrimSpace(request.ResourceProfile) != "" {
		project.ResourceProfile = normalizedResourceProfile(request.ResourceProfile)
	} else {
		project.ResourceProfile = normalizedResourceProfile(project.ResourceProfile)
	}
	project.UpdatedAt = time.Now().UTC()

	if err := s.store.Update(ctx, project); err != nil {
		return domain.Project{}, err
	}

	return project, nil
}

func (s *ProjectService) BuildGitHubBootstrapQuestions(ctx context.Context, projectID string, request domain.GitHubBootstrapQuestionsRequest) (domain.GitHubBootstrapQuestionsResponse, error) {
	project, exists := s.store.GetByID(ctx, projectID)
	if !exists {
		return domain.GitHubBootstrapQuestionsResponse{}, domain.ErrProjectNotFound
	}

	request.RepositoryOwner, request.RepositoryName = resolveRepositoryForRequest(request.RepositoryOwner, request.RepositoryName, project)
	if request.RepositoryOwner == "" || request.RepositoryName == "" {
		return domain.GitHubBootstrapQuestionsResponse{}, errors.New("repository owner and repository name are required")
	}
	if strings.Contains(request.RepositoryName, "://") {
		return domain.GitHubBootstrapQuestionsResponse{}, errors.New("repositoryName must be a plain name (e.g. \"soa\"), not a URL")
	}
	if strings.Contains(request.DockerfilePath, "://") {
		return domain.GitHubBootstrapQuestionsResponse{}, errors.New("dockerfilePath must be a path inside the repo (e.g. \"hw1/Dockerfile\"), not a URL")
	}
	request.GitHubToken = strings.TrimSpace(request.GitHubToken)
	if request.GitHubToken == "" {
		token, err := s.resolveGitHubToken(ctx, project)
		if err != nil {
			return domain.GitHubBootstrapQuestionsResponse{}, err
		}
		request.GitHubToken = token
	}

	response, err := s.automation.BuildBootstrapQuestions(ctx, projectID, request)
	if err != nil {
		return domain.GitHubBootstrapQuestionsResponse{}, err
	}

	project.RepositoryOwner = strings.TrimSpace(request.RepositoryOwner)
	project.RepositoryName = strings.TrimSpace(request.RepositoryName)
	project.BaseBranch = strings.TrimSpace(response.BaseBranch)
	project.ServiceName = strings.TrimSpace(response.DetectedServiceName)
	project.DockerfilePath = strings.TrimSpace(response.DetectedDockerfile)
	project.ServiceType = strings.TrimSpace(response.DetectedServiceType)
	project.ServicePort = response.DetectedServicePort
	project.ContainerPort = response.DetectedContainerPort
	project.ReplicaCount = normalizedReplicaCount(project.ReplicaCount)
	project.ResourceProfile = normalizedResourceProfile(project.ResourceProfile)
	project.UpdatedAt = time.Now().UTC()
	if err := s.store.Update(ctx, project); err != nil {
		return domain.GitHubBootstrapQuestionsResponse{}, err
	}

	return response, nil
}

func (s *ProjectService) BootstrapGitHubFlow(ctx context.Context, projectID string, request domain.BootstrapGitHubFlowRequest) (domain.BootstrapGitHubFlowResponse, error) {
	project, exists := s.store.GetByID(ctx, projectID)
	if !exists {
		return domain.BootstrapGitHubFlowResponse{}, domain.ErrProjectNotFound
	}

	if project.Status == domain.ProjectStatusSuspended {
		return domain.BootstrapGitHubFlowResponse{}, errors.New("project is suspended, resume it first")
	}

	request.RepositoryOwner, request.RepositoryName = resolveRepositoryForRequest(request.RepositoryOwner, request.RepositoryName, project)
	if request.RepositoryOwner == "" || request.RepositoryName == "" {
		return domain.BootstrapGitHubFlowResponse{}, errors.New("repository owner and repository name are required")
	}
	if strings.Contains(request.RepositoryName, "://") {
		return domain.BootstrapGitHubFlowResponse{}, errors.New("repositoryName must be a plain name (e.g. \"soa\"), not a URL")
	}
	if strings.Contains(request.DockerfilePath, "://") {
		return domain.BootstrapGitHubFlowResponse{}, errors.New("dockerfilePath must be a path inside the repo (e.g. \"hw1/Dockerfile\"), not a URL")
	}
	request.GitHubToken = strings.TrimSpace(request.GitHubToken)
	if request.GitHubToken == "" {
		token, err := s.resolveGitHubToken(ctx, project)
		if err != nil {
			return domain.BootstrapGitHubFlowResponse{}, err
		}
		request.GitHubToken = token
	}

	stageSlug := slugifyStageName(request.StageSlug)
	if stageSlug == "" {
		stageSlug = "production"
	}
	request.StageSlug = stageSlug
	stageID := ""
	if stage, ok := s.stageStore.GetBySlug(ctx, projectID, stageSlug); ok {
		stageID = stage.ID
	} else {
		if stageSlug == "production" {
			stage, err := s.getOrCreateProductionStage(ctx, projectID)
			if err != nil {
				return domain.BootstrapGitHubFlowResponse{}, fmt.Errorf("ensure production stage: %w", err)
			}
			stageID = stage.ID
		} else {
			return domain.BootstrapGitHubFlowResponse{}, fmt.Errorf("stage with slug %q not found", stageSlug)
		}
	}

	releaseBudget, err := s.reserveDeployBudget(ctx, project, request.ServiceType, request.DedicatedLoadBalancer)
	if err != nil {
		return domain.BootstrapGitHubFlowResponse{}, err
	}
	defer releaseBudget()

	request.AppsBaseDomain = s.effectiveBaseDomain(project)
	response, err := s.automation.BootstrapRepositoryFlow(ctx, projectID, request)
	if err != nil {
		return domain.BootstrapGitHubFlowResponse{}, err
	}

	resolvedServiceType := normalizedServiceType(request.ServiceType)
	if resolvedServiceType == "" {
		resolvedServiceType = normalizedServiceType(project.ServiceType)
	}

	if s.effectiveBaseDomain(project) != "" && resolvedServiceType == "loadbalancer" && !project.DedicatedLoadBalancer && !request.DedicatedLoadBalancer {
		if project.PublicURL == "" {
			project.PublicURL = s.projectPublicURL(project)
		}
		if stage, ok := s.stageStore.GetBySlug(ctx, projectID, stageSlug); ok && stage.PublicURL == "" {
			stage.PublicURL = s.stagePublicURL(project, stageSlug)
			stage.UpdatedAt = time.Now().UTC()
			_ = s.stageStore.Update(ctx, stage)
		}
	}
	project.RepositoryOwner = strings.TrimSpace(request.RepositoryOwner)
	project.RepositoryName = strings.TrimSpace(request.RepositoryName)
	if value := strings.TrimSpace(request.BaseBranch); value != "" {
		project.BaseBranch = value
	}
	if value := strings.TrimSpace(request.ServiceName); value != "" {
		project.ServiceName = value
	}
	if value := strings.TrimSpace(request.DockerfilePath); value != "" {
		project.DockerfilePath = value
	}
	if value := strings.TrimSpace(request.ServiceType); value != "" {
		project.ServiceType = value
	}
	project.DedicatedLoadBalancer = request.DedicatedLoadBalancer
	if request.ServicePort > 0 {
		project.ServicePort = request.ServicePort
	}
	if request.ContainerPort > 0 {
		project.ContainerPort = request.ContainerPort
	}
	if request.ReplicaCount > 0 {
		project.ReplicaCount = normalizedReplicaCount(request.ReplicaCount)
	} else {
		project.ReplicaCount = normalizedReplicaCount(project.ReplicaCount)
	}
	if strings.TrimSpace(request.ResourceProfile) != "" {
		project.ResourceProfile = normalizedResourceProfile(request.ResourceProfile)
	} else {
		project.ResourceProfile = normalizedResourceProfile(project.ResourceProfile)
	}
	project.UpdatedAt = time.Now().UTC()
	if updateErr := s.store.Update(ctx, project); updateErr != nil {
		return domain.BootstrapGitHubFlowResponse{}, updateErr
	}
	if err := s.createBootstrapPendingRelease(ctx, project.ID, stageID, response); err != nil {
		return domain.BootstrapGitHubFlowResponse{}, err
	}

	return response, nil
}

func (s *ProjectService) effectiveBaseDomain(project domain.Project) string {
	if d := strings.TrimSpace(project.AppsBaseDomain); d != "" {
		return d
	}
	return s.appsBaseDomain
}

func (s *ProjectService) projectPublicURL(project domain.Project) string {
	baseDomain := s.effectiveBaseDomain(project)
	if baseDomain == "" {
		return ""
	}
	host := fmt.Sprintf("%s.%s", project.ID, baseDomain)
	if s.appsURLPort == "" || s.appsURLPort == "80" || s.appsURLPort == "443" {
		return fmt.Sprintf("%s://%s", s.appsURLScheme, host)
	}
	return fmt.Sprintf("%s://%s:%s", s.appsURLScheme, host, s.appsURLPort)
}

func (s *ProjectService) stagePublicURL(project domain.Project, stageSlug string) string {
	baseDomain := s.effectiveBaseDomain(project)
	if baseDomain == "" {
		return ""
	}
	var host string
	if stageSlug == "" || stageSlug == "production" {
		host = fmt.Sprintf("%s.%s", project.ID, baseDomain)
	} else {
		host = fmt.Sprintf("%s.%s.%s", project.ID, stageSlug, baseDomain)
	}
	if s.appsURLPort == "" || s.appsURLPort == "80" || s.appsURLPort == "443" {
		return fmt.Sprintf("%s://%s", s.appsURLScheme, host)
	}
	return fmt.Sprintf("%s://%s:%s", s.appsURLScheme, host, s.appsURLPort)
}

func (s *ProjectService) projectGrafanaURL(project domain.Project) string {
	baseDomain := s.effectiveBaseDomain(project)
	if baseDomain == "" {
		return ""
	}
	host := fmt.Sprintf("grafana-%s.%s", project.ID, baseDomain)
	path := fmt.Sprintf("/d/%s/project-metrics?orgId=1&refresh=30s", domain.ProjectGrafanaDashboardUID(project.ID))
	return fmt.Sprintf("%s://%s%s", s.appsURLScheme, host, path)
}

func (s *ProjectService) createBootstrapPendingRelease(ctx context.Context, projectID, stageID string, response domain.BootstrapGitHubFlowResponse) error {
	if strings.TrimSpace(response.PullRequestURL) == "" {
		return nil
	}

	now := time.Now().UTC()
	release := domain.Release{
		ID:            domain.NewID(),
		ProjectID:     projectID,
		StageID:       stageID,
		Status:        domain.ReleaseStatusPending,
		CommitMessage: fmt.Sprintf("Ожидает запуска деплоя из PR: %s", response.PullRequestURL),
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	return s.releaseStore.Create(ctx, release)
}

func (s *ProjectService) resolveReleaseStageID(ctx context.Context, projectID, stageSlug string) string {
	if stage, ok := s.stageStore.GetBySlug(ctx, projectID, stageSlug); ok {
		return stage.ID
	}
	if stageSlug == "production" {
		if stage, err := s.getOrCreateProductionStage(ctx, projectID); err == nil {
			return stage.ID
		}
	}
	return ""
}

func (s *ProjectService) findPendingReleaseWithoutWorkflow(ctx context.Context, projectID, stageID string) (domain.Release, bool) {
	releases := s.releaseStore.ListByProject(ctx, projectID)
	var match domain.Release
	found := false
	for _, release := range releases {
		if release.WorkflowRunID != 0 || release.StageID != stageID {
			continue
		}
		switch release.Status {
		case domain.ReleaseStatusPending, domain.ReleaseStatusBuilding, domain.ReleaseStatusDeploying:
		default:
			continue
		}
		if !found || release.CreatedAt.After(match.CreatedAt) {
			match = release
			found = true
		}
	}
	return match, found
}

func normalizedServiceType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "loadbalancer":
		return "loadbalancer"
	case "clusterip":
		return "clusterip"
	default:
		return ""
	}
}

func slugifyStageName(value string) string {
	raw := strings.TrimSpace(strings.ToLower(value))
	if raw == "" {
		return ""
	}

	var b strings.Builder
	lastDash := false
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
			lastDash = false
		case r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}

	return strings.Trim(b.String(), "-")
}

func estimateRequiredDeployReserve(serviceType string, dedicatedLoadBalancer bool, spentThisMonth float64, replicaCount int, resourceProfile string) float64 {
	baseReserve := 0.45
	if serviceType == "loadbalancer" {
		baseReserve = 0.85
		if dedicatedLoadBalancer {
			baseReserve += 0.75
		}
	}
	profileMultiplier := 1.0
	switch normalizedResourceProfile(resourceProfile) {
	case "starter":
		profileMultiplier = 0.9
	case "balanced":
		profileMultiplier = 1.15
	case "performance":
		profileMultiplier = 1.45
	}
	replicaMultiplier := math.Max(1, float64(normalizedReplicaCount(replicaCount)))
	baseReserve = baseReserve * profileMultiplier * (0.85 + 0.15*replicaMultiplier)
	usageReserve := math.Min(3, spentThisMonth*0.08)
	return baseReserve + usageReserve
}

func normalizedReplicaCount(value int) int {
	switch {
	case value <= 0:
		return defaultReplicaCount
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
		return defaultResourceProfile
	}
}

func (s *ProjectService) SuspendProject(ctx context.Context, projectID string) error {
	project, exists := s.store.GetByID(ctx, projectID)
	if !exists {
		return domain.ErrProjectNotFound
	}
	if project.Status != domain.ProjectStatusActive {
		return fmt.Errorf("can only suspend an active project, current status: %s", project.Status)
	}
	project.Status = domain.ProjectStatusSuspended
	project.UpdatedAt = time.Now().UTC()
	if err := s.store.Update(ctx, project); err != nil {
		return err
	}
	if err := s.provisioner.SuspendProjectEnvironment(ctx, projectID); err != nil {
		return err
	}
	s.notifyUser(ctx, project.OwnerID, "project-suspended:"+project.ID, fmt.Sprintf("[warning] Проект %s приостановлен\nВозобнови проект, когда будешь готов продолжить работу.", project.Name), 15*time.Minute)
	return nil
}

func (s *ProjectService) ResumeProject(ctx context.Context, projectID string) error {
	project, exists := s.store.GetByID(ctx, projectID)
	if !exists {
		return domain.ErrProjectNotFound
	}
	if project.Status != domain.ProjectStatusSuspended {
		return fmt.Errorf("can only resume a suspended project, current status: %s", project.Status)
	}
	if err := s.provisioner.ResumeProjectEnvironment(ctx, projectID); err != nil {
		return err
	}
	project.Status = domain.ProjectStatusActive
	project.UpdatedAt = time.Now().UTC()
	if err := s.store.Update(ctx, project); err != nil {
		return err
	}
	s.notifyUser(ctx, project.OwnerID, "project-resumed:"+project.ID, fmt.Sprintf("[info] Проект %s снова активен\nОкружение возобновлено, можно продолжать деплой и проверку статуса.", project.Name), 15*time.Minute)
	return nil
}

func (s *ProjectService) ListReleases(ctx context.Context, projectID string) ([]domain.Release, error) {
	project, exists := s.store.GetByID(ctx, projectID)
	if !exists {
		return nil, domain.ErrProjectNotFound
	}
	releases := s.releaseStore.ListByProject(ctx, projectID)
	s.syncLatestUnresolvedRelease(ctx, project, releases)
	return s.releaseStore.ListByProject(ctx, projectID), nil
}

func (s *ProjectService) GetRelease(ctx context.Context, projectID, releaseID string) (domain.Release, error) {
	r, exists := s.releaseStore.GetByID(ctx, releaseID)
	if !exists || r.ProjectID != projectID {
		return domain.Release{}, domain.ErrReleaseNotFound
	}
	return r, nil
}

func (s *ProjectService) HandleGitHubWebhook(ctx context.Context, payload domain.GitHubWorkflowRunPayload) error {
	release, exists := s.releaseStore.GetByWorkflowRunID(ctx, payload.WorkflowRun.ID)
	if !exists {
		repoOwner := strings.TrimSpace(payload.Repository.Owner.Login)
		repoName := strings.TrimSpace(payload.Repository.Name)
		if repoOwner == "" || repoName == "" {
			return nil
		}
		var project domain.Project
		found := false
		for _, p := range s.store.List(ctx) {
			if p.Status == domain.ProjectStatusDeleted || p.Status == domain.ProjectStatusDeleting {
				continue
			}
			if strings.EqualFold(strings.TrimSpace(p.RepositoryOwner), repoOwner) &&
				strings.EqualFold(strings.TrimSpace(p.RepositoryName), repoName) {
				project = p
				found = true
				break
			}
		}
		if !found {
			return nil
		}
		now := time.Now().UTC()

		// Определяем StageID: сначала берем явный stage_slug, затем выводим из пути workflow,
		// иначе используем "production".
		stageSlug := slugifyStageName(payload.WorkflowRun.StageSlug)
		if stageSlug == "" {
			stageSlug = stageSlugFromWorkflowPath(payload.WorkflowRun.Path)
		}
		if stageSlug == "" {
			stageSlug = "production"
		}
		stageID := s.resolveReleaseStageID(ctx, project.ID, stageSlug)
		if pending, ok := s.findPendingReleaseWithoutWorkflow(ctx, project.ID, stageID); ok {
			release = pending
			release.WorkflowRunID = payload.WorkflowRun.ID
			if headSHA := strings.TrimSpace(payload.WorkflowRun.HeadSHA); headSHA != "" {
				release.CommitSHA = headSHA
			}
			if message := strings.TrimSpace(payload.WorkflowRun.HeadCommit.Message); message != "" {
				release.CommitMessage = message
			}
			release.UpdatedAt = now
		} else {
			release = domain.Release{
				ID:            domain.NewID(),
				ProjectID:     project.ID,
				StageID:       stageID,
				Status:        domain.ReleaseStatusPending,
				WorkflowRunID: payload.WorkflowRun.ID,
				CommitSHA:     payload.WorkflowRun.HeadSHA,
				CommitMessage: payload.WorkflowRun.HeadCommit.Message,
				CreatedAt:     now,
				UpdatedAt:     now,
			}
			if err := s.releaseStore.Create(ctx, release); err != nil {
				return err
			}
		}
	}
	var newStatus domain.ReleaseStatus
	switch {
	case payload.WorkflowRun.Status == "in_progress":
		newStatus = domain.ReleaseStatusBuilding
	case payload.WorkflowRun.Status == "completed" && payload.WorkflowRun.Conclusion == "success":
		newStatus = domain.ReleaseStatusSuccess
	case payload.WorkflowRun.Status == "completed":
		newStatus = domain.ReleaseStatusFailed
	default:
		return nil
	}
	release.Status = newStatus
	if imageTag := strings.TrimSpace(payload.WorkflowRun.ImageTag); imageTag != "" {
		release.ImageTag = imageTag
	}
	release.UpdatedAt = time.Now().UTC()
	if err := s.releaseStore.Update(ctx, release); err != nil {
		return err
	}
	s.updateStageStatusFromRelease(ctx, release)
	return nil
}

func (s *ProjectService) syncLatestUnresolvedRelease(ctx context.Context, project domain.Project, releases []domain.Release) {
	repositoryOwner := strings.TrimSpace(project.RepositoryOwner)
	repositoryName := strings.TrimSpace(project.RepositoryName)
	if repositoryOwner == "" || repositoryName == "" {
		return
	}
	token, err := s.resolveGitHubToken(ctx, project)
	if err != nil {
		return
	}

	// Для каждой стадии: берём самый свежий релиз (любого статуса) чтобы знать
	// с какого момента запрашивать GitHub. Также собираем pending-без-workflow для привязки.
	type stageInfo struct {
		stageSlug       string
		since           time.Time
		pendingNoRunID  *domain.Release // pending созданный при bootstrap, без WorkflowRunID
	}
	byStage := map[string]*stageInfo{}

	for i := range releases {
		r := &releases[i]
		stageSlug := "production"
		if r.StageID != "" {
			if stage, ok := s.stageStore.GetByID(ctx, r.StageID); ok {
				stageSlug = stage.Slug
			}
		}
		si, ok := byStage[stageSlug]
		if !ok {
			si = &stageInfo{stageSlug: stageSlug}
			byStage[stageSlug] = si
		}
		// Обновляем since до самого позднего createdAt по стадии.
		if r.CreatedAt.After(si.since) {
			si.since = r.CreatedAt
		}
		// Запоминаем pending без привязанного workflow (создан при bootstrap).
		if r.WorkflowRunID == 0 &&
			(r.Status == domain.ReleaseStatusPending || r.Status == domain.ReleaseStatusBuilding) {
			if si.pendingNoRunID == nil || r.CreatedAt.After(si.pendingNoRunID.CreatedAt) {
				si.pendingNoRunID = &releases[i]
			}
		}
	}

	for _, si := range byStage {
		runs, err := s.automation.FindLatestDeployWorkflowRun(ctx, domain.GitHubWorkflowRunLookupRequest{
			RepositoryOwner: repositoryOwner,
			RepositoryName:  repositoryName,
			GitHubToken:     token,
			Since:           si.since,
			WorkflowPath:    stageWorkflowPath(si.stageSlug),
		})
		if err != nil || len(runs) == 0 {
			continue
		}

		for _, run := range runs {
			newStatus := mapWorkflowRunStatus(run.Status, run.Conclusion)
			if newStatus == "" {
				continue
			}

			// Уже есть релиз с таким run.ID — только обновляем статус если изменился.
			if existing, ok := s.releaseStore.GetByWorkflowRunID(ctx, run.ID); ok {
				if existing.Status != newStatus {
					existing.Status = newStatus
					if !run.UpdatedAt.IsZero() {
						existing.UpdatedAt = run.UpdatedAt.UTC()
					} else {
						existing.UpdatedAt = time.Now().UTC()
					}
					if err := s.releaseStore.Update(ctx, existing); err == nil {
						s.updateStageStatusFromRelease(ctx, existing)
						if newStatus == domain.ReleaseStatusSuccess {
							s.invalidateProjectCaches(project.ID)
						}
					}
				}
				continue
			}

			// Есть pending без WorkflowRunID (bootstrap) — привязываем к нему.
			if si.pendingNoRunID != nil {
				candidate := si.pendingNoRunID
				si.pendingNoRunID = nil // использован
				candidate.WorkflowRunID = run.ID
				if headSHA := strings.TrimSpace(run.HeadSHA); headSHA != "" {
					candidate.CommitSHA = headSHA
				}
				if candidate.CommitMessage == "" || strings.HasPrefix(candidate.CommitMessage, "Ожидает запуска деплоя из PR:") {
					candidate.CommitMessage = run.CommitMessage
				}
				candidate.Status = newStatus
				if !run.UpdatedAt.IsZero() {
					candidate.UpdatedAt = run.UpdatedAt.UTC()
				} else {
					candidate.UpdatedAt = time.Now().UTC()
				}
				if err := s.releaseStore.Update(ctx, *candidate); err == nil {
					s.updateStageStatusFromRelease(ctx, *candidate)
					if newStatus == domain.ReleaseStatusSuccess {
						s.invalidateProjectCaches(project.ID)
					}
				}
				continue
			}

			// Новый run — создаём новый Release.
			now := time.Now().UTC()
			stageID := s.resolveReleaseStageID(ctx, project.ID, si.stageSlug)
			newRelease := domain.Release{
				ID:            domain.NewID(),
				ProjectID:     project.ID,
				StageID:       stageID,
				Status:        newStatus,
				WorkflowRunID: run.ID,
				CommitSHA:     run.HeadSHA,
				CommitMessage: run.CommitMessage,
				CreatedAt:     now,
				UpdatedAt:     now,
			}
			if err := s.releaseStore.Create(ctx, newRelease); err == nil {
				s.updateStageStatusFromRelease(ctx, newRelease)
				if newStatus == domain.ReleaseStatusSuccess {
					s.invalidateProjectCaches(project.ID)
				}
			}
		}
	}
}

func mapWorkflowRunStatus(status string, conclusion string) domain.ReleaseStatus {
	switch {
	case status == "in_progress":
		return domain.ReleaseStatusBuilding
	case status == "completed" && conclusion == "success":
		return domain.ReleaseStatusSuccess
	case status == "completed":
		return domain.ReleaseStatusFailed
	case status == "queued", status == "requested", status == "waiting":
		return domain.ReleaseStatusPending
	default:
		return ""
	}
}

func releaseMessageForStatus(release domain.Release, status domain.ReleaseStatus) string {
	message := strings.TrimSpace(release.CommitMessage)
	switch status {
	case domain.ReleaseStatusSuccess:
		return "Деплой выполнен успешно."
	case domain.ReleaseStatusFailed:
		if message == "" || strings.HasPrefix(message, "Ожидает запуска деплоя из PR:") {
			return "Деплой завершился с ошибкой."
		}
	}
	return message
}

func (s *ProjectService) updateStageStatusFromRelease(ctx context.Context, release domain.Release) {
	if release.StageID == "" {
		return
	}

	stage, exists := s.stageStore.GetByID(ctx, release.StageID)
	if !exists {
		return
	}

	desiredStatus := stage.Status
	switch release.Status {
	case domain.ReleaseStatusSuccess:
		desiredStatus = domain.StageStatusActive
	case domain.ReleaseStatusFailed:
		desiredStatus = domain.StageStatusFailed
	default:
		return
	}

	if stage.Status == desiredStatus {
		return
	}

	stage.Status = desiredStatus
	stage.UpdatedAt = time.Now().UTC()
	_ = s.stageStore.Update(ctx, stage)
}

func (s *ProjectService) hydrateProjectLinks(project domain.Project) domain.Project {
	if s.effectiveBaseDomain(project) == "" {
		return project
	}

	if expectedGrafanaURL := s.projectGrafanaURL(project); strings.TrimSpace(expectedGrafanaURL) != "" {
		project.GrafanaURL = expectedGrafanaURL
	}

	if strings.TrimSpace(project.PublicURL) == "" && usesSharedProjectIngress(project) {
		project.PublicURL = s.projectPublicURL(project)
	}

	return project
}

func (s *ProjectService) reconcileProjectGrafana(ctx context.Context, project *domain.Project) {
	if project == nil || s.effectiveBaseDomain(*project) == "" {
		return
	}
	if project.Status == domain.ProjectStatusDeleted || project.Status == domain.ProjectStatusDeleting {
		return
	}

	expected := strings.TrimSpace(s.projectGrafanaURL(*project))
	if expected == "" {
		return
	}

	if provisioner, ok := s.provisioner.(interface {
		EnsureProjectGrafana(context.Context, string) error
	}); ok {
		_ = provisioner.EnsureProjectGrafana(ctx, project.ID)
	}

	if strings.TrimSpace(project.GrafanaURL) == expected {
		return
	}

	project.GrafanaURL = expected
	_ = s.store.Update(ctx, *project)
}

func (s *ProjectService) reconcileProjectPublicAccess(ctx context.Context, project *domain.Project) {
	if project == nil || s.effectiveBaseDomain(*project) == "" {
		return
	}
	if !usesSharedProjectIngress(*project) {
		return
	}

	expected := strings.TrimSpace(s.projectPublicURL(*project))
	if expected == "" {
		return
	}

	if provisioner, ok := s.provisioner.(interface {
		EnsureProjectPublicIngress(context.Context, string) error
	}); ok {
		_ = provisioner.EnsureProjectPublicIngress(ctx, project.ID)
	}

	if strings.TrimSpace(project.PublicURL) == expected {
		return
	}

	project.PublicURL = expected
	_ = s.store.Update(ctx, *project)
}

func (s *ProjectService) GetProjectKubeconfig(ctx context.Context, projectID string) (string, error) {
	project, exists := s.store.GetByID(ctx, projectID)
	if !exists {
		return "", domain.ErrProjectNotFound
	}

	if project.KubeconfigEncrypted != "" {
		return s.crypto.Decrypt(project.KubeconfigEncrypted)
	}

	// Еще не закэшировано — получаем, шифруем и сохраняем.
	kubeconfig, err := s.provisioner.GetProjectKubeconfig(ctx, projectID)
	if err != nil {
		return "", mapKubeconfigAvailabilityError(fmt.Errorf("fetch kubeconfig: %w", err))
	}

	encrypted, err := s.crypto.Encrypt(kubeconfig)
	if err != nil {
		return "", fmt.Errorf("encrypt kubeconfig: %w", err)
	}

	if err := s.store.UpdateKubeconfig(ctx, projectID, encrypted); err != nil {
		return "", fmt.Errorf("store kubeconfig: %w", err)
	}

	return kubeconfig, nil
}

func (s *ProjectService) RotateProjectKubeconfig(ctx context.Context, projectID string) (string, error) {
	if _, exists := s.store.GetByID(ctx, projectID); !exists {
		return "", domain.ErrProjectNotFound
	}

	kubeconfig, err := s.provisioner.GetProjectKubeconfig(ctx, projectID)
	if err != nil {
		return "", mapKubeconfigAvailabilityError(fmt.Errorf("fetch kubeconfig: %w", err))
	}

	encrypted, err := s.crypto.Encrypt(kubeconfig)
	if err != nil {
		return "", fmt.Errorf("encrypt kubeconfig: %w", err)
	}

	if err := s.store.UpdateKubeconfig(ctx, projectID, encrypted); err != nil {
		return "", fmt.Errorf("store kubeconfig: %w", err)
	}

	return kubeconfig, nil
}

func (s *ProjectService) GetProjectRuntimeStatus(ctx context.Context, projectID string) (domain.ProjectRuntimeStatus, error) {
	project, exists := s.store.GetByID(ctx, projectID)
	if !exists {
		return domain.ProjectRuntimeStatus{}, domain.ErrProjectNotFound
	}

	status, err := s.provisioner.GetProjectRuntimeStatus(ctx, projectID)
	if err != nil {
		return domain.ProjectRuntimeStatus{}, err
	}
	if status.ProjectID == "" {
		status.ProjectID = projectID
	}

	// Runtime может предоставить внешний endpoint (например, IP LoadBalancer),
	// даже когда APPS_BASE_DOMAIN не задан.
	if strings.TrimSpace(project.PublicURL) == "" &&
		normalizedServiceType(project.ServiceType) == "loadbalancer" &&
		strings.TrimSpace(status.PublicURL) != "" {
		project.PublicURL = strings.TrimSpace(status.PublicURL)
		project.UpdatedAt = time.Now().UTC()
		_ = s.store.Update(ctx, project)
	}

	return status, nil
}

func usesSharedProjectIngress(project domain.Project) bool {
	return normalizedServiceType(project.ServiceType) == "loadbalancer" && !project.DedicatedLoadBalancer
}

func (s *ProjectService) ListBillingTransactions(ctx context.Context, userID string) ([]domain.BillingTransaction, error) {
	return s.txStore.ListByUser(ctx, userID)
}

func (s *ProjectService) ListProjectBillingTransactions(ctx context.Context, projectID, userID string) ([]domain.BillingTransaction, error) {
	if _, exists := s.store.GetByID(ctx, projectID); !exists {
		return nil, domain.ErrProjectNotFound
	}
	project, _ := s.store.GetByID(ctx, projectID)
	if project.OwnerID != userID {
		return nil, domain.ErrForbidden
	}
	return s.txStore.ListByProject(ctx, projectID)
}

func (s *ProjectService) GetServiceGitHubTokenStatus(ctx context.Context, userID string) (domain.ServiceGitHubTokenStatus, error) {
	user, exists := s.users.GetByID(ctx, userID)
	if !exists {
		return domain.ServiceGitHubTokenStatus{}, domain.ErrUserNotFound
	}
	return domain.ServiceGitHubTokenStatus{
		Configured: strings.TrimSpace(user.GitHubTokenEncrypted) != "",
	}, nil
}

func (s *ProjectService) UpsertServiceGitHubToken(ctx context.Context, userID, token string) (domain.ServiceGitHubTokenStatus, error) {
	if strings.TrimSpace(token) == "" {
		return domain.ServiceGitHubTokenStatus{}, errors.New("github token is required")
	}
	if _, exists := s.users.GetByID(ctx, userID); !exists {
		return domain.ServiceGitHubTokenStatus{}, domain.ErrUserNotFound
	}
	encrypted, err := s.crypto.Encrypt(strings.TrimSpace(token))
	if err != nil {
		return domain.ServiceGitHubTokenStatus{}, fmt.Errorf("encrypt github token: %w", err)
	}
	if err := s.users.UpdateGitHubToken(ctx, userID, encrypted); err != nil {
		return domain.ServiceGitHubTokenStatus{}, err
	}
	return domain.ServiceGitHubTokenStatus{Configured: true}, nil
}

func (s *ProjectService) DeleteServiceGitHubToken(ctx context.Context, userID string) error {
	return s.users.UpdateGitHubToken(ctx, userID, "")
}

func (s *ProjectService) GetProjectGitHubToken(ctx context.Context, projectID, userID string) (domain.ServiceGitHubTokenStatus, error) {
	project, exists := s.store.GetByID(ctx, projectID)
	if !exists {
		return domain.ServiceGitHubTokenStatus{}, domain.ErrProjectNotFound
	}
	if project.OwnerID != userID {
		return domain.ServiceGitHubTokenStatus{}, domain.ErrForbidden
	}
	return domain.ServiceGitHubTokenStatus{
		Configured: strings.TrimSpace(project.GitHubTokenEncrypted) != "",
	}, nil
}

func (s *ProjectService) UpsertProjectGitHubToken(ctx context.Context, projectID, userID, token string) (domain.ServiceGitHubTokenStatus, error) {
	if strings.TrimSpace(token) == "" {
		return domain.ServiceGitHubTokenStatus{}, errors.New("github token is required")
	}
	project, exists := s.store.GetByID(ctx, projectID)
	if !exists {
		return domain.ServiceGitHubTokenStatus{}, domain.ErrProjectNotFound
	}
	if project.OwnerID != userID {
		return domain.ServiceGitHubTokenStatus{}, domain.ErrForbidden
	}
	encrypted, err := s.crypto.Encrypt(strings.TrimSpace(token))
	if err != nil {
		return domain.ServiceGitHubTokenStatus{}, fmt.Errorf("encrypt github token: %w", err)
	}
	if err := s.store.UpdateGitHubToken(ctx, projectID, encrypted); err != nil {
		return domain.ServiceGitHubTokenStatus{}, err
	}
	return domain.ServiceGitHubTokenStatus{Configured: true}, nil
}

func (s *ProjectService) DeleteProjectGitHubToken(ctx context.Context, projectID, userID string) error {
	project, exists := s.store.GetByID(ctx, projectID)
	if !exists {
		return domain.ErrProjectNotFound
	}
	if project.OwnerID != userID {
		return domain.ErrForbidden
	}
	return s.store.UpdateGitHubToken(ctx, projectID, "")
}

func (s *ProjectService) RollbackToRelease(ctx context.Context, projectID, releaseID string) (domain.Release, error) {
	project, exists := s.store.GetByID(ctx, projectID)
	if !exists {
		return domain.Release{}, domain.ErrProjectNotFound
	}

	target, exists := s.releaseStore.GetByID(ctx, releaseID)
	if !exists || target.ProjectID != projectID {
		return domain.Release{}, domain.ErrReleaseNotFound
	}
	if target.Status != domain.ReleaseStatusSuccess {
		return domain.Release{}, errors.New("can only rollback to a successful release")
	}
	if strings.TrimSpace(target.ImageTag) == "" {
		return domain.Release{}, errors.New("cannot rollback: target release has no image tag recorded")
	}

	releaseBudget, err := s.reserveDeployBudget(ctx, project, project.ServiceType, project.DedicatedLoadBalancer)
	if err != nil {
		return domain.Release{}, err
	}
	defer releaseBudget()

	now := time.Now().UTC()
	rollback := domain.Release{
		ID:            domain.NewID(),
		ProjectID:     projectID,
		StageID:       target.StageID,
		ImageTag:      target.ImageTag,
		CommitSHA:     target.CommitSHA,
		CommitMessage: fmt.Sprintf("rollback to %s", target.ID),
		Status:        domain.ReleaseStatusPending,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.releaseStore.Create(ctx, rollback); err != nil {
		return domain.Release{}, err
	}

	// Деплоим в тот же stage, что и целевой релиз.
	log.Printf("[rollback][%s] target release=%s stageID=%q imageTag=%q", projectID, target.ID, target.StageID, target.ImageTag)
	var applyErr error
	if target.StageID != "" {
		if stage, ok := s.stageStore.GetByID(ctx, target.StageID); ok {
			log.Printf("[rollback][%s] applying to stage slug=%q", projectID, stage.Slug)
			applyErr = s.provisioner.ApplyImageToStage(ctx, projectID, stage.Slug, target.ImageTag)
		} else {
			log.Printf("[rollback][%s] stageID=%q not found, falling back to ApplyImage", projectID, target.StageID)
			applyErr = s.provisioner.ApplyImage(ctx, projectID, target.ImageTag)
		}
	} else {
		log.Printf("[rollback][%s] no stageID, using ApplyImage", projectID)
		applyErr = s.provisioner.ApplyImage(ctx, projectID, target.ImageTag)
	}

	if applyErr != nil {
		rollback.Status = domain.ReleaseStatusFailed
		rollback.UpdatedAt = time.Now().UTC()
		s.releaseStore.Update(ctx, rollback) //nolint:errcheck
		return domain.Release{}, applyErr
	}
	rollback.Status = domain.ReleaseStatusSuccess
	rollback.UpdatedAt = time.Now().UTC()
	if err := s.releaseStore.Update(ctx, rollback); err != nil {
		return domain.Release{}, err
	}
	return rollback, nil
}

func (s *ProjectService) getOrCreateProductionStage(ctx context.Context, projectID string) (domain.Stage, error) {
	if stage, ok := s.stageStore.GetBySlug(ctx, projectID, "production"); ok {
		return stage, nil
	}
	return s.createStage(ctx, projectID, "Production", "production")
}

func mapStageEnvironmentError(err error) error {
	return mapProjectEnvironmentError(err, stageEnvironmentUnavailableMessage)
}

func mapKubeconfigAvailabilityError(err error) error {
	return mapProjectEnvironmentError(err, kubeconfigUnavailableMessage)
}

func mapProjectEnvironmentError(err error, userMessage string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, domain.ErrProjectEnvironmentUnavailable) {
		return err
	}

	lower := strings.ToLower(err.Error())
	if strings.Contains(lower, "get vcluster kubeconfig for project") ||
		strings.Contains(lower, "get kubeconfig secret for project") ||
		(strings.Contains(lower, "kubeconfig secret") && strings.Contains(lower, "not ready")) ||
		strings.Contains(lower, "ensure external endpoint for project") ||
		strings.Contains(lower, "external ip for vcluster service") ||
		strings.Contains(lower, "ylb.networkloadbalancers.count") ||
		(strings.Contains(lower, "namespaces \"project-") && strings.Contains(lower, "not found")) ||
		(strings.Contains(lower, "secrets \"vc-vcluster-") && strings.Contains(lower, "not found")) {
		return fmt.Errorf("%w: %s", domain.ErrProjectEnvironmentUnavailable, userMessage)
	}
	return err
}

func resolveRepositoryForRequest(requestOwner, requestName string, project domain.Project) (string, string) {
	repositoryOwner := strings.TrimSpace(requestOwner)
	repositoryName := strings.TrimSpace(requestName)
	if repositoryOwner == "" {
		repositoryOwner = strings.TrimSpace(project.RepositoryOwner)
	}
	if repositoryName == "" {
		repositoryName = strings.TrimSpace(project.RepositoryName)
	}
	return repositoryOwner, repositoryName
}

// resolveGitHubToken возвращает токен уровня проекта, а если он не задан — использует токен пользователя.
func (s *ProjectService) resolveGitHubToken(ctx context.Context, project domain.Project) (string, error) {
	if encrypted := strings.TrimSpace(project.GitHubTokenEncrypted); encrypted != "" {
		token, err := s.crypto.Decrypt(encrypted)
		if err != nil {
			return "", fmt.Errorf("decrypt project github token: %w", err)
		}
		return strings.TrimSpace(token), nil
	}
	return s.serviceGitHubToken(ctx, project.OwnerID)
}

func (s *ProjectService) serviceGitHubToken(ctx context.Context, userID string) (string, error) {
	user, exists := s.users.GetByID(ctx, userID)
	if !exists {
		return "", domain.ErrUserNotFound
	}
	encrypted := strings.TrimSpace(user.GitHubTokenEncrypted)
	if encrypted == "" {
		return "", nil
	}
	token, err := s.crypto.Decrypt(encrypted)
	if err != nil {
		return "", fmt.Errorf("decrypt github token: %w", err)
	}
	return strings.TrimSpace(token), nil
}

// createStage — внутренний хелпер, используемый CreateProject и CreateStage.
func (s *ProjectService) createStage(ctx context.Context, projectID, name, slug string) (domain.Stage, error) {
	project, _ := s.store.GetByID(ctx, projectID)
	now := time.Now().UTC()
	stage := domain.Stage{
		ID:        domain.NewID(),
		ProjectID: projectID,
		Name:      name,
		Slug:      slug,
		Status:    domain.StageStatusCreating,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.stageStore.Create(ctx, stage); err != nil {
		return domain.Stage{}, err
	}
	if err := s.provisioner.CreateStageEnvironment(ctx, projectID, slug); err != nil {
		log.Printf("[stage][%s] create stage environment failed for slug=%s: %v", projectID, slug, err)
		stage.Status = domain.StageStatusFailed
		stage.UpdatedAt = time.Now().UTC()
		_ = s.stageStore.Update(ctx, stage)
		if project.OwnerID != "" {
			s.notifyUser(ctx, project.OwnerID, "stage-failed:"+projectID+":"+slug, fmt.Sprintf("[critical] Контур %s для проекта %s не создался\nПроверь, что базовая инфраструктура проекта доступна, и попробуй повторить позже.", name, project.Name), 30*time.Minute)
		}
		return domain.Stage{}, mapStageEnvironmentError(err)
	}
	stage.Status = domain.StageStatusActive
	stage.UpdatedAt = time.Now().UTC()
	if err := s.stageStore.Update(ctx, stage); err != nil {
		return domain.Stage{}, err
	}
	s.invalidateProjectCaches(projectID)
	if project.OwnerID != "" {
		s.notifyUser(ctx, project.OwnerID, "stage-created:"+projectID+":"+slug, fmt.Sprintf("[info] Контур %s для проекта %s готов\nМожно запускать деплой в этот stage.", name, project.Name), 30*time.Minute)
	}
	return stage, nil
}

func (s *ProjectService) notifyUser(ctx context.Context, userID, key, text string, ttl time.Duration) {
	if s.notifications == nil || userID == "" {
		return
	}
	if err := s.notifications.SendUserAlert(ctx, userID, key, text, ttl); err != nil {
		log.Printf("notify user %s failed: %v", userID, err)
	}
}

func (s *ProjectService) CreateStage(ctx context.Context, projectID, userID string, req domain.CreateStageRequest) (domain.Stage, error) {
	project, exists := s.store.GetByID(ctx, projectID)
	if !exists {
		return domain.Stage{}, domain.ErrProjectNotFound
	}
	if project.OwnerID != userID {
		return domain.Stage{}, domain.ErrForbidden
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return domain.Stage{}, errors.New("stage name is required")
	}
	slug := slugifyStageName(name)
	if slug == "" {
		return domain.Stage{}, errors.New("stage name must contain letters or numbers")
	}
	if _, ok := s.stageStore.GetBySlug(ctx, projectID, slug); ok {
		return domain.Stage{}, fmt.Errorf("stage with slug %q already exists", slug)
	}
	if _, err := s.provisioner.GetProjectKubeconfig(ctx, projectID); err != nil {
		log.Printf("[stage][%s] prevalidation failed: %v", projectID, err)
		return domain.Stage{}, mapStageEnvironmentError(err)
	}
	return s.createStage(ctx, projectID, name, slug)
}

func (s *ProjectService) ListStages(ctx context.Context, projectID, userID string) ([]domain.Stage, error) {
	project, exists := s.store.GetByID(ctx, projectID)
	if !exists {
		return nil, domain.ErrProjectNotFound
	}
	if project.OwnerID != userID {
		return nil, domain.ErrForbidden
	}
	stages := s.stageStore.ListByProject(ctx, projectID)

	// Backfill: если stage отсутствуют и проект активен, лениво создаем production stage.
	// Пропускаем, если проект еще в provisioning, чтобы избежать гонок с goroutine provisionProject.
	if len(stages) == 0 && project.Status == domain.ProjectStatusActive {
		if stage, err := s.getOrCreateProductionStage(ctx, projectID); err == nil {
			stages = []domain.Stage{stage}
		}
	}
	return stages, nil
}

func (s *ProjectService) GetStage(ctx context.Context, projectID, stageID, userID string) (domain.Stage, error) {
	project, exists := s.store.GetByID(ctx, projectID)
	if !exists {
		return domain.Stage{}, domain.ErrProjectNotFound
	}
	if project.OwnerID != userID {
		return domain.Stage{}, domain.ErrForbidden
	}
	stage, ok := s.stageStore.GetByID(ctx, stageID)
	if !ok || stage.ProjectID != projectID {
		return domain.Stage{}, domain.ErrStageNotFound
	}
	return stage, nil
}

func (s *ProjectService) DeleteStage(ctx context.Context, projectID, stageID, userID string) error {
	project, exists := s.store.GetByID(ctx, projectID)
	if !exists {
		return domain.ErrProjectNotFound
	}
	if project.OwnerID != userID {
		return domain.ErrForbidden
	}
	stage, ok := s.stageStore.GetByID(ctx, stageID)
	if !ok || stage.ProjectID != projectID {
		return domain.ErrStageNotFound
	}
	if stage.Slug == "production" {
		return errors.New("cannot delete the production stage")
	}
	stage.Status = domain.StageStatusDeleting
	stage.UpdatedAt = time.Now().UTC()
	if err := s.stageStore.Update(ctx, stage); err != nil {
		return err
	}
	if err := s.provisioner.DeleteStageEnvironment(ctx, projectID, stage.Slug); err != nil {
		return err
	}
	stage.Status = domain.StageStatusDeleted
	stage.UpdatedAt = time.Now().UTC()
	if err := s.stageStore.Update(ctx, stage); err != nil {
		return err
	}
	s.invalidateProjectCaches(projectID)
	return nil
}

func (s *ProjectService) GetStageRuntimeStatus(ctx context.Context, projectID, stageID, userID string) (domain.ProjectRuntimeStatus, error) {
	project, exists := s.store.GetByID(ctx, projectID)
	if !exists {
		return domain.ProjectRuntimeStatus{}, domain.ErrProjectNotFound
	}
	if project.OwnerID != userID {
		return domain.ProjectRuntimeStatus{}, domain.ErrForbidden
	}
	stage, ok := s.stageStore.GetByID(ctx, stageID)
	if !ok || stage.ProjectID != projectID {
		return domain.ProjectRuntimeStatus{}, domain.ErrStageNotFound
	}

	status, err := s.provisioner.GetStageRuntimeStatus(ctx, projectID, stage.Slug)
	if err != nil {
		return domain.ProjectRuntimeStatus{}, err
	}

	updatedStage := stage
	stageChanged := false

	if strings.TrimSpace(status.PublicURL) != "" && strings.TrimSpace(updatedStage.PublicURL) == "" {
		updatedStage.PublicURL = strings.TrimSpace(status.PublicURL)
		stageChanged = true
	}

	// Если stage ранее был помечен failed (например, из-за временного таймаута bootstrap),
	// но runtime уже имеет готовые реплики, возвращаем его в active.
	if status.ReadyReplicas > 0 && updatedStage.Status != domain.StageStatusActive {
		updatedStage.Status = domain.StageStatusActive
		stageChanged = true
	}

	if stageChanged {
		updatedStage.UpdatedAt = time.Now().UTC()
		_ = s.stageStore.Update(ctx, updatedStage)
	}

	if strings.TrimSpace(project.PublicURL) == "" &&
		normalizedServiceType(project.ServiceType) == "loadbalancer" &&
		strings.TrimSpace(status.PublicURL) != "" {
		project.PublicURL = strings.TrimSpace(status.PublicURL)
		project.UpdatedAt = time.Now().UTC()
		_ = s.store.Update(ctx, project)
	}

	return status, nil
}

func (s *ProjectService) ListProjectLogs(ctx context.Context, projectID, userID string, request domain.ProjectLogsRequest) (domain.ProjectLogsResponse, error) {
	project, ok := s.store.GetByID(ctx, projectID)
	if !ok {
		return domain.ProjectLogsResponse{}, domain.ErrProjectNotFound
	}
	if project.OwnerID != userID {
		return domain.ProjectLogsResponse{}, domain.ErrForbidden
	}
	if s.logReader == nil {
		return domain.ProjectLogsResponse{}, domain.ErrLogsUnavailable
	}
	if request.Limit <= 0 {
		request.Limit = 200
	}
	if request.Limit > 1000 {
		request.Limit = 1000
	}
	if strings.TrimSpace(request.Since) == "" {
		request.Since = "15m"
	}
	if request.StageID != "" {
		stage, ok := s.stageStore.GetByID(ctx, request.StageID)
		if !ok || stage.ProjectID != projectID {
			return domain.ProjectLogsResponse{}, domain.ErrStageNotFound
		}
		request.StageSlug = stage.Slug
		request.StageID = stage.ID
	}
	return s.logReader.ListProjectLogs(ctx, projectID, request)
}

const urlsCacheTTL = 2 * time.Minute

func (s *ProjectService) GetProjectURLs(ctx context.Context, projectID, userID string) (domain.ProjectURLsResponse, error) {
	project, exists := s.store.GetByID(ctx, projectID)
	if !exists {
		return domain.ProjectURLsResponse{}, domain.ErrProjectNotFound
	}
	if project.OwnerID != userID {
		return domain.ProjectURLsResponse{}, domain.ErrForbidden
	}

	s.perfMu.RLock()
	cached, ok := s.urlsCache[projectID]
	s.perfMu.RUnlock()
	if ok && time.Now().Before(cached.expiresAt) {
		return cached.value, nil
	}

	stages := s.stageStore.ListByProject(ctx, projectID)
	result := domain.ProjectURLsResponse{
		Stages: make([]domain.StageURLs, 0, len(stages)),
	}
	for _, stage := range stages {
		if stage.Status == domain.StageStatusDeleted || stage.Status == domain.StageStatusDeleting {
			continue
		}
		status, err := s.provisioner.GetStageRuntimeStatus(ctx, projectID, stage.Slug)
		if err != nil {
			result.Stages = append(result.Stages, domain.StageURLs{
				StageID:   stage.ID,
				StageName: stage.Name,
				Slug:      stage.Slug,
				Services:  nil,
			})
			continue
		}
		result.Stages = append(result.Stages, domain.StageURLs{
			StageID:   stage.ID,
			StageName: stage.Name,
			Slug:      stage.Slug,
			Services:  status.ServiceURLs,
		})
	}

	s.perfMu.Lock()
	s.urlsCache[projectID] = cachedProjectURLs{value: result, expiresAt: time.Now().Add(urlsCacheTTL)}
	s.perfMu.Unlock()

	return result, nil
}

// stageWorkflowPath возвращает путь к workflow-файлу для указанного slug stage.
func stageWorkflowPath(stageSlug string) string {
	if stageSlug == "" || stageSlug == "production" {
		return ".github/workflows/deploy-service.yml"
	}
	return fmt.Sprintf(".github/workflows/deploy-service-%s.yml", stageSlug)
}

// stageSlugFromWorkflowPath определяет целевой stage по пути к workflow GitHub Actions.
// Примеры:
// ".github/workflows/deploy-service.yml"         -> "production"
// ".github/workflows/deploy-service-staging.yml" -> "staging"
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

func (s *ProjectService) invalidateProjectCaches(projectID string) {
	s.perfMu.Lock()
	defer s.perfMu.Unlock()

	delete(s.costCache, projectID)
	delete(s.releasesCache, projectID)
	delete(s.urlsCache, projectID)
}
