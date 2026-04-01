package api_test

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

// T-038: GET /swagger без авторизации возвращает 200 (публичный эндпоинт).
func TestT038_SwaggerUINoAuth(t *testing.T) {
	r := do("GET", "/swagger", nil, nil)
	defer r.Body.Close()
	if r.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(r.Body)
		t.Fatalf("status: got %d, want 200; body: %s", r.StatusCode, b)
	}
}

// T-039: GET /swagger с заголовком Authorization все равно возвращает 200.
func TestT039_SwaggerUIWithAuth(t *testing.T) {
	_, token := setupUser(t)
	r := do("GET", "/swagger", nil, bearer(token))
	defer r.Body.Close()
	if r.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(r.Body)
		t.Fatalf("status: got %d, want 200; body: %s", r.StatusCode, b)
	}
}

// T-040: GET /swagger/openapi.yaml возвращает 200.
func TestT040_SwaggerOpenAPIYaml(t *testing.T) {
	r := do("GET", "/swagger/openapi.yaml", nil, nil)
	defer r.Body.Close()
	if r.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(r.Body)
		t.Fatalf("status: got %d, want 200; body: %s", r.StatusCode, b)
	}
}

// T-041: GET /swagger/openapi.yaml возвращает тип содержимого YAML.
func TestT041_SwaggerYamlContentType(t *testing.T) {
	r := do("GET", "/swagger/openapi.yaml", nil, nil)
	defer r.Body.Close()
	if r.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(r.Body)
		t.Fatalf("status: got %d, want 200; body: %s", r.StatusCode, b)
	}
	ct := r.Header.Get("Content-Type")
	if !strings.Contains(ct, "yaml") && !strings.Contains(ct, "text/plain") && !strings.Contains(ct, "octet-stream") {
		// Разрешаем типы содержимого, типичные для YAML-файлов.
		body, _ := io.ReadAll(r.Body)
		// Проверяем, что содержимое похоже на YAML (начинается с "openapi:").
		if !strings.HasPrefix(strings.TrimSpace(string(body)), "openapi:") {
			t.Errorf("response does not appear to be YAML; Content-Type: %q, body prefix: %.50s", ct, body)
		}
	}
}
