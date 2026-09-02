package agenthttp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSSLIssueTask_CertbotMissing(t *testing.T) {
	// Make sure certbot is NOT found on PATH.
	t.Setenv("PATH", t.TempDir())

	e := NewRouterWithOptions(RouterOptions{AllowStubSSL: true, AuthToken: "test-agent-token"})

	payload := map[string]any{
		"site_id": 1,
		"domain":  "example.com",
		"email":   "admin@example.com",
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/tasks/ssl/issue", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var result map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result["ok"] != true {
		t.Fatalf("expected ok true, got %v", result["ok"])
	}
	stub, _ := result["stub"].(bool)
	if !stub {
		t.Fatalf("expected stub=true, got %v", result["stub"])
	}
	msg, _ := result["message"].(string)
	if !strings.Contains(msg, "certbot not installed") {
		t.Fatalf("expected certbot-not-installed message, got %s", msg)
	}
}

func TestSSLIssueTask_InvalidDomain_400(t *testing.T) {
	// Ensure certbot cannot be found so we don't accidentally run a real certbot.
	t.Setenv("PATH", t.TempDir())

	e := NewRouterWithOptions(RouterOptions{AuthToken: "test-agent-token"})

	payload := map[string]any{
		"site_id": 1,
		"domain":  "-invalid",
		"email":   "admin@example.com",
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/tasks/ssl/issue", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	type errBody struct {
		Message string `json:"message"`
	}
	var result errBody
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.Contains(result.Message, "invalid domain") {
		t.Fatalf("expected invalid domain error, got %s", result.Message)
	}
}
