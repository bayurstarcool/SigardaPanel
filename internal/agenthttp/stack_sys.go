package agenthttp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// stackSystem is everything stack management needs from the host. The
// production implementation (hostStackSystem) runs real commands; routers
// built with RouterOptions get denyStackSystem unless a test injects a fake,
// so tests can never run apt-get or systemctl on the machine running them.
type stackSystem interface {
	// Run runs name with args, streaming combined output to w (may be nil),
	// and returns that output. env entries are added to the process
	// environment.
	Run(ctx context.Context, w io.Writer, env []string, name string, args ...string) (string, error)
	LookPath(name string) (string, error)
	Glob(pattern string) ([]string, error)
	Exists(path string) bool
	EvalSymlinks(path string) (string, error)
	ReadFile(path string) ([]byte, error)
	WriteFile(path string, data []byte, mode os.FileMode) error
	Sleep(ctx context.Context, d time.Duration) error
}

// hostStackSystem is the real host.
type hostStackSystem struct{}

const stackOutputCap = 1 << 20 // keep at most 1 MiB of a command's output

func (hostStackSystem) Run(ctx context.Context, w io.Writer, env []string, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/local/go/bin")
	cmd.Env = append(cmd.Env, env...)
	cmd.Stdin = nil
	buf := &cappedBuffer{max: stackOutputCap}
	var out io.Writer = buf
	if w != nil {
		out = io.MultiWriter(buf, w)
	}
	cmd.Stdout = out
	cmd.Stderr = out
	cmd.WaitDelay = 5 * time.Second
	err := cmd.Run()
	if err != nil {
		if ctx.Err() != nil {
			return buf.String(), fmt.Errorf("%s: %w", name, ctx.Err())
		}
		return buf.String(), fmt.Errorf("%s failed: %w", name, err)
	}
	return buf.String(), nil
}

func (hostStackSystem) LookPath(name string) (string, error) {
	if p, err := exec.LookPath(name); err == nil {
		return p, nil
	}
	// The agent's own PATH (systemd) may lack sbin or /usr/local/go/bin.
	for _, dir := range []string{"/usr/local/sbin", "/usr/local/bin", "/usr/sbin", "/usr/bin", "/sbin", "/bin", "/usr/local/go/bin"} {
		p := filepath.Join(dir, name)
		if st, err := os.Stat(p); err == nil && !st.IsDir() && st.Mode()&0o111 != 0 {
			return p, nil
		}
	}
	return "", exec.ErrNotFound
}

func (hostStackSystem) Glob(pattern string) ([]string, error) { return filepath.Glob(pattern) }
func (hostStackSystem) Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
func (hostStackSystem) EvalSymlinks(path string) (string, error) { return filepath.EvalSymlinks(path) }
func (hostStackSystem) ReadFile(path string) ([]byte, error)     { return os.ReadFile(path) }
func (hostStackSystem) WriteFile(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, mode)
}
func (hostStackSystem) Sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// denyStackSystem is the default for injected (test) routers: nothing is
// installed and nothing can be run.
type denyStackSystem struct{}

var errStackSystemDisabled = errors.New("stack commands are disabled in this router (no StackSystem injected)")

func (denyStackSystem) Run(context.Context, io.Writer, []string, string, ...string) (string, error) {
	return "", errStackSystemDisabled
}
func (denyStackSystem) LookPath(string) (string, error)                  { return "", exec.ErrNotFound }
func (denyStackSystem) Glob(string) ([]string, error)                    { return nil, nil }
func (denyStackSystem) Exists(string) bool                               { return false }
func (denyStackSystem) EvalSymlinks(p string) (string, error)            { return "", os.ErrNotExist }
func (denyStackSystem) ReadFile(string) ([]byte, error)                  { return nil, os.ErrNotExist }
func (denyStackSystem) WriteFile(string, []byte, os.FileMode) error      { return errStackSystemDisabled }
func (denyStackSystem) Sleep(ctx context.Context, d time.Duration) error { return nil }

// cappedBuffer keeps the last max bytes written to it.
type cappedBuffer struct {
	mu  sync.Mutex
	max int
	buf []byte
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf = append(b.buf, p...)
	if b.max > 0 && len(b.buf) > b.max {
		b.buf = append([]byte(nil), b.buf[len(b.buf)-b.max:]...)
	}
	return len(p), nil
}

func (b *cappedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.buf)
}

// tail returns at most n bytes from the end of s, starting at a line boundary.
func tailText(s string, n int) string {
	if len(s) <= n {
		return s
	}
	s = s[len(s)-n:]
	if i := strings.IndexByte(s, '\n'); i >= 0 && i < len(s)-1 {
		s = s[i+1:]
	}
	return "…\n" + s
}

// ─── apt helpers ────────────────────────────────────────────────────────────

// aptEnv keeps apt/dpkg from ever prompting (conffile questions, needrestart,
// debconf dialogs) — the agent has no terminal.
var aptEnv = []string{
	"DEBIAN_FRONTEND=noninteractive",
	"NEEDRESTART_MODE=a",
	"NEEDRESTART_SUSPEND=1",
	"APT_LISTCHANGES_FRONTEND=none",
	"UCF_FORCE_CONFFOLD=1",
	"LC_ALL=C.UTF-8",
	"LANG=C.UTF-8",
}

// aptOpts: wait up to 10 minutes for the dpkg lock (unattended-upgrades) and
// keep locally modified config files on upgrades.
var aptOpts = []string{
	"-y", "-q",
	"-o", "DPkg::Lock::Timeout=600",
	"-o", "Dpkg::Options::=--force-confdef",
	"-o", "Dpkg::Options::=--force-confold",
}

var aptLockRe = regexp.MustCompile(`(?i)could not get lock|unable to acquire the dpkg frontend lock|unable to lock directory|is another process using it`)

// apt runs apt-get with the non-interactive options, retrying while another
// package manager holds the lock and repairing an interrupted dpkg once.
func (m *stackManager) apt(ctx context.Context, w io.Writer, args ...string) (string, error) {
	full := append(append([]string{}, aptOpts...), args...)
	repaired := false
	var out string
	var err error
	for attempt := 0; attempt < 6; attempt++ {
		out, err = m.sys.Run(ctx, w, aptEnv, "apt-get", full...)
		if err == nil {
			return out, nil
		}
		if ctx.Err() != nil {
			return out, err
		}
		switch {
		case aptLockRe.MatchString(out):
			fmt.Fprintf(logWriter(w), "\n[sigardapanel] package manager is busy (lock held), retrying in 15s…\n")
			if serr := m.sys.Sleep(ctx, 15*time.Second); serr != nil {
				return out, err
			}
		case !repaired && strings.Contains(out, "dpkg --configure -a"):
			repaired = true
			fmt.Fprintf(logWriter(w), "\n[sigardapanel] dpkg was interrupted, running dpkg --configure -a…\n")
			m.sys.Run(ctx, w, aptEnv, "dpkg", "--configure", "-a", "--force-confdef", "--force-confold")
		default:
			return out, err
		}
	}
	return out, err
}

func logWriter(w io.Writer) io.Writer {
	if w == nil {
		return io.Discard
	}
	return w
}

func (m *stackManager) aptUpdate(ctx context.Context, w io.Writer) error {
	_, err := m.apt(ctx, w, "update")
	return err
}

func (m *stackManager) aptInstall(ctx context.Context, w io.Writer, pkgs ...string) error {
	if len(pkgs) == 0 {
		return nil
	}
	_, err := m.apt(ctx, w, append([]string{"install", "--no-install-recommends"}, pkgs...)...)
	return err
}

// aptUpgrade upgrades only the given (installed) packages.
func (m *stackManager) aptUpgrade(ctx context.Context, w io.Writer, pkgs ...string) error {
	if len(pkgs) == 0 {
		return fmt.Errorf("nothing to update: none of the packages is installed")
	}
	_, err := m.apt(ctx, w, append([]string{"install", "--only-upgrade"}, pkgs...)...)
	return err
}

// aptCandidates reports which packages have an installation candidate in the
// configured repositories.
func (m *stackManager) aptCandidates(ctx context.Context, pkgs ...string) map[string]bool {
	res := map[string]bool{}
	if len(pkgs) == 0 {
		return res
	}
	out, _ := m.sys.Run(ctx, nil, aptEnv, "apt-cache", append([]string{"policy"}, pkgs...)...)
	return parseAptPolicy(out)
}

// parseAptPolicy parses `apt-cache policy a b c` into package -> has candidate.
func parseAptPolicy(out string) map[string]bool {
	res := map[string]bool{}
	cur := ""
	for _, line := range strings.Split(out, "\n") {
		if line != "" && !strings.HasPrefix(line, " ") && strings.HasSuffix(strings.TrimSpace(line), ":") {
			cur = strings.TrimSuffix(strings.TrimSpace(line), ":")
			continue
		}
		t := strings.TrimSpace(line)
		if cur != "" && strings.HasPrefix(t, "Candidate:") {
			c := strings.TrimSpace(strings.TrimPrefix(t, "Candidate:"))
			res[cur] = c != "" && c != "(none)"
			cur = ""
		}
	}
	return res
}

// dpkgPackages lists packages matching the dpkg-query patterns that are
// installed (or, with withConfig, also those left with config files only).
func (m *stackManager) dpkgPackages(ctx context.Context, withConfig bool, patterns ...string) []string {
	args := append([]string{"-W", "-f=${db:Status-Abbrev}|${Package}\n"}, patterns...)
	out, _ := m.sys.Run(ctx, nil, aptEnv, "dpkg-query", args...)
	return parseDpkgQuery(out, withConfig)
}

func parseDpkgQuery(out string, withConfig bool) []string {
	seen := map[string]bool{}
	var pkgs []string
	for _, line := range strings.Split(out, "\n") {
		st, name, ok := strings.Cut(line, "|")
		name = strings.TrimSpace(name)
		if !ok || name == "" || len(st) < 2 {
			continue
		}
		state := st[1]
		installed := state != 'n' && state != 'c' && state != ' '
		if state == 'c' && withConfig {
			installed = true
		}
		if installed && !seen[name] {
			seen[name] = true
			pkgs = append(pkgs, name)
		}
	}
	sort.Strings(pkgs)
	return pkgs
}

// aptRemove removes (or purges) pkgs after simulating the removal: when apt
// would also take away packages that allowExtra does not accept (reverse
// dependencies such as phpmyadmin or ubuntu-minimal), nothing is removed and
// the error names them.
func (m *stackManager) aptRemove(ctx context.Context, w io.Writer, purge bool, pkgs []string, allowExtra func(string) bool) error {
	if len(pkgs) == 0 {
		return fmt.Errorf("nothing to remove: none of the packages is installed")
	}
	verb := "remove"
	if purge {
		verb = "purge"
	}
	sim, err := m.sys.Run(ctx, nil, aptEnv, "apt-get", append([]string{"-s", "-q", verb}, pkgs...)...)
	if err != nil {
		fmt.Fprintf(logWriter(w), "%s\n", sim)
		return fmt.Errorf("apt-get could not plan the removal: %s", lastLines(sim, 3))
	}
	want := map[string]bool{}
	for _, p := range pkgs {
		want[p] = true
	}
	var extra []string
	for _, p := range parseAptSimRemovals(sim) {
		if !want[p] && (allowExtra == nil || !allowExtra(p)) {
			extra = append(extra, p)
		}
	}
	if len(extra) > 0 {
		return fmt.Errorf("refusing to continue: removing %s would also remove other packages that depend on it: %s",
			strings.Join(pkgs, " "), strings.Join(extra, ", "))
	}
	fmt.Fprintf(logWriter(w), "[sigardapanel] apt-get %s %s\n", verb, strings.Join(pkgs, " "))
	_, err = m.apt(ctx, w, append([]string{verb}, pkgs...)...)
	return err
}

// parseAptSimRemovals extracts package names from `apt-get -s remove` lines
// such as "Remv php8.3-fpm [8.3.6-0ubuntu0.24.04.2]" or "Purg nginx [...]".
func parseAptSimRemovals(out string) []string {
	var pkgs []string
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 && (f[0] == "Remv" || f[0] == "Purg") {
			pkgs = append(pkgs, f[1])
		}
	}
	return pkgs
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, " / ")
}

// ─── OS / service helpers ───────────────────────────────────────────────────

type osRelease struct {
	ID         string
	IDLike     string
	Codename   string
	PrettyName string
	VersionID  string
}

func (m *stackManager) osRelease() osRelease {
	data, err := m.sys.ReadFile("/etc/os-release")
	if err != nil {
		return osRelease{}
	}
	return parseOSRelease(string(data))
}

func parseOSRelease(s string) osRelease {
	var o osRelease
	for _, line := range strings.Split(s, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		v = strings.Trim(v, `"'`)
		switch k {
		case "ID":
			o.ID = v
		case "ID_LIKE":
			o.IDLike = v
		case "VERSION_CODENAME":
			o.Codename = v
		case "PRETTY_NAME":
			o.PrettyName = v
		case "VERSION_ID":
			o.VersionID = v
		case "UBUNTU_CODENAME":
			if o.Codename == "" {
				o.Codename = v
			}
		}
	}
	return o
}

func (o osRelease) isUbuntu() bool { return o.ID == "ubuntu" }

// name is a human name for messages.
func (o osRelease) name() string {
	switch {
	case o.PrettyName != "":
		return o.PrettyName
	case o.ID != "":
		return strings.TrimSpace(o.ID + " " + o.VersionID)
	}
	return "this distribution"
}
func (o osRelease) isDebian() bool {
	return o.ID == "debian" || (!o.isUbuntu() && strings.Contains(o.IDLike, "debian"))
}

// unitStatus maps systemd state to running | starting | stopped | failed |
// missing (no such unit). Without systemctl the state is unknown: "installed".
func (m *stackManager) unitStatus(ctx context.Context, unit string) string {
	if _, err := m.sys.LookPath("systemctl"); err != nil {
		return "installed"
	}
	out, _ := m.sys.Run(ctx, nil, nil, "systemctl", "show", unit, "--property=LoadState,ActiveState", "--no-pager")
	return parseUnitState(out)
}

func parseUnitState(out string) string {
	load, active := "", ""
	for _, line := range strings.Split(out, "\n") {
		k, v, _ := strings.Cut(strings.TrimSpace(line), "=")
		switch k {
		case "LoadState":
			load = v
		case "ActiveState":
			active = v
		}
	}
	if load == "" && active == "" {
		return "installed"
	}
	if load == "not-found" {
		return "missing"
	}
	switch active {
	case "active", "reloading":
		return "running"
	case "activating":
		return "starting"
	case "failed":
		return "failed"
	default:
		return "stopped"
	}
}

// waitUnit polls until the unit reaches want ("running" or "stopped") or the
// timeout passes, and returns the last state seen.
func (m *stackManager) waitUnit(ctx context.Context, unit, want string, timeout time.Duration) string {
	// Counted in polls (not wall time) so the fake system in tests, whose
	// Sleep returns at once, does not spin.
	polls := int(timeout / time.Second)
	for i := 0; ; i++ {
		st := m.unitStatus(ctx, unit)
		ok := st == want || (want == "stopped" && (st == "stopped" || st == "failed" || st == "missing"))
		if ok || i >= polls || ctx.Err() != nil {
			return st
		}
		if m.sys.Sleep(ctx, time.Second) != nil {
			return st
		}
	}
}

// unitDiagnostics appends `systemctl status` and the journal tail for a unit
// that did not reach the expected state.
func (m *stackManager) unitDiagnostics(ctx context.Context, w io.Writer, unit string) string {
	out, _ := m.sys.Run(ctx, nil, nil, "systemctl", "status", unit, "--no-pager", "-l", "-n", "0")
	j, _ := m.sys.Run(ctx, nil, nil, "journalctl", "-u", unit, "-n", "25", "--no-pager", "-q")
	diag := strings.TrimSpace(out + "\n" + j)
	if w != nil && diag != "" {
		fmt.Fprintf(w, "\n[sigardapanel] %s diagnostics:\n%s\n", unit, diag)
	}
	return diag
}

var versionNumRe = regexp.MustCompile(`\d+\.\d+(?:\.\d+)?`)

// firstVersion returns the first dotted version number in s ("" if none).
func firstVersion(s string) string {
	if i := strings.Index(s, "Distrib "); i >= 0 {
		if v := versionNumRe.FindString(s[i:]); v != "" {
			return v
		}
	}
	return versionNumRe.FindString(s)
}

// compareVersions compares dotted numeric versions ("8.10" > "8.9").
func compareVersions(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) || i < len(bs); i++ {
		var x, y int
		if i < len(as) {
			x, _ = strconv.Atoi(as[i])
		}
		if i < len(bs) {
			y, _ = strconv.Atoi(bs[i])
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

// quick returns a context for detection commands.
func quick(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, 15*time.Second)
}
