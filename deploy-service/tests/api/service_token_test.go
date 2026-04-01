package api_test

import (
	"net/http"
	"testing"
)

func TestServiceGitHubTokenPersistsBetweenLogins(t *testing.T) {
	email, token := setupUser(t)

	var status struct {
		Configured bool `json:"configured"`
	}

	r := do("GET", "/service/github-token", nil, bearer(token))
	checkResp(t, r, http.StatusOK, &status)
	if status.Configured {
		t.Fatal("expected service token to be absent for new user")
	}

	r = do("PUT", "/service/github-token", map[string]any{"token": "ghp_stored_token"}, bearer(token))
	checkResp(t, r, http.StatusOK, &status)
	if !status.Configured {
		t.Fatal("expected service token to be configured after PUT")
	}

	var loginResp struct {
		Token string `json:"token"`
	}
	r = do("POST", "/auth/login", map[string]any{"email": email, "password": "password123"}, nil)
	checkResp(t, r, http.StatusOK, &loginResp)
	if loginResp.Token == "" {
		t.Fatal("expected token from login")
	}

	r = do("GET", "/service/github-token", nil, bearer(loginResp.Token))
	checkResp(t, r, http.StatusOK, &status)
	if !status.Configured {
		t.Fatal("expected token status to persist after relogin")
	}

	r = do("DELETE", "/service/github-token", nil, bearer(loginResp.Token))
	checkResp(t, r, http.StatusOK, &status)
	if status.Configured {
		t.Fatal("expected configured=false after delete")
	}
}
