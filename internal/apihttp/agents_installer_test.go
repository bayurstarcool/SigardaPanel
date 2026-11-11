package apihttp

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os/exec"
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
	for _, bad := range []string{`x"; touch /tmp/pwned; #`, `$(touch /tmp/pwned)`, "`touch /tmp/pwned`", `x'; touch /tmp/pwned; #`} {
		script := fmt.Sprintf(installScriptTemplate, shellQuote(bad), shellQuote(bad), shellQuote("v1.5.0"), shellQuote(releasePublicKey), shellQuote("0"))
		out, err := exec.Command("bash", "-c", script[:strings.Index(script, "INSTALL_DIR=")]+`printf '%s\n%s' "$PANEL_URL" "$REG_TOKEN"`).CombinedOutput()
		if err != nil || string(out) != bad+"\n"+bad {
			t.Fatalf("value %q did not survive as a literal: %q (%v)", bad, out, err)
		}
	}
}

func TestGeneratedInstallerVerifiesSignedRelease(t *testing.T) {
	for _, want := range []string{"SHA256SUMS.minisig", `minisign -Vm "$SUMS_DIR/SHA256SUMS" -P "$PUBKEY"`, `.data.server_id`, "sigardapanel-linux-$ARCH"} {
		if !strings.Contains(installScriptTemplate, want) {
			t.Errorf("installer missing %q", want)
		}
	}
	if strings.Contains(installScriptTemplate, "v1.3.1") {
		t.Error("installer still pins an old release")
	}
}

func TestGeneratedInstallerDashboardHashMatches(t *testing.T) {
	sum := sha256.Sum256([]byte(statusDashboardScript))
	if !strings.Contains(installScriptTemplate, hex.EncodeToString(sum[:])) {
		t.Fatalf("installer pins a dashboard hash that does not match statusDashboardScript (%x)", sum)
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
