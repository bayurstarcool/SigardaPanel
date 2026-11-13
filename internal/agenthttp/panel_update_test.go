package agenthttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type fakeUpdateHost struct {
	mu     sync.Mutex
	calls  [][]string
	active bool
}

func (f *fakeUpdateHost) run(_ context.Context, name string, args ...string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, append([]string{name}, args...))
	if name == "systemctl" {
		if f.active {
			return []byte("active\n"), nil
		}
		return []byte("inactive\n"), &os.PathError{Op: "is-active", Path: "unit", Err: os.ErrNotExist}
	}
	if name == "systemd-run" {
		f.active = true
	}
	return nil, nil
}

func newUpdateRouter(t *testing.T, version string, withEnv bool) (http.Handler, *fakeUpdateHost, string) {
	t.Helper()
	dir := t.TempDir()
	env := filepath.Join(dir, "panel.env")
	if withEnv {
		os.WriteFile(env, []byte("X=1\n"), 0o600)
	}
	host := &fakeUpdateHost{}
	e := NewRouterWithOptions(RouterOptions{
		PanelUpdateRun:      host.run,
		PanelUpdateStateDir: filepath.Join(dir, "state"),
		PanelUpdateEnvFile:  env,
		PanelUpdateVersion:  version,
	})
	return e, host, filepath.Join(dir, "state")
}

func doJSON(t *testing.T, h http.Handler, method, path, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var out map[string]any
	json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func TestPanelUpdateStartAndStatus(t *testing.T) {
	h, host, state := newUpdateRouter(t, "1.6.2", true)

	code, out := doJSON(t, h, "GET", "/tasks/panel/update", "")
	if code != 200 || out["state"] != "idle" || out["supported"] != true {
		t.Fatalf("initial status = %d %v", code, out)
	}

	code, out = doJSON(t, h, "POST", "/tasks/panel/update", `{"tag":"v1.7.0"}`)
	if code != http.StatusAccepted {
		t.Fatalf("start = %d %v", code, out)
	}
	var run []string
	for _, c := range host.calls {
		if c[0] == "systemd-run" {
			run = c
		}
	}
	joined := strings.Join(run, " ")
	if !strings.Contains(joined, "--unit=sigardapanel-update") || !strings.Contains(joined, "--setenv=SIGARDA_VERSION=v1.7.0") {
		t.Fatalf("unexpected systemd-run call: %v", run)
	}
	script, err := os.ReadFile(filepath.Join(state, "install.sh"))
	if err != nil || !strings.Contains(string(script), "minisign") {
		t.Fatalf("embedded installer not written: %v", err)
	}

	// A second start while the first runs is refused.
	if code, _ := doJSON(t, h, "POST", "/tasks/panel/update", `{"tag":"v1.7.0"}`); code != http.StatusConflict {
		t.Fatalf("second start = %d, want 409", code)
	}
	if _, out := doJSON(t, h, "GET", "/tasks/panel/update", ""); out["state"] != "running" {
		t.Fatalf("status while running = %v", out)
	}

	// The wrapper reports the result through the exit file.
	os.WriteFile(filepath.Join(state, "update.log"), []byte("\x1b[0;32m[  OK  ]\x1b[0m Updated to 1.7.0\n"), 0o600)
	os.WriteFile(filepath.Join(state, "exit"), []byte("0\n"), 0o600)
	host.active = false
	_, out = doJSON(t, h, "GET", "/tasks/panel/update", "")
	if out["state"] != "succeeded" || !strings.Contains(out["log"].(string), "[  OK  ] Updated") {
		t.Fatalf("final status = %v", out)
	}
}

func TestPanelUpdateRefusals(t *testing.T) {
	cases := []struct {
		name, version, body string
		env                 bool
		want                int
	}{
		{"dev build", "dev", `{"tag":"v1.7.0"}`, true, http.StatusConflict},
		{"not a panel host", "1.6.2", `{"tag":"v1.7.0"}`, false, http.StatusConflict},
		{"downgrade", "1.6.2", `{"tag":"v1.6.1"}`, true, http.StatusConflict},
		{"same version", "1.6.2", `{"tag":"v1.6.2"}`, true, http.StatusConflict},
		{"bad tag", "1.6.2", `{"tag":"v1.7.0 --setenv=X"}`, true, http.StatusBadRequest},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h, host, _ := newUpdateRouter(t, c.version, c.env)
			code, out := doJSON(t, h, "POST", "/tasks/panel/update", c.body)
			if code != c.want {
				t.Fatalf("code = %d (%v), want %d", code, out, c.want)
			}
			for _, call := range host.calls {
				if call[0] == "systemd-run" {
					t.Fatalf("systemd-run must not run: %v", call)
				}
			}
		})
	}
}

func TestPanelUpdateInjectedRouterRunsNothing(t *testing.T) {
	e := NewRouterWithOptions(RouterOptions{PanelUpdateVersion: "1.6.2"})
	code, out := doJSON(t, e, "POST", "/tasks/panel/update", `{"tag":"v1.7.0"}`)
	if code == http.StatusAccepted {
		t.Fatalf("injected router started an update: %v", out)
	}
}
