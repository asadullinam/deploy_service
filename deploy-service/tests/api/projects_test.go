package api_test

import (
	"net/http"
	"testing"
)

// T-010: Создание проекта возвращает 202 и объект проекта, затем он становится активным.
func TestT010_CreateProject(t *testing.T) {
	_, token := setupUser(t)
	var proj struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Status string `json:"status"`
	}
	r := do("POST", "/projects", map[string]any{"name": "test-project"}, bearer(token))
	checkResp(t, r, http.StatusAccepted, &proj)
	if proj.ID == "" {
		t.Error("expected non-empty project ID")
	}
	if proj.Name != "test-project" {
		t.Errorf("name: got %q, want %q", proj.Name, "test-project")
	}
	if proj.Status == "" {
		t.Error("expected non-empty status")
	}
	waitForProjectActive(t, token, proj.ID)
}

// T-011: Список проектов возвращает 200 и JSON-массив.
func TestT011_ListProjects(t *testing.T) {
	_, token := setupUser(t)
	createProject(t, token, "list-proj-1")

	r := do("GET", "/projects", nil, bearer(token))
	var projects []map[string]any
	checkResp(t, r, http.StatusOK, &projects)
	if len(projects) == 0 {
		t.Error("expected at least one project in list")
	}
}

// T-012: Получение проекта по ID возвращает 200 и объект проекта.
func TestT012_GetProject(t *testing.T) {
	_, token := setupUser(t)
	id := createProject(t, token, "get-proj")

	var proj struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	r := do("GET", "/projects/"+id, nil, bearer(token))
	checkResp(t, r, http.StatusOK, &proj)
	if proj.ID != id {
		t.Errorf("id: got %q, want %q", proj.ID, id)
	}
}

// T-013: Получение несуществующего проекта возвращает 404.
func TestT013_GetProjectNotFound(t *testing.T) {
	_, token := setupUser(t)
	var errResp struct {
		Error string `json:"error"`
	}
	r := do("GET", "/projects/nonexistent-id", nil, bearer(token))
	checkResp(t, r, http.StatusNotFound, &errResp)
	if errResp.Error == "" {
		t.Error("expected error message in body")
	}
}

// T-014: Удаление существующего проекта возвращает 200.
func TestT014_DeleteProject(t *testing.T) {
	_, token := setupUser(t)
	id := createProject(t, token, "delete-proj")

	var resp struct {
		Status string `json:"status"`
	}
	r := do("DELETE", "/projects/"+id, nil, bearer(token))
	checkResp(t, r, http.StatusOK, &resp)
	if resp.Status != "deleted" {
		t.Errorf("status: got %q, want %q", resp.Status, "deleted")
	}
}

// T-015: Удаление несуществующего проекта возвращает 404.
func TestT015_DeleteProjectNotFound(t *testing.T) {
	_, token := setupUser(t)
	var errResp struct {
		Error string `json:"error"`
	}
	r := do("DELETE", "/projects/nonexistent-id", nil, bearer(token))
	checkResp(t, r, http.StatusNotFound, &errResp)
	if errResp.Error == "" {
		t.Error("expected error message in body")
	}
}

// T-016: Создание проекта с пустым именем возвращает 400.
func TestT016_CreateProjectEmptyName(t *testing.T) {
	_, token := setupUser(t)
	var errResp struct {
		Error string `json:"error"`
	}
	r := do("POST", "/projects", map[string]any{"name": ""}, bearer(token))
	checkResp(t, r, http.StatusBadRequest, &errResp)
	if errResp.Error == "" {
		t.Error("expected error message in body")
	}
}

// TestGetProjectReturnsDeploymentSettings проверяет, что настройки деплоя,
// заданные через PUT /projects/{id}/deployment-settings, сохраняются и возвращаются
// в ответе GET /projects/{id}.
func TestGetProjectReturnsDeploymentSettings(t *testing.T) {
	_, token := setupUser(t)
	id := createProject(t, token, "settings-proj")

	settings := map[string]any{
		"serviceName":     "mysvc",
		"dockerfilePath":  "Dockerfile",
		"containerPort":   8080,
		"servicePort":     80,
		"serviceType":     "LoadBalancer",
		"baseBranch":      "main",
		"repositoryOwner": "myorg",
		"repositoryName":  "myrepo",
	}
	r := do("PUT", "/projects/"+id+"/deployment-settings", settings, bearer(token))
	checkResp(t, r, http.StatusOK, nil)

	var proj struct {
		ID              string `json:"id"`
		ServiceName     string `json:"serviceName"`
		DockerfilePath  string `json:"dockerfilePath"`
		ContainerPort   int    `json:"containerPort"`
		ServicePort     int    `json:"servicePort"`
		ServiceType     string `json:"serviceType"`
		BaseBranch      string `json:"baseBranch"`
		RepositoryOwner string `json:"repositoryOwner"`
		RepositoryName  string `json:"repositoryName"`
	}
	r = do("GET", "/projects/"+id, nil, bearer(token))
	checkResp(t, r, http.StatusOK, &proj)

	if proj.ServiceName != "mysvc" {
		t.Errorf("serviceName: got %q, want %q", proj.ServiceName, "mysvc")
	}
	if proj.DockerfilePath != "Dockerfile" {
		t.Errorf("dockerfilePath: got %q, want %q", proj.DockerfilePath, "Dockerfile")
	}
	if proj.ContainerPort != 8080 {
		t.Errorf("containerPort: got %d, want %d", proj.ContainerPort, 8080)
	}
	if proj.ServicePort != 80 {
		t.Errorf("servicePort: got %d, want %d", proj.ServicePort, 80)
	}
	if proj.ServiceType != "LoadBalancer" {
		t.Errorf("serviceType: got %q, want %q", proj.ServiceType, "LoadBalancer")
	}
	if proj.BaseBranch != "main" {
		t.Errorf("baseBranch: got %q, want %q", proj.BaseBranch, "main")
	}
	if proj.RepositoryOwner != "myorg" {
		t.Errorf("repositoryOwner: got %q, want %q", proj.RepositoryOwner, "myorg")
	}
	if proj.RepositoryName != "myrepo" {
		t.Errorf("repositoryName: got %q, want %q", proj.RepositoryName, "myrepo")
	}
}
