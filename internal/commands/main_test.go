package commands

import (
	"fmt"
	"os"
	"testing"
)

// TestMain isolates the whole package from the invoking user's real CLI
// state: tests save and delete config.json / token, and must never touch
// ~/.config/sigardapanel of whoever runs `go test` (this has wiped real
// tokens before). Environment that would redirect the CLI is cleared too.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "sigardapanel-cli-test-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, "mkdtemp:", err)
		os.Exit(1)
	}
	os.Setenv("XDG_CONFIG_HOME", dir)
	for _, k := range []string{"SIGARDAPANEL_TOKEN", "SIGARDAPANEL_API_URL", "SIGARDAPANEL_OUTPUT", "SIGARDAPANEL_USER", "SIGARDAPANEL_PASSWORD"} {
		os.Unsetenv(k)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}
