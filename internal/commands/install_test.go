package commands

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func noPorts(t *testing.T) {
	old := portOpen
	portOpen = func(int) bool { return false }
	t.Cleanup(func() { portOpen = old })
}

func TestInstallWizardCreatesNamedSuperAdmin(t *testing.T) {
	noPorts(t)
	t.Setenv("SIGARDAPANEL_DB_PATH", filepath.Join(t.TempDir(), "wizard.db"))
	var out, errb bytes.Buffer
	app := New(&out, &errb, strings.NewReader("alice\n\nsecret123\nsecret123\n"))
	if code := app.Install(nil); code != exitOK {
		t.Fatalf("exit %d\nstdout:%s\nstderr:%s", code, out.String(), errb.String())
	}
	if !strings.Contains(out.String(), "super_admin 'alice' created") || !strings.Contains(out.String(), "--user alice") {
		t.Fatalf("wizard did not report the chosen account:\n%s", out.String())
	}

	// Second run: the super_admin exists, no prompts are needed.
	out.Reset()
	app = New(&out, &errb, strings.NewReader(""))
	if code := app.Install(nil); code != exitOK {
		t.Fatalf("second run exit %d: %s", code, errb.String())
	}
	if !strings.Contains(out.String(), "super_admin 'alice' already exists") {
		t.Fatalf("second run should detect alice:\n%s", out.String())
	}
}

func TestInstallWizardStopsOnClosedStdin(t *testing.T) {
	noPorts(t)
	t.Setenv("SIGARDAPANEL_DB_PATH", filepath.Join(t.TempDir(), "wizard.db"))
	var out, errb bytes.Buffer
	// Only the username: the password prompt hits EOF. It used to loop forever.
	app := New(&out, &errb, strings.NewReader("bob\n"))
	if code := app.Install(nil); code != exitError {
		t.Fatalf("expected exitError on closed stdin, got %d", code)
	}
}
