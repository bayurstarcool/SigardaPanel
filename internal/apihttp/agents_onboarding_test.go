package apihttp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"testing"
)

func TestInstallScriptRendersAndParses(t *testing.T) {
	script := fmt.Sprintf(installScriptTemplate, shellQuote("https://panel.example.com"), shellQuote("tok"),
		shellQuote("v1.6.1"), shellQuote(releasePublicKey), shellQuote("1"))
	if strings.Contains(script, "%!") {
		t.Fatalf("template placeholders do not match the arguments")
	}
	cmd := exec.Command("bash", "-n")
	cmd.Stdin = strings.NewReader(script)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("bash -n: %v\n%s", err, out)
	}
	// The panel's site layout is /home/<user>/htdocs/<domain>; a /var/www site
	// root made every site.create on agents installed this way fail.
	if strings.Contains(script, "SIGARDAPANEL_AGENT_SITE_ROOT:-/var/www") {
		t.Fatal("installer still restricts site files to /var/www")
	}
	if strings.Contains(script, "\nmkdir -p \"$SITE_ROOT\"\n") {
		t.Fatal("installer runs mkdir on an empty SITE_ROOT")
	}
}

func createRegToken(t *testing.T, publicURL string) map[string]any {
	t.Helper()
	t.Setenv("SIGARDAPANEL_PUBLIC_URL", publicURL)
	e, s, ctx := newTestApp(t)
	token := newAdminToken(t, s, ctx)
	rec := doRequest(t, e, "POST", "/api/v1/agents/tokens", nil, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create token: %d %s", rec.Code, rec.Body.String())
	}
	var res struct {
		Data map[string]any `json:"data"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&res)
	return res.Data
}

func TestRegistrationTokenReturnsInstallCommand(t *testing.T) {
	d := createRegToken(t, "https://panel.example.com/")
	cmd, _ := d["install_command"].(string)
	tok, _ := d["token"].(string)
	if tok == "" || cmd == "" {
		t.Fatalf("missing token/install_command: %v", d)
	}
	if !strings.Contains(cmd, "X-Sigarda-Registration-Token: "+tok) {
		t.Fatalf("token must travel in the header: %s", cmd)
	}
	if !strings.Contains(cmd, "'https://panel.example.com/api/v1/agents/install'") {
		t.Fatalf("unexpected install URL: %s", cmd)
	}
}

func TestRegistrationTokenExplainsMissingHTTPS(t *testing.T) {
	d := createRegToken(t, "http://10.0.0.1")
	if _, ok := d["install_command"]; ok {
		t.Fatal("install_command offered although the installer cannot work over http")
	}
	if msg, _ := d["installer_error"].(string); !strings.Contains(msg, "https") {
		t.Fatalf("installer_error = %q", msg)
	}
}

func TestRegisterAgentPrivateIPNeedsOptIn(t *testing.T) {
	for _, allow := range []bool{false, true} {
		t.Run(fmt.Sprint("allow=", allow), func(t *testing.T) {
			if allow {
				t.Setenv("SIGARDAPANEL_ALLOW_PRIVATE_AGENTS", "true")
			}
			e, s, ctx := newTestApp(t)
			admin := newAdminToken(t, s, ctx)
			rec := doRequest(t, e, "POST", "/api/v1/agents/tokens", nil, admin)
			var tr struct {
				Data struct {
					Token string `json:"token"`
				} `json:"data"`
			}
			_ = json.NewDecoder(rec.Body).Decode(&tr)

			rec = doRequest(t, e, "POST", "/api/v1/agents/register", map[string]any{
				"registration_token": tr.Data.Token, "hostname": "web1", "ip_address": "10.0.0.5", "agent_token": "agent-tok",
			}, "")
			if allow && rec.Code != http.StatusCreated {
				t.Fatalf("opt-in: expected 201, got %d %s", rec.Code, rec.Body.String())
			}
			if !allow {
				if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "SIGARDAPANEL_ALLOW_PRIVATE_AGENTS") {
					t.Fatalf("expected 400 naming the opt-in, got %d %s", rec.Code, rec.Body.String())
				}
			}
		})
	}
}
