package agenthttp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
)

// setupTestBinaries creates fake nginx and systemctl scripts in a temp dir
// and prepends that dir to PATH so RunShell finds them.
func setupTestBinaries(t *testing.T) string {
	t.Helper()
	binDir := t.TempDir()

	fakeNginx := `#!/bin/sh
if [ "$1" = "-t" ]; then
	echo "nginx: the configuration file syntax is ok"
	echo "nginx: configuration file test is successful"
	exit 0
fi
exec /usr/local/bin/nginx "$@"
`
	if err := os.WriteFile(filepath.Join(binDir, "nginx"), []byte(fakeNginx), 0755); err != nil {
		t.Fatal(err)
	}

	fakeSystemctl := `#!/bin/sh
echo "done"
exit 0
`
	if err := os.WriteFile(filepath.Join(binDir, "systemctl"), []byte(fakeSystemctl), 0755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("SIGARDAPANEL_AGENT_TOKEN", "test-agent-token")
	return binDir
}

// setupGitRepo initializes a local git repo with one commit and returns its absolute path.
func setupGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	repoDir := filepath.Join(dir, "repo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "init", repoDir).CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, out)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", repoDir, "add", ".").CombinedOutput(); err != nil {
		t.Fatalf("git add failed: %v\n%s", err, out)
	}
	if out, err := exec.Command("git", "-C", repoDir, "commit", "-m", "init", "--no-gpg-sign").CombinedOutput(); err != nil {
		t.Fatalf("git commit failed: %v\n%s", err, out)
	}
	return repoDir
}

func TestSiteCreateTask_Real(t *testing.T) {
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

	// Verify files on disk
	if _, err := os.Stat(rootDir); err != nil {
		t.Fatalf("root_path should exist: %v", err)
	}
	indexPath := filepath.Join(rootDir, "index.html")
	if _, err := os.Stat(indexPath); err != nil {
		t.Fatalf("index.html should exist: %v", err)
	}
	nginxConfigPath := filepath.Join(nginxDir, "test.example.com.conf")
	if _, err := os.Stat(nginxConfigPath); err != nil {
		t.Fatalf("nginx config should exist: %v", err)
	}
}

func TestSiteCreateTask_InvalidDomain(t *testing.T) {
	setupTestBinaries(t)
	e := NewRouterWithOptions(RouterOptions{
		NginxConfDir: t.TempDir(),
	})

	payload := map[string]any{
		"site_id":   1,
		"domain":    "-invalid",
		"runtime":   "static",
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

func TestSiteCreateTask_InvalidPath(t *testing.T) {
	setupTestBinaries(t)
	e := NewRouterWithOptions(RouterOptions{
		NginxConfDir: t.TempDir(),
	})

	payload := map[string]any{
		"site_id":   1,
		"domain":    "test.example.com",
		"runtime":   "static",
		"root_path": "/var/www/../etc/passwd",
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

func TestBackupCreateTask_Real(t *testing.T) {
	rootDir := t.TempDir()
	backupDir := t.TempDir()

	// Create some files to back up
	if err := os.WriteFile(filepath.Join(rootDir, "index.html"), []byte("<h1>hello</h1>"), 0644); err != nil {
		t.Fatal(err)
	}
	subDir := filepath.Join(rootDir, "css")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "style.css"), []byte("body{}"), 0644); err != nil {
		t.Fatal(err)
	}

	e := NewRouterWithOptions(RouterOptions{
		BackupDir: backupDir,
	})

	payload := map[string]any{
		"backup_id": 1,
		"site_id":   42,
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
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("backup file should exist: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("backup file should be non-empty")
	}
}

func TestLogsReadTask_Real(t *testing.T) {
	logDir := t.TempDir()
	logFile := filepath.Join(logDir, "logs.example.com.access.log")
	lines := []string{
		"127.0.0.1 - - [01/Jan/2024:00:00:00 +0000] \"GET / HTTP/1.1\" 200 123",
		"127.0.0.1 - - [01/Jan/2024:00:00:01 +0000] \"GET /api HTTP/1.1\" 200 456",
		"127.0.0.1 - - [01/Jan/2024:00:00:02 +0000] \"POST /data HTTP/1.1\" 201 789",
	}
	content := lines[0] + "\n" + lines[1] + "\n" + lines[2] + "\n"
	if err := os.WriteFile(logFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	e := NewRouterWithOptions(RouterOptions{
		LogDir: logDir,
	})

	payload := map[string]any{
		"site_id": 1,
		"domain":  "logs.example.com",
		"type":    "access",
		"lines":   2,
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/tasks/logs/read", bytes.NewReader(body))
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

	resultLines, ok := result["lines"].([]interface{})
	if !ok {
		t.Fatalf("expected lines array, got %T", result["lines"])
	}
	if len(resultLines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(resultLines))
	}
	// Verify content: tail -n 2 returns lines 2 and 3 (1-indexed), so [0] is "GET /api"
	firstLine, _ := resultLines[0].(string)
	if !contains(firstLine, "GET /api") {
		t.Fatalf("unexpected first line content: %s", firstLine)
	}
	lastLine, _ := resultLines[1].(string)
	if !contains(lastLine, "POST /data") {
		t.Fatalf("unexpected last line content: %s", lastLine)
	}
}

func TestNginxReloadTask_BestEffort(t *testing.T) {
	// No fake binaries so systemctl should not be found on macOS.
	// The handler is best-effort and should return 200.
	e := NewRouterWithOptions(RouterOptions{AuthToken: "test-agent-token"})

	req := httptest.NewRequest(http.MethodPost, "/tasks/nginx/reload", nil)
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

func TestSiteDeployTask_Real(t *testing.T) {
	repoDir := setupGitRepo(t)
	rootDir := t.TempDir()
	deployDir := filepath.Join(rootDir, "deploy")

	// Create .gitconfig if not present to avoid "Please tell me who you are" errors in CI
	_ = exec.Command("git", "config", "--global", "user.email", "test@example.com").Run()
	_ = exec.Command("git", "config", "--global", "user.name", "Test User").Run()

	// Ensure the repo has a main branch (commit already created above)
	// Re-initialize with proper config in case global config was missing
	if out, err := exec.Command("git", "-C", repoDir, "commit", "--amend", "-m", "init", "--no-gpg-sign").CombinedOutput(); err != nil {
		t.Logf("git amend output: %s", out)
	}

	e := NewRouterWithOptions(RouterOptions{AuthToken: "test-agent-token"})

	repoURL := "file://" + repoDir
	payload := map[string]any{
		"site_id":   1,
		"repo":      repoURL,
		"branch":    "master",
		"root_path": deployDir,
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
	commit, _ := result["commit"].(string)
	if commit == "" {
		t.Fatal("expected non-empty commit")
	}
	// Verify files were cloned
	readmePath := filepath.Join(deployDir, "README.md")
	if _, err := os.Stat(readmePath); err != nil {
		t.Fatalf("deployed README.md should exist: %v", err)
	}
}

func TestValidation(t *testing.T) {
	tests := []struct {
		name  string
		fn    func(string) bool
		input string
		want  bool
	}{
		{"valid domain", isValidDomain, "example.com", true},
		{"valid subdomain", isValidDomain, "sub.example.com", true},
		{"invalid domain start", isValidDomain, "-invalid", false},
		{"valid path", isValidPath, "/var/www/site", true},
		{"path traversal", isValidPath, "/var/www/../etc", false},
		{"relative path", isValidPath, "var/www", false},
		{"valid git https", isValidGitURL, "https://github.com/user/repo.git", true},
		{"valid git ssh", isValidGitURL, "git@github.com:user/repo.git", true},
		{"valid git file", isValidGitURL, "file:///tmp/repo", true},
		{"invalid git url", isValidGitURL, "ftp://host/repo", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.fn(tt.input)
			if got != tt.want {
				t.Fatalf("%s(%q) = %v, want %v", tt.name, tt.input, got, tt.want)
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestSiteDeployTask_Pull(t *testing.T) {
	repoDir := setupGitRepo(t)
	deployDir := t.TempDir()

	// Pre-clone the repo so the handler takes the "pull" branch
	repoURL := "file://" + repoDir
	if out, err := exec.Command("git", "clone", "-b", "master", repoURL, deployDir).CombinedOutput(); err != nil {
		t.Fatalf("git clone failed: %v\n%s", err, out)
	}

	// Add a new commit to the remote repo
	newFile := filepath.Join(repoDir, "new.txt")
	if err := os.WriteFile(newFile, []byte("new content"), 0644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", repoDir, "add", ".").CombinedOutput(); err != nil {
		t.Fatalf("git add failed: %v\n%s", err, out)
	}
	if out, err := exec.Command("git", "-C", repoDir, "commit", "-m", "second", "--no-gpg-sign").CombinedOutput(); err != nil {
		t.Fatalf("git commit failed: %v\n%s", err, out)
	}

	e := NewRouterWithOptions(RouterOptions{AuthToken: "test-agent-token"})

	payload := map[string]any{
		"site_id":   1,
		"repo":      repoURL,
		"branch":    "master",
		"root_path": deployDir,
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
	// Verify the new file is present after pull
	newFilePath := filepath.Join(deployDir, "new.txt")
	if _, err := os.Stat(newFilePath); err != nil {
		t.Fatalf("new.txt should exist after pull: %v", err)
	}
}

func TestBackupCreateTask_SizeCorrect(t *testing.T) {
	rootDir := t.TempDir()
	backupDir := t.TempDir()

	bigContent := make([]byte, 4096)
	for i := range bigContent {
		bigContent[i] = byte('a' + i%26)
	}
	if err := os.WriteFile(filepath.Join(rootDir, "big.txt"), bigContent, 0644); err != nil {
		t.Fatal(err)
	}

	e := NewRouterWithOptions(RouterOptions{
		BackupDir: backupDir,
	})

	payload := map[string]any{
		"backup_id": 99,
		"site_id":   7,
		"domain":    "size.example.com",
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

	sizeFloat, ok := result["size_bytes"].(float64)
	if !ok {
		t.Fatalf("expected size_bytes as float64, got %T", result["size_bytes"])
	}
	if sizeFloat <= 0 {
		t.Fatalf("expected positive size, got %v", sizeFloat)
	}
	// Verify the tar.gz is really there using the path from result
	path, _ := result["path"].(string)
	if path == "" {
		t.Fatal("expected path in result")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected backup at %s: %v", path, err)
	}
}

func TestLogsReadTask_NonExisting(t *testing.T) {
	logDir := t.TempDir()
	e := NewRouterWithOptions(RouterOptions{
		LogDir: logDir,
	})

	payload := map[string]any{
		"site_id": 1,
		"domain":  "missing.example.com",
		"type":    "error",
		"lines":   50,
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/tasks/logs/read", bytes.NewReader(body))
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
	lines, ok := result["lines"].([]interface{})
	if !ok {
		t.Fatalf("expected lines array, got %T", result["lines"])
	}
	if len(lines) != 0 {
		t.Fatalf("expected 0 lines for missing file, got %d", len(lines))
	}
}

func TestLogsReadTask_ManyLines(t *testing.T) {
	logDir := t.TempDir()
	logFile := filepath.Join(logDir, "many.example.com.access.log")

	var b bytes.Buffer
	for i := 0; i < 150; i++ {
		fmt.Fprintf(&b, "line %d\n", i)
	}
	if err := os.WriteFile(logFile, b.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}

	e := NewRouterWithOptions(RouterOptions{
		LogDir: logDir,
	})

	payload := map[string]any{
		"site_id": 1,
		"domain":  "many.example.com",
		"type":    "access",
		"lines":   10,
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/tasks/logs/read", bytes.NewReader(body))
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
	lines, ok := result["lines"].([]interface{})
	if !ok {
		t.Fatalf("expected lines array, got %T", result["lines"])
	}
	if len(lines) != 10 {
		t.Fatalf("expected 10 lines, got %d", len(lines))
	}
	first, _ := lines[0].(string)
	if first != "line 140" {
		t.Fatalf("expected first line 'line 140', got %s", first)
	}
}

func TestSiteCreateTask_RuntimeNode(t *testing.T) {
	setupTestBinaries(t)
	rootDir := t.TempDir()
	nginxDir := t.TempDir()
	e := NewRouterWithOptions(RouterOptions{
		NginxConfDir: nginxDir,
	})

	payload := map[string]any{
		"site_id":   2,
		"name":      "Node App",
		"domain":    "node.example.com",
		"runtime":   "node",
		"root_path": rootDir,
		"port":      3000,
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/tasks/site/create", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify nginx config contains proxy_pass
	configPath := filepath.Join(nginxDir, "node.example.com.conf")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("nginx config should exist: %v", err)
	}
	if !bytes.Contains(data, []byte("proxy_pass")) {
		t.Fatal("node runtime config should contain proxy_pass")
	}
	if !bytes.Contains(data, []byte("3000")) {
		t.Fatal("node runtime config should contain port 3000")
	}
}

func TestNginxReloadTask_FakeSuccess(t *testing.T) {
	setupTestBinaries(t)
	e := NewRouterWithOptions(RouterOptions{AuthToken: "test-agent-token"})

	req := httptest.NewRequest(http.MethodPost, "/tasks/nginx/reload", nil)
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
	if msg != "nginx reloaded" {
		t.Fatalf("unexpected message: %s", msg)
	}
}

func TestBackupCreateTask_InvalidPath(t *testing.T) {
	e := NewRouterWithOptions(RouterOptions{
		BackupDir: t.TempDir(),
	})

	payload := map[string]any{
		"backup_id": 1,
		"site_id":   1,
		"domain":    "bad.example.com",
		"root_path": "../var/www/evil",
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/tasks/backup/create", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestSiteDeployTask_InvalidRepo(t *testing.T) {
	e := NewRouterWithOptions(RouterOptions{AuthToken: "test-agent-token"})

	payload := map[string]any{
		"site_id":   1,
		"repo":      "ftp://bad-host/repo.git",
		"branch":    "main",
		"root_path": t.TempDir(),
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/tasks/site/deploy", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestExecutorIntegrationInRouter(t *testing.T) {
	setupTestBinaries(t)
	rootDir := t.TempDir()
	nginxDir := t.TempDir()
	logDir := t.TempDir()
	backupDir := t.TempDir()

	e := NewRouterWithOptions(RouterOptions{
		NginxConfDir: nginxDir,
		LogDir:       logDir,
		BackupDir:    backupDir,
	})

	// Create site
	payload := map[string]any{
		"site_id":   1,
		"domain":    "full.example.com",
		"runtime":   "static",
		"root_path": rootDir,
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/tasks/site/create", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("site create failed: %d %s", rec.Code, rec.Body.String())
	}

	// Create log file and read it
	logFile := filepath.Join(logDir, "full.example.com.access.log")
	if err := os.WriteFile(logFile, []byte("log line 1\nlog line 2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	payload = map[string]any{
		"site_id": 1,
		"domain":  "full.example.com",
		"type":    "access",
		"lines":   100,
	}
	body, _ = json.Marshal(payload)
	req = httptest.NewRequest(http.MethodPost, "/tasks/logs/read", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("logs read failed: %d %s", rec.Code, rec.Body.String())
	}
	var lr map[string]any
	json.NewDecoder(rec.Body).Decode(&lr)
	lines, _ := lr["lines"].([]interface{})
	if len(lines) != 2 {
		t.Fatalf("expected 2 log lines, got %d", len(lines))
	}

	// Create backup
	payload = map[string]any{
		"backup_id": 1,
		"site_id":   1,
		"domain":    "full.example.com",
		"root_path": rootDir,
	}
	body, _ = json.Marshal(payload)
	req = httptest.NewRequest(http.MethodPost, "/tasks/backup/create", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("backup create failed: %d %s", rec.Code, rec.Body.String())
	}
	var br map[string]any
	json.NewDecoder(rec.Body).Decode(&br)
	if br["ok"] != true {
		t.Fatal("backup not ok")
	}
	size, _ := br["size_bytes"].(float64)
	if size <= 0 {
		t.Fatalf("expected positive backup size, got %v", size)
	}
}

func floatToInt(v any) int64 {
	if f, ok := v.(float64); ok {
		return int64(f)
	}
	if i, ok := v.(int64); ok {
		return i
	}
	if i, ok := v.(int); ok {
		return int64(i)
	}
	s, _ := strconv.ParseInt(fmt.Sprintf("%v", v), 10, 64)
	return s
}
