package agenthttp

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"sigardapanel/internal/httpjson"
)

// StackInfo represents detected software on the server.
type StackInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Status  string `json:"status"` // installed, running, stopped, missing
	Path    string `json:"path,omitempty"`
}

// StackRequest is the payload for stack operations.
type StackRequest struct {
	Package string `json:"package"` // nginx, php, node, python, go, docker, certbot, mariadb/mysql, redis, supervisor
	Version string `json:"version,omitempty"`
	Action  string `json:"action,omitempty"` // install, uninstall, restart, update, switch-version, start, stop
}

// StackResponse is the result of a stack operation.
type StackResponse struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
	Output  string `json:"output,omitempty"`
	Version string `json:"version,omitempty"`
}

// --- Agent task handlers ---

// stackInfoTask detects all installed software and their versions.
func (r *Router) stackInfoTask(c echo.Context) error {
	var packages []StackInfo

	detectors := []func() StackInfo{
		r.detectNginx,
		r.detectPHP,
		r.detectNode,
		r.detectPython,
		r.detectGo,
		r.detectDocker,
		r.detectCertbot,
		r.detectMariaDB,
		r.detectRedis,
		r.detectMemcached,
		r.detectSupervisor,
	}

	for _, detect := range detectors {
		packages = append(packages, detect())
	}

	return c.JSON(http.StatusOK, httpjson.Success{
		Data: map[string]any{
			"packages": packages,
			"os":       runtime.GOOS,
			"arch":     runtime.GOARCH,
		},
		RequestID: "",
	})
}

// stackInstallTask installs, updates, or removes a package.
func (r *Router) stackInstallTask(c echo.Context) error {
	ctx := c.Request().Context()
	// Stack actions always run real commands — no stubDev bypass

	var req StackRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid payload")
	}

	if req.Package == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "package is required")
	}
	if req.Action == "" {
		req.Action = "install"
	}

	// Handle special version listing actions
	if req.Action == "list-versions" {
		return r.listPackageVersions(c, req.Package)
	}
	if req.Action == "list-available" {
		return r.listPackageAvailableVersions(c, req.Package)
	}

	pm := detectPackageManager()
	if pm == "" {
		return echo.NewHTTPError(http.StatusInternalServerError, "unsupported package manager")
	}

	var output string
	var err error
	start := time.Now()

	switch req.Package {
	case "nginx":
		output, err = r.manageNginx(ctx, req.Action, req.Version, pm)
	case "php":
		output, err = r.managePHP(ctx, req.Action, req.Version, pm)
	case "node":
		output, err = r.manageNode(ctx, req.Action, req.Version, pm)
	case "python":
		output, err = r.managePython(ctx, req.Action, req.Version, pm)
	case "go":
		output, err = r.manageGo(ctx, req.Action, req.Version, pm)
	case "docker":
		output, err = r.manageDocker(ctx, req.Action, req.Version, pm)
	case "certbot":
		output, err = r.manageCertbot(ctx, req.Action, req.Version, pm)
	case "mariadb", "mysql":
		output, err = r.manageMariaDB(ctx, req.Action, req.Version, pm)
	case "redis":
		output, err = r.manageRedis(ctx, req.Action, req.Version, pm)
	case "memcached":
		output, err = r.manageMemcached(ctx, req.Action, req.Version, pm)
	case "supervisor":
		output, err = r.manageSupervisor(ctx, req.Action, req.Version, pm)
	default:
		return echo.NewHTTPError(http.StatusBadRequest, "unknown package: "+req.Package)
	}

	elapsed := time.Since(start).Milliseconds()
	if err != nil {
		slog.Error("agent: stack.install failed", "package", req.Package, "action", req.Action, "error", err, "elapsed_ms", elapsed)
		return c.JSON(http.StatusOK, StackResponse{
			OK:      false,
			Message: fmt.Sprintf("%s %s failed", req.Action, req.Package),
			Output:  output,
		})
	}

	slog.Info("agent: stack.install ok", "package", req.Package, "action", req.Action, "elapsed_ms", elapsed)

	info := r.detectPackage(req.Package)
	return c.JSON(http.StatusOK, StackResponse{
		OK:      true,
		Message: fmt.Sprintf("%s %s completed", req.Action, req.Package),
		Output:  output,
		Version: info.Version,
	})
}

// detectPackage dispatches to the right detector.
func (r *Router) detectPackage(name string) StackInfo {
	switch name {
	case "nginx":
		return r.detectNginx()
	case "php":
		return r.detectPHP()
	case "node":
		return r.detectNode()
	case "python":
		return r.detectPython()
	case "go":
		return r.detectGo()
	case "docker":
		return r.detectDocker()
	case "certbot":
		return r.detectCertbot()
	case "mariadb", "mysql":
		return r.detectMariaDB()
	case "redis":
		return r.detectRedis()
	case "memcached":
		return r.detectMemcached()
	case "supervisor":
		return r.detectSupervisor()
	default:
		return StackInfo{Name: name, Status: "unknown"}
	}
}

// --- Detect helpers ---

func (r *Router) detectNginx() StackInfo {
	out, err := exec.Command("nginx", "-v").CombinedOutput()
	if err != nil {
		return StackInfo{Name: "nginx", Status: "missing"}
	}
	ver := extractVersion(string(out))
	status := checkServiceStatus("nginx")
	path, _ := exec.LookPath("nginx")
	return StackInfo{Name: "nginx", Version: ver, Status: status, Path: path}
}

func (r *Router) detectPHP() StackInfo {
	// Detect all installed PHP versions
	var installedVersions []string
	for _, ver := range []string{"8.5", "8.4", "8.3", "8.2", "8.1", "8.0", "7.4", "7.3", "7.2", "7.1", "7.0", "5.6"} {
		bin := "php" + ver
		if _, err := exec.LookPath(bin); err == nil {
			installedVersions = append(installedVersions, ver)
		}
	}

	if len(installedVersions) == 0 {
		// Try plain "php"
		out, err := exec.Command("php", "-v").CombinedOutput()
		if err != nil {
			return StackInfo{Name: "php", Status: "missing"}
		}
		ver := extractVersion(string(out))
		path, _ := exec.LookPath("php")
		return StackInfo{Name: "php", Version: ver, Status: "installed", Path: path}
	}

	// Find the default (active) PHP version
	defaultVer := installedVersions[len(installedVersions)-1] // highest version
	defaultBin := "php" + defaultVer
	out, err := exec.Command(defaultBin, "-v").CombinedOutput()
	if err != nil {
		return StackInfo{Name: "php", Status: "missing"}
	}
	ver := extractVersion(string(out))

	// Check if any PHP-FPM is running
	status := "installed"
	for _, v := range installedVersions {
		svcName := "php" + v + "-fpm"
		if s := checkServiceStatus(svcName); s == "running" {
			status = "running"
			break
		}
		if s := checkServiceStatus(svcName); s == "stopped" {
			status = "stopped"
		}
	}

	path, _ := exec.LookPath(defaultBin)
	versions := strings.Join(installedVersions, ",")
	return StackInfo{
		Name:    "php",
		Version: ver + " (installed: " + versions + ")",
		Status:  status,
		Path:    path,
	}
}

func (r *Router) detectNode() StackInfo {
	out, err := exec.Command("node", "--version").CombinedOutput()
	if err != nil {
		return StackInfo{Name: "node", Status: "missing"}
	}
	ver := strings.TrimSpace(string(out))
	path, _ := exec.LookPath("node")
	npmVer := ""
	if npmOut, err := exec.Command("npm", "--version").Output(); err == nil {
		npmVer = " (npm " + strings.TrimSpace(string(npmOut)) + ")"
	}
	return StackInfo{Name: "node", Version: ver + npmVer, Status: "installed", Path: path}
}

func (r *Router) detectPython() StackInfo {
	for _, bin := range []string{"python3", "python"} {
		out, err := exec.Command(bin, "--version").CombinedOutput()
		if err == nil {
			ver := extractVersion(string(out))
			path, _ := exec.LookPath(bin)
			return StackInfo{Name: "python", Version: ver, Status: "installed", Path: path}
		}
	}
	return StackInfo{Name: "python", Status: "missing"}
}

func (r *Router) detectGo() StackInfo {
	out, err := exec.Command("go", "version").CombinedOutput()
	if err != nil {
		return StackInfo{Name: "go", Status: "missing"}
	}
	ver := "unknown"
	fields := strings.Fields(string(out))
	for _, f := range fields {
		if strings.HasPrefix(f, "go1.") || strings.HasPrefix(f, "go2.") {
			ver = strings.TrimPrefix(f, "go")
			break
		}
	}
	if ver == "unknown" {
		ver = extractVersion(string(out))
	}
	path, _ := exec.LookPath("go")
	return StackInfo{Name: "go", Version: ver, Status: "installed", Path: path}
}

func (r *Router) detectDocker() StackInfo {
	out, err := exec.Command("docker", "--version").CombinedOutput()
	if err != nil {
		return StackInfo{Name: "docker", Status: "missing"}
	}
	ver := extractVersion(string(out))
	status := checkServiceStatus("docker")
	path, _ := exec.LookPath("docker")
	return StackInfo{Name: "docker", Version: ver, Status: status, Path: path}
}

func (r *Router) detectCertbot() StackInfo {
	out, err := exec.Command("certbot", "--version").CombinedOutput()
	if err != nil {
		return StackInfo{Name: "certbot", Status: "missing"}
	}
	ver := extractVersion(string(out))
	path, _ := exec.LookPath("certbot")
	return StackInfo{Name: "certbot", Version: ver, Status: "installed", Path: path}
}

func (r *Router) detectMariaDB() StackInfo {
	for _, bin := range []string{"mysql", "mariadb"} {
		out, err := exec.Command(bin, "--version").CombinedOutput()
		if err == nil {
			ver := extractVersion(string(out))
			status := checkServiceStatus("mysql")
			if status == "missing" {
				status = checkServiceStatus("mariadb")
			}
			path, _ := exec.LookPath(bin)
			return StackInfo{Name: "mariadb", Version: ver, Status: status, Path: path}
		}
	}
	return StackInfo{Name: "mariadb", Status: "missing"}
}

func (r *Router) detectRedis() StackInfo {
	out, err := exec.Command("redis-server", "--version").CombinedOutput()
	if err != nil {
		return StackInfo{Name: "redis", Status: "missing"}
	}
	ver := extractVersion(string(out))
	status := checkServiceStatus("redis-server")
	path, _ := exec.LookPath("redis-server")
	return StackInfo{Name: "redis", Version: ver, Status: status, Path: path}
}

func (r *Router) detectSupervisor() StackInfo {
	out, err := exec.Command("supervisord", "--version").CombinedOutput()
	if err != nil {
		return StackInfo{Name: "supervisor", Status: "missing"}
	}
	ver := strings.TrimSpace(string(out))
	status := checkServiceStatus("supervisor")
	path, _ := exec.LookPath("supervisord")
	return StackInfo{Name: "supervisor", Version: ver, Status: status, Path: path}
}

// --- Manage helpers ---

func (r *Router) manageNginx(ctx context.Context, action, version, pm string) (string, error) {
	switch action {
	case "install", "update":
		return r.executor.RunShell(ctx, pm, "install", "-y", "nginx")
	case "uninstall":
		return r.executor.RunShell(ctx, pm, "remove", "-y", "nginx")
	case "restart":
		return r.executor.RunShell(ctx, "systemctl", "restart", "nginx")
	case "stop":
		return r.executor.RunShell(ctx, "systemctl", "stop", "nginx")
	case "start":
		return r.executor.RunShell(ctx, "systemctl", "start", "nginx")
	case "switch-version":
		if version == "" {
			return "", fmt.Errorf("version is required for switch-version")
		}
		return r.executor.RunShell(ctx, pm, "install", "-y", "nginx="+version)
	default:
		return "", fmt.Errorf("unknown action: %s", action)
	}
}

func (r *Router) managePHP(ctx context.Context, action, version, pm string) (string, error) {
	phpPkg := "php"
	if version != "" {
		phpPkg = "php" + version
	}

	switch action {
	case "install", "update":
		if pm == "apt" || pm == "apt-get" {
			r.executor.RunShell(ctx, "apt-get", "update", "-qq")
		}
		// Install each extension separately, skipping any that don't exist
		// (e.g. php8.5-opcache is built-in, not a separate package).
		candidates := []string{
			phpPkg + "-fpm", phpPkg + "-mysql", phpPkg + "-curl",
			phpPkg + "-gd", phpPkg + "-mbstring", phpPkg + "-xml",
			phpPkg + "-zip", phpPkg + "-bcmath", phpPkg + "-soap",
			phpPkg + "-intl", phpPkg + "-readline", phpPkg + "-cli",
			phpPkg + "-opcache",
		}
		missing := []string{}
		for _, pkg := range candidates {
			if _, err := exec.LookPath("apt-cache"); err == nil {
				if _, err := r.executor.RunShell(ctx, "apt-cache", "show", pkg); err != nil {
					missing = append(missing, pkg)
					continue
				}
			}
			if _, err := r.executor.RunShell(ctx, pm, "install", "-y", pkg); err != nil {
				missing = append(missing, pkg)
			}
		}
		if len(missing) > 0 {
			return fmt.Sprintf("installed (skipped missing: %s)", strings.Join(missing, ", ")), nil
		}
		return "all packages installed", nil
	case "uninstall":
		// apt-get does not expand globs: "remove -y php8.1*" either errors
		// (exit 100) or matches nothing. Enumerate installed packages for
		// the targeted version (or all php versions if version == "") and remove them.
		if pm == "apt" || pm == "apt-get" {
			pattern := "php[0-9.]"
			if version != "" {
				pattern = fmt.Sprintf("^php%s(-|$)", regexp.QuoteMeta(version))
			}
			cmd := fmt.Sprintf(`dpkg-query -W -f='${Package}\n' 'php*' 2>/dev/null | grep -E '%s' | sort -u | tr '\n' ' '`, pattern)
			out, err := r.executor.RunShell(ctx, "bash", "-c", cmd)
			if err == nil && strings.TrimSpace(out) != "" {
				return r.executor.RunShell(ctx, pm, append([]string{"remove", "-y"}, strings.Fields(out)...)...)
			}
			return fmt.Sprintf("no packages found for %s", phpPkg), nil
		}
		return r.executor.RunShell(ctx, pm, "remove", "-y", phpPkg)
	case "restart":
		return r.executor.RunShell(ctx, "systemctl", "restart", resolvePHPFPMService(phpPkg))
	case "stop":
		return r.executor.RunShell(ctx, "systemctl", "stop", resolvePHPFPMService(phpPkg))
	case "start":
		return r.executor.RunShell(ctx, "systemctl", "start", resolvePHPFPMService(phpPkg))
	case "switch-version":
		if version == "" {
			return "", fmt.Errorf("version is required")
		}
		newPkg := "php" + version + "-fpm"
		r.executor.RunShell(ctx, pm, "install", "-y", newPkg)
		r.executor.RunShell(ctx, "systemctl", "stop", phpPkg+"-fpm")
		return r.executor.RunShell(ctx, "systemctl", "start", newPkg)
	default:
		return "", fmt.Errorf("unknown action: %s", action)
	}
}

// resolvePHPFPMService returns the systemd service name for a php package.
// If the package is generic ("php"), picks the newest installed FPM version.
func resolvePHPFPMService(phpPkg string) string {
	if phpPkg != "php" {
		return phpPkg + "-fpm"
	}
	for _, v := range []string{"8.5", "8.4", "8.3", "8.2", "8.1", "8.0"} {
		if _, err := exec.LookPath("php-fpm" + v); err == nil {
			return "php" + v + "-fpm"
		}
	}
	return "php-fpm"
}

func (r *Router) manageNode(ctx context.Context, action, version, pm string) (string, error) {
	switch action {
	case "install", "update":
		if pm == "apt" || pm == "apt-get" {
			r.executor.RunShell(ctx, "bash", "-c", "curl -fsSL https://deb.nodesource.com/setup_lts.x | bash -")
			return r.executor.RunShell(ctx, "apt-get", "install", "-y", "nodejs")
		}
		return r.executor.RunShell(ctx, pm, "install", "-y", "nodejs", "npm")
	case "uninstall":
		return r.executor.RunShell(ctx, pm, "remove", "-y", "nodejs", "npm")
	case "switch-version":
		if version == "" {
			return "", fmt.Errorf("version is required")
		}
		return r.executor.RunShell(ctx, "sh", "-c",
			fmt.Sprintf("export NVM_DIR=\"$HOME/.nvm\" && [ -s \"$NVM_DIR/nvm.sh\" ] && . \"$NVM_DIR/nvm.sh\" && nvm install %s && nvm use %s", version, version))
	default:
		return "", fmt.Errorf("unknown action: %s", action)
	}
}

func (r *Router) managePython(ctx context.Context, action, version, pm string) (string, error) {
	switch action {
	case "install", "update":
		return r.executor.RunShell(ctx, pm, "install", "-y", "python3", "python3-pip", "python3-venv")
	case "uninstall":
		return r.executor.RunShell(ctx, pm, "remove", "-y", "python3", "python3-pip")
	default:
		return "", fmt.Errorf("unknown action: %s", action)
	}
}

func (r *Router) manageGo(ctx context.Context, action, version, pm string) (string, error) {
	switch action {
	case "install", "update":
		ver := version
		if ver == "" {
			ver = "1.24.0"
		}
		arch := "amd64"
		if runtime.GOARCH == "arm64" {
			arch = "arm64"
		}
		url := fmt.Sprintf("https://go.dev/dl/go%s.linux-%s.tar.gz", ver, arch)
		return r.executor.RunShell(ctx, "sh", "-c",
			fmt.Sprintf(`set -eu; curl -fsSL %s -o /tmp/go.tar.gz; rm -rf /usr/local/go; tar -C /usr/local -xzf /tmp/go.tar.gz; rm -f /tmp/go.tar.gz; ln -sfn /usr/local/go/bin/go /usr/local/bin/go; ln -sfn /usr/local/go/bin/gofmt /usr/local/bin/gofmt; printf '%%s\n' 'export PATH=/usr/local/go/bin:$PATH' 'export GOROOT=/usr/local/go' > /etc/profile.d/sigardapanel-go.sh; chmod 0644 /etc/profile.d/sigardapanel-go.sh`, url))
	case "uninstall":
		return r.executor.RunShell(ctx, "sh", "-c", "set -eu; rm -rf /usr/local/go; if [ \"$(readlink /usr/local/bin/go 2>/dev/null || true)\" = /usr/local/go/bin/go ]; then rm -f /usr/local/bin/go; fi; if [ \"$(readlink /usr/local/bin/gofmt 2>/dev/null || true)\" = /usr/local/go/bin/gofmt ]; then rm -f /usr/local/bin/gofmt; fi; rm -f /etc/profile.d/sigardapanel-go.sh")
	default:
		return "", fmt.Errorf("unknown action: %s", action)
	}
}

func (r *Router) manageDocker(ctx context.Context, action, version, pm string) (string, error) {
	switch action {
	case "install", "update":
		if pm == "apt" || pm == "apt-get" {
			r.executor.RunShell(ctx, "sh", "-c", "curl -fsSL https://get.docker.com | sh")
			r.executor.RunShell(ctx, "systemctl", "enable", "--now", "docker")
			return "docker installed and started", nil
		}
		return r.executor.RunShell(ctx, pm, "install", "-y", "docker", "docker-compose")
	case "uninstall":
		// Ubuntu/Debian ships docker as "docker.io" (binary /usr/bin/docker);
		// "docker" alone is a dummy/transitional package, so removing it did
		// nothing. Remove the real packages plus container runtime bits.
		pkgs := []string{
			"docker.io", "docker-ce", "docker-ce-cli", "containerd",
			"docker-compose", "docker-compose-v2", "docker-doc", "podman-docker",
		}
		return r.executor.RunShell(ctx, pm, append([]string{"remove", "-y"}, pkgs...)...)
	case "restart":
		return r.executor.RunShell(ctx, "systemctl", "restart", "docker")
	case "stop":
		return r.executor.RunShell(ctx, "systemctl", "stop", "docker")
	case "start":
		return r.executor.RunShell(ctx, "systemctl", "start", "docker")
	default:
		return "", fmt.Errorf("unknown action: %s", action)
	}
}

func (r *Router) manageCertbot(ctx context.Context, action, version, pm string) (string, error) {
	switch action {
	case "install", "update":
		if pm == "apt" || pm == "apt-get" {
			r.executor.RunShell(ctx, "apt-get", "install", "-y", "certbot", "python3-certbot-nginx")
		}
		return r.executor.RunShell(ctx, pm, "install", "-y", "certbot")
	case "uninstall":
		return r.executor.RunShell(ctx, pm, "remove", "-y", "certbot")
	default:
		return "", fmt.Errorf("unknown action: %s", action)
	}
}

func (r *Router) manageMariaDB(ctx context.Context, action, version, pm string) (string, error) {
	switch action {
	case "install", "update":
		return r.executor.RunShell(ctx, pm, "install", "-y", "mariadb-server", "mariadb-client")
	case "uninstall":
		return r.executor.RunShell(ctx, pm, "remove", "-y", "mariadb-server", "mariadb-client")
	case "restart":
		return r.executor.RunShell(ctx, "systemctl", "restart", "mariadb")
	case "stop":
		return r.executor.RunShell(ctx, "systemctl", "stop", "mariadb")
	case "start":
		return r.executor.RunShell(ctx, "systemctl", "start", "mariadb")
	default:
		return "", fmt.Errorf("unknown action: %s", action)
	}
}

func (r *Router) manageRedis(ctx context.Context, action, version, pm string) (string, error) {
	switch action {
	case "install", "update":
		return r.executor.RunShell(ctx, pm, "install", "-y", "redis-server")
	case "uninstall":
		if pm == "apt" || pm == "apt-get" {
			return r.executor.RunShell(ctx, pm, "remove", "-y", "redis-server", "redis-tools")
		}
		return r.executor.RunShell(ctx, pm, "remove", "-y", "redis-server")
	case "restart":
		return r.executor.RunShell(ctx, "systemctl", "restart", "redis-server")
	case "stop":
		return r.executor.RunShell(ctx, "systemctl", "stop", "redis-server")
	case "start":
		return r.executor.RunShell(ctx, "systemctl", "start", "redis-server")
	default:
		return "", fmt.Errorf("unknown action: %s", action)
	}
}

func (r *Router) detectMemcached() StackInfo {
	out, err := exec.Command("memcached", "-h").CombinedOutput()
	if err != nil {
		return StackInfo{Name: "memcached", Status: "missing"}
	}
	// memcached -h outputs version on first line, e.g. "memcached 1.6.18"
	ver := strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
	ver = strings.TrimPrefix(ver, "memcached ")
	status := checkServiceStatus("memcached")
	path, _ := exec.LookPath("memcached")
	return StackInfo{Name: "memcached", Version: ver, Status: status, Path: path}
}

func (r *Router) manageMemcached(ctx context.Context, action, version, pm string) (string, error) {
	switch action {
	case "install", "update":
		return r.executor.RunShell(ctx, pm, "install", "-y", "memcached")
	case "uninstall":
		return r.executor.RunShell(ctx, pm, "remove", "-y", "memcached")
	case "restart":
		return r.executor.RunShell(ctx, "systemctl", "restart", "memcached")
	case "stop":
		return r.executor.RunShell(ctx, "systemctl", "stop", "memcached")
	case "start":
		return r.executor.RunShell(ctx, "systemctl", "start", "memcached")
	default:
		return "", fmt.Errorf("unknown action: %s", action)
	}
}

func (r *Router) manageSupervisor(ctx context.Context, action, version, pm string) (string, error) {
	switch action {
	case "install", "update":
		return r.executor.RunShell(ctx, pm, "install", "-y", "supervisor")
	case "uninstall":
		return r.executor.RunShell(ctx, pm, "remove", "-y", "supervisor")
	case "restart":
		return r.executor.RunShell(ctx, "systemctl", "restart", "supervisor")
	case "stop":
		return r.executor.RunShell(ctx, "systemctl", "stop", "supervisor")
	case "start":
		return r.executor.RunShell(ctx, "systemctl", "start", "supervisor")
	default:
		return "", fmt.Errorf("unknown action: %s", action)
	}
}

// --- Utility ---

func detectPackageManager() string {
	if _, err := exec.LookPath("apt-get"); err == nil {
		return "apt-get"
	}
	if _, err := exec.LookPath("yum"); err == nil {
		return "yum"
	}
	if _, err := exec.LookPath("dnf"); err == nil {
		return "dnf"
	}
	if _, err := exec.LookPath("apk"); err == nil {
		return "apk"
	}
	return ""
}

func checkServiceStatus(name string) string {
	if out, err := exec.Command("systemctl", "is-active", name).Output(); err == nil {
		if strings.TrimSpace(string(out)) == "active" {
			return "running"
		}
		return "stopped"
	}
	if out, err := exec.Command("service", name, "status").Output(); err == nil {
		if strings.Contains(string(out), "running") {
			return "running"
		}
		return "stopped"
	}
	return "missing"
}

func extractVersion(output string) string {
	output = strings.TrimSpace(output)
	parts := strings.Fields(output)
	for _, p := range parts {
		if strings.HasPrefix(p, "v=") {
			return strings.TrimPrefix(p, "v=")
		}
		p = strings.TrimPrefix(p, "v")
		p = strings.TrimPrefix(p, "V")
		p = strings.TrimSuffix(p, ",")
		if len(p) >= 3 && p[0] >= '0' && p[0] <= '9' {
			return p
		}
		if idx := strings.IndexByte(p, '/'); idx >= 0 {
			ver := p[idx+1:]
			ver = strings.TrimSuffix(ver, ",")
			if len(ver) >= 3 {
				return ver
			}
		}
	}
	if len(parts) > 0 {
		return strings.TrimSuffix(parts[len(parts)-1], ",")
	}
	return "unknown"
}

type packageVersionItem struct {
	Version   string `json:"version"`
	Status    string `json:"status,omitempty"` // running, stopped, installed, missing
	Path      string `json:"path,omitempty"`
	Installed bool   `json:"installed"`
}

func (r *Router) listPackageVersions(c echo.Context, pkg string) error {
	switch pkg {
	case "php":
		return r.listPHPVersions(c)
	case "go":
		info := r.detectGo()
		var versions []packageVersionItem
		if info.Status != "missing" && info.Version != "" && info.Version != "unknown" {
			versions = append(versions, packageVersionItem{
				Version:   info.Version,
				Status:    info.Status,
				Path:      info.Path,
				Installed: true,
			})
		}
		return c.JSON(http.StatusOK, httpjson.Success{
			Data: map[string]any{"versions": versions, "count": len(versions)},
		})
	case "node":
		info := r.detectNode()
		var versions []packageVersionItem
		if info.Status != "missing" && info.Version != "" {
			versions = append(versions, packageVersionItem{
				Version:   info.Version,
				Status:    info.Status,
				Path:      info.Path,
				Installed: true,
			})
		}
		return c.JSON(http.StatusOK, httpjson.Success{
			Data: map[string]any{"versions": versions, "count": len(versions)},
		})
	default:
		info := r.detectPackage(pkg)
		var versions []packageVersionItem
		if info.Status != "missing" && info.Version != "" {
			versions = append(versions, packageVersionItem{
				Version:   info.Version,
				Status:    info.Status,
				Path:      info.Path,
				Installed: true,
			})
		}
		return c.JSON(http.StatusOK, httpjson.Success{
			Data: map[string]any{"versions": versions, "count": len(versions)},
		})
	}
}

func (r *Router) listPackageAvailableVersions(c echo.Context, pkg string) error {
	switch pkg {
	case "php":
		return r.listAvailablePHPVersions(c)
	case "go":
		known := []string{"1.24.0", "1.23.6", "1.22.12", "1.22.4", "1.21.13"}
		info := r.detectGo()
		var versions []packageVersionItem
		for _, v := range known {
			versions = append(versions, packageVersionItem{
				Version:   v,
				Installed: info.Version == v,
			})
		}
		return c.JSON(http.StatusOK, httpjson.Success{
			Data: map[string]any{"versions": versions, "count": len(versions)},
		})
	case "node":
		known := []string{"22", "20", "18"}
		info := r.detectNode()
		var versions []packageVersionItem
		for _, v := range known {
			inst := strings.HasPrefix(info.Version, "v"+v) || strings.HasPrefix(info.Version, v)
			versions = append(versions, packageVersionItem{
				Version:   v,
				Installed: inst,
			})
		}
		return c.JSON(http.StatusOK, httpjson.Success{
			Data: map[string]any{"versions": versions, "count": len(versions)},
		})
	default:
		info := r.detectPackage(pkg)
		var versions []packageVersionItem
		if info.Version != "" {
			versions = append(versions, packageVersionItem{
				Version:   info.Version,
				Installed: info.Status != "missing",
			})
		}
		return c.JSON(http.StatusOK, httpjson.Success{
			Data: map[string]any{"versions": versions, "count": len(versions)},
		})
	}
}

// listPHPVersions returns all installed PHP versions
func (r *Router) listPHPVersions(c echo.Context) error {
	type phpVersion struct {
		Version string `json:"version"`
		Status  string `json:"status"` // running, stopped, installed
		Path    string `json:"path"`
	}

	var versions []phpVersion
	for _, ver := range []string{"8.5", "8.4", "8.3", "8.2", "8.1", "8.0", "7.4", "7.3", "7.2", "7.1", "7.0", "5.6"} {
		bin := "php" + ver
		if _, err := exec.LookPath(bin); err == nil {
			svcName := "php" + ver + "-fpm"
			status := checkServiceStatus(svcName)
			if status == "missing" {
				status = "installed"
			}
			path, _ := exec.LookPath(bin)
			versions = append(versions, phpVersion{
				Version: ver,
				Status:  status,
				Path:    path,
			})
		}
	}

	return c.JSON(http.StatusOK, httpjson.Success{
		Data: map[string]any{
			"versions": versions,
			"count":    len(versions),
		},
		RequestID: "",
	})
}

// listAvailablePHPVersions returns PHP versions available for installation
func (r *Router) listAvailablePHPVersions(c echo.Context) error {
	type availableVersion struct {
		Version   string `json:"version"`
		Installed bool   `json:"installed"`
	}

	// Known PHP versions that can be installed via ondrej/php PPA
	knownVersions := []string{"8.5", "8.4", "8.3", "8.2", "8.1", "8.0", "7.4", "7.3", "7.2", "7.1", "7.0", "5.6"}

	var versions []availableVersion
	for _, ver := range knownVersions {
		bin := "php" + ver
		installed := false
		if _, err := exec.LookPath(bin); err == nil {
			installed = true
		}
		versions = append(versions, availableVersion{
			Version:   ver,
			Installed: installed,
		})
	}

	return c.JSON(http.StatusOK, httpjson.Success{
		Data: map[string]any{
			"versions": versions,
			"count":    len(versions),
		},
		RequestID: "",
	})
}
