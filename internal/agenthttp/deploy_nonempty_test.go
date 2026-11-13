package agenthttp

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestSiteDeployIntoExistingNonGitRoot(t *testing.T) {
	repoDir := setupGitRepo(t)
	_ = exec.Command("git", "-C", repoDir, "commit", "--amend", "-m", "init", "--no-gpg-sign").Run()
	branch := "master"
	if out, err := exec.Command("git", "-C", repoDir, "rev-parse", "--abbrev-ref", "HEAD").Output(); err == nil && len(out) > 1 {
		branch = string(out[:len(out)-1])
	}
	root := filepath.Join(t.TempDir(), "site")
	os.MkdirAll(root, 0o755)
	os.WriteFile(filepath.Join(root, "index.html"), []byte("<h1>starter</h1>"), 0o644)

	h := NewRouterWithOptions(RouterOptions{AuthToken: "test-agent-token"})
	agentPost(t, h, "/tasks/site/deploy", map[string]any{"site_id": 1, "repo": "file://" + repoDir, "branch": branch, "root_path": root})

	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		t.Fatal("the root is not a repository after the first deploy")
	}
	if got, err := os.ReadFile(filepath.Join(root, "README.md")); err != nil || string(got) != "hello" {
		t.Fatalf("repository files not checked out: %q %v", got, err)
	}
	// A second deploy takes the pull path.
	agentPost(t, h, "/tasks/site/deploy", map[string]any{"site_id": 1, "repo": "file://" + repoDir, "branch": branch, "root_path": root})
}
