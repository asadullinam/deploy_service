package api_test

import (
	"net/http"
	"testing"
)

// T-042: Пользователь B не может GET проект пользователя A — должен получить 403.
//
// TestGetProjectRejectsForeignOwner уже существует на уровне unit-тестов хендлера
// (internal/http/handler_test.go). Этот тест проверяет тот же инвариант
// сквозь весь HTTP-стек с реальными JWT-токенами и общим in-memory хранилищем.
func TestT042_GetProjectRejectsForeignOwner(t *testing.T) {
	_, tokenA := setupUser(t)
	_, tokenB := setupUser(t)

	projectID := createProject(t, tokenA, "owner-a-project")

	var errResp struct {
		Error string `json:"error"`
	}
	r := do("GET", "/projects/"+projectID, nil, bearer(tokenB))
	checkResp(t, r, http.StatusForbidden, &errResp)
	if errResp.Error == "" {
		t.Error("expected non-empty error message in body")
	}
}

// T-043: Пользователь B не может GET kubeconfig проекта пользователя A — должен получить 403.
func TestT043_GetKubeconfigRejectsForeignOwner(t *testing.T) {
	_, tokenA := setupUser(t)
	_, tokenB := setupUser(t)

	projectID := createProject(t, tokenA, "kubeconfig-owner-a-project")

	var errResp struct {
		Error string `json:"error"`
	}
	r := do("GET", "/projects/"+projectID+"/kubeconfig", nil, bearer(tokenB))
	checkResp(t, r, http.StatusForbidden, &errResp)
	if errResp.Error == "" {
		t.Error("expected non-empty error message in body")
	}
}

// T-044: Пользователь B не может выполнить rollback релиза в проекте пользователя A — должен получить 403.
//
// Хендлер проверяет владение через requireOwnedProject до поиска релиза,
// поэтому защита срабатывает независимо от того, существует ли ID релиза.
func TestT044_RollbackReleaseRejectsForeignOwner(t *testing.T) {
	_, tokenA := setupUser(t)
	_, tokenB := setupUser(t)

	projectID := createProject(t, tokenA, "rollback-owner-a-project")

	var errResp struct {
		Error string `json:"error"`
	}
	r := do("POST", "/projects/"+projectID+"/releases/rel-foreign/rollback", nil, bearer(tokenB))
	checkResp(t, r, http.StatusForbidden, &errResp)
	if errResp.Error == "" {
		t.Error("expected non-empty error message in body")
	}
}

// T-045: Пользователь B не может обновить настройки деплоя проекта пользователя A — должен получить 403.
func TestT045_UpdateDeploymentSettingsRejectsForeignOwner(t *testing.T) {
	_, tokenA := setupUser(t)
	_, tokenB := setupUser(t)

	projectID := createProject(t, tokenA, "settings-owner-a-project")

	body := map[string]any{
		"repositoryOwner": "attacker",
		"repositoryName":  "evil-repo",
		"baseBranch":      "main",
	}
	var errResp struct {
		Error string `json:"error"`
	}
	r := do("PUT", "/projects/"+projectID+"/deployment-settings", body, bearer(tokenB))
	checkResp(t, r, http.StatusForbidden, &errResp)
	if errResp.Error == "" {
		t.Error("expected non-empty error message in body")
	}
}
