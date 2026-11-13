package agenthttp

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCronSyncWritesAndRemovesSiteFile(t *testing.T) {
	sites, cronDir := t.TempDir(), t.TempDir()
	root := filepath.Join(sites, "c.example.com")
	h := NewRouterWithOptions(RouterOptions{SiteRootDir: sites, CronDir: cronDir, CronUserOK: func(n string) bool { return n == "alice" }})
	body := map[string]any{"site_id": 7, "system_user": "alice", "root_path": root, "jobs": []map[string]any{
		{"schedule": "*/5 * * * *", "command": "php artisan schedule:run"},
		{"schedule": "0 3 * * *", "command": "date +%F >> log.txt"},
	}}
	if rec := postJSON(t, h, "/tasks/cron/sync", body); rec.Code != http.StatusOK {
		t.Fatalf("sync = %d %s", rec.Code, rec.Body.String())
	}
	raw, err := os.ReadFile(filepath.Join(cronDir, "sigardapanel-site-7"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	for _, want := range []string{
		"*/5 * * * * alice cd '" + root + "' && php artisan schedule:run\n",
		`0 3 * * * alice cd '` + root + `' && date +\%F >> log.txt` + "\n",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q in:\n%s", want, s)
		}
	}
	if rec := postJSON(t, h, "/tasks/cron/sync", map[string]any{"site_id": 7}); rec.Code != http.StatusOK {
		t.Fatalf("clear = %d", rec.Code)
	}
	if _, err := os.Stat(filepath.Join(cronDir, "sigardapanel-site-7")); err == nil {
		t.Fatal("cron file left after clearing")
	}
}

func TestCronSyncRefusesUnsafeInput(t *testing.T) {
	sites, cronDir := t.TempDir(), t.TempDir()
	root := filepath.Join(sites, "c.example.com")
	h := NewRouterWithOptions(RouterOptions{SiteRootDir: sites, CronDir: cronDir, CronUserOK: func(n string) bool { return n == "alice" }})
	job := func(s, c string) []map[string]any { return []map[string]any{{"schedule": s, "command": c}} }
	for _, b := range []map[string]any{
		{"site_id": 1, "system_user": "root", "root_path": root, "jobs": job("* * * * *", "id")},
		{"site_id": 1, "system_user": "alice", "root_path": "/etc", "jobs": job("* * * * *", "id")},
		{"site_id": 1, "system_user": "alice", "root_path": root, "jobs": job("* * * * * root", "id")},
		{"site_id": 1, "system_user": "alice", "root_path": root, "jobs": job("* * * * *", "id\n* * * * * root sh")},
	} {
		if rec := postJSON(t, h, "/tasks/cron/sync", b); rec.Code != http.StatusBadRequest {
			t.Errorf("%v = %d %s", b, rec.Code, rec.Body.String())
		}
	}
	if entries, _ := os.ReadDir(cronDir); len(entries) != 0 {
		t.Fatalf("files written for refused input: %v", entries)
	}
}
