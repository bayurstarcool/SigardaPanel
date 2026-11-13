package agenthttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/labstack/echo/v5"
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
	agentUpdateUnit       = "sigardapanel-agent-update"
	defaultPanelUpdateDir = "/var/lib/sigardapanel-update"
	defaultAgentUpdateDir = "/var/lib/sigardapanel-agent-update"
	defaultPanelEnvFile   = "/etc/sigardapanel/panel.env"
	defaultAgentEnvFile   = "/etc/sigardapanel/agent.env"
	defaultPanelBackupDir = "/var/backups/sigardapanel-updates"
	defaultAgentBackupDir = "/var/backups/sigardapanel-agent-updates"
	panelUpdateMaxRuntime = 45 * time.Minute
	panelUpdateHistoryMax = 10
	// Free space the installer needs: the backup holds the binary, the web
	// bundle and a copy of the database; /tmp holds the download.
	panelUpdateMinFreeBackup = 512 << 20
	panelUpdateMinFreeTmp    = 300 << 20
	panelUpdateMinFreeBin    = 200 << 20
)

// panelUpdateRunFunc runs a command and returns its combined output.
type panelUpdateRunFunc func(ctx context.Context, name string, args ...string) ([]byte, error)

// panelUpdater runs one kind of self-update in the background:
//   - kind "panel": the panel host (install.sh update / rollback), and
//   - kind "agent": a remote server's agent (agent-update.sh).
type panelUpdater struct {
	kind         string
	unit         string
	script       []byte
	scriptName   string
	agentEnvFile string // kind agent: written by the agent install command
	// free space the preflight requires for backup, download and binary
	minBackup, minTmp, minBin uint64

	mu       sync.Mutex
	stateDir string
	envFile  string // present only on a panel host installed by install.sh
	version  string
	run      panelUpdateRunFunc
	euid     func() int
	lookPath func(string) (string, error)
	// freeBytes reports the free space of the file system holding path.
	freeBytes func(path string) (uint64, error)
	// paths checked for free space
	backupDir, tmpDir, binDir, dbPath string
}

func hostFreeBytes(path string) (uint64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, err
	}
	return st.Bavail * uint64(st.Bsize), nil
}

func hostPanelUpdateRun(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

func denyPanelUpdateRun(context.Context, string, ...string) ([]byte, error) {
	return nil, errors.New("panel update commands are disabled on injected routers")
}

type panelUpdateMeta struct {
	// Action is "update" (default) or "rollback".
	Action      string    `json:"action,omitempty"`
	Tag         string    `json:"tag"`
	Backup      string    `json:"backup,omitempty"`
	FromVersion string    `json:"from_version"`
	StartedAt   time.Time `json:"started_at"`
}

// PanelUpdateStatus is what GET /tasks/panel/update answers.
type PanelUpdateStatus struct {
	// State: idle (never ran), running, succeeded, failed.
	State        string     `json:"state"`
	Kind         string     `json:"kind"`
	Action       string     `json:"action,omitempty"`
	Backup       string     `json:"backup,omitempty"`
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
	// Preflight lists the checks an update must pass before it starts.
	Preflight []PanelUpdateCheck `json:"preflight,omitempty"`
	// History holds earlier runs, newest first (without their logs).
	History []PanelUpdateRun `json:"history"`
	// HoldTag: after a rollback from this version, automatic updates skip
	// it (and older releases) until a manual update clears the hold.
	HoldTag string `json:"hold_tag,omitempty"`
}

// PanelUpdateCheck is one preflight check.
type PanelUpdateCheck struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

// PanelUpdateRun is a finished update kept in the history.
type PanelUpdateRun struct {
	Action      string     `json:"action,omitempty"`
	Backup      string     `json:"backup,omitempty"`
	Tag         string     `json:"tag"`
	FromVersion string     `json:"from_version"`
	State       string     `json:"state"`
	StartedAt   time.Time  `json:"started_at"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"`
	ExitCode    *int       `json:"exit_code,omitempty"`
	Message     string     `json:"message,omitempty"`
}

func (u *panelUpdater) path(name string) string { return filepath.Join(u.stateDir, name) }

// unsupported explains why this agent cannot run a panel update ("" = it can).
func (u *panelUpdater) unsupported() string {
	if !selfupdate.IsRelease(u.version) {
		return "This is a development build (version " + u.version + "); only release installs can update themselves."
	}
	_, panelErr := os.Stat(u.envFile)
	if u.kind == "agent" {
		if panelErr == nil {
			return "This is the panel host: its agent is updated together with the panel (Settings → Panel update)."
		}
		if _, err := os.Stat(u.agentEnvFile); err != nil {
			return "This server was not set up with the agent install command (" + u.agentEnvFile + " not found); reinstall the agent to enable updates from the panel."
		}
	} else if panelErr != nil {
		return "This server is not a panel host installed with install.sh (" + u.envFile + " not found)."
	}
	if u.euid() != 0 {
		return "The agent is not running as root."
	}
	return ""
}

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)

func readLogTail(path string, max int64) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	truncated := false
	if st, err := f.Stat(); err == nil && st.Size() > max {
		_, _ = f.Seek(st.Size()-max, 0)
		truncated = true
	}
	b := make([]byte, max)
	n, _ := io.ReadFull(f, b)
	out := string(b[:n])
	if truncated {
		// Start at a whole line (and never in the middle of a UTF-8 rune).
		if i := strings.IndexByte(out, '\n'); i >= 0 {
			out = out[i+1:]
		}
		out = "… (earlier lines omitted)\n" + out
	}
	out = ansiRe.ReplaceAllString(out, "")
	return strings.ToValidUTF8(out, "")
}

func (u *panelUpdater) logTail(max int64) string { return readLogTail(u.path("update.log"), max) }

func (u *panelUpdater) unitActive(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := u.run(ctx, "systemctl", "is-active", u.unit+".service")
	s := strings.TrimSpace(string(out))
	return err == nil && (s == "active" || s == "activating" || s == "reloading")
}

func (u *panelUpdater) status(ctx context.Context) PanelUpdateStatus {
	st := u.current(ctx)
	if reason := u.unsupported(); reason != "" {
		st.Unsupported = reason
	} else {
		st.Supported = true
		st.Preflight = u.preflight()
	}
	st.History = u.history()
	return st
}

// current describes the latest run (state idle when there never was one).
func (u *panelUpdater) current(ctx context.Context) PanelUpdateStatus {
	st := PanelUpdateStatus{State: "idle", Kind: u.kind, AgentVersion: u.version, History: []PanelUpdateRun{}}
	if raw, err := os.ReadFile(u.path("hold")); err == nil {
		st.HoldTag = strings.TrimSpace(string(raw))
	}
	raw, err := os.ReadFile(u.path("meta.json"))
	if err != nil {
		return st
	}
	var meta panelUpdateMeta
	if json.Unmarshal(raw, &meta) != nil {
		return st
	}
	if meta.Action == "" {
		meta.Action = "update"
	}
	st.Action, st.Backup, st.Tag, st.FromVersion = meta.Action, meta.Backup, meta.Tag, meta.FromVersion
	what := "update to " + meta.Tag
	if meta.Action == "rollback" {
		what = "rollback to " + meta.Tag + " (backup " + meta.Backup + ")"
	}
	if u.kind == "agent" {
		what = "agent " + what
	}
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
			switch {
			case meta.Action == "rollback":
				st.Message = "Rolled back to " + meta.Tag
			case u.kind == "agent":
				st.Message = "Agent updated to " + meta.Tag
			default:
				st.Message = "Updated to " + meta.Tag
			}
		} else {
			st.State = "failed"
			st.Message = "The " + what + " failed (exit code " + strconv.Itoa(code) + "); see the log. The previous version is restored automatically."
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
	st.Message = "The " + what + " stopped without reporting a result; see the log."
	return st
}

// start launches the update to tag in the background. auto marks a start
// by the panel's automatic updater, which respects a rollback hold.
func (u *panelUpdater) start(ctx context.Context, tag string, auto bool) (int, string) {
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
	if auto {
		if raw, err := os.ReadFile(u.path("hold")); err == nil {
			if hold := strings.TrimSpace(string(raw)); hold != "" && !selfupdate.Newer(tag, hold) {
				return http.StatusConflict, "automatic updates skip " + tag + ": the panel was rolled back from " + hold + " (a manual update clears this)"
			}
		}
	}
	args := []string{"update"}
	env := []string{"SIGARDA_VERSION=" + tag}
	desc := "SigardaPanel update to " + tag
	if u.kind == "agent" {
		args, env, desc = []string{tag}, nil, "SigardaPanel agent update to "+tag
	}
	code, msg := u.launch(ctx, panelUpdateMeta{Action: "update", Tag: tag, FromVersion: u.version}, desc, args, env)
	if code == http.StatusAccepted && !auto {
		_ = os.Remove(u.path("hold"))
	}
	return code, msg
}

// rollback launches "install.sh rollback <backup>" (panel only).
func (u *panelUpdater) rollback(ctx context.Context, backup string) (int, string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.kind != "panel" {
		return http.StatusBadRequest, "rollback is only available for the panel"
	}
	if reason := u.unsupported(); reason != "" {
		return http.StatusConflict, reason
	}
	if !panelBackupRe.MatchString(backup) {
		return http.StatusBadRequest, "invalid backup name"
	}
	var target *PanelBackup
	for _, b := range u.backups() {
		if b.Name == backup {
			b := b
			target = &b
		}
	}
	if target == nil {
		return http.StatusNotFound, "backup " + backup + " not found (or incomplete)"
	}
	tag := target.Version
	if selfupdate.IsRelease(tag) {
		tag = selfupdate.Tag(tag)
	}
	code, msg := u.launch(ctx, panelUpdateMeta{Action: "rollback", Tag: tag, Backup: backup, FromVersion: u.version},
		"SigardaPanel rollback to "+tag, []string{"rollback", backup}, nil)
	if code == http.StatusAccepted && selfupdate.IsRelease(u.version) {
		// Keep the automatic updater from installing this version again.
		_ = os.WriteFile(u.path("hold"), []byte(selfupdate.Tag(u.version)+"\n"), 0o600)
	}
	return code, msg
}

// launch runs the embedded script with args in the transient unit. The
// caller holds u.mu.
func (u *panelUpdater) launch(ctx context.Context, meta panelUpdateMeta, desc string, args, env []string) (int, string) {
	prev := u.current(ctx)
	if prev.State == "running" {
		return http.StatusConflict, "an " + prev.Action + " to " + prev.Tag + " is already running"
	}
	for _, c := range u.preflight() {
		if !c.OK {
			return http.StatusConflict, "preflight check failed: " + c.Name + ": " + c.Detail
		}
	}
	if err := os.MkdirAll(u.stateDir, 0o700); err != nil {
		return http.StatusInternalServerError, "cannot create " + u.stateDir + ": " + err.Error()
	}
	_ = os.Chmod(u.stateDir, 0o700)
	u.archive(prev)
	for _, f := range []string{"exit", "exit.tmp", "update.log", "meta.json"} {
		_ = os.Remove(u.path(f))
	}
	script := u.path(u.scriptName)
	if err := os.WriteFile(script+".tmp", u.script, 0o700); err != nil {
		return http.StatusInternalServerError, "cannot write the updater script: " + err.Error()
	}
	if err := os.Rename(script+".tmp", script); err != nil {
		return http.StatusInternalServerError, "cannot write the updater script: " + err.Error()
	}
	meta.StartedAt = time.Now().UTC()
	raw, _ := json.Marshal(meta)
	if err := os.WriteFile(u.path("meta.json"), raw, 0o600); err != nil {
		return http.StatusInternalServerError, "cannot write the update state: " + err.Error()
	}

	// The exit code is written through a temp file + rename so a reader never
	// sees a half-written value.
	const wrapper = `s="$1"; l="$2"; x="$3"; shift 3; bash "$s" "$@" >"$l" 2>&1; rc=$?; echo "$rc" >"$x.tmp" && mv -f "$x.tmp" "$x"`
	cmd := []string{
		"--unit=" + u.unit,
		"--collect",
		"--quiet",
		"--description=" + desc,
		"--setenv=DEBIAN_FRONTEND=noninteractive",
		"--property=RuntimeMaxSec=" + strconv.Itoa(int(panelUpdateMaxRuntime.Seconds())),
	}
	for _, e := range env {
		cmd = append(cmd, "--setenv="+e)
	}
	cmd = append(cmd, "/bin/bash", "-c", wrapper, u.unit, script, u.path("update.log"), u.path("exit"))
	cmd = append(cmd, args...)
	runCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	out, err := u.run(runCtx, "systemd-run", cmd...)
	if err != nil {
		_ = os.Remove(u.path("meta.json"))
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return http.StatusInternalServerError, "could not start: " + msg
	}
	return http.StatusAccepted, desc + " started"
}

// PanelBackup is a pre-update backup that "rollback" can restore.
type PanelBackup struct {
	Name        string    `json:"name"`
	CreatedAt   time.Time `json:"created_at"`
	Version     string    `json:"version"`
	Target      string    `json:"target,omitempty"`
	Kind        string    `json:"kind"`
	HasDatabase bool      `json:"has_database"`
	SizeBytes   int64     `json:"size_bytes"`
}

var panelBackupRe = regexp.MustCompile(`^20[0-9]{6}T[0-9]{6}Z$`)

func readSmall(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil || len(raw) > 256 {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

// backups lists the complete update backups, newest first (the same rules
// as install.sh rollback list).
func (u *panelUpdater) backups() []PanelBackup {
	entries, err := os.ReadDir(u.backupDir)
	if err != nil {
		return []PanelBackup{}
	}
	out := []PanelBackup{}
	for _, e := range entries {
		if !e.IsDir() || !panelBackupRe.MatchString(e.Name()) {
			continue
		}
		dir := filepath.Join(u.backupDir, e.Name())
		bin, err1 := os.Stat(filepath.Join(dir, "sigardapanel.bin"))
		_, err2 := os.Stat(filepath.Join(dir, "web", "index.js"))
		if err1 != nil || err2 != nil {
			continue
		}
		created, _ := time.Parse("20060102T150405Z", e.Name())
		b := PanelBackup{
			Name: e.Name(), CreatedAt: created, Version: readSmall(filepath.Join(dir, "VERSION")),
			Target: readSmall(filepath.Join(dir, "TARGET")), Kind: readSmall(filepath.Join(dir, "KIND")),
			SizeBytes: bin.Size(),
		}
		if b.Version == "" {
			b.Version = "unknown"
		}
		if b.Kind == "" {
			b.Kind = "update"
		}
		if db, err := os.Stat(filepath.Join(dir, "sigardapanel.db")); err == nil {
			b.HasDatabase = true
			b.SizeBytes += db.Size()
		}
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name > out[j].Name })
	return out
}

// archive moves a finished run (meta, exit code, log) into history/ and
// keeps the newest panelUpdateHistoryMax runs.
func (u *panelUpdater) archive(prev PanelUpdateStatus) {
	if prev.State == "idle" || prev.StartedAt == nil {
		return
	}
	dir := u.path("history")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	base := fmt.Sprintf("%d-%s", prev.StartedAt.Unix(), prev.Tag)
	run := PanelUpdateRun{
		Action: prev.Action, Backup: prev.Backup,
		Tag: prev.Tag, FromVersion: prev.FromVersion, State: prev.State, StartedAt: *prev.StartedAt,
		FinishedAt: prev.FinishedAt, ExitCode: prev.ExitCode, Message: prev.Message,
	}
	if b, err := json.Marshal(run); err == nil {
		_ = os.WriteFile(filepath.Join(dir, base+".json"), b, 0o600)
	}
	if log := readLogTail(u.path("update.log"), 256<<10); log != "" {
		_ = os.WriteFile(filepath.Join(dir, base+".log"), []byte(log), 0o600)
	}
	entries, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	sort.Strings(entries) // names start with the Unix start time
	for len(entries) > panelUpdateHistoryMax {
		old := strings.TrimSuffix(entries[0], ".json")
		_ = os.Remove(old + ".json")
		_ = os.Remove(old + ".log")
		entries = entries[1:]
	}
}

// history returns the archived runs, newest first.
func (u *panelUpdater) history() []PanelUpdateRun {
	entries, _ := filepath.Glob(filepath.Join(u.path("history"), "*.json"))
	runs := make([]PanelUpdateRun, 0, len(entries))
	for _, e := range entries {
		raw, err := os.ReadFile(e)
		if err != nil {
			continue
		}
		var r PanelUpdateRun
		if json.Unmarshal(raw, &r) == nil && r.Tag != "" {
			runs = append(runs, r)
		}
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i].StartedAt.After(runs[j].StartedAt) })
	return runs
}

// preflight checks what the installer needs before an update may start.
func (u *panelUpdater) preflight() []PanelUpdateCheck {
	var checks []PanelUpdateCheck
	if _, err := u.lookPath("systemd-run"); err != nil {
		checks = append(checks, PanelUpdateCheck{Name: "systemd", Detail: "systemd-run not found; the update needs systemd"})
	} else {
		checks = append(checks, PanelUpdateCheck{Name: "systemd", OK: true, Detail: "systemd-run available"})
	}
	var dbSize uint64
	if u.dbPath != "" {
		if fi, err := os.Stat(u.dbPath); err == nil {
			dbSize = uint64(fi.Size())
		}
	}
	space := func(name, path string, need uint64) {
		// Check the nearest existing directory (the backup dir may not exist yet).
		p := path
		for p != "/" && p != "." {
			if _, err := os.Stat(p); err == nil {
				break
			}
			p = filepath.Dir(p)
		}
		free, err := u.freeBytes(p)
		switch {
		case err != nil:
			checks = append(checks, PanelUpdateCheck{Name: name, Detail: "cannot read free space of " + path + ": " + err.Error()})
		case free < need:
			checks = append(checks, PanelUpdateCheck{Name: name, Detail: fmt.Sprintf("%s has %s free, needs %s", path, humanBytes(free), humanBytes(need))})
		default:
			checks = append(checks, PanelUpdateCheck{Name: name, OK: true, Detail: fmt.Sprintf("%s: %s free", path, humanBytes(free))})
		}
	}
	space("backup space", u.backupDir, u.minBackup+dbSize)
	space("download space", u.tmpDir, u.minTmp)
	space("install space", u.binDir, u.minBin)
	return checks
}

func humanBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

// panelDBPath reads SIGARDAPANEL_DB_PATH from panel.env (for the backup
// space estimate); "" when unknown.
func panelDBPath(envFile string) string {
	raw, err := os.ReadFile(envFile)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), "SIGARDAPANEL_DB_PATH="); ok {
			return strings.Trim(v, `"'`)
		}
	}
	return ""
}

func newPanelUpdater(opts *RouterOptions) *panelUpdater {
	return newUpdater("panel", opts)
}

func newAgentUpdater(opts *RouterOptions) *panelUpdater {
	return newUpdater("agent", opts)
}

func newUpdater(kind string, opts *RouterOptions) *panelUpdater {
	u := &panelUpdater{
		kind:         kind,
		unit:         panelUpdateUnit,
		script:       installer.Script,
		scriptName:   "install.sh",
		stateDir:     defaultPanelUpdateDir,
		envFile:      defaultPanelEnvFile,
		agentEnvFile: defaultAgentEnvFile,
		version:      buildinfo.Version,
		run:          hostPanelUpdateRun,
		euid:         os.Geteuid,
		lookPath:     exec.LookPath,
		freeBytes:    hostFreeBytes,
		backupDir:    defaultPanelBackupDir,
		tmpDir:       os.TempDir(),
		binDir:       "/usr/local/bin",
		minBackup:    panelUpdateMinFreeBackup,
		minTmp:       panelUpdateMinFreeTmp,
		minBin:       panelUpdateMinFreeBin,
	}
	if kind == "agent" {
		u.unit, u.script, u.scriptName = agentUpdateUnit, installer.AgentScript, "agent-update.sh"
		u.stateDir, u.backupDir = defaultAgentUpdateDir, defaultAgentBackupDir
		u.minBackup, u.minTmp, u.minBin = 150<<20, 150<<20, 150<<20
	}
	if opts != nil {
		// Injected routers (tests) never touch the real host.
		u.run = denyPanelUpdateRun
		u.stateDir = filepath.Join(os.TempDir(), "sigardapanel-agent-test-update-"+kind)
		u.envFile = filepath.Join(os.TempDir(), "sigardapanel-agent-test-update-panel.env-not-configured")
		u.agentEnvFile = filepath.Join(os.TempDir(), "sigardapanel-agent-test-update-agent.env-not-configured")
		if opts.PanelUpdateRun != nil {
			u.run = opts.PanelUpdateRun
		}
		if opts.PanelUpdateStateDir != "" {
			u.stateDir = opts.PanelUpdateStateDir
			if kind == "agent" {
				u.stateDir += "-agent"
			}
		}
		if opts.PanelUpdateEnvFile != "" {
			u.envFile = opts.PanelUpdateEnvFile
		}
		if opts.AgentUpdateEnvFile != "" {
			u.agentEnvFile = opts.AgentUpdateEnvFile
		}
		if opts.PanelUpdateVersion != "" {
			u.version = opts.PanelUpdateVersion
		}
		u.euid = func() int { return 0 }
		u.lookPath = func(string) (string, error) { return "/usr/bin/systemd-run", nil }
		u.freeBytes = func(string) (uint64, error) { return 100 << 30, nil }
		if opts.PanelUpdateFreeBytes != nil {
			u.freeBytes = opts.PanelUpdateFreeBytes
		}
		u.backupDir, u.tmpDir, u.binDir = u.stateDir, u.stateDir, u.stateDir
		if opts.PanelUpdateBackupDir != "" && kind == "panel" {
			u.backupDir = opts.PanelUpdateBackupDir
		}
	}
	if kind == "panel" {
		u.dbPath = panelDBPath(u.envFile)
	}
	return u
}

// panelUpdateStatusTask answers GET /tasks/panel/update.
func (r *Router) panelUpdateStatusTask(c *echo.Context) error {
	return c.JSON(http.StatusOK, r.panelUpdate.status(c.Request().Context()))
}

// panelUpdateStartTask answers POST /tasks/panel/update {"tag":"v1.2.3","auto":false}.
func (r *Router) panelUpdateStartTask(c *echo.Context) error {
	return startUpdateTask(c, r.panelUpdate)
}

// agentUpdateStatusTask answers GET /tasks/agent/update (remote servers).
func (r *Router) agentUpdateStatusTask(c *echo.Context) error {
	return c.JSON(http.StatusOK, r.agentUpdate.status(c.Request().Context()))
}

// agentUpdateStartTask answers POST /tasks/agent/update {"tag":"v1.2.3"}.
func (r *Router) agentUpdateStartTask(c *echo.Context) error {
	return startUpdateTask(c, r.agentUpdate)
}

func startUpdateTask(c *echo.Context, u *panelUpdater) error {
	var req struct {
		Tag  string `json:"tag"`
		Auto bool   `json:"auto"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"ok": false, "message": "invalid payload"})
	}
	code, msg := u.start(c.Request().Context(), strings.TrimSpace(req.Tag), req.Auto)
	if code != http.StatusAccepted {
		return c.JSON(code, map[string]any{"ok": false, "message": msg})
	}
	return c.JSON(code, map[string]any{"ok": true, "message": msg, "status": u.status(c.Request().Context())})
}

// panelBackupsTask answers GET /tasks/panel/backups.
func (r *Router) panelBackupsTask(c *echo.Context) error {
	return c.JSON(http.StatusOK, map[string]any{"backups": r.panelUpdate.backups()})
}

// panelRollbackTask answers POST /tasks/panel/rollback {"backup":"20261101T020000Z"}.
func (r *Router) panelRollbackTask(c *echo.Context) error {
	var req struct {
		Backup string `json:"backup"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"ok": false, "message": "invalid payload"})
	}
	code, msg := r.panelUpdate.rollback(c.Request().Context(), strings.TrimSpace(req.Backup))
	if code != http.StatusAccepted {
		return c.JSON(code, map[string]any{"ok": false, "message": msg})
	}
	return c.JSON(code, map[string]any{"ok": true, "message": msg, "status": r.panelUpdate.status(c.Request().Context())})
}
