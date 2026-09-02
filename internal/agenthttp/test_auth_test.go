package agenthttp

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	if os.Getenv("SIGARDAPANEL_AGENT_TOKEN") == "" {
		_ = os.Setenv("SIGARDAPANEL_AGENT_TOKEN", "test-agent-token")
	}
	os.Exit(m.Run())
}
