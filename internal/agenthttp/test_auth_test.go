package agenthttp

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	if os.Getenv("SIGARDAPANEL_AGENT_TOKEN") == "" {
		_ = os.Setenv("SIGARDAPANEL_AGENT_TOKEN", "test-agent-token")
	}
	// CI runners have no git identity: fixture repositories commit with this one.
	for k, v := range map[string]string{
		"GIT_AUTHOR_NAME": "sigardapanel-test", "GIT_AUTHOR_EMAIL": "test@example.invalid",
		"GIT_COMMITTER_NAME": "sigardapanel-test", "GIT_COMMITTER_EMAIL": "test@example.invalid",
	} {
		if os.Getenv(k) == "" {
			_ = os.Setenv(k, v)
		}
	}
	os.Exit(m.Run())
}
