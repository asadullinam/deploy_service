package api_test

import (
	"net/http"
	"testing"
)

// T-007: Защищенный эндпоинт без заголовка Authorization возвращает 401.
func TestT007_NoAuthHeader(t *testing.T) {
	var errResp struct {
		Error string `json:"error"`
	}
	r := do("GET", "/projects", nil, nil)
	checkResp(t, r, http.StatusUnauthorized, &errResp)
	if errResp.Error == "" {
		t.Error("expected error message in body")
	}
}

// T-008: Защищенный эндпоинт с некорректным (не JWT) токеном возвращает 401.
func TestT008_MalformedToken(t *testing.T) {
	var errResp struct {
		Error string `json:"error"`
	}
	r := do("GET", "/projects", nil, bearer("this-is-not-a-jwt"))
	checkResp(t, r, http.StatusUnauthorized, &errResp)
	if errResp.Error == "" {
		t.Error("expected error message in body")
	}
}

// T-009: Защищенный эндпоинт с истекшим токеном возвращает 401.
func TestT009_ExpiredToken(t *testing.T) {
	tok := expiredToken()
	var errResp struct {
		Error string `json:"error"`
	}
	r := do("GET", "/projects", nil, bearer(tok))
	checkResp(t, r, http.StatusUnauthorized, &errResp)
	if errResp.Error == "" {
		t.Error("expected error message in body")
	}
}
