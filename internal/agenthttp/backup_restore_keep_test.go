package agenthttp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func agentPost(t *testing.T, h http.Handler, path string, payload map[string]any) map[string]any {
	t.Helper()
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("%s = %d %s", path, rec.Code, rec.Body.String())
	}
	var out map[string]any
	json.NewDecoder(rec.Body).Decode(&out)
	return out
}

func TestBackupRestoreKeepsExcludedAndNestedFiles(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "keep.example.com")
	write := func(rel, content string) {
		p := filepath.Join(root, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(content), 0o644)
	}
	write("index.php", "v1")
	write(".env", "APP_KEY=secret")
	write("vendor/lib/a.php", "lib")
	write("storage/framework/cache/keep.txt", "nested cache")
	write("app/backups/report.csv", "nested backups dir")
	h := NewRouterWithOptions(RouterOptions{BackupDir: t.TempDir()})

	out := agentPost(t, h, "/tasks/backup/create", map[string]any{"backup_id": 7, "site_id": 1, "domain": "keep.example.com", "root_path": root})
	archive, _ := out["path"].(string)
	write("index.php", "v2 (to be rolled back)")

	agentPost(t, h, "/tasks/backup/restore", map[string]any{"backup_id": 7, "site_id": 1, "domain": "keep.example.com", "root_path": root, "backup_path": archive})

	check := func(rel, want string) {
		got, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil || string(got) != want {
			t.Errorf("%s = %q, %v; want %q", rel, got, err, want)
		}
	}
	check("index.php", "v1")
	check(".env", "APP_KEY=secret")
	check("vendor/lib/a.php", "lib")
	check("storage/framework/cache/keep.txt", "nested cache")
	check("app/backups/report.csv", "nested backups dir")
	if _, err := os.Stat(archive); err != nil {
		t.Errorf("the restored backup archive itself was deleted: %v", err)
	}
}
