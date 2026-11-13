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
	"time"
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

func TestPanelUpdateArchivesHistory(t *testing.T) {
	h, host, state := newUpdateRouter(t, "1.6.2", true)
	for i := 0; i < panelUpdateHistoryMax+2; i++ {
		if code, out := doJSON(t, h, "POST", "/tasks/panel/update", `{"tag":"v1.7.0"}`); code != http.StatusAccepted {
			t.Fatalf("start %d = %d %v", i, code, out)
		}
		// Give every run a distinct start second, then let it fail.
		raw, _ := os.ReadFile(filepath.Join(state, "meta.json"))
		var m panelUpdateMeta
		json.Unmarshal(raw, &m)
		m.StartedAt = m.StartedAt.Add(time.Duration(i) * time.Minute)
		raw, _ = json.Marshal(m)
		os.WriteFile(filepath.Join(state, "meta.json"), raw, 0o600)
		os.WriteFile(filepath.Join(state, "update.log"), []byte("run log\n"), 0o600)
		os.WriteFile(filepath.Join(state, "exit"), []byte("1\n"), 0o600)
		host.active = false
	}
	_, out := doJSON(t, h, "GET", "/tasks/panel/update", "")
	hist, _ := out["history"].([]any)
	if len(hist) != panelUpdateHistoryMax {
		t.Fatalf("history has %d runs, want %d", len(hist), panelUpdateHistoryMax)
	}
	first := hist[0].(map[string]any)
	if first["state"] != "failed" || first["tag"] != "v1.7.0" {
		t.Fatalf("unexpected newest history entry: %v", first)
	}
	logs, _ := filepath.Glob(filepath.Join(state, "history", "*.log"))
	if len(logs) != panelUpdateHistoryMax {
		t.Fatalf("kept %d logs, want %d", len(logs), panelUpdateHistoryMax)
	}
}

func TestPanelUpdatePreflightRefusesLowDisk(t *testing.T) {
	dir := t.TempDir()
	env := filepath.Join(dir, "panel.env")
	os.WriteFile(env, []byte("X=1\n"), 0o600)
	host := &fakeUpdateHost{}
	e := NewRouterWithOptions(RouterOptions{
		PanelUpdateRun: host.run, PanelUpdateStateDir: filepath.Join(dir, "state"),
		PanelUpdateEnvFile: env, PanelUpdateVersion: "1.6.2",
		PanelUpdateFreeBytes: func(string) (uint64, error) { return 10 << 20, nil },
	})
	code, out := doJSON(t, e, "POST", "/tasks/panel/update", `{"tag":"v1.7.0"}`)
	if code != http.StatusConflict || !strings.Contains(out["message"].(string), "preflight") {
		t.Fatalf("start with a full disk = %d %v", code, out)
	}
	for _, c := range host.calls {
		if c[0] == "systemd-run" {
			t.Fatal("systemd-run ran despite a failed preflight")
		}
	}
	_, st := doJSON(t, e, "GET", "/tasks/panel/update", "")
	checks, _ := st["preflight"].([]any)
	failed := 0
	for _, c := range checks {
		if c.(map[string]any)["ok"] == false {
			failed++
		}
	}
	if failed == 0 {
		t.Fatalf("preflight did not report the low disk: %v", st["preflight"])
	}
}

func TestReadLogTailStartsAtWholeLine(t *testing.T) {
	p := filepath.Join(t.TempDir(), "log")
	os.WriteFile(p, []byte("first line\nsecond \x1b[0;32mline\x1b[0m\nthird line\n"), 0o600)
	got := readLogTail(p, 25)
	if strings.Contains(got, "first") || !strings.Contains(got, "third line") || strings.Contains(got, "\x1b") {
		t.Fatalf("readLogTail = %q", got)
	}
	if !strings.HasPrefix(got, "… (earlier lines omitted)") {
		t.Fatalf("truncation is not marked: %q", got)
	}
}
