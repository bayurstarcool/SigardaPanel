package agenthttp

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sigardapanel/internal/executor"
)

func sshTestRouter(t *testing.T) (http.Handler, *Router, string) {
	t.Helper()
	bin := setupTestBinaries(t)
	for _, b := range []string{"umount", "userdel", "sshd", "mount"} {
		os.WriteFile(filepath.Join(bin, b), []byte("#!/bin/sh\nexit 0\n"), 0o755)
	}
	dir := t.TempDir()
	sites := filepath.Join(dir, "sites")
	os.MkdirAll(filepath.Join(sites, "s.example.com"), 0o755)
	opts := RouterOptions{SiteRootDir: sites, SSHHomeBase: filepath.Join(dir, "home"), FstabPath: filepath.Join(dir, "fstab"), SSHDConfigPath: filepath.Join(dir, "sshd.conf")}
	os.MkdirAll(opts.SSHHomeBase, 0o755)
	r := &Router{sshHomeBase: opts.SSHHomeBase, fstabPath: opts.FstabPath, sshdConfigPath: opts.SSHDConfigPath}
	return NewRouterWithOptions(opts), r, dir
}

func TestSSHCreateValidates(t *testing.T) {
	h, _, dir := sshTestRouter(t)
	root := filepath.Join(dir, "sites", "s.example.com")
	cases := []struct {
		body map[string]any
		want int
	}{
		{map[string]any{"username": "Bad Name", "site_root": root}, http.StatusBadRequest},
		{map[string]any{"username": "deploy-s", "site_root": "/etc"}, http.StatusBadRequest},
		{map[string]any{"username": "deploy-s", "site_root": root, "ssh_keys": "command=\"rm -rf /\" ssh-ed25519 AAAA"}, http.StatusBadRequest},
		{map[string]any{"username": "root", "site_root": root}, http.StatusConflict}, // an existing Linux user
	}
	for _, c := range cases {
		if rec := postJSON(t, h, "/tasks/sshftp/create", c.body); rec.Code != c.want {
			t.Errorf("%v = %d %s", c.body, rec.Code, rec.Body.String())
		}
	}
}

func TestSSHRemoveAccountCleansExactly(t *testing.T) {
	_, r, dir := sshTestRouter(t)
	r.executor = newTestExecutor()
	home := filepath.Join(r.sshHomeBase, "bo")
	os.MkdirAll(filepath.Join(home, "htdocs"), 0o755)
	os.WriteFile(r.fstabPath, []byte(strings.Join([]string{
		"/dev/sda1 / ext4 defaults 0 1",
		"/srv/site " + home + "/htdocs none bind 0 0  # sigardapanel:bo",
		"/usr " + filepath.Join(r.sshHomeBase, "bob") + "/usr none bind,ro 0 0  # sigardapanel:bob",
	}, "\n")+"\n"), 0o644)
	if err := r.removeSSHAccount(t.Context(), "bo", home); err != nil {
		t.Fatal(err)
	}
	fstab, _ := os.ReadFile(r.fstabPath)
	if strings.Contains(string(fstab), "sigardapanel:bo\n") || !strings.Contains(string(fstab), "sigardapanel:bob") || !strings.Contains(string(fstab), "/dev/sda1") {
		t.Fatalf("fstab after removing bo:\n%s", fstab)
	}
	if _, err := os.Stat(home); err == nil {
		t.Fatal("jail not removed")
	}
	// Refuses to remove anything outside the jail base.
	if err := r.removeSSHAccount(t.Context(), "bo", filepath.Join(dir, "sites")); err == nil {
		t.Fatal("removed a directory outside the SSH home base")
	}
}

func TestSyncSSHDConfigOnlyListsValidJails(t *testing.T) {
	_, r, _ := sshTestRouter(t)
	r.executor = newTestExecutor()
	accounts := []sshAccountRef{
		{Username: "deploy-a", HomeDir: r.sshHome("deploy-a")},
		{Username: "evil", HomeDir: "/"},
		{Username: "Bad\nMatch All", HomeDir: r.sshHome("x")},
	}
	if err := r.syncSSHDConfig(t.Context(), accounts); err != nil {
		t.Fatal(err)
	}
	conf, _ := os.ReadFile(r.sshdConfigPath)
	if !strings.Contains(string(conf), "Match User deploy-a") || strings.Contains(string(conf), "evil") || strings.Contains(string(conf), "Match All") {
		t.Fatalf("sshd config:\n%s", conf)
	}
}

func newTestExecutor() *executor.Executor { return &executor.Executor{} }
