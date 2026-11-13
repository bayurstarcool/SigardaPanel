package agenthttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/labstack/echo/v5"
)

// fakeStackSystem is an in-memory host: a virtual file system, a set of
// binaries on PATH and scripted command answers. Every command is recorded;
// nothing is ever executed.
type fakeStackSystem struct {
	mu    sync.Mutex
	files map[string]string // path -> content
	links map[string]string // symlink path -> target
	bins  map[string]bool
	cmds  []string
	envs  [][]string
	// answer returns output and error for a command line ("apt-get -y …").
	answer func(f *fakeStackSystem, line string) (string, error)
}

func newFakeStack() *fakeStackSystem {
	return &fakeStackSystem{files: map[string]string{}, links: map[string]string{}, bins: map[string]bool{"apt-get": true, "systemctl": true}}
}

func (f *fakeStackSystem) Run(ctx context.Context, w io.Writer, env []string, name string, args ...string) (string, error) {
	line := strings.TrimSpace(filepath.Base(name) + " " + strings.Join(args, " "))
	f.mu.Lock()
	f.cmds = append(f.cmds, line)
	f.envs = append(f.envs, env)
	answer := f.answer
	f.mu.Unlock()
	out, err := "", error(nil)
	if answer != nil {
		out, err = answer(f, line)
	}
	if w != nil && out != "" {
		io.WriteString(w, out)
	}
	return out, err
}

func (f *fakeStackSystem) LookPath(name string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.bins[name] {
		return "/usr/bin/" + name, nil
	}
	return "", errors.New("not found")
}

func (f *fakeStackSystem) Glob(pattern string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for p := range f.files {
		if ok, _ := filepath.Match(pattern, p); ok {
			out = append(out, p)
		}
	}
	return out, nil
}

func (f *fakeStackSystem) Exists(path string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.files[path]
	return ok
}

func (f *fakeStackSystem) EvalSymlinks(path string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if t, ok := f.links[path]; ok {
		return t, nil
	}
	return "", os.ErrNotExist
}

func (f *fakeStackSystem) ReadFile(path string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if c, ok := f.files[path]; ok {
		return []byte(c), nil
	}
	return nil, os.ErrNotExist
}

func (f *fakeStackSystem) WriteFile(path string, data []byte, mode os.FileMode) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.files[path] = string(data)
	return nil
}

func (f *fakeStackSystem) Sleep(ctx context.Context, d time.Duration) error { return nil }

func (f *fakeStackSystem) setFile(path, content string) {
	f.mu.Lock()
	f.files[path] = content
	f.mu.Unlock()
}

func (f *fakeStackSystem) commands() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.cmds...)
}

func (f *fakeStackSystem) ran(prefix string) bool {
	for _, c := range f.commands() {
		if strings.HasPrefix(c, prefix) {
			return true
		}
	}
	return false
}

func newStackRouter(t *testing.T, sys *fakeStackSystem) (*echo.Echo, string) {
	t.Helper()
	nginxDir := t.TempDir()
	e := NewRouterWithOptions(RouterOptions{
		NginxConfDir: nginxDir,
		PHPRunDir:    "/run/php",
		StackSystem:  sys,
		StackBinDir:  "/usr/bin",
		StackSbinDir: "/usr/sbin",
	})
	return e, nginxDir
}

func stackPost(t *testing.T, e *echo.Echo, body map[string]any) (int, map[string]any) {
	t.Helper()
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/tasks/stack/install", strings.NewReader(string(b)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-agent-token")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	var out map[string]any
	json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func stackGet(t *testing.T, e *echo.Echo, path string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Authorization", "Bearer test-agent-token")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	var out map[string]any
	json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

// waitOp polls the operation until it is done.
func waitOp(t *testing.T, e *echo.Echo, opID string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		code, out := stackGet(t, e, "/tasks/stack/ops/"+opID)
		if code != http.StatusOK {
			t.Fatalf("op status %d: %v", code, out)
		}
		if out["done"] == true {
			return out
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("operation did not finish")
	return nil
}

func startOp(t *testing.T, e *echo.Echo, body map[string]any) map[string]any {
	t.Helper()
	code, out := stackPost(t, e, body)
	if code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %v", code, out)
	}
	id, _ := out["op_id"].(string)
	if id == "" {
		t.Fatalf("no op_id: %v", out)
	}
	return waitOp(t, e, id)
}

// ─── parsers ────────────────────────────────────────────────────────────────

func TestParseAptPolicy(t *testing.T) {
	out := `php8.3-fpm:
  Installed: (none)
  Candidate: 8.3.6-0ubuntu0.24.04.2
  Version table:
php8.5-opcache:
  Installed: (none)
  Candidate: (none)
php8.2-fpm:
  Installed: 8.2.1
  Candidate: 8.2.2
`
	got := parseAptPolicy(out)
	if !got["php8.3-fpm"] || got["php8.5-opcache"] || !got["php8.2-fpm"] || got["missing"] {
		t.Fatalf("parseAptPolicy = %v", got)
	}
}

func TestParseDpkgQuery(t *testing.T) {
	out := "ii |php8.3-fpm\nrc |php8.3-old\nun |php8.3-xdebug\niU |php8.3-cli\n"
	if got := strings.Join(parseDpkgQuery(out, false), ","); got != "php8.3-cli,php8.3-fpm" {
		t.Fatalf("installed = %s", got)
	}
	if got := strings.Join(parseDpkgQuery(out, true), ","); got != "php8.3-cli,php8.3-fpm,php8.3-old" {
		t.Fatalf("with config = %s", got)
	}
}

func TestParseAptSimRemovals(t *testing.T) {
	out := "NOTE: This is only a simulation!\nRemv phpmyadmin [4:5.2.1]\nPurg php8.3-fpm [8.3.6]\nConf x\n"
	if got := strings.Join(parseAptSimRemovals(out), ","); got != "phpmyadmin,php8.3-fpm" {
		t.Fatalf("got %s", got)
	}
}

func TestParseUnitState(t *testing.T) {
	cases := map[string]string{
		"LoadState=loaded\nActiveState=active\n":     "running",
		"LoadState=loaded\nActiveState=inactive\n":   "stopped",
		"LoadState=loaded\nActiveState=failed\n":     "failed",
		"LoadState=loaded\nActiveState=activating\n": "starting",
		"LoadState=not-found\nActiveState=inactive":  "missing",
		"": "installed",
	}
	for in, want := range cases {
		if got := parseUnitState(in); got != want {
			t.Errorf("parseUnitState(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestFirstVersion(t *testing.T) {
	cases := map[string]string{
		"nginx version: nginx/1.24.0 (Ubuntu)":                            "1.24.0",
		"PHP 8.3.6 (cli) (built: Apr 15 2024)":                            "8.3.6",
		"v22.11.0":                                                        "22.11.0",
		"Docker version 27.3.1, build ce12230":                            "27.3.1",
		"mariadb  Ver 15.1 Distrib 10.11.8-MariaDB, for debian-linux-gnu": "10.11.8",
		"/usr/sbin/mariadbd  Ver 10.11.6-MariaDB-0+deb12u1 for debian":    "10.11.6",
		"Redis server v=7.0.15 sha=00000000:0 malloc=jemalloc-5.3.0":      "7.0.15",
		"go version go1.25.4 linux/amd64":                                 "1.25.4",
		"memcached 1.6.24":                                                "1.6.24",
		"4.2.5":                                                           "4.2.5",
	}
	for in, want := range cases {
		if got := firstVersion(in); got != want {
			t.Errorf("firstVersion(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseOSRelease(t *testing.T) {
	o := parseOSRelease("PRETTY_NAME=\"Debian GNU/Linux 12 (bookworm)\"\nID=debian\nVERSION_CODENAME=bookworm\n")
	if !o.isDebian() || o.isUbuntu() || o.Codename != "bookworm" {
		t.Fatalf("%+v", o)
	}
	u := parseOSRelease("ID=ubuntu\nID_LIKE=debian\nVERSION_CODENAME=noble\n")
	if !u.isUbuntu() || u.isDebian() {
		t.Fatalf("%+v", u)
	}
}

// ─── safety defaults ────────────────────────────────────────────────────────

// Injected routers without a StackSystem must never run anything.
func TestStackDefaultTestRouterRunsNothing(t *testing.T) {
	e := NewRouterWithOptions(RouterOptions{NginxConfDir: t.TempDir()})
	code, out := stackPost(t, e, map[string]any{"package": "nginx", "action": "install"})
	if code != http.StatusNotImplemented {
		t.Fatalf("expected 501 without apt-get, got %d: %v", code, out)
	}
	code, out = stackGet(t, e, "/tasks/stack/info")
	if code != http.StatusOK {
		t.Fatalf("info: %d", code)
	}
	data := out["data"].(map[string]any)
	for _, p := range data["packages"].([]any) {
		if st := p.(map[string]any)["status"]; st != "missing" {
			t.Fatalf("expected everything missing, got %v", p)
		}
	}
}

// ─── PHP ────────────────────────────────────────────────────────────────────

// phpHost simulates apt for PHP: versions in repo are installable; installing
// creates the binaries, socket and a running unit.
func phpHost(repo map[string]bool, osRelease string) *fakeStackSystem {
	f := newFakeStack()
	f.setFile("/etc/os-release", osRelease)
	running := map[string]bool{}
	installed := map[string]bool{}
	f.answer = func(f *fakeStackSystem, line string) (string, error) {
		switch {
		case strings.HasPrefix(line, "add-apt-repository"):
			for _, v := range []string{"8.2", "8.4"} {
				repo[v] = true
			}
			return "", nil
		case strings.HasPrefix(line, "apt-cache policy"):
			var b strings.Builder
			for _, p := range strings.Fields(strings.TrimPrefix(line, "apt-cache policy")) {
				cand := "(none)"
				if len(p) > 6 && repo[p[3:6]] && !strings.HasSuffix(p, "-opcache") {
					cand = "1.0"
				}
				fmt.Fprintf(&b, "%s:\n  Installed: (none)\n  Candidate: %s\n", p, cand)
			}
			return b.String(), nil
		case strings.HasPrefix(line, "apt-get") && strings.Contains(line, " install "):
			for _, p := range strings.Fields(line) {
				if strings.HasPrefix(p, "php") && strings.HasSuffix(p, "-fpm") {
					v := p[3:6]
					installed[v] = true
					f.files["/usr/bin/php"+v] = ""
					f.files["/usr/sbin/php-fpm"+v] = ""
				}
			}
			return "Setting up …\n", nil
		case strings.HasPrefix(line, "systemctl enable --now php"):
			v := line[len("systemctl enable --now php"):][:3]
			running[v] = true
			f.files["/run/php/php"+v+"-fpm.sock"] = ""
			return "", nil
		case strings.HasPrefix(line, "systemctl show php"):
			v := line[len("systemctl show php"):][:3]
			if !installed[v] {
				return "LoadState=not-found\nActiveState=inactive\n", nil
			}
			if running[v] {
				return "LoadState=loaded\nActiveState=active\n", nil
			}
			return "LoadState=loaded\nActiveState=inactive\n", nil
		case strings.HasPrefix(line, "php") && strings.Contains(line, "PHP_VERSION"):
			return line[3:6] + ".9", nil
		}
		return "", nil
	}
	return f
}

func TestStackPHPInstallUbuntuAddsPPAAndVerifies(t *testing.T) {
	sys := phpHost(map[string]bool{"8.3": true}, "ID=ubuntu\nVERSION_CODENAME=noble\nPRETTY_NAME=\"Ubuntu 24.04\"\n")
	e, _ := newStackRouter(t, sys)

	op := startOp(t, e, map[string]any{"package": "php", "action": "install", "version": "8.2"})
	if op["status"] != "succeeded" {
		t.Fatalf("install failed: %v\n%v", op["message"], op["output"])
	}
	if !sys.ran("add-apt-repository -y -n ppa:ondrej/php") {
		t.Fatalf("PPA not added: %v", sys.commands())
	}
	var install string
	for i, c := range sys.commands() {
		if strings.HasPrefix(c, "apt-get") && strings.Contains(c, "install --no-install-recommends php8.2-fpm") {
			install = c
			env := strings.Join(sys.envs[i], " ")
			if !strings.Contains(env, "DEBIAN_FRONTEND=noninteractive") {
				t.Errorf("apt without DEBIAN_FRONTEND: %s", env)
			}
		}
	}
	for _, want := range []string{"DPkg::Lock::Timeout=600", "Dpkg::Options::=--force-confold", "php8.2-cli", "php8.2-mysql"} {
		if !strings.Contains(install, want) {
			t.Errorf("install command lacks %q: %s", want, install)
		}
	}
	if strings.Contains(install, "php8.2-opcache") {
		t.Errorf("package without candidate must be skipped: %s", install)
	}
	if !strings.Contains(op["message"].(string), "skipped: php8.2-opcache") {
		t.Errorf("message should name skipped packages: %v", op["message"])
	}
	if !sys.ran("systemctl enable --now php8.2-fpm") {
		t.Error("php8.2-fpm not enabled")
	}
}

func TestStackPHPInstallDebianUsesSury(t *testing.T) {
	sys := phpHost(map[string]bool{"8.2": true}, "ID=debian\nVERSION_CODENAME=bookworm\n")
	e, _ := newStackRouter(t, sys)
	op := startOp(t, e, map[string]any{"package": "php", "action": "install", "version": "8.4"})
	// The fake repo never gains 8.4 for Debian, so the install must fail
	// truthfully after configuring sury.org.
	if op["status"] != "failed" || !strings.Contains(op["message"].(string), "not available") {
		t.Fatalf("expected a not-available failure, got %v: %v", op["status"], op["message"])
	}
	src := sys.files["/etc/apt/sources.list.d/sury-php.list"]
	if !strings.Contains(src, "https://packages.sury.org/php/ bookworm main") || !strings.Contains(src, "signed-by=") {
		t.Fatalf("sury source = %q", src)
	}
	if sys.ran("add-apt-repository") {
		t.Fatal("Debian must not use add-apt-repository")
	}
}

func TestStackPHPInstallFailsWhenFPMDoesNotStart(t *testing.T) {
	sys := phpHost(map[string]bool{"8.3": true}, "ID=ubuntu\n")
	base := sys.answer
	sys.answer = func(f *fakeStackSystem, line string) (string, error) {
		if strings.HasPrefix(line, "systemctl enable --now php8.3-fpm") {
			return "Job failed", errors.New("exit 1")
		}
		if strings.HasPrefix(line, "systemctl show php8.3-fpm") {
			return "LoadState=loaded\nActiveState=failed\n", nil
		}
		return base(f, line)
	}
	e, _ := newStackRouter(t, sys)
	op := startOp(t, e, map[string]any{"package": "php", "action": "install", "version": "8.3"})
	if op["status"] != "failed" || !strings.Contains(op["message"].(string), "php8.3-fpm is failed") {
		t.Fatalf("expected verified failure, got %v: %v", op["status"], op["message"])
	}
	if !sys.ran("journalctl -u php8.3-fpm") {
		t.Error("diagnostics (journal tail) not collected")
	}
}

func TestStackPHPUninstallRefusedWhileVhostUsesIt(t *testing.T) {
	sys := phpHost(map[string]bool{"8.2": true}, "ID=ubuntu\n")
	sys.setFile("/usr/bin/php8.2", "")
	e, nginxDir := newStackRouter(t, sys)
	sys.setFile(filepath.Join(nginxDir, "default"), "#\tfastcgi_pass unix:/run/php/php8.2-fpm.sock;\n")
	sys.setFile(filepath.Join(nginxDir, "shop.example.com.conf"), "server {\n  fastcgi_pass unix:/run/php/php8.2-fpm.sock; # live\n}\n")

	code, out := stackPost(t, e, map[string]any{"package": "php", "action": "uninstall", "version": "8.2"})
	if code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %v", code, out)
	}
	msg := out["message"].(string)
	if !strings.Contains(msg, "shop.example.com.conf") || strings.Contains(msg, "default") {
		t.Fatalf("message should name only the live vhost: %s", msg)
	}
	if sys.ran("apt-get") {
		t.Fatal("nothing may be removed")
	}
}

func TestStackPHPUninstallRefusesCollateralRemovals(t *testing.T) {
	sys := newFakeStack()
	sys.setFile("/usr/bin/php8.1", "")
	sys.answer = func(f *fakeStackSystem, line string) (string, error) {
		switch {
		case strings.HasPrefix(line, "dpkg-query"):
			return "ii |php8.1-fpm\nii |php8.1-cli\nii |php8.1-common\n", nil
		case strings.HasPrefix(line, "apt-get -s -q purge"):
			return "Purg php8.1-fpm [1]\nPurg php8.1-cli [1]\nPurg php8.1-common [1]\nPurg php-fpm [1]\nRemv phpmyadmin [1]\n", nil
		}
		return "", nil
	}
	e, _ := newStackRouter(t, sys)
	op := startOp(t, e, map[string]any{"package": "php", "action": "uninstall", "version": "8.1"})
	if op["status"] != "failed" || !strings.Contains(op["message"].(string), "phpmyadmin") || strings.Contains(op["message"].(string), "php-fpm,") {
		t.Fatalf("expected refusal naming phpmyadmin only, got %v", op["message"])
	}
	for _, c := range sys.commands() {
		if strings.HasPrefix(c, "apt-get -y") && strings.Contains(c, "purge") {
			t.Fatalf("purge must not run: %s", c)
		}
	}
	if !sys.ran("systemctl enable --now php8.1-fpm") {
		t.Error("FPM should be started again after the refused removal")
	}
}

func TestStackPHPVersionRequiredAndValidated(t *testing.T) {
	sys := newFakeStack()
	e, _ := newStackRouter(t, sys)
	for _, v := range []string{"", "8", "8.3;rm", "10.1"} {
		code, out := stackPost(t, e, map[string]any{"package": "php", "action": "install", "version": v})
		if code != http.StatusBadRequest {
			t.Errorf("version %q: expected 400, got %d %v", v, code, out)
		}
	}
	code, _ := stackPost(t, e, map[string]any{"package": "php", "action": "uninstall-extension", "version": "8.3", "extension": "fpm"})
	if code != http.StatusBadRequest {
		t.Errorf("removing the fpm 'extension' must be refused, got %d", code)
	}
	if sys.ran("apt-get") {
		t.Fatal("no apt command may run for invalid input")
	}
}

// ─── apt behaviour ──────────────────────────────────────────────────────────

func TestStackAptRetriesWhileLocked(t *testing.T) {
	sys := newFakeStack()
	sys.bins["redis-server"] = false
	attempts := 0
	sys.answer = func(f *fakeStackSystem, line string) (string, error) {
		switch {
		case strings.HasPrefix(line, "apt-get") && strings.HasSuffix(line, " update"):
			attempts++
			if attempts == 1 {
				return "E: Could not get lock /var/lib/apt/lists/lock. It is held by process 42 (apt-get)", errors.New("exit 100")
			}
			return "", nil
		case strings.HasPrefix(line, "apt-cache policy"):
			return "redis-server:\n  Candidate: 7.0\n", nil
		case strings.HasPrefix(line, "apt-get") && strings.Contains(line, "install"):
			f.mu.Lock()
			f.bins["redis-server"] = true
			f.mu.Unlock()
			return "", nil
		case strings.HasPrefix(line, "systemctl show redis-server"):
			return "LoadState=loaded\nActiveState=active\n", nil
		case strings.HasPrefix(line, "redis-server --version"):
			return "Redis server v=7.0.15", nil
		}
		return "", nil
	}
	e, _ := newStackRouter(t, sys)
	op := startOp(t, e, map[string]any{"package": "redis", "action": "install"})
	if op["status"] != "succeeded" || attempts != 2 {
		t.Fatalf("status %v after %d update attempts: %v", op["status"], attempts, op["message"])
	}
	if !strings.Contains(op["output"].(string), "retrying") {
		t.Error("log should mention the lock retry")
	}
}

func TestStackUninstallOnlyInstalledPackages(t *testing.T) {
	sys := newFakeStack()
	sys.bins["docker"] = true
	sys.answer = func(f *fakeStackSystem, line string) (string, error) {
		switch {
		case strings.HasPrefix(line, "docker ps -q"):
			return "", nil
		case strings.HasPrefix(line, "dpkg-query"):
			if strings.Contains(line, "docker.io") && !strings.Contains(line, "docker-ce") {
				return "", nil // after removal
			}
			return "ii |docker.io\nun |docker-ce\n", nil
		case strings.HasPrefix(line, "apt-get -s -q remove"):
			return "Remv docker.io [1]\n", nil
		case strings.HasPrefix(line, "apt-get -y") && strings.Contains(line, " remove "):
			f.mu.Lock()
			f.bins["docker"] = false
			f.mu.Unlock()
		}
		return "", nil
	}
	e, _ := newStackRouter(t, sys)
	op := startOp(t, e, map[string]any{"package": "docker", "action": "uninstall"})
	if op["status"] != "succeeded" {
		t.Fatalf("uninstall failed: %v\n%v", op["message"], sys.commands())
	}
	for _, c := range sys.commands() {
		if strings.HasPrefix(c, "apt-get -y") && strings.Contains(c, " remove ") && strings.Contains(c, "docker-ce") {
			t.Fatalf("must not name packages that are not installed: %s", c)
		}
	}
}

// Removing the meta packages but leaving the binary must not report success.
func TestStackUninstallVerifiesComponentIsGone(t *testing.T) {
	sys := newFakeStack()
	sys.bins["mariadbd"] = true
	sys.answer = func(f *fakeStackSystem, line string) (string, error) {
		switch {
		case strings.HasPrefix(line, "dpkg-query") && strings.Contains(line, "mariadb-server-core*"):
			return "ii |mariadb-server\n", nil
		case strings.HasPrefix(line, "dpkg-query"):
			return "", nil
		case strings.HasPrefix(line, "apt-get -s -q remove"):
			return "Remv mariadb-server [1]\n", nil
		}
		return "", nil
	}
	e, _ := newStackRouter(t, sys)
	op := startOp(t, e, map[string]any{"package": "mariadb", "action": "uninstall"})
	if op["status"] != "failed" || !strings.Contains(op["message"].(string), "still present") {
		t.Fatalf("got %v: %v", op["status"], op["message"])
	}
}

func TestStackDockerUninstallRefusedWithRunningContainers(t *testing.T) {
	sys := newFakeStack()
	sys.bins["docker"] = true
	sys.answer = func(f *fakeStackSystem, line string) (string, error) {
		if strings.HasPrefix(line, "docker ps -q") {
			return "abc123\ndef456\n", nil
		}
		return "", nil
	}
	e, _ := newStackRouter(t, sys)
	op := startOp(t, e, map[string]any{"package": "docker", "action": "uninstall"})
	if op["status"] != "failed" || !strings.Contains(op["message"].(string), "2 Docker container(s) are running") {
		t.Fatalf("got %v: %v", op["status"], op["message"])
	}
}

func TestStackOneOperationAtATime(t *testing.T) {
	sys := newFakeStack()
	release := make(chan struct{})
	sys.answer = func(f *fakeStackSystem, line string) (string, error) {
		if strings.HasSuffix(line, " update") {
			<-release
		}
		return "", nil
	}
	e, _ := newStackRouter(t, sys)
	code, first := stackPost(t, e, map[string]any{"package": "memcached", "action": "install"})
	if code != http.StatusAccepted {
		t.Fatalf("first: %d %v", code, first)
	}
	code, second := stackPost(t, e, map[string]any{"package": "redis", "action": "install"})
	if code != http.StatusConflict || !strings.Contains(second["message"].(string), "another stack operation is running") {
		t.Fatalf("second: %d %v", code, second)
	}
	_, info := stackGet(t, e, "/tasks/stack/info")
	if info["data"].(map[string]any)["operation"] == nil {
		t.Error("stack info should report the running operation")
	}
	close(release)
	waitOp(t, e, first["op_id"].(string))
}

// ─── services ───────────────────────────────────────────────────────────────

func TestStackNginxRestartRefusedOnBadConfig(t *testing.T) {
	sys := newFakeStack()
	sys.bins["nginx"] = true
	sys.answer = func(f *fakeStackSystem, line string) (string, error) {
		switch {
		case strings.HasPrefix(line, "systemctl show nginx"):
			return "LoadState=loaded\nActiveState=active\n", nil
		case line == "nginx -t":
			return "nginx: [emerg] unknown directive \"foo\" in /etc/nginx/sites-enabled/x:3\nnginx: configuration file /etc/nginx/nginx.conf test failed", errors.New("exit 1")
		}
		return "", nil
	}
	e, _ := newStackRouter(t, sys)
	code, out := stackPost(t, e, map[string]any{"package": "nginx", "action": "restart"})
	if code != http.StatusConflict || !strings.Contains(out["message"].(string), "unknown directive") {
		t.Fatalf("got %d %v", code, out)
	}
	if sys.ran("systemctl restart nginx") {
		t.Fatal("nginx must not be restarted with a broken config")
	}
}

func TestStackRestartVerifiesState(t *testing.T) {
	sys := newFakeStack()
	sys.bins["redis-server"] = true
	sys.answer = func(f *fakeStackSystem, line string) (string, error) {
		if strings.HasPrefix(line, "systemctl show redis-server") {
			return "LoadState=loaded\nActiveState=failed\n", nil
		}
		return "", nil
	}
	e, _ := newStackRouter(t, sys)
	code, out := stackPost(t, e, map[string]any{"package": "redis", "action": "restart"})
	if code != http.StatusOK || out["ok"] != false || !strings.Contains(out["message"].(string), "failed") {
		t.Fatalf("restart into a failed unit must report failure: %d %v", code, out)
	}
}

func TestStackPHPRestartAllVersions(t *testing.T) {
	sys := newFakeStack()
	sys.setFile("/usr/sbin/php-fpm8.3", "")
	sys.setFile("/usr/sbin/php-fpm8.2", "")
	sys.setFile("/usr/bin/php8.3", "")
	sys.answer = func(f *fakeStackSystem, line string) (string, error) {
		if strings.HasPrefix(line, "systemctl show php") {
			return "LoadState=loaded\nActiveState=active\n", nil
		}
		return "", nil
	}
	e, _ := newStackRouter(t, sys)
	code, out := stackPost(t, e, map[string]any{"package": "php", "action": "restart"})
	if code != http.StatusOK || out["ok"] != true {
		t.Fatalf("%d %v", code, out)
	}
	if !sys.ran("systemctl restart php8.3-fpm") || !sys.ran("systemctl restart php8.2-fpm") {
		t.Fatalf("both FPM services should restart: %v", sys.commands())
	}
}

func TestStackPHPVersionsDetection(t *testing.T) {
	sys := newFakeStack()
	sys.setFile("/usr/bin/php8.3", "")
	sys.setFile("/usr/bin/php8.10", "")
	sys.setFile("/usr/bin/php-config8.3", "")
	sys.setFile("/usr/sbin/php-fpm8.3", "")
	sys.setFile("/run/php/php8.3-fpm.sock", "")
	sys.links["/usr/bin/php"] = "/usr/bin/php8.3"
	sys.answer = func(f *fakeStackSystem, line string) (string, error) {
		if strings.HasPrefix(line, "systemctl show php8.3-fpm") {
			return "LoadState=loaded\nActiveState=active\n", nil
		}
		return "", nil
	}
	e, _ := newStackRouter(t, sys)
	_, out := stackPost(t, e, map[string]any{"package": "php", "action": "list-versions"})
	list := out["data"].(map[string]any)["versions"].([]any)
	if len(list) != 2 {
		t.Fatalf("versions = %v", list)
	}
	first := list[0].(map[string]any)
	if first["version"] != "8.10" {
		t.Errorf("versions must sort numerically, got %v first", first["version"])
	}
	v83 := list[1].(map[string]any)
	if v83["fpm"] != true || v83["socket"] != true || v83["default_cli"] != true || v83["status"] != "running" {
		t.Errorf("8.3 = %v", v83)
	}
	if list[0].(map[string]any)["status"] != "installed" {
		t.Errorf("CLI-only version should be 'installed': %v", list[0])
	}
}

func TestStackUnknownPackageAndAction(t *testing.T) {
	e, _ := newStackRouter(t, newFakeStack())
	if code, _ := stackPost(t, e, map[string]any{"package": "bash", "action": "install"}); code != http.StatusBadRequest {
		t.Errorf("unknown package: %d", code)
	}
	if code, _ := stackPost(t, e, map[string]any{"package": "nginx", "action": "switch-version", "version": "1.2"}); code != http.StatusBadRequest {
		t.Errorf("unknown action: %d", code)
	}
	if code, _ := stackPost(t, e, map[string]any{"package": "nginx", "action": "install", "version": "1.26.0"}); code != http.StatusBadRequest {
		t.Errorf("version for a distro package: %d", code)
	}
	if code, _ := stackGet(t, e, "/tasks/stack/ops/op_0000000000000000"); code != http.StatusNotFound {
		t.Errorf("unknown op: %d", code)
	}
}
