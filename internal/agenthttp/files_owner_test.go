package agenthttp

import (
	"net/http"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestFileManagerHandsNewFilesToTheSiteOwner(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("needs root to chown")
	}
	sites := t.TempDir()
	root := filepath.Join(sites, "own.example.com")
	os.MkdirAll(root, 0o755)
	os.Chown(root, 4321, 4321) // the site owner (no system_user sent)
	h := NewRouterWithOptions(RouterOptions{SiteRootDir: sites})
	for _, op := range []map[string]any{
		{"op": "create", "path": "a/b", "is_dir": true},
		{"op": "write", "path": "a/b/c.txt", "content": "x"},
	} {
		op["root_path"] = root
		if rec := postJSON(t, h, "/tasks/files/op", op); rec.Code != http.StatusOK {
			t.Fatalf("%v = %d %s", op["op"], rec.Code, rec.Body.String())
		}
	}
	for _, p := range []string{"a", "a/b", "a/b/c.txt"} {
		fi, _ := os.Stat(filepath.Join(root, p))
		if st := fi.Sys().(*syscall.Stat_t); st.Uid != 4321 {
			t.Errorf("%s owned by uid %d, want the site owner 4321", p, st.Uid)
		}
	}
	// Paths outside the agent's site root are refused.
	if rec := postJSON(t, h, "/tasks/files/op", map[string]any{"op": "list", "root_path": "/etc"}); rec.Code != http.StatusBadRequest {
		t.Fatalf("root /etc = %d", rec.Code)
	}
}
