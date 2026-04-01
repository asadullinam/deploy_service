package api_test

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"testing"
)

// githubSig вычисляет HMAC-SHA256 подпись в стиле GitHub для полезной нагрузки.
func githubSig(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// doWebhook отправляет POST на /webhooks/github с указанными заголовками и сырым JSON-телом.
func doWebhook(body []byte, extraHeaders map[string]string) *http.Response {
	req, _ := http.NewRequest("POST", testServer.URL+"/webhooks/github", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}
	resp, _ := http.DefaultClient.Do(req)
	return resp
}

// T-032: Webhook-запрос без заголовка X-GitHub-Event возвращает 204 No Content.
func TestT032_WebhookNoEventHeader(t *testing.T) {
	body := []byte(`{"action":"completed"}`)
	sig := githubSig(body, testWebhookSecret)
	r := doWebhook(body, map[string]string{
		"X-Hub-Signature-256": sig,
	})
	r.Body.Close()
	if r.StatusCode != http.StatusNoContent {
		t.Errorf("status: got %d, want 204", r.StatusCode)
	}
}

// T-033: Webhook с типом события не workflow_run возвращает 204 No Content.
func TestT033_WebhookNonWorkflowEvent(t *testing.T) {
	body := []byte(`{"action":"opened"}`)
	sig := githubSig(body, testWebhookSecret)
	r := doWebhook(body, map[string]string{
		"X-GitHub-Event":      "push",
		"X-Hub-Signature-256": sig,
	})
	r.Body.Close()
	if r.StatusCode != http.StatusNoContent {
		t.Errorf("status: got %d, want 204", r.StatusCode)
	}
}

// T-034: Webhook без подписи при настроенном секрете возвращает 401.
func TestT034_WebhookNoSignature(t *testing.T) {
	body := []byte(`{"action":"completed","workflow_run":{"id":1,"status":"completed","conclusion":"success","head_sha":"abc"}}`)
	r := doWebhook(body, map[string]string{
		"X-GitHub-Event": "workflow_run",
	})
	r.Body.Close()
	if r.StatusCode != http.StatusUnauthorized {
		t.Errorf("status: got %d, want 401", r.StatusCode)
	}
}

// T-035: Webhook с неверной подписью возвращает 401.
func TestT035_WebhookInvalidSignature(t *testing.T) {
	body := []byte(`{"action":"completed","workflow_run":{"id":1,"status":"completed","conclusion":"success","head_sha":"abc"}}`)
	r := doWebhook(body, map[string]string{
		"X-GitHub-Event":      "workflow_run",
		"X-Hub-Signature-256": "sha256=invalidsignature",
	})
	r.Body.Close()
	if r.StatusCode != http.StatusUnauthorized {
		t.Errorf("status: got %d, want 401", r.StatusCode)
	}
}

// T-036: Webhook с корректной HMAC-подписью и валидным JSON возвращает 204.
func TestT036_WebhookValidSignature(t *testing.T) {
	payload := map[string]any{
		"action": "completed",
		"workflow_run": map[string]any{
			"id":         int64(999999),
			"status":     "completed",
			"conclusion": "success",
			"head_sha":   "abc123",
		},
	}
	body, _ := json.Marshal(payload)
	sig := githubSig(body, testWebhookSecret)

	r := doWebhook(body, map[string]string{
		"X-GitHub-Event":      "workflow_run",
		"X-Hub-Signature-256": sig,
	})
	r.Body.Close()
	if r.StatusCode != http.StatusNoContent {
		t.Errorf("status: got %d, want 204", r.StatusCode)
	}
}

// T-037: Webhook с корректной подписью, но некорректным JSON возвращает 400.
func TestT037_WebhookInvalidJSON(t *testing.T) {
	body := []byte(`not valid json`)
	sig := githubSig(body, testWebhookSecret)

	r := doWebhook(body, map[string]string{
		"X-GitHub-Event":      "workflow_run",
		"X-Hub-Signature-256": sig,
	})
	r.Body.Close()
	if r.StatusCode != http.StatusBadRequest {
		t.Errorf("status: got %d, want 400", r.StatusCode)
	}
}
