package agenthttp

import (
	"strings"
	"testing"

	"sigardapanel/internal/nginx"
)

// The old starters must match what the pre-v1.12.3 template wrote, and the
// new template must no longer produce them.
func TestOldStartersDifferFromCurrent(t *testing.T) {
	for _, c := range []struct{ runtime, file, old string }{
		{"node", "server.js", oldNodeStarter("a.test", 30000)},
		{"go", "main.go", oldGoStarter("a.test", 30000)},
	} {
		cur := nginx.TemplateForRuntime(c.runtime, "a.test", 30000)[c.file]
		if cur == "" || cur == c.old {
			t.Fatalf("%s: current starter equals the old one", c.runtime)
		}
		if !strings.Contains(cur, "127.0.0.1") {
			t.Fatalf("%s: current starter does not bind 127.0.0.1", c.runtime)
		}
	}
}
