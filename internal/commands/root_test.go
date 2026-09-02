package commands

import (
	"bytes"
	"strings"
	"testing"
)

func newApp(stdout, stderr *bytes.Buffer) *App {
	return New(stdout, stderr, strings.NewReader(""))
}

func TestVersion(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	app := newApp(&stdout, &stderr)

	code := app.Run([]string{"version"})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if strings.TrimSpace(stdout.String()) != Version {
		t.Fatalf("expected version %q, got %q", Version, strings.TrimSpace(stdout.String()))
	}
}

func TestUnknownCommand(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	app := newApp(&stdout, &stderr)

	code := app.Run([]string{"missing"})
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	if !strings.Contains(stderr.String(), "unknown command") {
		t.Fatalf("expected unknown command error, got %q", stderr.String())
	}
}
