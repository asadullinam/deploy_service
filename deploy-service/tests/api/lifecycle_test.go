package api_test

import (
	"net/http"
	"testing"
)

// T-017: Приостановка активного проекта возвращает 200.
func TestT017_SuspendActiveProject(t *testing.T) {
	_, token := setupUser(t)
	id := createProject(t, token, "suspend-proj")

	var resp struct {
		Status string `json:"status"`
	}
	r := do("POST", "/projects/"+id+"/suspend", nil, bearer(token))
	checkResp(t, r, http.StatusOK, &resp)
	if resp.Status != "suspended" {
		t.Errorf("status: got %q, want %q", resp.Status, "suspended")
	}
}

// T-018: Приостановка уже приостановленного проекта возвращает 400.
func TestT018_SuspendAlreadySuspended(t *testing.T) {
	_, token := setupUser(t)
	id := createProject(t, token, "double-suspend-proj")

	do("POST", "/projects/"+id+"/suspend", nil, bearer(token)).Body.Close()

	var errResp struct {
		Error string `json:"error"`
	}
	r := do("POST", "/projects/"+id+"/suspend", nil, bearer(token))
	checkResp(t, r, http.StatusBadRequest, &errResp)
	if errResp.Error == "" {
		t.Error("expected error message in body")
	}
}

// T-019: Возобновление приостановленного проекта возвращает 200.
func TestT019_ResumeProject(t *testing.T) {
	_, token := setupUser(t)
	id := createProject(t, token, "resume-proj")

	do("POST", "/projects/"+id+"/suspend", nil, bearer(token)).Body.Close()

	var resp struct {
		Status string `json:"status"`
	}
	r := do("POST", "/projects/"+id+"/resume", nil, bearer(token))
	checkResp(t, r, http.StatusOK, &resp)
	if resp.Status != "active" {
		t.Errorf("status: got %q, want %q", resp.Status, "active")
	}
}

// T-020: Возобновление уже активного проекта возвращает 400.
func TestT020_ResumeAlreadyActive(t *testing.T) {
	_, token := setupUser(t)
	id := createProject(t, token, "resume-active-proj")

	var errResp struct {
		Error string `json:"error"`
	}
	r := do("POST", "/projects/"+id+"/resume", nil, bearer(token))
	checkResp(t, r, http.StatusBadRequest, &errResp)
	if errResp.Error == "" {
		t.Error("expected error message in body")
	}
}

// T-021: Приостановка несуществующего проекта возвращает 404.
func TestT021_SuspendNotFound(t *testing.T) {
	_, token := setupUser(t)
	var errResp struct {
		Error string `json:"error"`
	}
	r := do("POST", "/projects/nonexistent-id/suspend", nil, bearer(token))
	checkResp(t, r, http.StatusNotFound, &errResp)
	if errResp.Error == "" {
		t.Error("expected error message in body")
	}
}

// T-022: Возобновление несуществующего проекта возвращает 404.
func TestT022_ResumeNotFound(t *testing.T) {
	_, token := setupUser(t)
	var errResp struct {
		Error string `json:"error"`
	}
	r := do("POST", "/projects/nonexistent-id/resume", nil, bearer(token))
	checkResp(t, r, http.StatusNotFound, &errResp)
	if errResp.Error == "" {
		t.Error("expected error message in body")
	}
}

// T-023: Проверка смены статусов в цикле suspend -> resume.
func TestT023_StatusTransitions(t *testing.T) {
	_, token := setupUser(t)
	id := createProject(t, token, "status-transition-proj")

	// Проверяем, что начальный статус — active.
	var proj struct {
		Status string `json:"status"`
	}
	r := do("GET", "/projects/"+id, nil, bearer(token))
	checkResp(t, r, http.StatusOK, &proj)
	if proj.Status != "active" {
		t.Errorf("initial status: got %q, want %q", proj.Status, "active")
	}

	// Приостановить.
	do("POST", "/projects/"+id+"/suspend", nil, bearer(token)).Body.Close()

	r = do("GET", "/projects/"+id, nil, bearer(token))
	checkResp(t, r, http.StatusOK, &proj)
	if proj.Status != "suspended" {
		t.Errorf("after suspend: got %q, want %q", proj.Status, "suspended")
	}

	// Возобновить.
	do("POST", "/projects/"+id+"/resume", nil, bearer(token)).Body.Close()

	r = do("GET", "/projects/"+id, nil, bearer(token))
	checkResp(t, r, http.StatusOK, &proj)
	if proj.Status != "active" {
		t.Errorf("after resume: got %q, want %q", proj.Status, "active")
	}
}
