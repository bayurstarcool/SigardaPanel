package apihttp

import (
	"fmt"
	"strings"
	"testing"
)

func TestGeneratedAgentInstallerProductionContract(t *testing.T) {
	required := []string{
		"SIGARDAPANEL_AGENT_ADDR=:7710",
		"/etc/profile.d/sigardapanel-info.sh",
		"curl http://localhost:7710/health",
		"systemctl enable \"$SERVICE_NAME\"",
	}
	for _, want := range required {
		if !strings.Contains(installScriptTemplate, want) {
			t.Errorf("generated installer missing %q", want)
		}
	}

	forbidden := []string{
		"SIGARDAPANEL_AGENT_ADDR=:9090",
		"http://$SERVER_IP:9090",
	}
	for _, bad := range forbidden {
		if strings.Contains(installScriptTemplate, bad) {
			t.Errorf("generated installer contains stale contract %q", bad)
		}
	}
}

func TestGeneratedAgentInstallerDoesNotUseMissingDownloadEndpoint(t *testing.T) {
	if strings.Contains(installScriptTemplate, "/api/v1/agent/download") {
		t.Fatal("generated installer references an API endpoint that is not registered")
	}
}

func TestGeneratedAgentInstallerShellQuotesDynamicValues(t *testing.T) {
	malicious := `x"; touch /tmp/pwned; #`
	script := fmt.Sprintf(installScriptTemplate, fmt.Sprintf("%q", malicious), fmt.Sprintf("%q", malicious))
	if strings.Contains(script, `PANEL_URL="x"; touch`) || strings.Contains(script, `REG_TOKEN="x"; touch`) {
		t.Fatal("dynamic installer values escaped their assignment and became shell code")
	}
}

func TestInstallEndpointDoesNotAcceptTokenInQueryString(t *testing.T) {
	if strings.Contains(installScriptTemplate, "?token=") {
		t.Fatal("generated installer leaks registration token in URL")
	}
}

func TestGeneratedInstallerHasNoDashboardFallback(t *testing.T) {
	if strings.Contains(installScriptTemplate, "status-dashboard\" -o \"$DASHBOARD_TMP\" ||") {
		t.Fatal("generated installer has a dashboard fallback")
	}
}

func TestGeneratedInstallerCleansPreviousBinaryOnExit(t *testing.T) {
	if !strings.Contains(installScriptTemplate, `rm -rf "$ROLLBACK_DIR" "$INSTALL_DIR/.sigardapanel.previous"`) {
		t.Fatal("generated installer does not clean previous binary artifact")
	}
}
