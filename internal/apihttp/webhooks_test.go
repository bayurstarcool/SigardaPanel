package apihttp

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/labstack/echo/v4"
	"sigardapanel/internal/store"
)

func TestDeployWebhook_ValidSignature(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	token := newAdminToken(t, s, ctx)
	srvID := createServer(t, s, ctx)

	adminUser, _ := s.Users.GetByUsername(ctx, "admin")
	site := &store.Site{ServerID: srvID, Name: "Dep", Domain: "dep.example.com", Runtime: "static", OwnerID: nullInt(adminUser.ID)}
	s.Sites.Create(ctx, site)

	// Set git config and generate webhook secret
	if err := s.Sites.UpdateGitConfig(ctx, site.ID, "https://github.com/user/repo.git", "main"); err != nil {
		t.Fatalf("update git config: %v", err)
	}
	secret, err := s.Sites.GenerateWebhookSecret(ctx, site.ID)
	if err != nil {
		t.Fatalf("generate webhook secret: %v", err)
	}

	body := []byte(`{"ref":"refs/heads/main"}`)
	sig := computeHMAC(body, secret)

	rec := doRequestWithHeaders(t, e, "POST", "/api/v1/webhooks/deploy/"+strconv.FormatInt(site.ID, 10), body, token, map[string]string{"X-Hub-Signature-256": sig})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", rec.Code, rec.Body.String())
	}

	var result map[string]any
	json.NewDecoder(rec.Body).Decode(&result)
	if result["ok"] != true {
		t.Fatalf("expected ok true, got %v", result["ok"])
	}
	if result["message"] != "deploy queued" {
		t.Fatalf("expected deploy queued, got %v", result["message"])
	}
	if result["job_id"] == nil || result["job_id"].(float64) == 0 {
		t.Fatal("expected non-zero job_id")
	}
}

func TestDeployWebhook_InvalidSignature(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	token := newAdminToken(t, s, ctx)
	srvID := createServer(t, s, ctx)

	adminUser, _ := s.Users.GetByUsername(ctx, "admin")
	site := &store.Site{ServerID: srvID, Name: "Dep", Domain: "dep2.example.com", Runtime: "static", OwnerID: nullInt(adminUser.ID)}
	s.Sites.Create(ctx, site)

	if err := s.Sites.UpdateGitConfig(ctx, site.ID, "https://github.com/user/repo.git", "main"); err != nil {
		t.Fatalf("update git config: %v", err)
	}
	_, err := s.Sites.GenerateWebhookSecret(ctx, site.ID)
	if err != nil {
		t.Fatalf("generate webhook secret: %v", err)
	}

	body := []byte(`{"ref":"refs/heads/main"}`)
	rec := doRequestWithHeaders(t, e, "POST", "/api/v1/webhooks/deploy/"+strconv.FormatInt(site.ID, 10), body, token, map[string]string{"X-Hub-Signature-256": "sha256=badbadbad"})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var result map[string]any
	json.NewDecoder(rec.Body).Decode(&result)
	if result["ok"] != true {
		t.Fatalf("expected ok true, got %v", result["ok"])
	}
	if result["message"] != "ignored" {
		t.Fatalf("expected ignored, got %v", result["message"])
	}
}

func TestDeployWebhook_BranchMismatch(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	token := newAdminToken(t, s, ctx)
	srvID := createServer(t, s, ctx)

	adminUser, _ := s.Users.GetByUsername(ctx, "admin")
	site := &store.Site{ServerID: srvID, Name: "Dep", Domain: "dep3.example.com", Runtime: "static", OwnerID: nullInt(adminUser.ID)}
	s.Sites.Create(ctx, site)

	if err := s.Sites.UpdateGitConfig(ctx, site.ID, "https://github.com/user/repo.git", "main"); err != nil {
		t.Fatalf("update git config: %v", err)
	}
	secret, err := s.Sites.GenerateWebhookSecret(ctx, site.ID)
	if err != nil {
		t.Fatalf("generate webhook secret: %v", err)
	}

	body := []byte(`{"ref":"refs/heads/develop"}`)
	sig := computeHMAC(body, secret)

	rec := doRequestWithHeaders(t, e, "POST", "/api/v1/webhooks/deploy/"+strconv.FormatInt(site.ID, 10), body, token, map[string]string{"X-Hub-Signature-256": sig})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var result map[string]any
	json.NewDecoder(rec.Body).Decode(&result)
	if result["ok"] != true {
		t.Fatalf("expected ok true, got %v", result["ok"])
	}
	if result["message"] != "branch mismatch" {
		t.Fatalf("expected branch mismatch, got %v", result["message"])
	}
}

func TestDeployWebhook_NoGitRepo(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	token := newAdminToken(t, s, ctx)
	srvID := createServer(t, s, ctx)

	adminUser, _ := s.Users.GetByUsername(ctx, "admin")
	site := &store.Site{ServerID: srvID, Name: "Dep", Domain: "dep4.example.com", Runtime: "static", OwnerID: nullInt(adminUser.ID)}
	s.Sites.Create(ctx, site)
	// No git config set

	rec := doRequestWithHeaders(t, e, "POST", "/api/v1/webhooks/deploy/"+strconv.FormatInt(site.ID, 10), []byte(`{"ref":"refs/heads/main"}`), token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var result map[string]any
	json.NewDecoder(rec.Body).Decode(&result)
	if result["ok"] != true {
		t.Fatalf("expected ok true, got %v", result["ok"])
	}
}

func TestDeployWebhook_ComputeHMAC(t *testing.T) {
	body := []byte("hello")
	secret := "secret123"
	expectedSig := computeHMAC(body, secret)

	h := hmac.New(sha256.New, []byte(secret))
	h.Write(body)
	want := "sha256=" + hex.EncodeToString(h.Sum(nil))

	if expectedSig != want {
		t.Fatalf("expected %s, got %s", want, expectedSig)
	}
}

func doRequestWithHeaders(t *testing.T, e *echo.Echo, method, path string, body []byte, token string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}
	req := httptest.NewRequest(method, path, bodyReader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}
