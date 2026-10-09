package agenthttp

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"sigardapanel/internal/executor"
)

// A site owner controls .git/hooks. Before the fix the root agent ran
// `git checkout` inside such a repo and executed the hook as root.
func TestRunGitDropsPrivilegesForUserOwnedRepo(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("needs root to chown")
	}
	const nobody = 65534
	base, err := os.MkdirTemp("", "rungit")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(base)
	os.Chmod(base, 0o755)
	repo := filepath.Join(base, "site")
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repo, "-c", "user.email=a@b.c", "-c", "user.name=t"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	os.MkdirAll(repo, 0o755)
	run("init", "-q", "-b", "main")
	os.WriteFile(filepath.Join(repo, "f"), []byte("1"), 0o644)
	run("add", ".")
	run("commit", "-q", "-m", "one")
	run("branch", "other")

	marker := filepath.Join(repo, "hook-ran-as")
	hook := "#!/bin/sh\nid -u > " + marker + "\n"
	os.WriteFile(filepath.Join(repo, ".git", "hooks", "post-checkout"), []byte(hook), 0o755)
	if out, err := exec.Command("chown", "-R", "65534:65534", repo).CombinedOutput(); err != nil {
		t.Fatalf("chown: %v %s", err, out)
	}

	r := &Router{executor: &executor.Executor{}}
	if out, err := r.runGit(context.Background(), repo, "-c", "safe.directory="+repo, "checkout", "other"); err != nil {
		t.Fatalf("runGit: %v\n%s", err, out)
	}
	got, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("hook did not run at all: %v", err)
	}
	if strings.TrimSpace(string(got)) != "65534" {
		t.Fatalf("hook ran as uid %q, want 65534 (not root)", strings.TrimSpace(string(got)))
	}
}

func TestRunAsUserRefusesRoot(t *testing.T) {
	e := &executor.Executor{}
	if _, err := e.RunAsUser(context.Background(), 0, 0, "/", "echo", "x"); err == nil {
		t.Fatal("uid 0 must be refused")
	}
	if _, err := e.RunAsUser(context.Background(), 65534, 65534, "/", "rm", "-rf", "/"); err == nil {
		t.Fatal("non-allowlisted command must be refused")
	}
}
