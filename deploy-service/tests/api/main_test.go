package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"deploy-service/internal/app"
	"deploy-service/internal/auth"
)

const testJWTSecret = "test-secret-for-testing-1234567890"
const testWebhookSecret = "test-webhook-secret"

var (
	testServer *httptest.Server
	userSeq    atomic.Int64
)

func TestMain(m *testing.M) {
	os.Setenv("KUBERNETES_PROVISIONER", "mock")
	os.Setenv("GITHUB_AUTOMATION_MODE", "mock")
	os.Setenv("JWT_SECRET", testJWTSecret)
	os.Setenv("GITHUB_WEBHOOK_SECRET", testWebhookSecret)
	os.Unsetenv("DATABASE_URL")

	application, err := app.New(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create app: %v\n", err)
		os.Exit(1)
	}

	testServer = httptest.NewServer(application.Router)
	code := m.Run()
	testServer.Close()
	application.Close()
	os.Exit(code)
}

// uniqueEmail возвращает email, уникальный в рамках запуска тестов.
func uniqueEmail() string {
	n := userSeq.Add(1)
	return fmt.Sprintf("user%d@test.com", n)
}

// do выполняет HTTP-запрос к тестовому серверу.
func do(method, path string, body any, headers map[string]string) *http.Response {
	var bodyReader io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, testServer.URL+path, bodyReader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		panic(fmt.Sprintf("do %s %s: %v", method, path, err))
	}
	return resp
}

// checkResp проверяет статус ответа и при необходимости декодирует JSON-тело.
func checkResp(t *testing.T, resp *http.Response, wantStatus int, v any) {
	t.Helper()
	defer resp.Body.Close()
	if resp.StatusCode != wantStatus {
		b, _ := io.ReadAll(resp.Body)
		t.Errorf("status: got %d, want %d; body: %s", resp.StatusCode, wantStatus, b)
		return
	}
	if v != nil {
		if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
			t.Errorf("decode response: %v", err)
		}
	}
}

// bearer возвращает карту заголовков Authorization.
func bearer(token string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + token}
}

// setupUser регистрирует нового уникального пользователя и возвращает его email и JWT-токен.
func setupUser(t *testing.T) (email, token string) {
	t.Helper()
	email = uniqueEmail()
	var tok struct {
		Token string `json:"token"`
	}
	resp := do("POST", "/auth/register", map[string]any{"email": email, "password": "password123"}, nil)
	checkResp(t, resp, http.StatusCreated, &tok)
	if tok.Token == "" {
		t.Fatal("empty token from register")
	}
	return email, tok.Token
}

// createProject создает проект для пользователя, ждет активации и возвращает ID проекта.
func createProject(t *testing.T, token, name string) string {
	t.Helper()
	var proj struct {
		ID string `json:"id"`
	}
	resp := do("POST", "/projects", map[string]any{"name": name}, bearer(token))
	checkResp(t, resp, http.StatusAccepted, &proj)
	if proj.ID == "" {
		t.Fatal("empty project ID")
	}
	waitForProjectActive(t, token, proj.ID)
	return proj.ID
}

// waitForProjectActive опрашивает GET /projects/{id} до status == "active" или до таймаута.
func waitForProjectActive(t *testing.T, token, projectID string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var p struct {
			Status string `json:"status"`
		}
		resp := do("GET", "/projects/"+projectID, nil, bearer(token))
		if resp.StatusCode == http.StatusOK {
			_ = json.NewDecoder(resp.Body).Decode(&p)
			resp.Body.Close()
			if p.Status == "active" {
				return
			}
		} else {
			resp.Body.Close()
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Errorf("project %s did not become active within timeout", projectID)
}

// expiredToken генерирует JWT с отрицательным TTL (уже истек).
func expiredToken() string {
	tok, _ := auth.GenerateToken("user-expired", "expired@test.com", testJWTSecret, -1*time.Minute)
	return tok
}
