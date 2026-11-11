package agenthttp

import (
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// fakeCommand writes an executable that appends its name and arguments to
// log and exits with code.
func fakeCommand(t *testing.T, dir, name, log string, code int) string {
	t.Helper()
	p := filepath.Join(dir, name)
	script := "#!/bin/sh\necho \"" + name + " $*\" >> '" + log + "'\nexit " + strconv.Itoa(code) + "\n"
	if err := os.WriteFile(p, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

const testSiteOwner = "spsiteownertest"

func skipIfOwnerExists(t *testing.T) {
	t.Helper()
	if _, err := user.Lookup(testSiteOwner); err == nil {
		t.Skipf("user %s exists on this machine", testSiteOwner)
	}
}

func TestSiteCreate_RejectsUnsafeSystemUser(t *testing.T) {
	setupTestBinaries(t)
	for _, name := range []string{"root", "www-data", "../x", "a b"} {
		rec := postSiteCreate(t, RouterOptions{NginxConfDir: t.TempDir()}, map[string]any{
			"site_id": 1, "domain": "x.example.com", "runtime": "static", "root_path": filepath.Join(t.TempDir(), "s"), "system_user": name,
		})
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("system_user %q: expected 400, got %d %s", name, rec.Code, rec.Body.String())
		}
	}
}

func TestSiteCreate_CreatesOwnerAndChownsNewRoot(t *testing.T) {
	skipIfOwnerExists(t)
	bin := setupTestBinaries(t)
	log := filepath.Join(t.TempDir(), "calls.log")
	useradd := fakeCommand(t, bin, "fake-useradd", log, 0)
	fakeCommand(t, bin, "chown", log, 0)

	root := filepath.Join(t.TempDir(), "htdocs", "own.example.com")
	rec := postSiteCreate(t, RouterOptions{NginxConfDir: t.TempDir(), UseraddCommand: useradd}, map[string]any{
		"site_id": 1, "domain": "own.example.com", "runtime": "static", "root_path": root, "system_user": testSiteOwner,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d %s", rec.Code, rec.Body.String())
	}
	calls, _ := os.ReadFile(log)
	for _, want := range []string{
		"fake-useradd -m -s /bin/bash " + testSiteOwner,
		"chown -R " + testSiteOwner + ": " + root,
	} {
		if !strings.Contains(string(calls), want) {
			t.Fatalf("expected %q in calls:\n%s", want, calls)
		}
	}
}

func TestSiteCreate_ExistingRootKeepsOwnership(t *testing.T) {
	skipIfOwnerExists(t)
	bin := setupTestBinaries(t)
	log := filepath.Join(t.TempDir(), "calls.log")
	useradd := fakeCommand(t, bin, "fake-useradd", log, 0)
	fakeCommand(t, bin, "chown", log, 0)

	root := t.TempDir() // already exists, maybe with someone's files
	rec := postSiteCreate(t, RouterOptions{NginxConfDir: t.TempDir(), UseraddCommand: useradd}, map[string]any{
		"site_id": 1, "domain": "own.example.com", "runtime": "static", "root_path": root, "system_user": testSiteOwner,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d %s", rec.Code, rec.Body.String())
	}
	calls, _ := os.ReadFile(log)
	if !strings.Contains(string(calls), "fake-useradd") {
		t.Fatalf("missing owner must still be created:\n%s", calls)
	}
	if strings.Contains(string(calls), "chown") {
		t.Fatalf("an existing site root must keep its ownership:\n%s", calls)
	}
}

func TestSiteCreate_UseraddFailureIsReadableAndCreatesNothing(t *testing.T) {
	skipIfOwnerExists(t)
	bin := setupTestBinaries(t)
	log := filepath.Join(t.TempDir(), "calls.log")
	useradd := fakeCommand(t, bin, "fake-useradd", log, 1)

	root := filepath.Join(t.TempDir(), "s")
	rec := postSiteCreate(t, RouterOptions{NginxConfDir: t.TempDir(), UseraddCommand: useradd}, map[string]any{
		"site_id": 1, "domain": "own.example.com", "runtime": "static", "root_path": root, "system_user": testSiteOwner,
	})
	if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), "create Linux user "+testSiteOwner) {
		t.Fatalf("expected a readable 500, got %d %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatal("site root created although the owner could not be")
	}
}

func TestSiteCreate_InjectedRouterNeverRunsRealUseradd(t *testing.T) {
	skipIfOwnerExists(t)
	setupTestBinaries(t)
	rec := postSiteCreate(t, RouterOptions{NginxConfDir: t.TempDir()}, map[string]any{
		"site_id": 1, "domain": "own.example.com", "runtime": "static", "root_path": filepath.Join(t.TempDir(), "s"), "system_user": testSiteOwner,
	})
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected the placeholder useradd to fail, got %d %s", rec.Code, rec.Body.String())
	}
	if _, err := user.Lookup(testSiteOwner); err == nil {
		t.Fatal("a real account was created")
	}
}

// A home created with HOME_MODE 0750 (Ubuntu 24.04) must become traversable,
// or nginx 404s every file under htdocs/.
func TestAllowTraverse(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "home")
	if err := os.Mkdir(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	allowTraverse(dir)
	fi, _ := os.Stat(dir)
	if fi.Mode().Perm() != 0o751 {
		t.Fatalf("mode %o, want 751", fi.Mode().Perm())
	}
	allowTraverse(filepath.Join(dir, "missing")) // no panic, no error
}

func TestSiteCreate_WithoutSystemUserTouchesNoAccounts(t *testing.T) {
	bin := setupTestBinaries(t)
	log := filepath.Join(t.TempDir(), "calls.log")
	useradd := fakeCommand(t, bin, "fake-useradd", log, 0)
	fakeCommand(t, bin, "chown", log, 0)
	rec := postSiteCreate(t, RouterOptions{NginxConfDir: t.TempDir(), UseraddCommand: useradd}, map[string]any{
		"site_id": 1, "domain": "plain.example.com", "runtime": "static", "root_path": filepath.Join(t.TempDir(), "s"),
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d %s", rec.Code, rec.Body.String())
	}
	if calls, _ := os.ReadFile(log); len(calls) != 0 {
		t.Fatalf("no account/ownership changes expected (vhost regenerations send no system_user):\n%s", calls)
	}
}
