package agenthttp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=Dev", "-c", "user.email=dev@example.com"}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func deployCall(t *testing.T, h http.Handler, payload map[string]any) (int, map[string]any) {
	t.Helper()
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/tasks/site/deploy", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var out map[string]any
	json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func TestDeployPipelineScriptRollbackAndFailure(t *testing.T) {
	defer useCurrentDeployUser()()
	repo := filepath.Join(t.TempDir(), "repo")
	os.MkdirAll(repo, 0o755)
	gitIn(t, repo, "init", "-b", "main")
	os.WriteFile(filepath.Join(repo, "index.html"), []byte("v1"), 0o644)
	gitIn(t, repo, "add", ".")
	gitIn(t, repo, "commit", "-m", "first version", "--no-gpg-sign")
	first := gitIn(t, repo, "rev-parse", "HEAD")
	os.WriteFile(filepath.Join(repo, "index.html"), []byte("v2"), 0o644)
	gitIn(t, repo, "commit", "-am", "second version", "--no-gpg-sign")

	root := filepath.Join(t.TempDir(), "site")
	h := NewRouterWithOptions(RouterOptions{})
	base := map[string]any{"site_id": 7, "repo": "file://" + repo, "branch": "main", "root_path": root,
		"deploy_script": "echo built > build.txt\necho script-ran"}
	code, out := deployCall(t, h, base)
	if code != 200 || out["commit_message"] != "second version" || out["commit_author"] == "" {
		t.Fatalf("deploy = %d %v", code, out)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "build.txt")); strings.TrimSpace(string(b)) != "built" {
		t.Fatal("deploy script did not run in the site root")
	}
	if log := out["deploy_log"].(string); !strings.Contains(log, "script-ran") || !strings.Contains(log, "Deploy script") {
		t.Fatalf("log = %s", log)
	}

	// Rollback to the first commit.
	rb := map[string]any{}
	for k, v := range base {
		rb[k] = v
	}
	rb["target_commit"] = first
	if code, out = deployCall(t, h, rb); code != 200 || out["commit"] != first {
		t.Fatalf("rollback = %d %v", code, out)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "index.html")); string(b) != "v1" {
		t.Fatalf("rollback content = %s", b)
	}

	// A new push is picked up again (fetch + checkout of the branch).
	os.WriteFile(filepath.Join(repo, "index.html"), []byte("v3"), 0o644)
	gitIn(t, repo, "commit", "-am", "third", "--no-gpg-sign")
	if code, out = deployCall(t, h, base); code != 200 || out["commit_message"] != "third" {
		t.Fatalf("redeploy = %d %v", code, out)
	}

	// A failing script fails the deploy, with the log.
	bad := map[string]any{}
	for k, v := range base {
		bad[k] = v
	}
	bad["deploy_script"] = "echo before\nfalse\necho never"
	code, out = deployCall(t, h, bad)
	if code != http.StatusUnprocessableEntity || out["ok"] != false || !strings.Contains(out["deploy_log"].(string), "before") ||
		strings.Contains(out["deploy_log"].(string), "never") || out["commit_message"] != "third" {
		t.Fatalf("failing script = %d %v", code, out)
	}

	// Invalid commit refs are refused before git runs.
	rb["target_commit"] = "--upload-pack=x"
	if code, _ = deployCall(t, h, rb); code != http.StatusBadRequest {
		t.Fatalf("bad commit = %d", code)
	}
}

func TestDeployKeyTaskCreatesOnceAndDeletes(t *testing.T) {
	if _, err := exec.LookPath("ssh-keygen"); err != nil {
		t.Skip("ssh-keygen not installed")
	}
	defer useCurrentDeployUser()()
	home := t.TempDir()
	prev := siteHomeDir
	siteHomeDir = func(uint32) string { return home }
	defer func() { siteHomeDir = prev }()
	h := NewRouterWithOptions(RouterOptions{})
	out := agentPost(t, h, "/tasks/site/deploy-key", map[string]any{"site_id": 3, "system_user": "x", "action": "create"})
	pub, _ := out["public_key"].(string)
	if !strings.HasPrefix(pub, "ssh-ed25519 ") || !strings.Contains(pub, "sigardapanel-site-3@") {
		t.Fatalf("public key = %v", out)
	}
	if st, err := os.Stat(filepath.Join(home, ".ssh", "sigardapanel_site_3")); err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("private key: %v %v", st, err)
	}
	again := agentPost(t, h, "/tasks/site/deploy-key", map[string]any{"site_id": 3, "system_user": "x"})
	if again["public_key"] != pub {
		t.Fatal("the key was regenerated")
	}
	agentPost(t, h, "/tasks/site/deploy-key", map[string]any{"site_id": 3, "system_user": "x", "action": "delete"})
	if _, err := os.Stat(filepath.Join(home, ".ssh", "sigardapanel_site_3")); !os.IsNotExist(err) {
		t.Fatal("key not deleted")
	}
}

func TestGitURLAcceptsSSHForms(t *testing.T) {
	for _, u := range []string{"git@github.com:o/r.git", "ssh://git@git.example.com:2222/o/r.git", "https://github.com/o/r.git"} {
		if !isValidGitURL(u) {
			t.Errorf("%s refused", u)
		}
	}
	for _, u := range []string{"ext::sh -c touch", "--upload-pack=x", "git@github.com:o/r.git;id"} {
		if isValidGitURL(u) {
			t.Errorf("%s accepted", u)
		}
	}
}
