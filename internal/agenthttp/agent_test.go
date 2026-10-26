package agenthttp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"sigardapanel/internal/buildinfo"
	"sigardapanel/internal/executor"
)

func TestSiteCreateTask_Success(t *testing.T) {
	setupTestBinaries(t)
	rootDir := t.TempDir()
	nginxDir := t.TempDir()
	e := NewRouterWithOptions(RouterOptions{
		NginxConfDir: nginxDir,
	})

	payload := map[string]any{
		"site_id":   1,
		"name":      "Test Site",
		"domain":    "test.example.com",
		"runtime":   "static",
		"root_path": rootDir,
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/tasks/site/create", bytes.NewReader(body))
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
	msg, _ := result["message"].(string)
	if msg == "" {
		t.Fatal("expected message field")
	}
}

func TestSiteCreateTask_InvalidPayload(t *testing.T) {
	e := NewRouterWithOptions(RouterOptions{AllowStubSSL: true})

	req := httptest.NewRequest(http.MethodPost, "/tasks/site/create", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestSiteDeleteTask_Success(t *testing.T) {
	setupTestBinaries(t)
	rootDir := filepath.Join(t.TempDir(), "delete.example.com")
	nginxDir := t.TempDir()
	if err := os.MkdirAll(rootDir, 0755); err != nil {
		t.Fatalf("mkdir root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(nginxDir, "delete.example.com.conf"), []byte("server {}"), 0644); err != nil {
		t.Fatalf("write nginx conf: %v", err)
	}
	e := NewRouterWithOptions(RouterOptions{NginxConfDir: nginxDir, AllowStubSSL: true})

	payload := map[string]any{
		"site_id":   1,
		"domain":    "delete.example.com",
		"root_path": rootDir,
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/tasks/site/delete", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var result map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result["ok"] != true {
		t.Fatalf("expected ok true, got %v", result["ok"])
	}
}

func TestSiteDeployTask_Success(t *testing.T) {
	repoDir := setupGitRepo(t)
	rootDir := t.TempDir()
	e := NewRouterWithOptions(RouterOptions{AllowStubSSL: true})

	payload := map[string]any{
		"site_id":   1,
		"repo":      "file://" + repoDir,
		"branch":    "master",
		"root_path": rootDir,
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/tasks/site/deploy", bytes.NewReader(body))
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
}

func TestSiteCreateTask_GeneratesNginxConfig(t *testing.T) {
	setupTestBinaries(t)
	rootDir := t.TempDir()
	nginxDir := t.TempDir()
	e := NewRouterWithOptions(RouterOptions{
		NginxConfDir: nginxDir,
	})

	payload := map[string]any{
		"site_id":   1,
		"name":      "Test Site",
		"domain":    "test.example.com",
		"runtime":   "static",
		"root_path": rootDir,
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/tasks/site/create", bytes.NewReader(body))
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
	nginxConfig, ok := result["nginx_config"].(string)
	if !ok || nginxConfig == "" {
		t.Fatal("expected nginx_config field in response")
	}
}

func TestSiteCreateTask_InvalidRuntime(t *testing.T) {
	setupTestBinaries(t)
	e := NewRouterWithOptions(RouterOptions{
		NginxConfDir: t.TempDir(),
	})

	payload := map[string]any{
		"site_id":   1,
		"name":      "Test Site",
		"domain":    "test.example.com",
		"runtime":   "ruby",
		"root_path": t.TempDir(),
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/tasks/site/create", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestNginxReloadTask(t *testing.T) {
	e := NewRouterWithOptions(RouterOptions{AllowStubSSL: true})

	req := httptest.NewRequest(http.MethodPost, "/tasks/nginx/reload", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var result map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result["ok"] != true {
		t.Fatalf("expected ok true, got %v", result["ok"])
	}
}

func TestAgentHealth(t *testing.T) {
	e := NewRouterWithOptions(RouterOptions{AuthToken: "test-agent-token"})
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var result map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	data, ok := result["data"].(map[string]any)
	if !ok {
		t.Fatal("expected data object")
	}
	if data["status"] != "ok" {
		t.Fatalf("expected status ok, got %v", data["status"])
	}
}

// The agent must report the version stamped into the binary at release time.
func TestAgentHealthReportsBuildVersion(t *testing.T) {
	orig := buildinfo.Version
	buildinfo.Version = "9.8.7-test"
	t.Cleanup(func() { buildinfo.Version = orig })

	e := NewRouterWithOptions(RouterOptions{AuthToken: "test-agent-token"})
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var result struct {
		Data struct {
			Version string `json:"version"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Data.Version != "9.8.7-test" {
		t.Fatalf("expected version 9.8.7-test, got %q", result.Data.Version)
	}
}

func TestSSLIssueTask_Success(t *testing.T) {
	// Ensure certbot is not on PATH so we get the stub fallback.
	t.Setenv("PATH", t.TempDir())

	e := NewRouterWithOptions(RouterOptions{AllowStubSSL: true})

	payload := map[string]any{
		"site_id": 1,
		"domain":  "ssl.example.com",
		"email":   "admin@example.com",
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/tasks/ssl/issue", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
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
		t.Fatalf("expected stub fallback when certbot is missing")
	}
	expiresAt, ok := result["expires_at"].(string)
	if !ok || expiresAt == "" {
		t.Fatal("expected expires_at field")
	}
}

func TestSSLIssueTask_InvalidPayload(t *testing.T) {
	e := NewRouterWithOptions(RouterOptions{AuthToken: "test-agent-token"})

	req := httptest.NewRequest(http.MethodPost, "/tasks/ssl/issue", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestSSLRenewTask_Success(t *testing.T) {
	// Ensure certbot is not on PATH so we get the stub fallback.
	t.Setenv("PATH", t.TempDir())

	e := NewRouterWithOptions(RouterOptions{AllowStubSSL: true})

	payload := map[string]any{
		"site_id": 1,
		"domain":  "ssl.example.com",
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/tasks/ssl/renew", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
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
		t.Fatalf("expected stub fallback when certbot is missing")
	}
}

func TestLogsReadTask_Success(t *testing.T) {
	e := NewRouterWithOptions(RouterOptions{AuthToken: "test-agent-token"})

	payload := map[string]any{
		"site_id": 1,
		"domain":  "logs.example.com",
		"type":    "access",
		"lines":   50,
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/tasks/logs/read", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var result map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result["ok"] != true {
		t.Fatalf("expected ok true, got %v", result["ok"])
	}
	path, _ := result["path"].(string)
	if path != "/var/log/nginx/logs.example.com.access.log" {
		t.Fatalf("unexpected path: %s", path)
	}
}

func TestLogsReadTask_InvalidType(t *testing.T) {
	e := NewRouterWithOptions(RouterOptions{AuthToken: "test-agent-token"})

	payload := map[string]any{
		"site_id": 1,
		"domain":  "logs.example.com",
		"type":    "something",
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/tasks/logs/read", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestLogsReadTask_PathTraversal(t *testing.T) {
	e := NewRouterWithOptions(RouterOptions{AuthToken: "test-agent-token"})

	payload := map[string]any{
		"site_id": 1,
		"domain":  "../etc/passwd",
		"type":    "access",
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/tasks/logs/read", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestBackupCreateTask_Success(t *testing.T) {
	rootDir := t.TempDir()
	backupDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(rootDir, "index.html"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	e := NewRouterWithOptions(RouterOptions{
		BackupDir: backupDir,
	})

	payload := map[string]any{
		"backup_id": 1,
		"site_id":   1,
		"domain":    "backup.example.com",
		"root_path": rootDir,
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/tasks/backup/create", bytes.NewReader(body))
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
	path, _ := result["path"].(string)
	if path == "" {
		t.Fatal("expected path field")
	}
	size, ok := result["size_bytes"].(float64)
	if !ok || size == 0 {
		t.Fatal("expected size_bytes field")
	}
}

func TestBackupCreateTask_InvalidPayload(t *testing.T) {
	e := NewRouterWithOptions(RouterOptions{AuthToken: "test-agent-token"})

	req := httptest.NewRequest(http.MethodPost, "/tasks/backup/create", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestBackupRestoreTask_Success(t *testing.T) {
	backupDir := t.TempDir()
	parentDir := t.TempDir()
	rootDir := filepath.Join(parentDir, "backup.example.com")
	if err := os.MkdirAll(rootDir, 0755); err != nil {
		t.Fatalf("mkdir root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(rootDir, "index.html"), []byte("old"), 0644); err != nil {
		t.Fatalf("write old file: %v", err)
	}
	sourceParent := t.TempDir()
	sourceRoot := filepath.Join(sourceParent, "backup.example.com")
	if err := os.MkdirAll(sourceRoot, 0755); err != nil {
		t.Fatalf("mkdir source root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceRoot, "index.html"), []byte("restored"), 0644); err != nil {
		t.Fatalf("write source file: %v", err)
	}
	backupPath := filepath.Join(backupDir, "1.tar.gz")
	if output, err := (&executor.Executor{}).RunShell(context.Background(), "tar", "-czf", backupPath, "-C", sourceParent, "backup.example.com"); err != nil {
		t.Fatalf("create test tar: %v: %s", err, output)
	}
	e := NewRouterWithOptions(RouterOptions{BackupDir: backupDir})

	payload := map[string]any{
		"backup_id":   1,
		"site_id":     1,
		"backup_path": backupPath,
		"root_path":   rootDir,
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/tasks/backup/restore", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var result map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result["ok"] != true {
		t.Fatalf("expected ok true, got %v", result["ok"])
	}
	extracted, ok := result["files_extracted"].(float64)
	if !ok || extracted == 0 {
		t.Fatal("expected files_extracted field")
	}
}
