package api_test

import (
	"net/http"
	"testing"
)

const invalidCredentialsText = "Неверный логин или пароль"

// T-001: Регистрация нового пользователя возвращает 201 и JWT-токен.
func TestT001_RegisterNewUser(t *testing.T) {
	email := uniqueEmail()
	var resp struct {
		Token string `json:"token"`
	}
	r := do("POST", "/auth/register", map[string]any{"email": email, "password": "password123"}, nil)
	checkResp(t, r, http.StatusCreated, &resp)
	if resp.Token == "" {
		t.Error("expected non-empty token")
	}
}

// T-002: Регистрация с уже существующим email возвращает 409 Conflict.
func TestT002_RegisterDuplicateEmail(t *testing.T) {
	email := uniqueEmail()
	do("POST", "/auth/register", map[string]any{"email": email, "password": "pass1"}, nil).Body.Close()

	var errResp struct {
		Error string `json:"error"`
	}
	r := do("POST", "/auth/register", map[string]any{"email": email, "password": "pass2"}, nil)
	checkResp(t, r, http.StatusConflict, &errResp)
	if errResp.Error == "" {
		t.Error("expected error message in body")
	}
}

// T-003: Регистрация с пустым email возвращает 400.
func TestT003_RegisterEmptyEmail(t *testing.T) {
	var errResp struct {
		Error string `json:"error"`
	}
	r := do("POST", "/auth/register", map[string]any{"email": "", "password": "password123"}, nil)
	checkResp(t, r, http.StatusBadRequest, &errResp)
	if errResp.Error == "" {
		t.Error("expected error message in body")
	}
}

// T-004: Регистрация с пустым паролем возвращает 400.
func TestT004_RegisterEmptyPassword(t *testing.T) {
	var errResp struct {
		Error string `json:"error"`
	}
	r := do("POST", "/auth/register", map[string]any{"email": uniqueEmail(), "password": ""}, nil)
	checkResp(t, r, http.StatusBadRequest, &errResp)
	if errResp.Error == "" {
		t.Error("expected error message in body")
	}
}

// T-005: Вход с корректными учетными данными возвращает 200 и JWT-токен.
func TestT005_LoginValid(t *testing.T) {
	email := uniqueEmail()
	do("POST", "/auth/register", map[string]any{"email": email, "password": "mypass"}, nil).Body.Close()

	var resp struct {
		Token string `json:"token"`
	}
	r := do("POST", "/auth/login", map[string]any{"email": email, "password": "mypass"}, nil)
	checkResp(t, r, http.StatusOK, &resp)
	if resp.Token == "" {
		t.Error("expected non-empty token")
	}
}

// T-006: Вход с неверным паролем возвращает 401 Unauthorized.
func TestT006_LoginWrongPassword(t *testing.T) {
	email := uniqueEmail()
	do("POST", "/auth/register", map[string]any{"email": email, "password": "correct"}, nil).Body.Close()

	var errResp struct {
		Error string `json:"error"`
	}
	r := do("POST", "/auth/login", map[string]any{"email": email, "password": "wrong"}, nil)
	checkResp(t, r, http.StatusUnauthorized, &errResp)
	if errResp.Error != invalidCredentialsText {
		t.Errorf("expected %q, got %q", invalidCredentialsText, errResp.Error)
	}
}

// T-007: Вход с неизвестным email возвращает 401 с общим сообщением о неверных учетных данных.
func TestT007_LoginUnknownEmail(t *testing.T) {
	var errResp struct {
		Error string `json:"error"`
	}
	r := do("POST", "/auth/login", map[string]any{"email": uniqueEmail(), "password": "wrong"}, nil)
	checkResp(t, r, http.StatusUnauthorized, &errResp)
	if errResp.Error != invalidCredentialsText {
		t.Errorf("expected %q, got %q", invalidCredentialsText, errResp.Error)
	}
}
