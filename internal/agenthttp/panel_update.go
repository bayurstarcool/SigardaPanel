package agenthttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"sigardapanel/installer"
	"sigardapanel/internal/buildinfo"
	"sigardapanel/internal/selfupdate"
)

// Panel self-update.
//
// The update itself is installer/install.sh ("update"), embedded in this
// binary: it downloads the release, verifies the minisign signature and the
// checksums, backs up the binary, web bundle and database, swaps them in,
// restarts the services and rolls everything back when the panel is not
// healthy afterwards. The agent only launches it in its own transient systemd
// unit, so the script survives the agent, API and web restarts it performs.
// Progress lives in files under stateDir, which survive those restarts too.

const (
	panelUpdateUnit       = "sigardapanel-update"
	defaultPanelUpdateDir = "/var/lib/sigardapanel-update"
	defaultPanelEnvFile   = "/etc/sigardapanel/panel.env"
	panelUpdateMaxRuntime = 45 * time.Minute
)

// panelUpdateRunFunc runs a command and returns its combined output.
type panelUpdateRunFunc func(ctx context.Context, name string, args ...string) ([]byte, error)

type panelUpdater struct {
	mu       sync.Mutex
	stateDir string
	envFile  string // present only on a panel host installed by install.sh
	version  string
	run      panelUpdateRunFunc
	euid     func() int
}

func hostPanelUpdateRun(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

func denyPanelUpdateRun(context.Context, string, ...string) ([]byte, error) {
	return nil, errors.New("panel update commands are disabled on injected routers")
}

type panelUpdateMeta struct {
	Tag         string    `json:"tag"`
	FromVersion string    `json:"from_version"`
	StartedAt   time.Time `json:"started_at"`
}

// PanelUpdateStatus is what GET /tasks/panel/update answers.
type PanelUpdateStatus struct {
	// State: idle (never ran), running, succeeded, failed.
	State        string     `json:"state"`
	Tag          string     `json:"tag,omitempty"`
	FromVersion  string     `json:"from_version,omitempty"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
	ExitCode     *int       `json:"exit_code,omitempty"`
	Message      string     `json:"message,omitempty"`
	Log          string     `json:"log,omitempty"`
	AgentVersion string     `json:"agent_version"`
	// Supported is false when this agent cannot update the panel (not the
	// panel host, not installed with install.sh, a development build...).
	Supported   bool   `json:"supported"`
	Unsupported string `json:"unsupported_reason,omitempty"`
}

func (u *panelUpdater) path(name string) string { return filepath.Join(u.stateDir, name) }

// unsupported explains why this agent cannot run a panel update ("" = it can).
func (u *panelUpdater) unsupported() string {
	if !selfupdate.IsRelease(u.version) {
		return "This is a development build (version " + u.version + "); only release installs can update themselves."
	}
	if _, err := os.Stat(u.envFile); err != nil {
		return "This server is not a panel host installed with install.sh (" + u.envFile + " not found)."
	}
	if u.euid() != 0 {
		return "The agent is not running as root."
	}
	return ""
}

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)

func (u *panelUpdater) logTail(max int64) string {
	f, err := os.Open(u.path("update.log"))
	if err != nil {
		return ""
	}
	defer f.Close()
	if st, err := f.Stat(); err == nil && st.Size() > max {
		_, _ = f.Seek(st.Size()-max, 0)
	}
	b := make([]byte, max)
	n, _ := f.Read(b)
	return ansiRe.ReplaceAllString(string(b[:n]), "")
}

func (u *panelUpdater) unitActive(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := u.run(ctx, "systemctl", "is-active", panelUpdateUnit+".service")
	s := strings.TrimSpace(string(out))
	return err == nil && (s == "active" || s == "activating" || s == "reloading")
}

func (u *panelUpdater) status(ctx context.Context) PanelUpdateStatus {
	st := PanelUpdateStatus{State: "idle", AgentVersion: u.version}
	if reason := u.unsupported(); reason != "" {
		st.Unsupported = reason
	} else {
		st.Supported = true
	}
	raw, err := os.ReadFile(u.path("meta.json"))
	if err != nil {
		return st
	}
	var meta panelUpdateMeta
	if json.Unmarshal(raw, &meta) != nil {
		return st
	}
	st.Tag, st.FromVersion = meta.Tag, meta.FromVersion
	started := meta.StartedAt
	st.StartedAt = &started
	st.Log = u.logTail(64 << 10)

	if raw, err := os.ReadFile(u.path("exit")); err == nil {
		code, convErr := strconv.Atoi(strings.TrimSpace(string(raw)))
		if convErr != nil {
			code = -1
		}
		st.ExitCode = &code
		if fi, err := os.Stat(u.path("exit")); err == nil {
			t := fi.ModTime().UTC()
			st.FinishedAt = &t
		}
		if code == 0 {
			st.State = "succeeded"
			st.Message = "Updated to " + meta.Tag
		} else {
			st.State = "failed"
			st.Message = "The update to " + meta.Tag + " failed (exit code " + strconv.Itoa(code) + "); see the log. The installer restores the previous version automatically."
		}
		return st
	}
	if u.unitActive(ctx) {
		st.State = "running"
		return st
	}
	if time.Since(meta.StartedAt) < 15*time.Second {
		// systemd may not report the unit yet right after it was started.
		st.State = "running"
		return st
	}
	st.State = "failed"
	st.Message = "The update process stopped without reporting a result; see the log."
	return st
}

// start launches the update to tag in the background.
func (u *panelUpdater) start(ctx context.Context, tag string) (int, string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if reason := u.unsupported(); reason != "" {
		return http.StatusConflict, reason
	}
	if !selfupdate.ValidTag(tag) {
		return http.StatusBadRequest, "invalid release tag"
	}
	if !selfupdate.Newer(tag, u.version) {
		return http.StatusConflict, tag + " is not newer than the installed version " + u.version
	}
	if st := u.status(ctx); st.State == "running" {
		return http.StatusConflict, "an update to " + st.Tag + " is already running"
	}
	if err := os.MkdirAll(u.stateDir, 0o700); err != nil {
		return http.StatusInternalServerError, "cannot create " + u.stateDir + ": " + err.Error()
	}
	_ = os.Chmod(u.stateDir, 0o700)
	for _, f := range []string{"exit", "exit.tmp", "update.log"} {
		_ = os.Remove(u.path(f))
	}
	script := u.path("install.sh")
	if err := os.WriteFile(script+".tmp", installer.Script, 0o700); err != nil {
		return http.StatusInternalServerError, "cannot write the installer: " + err.Error()
	}
	if err := os.Rename(script+".tmp", script); err != nil {
		return http.StatusInternalServerError, "cannot write the installer: " + err.Error()
	}
	meta, _ := json.Marshal(panelUpdateMeta{Tag: tag, FromVersion: u.version, StartedAt: time.Now().UTC()})
	if err := os.WriteFile(u.path("meta.json"), meta, 0o600); err != nil {
		return http.StatusInternalServerError, "cannot write the update state: " + err.Error()
	}

	// The exit code is written through a temp file + rename so a reader never
	// sees a half-written value.
	const wrapper = `bash "$1" update >"$2" 2>&1; rc=$?; echo "$rc" >"$3.tmp" && mv -f "$3.tmp" "$3"`
	runCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	out, err := u.run(runCtx, "systemd-run",
		"--unit="+panelUpdateUnit,
		"--collect",
		"--quiet",
		"--description=SigardaPanel update to "+tag,
		"--setenv=SIGARDA_VERSION="+tag,
		"--setenv=DEBIAN_FRONTEND=noninteractive",
		"--property=RuntimeMaxSec="+strconv.Itoa(int(panelUpdateMaxRuntime.Seconds())),
		"/bin/bash", "-c", wrapper, panelUpdateUnit, script, u.path("update.log"), u.path("exit"),
	)
	if err != nil {
		_ = os.Remove(u.path("meta.json"))
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return http.StatusInternalServerError, "could not start the update: " + msg
	}
	return http.StatusAccepted, "update to " + tag + " started"
}

func newPanelUpdater(opts *RouterOptions) *panelUpdater {
	u := &panelUpdater{
		stateDir: defaultPanelUpdateDir,
		envFile:  defaultPanelEnvFile,
		version:  buildinfo.Version,
		run:      hostPanelUpdateRun,
		euid:     os.Geteuid,
	}
	if opts != nil {
		// Injected routers (tests) never touch the real host.
		u.run = denyPanelUpdateRun
		u.stateDir = filepath.Join(os.TempDir(), "sigardapanel-agent-test-update")
		u.envFile = filepath.Join(os.TempDir(), "sigardapanel-agent-test-update-panel.env-not-configured")
		if opts.PanelUpdateRun != nil {
			u.run = opts.PanelUpdateRun
		}
		if opts.PanelUpdateStateDir != "" {
			u.stateDir = opts.PanelUpdateStateDir
		}
		if opts.PanelUpdateEnvFile != "" {
			u.envFile = opts.PanelUpdateEnvFile
		}
		if opts.PanelUpdateVersion != "" {
			u.version = opts.PanelUpdateVersion
		}
		u.euid = func() int { return 0 }
	}
	return u
}

// panelUpdateStatusTask answers GET /tasks/panel/update.
func (r *Router) panelUpdateStatusTask(c echo.Context) error {
	return c.JSON(http.StatusOK, r.panelUpdate.status(c.Request().Context()))
}

// panelUpdateStartTask answers POST /tasks/panel/update {"tag":"v1.2.3"}.
func (r *Router) panelUpdateStartTask(c echo.Context) error {
	var req struct {
		Tag string `json:"tag"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"ok": false, "message": "invalid payload"})
	}
	code, msg := r.panelUpdate.start(c.Request().Context(), strings.TrimSpace(req.Tag))
	if code != http.StatusAccepted {
		return c.JSON(code, map[string]any{"ok": false, "message": msg})
	}
	return c.JSON(code, map[string]any{"ok": true, "message": msg, "status": r.panelUpdate.status(c.Request().Context())})
}
