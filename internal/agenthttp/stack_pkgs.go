package agenthttp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"sigardapanel/internal/nginx"
)

// stackError is a refusal with an HTTP status (400 bad request, 409 conflict,
// 501 unsupported) that the handler returns as is.
type stackError struct {
	Code int
	Msg  string
}

func (e *stackError) Error() string { return e.Msg }

func badRequest(format string, a ...any) error {
	return &stackError{Code: http.StatusBadRequest, Msg: fmt.Sprintf(format, a...)}
}
func conflict(format string, a ...any) error {
	return &stackError{Code: http.StatusConflict, Msg: fmt.Sprintf(format, a...)}
}

// stackPackages lists every component the Stack page manages, in display order.
var stackPackages = []string{"nginx", "php", "node", "python", "go", "docker", "certbot", "mariadb", "redis", "memcached", "supervisor"}

// serviceUnits maps components with a single systemd service to its unit.
// PHP (php<version>-fpm) and MariaDB/MySQL (mariadb or mysql) are resolved
// at runtime.
var serviceUnits = map[string]string{
	"nginx":      "nginx",
	"docker":     "docker",
	"redis":      "redis-server",
	"memcached":  "memcached",
	"supervisor": "supervisor",
}

var (
	phpVersionArgRe = regexp.MustCompile(`^[5-9]\.[0-9]$`)
	phpExtRe        = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,40}$`)
	nodeMajorRe     = regexp.MustCompile(`^[0-9]{2}$`)
	goVersionRe     = regexp.MustCompile(`^[0-9]+\.[0-9]+(\.[0-9]+)?$`)
	phpBinRe        = regexp.MustCompile(`^php([0-9]+\.[0-9]+)$`)
	phpFPMBinRe     = regexp.MustCompile(`^php-fpm([0-9]+\.[0-9]+)$`)
)

// phpKnownVersions are offered for installation (ondrej PPA / sury.org).
var phpKnownVersions = []string{"8.5", "8.4", "8.3", "8.2", "8.1", "8.0", "7.4"}

// phpBaseExtensions are installed with every PHP version (when the
// repository has them; e.g. opcache is built in from 8.5).
var phpBaseExtensions = []string{"fpm", "cli", "common", "mysql", "curl", "gd", "mbstring", "xml", "zip", "bcmath", "soap", "intl", "readline", "opcache"}

// phpCoreParts cannot be removed as "extensions" — that is uninstalling the version.
var phpCoreParts = map[string]bool{"fpm": true, "cli": true, "common": true}

// nodeMajors are the Node.js lines offered (NodeSource).
var nodeMajors = []string{"24", "22", "20"}

const nodeDefaultMajor = "22"

// ─── detection ──────────────────────────────────────────────────────────────

// StackInfo represents detected software on the server.
type StackInfo struct {
	Name     string   `json:"name"`
	Version  string   `json:"version"`
	Status   string   `json:"status"` // running, starting, stopped, failed, installed, missing
	Path     string   `json:"path,omitempty"`
	Service  string   `json:"service,omitempty"`
	Versions []string `json:"versions,omitempty"` // PHP: installed versions
	Detail   string   `json:"detail,omitempty"`
}

type phpVersionInfo struct {
	Version    string `json:"version"`
	Full       string `json:"full,omitempty"`
	Status     string `json:"status"` // running | stopped | failed | starting | installed (cli only)
	Path       string `json:"path,omitempty"`
	FPM        bool   `json:"fpm"`
	Socket     bool   `json:"socket"`
	DefaultCLI bool   `json:"default_cli"`
	Service    string `json:"service,omitempty"`
	Installed  bool   `json:"installed"`
}

func (m *stackManager) detect(ctx context.Context, name string) StackInfo {
	ctx, cancel := quick(ctx)
	defer cancel()
	switch name {
	case "nginx":
		return m.detectBinary(ctx, "nginx", "nginx", []string{"-v"}, "nginx")
	case "php":
		return m.detectPHP(ctx)
	case "node":
		info := m.detectBinary(ctx, "node", "node", []string{"--version"}, "")
		if info.Status != "missing" {
			if p, err := m.sys.LookPath("npm"); err == nil {
				if out, err := m.sys.Run(ctx, nil, nil, p, "--version"); err == nil {
					info.Detail = "npm " + strings.TrimSpace(out)
				}
			}
		}
		return info
	case "python":
		return m.detectBinary(ctx, "python", "python3", []string{"--version"}, "")
	case "go":
		info := m.detectBinary(ctx, "go", "go", []string{"version"}, "")
		if info.Status == "missing" && m.sys.Exists("/usr/local/go/bin/go") {
			out, _ := m.sys.Run(ctx, nil, nil, "/usr/local/go/bin/go", "version")
			info = StackInfo{Name: "go", Version: firstVersion(out), Status: "installed", Path: "/usr/local/go/bin/go"}
		}
		return info
	case "docker":
		return m.detectBinary(ctx, "docker", "docker", []string{"--version"}, "docker")
	case "certbot":
		return m.detectBinary(ctx, "certbot", "certbot", []string{"--version"}, "")
	case "mariadb", "mysql":
		return m.detectMariaDB(ctx)
	case "redis":
		return m.detectBinary(ctx, "redis", "redis-server", []string{"--version"}, "redis-server")
	case "memcached":
		return m.detectBinary(ctx, "memcached", "memcached", []string{"-V"}, "memcached")
	case "supervisor":
		return m.detectBinary(ctx, "supervisor", "supervisord", []string{"--version"}, "supervisor")
	}
	return StackInfo{Name: name, Status: "unknown"}
}

// detectBinary reports a component as missing when its binary is absent,
// otherwise with its version and (for services) the systemd state.
func (m *stackManager) detectBinary(ctx context.Context, name, bin string, args []string, unit string) StackInfo {
	path, err := m.sys.LookPath(bin)
	if err != nil {
		return StackInfo{Name: name, Status: "missing", Service: unit}
	}
	out, _ := m.sys.Run(ctx, nil, nil, path, args...)
	info := StackInfo{Name: name, Version: firstVersion(out), Status: "installed", Path: path, Service: unit}
	if unit != "" {
		if st := m.unitStatus(ctx, unit); st != "missing" {
			info.Status = st
		}
	}
	return info
}

func (m *stackManager) detectMariaDB(ctx context.Context) StackInfo {
	info := StackInfo{Name: "mariadb", Status: "missing"}
	server := ""
	for _, b := range []string{"mariadbd", "mysqld"} {
		if p, err := m.sys.LookPath(b); err == nil {
			server = p
			break
		}
	}
	client := ""
	for _, b := range []string{"mariadb", "mysql"} {
		if p, err := m.sys.LookPath(b); err == nil {
			client = p
			break
		}
	}
	if server == "" && client == "" {
		return info
	}
	bin := server
	if bin == "" {
		bin = client
		info.Detail = "client only (no server installed)"
	}
	out, _ := m.sys.Run(ctx, nil, nil, bin, "--version")
	info.Version = firstVersion(out)
	info.Path = bin
	info.Status = "installed"
	if strings.Contains(strings.ToLower(out), "mariadb") {
		if info.Detail == "" {
			info.Detail = "MariaDB"
		}
	} else if out != "" && info.Detail == "" {
		info.Detail = "MySQL"
	}
	if server != "" {
		unit := m.mariadbUnit(ctx)
		info.Service = unit
		if st := m.unitStatus(ctx, unit); st != "missing" {
			info.Status = st
		}
	}
	return info
}

// mariadbUnit is mariadb.service when it exists, else mysql.service (MySQL
// server, or older MariaDB packages).
func (m *stackManager) mariadbUnit(ctx context.Context) string {
	if m.unitStatus(ctx, "mariadb") != "missing" {
		return "mariadb"
	}
	return "mysql"
}

// phpVersions lists PHP versions that have a CLI binary or an FPM binary.
func (m *stackManager) phpVersions(ctx context.Context) []phpVersionInfo {
	found := map[string]*phpVersionInfo{}
	get := func(v string) *phpVersionInfo {
		if found[v] == nil {
			found[v] = &phpVersionInfo{Version: v, Installed: true, Status: "installed"}
		}
		return found[v]
	}
	bins, _ := m.sys.Glob(filepath.Join(m.binDir, "php*"))
	for _, b := range bins {
		if mm := phpBinRe.FindStringSubmatch(filepath.Base(b)); mm != nil {
			get(mm[1]).Path = b
		}
	}
	sbins, _ := m.sys.Glob(filepath.Join(m.sbinDir, "php-fpm*"))
	for _, b := range sbins {
		if mm := phpFPMBinRe.FindStringSubmatch(filepath.Base(b)); mm != nil {
			get(mm[1]).FPM = true
		}
	}
	defaultCLI := ""
	if target, err := m.sys.EvalSymlinks(filepath.Join(m.binDir, "php")); err == nil {
		if mm := phpBinRe.FindStringSubmatch(filepath.Base(target)); mm != nil {
			defaultCLI = mm[1]
		}
	}
	out := []phpVersionInfo{}
	for _, v := range found {
		v.DefaultCLI = v.Version == defaultCLI
		v.Socket = m.sys.Exists(filepath.Join(m.phpRunDir, "php"+v.Version+"-fpm.sock"))
		if v.FPM {
			v.Service = "php" + v.Version + "-fpm"
			if st := m.unitStatus(ctx, v.Service); st != "missing" {
				v.Status = st
			}
		}
		if v.Path != "" {
			if o, err := m.sys.Run(ctx, nil, nil, v.Path, "-r", "echo PHP_VERSION;"); err == nil {
				v.Full = firstVersion(o)
			}
		}
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool { return compareVersions(out[i].Version, out[j].Version) > 0 })
	return out
}

func (m *stackManager) detectPHP(ctx context.Context) StackInfo {
	versions := m.phpVersions(ctx)
	if len(versions) == 0 {
		return StackInfo{Name: "php", Status: "missing"}
	}
	info := StackInfo{Name: "php", Status: "installed"}
	var list []string
	anyRunning, anyFailed, anyFPM := false, false, false
	for _, v := range versions {
		list = append(list, v.Version)
		if v.FPM {
			anyFPM = true
		}
		switch v.Status {
		case "running":
			anyRunning = true
		case "failed":
			anyFailed = true
		}
	}
	main := versions[0]
	for _, v := range versions {
		if v.DefaultCLI {
			main = v
		}
	}
	info.Version = main.Full
	if info.Version == "" {
		info.Version = main.Version
	}
	info.Path = main.Path
	info.Versions = list
	switch {
	case anyFailed:
		info.Status = "failed"
	case anyRunning:
		info.Status = "running"
	case anyFPM:
		info.Status = "stopped"
	}
	info.Detail = "installed: " + strings.Join(list, ", ")
	return info
}

// ─── version lists ──────────────────────────────────────────────────────────

type packageVersionItem struct {
	Version   string `json:"version"`
	Status    string `json:"status,omitempty"`
	Path      string `json:"path,omitempty"`
	Installed bool   `json:"installed"`
	Detail    string `json:"detail,omitempty"`
}

func (m *stackManager) listInstalled(ctx context.Context, pkg string) any {
	if pkg == "php" {
		return m.phpVersions(ctx)
	}
	info := m.detect(ctx, pkg)
	versions := []packageVersionItem{}
	if info.Status != "missing" {
		versions = append(versions, packageVersionItem{Version: info.Version, Status: info.Status, Path: info.Path, Installed: true, Detail: info.Detail})
	}
	return versions
}

func (m *stackManager) listAvailable(ctx context.Context, pkg string) []packageVersionItem {
	out := []packageVersionItem{}
	switch pkg {
	case "php":
		installed := map[string]bool{}
		for _, v := range m.phpVersions(ctx) {
			installed[v.Version] = true
		}
		for _, v := range phpKnownVersions {
			out = append(out, packageVersionItem{Version: v, Installed: installed[v]})
		}
	case "node":
		info := m.detect(ctx, "node")
		for _, v := range nodeMajors {
			out = append(out, packageVersionItem{Version: v, Installed: info.Status != "missing" && strings.HasPrefix(info.Version, v+".")})
		}
	case "go":
		info := m.detect(ctx, "go")
		for _, v := range m.goStableVersions(ctx) {
			out = append(out, packageVersionItem{Version: v, Installed: info.Version == v})
		}
	}
	return out
}

// ─── Go releases (go.dev) ───────────────────────────────────────────────────

type goRelease struct {
	Version string `json:"version"`
	Stable  bool   `json:"stable"`
	Files   []struct {
		Filename string `json:"filename"`
		OS       string `json:"os"`
		Arch     string `json:"arch"`
		SHA256   string `json:"sha256"`
		Kind     string `json:"kind"`
	} `json:"files"`
}

var goFallbackVersions = []string{"1.25.4", "1.24.10"}

func (m *stackManager) goReleases(ctx context.Context, all bool) ([]goRelease, error) {
	u := m.goDLURL + "?mode=json"
	if all {
		u += "&include=all"
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("go.dev returned %d", resp.StatusCode)
	}
	var rel []goRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, 32<<20)).Decode(&rel); err != nil {
		return nil, err
	}
	return rel, nil
}

func (m *stackManager) goStableVersions(ctx context.Context) []string {
	rel, err := m.goReleases(ctx, false)
	if err != nil || len(rel) == 0 {
		return goFallbackVersions
	}
	var out []string
	for _, r := range rel {
		if r.Stable {
			out = append(out, strings.TrimPrefix(r.Version, "go"))
		}
	}
	if len(out) == 0 {
		return goFallbackVersions
	}
	return out
}

// goArchive returns the download URL and sha256 of the linux tarball for
// version on this architecture.
func (m *stackManager) goArchive(ctx context.Context, version string) (string, string, error) {
	for _, all := range []bool{false, true} {
		rel, err := m.goReleases(ctx, all)
		if err != nil {
			return "", "", fmt.Errorf("cannot read the Go release list from %s: %v", m.goDLURL, err)
		}
		for _, r := range rel {
			if r.Version != "go"+version {
				continue
			}
			for _, f := range r.Files {
				if f.OS == "linux" && f.Arch == runtime.GOARCH && f.Kind == "archive" {
					return strings.TrimRight(m.goDLURL, "/") + "/" + f.Filename, f.SHA256, nil
				}
			}
		}
	}
	return "", "", badRequest("Go %s is not published for linux/%s", version, runtime.GOARCH)
}

// ─── PHP ────────────────────────────────────────────────────────────────────

// ensurePHPRepo makes php<version>-fpm installable: the distribution's own
// packages when it has that version, else ondrej/php (Ubuntu) or
// packages.sury.org (Debian). Idempotent.
func (m *stackManager) ensurePHPRepo(ctx context.Context, w io.Writer, version string) error {
	pkg := "php" + version + "-fpm"
	if err := m.aptUpdate(ctx, w); err != nil {
		return fmt.Errorf("apt-get update failed")
	}
	if m.aptCandidates(ctx, pkg)[pkg] {
		return nil
	}
	osr := m.osRelease()
	switch {
	case osr.isUbuntu():
		fmt.Fprintf(w, "\n[sigardapanel] PHP %s is not in the Ubuntu archive, adding ppa:ondrej/php…\n", version)
		if err := m.aptInstall(ctx, w, "software-properties-common", "ca-certificates", "gnupg"); err != nil {
			return fmt.Errorf("could not install software-properties-common")
		}
		if _, err := m.sys.Run(ctx, w, aptEnv, "add-apt-repository", "-y", "-n", "ppa:ondrej/php"); err != nil {
			return fmt.Errorf("add-apt-repository ppa:ondrej/php failed")
		}
	case osr.isDebian():
		if osr.Codename == "" {
			return fmt.Errorf("cannot determine the Debian codename from /etc/os-release")
		}
		fmt.Fprintf(w, "\n[sigardapanel] PHP %s is not in the Debian archive, adding packages.sury.org/php…\n", version)
		if err := m.aptInstall(ctx, w, "ca-certificates", "curl"); err != nil {
			return fmt.Errorf("could not install curl/ca-certificates")
		}
		keyring := "/usr/share/keyrings/deb.sury.org-php.gpg"
		if !m.sys.Exists(keyring) {
			if _, err := m.sys.Run(ctx, w, nil, "curl", "-fsSL", "--retry", "3", "-o", keyring, "https://packages.sury.org/php/apt.gpg"); err != nil {
				return fmt.Errorf("could not download the packages.sury.org signing key")
			}
		}
		line := fmt.Sprintf("deb [signed-by=%s] https://packages.sury.org/php/ %s main\n", keyring, osr.Codename)
		if err := m.sys.WriteFile("/etc/apt/sources.list.d/sury-php.list", []byte(line), 0o644); err != nil {
			return fmt.Errorf("could not write the sury.org apt source: %v", err)
		}
	default:
		return &stackError{Code: http.StatusNotImplemented, Msg: fmt.Sprintf("PHP %s is not available from this distribution's repositories (%s) and only Ubuntu/Debian third-party repositories are supported", version, osr.name())}
	}
	if err := m.aptUpdate(ctx, w); err != nil {
		return fmt.Errorf("apt-get update failed after adding the PHP repository")
	}
	if !m.aptCandidates(ctx, pkg)[pkg] {
		return badRequest("PHP %s is not available for %s (no %s package, even with the third-party PHP repository)", version, osr.name(), pkg)
	}
	return nil
}

func (m *stackManager) phpInstalledPackages(ctx context.Context, version string, withConfig bool) []string {
	var out []string
	prefix := "php" + version
	for _, p := range m.dpkgPackages(ctx, withConfig, prefix, prefix+"-*") {
		if p == prefix || strings.HasPrefix(p, prefix+"-") {
			out = append(out, p)
		}
	}
	return out
}

func (m *stackManager) phpInstall(ctx context.Context, w io.Writer, version string) (string, error) {
	if err := m.ensurePHPRepo(ctx, w, version); err != nil {
		return "", err
	}
	var want []string
	for _, ext := range phpBaseExtensions {
		want = append(want, "php"+version+"-"+ext)
	}
	cand := m.aptCandidates(ctx, want...)
	var pkgs, skipped []string
	for _, p := range want {
		if cand[p] {
			pkgs = append(pkgs, p)
		} else {
			skipped = append(skipped, p)
		}
	}
	if !cand["php"+version+"-cli"] {
		return "", badRequest("php%s-cli is not available", version)
	}
	if err := m.aptInstall(ctx, w, pkgs...); err != nil {
		return "", fmt.Errorf("apt-get install failed")
	}
	unit := "php" + version + "-fpm"
	m.sys.Run(ctx, w, nil, "systemctl", "enable", "--now", unit)
	if err := m.verifyPHP(ctx, w, version); err != nil {
		return "", err
	}
	msg := fmt.Sprintf("PHP %s installed, %s running", version, unit)
	if len(skipped) > 0 {
		msg += " (not in the repository, skipped: " + strings.Join(skipped, ", ") + ")"
	}
	return msg, nil
}

// verifyPHP checks the version's CLI, FPM service and socket.
func (m *stackManager) verifyPHP(ctx context.Context, w io.Writer, version string) error {
	unit := "php" + version + "-fpm"
	if st := m.waitUnit(ctx, unit, "running", 20*time.Second); st != "running" {
		m.unitDiagnostics(ctx, w, unit)
		return fmt.Errorf("packages are installed but %s is %s", unit, st)
	}
	sock := filepath.Join(m.phpRunDir, "php"+version+"-fpm.sock")
	for i := 0; i < 15 && !m.sys.Exists(sock); i++ {
		m.sys.Sleep(ctx, time.Second)
	}
	if !m.sys.Exists(sock) {
		return fmt.Errorf("%s is running but %s does not exist (check the pool's listen setting)", unit, sock)
	}
	for _, v := range m.phpVersions(ctx) {
		if v.Version == version && v.Path != "" {
			return nil
		}
	}
	return fmt.Errorf("php%s CLI binary not found after installation", version)
}

// phpVhostUsers lists nginx config files that pass PHP to this version's
// socket or CloudPanel-style TCP port (comments ignored).
func (m *stackManager) phpVhostUsers(version string) []string {
	needles := []string{"php" + version + "-fpm.sock"}
	if port, ok := nginx.PHPFPMTCPPort(version); ok {
		needles = append(needles, fmt.Sprintf("127.0.0.1:%d", port))
	}
	var users []string
	seen := map[string]bool{}
	for _, dir := range m.vhostDirs {
		files, _ := m.sys.Glob(filepath.Join(dir, "*"))
		for _, f := range files {
			data, err := m.sys.ReadFile(f)
			if err != nil {
				continue
			}
			for _, line := range strings.Split(string(data), "\n") {
				t := strings.TrimSpace(line)
				if t == "" || strings.HasPrefix(t, "#") {
					continue
				}
				if i := strings.Index(t, "#"); i >= 0 {
					t = t[:i]
				}
				hit := false
				for _, n := range needles {
					if strings.Contains(t, n) {
						hit = true
					}
				}
				if hit {
					if name := filepath.Base(f); !seen[name] {
						seen[name] = true
						users = append(users, name)
					}
					break
				}
			}
		}
	}
	sort.Strings(users)
	return users
}

func (m *stackManager) phpUninstall(ctx context.Context, w io.Writer, version string) (string, error) {
	if users := m.phpVhostUsers(version); len(users) > 0 {
		return "", conflict("PHP %s is still used by nginx vhost(s): %s. Switch those sites to another PHP version first", version, strings.Join(users, ", "))
	}
	pkgs := m.phpInstalledPackages(ctx, version, true)
	if len(pkgs) == 0 {
		return "", badRequest("PHP %s is not installed", version)
	}
	// Version-less meta packages (php, php-fpm, php-mysql …) depend on the
	// distro's default version and may go with it.
	allow := func(p string) bool {
		return p == "php" || (strings.HasPrefix(p, "php-") && !regexp.MustCompile(`[0-9]`).MatchString(p))
	}
	m.sys.Run(ctx, w, nil, "systemctl", "disable", "--now", "php"+version+"-fpm")
	if err := m.aptRemove(ctx, w, true, pkgs, allow); err != nil {
		// put the service back the way it was
		m.sys.Run(ctx, w, nil, "systemctl", "enable", "--now", "php"+version+"-fpm")
		return "", err
	}
	for _, v := range m.phpVersions(ctx) {
		if v.Version == version {
			return "", fmt.Errorf("apt-get finished but PHP %s is still detected (%s)", version, v.Path)
		}
	}
	return fmt.Sprintf("PHP %s removed (%d packages purged)", version, len(pkgs)), nil
}

func (m *stackManager) phpUpdate(ctx context.Context, w io.Writer, version string) (string, error) {
	versions := []string{version}
	if version == "" {
		versions = nil
		for _, v := range m.phpVersions(ctx) {
			versions = append(versions, v.Version)
		}
	}
	if len(versions) == 0 {
		return "", badRequest("no PHP version is installed")
	}
	if err := m.aptUpdate(ctx, w); err != nil {
		return "", fmt.Errorf("apt-get update failed")
	}
	full := func() map[string]string {
		res := map[string]string{}
		for _, v := range m.phpVersions(ctx) {
			res[v.Version] = v.Full
		}
		return res
	}
	before := full()
	var pkgs []string
	for _, v := range versions {
		pkgs = append(pkgs, m.phpInstalledPackages(ctx, v, false)...)
	}
	if err := m.aptUpgrade(ctx, w, pkgs...); err != nil {
		return "", fmt.Errorf("apt-get upgrade failed")
	}
	for _, v := range versions {
		unit := "php" + v + "-fpm"
		if m.unitStatus(ctx, unit) == "missing" {
			continue
		}
		m.sys.Run(ctx, w, nil, "systemctl", "restart", unit)
		if st := m.waitUnit(ctx, unit, "running", 20*time.Second); st != "running" {
			m.unitDiagnostics(ctx, w, unit)
			return "", fmt.Errorf("%s is %s after the update", unit, st)
		}
	}
	after := full()
	var parts []string
	for _, v := range versions {
		if before[v] != after[v] {
			parts = append(parts, fmt.Sprintf("PHP %s updated %s → %s", v, before[v], after[v]))
		} else {
			parts = append(parts, fmt.Sprintf("PHP %s is already the newest packaged version (%s)", v, after[v]))
		}
	}
	return strings.Join(parts, "; "), nil
}

func (m *stackManager) phpExtensionInstall(ctx context.Context, w io.Writer, version, ext string) (string, error) {
	pkg := "php" + version + "-" + ext
	if err := m.aptUpdate(ctx, w); err != nil {
		return "", fmt.Errorf("apt-get update failed")
	}
	if !m.aptCandidates(ctx, pkg)[pkg] {
		return "", badRequest("%s is not available in the configured repositories", pkg)
	}
	if err := m.aptInstall(ctx, w, pkg); err != nil {
		return "", fmt.Errorf("apt-get install %s failed", pkg)
	}
	if len(m.dpkgPackages(ctx, false, pkg)) == 0 {
		return "", fmt.Errorf("apt-get finished but %s is not installed", pkg)
	}
	if err := m.restartPHPFPM(ctx, w, version); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s installed, php%s-fpm restarted", pkg, version), nil
}

func (m *stackManager) phpExtensionUninstall(ctx context.Context, w io.Writer, version, ext string) (string, error) {
	if phpCoreParts[ext] {
		return "", badRequest("%s is part of PHP %s itself; uninstall the version instead", ext, version)
	}
	pkg := "php" + version + "-" + ext
	if len(m.dpkgPackages(ctx, true, pkg)) == 0 {
		return "", badRequest("%s is not installed", pkg)
	}
	if err := m.aptRemove(ctx, w, true, []string{pkg}, nil); err != nil {
		return "", err
	}
	if len(m.dpkgPackages(ctx, false, pkg)) > 0 {
		return "", fmt.Errorf("apt-get finished but %s is still installed", pkg)
	}
	if err := m.restartPHPFPM(ctx, w, version); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s removed, php%s-fpm restarted", pkg, version), nil
}

func (m *stackManager) restartPHPFPM(ctx context.Context, w io.Writer, version string) error {
	unit := "php" + version + "-fpm"
	if m.unitStatus(ctx, unit) == "missing" {
		return nil
	}
	m.sys.Run(ctx, w, nil, "systemctl", "restart", unit)
	if st := m.waitUnit(ctx, unit, "running", 20*time.Second); st != "running" {
		m.unitDiagnostics(ctx, w, unit)
		return fmt.Errorf("%s is %s after the restart", unit, st)
	}
	return nil
}

type phpExtensions struct {
	Version   string   `json:"version"`
	Installed []string `json:"installed"`
	Available []string `json:"available"`
	Modules   []string `json:"modules"`
}

func (m *stackManager) phpListExtensions(ctx context.Context, version string) (phpExtensions, error) {
	res := phpExtensions{Version: version, Installed: []string{}, Available: []string{}, Modules: []string{}}
	var found bool
	var bin string
	for _, v := range m.phpVersions(ctx) {
		if v.Version == version {
			found, bin = true, v.Path
		}
	}
	if !found {
		return res, badRequest("PHP %s is not installed", version)
	}
	prefix := "php" + version + "-"
	installed := map[string]bool{}
	for _, p := range m.phpInstalledPackages(ctx, version, false) {
		if ext := strings.TrimPrefix(p, prefix); ext != p && !phpCoreParts[ext] {
			res.Installed = append(res.Installed, ext)
			installed[ext] = true
		}
	}
	lctx, cancel := quick(ctx)
	defer cancel()
	out, _ := m.sys.Run(lctx, nil, aptEnv, "apt-cache", "pkgnames", prefix)
	seen := map[string]bool{}
	for _, p := range strings.Fields(out) {
		ext := strings.TrimPrefix(p, prefix)
		if ext == p || phpCoreParts[ext] || installed[ext] || seen[ext] || !phpExtRe.MatchString(ext) || strings.HasSuffix(ext, "-dbgsym") || ext == "dev" {
			continue
		}
		seen[ext] = true
		res.Available = append(res.Available, ext)
	}
	sort.Strings(res.Available)
	if bin != "" {
		mods, _ := m.sys.Run(lctx, nil, nil, bin, "-m")
		for _, l := range strings.Split(mods, "\n") {
			l = strings.TrimSpace(l)
			if l != "" && !strings.HasPrefix(l, "[") {
				res.Modules = append(res.Modules, l)
			}
		}
	}
	return res, nil
}

func (m *stackManager) phpSetDefault(ctx context.Context, w io.Writer, version string) (string, error) {
	bin := filepath.Join(m.binDir, "php"+version)
	if !m.sys.Exists(bin) {
		return "", badRequest("PHP %s CLI is not installed", version)
	}
	if out, err := m.sys.Run(ctx, w, nil, "update-alternatives", "--set", "php", bin); err != nil {
		return "", fmt.Errorf("update-alternatives failed: %s", lastLines(out, 2))
	}
	for _, v := range m.phpVersions(ctx) {
		if v.Version == version && v.DefaultCLI {
			return "php now runs PHP " + version + " on the command line", nil
		}
	}
	return "", fmt.Errorf("update-alternatives finished but %s/php does not point to PHP %s", m.binDir, version)
}

// ─── Node.js (NodeSource) ───────────────────────────────────────────────────

func (m *stackManager) nodeInstall(ctx context.Context, w io.Writer, major string) (string, error) {
	if major == "" {
		major = nodeDefaultMajor
	}
	if !nodeMajorRe.MatchString(major) {
		return "", badRequest("Node.js version must be a major version such as %s", nodeDefaultMajor)
	}
	if err := m.aptUpdate(ctx, w); err != nil {
		return "", fmt.Errorf("apt-get update failed")
	}
	if err := m.aptInstall(ctx, w, "ca-certificates", "curl", "gnupg"); err != nil {
		return "", fmt.Errorf("could not install curl/gnupg")
	}
	keyring := "/etc/apt/keyrings/nodesource.gpg"
	if !m.sys.Exists(keyring) {
		m.sys.Run(ctx, w, nil, "mkdir", "-p", "/etc/apt/keyrings")
		if _, err := m.sys.Run(ctx, w, nil, "sh", "-c",
			"set -e; curl -fsSL --retry 3 https://deb.nodesource.com/gpgkey/nodesource-repo.gpg.key | gpg --dearmor --batch --yes -o "+keyring); err != nil {
			return "", fmt.Errorf("could not download the NodeSource signing key")
		}
	}
	src := fmt.Sprintf("deb [signed-by=%s] https://deb.nodesource.com/node_%s.x nodistro main\n", keyring, major)
	if err := m.sys.WriteFile("/etc/apt/sources.list.d/nodesource.list", []byte(src), 0o644); err != nil {
		return "", fmt.Errorf("could not write the NodeSource apt source: %v", err)
	}
	pin := "Package: nodejs\nPin: origin deb.nodesource.com\nPin-Priority: 600\n"
	if err := m.sys.WriteFile("/etc/apt/preferences.d/nodesource", []byte(pin), 0o644); err != nil {
		return "", fmt.Errorf("could not write the NodeSource apt pin: %v", err)
	}
	if err := m.aptUpdate(ctx, w); err != nil {
		return "", fmt.Errorf("apt-get update failed after adding NodeSource")
	}
	if _, err := m.apt(ctx, w, "install", "--allow-downgrades", "nodejs"); err != nil {
		return "", fmt.Errorf("apt-get install nodejs failed")
	}
	info := m.detect(ctx, "node")
	if info.Status == "missing" {
		return "", fmt.Errorf("apt-get finished but node is not on PATH")
	}
	if !strings.HasPrefix(info.Version, major+".") {
		return "", fmt.Errorf("node %s is installed, not the requested %s.x (check apt pinning)", info.Version, major)
	}
	return fmt.Sprintf("Node.js %s installed (%s)", info.Version, info.Detail), nil
}

// ─── Go (official tarball) ──────────────────────────────────────────────────

func (m *stackManager) goInstall(ctx context.Context, w io.Writer, version string) (string, error) {
	if version == "" {
		v := m.goStableVersions(ctx)
		version = v[0]
	}
	if !goVersionRe.MatchString(version) {
		return "", badRequest("invalid Go version %q (expected e.g. 1.25.4)", version)
	}
	url, sum, err := m.goArchive(ctx, version)
	if err != nil {
		return "", err
	}
	if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(sum) || !strings.HasPrefix(url, "https://") {
		return "", fmt.Errorf("unexpected release metadata for Go %s", version)
	}
	script := fmt.Sprintf(`set -eu
d=$(mktemp -d); trap 'rm -rf "$d"' EXIT
curl -fsSL --retry 3 -o "$d/go.tgz" '%s'
echo '%s  '"$d/go.tgz" | sha256sum -c -
rm -rf /usr/local/go.new; mkdir /usr/local/go.new
tar -C /usr/local/go.new -xzf "$d/go.tgz"
rm -rf /usr/local/go; mv /usr/local/go.new/go /usr/local/go; rmdir /usr/local/go.new
ln -sfn /usr/local/go/bin/go /usr/local/bin/go
ln -sfn /usr/local/go/bin/gofmt /usr/local/bin/gofmt
printf '%%s\n' 'export PATH=/usr/local/go/bin:$PATH' > /etc/profile.d/sigardapanel-go.sh
chmod 0644 /etc/profile.d/sigardapanel-go.sh`, url, sum)
	if _, err := m.sys.Run(ctx, w, nil, "sh", "-c", script); err != nil {
		return "", fmt.Errorf("download or checksum verification of Go %s failed", version)
	}
	info := m.detect(ctx, "go")
	if info.Version != version {
		return "", fmt.Errorf("installed Go reports %q, expected %s", info.Version, version)
	}
	return "Go " + version + " installed in /usr/local/go (sha256 verified)", nil
}

const goUninstallScript = `set -eu
rm -rf /usr/local/go
if [ "$(readlink /usr/local/bin/go 2>/dev/null || true)" = /usr/local/go/bin/go ]; then rm -f /usr/local/bin/go; fi
if [ "$(readlink /usr/local/bin/gofmt 2>/dev/null || true)" = /usr/local/go/bin/gofmt ]; then rm -f /usr/local/bin/gofmt; fi
rm -f /etc/profile.d/sigardapanel-go.sh`

// ─── generic apt components ─────────────────────────────────────────────────

// aptComponent describes a component installed from distribution packages.
type aptComponent struct {
	install []string // packages to install
	remove  []string // packages removed on uninstall (only the installed ones)
	unit    string   // systemd unit to enable/start after install ("" = none)
}

func (m *stackManager) aptSpec(ctx context.Context, pkg string) aptComponent {
	switch pkg {
	case "nginx":
		return aptComponent{install: []string{"nginx"}, remove: []string{"nginx", "nginx-core", "nginx-full", "nginx-light", "nginx-extras"}, unit: "nginx"}
	case "python":
		// python3 itself belongs to the base system (removing it takes
		// apt, netplan, cloud-init… with it); only pip/venv are managed.
		return aptComponent{install: []string{"python3", "python3-pip", "python3-venv"}, remove: []string{"python3-pip", "python3-venv"}}
	case "node":
		return aptComponent{remove: []string{"nodejs", "npm"}}
	case "docker":
		return aptComponent{remove: []string{"docker.io", "docker-ce", "docker-ce-cli", "docker-ce-rootless-extras", "docker-compose", "docker-compose-v2", "docker-compose-plugin", "docker-buildx-plugin", "docker-buildx", "podman-docker"}, unit: "docker"}
	case "certbot":
		return aptComponent{install: []string{"certbot", "python3-certbot-nginx"}, remove: []string{"certbot", "python3-certbot-nginx"}}
	case "mariadb":
		// The -core packages hold mariadbd / mariadb; removing only the
		// meta packages leaves the server binary behind.
		return aptComponent{install: []string{"mariadb-server", "mariadb-client"},
			remove: []string{"mariadb-server", "mariadb-server-core*", "mariadb-server-1*", "mariadb-client", "mariadb-client-core*", "mariadb-client-1*", "mariadb-plugin-*", "mariadb-backup"}, unit: "mariadb"}
	case "redis":
		return aptComponent{install: []string{"redis-server"}, remove: []string{"redis-server", "redis-tools"}, unit: "redis-server"}
	case "memcached":
		return aptComponent{install: []string{"memcached"}, remove: []string{"memcached"}, unit: "memcached"}
	case "supervisor":
		return aptComponent{install: []string{"supervisor"}, remove: []string{"supervisor"}, unit: "supervisor"}
	}
	return aptComponent{}
}

// dockerInstallPackages picks Docker CE when its repository is configured,
// else the distribution's docker.io plus a compose v2 plugin when available.
func (m *stackManager) dockerInstallPackages(ctx context.Context) []string {
	c := m.aptCandidates(ctx, "docker-ce", "docker-ce-cli", "containerd.io", "docker-compose-plugin", "docker-buildx-plugin", "docker.io", "docker-compose-v2")
	if c["docker-ce"] && c["docker-ce-cli"] && c["containerd.io"] {
		pkgs := []string{"docker-ce", "docker-ce-cli", "containerd.io"}
		for _, p := range []string{"docker-compose-plugin", "docker-buildx-plugin"} {
			if c[p] {
				pkgs = append(pkgs, p)
			}
		}
		return pkgs
	}
	pkgs := []string{"docker.io"}
	if c["docker-compose-v2"] {
		pkgs = append(pkgs, "docker-compose-v2")
	} else if c["docker-compose-plugin"] {
		pkgs = append(pkgs, "docker-compose-plugin")
	}
	return pkgs
}

func (m *stackManager) aptComponentInstall(ctx context.Context, w io.Writer, pkg string) (string, error) {
	spec := m.aptSpec(ctx, pkg)
	if err := m.aptUpdate(ctx, w); err != nil {
		return "", fmt.Errorf("apt-get update failed")
	}
	pkgs := spec.install
	if pkg == "docker" {
		pkgs = m.dockerInstallPackages(ctx)
	}
	if pkg == "mariadb" {
		if mysql := m.dpkgPackages(ctx, false, "mysql-server*"); len(mysql) > 0 {
			return "", conflict("MySQL server is installed (%s); installing MariaDB would replace it. Manage the existing MySQL instead", strings.Join(mysql, ", "))
		}
	}
	cand := m.aptCandidates(ctx, pkgs...)
	for _, p := range pkgs {
		if !cand[p] {
			return "", badRequest("package %s is not available in the configured repositories", p)
		}
	}
	if err := m.aptInstall(ctx, w, pkgs...); err != nil {
		return "", fmt.Errorf("apt-get install failed")
	}
	unit := spec.unit
	if pkg == "mariadb" {
		unit = m.mariadbUnit(ctx)
	}
	info := m.detect(ctx, pkg)
	if info.Status == "missing" {
		return "", fmt.Errorf("apt-get finished but %s is not detected", pkg)
	}
	if unit != "" {
		if pkg == "nginx" {
			if out, err := m.sys.Run(ctx, w, nil, "nginx", "-t"); err != nil {
				return "", fmt.Errorf("nginx is installed but its configuration is invalid: %s", lastLines(out, 3))
			}
		}
		m.sys.Run(ctx, w, nil, "systemctl", "enable", "--now", unit)
		if st := m.waitUnit(ctx, unit, "running", 30*time.Second); st != "running" {
			m.unitDiagnostics(ctx, w, unit)
			return "", fmt.Errorf("%s is installed but the %s service is %s", pkg, unit, st)
		}
		info = m.detect(ctx, pkg)
	}
	msg := fmt.Sprintf("%s %s installed", pkg, info.Version)
	if unit != "" {
		msg += ", " + unit + " running"
	}
	return msg, nil
}

func (m *stackManager) aptComponentUninstall(ctx context.Context, w io.Writer, pkg string) (string, error) {
	spec := m.aptSpec(ctx, pkg)
	if pkg == "docker" {
		if p, err := m.sys.LookPath("docker"); err == nil {
			qctx, cancel := quick(ctx)
			out, err := m.sys.Run(qctx, nil, nil, p, "ps", "-q")
			cancel()
			if err == nil && strings.TrimSpace(out) != "" {
				n := len(strings.Fields(out))
				return "", conflict("%d Docker container(s) are running; stop or remove them first", n)
			}
		}
	}
	installed := m.dpkgPackages(ctx, false, spec.remove...)
	if len(installed) == 0 {
		return "", badRequest("%s is not installed from packages (nothing to remove)", pkg)
	}
	if err := m.aptRemove(ctx, w, false, installed, nil); err != nil {
		return "", err
	}
	if left := m.dpkgPackages(ctx, false, installed...); len(left) > 0 {
		return "", fmt.Errorf("apt-get finished but still installed: %s", strings.Join(left, ", "))
	}
	// Report success only when the component is really gone (python3 is
	// kept on purpose).
	if pkg != "python" {
		if info := m.detect(ctx, pkg); info.Status != "missing" {
			return "", fmt.Errorf("removed %s, but %s is still present at %s (installed from another source?)",
				strings.Join(installed, ", "), pkg, info.Path)
		}
	}
	msg := fmt.Sprintf("%s removed (%s)", pkg, strings.Join(installed, ", "))
	switch pkg {
	case "python":
		msg += "; python3 itself is part of the base system and stays installed"
	case "mariadb":
		msg += "; the data directory /var/lib/mysql was kept"
	case "redis":
		msg += "; /var/lib/redis was kept"
	}
	return msg, nil
}

func (m *stackManager) aptComponentUpdate(ctx context.Context, w io.Writer, pkg string) (string, error) {
	spec := m.aptSpec(ctx, pkg)
	names := append(append([]string{}, spec.install...), spec.remove...)
	if pkg == "python" {
		names = append(names, "python3-minimal")
	}
	installed := m.dpkgPackages(ctx, false, names...)
	if len(installed) == 0 {
		return "", badRequest("%s is not installed from packages", pkg)
	}
	if err := m.aptUpdate(ctx, w); err != nil {
		return "", fmt.Errorf("apt-get update failed")
	}
	before := m.detect(ctx, pkg)
	if err := m.aptUpgrade(ctx, w, installed...); err != nil {
		return "", fmt.Errorf("apt-get upgrade failed")
	}
	unit := spec.unit
	if pkg == "mariadb" {
		unit = m.mariadbUnit(ctx)
	}
	if unit != "" && before.Status == "running" {
		if st := m.waitUnit(ctx, unit, "running", 30*time.Second); st != "running" {
			m.unitDiagnostics(ctx, w, unit)
			return "", fmt.Errorf("%s is %s after the update", unit, st)
		}
	}
	after := m.detect(ctx, pkg)
	if after.Version == before.Version {
		return fmt.Sprintf("%s is already the newest version (%s)", pkg, after.Version), nil
	}
	return fmt.Sprintf("%s updated %s → %s", pkg, before.Version, after.Version), nil
}

// ─── services ───────────────────────────────────────────────────────────────

// serviceAction runs start/stop/restart/reload and verifies the resulting
// state. For PHP an empty version means every installed FPM service.
func (m *stackManager) serviceAction(ctx context.Context, pkg, action, version string) (string, string, error) {
	var units []string
	switch pkg {
	case "php":
		if version != "" {
			units = []string{"php" + version + "-fpm"}
		} else {
			for _, v := range m.phpVersions(ctx) {
				if v.FPM {
					units = append(units, v.Service)
				}
			}
		}
	case "mariadb", "mysql":
		units = []string{m.mariadbUnit(ctx)}
	default:
		u, ok := serviceUnits[pkg]
		if !ok {
			return "", "", badRequest("%s has no service to %s", pkg, action)
		}
		units = []string{u}
	}
	if len(units) == 0 {
		return "", "", badRequest("no PHP-FPM service is installed")
	}
	var log strings.Builder
	for _, u := range units {
		if m.unitStatus(ctx, u) == "missing" {
			return "", log.String(), badRequest("%s is not installed (no %s service)", pkg, u)
		}
	}
	if pkg == "nginx" && action != "stop" {
		out, err := m.sys.Run(ctx, &log, nil, "nginx", "-t")
		if err != nil {
			return "", log.String(), conflict("nginx configuration test failed, not %sing: %s", action, lastLines(out, 3))
		}
	}
	verb := action
	if action == "reload" {
		verb = "reload-or-restart"
	}
	var done []string
	for _, u := range units {
		m.sys.Run(ctx, &log, nil, "systemctl", verb, u)
		want := "running"
		if action == "stop" {
			want = "stopped"
		}
		st := m.waitUnit(ctx, u, want, 20*time.Second)
		ok := st == want || (want == "stopped" && st != "running" && st != "starting")
		if !ok {
			m.unitDiagnostics(ctx, &log, u)
			return "", log.String(), fmt.Errorf("%s %s: service is %s", action, u, st)
		}
		done = append(done, u)
	}
	past := map[string]string{"start": "started", "stop": "stopped", "restart": "restarted", "reload": "reloaded"}[action]
	return fmt.Sprintf("%s %s", strings.Join(done, ", "), past), log.String(), nil
}
