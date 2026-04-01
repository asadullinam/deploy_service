package api_test

import (
	"net/http"
	"testing"
)

// T-024: Получение детализации стоимости проекта возвращает 200 с числовыми полями.
func TestT024_GetProjectCost(t *testing.T) {
	_, token := setupUser(t)
	id := createProject(t, token, "cost-proj")

	var cost struct {
		ProjectID string  `json:"projectId"`
		Total     float64 `json:"total"`
		Currency  string  `json:"currency"`
	}
	r := do("GET", "/projects/"+id+"/cost", nil, bearer(token))
	checkResp(t, r, http.StatusOK, &cost)
	if cost.ProjectID == "" {
		t.Error("expected non-empty projectId")
	}
	if cost.Currency == "" {
		t.Error("expected non-empty currency")
	}
}
