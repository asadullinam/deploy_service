package api_test

import (
	"net/http"
	"testing"
)

// T-025: Список релизов для нового проекта возвращает 200 и пустой массив.
func TestT025_ListReleasesEmpty(t *testing.T) {
	_, token := setupUser(t)
	id := createProject(t, token, "releases-proj")

	var releases []map[string]any
	r := do("GET", "/projects/"+id+"/releases", nil, bearer(token))
	checkResp(t, r, http.StatusOK, &releases)
	if releases == nil {
		t.Error("expected non-nil releases array (may be empty)")
	}
}

// T-026: Получение несуществующего релиза возвращает 404.
func TestT026_GetReleaseNotFound(t *testing.T) {
	_, token := setupUser(t)
	id := createProject(t, token, "releases-get-proj")

	var errResp struct {
		Error string `json:"error"`
	}
	r := do("GET", "/projects/"+id+"/releases/nonexistent-release-id", nil, bearer(token))
	checkResp(t, r, http.StatusNotFound, &errResp)
	if errResp.Error == "" {
		t.Error("expected error message in body")
	}
}

// T-027: Откат к несуществующему релизу возвращает 404.
func TestT027_RollbackReleaseNotFound(t *testing.T) {
	_, token := setupUser(t)
	id := createProject(t, token, "rollback-proj")

	var errResp struct {
		Error string `json:"error"`
	}
	r := do("POST", "/projects/"+id+"/releases/nonexistent-release-id/rollback", nil, bearer(token))
	checkResp(t, r, http.StatusNotFound, &errResp)
	if errResp.Error == "" {
		t.Error("expected error message in body")
	}
}
