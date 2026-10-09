package agenthttp

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestResolveBackupFile(t *testing.T) {
	base := t.TempDir()
	site := filepath.Join(base, "home", "u", "htdocs", "a.com")
	if err := os.MkdirAll(filepath.Join(site, "backups"), 0o755); err != nil {
		t.Fatal(err)
	}
	good := filepath.Join(site, "backups", "files-1.tar.gz")
	os.WriteFile(good, []byte("x"), 0o644)
	os.WriteFile(filepath.Join(site, "backups", "notes.txt"), []byte("x"), 0o644)

	// directory swapped for a symlink to a system-like location
	evil := filepath.Join(base, "etc")
	os.MkdirAll(evil, 0o755)
	os.WriteFile(filepath.Join(evil, "victim.tar.gz"), []byte("x"), 0o644)
	os.MkdirAll(filepath.Join(site, "other"), 0o755)
	os.Symlink(evil, filepath.Join(site, "other", "backups"))

	r := &Router{backupDir: filepath.Join(base, "var", "backups", "sigardapanel")}
	if _, ok := r.resolveBackupFile(good); !ok {
		t.Error("regular backup file should be accepted")
	}
	for name, p := range map[string]string{
		"wrong extension":      filepath.Join(site, "backups", "notes.txt"),
		"symlinked parent dir": filepath.Join(site, "other", "backups", "victim.tar.gz"),
		"traversal":            filepath.Join(site, "backups", "..", "..", "x.tar.gz"),
		"system file":          "/etc/backups/passwd.tar.gz",
		"os backups dir":       "/var/backups/dpkg.tar.gz",
	} {
		if _, ok := r.resolveBackupFile(p); ok {
			t.Errorf("%s: %s must be rejected", name, p)
		}
	}
}

func TestSanitizeRestoredTree(t *testing.T) {
	root := t.TempDir()
	suid := filepath.Join(root, "bin")
	os.WriteFile(suid, []byte("x"), 0o755)
	if err := os.Chmod(suid, 0o755|os.ModeSetuid|os.ModeSetgid); err != nil {
		t.Fatal(err)
	}
	fifo := filepath.Join(root, "pipe")
	if err := syscall.Mkfifo(fifo, 0o644); err != nil {
		t.Fatal(err)
	}
	os.Symlink("/etc/passwd", filepath.Join(root, "link"))

	if err := sanitizeRestoredTree(root, os.Getuid(), os.Getgid()); err != nil {
		t.Fatal(err)
	}
	fi, _ := os.Lstat(suid)
	if fi.Mode()&(os.ModeSetuid|os.ModeSetgid) != 0 {
		t.Error("setuid/setgid must be cleared")
	}
	if _, err := os.Lstat(fifo); err == nil {
		t.Error("special files must be removed")
	}
	if _, err := os.Lstat(filepath.Join(root, "link")); err != nil {
		t.Error("symlinks are kept (not followed)")
	}
}

func TestSiteRootFallbackRejectsSystemTrees(t *testing.T) {
	r := &Router{} // no SIGARDAPANEL_AGENT_SITE_ROOT configured
	for _, p := range []string{"/etc", "/etc/nginx", "/root/x", "/home/u", "/", "/usr/share/a"} {
		if r.isAllowedSitePath(p) {
			t.Errorf("%s must not be an allowed site path", p)
		}
	}
	for _, p := range []string{"/home/u/htdocs/a.com", "/opt/gps-tracker"} {
		if !r.isAllowedSitePath(p) {
			t.Errorf("%s should be allowed", p)
		}
	}
}
