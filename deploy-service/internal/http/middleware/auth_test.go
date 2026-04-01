//go:build !integration

package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"deploy-service/internal/auth"
)

func TestRequireAuthRejectsMissingBearerToken(t *testing.T) {
	t.Parallel()

	called := false
	handler := RequireAuth("secret")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/projects", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", rec.Code)
	}
	if called {
		t.Fatal("expected downstream handler not to be called")
	}
}

func TestRequireAuthInjectsUserIDIntoContext(t *testing.T) {
	t.Parallel()

	token, err := auth.GenerateToken("usr-1", "alice@example.com", "secret", time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}

	handler := RequireAuth("secret")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := UserIDFromContext(r.Context()); got != "usr-1" {
			t.Fatalf("expected user id usr-1 in context, got %q", got)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/projects", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
}
