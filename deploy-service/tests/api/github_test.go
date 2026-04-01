package api_test

import (
	"net/http"
	"testing"
)

// T-028: Запрос вопросов bootstrap GitHub с пустыми полями репозитория возвращает 400.
func TestT028_GitHubQuestionsEmptyFields(t *testing.T) {
	_, token := setupUser(t)
	id := createProject(t, token, "gh-questions-empty-proj")

	var errResp struct {
		Error string `json:"error"`
	}
	body := map[string]any{
		"repositoryOwner": "",
		"repositoryName":  "",
		"baseBranch":      "main",
	}
	r := do("POST", "/projects/"+id+"/github/questions", body, bearer(token))
	checkResp(t, r, http.StatusBadRequest, &errResp)
	if errResp.Error == "" {
		t.Error("expected error message in body")
	}
}

// T-029: Запрос вопросов bootstrap GitHub с корректными данными возвращает 200 с вопросами.
func TestT029_GitHubQuestionsValid(t *testing.T) {
	_, token := setupUser(t)
	id := createProject(t, token, "gh-questions-valid-proj")

	var resp struct {
		RepositoryOwner string `json:"repositoryOwner"`
		RepositoryName  string `json:"repositoryName"`
		Questions       []any  `json:"questions"`
	}
	body := map[string]any{
		"repositoryOwner": "testowner",
		"repositoryName":  "testrepo",
		"baseBranch":      "main",
		"dockerfilePath":  "Dockerfile",
	}
	r := do("POST", "/projects/"+id+"/github/questions", body, bearer(token))
	checkResp(t, r, http.StatusOK, &resp)
	if resp.RepositoryOwner != "testowner" {
		t.Errorf("repositoryOwner: got %q, want %q", resp.RepositoryOwner, "testowner")
	}
	if resp.RepositoryName != "testrepo" {
		t.Errorf("repositoryName: got %q, want %q", resp.RepositoryName, "testrepo")
	}
	if len(resp.Questions) == 0 {
		t.Error("expected at least one question")
	}
}
