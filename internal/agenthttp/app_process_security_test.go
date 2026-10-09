package agenthttp

import (
	"strings"
	"testing"
)

func TestSystemdQuote(t *testing.T) {
	got := systemdQuote(`exec node a.js --name "x" 100% $HOME \n 'q'`)
	want := `"exec node a.js --name \"x\" 100%% $$HOME \\n 'q'"`
	if got != want {
		t.Fatalf("got %s\nwant %s", got, want)
	}
}

func TestUnitContentCannotInjectDirectives(t *testing.T) {
	// A newline would let the caller add e.g. ExecStartPre=; validation refuses it
	// before the unit is ever rendered.
	if err := validateAppUnitInput("a.example.com", "root", "/home/u/htdocs/a", "node server.js"); err == nil {
		t.Error("root must be refused")
	}
	if err := validateAppUnitInput("a.example.com", "", "/home/u/htdocs/a", "node server.js"); err == nil {
		t.Error("empty system_user (would run as root) must be refused")
	}
	if err := validateAppUnitInput("a.example.com", "nobody", "/home/u/htdocs/a", "node server.js"); err == nil {
		t.Error("reserved account must be refused")
	}
	if err := validateAppUnitInput("a.example.com", "daemon", "/home/u/htdocs/a\nUser=root", "node server.js"); err == nil {
		t.Error("newline in root path must be refused")
	}
	if err := validateAppUnitInput("a.example.com", "no_such_user_xyz", "/home/u/htdocs/a", "node server.js"); err == nil {
		t.Error("unknown account must be refused")
	}
	unit := systemdUnitContent("a.example.com", "app", "/home/u/htdocs/a", "node server.js\nExecStartPre=/bin/evil", 30001, 256)
	if strings.Contains(unit, "\nExecStartPre=") {
		t.Errorf("quoting failed to neutralise newline injection:\n%s", unit)
	}
}
