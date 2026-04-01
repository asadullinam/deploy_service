package api_test

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

// T-030: Получение kubeconfig проекта возвращает 200 с YAML-контентом.
func TestT030_GetKubeconfig(t *testing.T) {
	_, token := setupUser(t)
	id := createProject(t, token, "kubeconfig-get-proj")

	r := do("GET", "/projects/"+id+"/kubeconfig", nil, bearer(token))
	defer r.Body.Close()
	if r.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(r.Body)
		t.Fatalf("status: got %d, want 200; body: %s", r.StatusCode, b)
	}
	contentType := r.Header.Get("Content-Type")
	if !strings.Contains(contentType, "yaml") {
		t.Errorf("Content-Type: got %q, expected yaml", contentType)
	}
	body, _ := io.ReadAll(r.Body)
	if len(body) == 0 {
		t.Error("expected non-empty kubeconfig body")
	}
}

// T-031: Ротация kubeconfig возвращает 200 с новым YAML-контентом.
func TestT031_RotateKubeconfig(t *testing.T) {
	_, token := setupUser(t)
	id := createProject(t, token, "kubeconfig-rotate-proj")

	r := do("POST", "/projects/"+id+"/kubeconfig/rotate", nil, bearer(token))
	defer r.Body.Close()
	if r.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(r.Body)
		t.Fatalf("status: got %d, want 200; body: %s", r.StatusCode, b)
	}
	contentType := r.Header.Get("Content-Type")
	if !strings.Contains(contentType, "yaml") {
		t.Errorf("Content-Type: got %q, expected yaml", contentType)
	}
	body, _ := io.ReadAll(r.Body)
	if len(body) == 0 {
		t.Error("expected non-empty kubeconfig body")
	}
}
