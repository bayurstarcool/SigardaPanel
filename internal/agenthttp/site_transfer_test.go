package agenthttp

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestSiteTransferMovesRoot(t *testing.T) {
	bin := setupTestBinaries(t)
	log := filepath.Join(t.TempDir(), "calls")
	useradd := fakeCommand(t, bin, "fake-useradd", log, 0)
	sites := t.TempDir()
	h := NewRouterWithOptions(RouterOptions{SiteRootDir: sites, UseraddCommand: useradd})
	oldRoot := filepath.Join(sites, "alice", "htdocs", "t.example.com")
	newRoot := filepath.Join(sites, "bob", "htdocs", "t.example.com")
	os.MkdirAll(oldRoot, 0o755)
	os.WriteFile(filepath.Join(oldRoot, "index.php"), []byte("<?php"), 0o644)

	body := map[string]any{"site_id": 1, "domain": "t.example.com", "old_root": oldRoot, "new_root": newRoot, "system_user": "sigarda-bob"}
	if rec := postJSON(t, h, "/tasks/site/transfer", body); rec.Code != http.StatusOK {
		t.Fatalf("transfer = %d %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(newRoot, "index.php")); err != nil {
		t.Fatal("files not moved")
	}
	if _, err := os.Stat(oldRoot); err == nil {
		t.Fatal("old root left behind")
	}
	// A retry after the move succeeded is a success, not a conflict.
	if rec := postJSON(t, h, "/tasks/site/transfer", body); rec.Code != http.StatusOK {
		t.Fatalf("retried transfer = %d %s", rec.Code, rec.Body.String())
	}
	// The target exists and the old root too: a second transfer is refused.
	os.MkdirAll(oldRoot, 0o755)
	if rec := postJSON(t, h, "/tasks/site/transfer", body); rec.Code != http.StatusConflict {
		t.Fatalf("transfer onto an existing root = %d %s", rec.Code, rec.Body.String())
	}
	for _, bad := range []map[string]any{
		{"domain": "t.example.com", "old_root": oldRoot, "new_root": newRoot + "x", "system_user": "root"},
		{"domain": "t.example.com", "old_root": "/etc", "new_root": newRoot, "system_user": "sigarda-bob"},
		{"domain": "t.example.com", "old_root": oldRoot, "new_root": filepath.Join(sites, "bob", "other"), "system_user": "sigarda-bob"},
	} {
		if rec := postJSON(t, h, "/tasks/site/transfer", bad); rec.Code != http.StatusBadRequest {
			t.Errorf("%v = %d", bad, rec.Code)
		}
	}
}
