package agenthttp

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/labstack/echo/v5"
)

func TestBackupDownloadStaysInsideTheSite(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "site.test")
	outside := filepath.Join(base, "secret.db")
	if err := os.MkdirAll(filepath.Join(root, "backup", "db"), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(root, "backup", "db", "dump.sql"), []byte("CREATE TABLE t;"), 0o600)
	os.WriteFile(outside, []byte("panel secret"), 0o600)
	os.Symlink(outside, filepath.Join(root, "backup", "db", "link.sql"))
	os.Symlink(base, filepath.Join(root, "backups"))
	os.WriteFile(filepath.Join(root, "index.php"), []byte("<?php"), 0o644)

	r := &Router{siteRootDir: base}
	get := func(rel string) (int, string) {
		e := echo.New()
		q := url.Values{"root_path": {root}, "path": {rel}}
		req := httptest.NewRequest(http.MethodGet, "/tasks/backups/download?"+q.Encode(), nil)
		rec := httptest.NewRecorder()
		if err := r.backupDownloadTask(e.NewContext(req, rec)); err != nil {
			t.Fatal(err)
		}
		return rec.Code, rec.Body.String()
	}
	if code, body := get("backup/db/dump.sql"); code != 200 || body != "CREATE TABLE t;" {
		t.Fatalf("backup: %d %q", code, body)
	}
	for _, rel := range []string{"backup/db/link.sql", "backups/secret.db", "index.php", "../secret.db", "backup/../../secret.db"} {
		if code, body := get(rel); code == 200 || body == "panel secret" {
			t.Errorf("%s: served (%d %q)", rel, code, body)
		}
	}
}
