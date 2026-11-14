package agenthttp

import (
	"bufio"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"sigardapanel/internal/siteinput"
)

// plantLink makes a symlink as a site user would (non-root owner when the
// test runs as root).
func plantLink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if os.Geteuid() == 0 {
		_ = os.Lchown(link, 4242, 4242)
	}
}

// A site user who swaps the root (or a parent) for a symlink must not get
// root to delete, archive or create through it.
func TestSiteOpsRefuseSymlinkedRoot(t *testing.T) {
	setupTestBinaries(t)
	base := t.TempDir()
	victim := filepath.Join(base, "victim")
	os.MkdirAll(victim, 0o755)
	os.WriteFile(filepath.Join(victim, "shadow"), []byte("secret"), 0o600)
	htdocs := filepath.Join(base, "home", "bob", "htdocs")
	os.MkdirAll(htdocs, 0o755)
	root := filepath.Join(htdocs, "a.example.com")
	plantLink(t, victim, root)

	h := NewRouterWithOptions(RouterOptions{NginxConfDir: t.TempDir(), BackupDir: t.TempDir()})
	if rec := postJSON(t, h, "/tasks/site/delete", map[string]any{"site_id": 1, "domain": "a.example.com", "root_path": root}); rec.Code == http.StatusOK {
		t.Fatalf("delete through a symlinked root = %d %s", rec.Code, rec.Body.String())
	}
	if rec := postJSON(t, h, "/tasks/backup/create", map[string]any{"backup_id": 1, "site_id": 1, "domain": "a.example.com", "root_path": root}); rec.Code == http.StatusOK {
		t.Fatalf("backup through a symlinked root = %d %s", rec.Code, rec.Body.String())
	}
	if rec := postJSON(t, h, "/tasks/site/usage", map[string]any{"root_path": root}); rec.Code == http.StatusOK {
		t.Fatalf("usage through a symlinked root = %d %s", rec.Code, rec.Body.String())
	}
	// Parent swapped: ~/htdocs -> victim, then a site "inside" it.
	os.RemoveAll(htdocs)
	plantLink(t, victim, htdocs)
	rec := postSiteCreate(t, RouterOptions{NginxConfDir: t.TempDir()}, map[string]any{
		"site_id": 2, "domain": "b.example.com", "runtime": "static", "root_path": filepath.Join(htdocs, "b.example.com"),
	})
	if rec.Code == http.StatusOK {
		t.Fatalf("create through a symlinked parent = %d %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(victim, "b.example.com")); err == nil {
		t.Fatal("site root created in the link target")
	}
	if b, err := os.ReadFile(filepath.Join(victim, "shadow")); err != nil || string(b) != "secret" {
		t.Fatal("link target was changed")
	}
}

// The SSH jail is created by the agent, root-owned, and never reuses what is
// already at its path (a planted directory or symlink).
func TestSSHCreateRefusesExistingJailPath(t *testing.T) {
	h, r, dir := sshTestRouter(t)
	root := filepath.Join(dir, "sites", "s.example.com")
	victim := filepath.Join(dir, "victim")
	os.MkdirAll(victim, 0o755)
	plantLink(t, victim, filepath.Join(r.sshHomeBase, "deploy-l"))
	if rec := postJSON(t, h, "/tasks/sshftp/create", map[string]any{"username": "deploy-l", "site_root": root}); rec.Code != http.StatusConflict {
		t.Fatalf("symlinked jail path = %d %s", rec.Code, rec.Body.String())
	}
	os.MkdirAll(filepath.Join(r.sshHomeBase, "deploy-d", "tmp"), 0o755)
	if rec := postJSON(t, h, "/tasks/sshftp/create", map[string]any{"username": "deploy-d", "site_root": root}); rec.Code != http.StatusConflict {
		t.Fatalf("pre-existing jail dir = %d %s", rec.Code, rec.Body.String())
	}
	if entries, _ := os.ReadDir(victim); len(entries) != 0 {
		t.Fatalf("agent wrote into the link target: %v", entries)
	}
}

func TestJailHelpersNeverFollowLinks(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(t.TempDir(), "etc")
	os.MkdirAll(target, 0o755)
	plantLink(t, target, filepath.Join(dir, "tmp"))
	if err := jailMkdir(filepath.Join(dir, "tmp"), 0o1777|os.ModeSticky); err == nil {
		t.Fatal("jailMkdir reused a symlink")
	}
	if fi, _ := os.Stat(target); fi.Mode().Perm() != 0o755 {
		t.Fatalf("link target mode changed to %v", fi.Mode())
	}
	plantLink(t, filepath.Join(target, "authorized_keys"), filepath.Join(dir, "authorized_keys"))
	if err := jailWriteFile(filepath.Join(dir, "authorized_keys"), []byte("k"), 0o644); err == nil {
		t.Fatal("jailWriteFile followed a symlink")
	}
	if _, err := os.Stat(filepath.Join(target, "authorized_keys")); err == nil {
		t.Fatal("file created in the link target")
	}
}

// Service accounts (postgres, mysql, www-data...: UID < 1000) are never used
// as a site user (terminal ?user=, file ownership, git, SSH uid).
func TestSiteUserIDsRefusesServiceAccounts(t *testing.T) {
	f, err := os.Open("/etc/passwd")
	if err != nil {
		t.Skip("no /etc/passwd")
	}
	defer f.Close()
	checked := 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		parts := strings.Split(sc.Text(), ":")
		if len(parts) < 3 {
			continue
		}
		uid, err := strconv.Atoi(parts[2])
		if err != nil || (uid >= minSiteUserUID && uid != 65534) || siteinput.SystemUser(parts[0]) != nil {
			continue
		}
		checked++
		if _, _, ok := siteUserIDs(parts[0]); ok {
			t.Errorf("service account %s (uid %d) accepted as a site user", parts[0], uid)
		}
	}
	if checked == 0 {
		t.Skip("no service accounts to check")
	}
}
