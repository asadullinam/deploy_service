package api_test

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

// TestE2E_ScenarioA покрывает полный путь пользователя от авторизации до бутстрапа:
// регистрация -> вход -> создание проекта -> сохранение настроек деплоя ->
// github/questions -> github/bootstrap -> получение kubeconfig
func TestE2E_ScenarioA(t *testing.T) {
	// 1. Регистрация
	email := uniqueEmail()
	var regResp struct {
		Token string `json:"token"`
	}
	r := do("POST", "/auth/register", map[string]any{"email": email, "password": "password123"}, nil)
	checkResp(t, r, http.StatusCreated, &regResp)
	if regResp.Token == "" {
		t.Fatal("expected token from register")
	}
	tok := regResp.Token

	// 2. Вход
	var loginResp struct {
		Token string `json:"token"`
	}
	r = do("POST", "/auth/login", map[string]any{"email": email, "password": "password123"}, nil)
	checkResp(t, r, http.StatusOK, &loginResp)
	if loginResp.Token == "" {
		t.Fatal("expected token from login")
	}
	tok = loginResp.Token

	// 3. Создание проекта
	var proj struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	r = do("POST", "/projects", map[string]any{"name": "e2e-scenario-a"}, bearer(tok))
	checkResp(t, r, http.StatusAccepted, &proj)
	if proj.ID == "" {
		t.Fatal("expected project ID")
	}
	projectID := proj.ID
	waitForProjectActive(t, tok, projectID)

	// 4. Сохранение настроек деплоя
	settings := map[string]any{
		"serviceName":     "e2e-svc",
		"dockerfilePath":  "Dockerfile",
		"containerPort":   8080,
		"servicePort":     80,
		"serviceType":     "LoadBalancer",
		"baseBranch":      "main",
		"repositoryOwner": "e2e-owner",
		"repositoryName":  "e2e-repo",
	}
	r = do("PUT", "/projects/"+projectID+"/deployment-settings", settings, bearer(tok))
	var updatedProj struct {
		ServiceName string `json:"serviceName"`
	}
	checkResp(t, r, http.StatusOK, &updatedProj)
	if updatedProj.ServiceName != "e2e-svc" {
		t.Errorf("serviceName after settings save: got %q, want %q", updatedProj.ServiceName, "e2e-svc")
	}

	// Проверяем, что настройки сохраняются через GET
	var fetchedProj struct {
		RepositoryOwner string `json:"repositoryOwner"`
		RepositoryName  string `json:"repositoryName"`
	}
	r = do("GET", "/projects/"+projectID, nil, bearer(tok))
	checkResp(t, r, http.StatusOK, &fetchedProj)
	if fetchedProj.RepositoryOwner != "e2e-owner" {
		t.Errorf("repositoryOwner: got %q, want %q", fetchedProj.RepositoryOwner, "e2e-owner")
	}

	// 4b. Пополняем баланс, чтобы бутстрап мог продолжиться
	r = do("POST", "/billing/top-up", map[string]any{"amountRub": 100.0}, bearer(tok))
	checkResp(t, r, http.StatusOK, nil)

	// 5. Вопросы GitHub
	var questionsResp struct {
		RepositoryOwner string `json:"repositoryOwner"`
		Questions       []any  `json:"questions"`
	}
	questionsBody := map[string]any{
		"repositoryOwner": "e2e-owner",
		"repositoryName":  "e2e-repo",
		"baseBranch":      "main",
		"dockerfilePath":  "Dockerfile",
		"githubToken":     "mock-token",
	}
	r = do("POST", "/projects/"+projectID+"/github/questions", questionsBody, bearer(tok))
	checkResp(t, r, http.StatusOK, &questionsResp)
	if questionsResp.RepositoryOwner != "e2e-owner" {
		t.Errorf("questions repositoryOwner: got %q, want %q", questionsResp.RepositoryOwner, "e2e-owner")
	}
	if len(questionsResp.Questions) == 0 {
		t.Error("expected at least one question in response")
	}

	// 6. Бутстрап
	var bootstrapResp struct {
		BranchName     string `json:"branchName"`
		PullRequestURL string `json:"pullRequestUrl"`
	}
	bootstrapBody := map[string]any{
		"repositoryOwner": "e2e-owner",
		"repositoryName":  "e2e-repo",
		"baseBranch":      "main",
		"serviceName":     "e2e-svc",
		"containerPort":   8080,
		"servicePort":     80,
		"serviceType":     "LoadBalancer",
		"dockerfilePath":  "Dockerfile",
		"githubToken":     "mock-token",
	}
	r = do("POST", "/projects/"+projectID+"/github/bootstrap", bootstrapBody, bearer(tok))
	checkResp(t, r, http.StatusOK, &bootstrapResp)
	if bootstrapResp.BranchName == "" {
		t.Error("expected branch name in bootstrap response")
	}
	if bootstrapResp.PullRequestURL == "" {
		t.Error("expected pull request URL in bootstrap response")
	}

	// 7. Получение kubeconfig (возвращает обычный YAML, не JSON)
	r = do("GET", "/projects/"+projectID+"/kubeconfig", nil, bearer(tok))
	defer r.Body.Close()
	if r.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(r.Body)
		t.Fatalf("kubeconfig: got %d, want 200; body: %s", r.StatusCode, body)
	}
	body, _ := io.ReadAll(r.Body)
	if len(body) == 0 {
		t.Error("expected non-empty kubeconfig body")
	}
}

// TestE2E_ScenarioB покрывает полный жизненный цикл релиза через webhook:
// создание проекта -> webhook in_progress -> webhook success -> проверка релизов -> откат
func TestE2E_ScenarioB(t *testing.T) {
	_, tok := setupUser(t)
	projectID := createProject(t, tok, "e2e-scenario-b")

	// Задаем repositoryOwner/repositoryName, чтобы webhook смог сопоставить проект
	// при автосоздании релиза.
	r := do("PUT", "/projects/"+projectID+"/deployment-settings", map[string]any{
		"repositoryOwner": "e2e-org",
		"repositoryName":  "e2e-svc-repo",
		"baseBranch":      "main",
		"serviceName":     "svc",
	}, bearer(tok))
	checkResp(t, r, http.StatusOK, nil)

	const runID = int64(77001)

	// 1. Webhook: workflow in_progress — должен автоматически создать релиз
	sendWebhook := func(t *testing.T, status, conclusion string) {
		t.Helper()
		payload := map[string]any{
			"action": "workflow_run",
			"workflow_run": map[string]any{
				"id":         runID,
				"status":     status,
				"conclusion": conclusion,
				"head_sha":   "deadbeef",
				"image_tag":  "e2e-org/e2e-svc-repo:deadbeef",
				"head_commit": map[string]any{
					"message": "chore: e2e deploy",
				},
			},
			"repository": map[string]any{
				"name": "e2e-svc-repo",
				"owner": map[string]any{
					"login": "e2e-org",
				},
			},
		}
		body, _ := json.Marshal(payload)
		sig := githubSig(body, testWebhookSecret)
		resp := doWebhook(body, map[string]string{
			"X-GitHub-Event":      "workflow_run",
			"X-Hub-Signature-256": sig,
		})
		resp.Body.Close()
		if resp.StatusCode != http.StatusNoContent {
			t.Errorf("webhook: got %d, want 204", resp.StatusCode)
		}
	}

	sendWebhook(t, "in_progress", "")

	// 2. Список релизов должен содержать автосозданный релиз со статусом building
	var releases []struct {
		ID            string `json:"id"`
		Status        string `json:"status"`
		WorkflowRunID int64  `json:"workflowRunId"`
		CommitSHA     string `json:"commitSha"`
	}
	r = do("GET", "/projects/"+projectID+"/releases", nil, bearer(tok))
	checkResp(t, r, http.StatusOK, &releases)
	if len(releases) != 1 {
		t.Fatalf("expected 1 release after in_progress webhook, got %d", len(releases))
	}
	rel := releases[0]
	if rel.WorkflowRunID != runID {
		t.Errorf("workflowRunId: got %d, want %d", rel.WorkflowRunID, runID)
	}
	if rel.Status != "building" {
		t.Errorf("status after in_progress: got %q, want %q", rel.Status, "building")
	}
	if rel.CommitSHA != "deadbeef" {
		t.Errorf("commitSha: got %q, want %q", rel.CommitSHA, "deadbeef")
	}
	releaseID := rel.ID

	// 3. Webhook: workflow completed success
	sendWebhook(t, "completed", "success")

	r = do("GET", "/projects/"+projectID+"/releases/"+releaseID, nil, bearer(tok))
	var finalRelease struct {
		Status string `json:"status"`
	}
	checkResp(t, r, http.StatusOK, &finalRelease)
	if finalRelease.Status != "success" {
		t.Errorf("status after completed+success: got %q, want %q", finalRelease.Status, "success")
	}

	// 3b. Откат защищен биллинговым ограничением, поэтому сначала пополняем баланс.
	r = do("POST", "/billing/top-up", map[string]any{"amountRub": 100.0}, bearer(tok))
	checkResp(t, r, http.StatusOK, nil)

	// 4. Откат — должен создать новую запись релиза
	var rollbackResp struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	r = do("POST", "/projects/"+projectID+"/releases/"+releaseID+"/rollback", nil, bearer(tok))
	checkResp(t, r, http.StatusOK, &rollbackResp)
	if rollbackResp.ID == "" {
		t.Error("expected new release ID from rollback")
	}
	if rollbackResp.ID == releaseID {
		t.Error("rollback should create a new release, not return the same ID")
	}

	// 5. Список релизов теперь должен содержать 2 записи
	var allReleases []map[string]any
	r = do("GET", "/projects/"+projectID+"/releases", nil, bearer(tok))
	checkResp(t, r, http.StatusOK, &allReleases)
	if len(allReleases) != 2 {
		t.Errorf("expected 2 releases after rollback, got %d", len(allReleases))
	}
}
