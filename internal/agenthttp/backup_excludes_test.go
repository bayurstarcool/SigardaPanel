package agenthttp

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestExcludePatterns(t *testing.T) {
	if _, err := cleanExcludePatterns([]string{"node_modules", "*.log", "cache/tmp"}); err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][]string{{"*"}, {"a\nb"}, {"[x"}} {
		if _, err := cleanExcludePatterns(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	pats := []string{"node_modules", "*.log", "cache/tmp"}
	for rel, want := range map[string]bool{
		"node_modules": true, "app/node_modules": true, "app/node_modules/x/y.js": true,
		"err.log": true, "logs/a.log": true, "cache/tmp": true, "site/cache/tmp/f": true,
		"index.php": false, "cache/keep": false,
	} {
		if got := matchesExclude(rel, pats); got != want {
			t.Errorf("%s = %v, want %v", rel, got, want)
		}
	}
}

func TestCarryExcludedKeepsWhatTheArchiveLeftOut(t *testing.T) {
	old, restored := t.TempDir(), t.TempDir()
	os.MkdirAll(filepath.Join(old, "app", "node_modules", "pkg"), 0o755)
	os.WriteFile(filepath.Join(old, "app", "node_modules", "pkg", "i.js"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(old, "debug.log"), []byte("log"), 0o644)
	os.WriteFile(filepath.Join(old, "index.php"), []byte("old"), 0o644)
	os.MkdirAll(filepath.Join(restored, "app"), 0o755)
	os.WriteFile(filepath.Join(restored, "index.php"), []byte("restored"), 0o644)

	carryExcluded(old, restored, []string{"node_modules", "*.log"})
	if _, err := os.Stat(filepath.Join(restored, "app", "node_modules", "pkg", "i.js")); err != nil {
		t.Fatal("excluded directory not kept")
	}
	if _, err := os.Stat(filepath.Join(restored, "debug.log")); err != nil {
		t.Fatal("excluded file not kept")
	}
	if b, _ := os.ReadFile(filepath.Join(restored, "index.php")); string(b) != "restored" {
		t.Fatal("restored file overwritten")
	}
}

func TestBackupCreateHonoursExcludePatterns(t *testing.T) {
	root := filepath.Join(t.TempDir(), "ex.example.com")
	os.MkdirAll(filepath.Join(root, "node_modules", "p"), 0o755)
	os.WriteFile(filepath.Join(root, "node_modules", "p", "i.js"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(root, "index.php"), []byte("<?php"), 0o644)
	os.WriteFile(filepath.Join(root, "debug.log"), []byte("log"), 0o644)
	e := NewRouterWithOptions(RouterOptions{BackupDir: t.TempDir()})
	rec := postJSON(t, e, "/tasks/backup/create", map[string]any{
		"backup_id": 5, "site_id": 1, "domain": "ex.example.com", "root_path": root,
		"exclude_patterns": []string{"node_modules", "*.log"},
	})
	if rec.Code != 200 {
		t.Fatalf("backup = %d %s", rec.Code, rec.Body.String())
	}
	var res map[string]any
	json.Unmarshal(rec.Body.Bytes(), &res)
	out, err := exec.Command("tar", "-tzf", res["path"].(string)).CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	list := string(out)
	if strings.Contains(list, "node_modules") || strings.Contains(list, "debug.log") ||
		!strings.Contains(list, "ex.example.com/index.php") || !strings.Contains(list, excludeManifest) {
		t.Fatalf("archive contents:\n%s", list)
	}
}
