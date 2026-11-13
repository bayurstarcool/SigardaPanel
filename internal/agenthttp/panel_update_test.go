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

func (f *fakeUpdateHost) lastRun() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := len(f.calls) - 1; i >= 0; i-- {
		if f.calls[i][0] == "systemd-run" {
			return f.calls[i]
		}
	}
	return []string{""}
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

func mkPanelBackup(t *testing.T, dir, name, version string, withDB bool) {
	t.Helper()
	d := filepath.Join(dir, name)
	os.MkdirAll(filepath.Join(d, "web"), 0o700)
	os.WriteFile(filepath.Join(d, "sigardapanel.bin"), []byte("bin"), 0o700)
	os.WriteFile(filepath.Join(d, "web", "index.js"), []byte("js"), 0o600)
	os.WriteFile(filepath.Join(d, "VERSION"), []byte(version+"\n"), 0o600)
	os.WriteFile(filepath.Join(d, "KIND"), []byte("update\n"), 0o600)
	if withDB {
		os.WriteFile(filepath.Join(d, "sigardapanel.db"), []byte("db"), 0o600)
	}
}

func TestPanelRollbackAndHold(t *testing.T) {
	dir := t.TempDir()
	env := filepath.Join(dir, "panel.env")
	os.WriteFile(env, []byte("X=1\n"), 0o600)
	backups := filepath.Join(dir, "backups")
	mkPanelBackup(t, backups, "20261101T020000Z", "1.6.0", true)
	mkPanelBackup(t, backups, "20261105T020000Z", "1.6.1", false)
	os.MkdirAll(filepath.Join(backups, "20261106T020000Z"), 0o700) // incomplete: not listed
	os.MkdirAll(filepath.Join(backups, "not-a-backup"), 0o700)
	host := &fakeUpdateHost{}
	state := filepath.Join(dir, "state")
	h := NewRouterWithOptions(RouterOptions{
		PanelUpdateRun: host.run, PanelUpdateStateDir: state, PanelUpdateEnvFile: env,
		PanelUpdateVersion: "1.7.0", PanelUpdateBackupDir: backups,
	})

	_, out := doJSON(t, h, "GET", "/tasks/panel/backups", "")
	list, _ := out["backups"].([]any)
	if len(list) != 2 || list[0].(map[string]any)["name"] != "20261105T020000Z" || list[1].(map[string]any)["has_database"] != true {
		t.Fatalf("backups = %v", out)
	}

	for _, bad := range []string{`{"backup":"../../etc"}`, `{"backup":"20261106T020000Z"}`} {
		if code, _ := doJSON(t, h, "POST", "/tasks/panel/rollback", bad); code == http.StatusAccepted {
			t.Fatalf("rollback %s accepted", bad)
		}
	}
	code, out := doJSON(t, h, "POST", "/tasks/panel/rollback", `{"backup":"20261101T020000Z"}`)
	if code != http.StatusAccepted {
		t.Fatalf("rollback = %d %v", code, out)
	}
	last := host.lastRun()
	joined := strings.Join(last, " ")
	if last[0] != "systemd-run" || !strings.HasSuffix(joined, " rollback 20261101T020000Z") || !strings.Contains(joined, "--unit=sigardapanel-update") {
		t.Fatalf("unexpected rollback command: %v", last)
	}
	st := out["status"].(map[string]any)
	if st["action"] != "rollback" || st["tag"] != "v1.6.0" || st["hold_tag"] != "v1.7.0" {
		t.Fatalf("rollback status = %v", st)
	}

	// Finish the rollback; the agent (now 1.6.0 in reality) keeps the hold.
	os.WriteFile(filepath.Join(state, "exit"), []byte("0\n"), 0o600)
	host.active = false
	if _, out := doJSON(t, h, "GET", "/tasks/panel/update", ""); out["message"] != "Rolled back to v1.6.0" {
		t.Fatalf("finished rollback = %v", out)
	}
	h2 := NewRouterWithOptions(RouterOptions{
		PanelUpdateRun: host.run, PanelUpdateStateDir: state, PanelUpdateEnvFile: env,
		PanelUpdateVersion: "1.6.0", PanelUpdateBackupDir: backups,
	})
	if code, out := doJSON(t, h2, "POST", "/tasks/panel/update", `{"tag":"v1.7.0","auto":true}`); code != http.StatusConflict || !strings.Contains(out["message"].(string), "rolled back from v1.7.0") {
		t.Fatalf("auto update to the held version = %d %v", code, out)
	}
	if code, out := doJSON(t, h2, "POST", "/tasks/panel/update", `{"tag":"v1.7.1","auto":true}`); code != http.StatusAccepted {
		t.Fatalf("auto update to a newer release = %d %v", code, out)
	}
	if _, err := os.Stat(filepath.Join(state, "hold")); err != nil {
		t.Fatal("an automatic update must not clear the hold")
	}
	os.WriteFile(filepath.Join(state, "exit"), []byte("1\n"), 0o600)
	host.active = false
	if code, out := doJSON(t, h2, "POST", "/tasks/panel/update", `{"tag":"v1.7.0"}`); code != http.StatusAccepted {
		t.Fatalf("manual update = %d %v", code, out)
	}
	if _, err := os.Stat(filepath.Join(state, "hold")); err == nil {
		t.Fatal("a manual update must clear the hold")
	}
}

func TestAgentUpdateOnRemoteServer(t *testing.T) {
	dir := t.TempDir()
	agentEnv := filepath.Join(dir, "agent.env")
	os.WriteFile(agentEnv, []byte("SIGARDAPANEL_AGENT_ADDR=:7710\n"), 0o600)
	host := &fakeUpdateHost{}
	h := NewRouterWithOptions(RouterOptions{
		PanelUpdateRun: host.run, PanelUpdateStateDir: filepath.Join(dir, "state"),
		PanelUpdateEnvFile: filepath.Join(dir, "no-panel.env"), AgentUpdateEnvFile: agentEnv, PanelUpdateVersion: "1.6.2",
	})
	if _, out := doJSON(t, h, "GET", "/tasks/agent/update", ""); out["supported"] != true || out["kind"] != "agent" {
		t.Fatalf("agent status = %v", out)
	}
	// The panel updater is not available on a remote server.
	if code, _ := doJSON(t, h, "POST", "/tasks/panel/update", `{"tag":"v1.7.0"}`); code != http.StatusConflict {
		t.Fatalf("panel update on a remote server = %d", code)
	}
	code, out := doJSON(t, h, "POST", "/tasks/agent/update", `{"tag":"v1.7.0"}`)
	if code != http.StatusAccepted {
		t.Fatalf("agent update = %d %v", code, out)
	}
	last := strings.Join(host.lastRun(), " ")
	if !strings.Contains(last, "--unit=sigardapanel-agent-update") || !strings.HasSuffix(last, "agent-update.sh "+filepath.Join(dir, "state-agent", "update.log")+" "+filepath.Join(dir, "state-agent", "exit")+" v1.7.0") {
		t.Fatalf("unexpected agent update command: %s", last)
	}
	script, _ := os.ReadFile(filepath.Join(dir, "state-agent", "agent-update.sh"))
	if !strings.Contains(string(script), "SIGNATURE VERIFICATION FAILED") {
		t.Fatal("embedded agent updater not written")
	}
	if code, _ := doJSON(t, h, "POST", "/tasks/agent/update", `{"tag":"v1.7.0"}`); code != http.StatusConflict {
		t.Fatalf("second agent update = %d", code)
	}
}

func TestAgentUpdateRefusedOnPanelHost(t *testing.T) {
	h, host, _ := newUpdateRouter(t, "1.6.2", true)
	code, out := doJSON(t, h, "POST", "/tasks/agent/update", `{"tag":"v1.7.0"}`)
	if code != http.StatusConflict || !strings.Contains(out["message"].(string), "panel host") {
		t.Fatalf("agent update on the panel host = %d %v", code, out)
	}
	for _, c := range host.calls {
		if c[0] == "systemd-run" {
			t.Fatal("systemd-run ran")
		}
	}
}
