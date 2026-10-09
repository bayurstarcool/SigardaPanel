package agenthttp

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"sigardapanel/internal/config"
	"sigardapanel/internal/executor"
	"sigardapanel/internal/httpjson"
	"sigardapanel/internal/nginx"
	"sigardapanel/internal/siteinput"
	"sigardapanel/internal/store"
)

// isPortAvailable checks if a TCP port is available on the host.
func isPortAvailable(port string) bool {
	ln, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return false
	}
	ln.Close()
	return true
}

const version = "0.1.0-dev"

// Router holds agent HTTP handlers and dependencies.
type Router struct {
	executor     *executor.Executor
	nginxConfDir string
	logDir       string
	backupDir    string
	authToken    string
	siteRootDir  string
	allowStubSSL bool
	stubDev      bool
	testAuth     bool
}

// RouterOptions allows overriding router defaults for testing.
type RouterOptions struct {
	NginxConfDir string
	LogDir       string
	BackupDir    string
	AuthToken    string
	SiteRootDir  string
	AllowStubSSL bool
	StubDev      bool
	Executor     *executor.Executor
}

// NewRouter returns a new Echo router for the agent with production defaults.
func NewRouter() *echo.Echo {
	return newRouter(nil)
}

// NewRouterWithOptions returns a new Echo router with configurable paths and dependencies.
func NewRouterWithOptions(opts RouterOptions) *echo.Echo {
	return newRouter(&opts)
}

func newRouter(opts *RouterOptions) *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true

	r := &Router{
		executor:     &executor.Executor{Timeout: 30 * time.Minute},
		nginxConfDir: "/etc/nginx/sites-enabled",
		logDir:       "/var/log/nginx",
		backupDir:    "/var/backups/sigardapanel",
		authToken:    config.AgentToken(),
		siteRootDir:  config.AgentSiteRoot(),
		allowStubSSL: config.AgentAllowStubSSL(),
		stubDev:      os.Getenv("SIGARDAPANEL_DEV") == "true",
	}
	if opts != nil {
		// RouterOptions is test/dependency-injection surface. Keep production
		// NewRouter strict while allowing isolated handler tests to call tasks
		// without transport auth setup.
		r.testAuth = true
		if opts.AuthToken == "" {
			r.authToken = "test-agent-token"
		}
		if opts.NginxConfDir != "" {
			r.nginxConfDir = opts.NginxConfDir
		}
		if opts.LogDir != "" {
			r.logDir = opts.LogDir
		}
		if opts.BackupDir != "" {
			r.backupDir = opts.BackupDir
		}
		if opts.AuthToken != "" {
			r.authToken = opts.AuthToken
		}
		if opts.SiteRootDir != "" {
			r.siteRootDir = opts.SiteRootDir
		}
		if opts.AllowStubSSL {
			r.allowStubSSL = true
		}
		if opts.Executor != nil {
			r.executor = opts.Executor
		}
	}

	e.GET("/health", healthHandler)
	e.GET("/metrics", r.metricsHandler)
	e.GET("/capabilities", capabilitiesHandler)

	// WebSocket terminal endpoint (auth via query param or header)
	e.GET("/ws/terminal", r.terminalWebSocket, r.requireAgentAuth)

	tasks := e.Group("/tasks", r.requireAgentAuth)
	tasks.POST("/site/create", r.siteCreateTask)
	tasks.POST("/site/delete", r.siteDeleteTask)
	tasks.POST("/site/deploy", r.siteDeployTask)
	tasks.POST("/nginx/reload", r.nginxReloadTask)
	tasks.POST("/ssl/issue", r.sslIssueTask)
	tasks.POST("/ssl/renew", r.sslRenewTask)
	tasks.POST("/logs/read", r.logsReadTask)
	tasks.POST("/backup/create", r.backupCreateTask)
	tasks.POST("/backup/restore", r.backupRestoreTask)
	tasks.POST("/backup/database", r.databaseBackupTask)
	tasks.POST("/backup/delete", r.backupDeleteTask)
	tasks.POST("/database/create", r.databaseCreateTask)
	tasks.POST("/database/delete", r.databaseDeleteTask)
	tasks.POST("/dbuser/create", r.dbUserCreateTask)
	tasks.POST("/dbuser/delete", r.dbUserDeleteTask)
	tasks.POST("/app/start", r.appStartTask)
	tasks.POST("/app/stop", r.appStopTask)
	tasks.POST("/app/restart", r.appRestartTask)
	tasks.POST("/command/exec", r.commandExecTask)

	// System specifications and benchmarks
	tasks.GET("/system/spec", r.systemSpecTask)
	tasks.GET("/system/benchmark", r.systemBenchmarkTask)

	// Stack management (install/manage software on the server)
	tasks.GET("/stack/info", r.stackInfoTask)
	tasks.POST("/stack/install", r.stackInstallTask)

	// Application templates (WordPress, Laravel, etc.)
	tasks.POST("/templates/wordpress", r.installWordPress)
	tasks.POST("/templates/laravel", r.installLaravel)

	// PM2 process manager
	tasks.POST("/pm2/manage", r.managePM2)

	// Email management (Postfix + Dovecot)
	tasks.POST("/email/manage", r.manageEmail)

	// Python app management (Gunicorn/WSGI)
	tasks.POST("/python/manage", r.managePythonApp)

	// Bot blocker management
	tasks.POST("/bot-blocker/manage", r.manageBotBlocker)

	// Fail2ban management
	tasks.GET("/fail2ban/status", r.fail2banStatus)
	tasks.GET("/security/overview", r.securityOverviewTask)
	tasks.GET("/fail2ban/jail", r.fail2banJail)
	tasks.POST("/fail2ban/unban", r.fail2banUnban)
	tasks.POST("/fail2ban/ban", r.fail2banBan)
	tasks.POST("/fail2ban/enable", r.fail2banEnable)
	tasks.POST("/fail2ban/disable", r.fail2banDisable)

	// Docker container management
	tasks.GET("/docker/containers", r.dockerContainersTask)
	tasks.POST("/docker/container/start", r.dockerContainerStartTask)
	tasks.POST("/docker/container/stop", r.dockerContainerStopTask)
	tasks.POST("/docker/container/restart", r.dockerContainerRestartTask)
	tasks.POST("/docker/container/remove", r.dockerContainerRemoveTask)
	tasks.POST("/docker/container/create", r.dockerContainerCreateTask)
	tasks.GET("/docker/container/logs", r.dockerContainerLogsTask)
	tasks.POST("/docker/container/exec", r.dockerContainerExecTask)
	tasks.GET("/docker/images", r.dockerImagesTask)
	tasks.POST("/docker/image/pull", r.dockerImagePullTask)
	tasks.POST("/docker/image/remove", r.dockerImageRemoveTask)
	tasks.GET("/docker/volumes", r.dockerVolumesTask)
	tasks.POST("/docker/volume/create", r.dockerVolumeCreateTask)
	tasks.POST("/docker/volume/remove", r.dockerVolumeRemoveTask)
	tasks.GET("/docker/networks", r.dockerNetworksTask)
	tasks.POST("/docker/network/create", r.dockerNetworkCreateTask)
	tasks.POST("/docker/network/remove", r.dockerNetworkRemoveTask)
	tasks.GET("/docker/info", r.dockerInfoTask)
	tasks.GET("/docker/disk-usage", r.dockerDiskUsageTask)
	tasks.POST("/docker/compose/up", r.dockerComposeUpTask)
	tasks.POST("/docker/compose/down", r.dockerComposeDownTask)

	// Git deploy v2
	tasks.GET("/git/branches", r.gitBranchesTask)
	tasks.GET("/git/log", r.gitLogTask)
	tasks.POST("/git/rollback", r.gitRollbackTask)
	tasks.POST("/git/checkout", r.gitCheckoutTask)

	// Firewall (UFW) management
	tasks.GET("/firewall/status", r.firewallStatusTask)
	tasks.POST("/firewall/enable", r.firewallEnableTask)
	tasks.POST("/firewall/disable", r.firewallDisableTask)
	tasks.POST("/firewall/allow", r.firewallAllowTask)
	tasks.POST("/firewall/deny", r.firewallDenyTask)
	tasks.POST("/firewall/delete", r.firewallDeleteTask)
	tasks.GET("/firewall/rules", r.firewallRulesTask)
	tasks.POST("/firewall/reset", r.firewallResetTask)

	// Redis management
	tasks.GET("/redis/stats", r.redisStatsTask)
	tasks.POST("/redis/flush", r.redisFlushTask)
	tasks.GET("/redis/info", r.redisInfoTask)
	tasks.POST("/redis/flushdb", r.redisFlushDBTask)

	return e
}

func (r *Router) requireAgentAuth(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if r.testAuth {
			return next(c)
		}
		if r.authToken == "" {
			slog.Error("agent task endpoint blocked: SIGARDAPANEL_AGENT_TOKEN not configured; set the token before starting the agent")
			return echo.NewHTTPError(http.StatusUnauthorized, "agent token not configured")
		}
		const prefix = "Bearer "
		header := c.Request().Header.Get("Authorization")
		if !strings.HasPrefix(header, prefix) {
			return echo.NewHTTPError(http.StatusUnauthorized, "missing agent token")
		}
		given := strings.TrimPrefix(header, prefix)
		if subtle.ConstantTimeCompare([]byte(given), []byte(r.authToken)) != 1 {
			return echo.NewHTTPError(http.StatusUnauthorized, "invalid agent token")
		}
		return next(c)
	}
}

func healthHandler(c echo.Context) error {
	return c.JSON(http.StatusOK, httpjson.Success{
		Data: map[string]any{
			"status":  "ok",
			"service": "sigardapanel-agent",
			"version": version,
			"time":    time.Now().UTC().Format(time.RFC3339),
		},
		RequestID: "",
	})
}

func (r *Router) metricsHandler(c echo.Context) error {
	var m struct {
		CPUPercent  float64 `json:"cpu_percent"`
		MemUsed     int64   `json:"mem_used"`
		MemTotal    int64   `json:"mem_total"`
		DiskUsed    int64   `json:"disk_used"`
		DiskTotal   int64   `json:"disk_total"`
		SwapUsed    int64   `json:"swap_used"`
		SwapTotal   int64   `json:"swap_total"`
		NetInBytes  int64   `json:"net_in_bytes"`
		NetOutBytes int64   `json:"net_out_bytes"`
		GPUName     string  `json:"gpu_name,omitempty"`
		GPUCount    int     `json:"gpu_count,omitempty"`
		GPUUtil     float64 `json:"gpu_utilization,omitempty"`
		GPUMemUsed  int64   `json:"gpu_memory_used,omitempty"`
		GPUMemTotal int64   `json:"gpu_memory_total,omitempty"`
		GPUTemp     float64 `json:"gpu_temperature,omitempty"`
		GPUPower    float64 `json:"gpu_power_draw,omitempty"`
		GPUFanSpeed float64 `json:"gpu_fan_speed,omitempty"`
	}
	m.CPUPercent = readCPU()
	m.MemUsed, m.MemTotal = readMem()
	m.DiskUsed, m.DiskTotal = readDisk()
	m.SwapUsed, m.SwapTotal = readSwap()
	m.NetInBytes, m.NetOutBytes = readNet()
	gpu := readGPU()
	if gpu != nil {
		m.GPUName = gpu.Name
		m.GPUCount = gpu.Count
		m.GPUUtil = gpu.Utilization
		m.GPUMemUsed = gpu.MemoryUsed
		m.GPUMemTotal = gpu.MemoryTotal
		m.GPUTemp = gpu.Temperature
		m.GPUPower = gpu.PowerDraw
		m.GPUFanSpeed = gpu.FanSpeed
	}
	return c.JSON(http.StatusOK, httpjson.Success{Data: m, RequestID: ""})
}

func capabilitiesHandler(c echo.Context) error {
	// Auto-detect installed software
	nginx := false
	php := false
	node := false
	python := false
	docker := false
	systemd := false
	gpu := false

	if _, err := exec.LookPath("nvidia-smi"); err == nil {
		gpu = true
	}

	if _, err := exec.LookPath("nginx"); err == nil {
		nginx = true
	}
	if _, err := exec.LookPath("php"); err == nil {
		php = true
	} else if _, err := exec.LookPath("php8.4"); err == nil {
		php = true
	}
	if _, err := exec.LookPath("node"); err == nil {
		node = true
	}
	if _, err := exec.LookPath("python3"); err == nil {
		python = true
	} else if _, err := exec.LookPath("python"); err == nil {
		python = true
	}
	if _, err := exec.LookPath("docker"); err == nil {
		docker = true
	}
	if _, err := exec.LookPath("systemctl"); err == nil {
		systemd = true
	}

	return c.JSON(http.StatusOK, httpjson.Success{
		Data: map[string]any{
			"agent_version": version,
			"os":            runtime.GOOS,
			"arch":          runtime.GOARCH,
			"capabilities": map[string]bool{
				"systemd": systemd,
				"nginx":   nginx,
				"caddy":   false,
				"node":    node,
				"go":      true,
				"php":     php,
				"python":  python,
				"docker":  docker,
				"gpu":     gpu,
			},
		},
		RequestID: "",
	})
}

func (r *Router) siteCreateTask(c echo.Context) error {
	ctx := c.Request().Context()
	var payload struct {
		SiteID         int64  `json:"site_id"`
		Name           string `json:"name"`
		Domain         string `json:"domain"`
		Runtime        string `json:"runtime"`
		RootPath       string `json:"root_path"`
		Port           int    `json:"port,omitempty"`
		ProxyPassURL   string `json:"proxy_pass_url,omitempty"`
		PHPVersion     string `json:"php_version,omitempty"`
		PHPIni         string `json:"php_ini,omitempty"`
		DockerImage    string `json:"docker_image,omitempty"`
		DockerLanguage string `json:"docker_language,omitempty"`
		DockerName     string `json:"docker_name,omitempty"`
		DockerPorts    string `json:"docker_ports,omitempty"`
		DockerEnv      string `json:"docker_env,omitempty"`
		DockerVolume   string `json:"docker_volume,omitempty"`
		DockerRestart  string `json:"docker_restart,omitempty"`
	}
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid payload")
	}

	if !isValidDomain(payload.Domain) || !r.isAllowedSitePath(payload.RootPath) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid domain or path")
	}

	// 1. Create directory
	if err := r.executor.Mkdir(ctx, payload.RootPath); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "mkdir failed: "+err.Error())
	}

	// 2. Write runtime starter template (skip if site already has content — don't overwrite user code)
	if templates := nginx.TemplateForRuntime(payload.Runtime, payload.Domain, payload.Port); templates != nil {
		for fname, content := range templates {
			fpath := filepath.Join(payload.RootPath, fname)
			if _, err := r.executor.RunShell(ctx, "test", "-f", fpath); err != nil {
				r.executor.WriteFile(ctx, fpath, content, 0644)
			}
		}
	} else {
		// static/proxy: write placeholder index.html
		if _, err := r.executor.RunShell(ctx, "test", "-f", filepath.Join(payload.RootPath, "index.html")); err != nil {
			indexHTML := "<h1>" + payload.Domain + "</h1><p>Site created via SigardaPanel</p>"
			r.executor.WriteFile(ctx, filepath.Join(payload.RootPath, "index.html"), indexHTML, 0644)
		}
	}

	// ─── Docker runtime: pull image + create container ───
	if payload.Runtime == "docker" {
		// Default image if not specified
		if payload.DockerImage == "" {
			payload.DockerImage = "nginx:alpine"
		}
		// The agent runs docker as root: never trust the API for image names,
		// host bind mounts, port mappings or restart flags.
		if err := siteinput.DockerOptions(payload.DockerImage, payload.DockerName, payload.DockerPorts,
			payload.DockerEnv, payload.DockerVolume, payload.DockerRestart, payload.DockerLanguage, payload.RootPath); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}

		// Detect language from image or explicit docker_language field
		dockerLang := payload.DockerLanguage
		if dockerLang == "" {
			dockerLang = detectDockerLanguage(payload.DockerImage)
		}

		// Generate hello world files — skip for bitnami images (they handle project creation)
		isBitnami := strings.Contains(strings.ToLower(payload.DockerImage), "bitnami")
		if !isBitnami {
			if helloFiles := dockerHelloWorld(dockerLang, payload.Domain); helloFiles != nil {
				r.executor.Mkdir(ctx, payload.RootPath)
				for fname, content := range helloFiles {
					fpath := filepath.Join(payload.RootPath, fname)
					if _, err := r.executor.RunShell(ctx, "test", "-f", fpath); err != nil {
						r.executor.WriteFile(ctx, fpath, content, 0644)
					}
				}
			}
		}

		containerName := payload.DockerName
		if containerName == "" {
			containerName = "site-" + strings.Map(func(r rune) rune {
				if r == ' ' {
					return '-'
				}
				return r
			}, payload.Name)
		}
		if err := siteinput.DockerName(containerName); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}

		// Pull image
		slog.Info("agent: docker site pull", "image", payload.DockerImage, "domain", payload.Domain, "language", dockerLang)
		pullOut, pullErr := r.executor.RunShell(ctx, "docker", "pull", payload.DockerImage)
		if pullErr != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "docker pull failed: "+strings.TrimSpace(pullOut))
		}

		// Remove existing container if any
		r.executor.RunShell(ctx, "docker", "rm", "-f", containerName)

		// Build docker run args
		args := []string{"run", "-d", "--name", containerName, "--security-opt", "no-new-privileges"}
		if payload.DockerRestart != "" && payload.DockerRestart != "no" {
			args = append(args, "--restart", payload.DockerRestart)
		} else {
			args = append(args, "--restart", "unless-stopped")
		}

		// Determine container port (different images expose different ports)
		containerPort := dockerContainerPort(dockerLang, payload.DockerImage)

		// Port mapping: use container port mapped to a host port
		hostPort := fmt.Sprintf("%d:%d", 10000+payload.SiteID%50000, containerPort)
		if payload.DockerPorts != "" {
			// Validate requested port is available
			requestedHostPort := strings.Split(payload.DockerPorts, ":")[0]
			if isPortAvailable(requestedHostPort) {
				args = append(args, "-p", payload.DockerPorts)
				hostPort = payload.DockerPorts
			} else {
				slog.Warn("agent: requested port unavailable, using auto-assigned", "requested", requestedHostPort, "auto", hostPort)
				args = append(args, "-p", hostPort)
			}
		} else {
			// Auto-assign: find next available port
			for i := 0; i < 100; i++ {
				port := fmt.Sprintf("%d", 10000+((int(payload.SiteID)+i)%50000))
				if isPortAvailable(port) {
					hostPort = fmt.Sprintf("%s:%d", port, containerPort)
					break
				}
			}
			args = append(args, "-p", hostPort)
		}

		// Volumes — auto-mount site root into container
		siteRoot := payload.RootPath
		if siteRoot != "" {
			r.executor.Mkdir(ctx, siteRoot)
			containerWebRoot := dockerMountTarget(dockerLang, payload.DockerImage)
			args = append(args, "-v", siteRoot+":"+containerWebRoot)
		}

		// Additional user-specified volumes
		if payload.DockerVolume != "" {
			for _, v := range strings.Split(payload.DockerVolume, ",") {
				v = strings.TrimSpace(v)
				if v != "" {
					args = append(args, "-v", v)
				}
			}
		}

		// Environment variables
		if payload.DockerEnv != "" {
			for _, e := range strings.Split(payload.DockerEnv, "\n") {
				e = strings.TrimSpace(e)
				if e != "" {
					args = append(args, "-e", e)
				}
			}
		}

		// Working directory inside container for app runtimes
		if dockerLang != "static" && dockerLang != "" {
			workDir := dockerWorkDir(dockerLang)
			args = append(args, "-w", workDir)
		}

		args = append(args, payload.DockerImage)

		// For app runtimes, append the default command if no custom command in env
		if cmd := dockerDefaultCmd(dockerLang); cmd != "" {
			// Override CMD unless user specified a custom command in env (look for CMD= or ENTRYPOINT=)
			hasCustomCmd := strings.Contains(payload.DockerEnv, "CMD=") || strings.Contains(payload.DockerEnv, "ENTRYPOINT=")
			if !hasCustomCmd {
				args = append(args, "/bin/sh", "-c", cmd)
			}
		}

		createOut, createErr := r.executor.RunShell(ctx, "docker", args...)
		if createErr != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "docker create failed: "+strings.TrimSpace(createOut))
		}
		slog.Info("agent: docker container created", "name", containerName, "image", payload.DockerImage, "language", dockerLang)

		// Set proxy_pass to the container's host port
		if payload.ProxyPassURL == "" {
			payload.ProxyPassURL = "http://127.0.0.1:" + strings.Split(hostPort, ":")[0]
		}
	}

	// 3. Generate & write nginx config (always regenerate — supports port/command changes)
	site := &store.Site{
		Domain:       payload.Domain,
		Runtime:      payload.Runtime,
		RootPath:     payload.RootPath,
		ProxyPassURL: sql.NullString{String: payload.ProxyPassURL, Valid: payload.ProxyPassURL != ""},
		PHPVersion:   sql.NullString{String: payload.PHPVersion, Valid: payload.PHPVersion != ""},
		PHPIni:       sql.NullString{String: payload.PHPIni, Valid: payload.PHPIni != ""},
	}
	config, err := nginx.GenerateVhost(site, payload.Port, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "config gen failed: "+err.Error())
	}

	nginxConfigPath := filepath.Join(r.nginxConfDir, payload.Domain+".conf")
	if err := r.executor.WriteFile(ctx, nginxConfigPath, config, 0644); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "write nginx config failed: "+err.Error())
	}

	// Regenerate wildcard SSL cert to include this domain
	if _, ok := r.executor.RunShell(ctx, "test", "-f", "/etc/nginx/ssl/sigardapanel.crt"); ok == nil {
		r.executor.RunShell(ctx, "bash", "/root/SigardaPanel-Enterprise/scripts/regenerate-ssl.sh")
	}

	// 3b. logrotate: the distro default /etc/logrotate.d/nginx already rotates
	// /var/log/nginx/*.log. A per-domain file duplicates those log entries and
	// makes logrotate abort the WHOLE run ("duplicate log entry" → every
	// logrotate.service invocation FAILS). So only clean up legacy files here.
	logrotatePath := filepath.Join("/etc/logrotate.d", "sigarda_"+payload.Domain)
	r.executor.RunShell(ctx, "rm", "-f", logrotatePath)

	// 4. Test nginx config
	output, err := r.executor.RunShell(ctx, "nginx", "-t")
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "nginx test failed: "+strings.TrimSpace(output))
	}

	slog.Info("agent: site.create", "site_id", payload.SiteID, "domain", payload.Domain, "runtime", payload.Runtime)

	// 5. Reload nginx to pick up new config
	reloadOut, reloadErr := r.executor.RunShell(ctx, "nginx", "-s", "reload")
	if reloadErr != nil {
		slog.Warn("agent: nginx reload after site.create", "error", reloadErr, "output", strings.TrimSpace(reloadOut))
	}

	return c.JSON(http.StatusOK, map[string]any{
		"ok":                true,
		"message":           fmt.Sprintf("site %s created (runtime: %s)", payload.Domain, payload.Runtime),
		"root_path":         payload.RootPath,
		"nginx_config":      nginxConfigPath,
		"proxy_pass_url":    payload.ProxyPassURL,
		"nginx_test_output": strings.TrimSpace(output),
	})
}

// ─── Docker language helpers ───────────────────────────────────────

// detectDockerLanguage detects the programming language from a Docker image name.
func detectDockerLanguage(image string) string {
	img := strings.ToLower(image)
	switch {
	case strings.Contains(img, "nginx"):
		return "static"
	case strings.Contains(img, "node") || strings.Contains(img, "bun"):
		return "node"
	case strings.Contains(img, "python"):
		return "python"
	case strings.Contains(img, "php"):
		return "php"
	case strings.Contains(img, "golang") || strings.Contains(img, "go:"):
		return "go"
	case strings.Contains(img, "ruby"):
		return "ruby"
	case strings.Contains(img, "java") || strings.Contains(img, "temurin"):
		return "java"
	case strings.Contains(img, "rust"):
		return "rust"
	default:
		return "static"
	}
}

// dockerHelloWorld returns starter files for a Docker-based site.
func dockerHelloWorld(lang, domain string) map[string]string {
	switch lang {
	case "node":
		return map[string]string{
			"package.json": fmt.Sprintf(`{
  "name": %q,
  "version": "1.0.0",
  "scripts": { "start": "node server.js" }
}`, domain),
			"server.js": fmt.Sprintf(`const http = require('http');
const PORT = process.env.PORT || 3000;
const server = http.createServer((req, res) => {
  res.writeHead(200, { 'Content-Type': 'text/html' });
  res.end('<h1>%s</h1><p>Hello from Node.js 🚀</p><p>Runtime: Docker</p>');
});
server.listen(PORT, '0.0.0.0', () => console.log('Listening on port ' + PORT));
`, domain),
		}
	case "python":
		return map[string]string{
			"app.py": fmt.Sprintf(`from http.server import HTTPServer, SimpleHTTPRequestHandler
import os

PORT = int(os.environ.get('PORT', 8000))

class Handler(SimpleHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200)
        self.send_header('Content-type', 'text/html')
        self.end_headers()
        self.wfile.write(b'<h1>%s</h1><p>Hello from Python 🐍</p><p>Runtime: Docker</p>')

if __name__ == '__main__':
    os.chdir(os.path.dirname(os.path.abspath(__file__)))
    HTTPServer(('0.0.0.0', PORT), Handler).serve_forever()
`, domain),
			"requirements.txt": "",
		}
	case "php":
		return map[string]string{
			"index.php": fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><title>%s</title></head>
<body>
  <h1><?php echo '%s'; ?></h1>
  <p>Hello from PHP 🐘</p>
  <p>Runtime: Docker</p>
</body>
</html>
`, domain, domain),
		}
	case "go":
		return map[string]string{
			"main.go": fmt.Sprintf(`package main

import (
    "fmt"
    "net/http"
    "os"
)

func main() {
    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        fmt.Fprintf(w, "<h1>%s</h1><p>Hello from Go 🐹</p><p>Runtime: Docker</p>")
    })
    port := os.Getenv("PORT")
    if port == "" {
			port = "8080"
		}
    fmt.Println("Listening on port", port)
    http.ListenAndServe(":"+port, nil)
}
`, domain),
			"go.mod": fmt.Sprintf("module %s\n\ngo 1.22\n", strings.ReplaceAll(domain, ".", "-")),
		}
	case "ruby":
		return map[string]string{
			"app.rb": fmt.Sprintf(`require 'webrick'

server = WEBrick::HTTPServer.new(Port: ENV['PORT'] || 3000, BindAddress: '0.0.0.0')

server.mount_proc '/' do |req, res|
  res.body = '<h1>%s</h1><p>Hello from Ruby 💎</p><p>Runtime: Docker</p>'
end

trap('INT') { server.shutdown }
server.start
`, domain),
			"Gemfile": "source 'https://rubygems.org'\n",
		}
	case "java":
		return map[string]string{
			"Main.java": fmt.Sprintf(`import com.sun.net.httpserver.HttpServer;
import java.net.InetSocketAddress;

public class Main {
    public static void main(String[] args) throws Exception {
        int port = Integer.parseInt(System.getenv().getOrDefault("PORT", "8080"));
        HttpServer server = HttpServer.create(new InetSocketAddress(port), 0);
        server.createContext("/", exchange -> {
            String resp = "<h1>%s</h1><p>Hello from Java ☕</p><p>Runtime: Docker</p>";
            exchange.sendResponseHeaders(200, resp.getBytes().length);
            exchange.getResponseBody().write(resp.getBytes());
            exchange.getResponseBody().close();
        });
        System.out.println("Listening on port " + port);
        server.start();
    }
}
`, domain),
		}
	case "rust":
		return map[string]string{
			"main.rs": fmt.Sprintf(`use std::net::TcpListener;
use std::io::{Read, Write};

fn main() {
    let port = std::env::var("PORT").unwrap_or_else(|_| "8080".to_string());
    let listener = TcpListener::bind(format!("0.0.0.0:{}", port)).unwrap();
    println!("Listening on port {}", port);

    for stream in listener.incoming() {
        if let Ok(mut stream) = stream {
            let body = "<h1>%s</h1><p>Hello from Rust 🦀</p><p>Runtime: Docker</p>";
            let response = format!("HTTP/1.1 200 OK\r\nContent-Length: {}\r\n\r\n{}", body.len(), body);
            stream.write_all(response.as_bytes()).ok();
        }
    }
}
`, domain),
		}
	default: // static
		return map[string]string{
			"index.html": fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><title>%s</title></head>
<body>
  <h1>%s</h1>
  <p>Hello from Docker 🐳</p>
  <p>Runtime: Static</p>
</body>
</html>
`, domain, domain),
		}
	}
}

// dockerContainerPort returns the port the container listens on.
func dockerContainerPort(lang, image string) int {
	switch lang {
	case "node", "ruby":
		return 3000
	case "python":
		return 8000
	case "go", "java":
		return 8080
	case "rust":
		return 8080
	default:
		return 80 // nginx, php-fpm, etc.
	}
}

// dockerMountTarget returns the path inside the container where files are mounted.
func dockerMountTarget(lang, image string) string {
	img := strings.ToLower(image)
	switch {
	case strings.Contains(img, "nginx"):
		return "/usr/share/nginx/html"
	case lang == "php":
		return "/var/www/html"
	case lang == "node":
		return "/app"
	case lang == "python":
		return "/app"
	case lang == "go":
		return "/app"
	case lang == "ruby":
		return "/app"
	case lang == "java":
		return "/app"
	case lang == "rust":
		return "/app"
	default:
		return "/var/www/html"
	}
}

// dockerWorkDir returns the working directory for app runtimes.
func dockerWorkDir(lang string) string {
	switch lang {
	case "node", "python", "go", "ruby", "java", "rust":
		return "/app"
	default:
		return "/"
	}
}

// dockerDefaultCmd returns the default command to run inside the container.
func dockerDefaultCmd(lang string) string {
	switch lang {
	case "node":
		return "npm install && node server.js"
	case "python":
		return "pip install -r requirements.txt 2>/dev/null; python3 app.py"
	case "go":
		return "go run main.go"
	case "ruby":
		return "bundle install 2>/dev/null; ruby app.rb"
	case "java":
		return "javac Main.java && java Main"
	case "rust":
		return "rustc main.rs -o /app/main && /app/main"
	default:
		return ""
	}
}

func (r *Router) siteDeleteTask(c echo.Context) error {
	ctx := c.Request().Context()
	var payload struct {
		SiteID   int64  `json:"site_id"`
		Domain   string `json:"domain"`
		RootPath string `json:"root_path"`
	}
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid payload")
	}
	if !isValidDomain(payload.Domain) || !r.isAllowedSitePath(payload.RootPath) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid domain or path")
	}

	slog.Info("agent: site.delete", "site_id", payload.SiteID, "domain", payload.Domain)
	cleaned := []string{}
	errs := []string{}

	// 1. Remove systemd app service if exists
	svcName := appServiceName(payload.SiteID)
	if _, err := r.executor.RunShell(ctx, "systemctl", "stop", svcName); err == nil {
		r.executor.RunShell(ctx, "systemctl", "disable", svcName)
		r.executor.RunShell(ctx, "rm", "-f", "/etc/systemd/system/"+svcName+".service")
		r.executor.RunShell(ctx, "systemctl", "daemon-reload")
		cleaned = append(cleaned, "app service")
	}

	// 2. Remove nginx config
	nginxConfigPath := filepath.Join(r.nginxConfDir, payload.Domain+".conf")
	if err := r.removeGeneratedFile(ctx, nginxConfigPath, r.nginxConfDir); err != nil {
		errs = append(errs, "nginx config: "+err.Error())
	} else {
		cleaned = append(cleaned, "nginx config")
	}

	// 2b. Remove logrotate config
	logrotatePath := "/etc/logrotate.d/sigarda_" + payload.Domain
	if _, err := r.executor.RunShell(ctx, "rm", "-f", logrotatePath); err == nil {
		cleaned = append(cleaned, "logrotate config")
	}

	// 3. Remove SSL certificate via certbot
	if _, err := exec.LookPath("certbot"); err == nil {
		r.executor.RunShell(ctx, "certbot", "delete", "--non-interactive", "--cert-name", payload.Domain)
	}

	// 4. Remove site directory
	if err := r.executor.RemoveAll(ctx, payload.RootPath); err != nil {
		errs = append(errs, "site root: "+err.Error())
	} else {
		cleaned = append(cleaned, "site files")
	}

	// 5. Remove htpasswd if exists
	if _, err := r.executor.Stat(ctx, "/etc/nginx/.htpasswd_"+payload.Domain); err == nil {
		r.executor.RunShell(ctx, "rm", "-f", "/etc/nginx/.htpasswd_"+payload.Domain)
		cleaned = append(cleaned, "htpasswd")
	}

	// 6. Remove nginx logs
	logAccess := filepath.Join(r.logDir, payload.Domain+".access.log")
	logError := filepath.Join(r.logDir, payload.Domain+".error.log")
	r.executor.RunShell(ctx, "rm", "-f", logAccess)
	r.executor.RunShell(ctx, "rm", "-f", logError)

	// 7. Remove backup directory if exists
	backupPath := filepath.Join(r.backupDir, "sites", sanitizeBackupName(payload.Domain))
	r.executor.RemoveAll(ctx, backupPath)

	// 8. Remove ACME webroot challenge dir
	r.executor.RemoveAll(ctx, "/var/www/certbot/"+payload.Domain)

	// 9. Test & reload nginx
	if output, err := r.executor.RunShell(ctx, "nginx", "-t"); err != nil {
		errs = append(errs, "nginx test: "+strings.TrimSpace(output))
	}
	if output, err := r.executor.RunShell(ctx, "systemctl", "reload", "nginx"); err != nil {
		slog.Warn("agent: nginx reload failed after site.delete", "site_id", payload.SiteID, "error", err, "output", strings.TrimSpace(output))
	}

	message := fmt.Sprintf("site %s deleted. Cleaned up: %s", payload.Domain, strings.Join(cleaned, ", "))
	if len(errs) > 0 {
		message += ". Errors: " + strings.Join(errs, "; ")
	}

	slog.Info("agent: site.delete complete", "site_id", payload.SiteID, "domain", payload.Domain)
	return c.JSON(http.StatusOK, map[string]any{
		"ok":      true,
		"message": message,
		"cleaned": cleaned,
		"errors":  errs,
	})
}

func (r *Router) siteDeployTask(c echo.Context) error {
	ctx := c.Request().Context()
	var payload struct {
		SiteID       int64  `json:"site_id"`
		Domain       string `json:"domain"`
		Repo         string `json:"repo"`
		Branch       string `json:"branch"`
		RootPath     string `json:"root_path"`
		Runtime      string `json:"runtime"`
		AppStatus    string `json:"app_status"`
		AppPort      int64  `json:"app_port"`
		AppCommand   string `json:"app_command"`
		SystemUser   string `json:"system_user"`
		AppMemoryMax int64  `json:"app_memory_max"`
	}
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid payload")
	}

	if !isValidGitURL(payload.Repo) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid repo URL")
	}
	if payload.Branch != "" && !isValidGitRef(payload.Branch) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid branch name")
	}
	if !r.isAllowedSitePath(payload.RootPath) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid root_path")
	}

	exists, _ := r.executor.Stat(ctx, payload.RootPath)
	isGitRepo := false
	if exists {
		gitDir := filepath.Join(payload.RootPath, ".git")
		isGitRepo, _ = r.executor.Stat(ctx, gitDir)
	}
	if !exists || !isGitRepo {
		output, err := r.executor.RunShell(ctx, "git", "clone", "-b", payload.Branch, payload.Repo, payload.RootPath)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "git clone failed: "+strings.TrimSpace(output))
		}
	} else {
		output, err := r.executor.RunShell(ctx, "git", "-C", payload.RootPath, "pull", "origin", payload.Branch)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "git pull failed: "+strings.TrimSpace(output))
		}
	}

	commitOut, _ := r.executor.RunShell(ctx, "git", "-C", payload.RootPath, "rev-parse", "HEAD")
	commit := strings.TrimSpace(commitOut)

	// ─── Step 2: Install dependencies based on runtime ───
	var installOutput string
	switch payload.Runtime {
	case "node":
		if _, err := r.executor.Stat(ctx, filepath.Join(payload.RootPath, "package.json")); err == nil {
			out, err := r.executor.RunShellDir(ctx, payload.RootPath, "npm", "install", "--production")
			installOutput = strings.TrimSpace(out)
			if err != nil {
				slog.Warn("agent: deploy npm install failed", "error", err, "output", installOutput)
			}
		}
	case "python":
		if _, err := r.executor.Stat(ctx, filepath.Join(payload.RootPath, "requirements.txt")); err == nil {
			out, err := r.executor.RunShellDir(ctx, payload.RootPath, "pip", "install", "-r", "requirements.txt")
			installOutput = strings.TrimSpace(out)
			if err != nil {
				slog.Warn("agent: deploy pip install failed", "error", err, "output", installOutput)
			}
		}
	case "go":
		if _, err := r.executor.Stat(ctx, filepath.Join(payload.RootPath, "go.mod")); err == nil {
			out, err := r.executor.RunShellDir(ctx, payload.RootPath, "go", "mod", "download")
			installOutput = strings.TrimSpace(out)
			if err != nil {
				slog.Warn("agent: deploy go mod download failed", "error", err, "output", installOutput)
			}
		}
	}

	// ─── Step 3: Restart app if it was running ───
	var restartMsg string
	if payload.AppStatus == "running" && payload.AppPort > 0 {
		svc := appServiceName(payload.SiteID)
		if _, err := r.executor.RunShell(ctx, "systemctl", "restart", svc); err == nil {
			// Verify it's actually running
			statusOut, _ := r.executor.RunShell(ctx, "systemctl", "is-active", svc)
			active := strings.TrimSpace(statusOut) == "active"
			if active {
				restartMsg = fmt.Sprintf("app restarted on port %d", payload.AppPort)
			} else {
				logs, _ := r.executor.RunShell(ctx, "journalctl", "-u", svc, "-n", "10", "--no-pager")
				restartMsg = fmt.Sprintf("app restart failed: %s", strings.TrimSpace(logs))
			}
		} else {
			restartMsg = "app restart skipped (service not found)"
		}
	}

	slog.Info("agent: site.deploy", "site_id", payload.SiteID, "repo", payload.Repo, "branch", payload.Branch, "commit", commit, "runtime", payload.Runtime)

	resp := map[string]any{
		"ok":      true,
		"message": fmt.Sprintf("deployed %s@%s", payload.Branch, commit),
		"commit":  commit,
	}
	if installOutput != "" {
		resp["install_output"] = installOutput
	}
	if restartMsg != "" {
		resp["restart"] = restartMsg
	}

	return c.JSON(http.StatusOK, resp)
}

func (r *Router) nginxReloadTask(c echo.Context) error {
	ctx := c.Request().Context()
	slog.Info("agent: nginx.reload requested")
	output, err := r.executor.RunShell(ctx, "systemctl", "reload", "nginx")
	if err != nil {
		// Best-effort: return warning if systemctl is unavailable
		return c.JSON(http.StatusOK, map[string]any{
			"ok":      true,
			"message": "nginx reload attempted but systemctl may be unavailable",
			"warning": strings.TrimSpace(output + " " + err.Error()),
		})
	}
	return c.JSON(http.StatusOK, map[string]any{
		"ok":      true,
		"message": "nginx reloaded",
		"output":  strings.TrimSpace(output),
	})
}

func (r *Router) sslIssueTask(c echo.Context) error {
	var payload struct {
		SiteID int64  `json:"site_id"`
		Domain string `json:"domain"`
		Email  string `json:"email"`
	}
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid payload")
	}

	if !isValidDomain(payload.Domain) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid domain")
	}

	slog.Info("agent: ssl.issue", "site_id", payload.SiteID, "domain", payload.Domain, "email", payload.Email)
	ctx := c.Request().Context()

	if _, err := exec.LookPath("certbot"); err != nil {
		if !r.allowStubSSL {
			return echo.NewHTTPError(http.StatusFailedDependency, "certbot not installed")
		}
		slog.Warn("agent: certbot not installed, returning stub", "domain", payload.Domain)
		return c.JSON(http.StatusOK, map[string]any{
			"ok":         true,
			"message":    "certbot not installed on this host — returning stub cert",
			"expires_at": time.Now().Add(90 * 24 * time.Hour).Format(time.RFC3339),
			"stub":       true,
		})
	}

	// Ensure webroot directory exists for ACME challenge
	webrootPath := "/var/www/certbot/" + payload.Domain
	r.executor.Mkdir(ctx, webrootPath)

	// Use webroot mode (does NOT modify nginx config — prevents redirect loop)
	output, err := r.executor.RunShell(ctx, "certbot",
		"certonly",
		"--webroot",
		"-w", webrootPath,
		"-d", payload.Domain,
		"--email", payload.Email,
		"--agree-tos",
		"--no-eff-email",
		"--non-interactive",
	)

	if err != nil {
		// webroot failed — likely Cloudflare proxy blocking. Return error, don't fallback to --nginx
		// which would modify nginx config and cause redirect loops
		return echo.NewHTTPError(http.StatusInternalServerError, "certbot failed: "+output)
	}

	// Generate SSL nginx config
	certPath := fmt.Sprintf("/etc/letsencrypt/live/%s/fullchain.pem", payload.Domain)
	var expiresAt string
	if expiryOut, err := r.executor.RunShell(ctx, "openssl", "x509", "-enddate", "-noout", "-in", certPath); err == nil {
		parts := strings.SplitN(expiryOut, "=", 2)
		if len(parts) == 2 {
			expiresAt = strings.TrimSpace(parts[1])
		}
	}

	// Update nginx config to include SSL
	slog.Info("agent: ssl.issue success, updating nginx config", "domain", payload.Domain)

	return c.JSON(http.StatusOK, map[string]any{
		"ok":        true,
		"message":   "certificate issued",
		"domain":    payload.Domain,
		"cert_path": certPath,
		"expires":   expiresAt,
	})
}

func (r *Router) sslRenewTask(c echo.Context) error {
	var payload struct {
		SiteID int64  `json:"site_id"`
		Domain string `json:"domain"`
	}
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid payload")
	}

	if !isValidDomain(payload.Domain) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid domain")
	}

	slog.Info("agent: ssl.renew", "site_id", payload.SiteID, "domain", payload.Domain)
	ctx := c.Request().Context()

	if _, err := exec.LookPath("certbot"); err != nil {
		if !r.allowStubSSL {
			return echo.NewHTTPError(http.StatusFailedDependency, "certbot not installed")
		}
		return c.JSON(http.StatusOK, map[string]any{
			"ok":      true,
			"message": "certbot not installed — stub response",
			"stub":    true,
		})
	}

	output, err := r.executor.RunShell(ctx, "certbot", "renew",
		"--cert-name", payload.Domain,
		"--non-interactive",
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "certbot renew failed: "+output)
	}

	// Reload nginx so renewed cert takes effect
	_, _ = r.executor.RunShell(ctx, "nginx", "-s", "reload")
	slog.Info("agent: nginx reloaded after ssl.renew", "domain", payload.Domain)

	return c.JSON(http.StatusOK, map[string]any{
		"ok":      true,
		"message": "certificate renewed",
		"domain":  payload.Domain,
		"output":  output,
	})
}

func (r *Router) logsReadTask(c echo.Context) error {
	ctx := c.Request().Context()
	var payload struct {
		SiteID int64  `json:"site_id"`
		Domain string `json:"domain"`
		Type   string `json:"type"`
		Lines  int    `json:"lines"`
	}
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid payload")
	}

	if payload.Lines <= 0 || payload.Lines > 10000 {
		payload.Lines = 100
	}
	if payload.Type != "access" && payload.Type != "error" {
		return echo.NewHTTPError(http.StatusBadRequest, "type must be access or error")
	}

	if !isValidDomain(payload.Domain) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid domain")
	}

	logPath := filepath.Join(r.logDir, payload.Domain+"."+payload.Type+".log")
	exists, _ := r.executor.Stat(ctx, logPath)
	if !exists {
		return c.JSON(http.StatusOK, map[string]any{
			"ok":      true,
			"path":    logPath,
			"lines":   []string{},
			"message": "log file does not exist yet",
		})
	}

	output, err := r.executor.RunShell(ctx, "tail", "-n", strconv.Itoa(payload.Lines), logPath)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "tail failed: "+strings.TrimSpace(output))
	}

	lines := []string{}
	if trimmed := strings.TrimRight(output, "\n"); trimmed != "" {
		lines = strings.Split(trimmed, "\n")
	}

	slog.Info("agent: logs.read", "site_id", payload.SiteID, "domain", payload.Domain, "type", payload.Type, "lines", payload.Lines)

	return c.JSON(http.StatusOK, map[string]any{
		"ok":    true,
		"path":  logPath,
		"lines": lines,
	})
}

func (r *Router) backupCreateTask(c echo.Context) error {
	ctx := c.Request().Context()
	var payload struct {
		BackupID   int64  `json:"backup_id"`
		SiteID     int64  `json:"site_id"`
		Domain     string `json:"domain"`
		RootPath   string `json:"root_path"`
		Storage    string `json:"storage"`
		BackupType string `json:"backup_type"`
		Databases  []struct {
			Name     string `json:"name"`
			Type     string `json:"type"`
			User     string `json:"user"`
			Password string `json:"password"`
		} `json:"databases"`
		S3Config *struct {
			Bucket    string `json:"bucket"`
			Endpoint  string `json:"endpoint"`
			Region    string `json:"region"`
			AccessKey string `json:"access_key"`
			SecretKey string `json:"secret_key"`
		} `json:"s3_config"`
	}
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid payload")
	}

	if !r.isAllowedSitePath(payload.RootPath) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid root_path")
	}

	// Use site-root backups/ dir (outside site root to avoid tar snowball)
	// and explicitly excluded from tar so no circular inclusion.
	globalBackupDir := filepath.Join(payload.RootPath, "backups")
	if err := r.executor.EnsureDir(ctx, globalBackupDir); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "ensure backup dir failed: "+err.Error())
	}

	kind := "files"
	if payload.BackupType == "both" || len(payload.Databases) > 0 {
		kind = "full"
	}
	backupName := buildBackupFileName(kind, payload.BackupID, ".tar.gz")
	backupFile := filepath.Join(globalBackupDir, backupName)
	var output string
	var err error
	if len(payload.Databases) > 0 {
		bundleDir, err := os.MkdirTemp(globalBackupDir, "full-*")
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "create bundle temp failed: "+err.Error())
		}
		defer os.RemoveAll(bundleDir)
		dumpsDir := filepath.Join(bundleDir, "database_dumps")
		if err := os.MkdirAll(dumpsDir, 0700); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "create dumps dir failed: "+err.Error())
		}
		for _, db := range payload.Databases {
			if !isValidDBIdentifier(db.Name) || !isValidDBIdentifier(db.User) {
				return echo.NewHTTPError(http.StatusBadRequest, "invalid database payload")
			}
			dumpFile := filepath.Join(dumpsDir, sanitizeBackupName(db.Name)+".sql")
			output, err = r.dumpDatabase(ctx, db.Type, db.User, db.Password, db.Name, dumpFile)
			if err != nil && output == errUnsupportedDBType.Error() {
				return echo.NewHTTPError(http.StatusBadRequest, "unsupported database type: "+db.Type)
			}
			if err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, "database dump failed: "+strings.TrimSpace(output))
			}
		}
		output, err = r.executor.RunShell(ctx, "tar", "-czf", backupFile,
			"--exclude=backups", "--exclude=node_modules", "--exclude=vendor",
			"--exclude=.git", "--exclude=.env", "--exclude=cache",
			"-C", filepath.Dir(payload.RootPath), filepath.Base(payload.RootPath),
			"-C", bundleDir, "database_dumps")
	} else {
		output, err = r.executor.RunShell(ctx, "tar", "-czf", backupFile,
			"--exclude=backups", "--exclude=node_modules", "--exclude=vendor",
			"--exclude=.git", "--exclude=.env", "--exclude=cache",
			"-C", filepath.Dir(payload.RootPath), filepath.Base(payload.RootPath))
	}
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "tar failed: "+strings.TrimSpace(output))
	}

	// Get file size using os.Stat for cross-platform compatibility
	info, err := os.Stat(backupFile)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "stat backup failed: "+err.Error())
	}

	slog.Info("agent: backup.create", "backup_id", payload.BackupID, "site_id", payload.SiteID, "path", backupFile, "status", "archived")

	resp := map[string]any{
		"ok":         true,
		"message":    "backup created",
		"path":       backupFile,
		"size_bytes": info.Size(),
		"backup_id":  payload.BackupID,
	}

	// Upload to S3-compatible storage if config provided
	slog.Info("agent: checking S3 config", "backup_id", payload.BackupID, "has_s3", payload.S3Config != nil, "bucket", payload.S3Config != nil && payload.S3Config.Bucket != "")
	if payload.S3Config != nil && payload.S3Config.Bucket != "" {
		s3Path := fmt.Sprintf("s3://%s/sites/%s/%s", payload.S3Config.Bucket, sanitizeBackupName(payload.Domain), backupName)

		endpoint := normalizeS3Endpoint(payload.S3Config.Endpoint)
		awsArgs := []string{
			"s3", "cp", backupFile, s3Path,
			"--endpoint-url", endpoint,
			"--region", payload.S3Config.Region,
		}

		env := []string{
			fmt.Sprintf("AWS_ACCESS_KEY_ID=%s", payload.S3Config.AccessKey),
			fmt.Sprintf("AWS_SECRET_ACCESS_KEY=%s", payload.S3Config.SecretKey),
		}

		slog.Info("agent: uploading backup to S3", "backup_id", payload.BackupID, "path", s3Path)
		uploadOut, err := r.executor.RunShellEnv(ctx, env, "aws", awsArgs...)
		if err != nil {
			slog.Error("agent: s3 upload failed", "backup_id", payload.BackupID, "error", err, "output", uploadOut)
			// Don't fail the backup — local file is still available
			resp["s3_upload_error"] = strings.TrimSpace(uploadOut)
		} else {
			resp["s3_path"] = s3Path
			resp["message"] = "backup created and uploaded to S3"
			slog.Info("agent: backup uploaded to S3", "backup_id", payload.BackupID, "path", s3Path)
		}
	}

	slog.Info("agent: backup.create", "backup_id", payload.BackupID, "site_id", payload.SiteID, "path", backupFile)

	return c.JSON(http.StatusOK, resp)
}

var errUnsupportedDBType = fmt.Errorf("unsupported database type")

// dumpDatabase runs mysqldump/pg_dump for the given database, writing the SQL
// dump to dumpFile. The password is passed via environment variable (MYSQL_PWD
// / PGPASSWORD), never on the command line or through a shell, so it cannot be
// injected or leaked via the process list. Returns stderr output and an error;
// when dbType is unrecognized the returned error is errUnsupportedDBType and the
// returned string equals its message.
func (r *Router) dumpDatabase(ctx context.Context, dbType, dbUser, dbPassword, dbName, dumpFile string) (string, error) {
	switch dbType {
	case "mysql":
		var env []string
		if dbPassword != "" {
			env = append(env, "MYSQL_PWD="+dbPassword)
		}
		return r.executor.RunToFileEnv(ctx, env, dumpFile, "mysqldump",
			"-u"+dbUser, "--no-tablespaces", "--single-transaction", "--quick", "--routines", "--triggers", dbName)
	case "postgres":
		env := []string{"PGPASSWORD=" + dbPassword}
		return r.executor.RunToFileEnv(ctx, env, dumpFile, "pg_dump",
			"-U"+dbUser, "-h", "localhost", "--no-owner", "--no-acl", dbName)
	default:
		return errUnsupportedDBType.Error(), errUnsupportedDBType
	}
}

func buildBackupFileName(kind string, backupID int64, ext string) string {
	k := sanitizeBackupName(kind)
	if k == "" {
		k = "backup"
	}
	stamp := time.Now().UTC().Format("20060102-150405")
	return fmt.Sprintf("%s_%s_%d%s", k, stamp, backupID, ext)
}

func sanitizeBackupName(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '-' || r == '_'
		if ok {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-_.")
}

func normalizeS3Endpoint(endpoint string) string {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return endpoint
	}
	if strings.HasPrefix(endpoint, "http://") || strings.HasPrefix(endpoint, "https://") {
		return endpoint
	}
	return "https://" + endpoint
}

func (r *Router) backupRestoreTask(c echo.Context) error {
	ctx := c.Request().Context()
	var payload struct {
		BackupID   int64  `json:"backup_id"`
		SiteID     int64  `json:"site_id"`
		Domain     string `json:"domain"`
		BackupPath string `json:"backup_path"`
		RootPath   string `json:"root_path"`
		S3Config   *struct {
			Bucket    string `json:"bucket"`
			Endpoint  string `json:"endpoint"`
			Region    string `json:"region"`
			AccessKey string `json:"access_key"`
			SecretKey string `json:"secret_key"`
		} `json:"s3_config"`
	}
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid payload")
	}
	if !r.isAllowedSitePath(payload.RootPath) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid root_path")
	}

	backupPath := payload.BackupPath

	// If S3 path, download to local first
	if strings.HasPrefix(backupPath, "s3://") {
		if payload.S3Config == nil || payload.S3Config.Bucket == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "s3_config required for S3 backup path")
		}

		siteBackupDir := filepath.Join(payload.RootPath, "backup")
		if err := r.executor.EnsureDir(ctx, siteBackupDir); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "ensure dir failed: "+err.Error())
		}

		localFile := filepath.Join(siteBackupDir, fmt.Sprintf("restore_%d.tar.gz", payload.BackupID))
		endpoint := normalizeS3Endpoint(payload.S3Config.Endpoint)
		awsArgs := []string{
			"s3", "cp", backupPath, localFile,
			"--endpoint-url", endpoint,
			"--region", payload.S3Config.Region,
		}
		env := []string{
			fmt.Sprintf("AWS_ACCESS_KEY_ID=%s", payload.S3Config.AccessKey),
			fmt.Sprintf("AWS_SECRET_ACCESS_KEY=%s", payload.S3Config.SecretKey),
		}

		slog.Info("agent: downloading backup from S3 for restore", "backup_id", payload.BackupID, "path", backupPath)
		dlOut, err := r.executor.RunShellEnv(ctx, env, "aws", awsArgs...)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "S3 download failed: "+strings.TrimSpace(dlOut))
		}
		backupPath = localFile
	}

	if !r.isAllowedBackupPath(backupPath) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid backup_path")
	}

	info, err := os.Stat(backupPath)
	if err != nil || info.IsDir() {
		return echo.NewHTTPError(http.StatusNotFound, "backup file not found")
	}

	tmpDir, err := os.MkdirTemp(r.backupDir, "restore-*")
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "create restore temp dir failed: "+err.Error())
	}
	defer os.RemoveAll(tmpDir)

	output, err := r.executor.RunShell(ctx, "tar", "-xzf", backupPath, "-C", tmpDir)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "extract backup failed: "+strings.TrimSpace(output))
	}

	extractedRoot := filepath.Join(tmpDir, filepath.Base(payload.RootPath))
	if exists, _ := r.executor.Stat(ctx, extractedRoot); !exists {
		return echo.NewHTTPError(http.StatusInternalServerError, "backup archive missing expected site root")
	}
	if err := os.MkdirAll(filepath.Dir(payload.RootPath), 0755); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "create parent dir failed: "+err.Error())
	}

	// Crash-safe swap: move the current root aside instead of deleting it, so a
	// failure while installing the restored copy can be rolled back and never
	// leaves the site with no data.
	oldRoot := ""
	if exists, _ := r.executor.Stat(ctx, payload.RootPath); exists {
		oldRoot = fmt.Sprintf("%s.restore-old-%d", payload.RootPath, payload.BackupID)
		_ = os.RemoveAll(oldRoot)
		if err := movePath(payload.RootPath, oldRoot); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "stage existing root failed: "+err.Error())
		}
	}

	if err := movePath(extractedRoot, payload.RootPath); err != nil {
		// Roll back: restore the original root from the staged copy.
		if oldRoot != "" {
			_ = os.RemoveAll(payload.RootPath)
			if rbErr := movePath(oldRoot, payload.RootPath); rbErr != nil {
				slog.Error("agent: backup.restore rollback failed", "backup_id", payload.BackupID,
					"root_path", payload.RootPath, "staged", oldRoot, "error", rbErr)
				return echo.NewHTTPError(http.StatusInternalServerError,
					"move restored root failed and rollback failed; original site data preserved at "+oldRoot+": "+err.Error())
			}
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "move restored root failed: "+err.Error())
	}

	// Restore succeeded — discard the staged old copy (best effort).
	if oldRoot != "" {
		if err := os.RemoveAll(oldRoot); err != nil {
			slog.Warn("agent: backup.restore cleanup of staged old root failed", "backup_id", payload.BackupID, "staged", oldRoot, "error", err)
		}
	}
	filesExtracted := countFiles(payload.RootPath)

	slog.Info("agent: backup.restore", "backup_id", payload.BackupID, "site_id", payload.SiteID)

	return c.JSON(http.StatusOK, map[string]any{
		"ok":              true,
		"files_extracted": filesExtracted,
		"message":         "backup restored",
		"root_path":       payload.RootPath,
	})
}

func (r *Router) databaseBackupTask(c echo.Context) error {
	ctx := c.Request().Context()
	var payload struct {
		BackupID   int64  `json:"backup_id"`
		SiteID     int64  `json:"site_id"`
		Domain     string `json:"domain"`
		RootPath   string `json:"root_path"`
		DBName     string `json:"db_name"`
		DBType     string `json:"db_type"`
		DBUser     string `json:"db_user"`
		DBPassword string `json:"db_password"`
		S3Config   *struct {
			Bucket    string `json:"bucket"`
			Endpoint  string `json:"endpoint"`
			Region    string `json:"region"`
			AccessKey string `json:"access_key"`
			SecretKey string `json:"secret_key"`
		} `json:"s3_config"`
	}
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid payload")
	}

	if !isValidDBIdentifier(payload.DBName) || !isValidDBIdentifier(payload.DBUser) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid db_name or db_user")
	}

	siteBackupDir := filepath.Join(payload.RootPath, "backup", "db")
	if err := r.executor.EnsureDir(ctx, siteBackupDir); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "ensure dir failed: "+err.Error())
	}

	dumpName := buildBackupFileName("database_"+sanitizeBackupName(payload.DBName), payload.BackupID, ".sql")
	dumpFile := filepath.Join(siteBackupDir, dumpName)

	output, err := r.dumpDatabase(ctx, payload.DBType, payload.DBUser, payload.DBPassword, payload.DBName, dumpFile)
	if err != nil {
		if output == errUnsupportedDBType.Error() {
			return echo.NewHTTPError(http.StatusBadRequest, "unsupported database type: "+payload.DBType)
		}
		return echo.NewHTTPError(http.StatusInternalServerError, payload.DBType+" dump failed: "+strings.TrimSpace(output))
	}

	info, err := os.Stat(dumpFile)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "dump file not found after export: "+err.Error())
	}

	slog.Info("agent: db_backup.create", "backup_id", payload.BackupID, "db_name", payload.DBName, "path", dumpFile, "status", "archived")

	resp := map[string]any{
		"ok":         true,
		"message":    "database backup created",
		"path":       dumpFile,
		"size_bytes": info.Size(),
		"backup_id":  payload.BackupID,
		"db_name":    payload.DBName,
	}

	// Upload to S3 if config provided
	if payload.S3Config != nil && payload.S3Config.Bucket != "" {
		s3Path := fmt.Sprintf("s3://%s/sites-db/%s/%s", payload.S3Config.Bucket, sanitizeBackupName(payload.Domain), dumpName)
		endpoint := normalizeS3Endpoint(payload.S3Config.Endpoint)
		awsArgs := []string{
			"s3", "cp", dumpFile, s3Path,
			"--endpoint-url", endpoint,
			"--region", payload.S3Config.Region,
		}
		env := []string{
			fmt.Sprintf("AWS_ACCESS_KEY_ID=%s", payload.S3Config.AccessKey),
			fmt.Sprintf("AWS_SECRET_ACCESS_KEY=%s", payload.S3Config.SecretKey),
		}

		slog.Info("agent: uploading db backup to S3", "backup_id", payload.BackupID, "path", s3Path)
		uploadOut, err := r.executor.RunShellEnv(ctx, env, "aws", awsArgs...)
		if err != nil {
			slog.Error("agent: S3 upload failed for db backup", "backup_id", payload.BackupID, "error", err, "output", uploadOut)
			resp["s3_upload_error"] = strings.TrimSpace(uploadOut)
		} else {
			resp["s3_path"] = s3Path
			resp["message"] = "database backup created and uploaded to S3"
			slog.Info("agent: db backup uploaded to S3", "backup_id", payload.BackupID, "path", s3Path)
		}
	}

	slog.Info("agent: backup.database", "backup_id", payload.BackupID, "site_id", payload.SiteID, "db_name", payload.DBName)

	return c.JSON(http.StatusOK, resp)
}

// backupDeleteTask removes a backup file from the agent's disk. It is invoked
// by the panel's retention cleanup so that expired local backups do not
// accumulate and fill the VPS disk. The path is validated against the allowed
// backup locations; S3-only paths are ignored (handled panel-side).
func (r *Router) backupDeleteTask(c echo.Context) error {
	var payload struct {
		BackupID    int64  `json:"backup_id"`
		StoragePath string `json:"storage_path"`
	}
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid payload")
	}

	path := strings.TrimSpace(payload.StoragePath)
	if path == "" || strings.HasPrefix(path, "s3://") {
		// Nothing local to remove.
		return c.JSON(http.StatusOK, map[string]any{"ok": true, "message": "no local file to delete", "backup_id": payload.BackupID})
	}
	if !r.isAllowedBackupPath(path) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid storage_path")
	}

	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return c.JSON(http.StatusOK, map[string]any{"ok": true, "message": "already gone", "backup_id": payload.BackupID})
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "remove backup file failed: "+err.Error())
	}

	slog.Info("agent: backup.delete", "backup_id", payload.BackupID, "path", path)
	return c.JSON(http.StatusOK, map[string]any{"ok": true, "message": "backup deleted", "backup_id": payload.BackupID, "path": path})
}

func (r *Router) databaseCreateTask(c echo.Context) error {
	ctx := c.Request().Context()
	var payload struct {
		DatabaseID int64  `json:"database_id"`
		SiteID     int64  `json:"site_id"`
		Name       string `json:"name"`
		Type       string `json:"type"`
	}
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid payload")
	}
	if !isValidDBIdentifier(payload.Name) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid database name")
	}

	switch payload.Type {
	case "mysql":
		sql := fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s`;\n", payload.Name)
		if output, err := r.executor.RunWithInput(ctx, sql, "mysql"); err != nil {
			return echo.NewHTTPError(http.StatusFailedDependency, "mysql create database failed: "+strings.TrimSpace(output))
		}
	case "postgres":
		sql := fmt.Sprintf("SELECT 'CREATE DATABASE %s' WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = %s)\\gexec\n", payload.Name, sqlQuote(payload.Name))
		if output, err := r.executor.RunWithInput(ctx, sql, "psql", "-v", "ON_ERROR_STOP=1", "-d", "postgres"); err != nil {
			return echo.NewHTTPError(http.StatusFailedDependency, "postgres create database failed: "+strings.TrimSpace(output))
		}
	default:
		return echo.NewHTTPError(http.StatusBadRequest, "unsupported database type")
	}

	slog.Info("agent: database.create", "database_id", payload.DatabaseID, "name", payload.Name, "type", payload.Type)
	return c.JSON(http.StatusOK, map[string]any{
		"ok":      true,
		"message": fmt.Sprintf("database %s (%s) created", payload.Name, payload.Type),
	})
}

func (r *Router) databaseDeleteTask(c echo.Context) error {
	ctx := c.Request().Context()
	var payload struct {
		DatabaseID int64  `json:"database_id"`
		Name       string `json:"name"`
		Type       string `json:"type"`
	}
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid payload")
	}
	if !isValidDBIdentifier(payload.Name) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid database name")
	}

	switch payload.Type {
	case "mysql":
		sql := fmt.Sprintf("DROP DATABASE IF EXISTS `%s`;\n", payload.Name)
		if output, err := r.executor.RunWithInput(ctx, sql, "mysql"); err != nil {
			return echo.NewHTTPError(http.StatusFailedDependency, "mysql drop database failed: "+strings.TrimSpace(output))
		}
	case "postgres":
		sql := fmt.Sprintf("DROP DATABASE IF EXISTS %s;\n", payload.Name)
		if output, err := r.executor.RunWithInput(ctx, sql, "psql", "-v", "ON_ERROR_STOP=1", "-d", "postgres"); err != nil {
			return echo.NewHTTPError(http.StatusFailedDependency, "postgres drop database failed: "+strings.TrimSpace(output))
		}
	default:
		return echo.NewHTTPError(http.StatusBadRequest, "unsupported database type")
	}

	slog.Info("agent: database.delete", "database_id", payload.DatabaseID)
	return c.JSON(http.StatusOK, map[string]any{
		"ok":      true,
		"message": fmt.Sprintf("database %s deleted", payload.Name),
	})
}

func (r *Router) dbUserCreateTask(c echo.Context) error {
	ctx := c.Request().Context()
	var payload struct {
		UserID       int64  `json:"user_id"`
		DatabaseID   int64  `json:"database_id"`
		DatabaseName string `json:"database_name"`
		Type         string `json:"type"`
		Username     string `json:"username"`
		Password     string `json:"password"`
		HostPattern  string `json:"host_pattern"`
		Privileges   string `json:"privileges"`
	}
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid payload")
	}
	if !isValidDBIdentifier(payload.DatabaseName) || !isValidDBIdentifier(payload.Username) || payload.Password == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid database user payload")
	}
	// The password is embedded in SQL fed to mysql/psql running as root.
	if err := siteinput.DBPassword(payload.Password); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if payload.HostPattern == "" {
		payload.HostPattern = "localhost"
	}
	if payload.Privileges == "" {
		payload.Privileges = "ALL"
	}
	if !isValidDBHostPattern(payload.HostPattern) || !isValidDBPrivileges(payload.Privileges) ||
		siteinput.DBPrivileges(payload.Privileges) != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid database user grants")
	}

	switch payload.Type {
	case "mysql":
		sql := fmt.Sprintf(
			"CREATE USER IF NOT EXISTS %s@%s IDENTIFIED BY %s; GRANT %s ON `%s`.* TO %s@%s; FLUSH PRIVILEGES;\n",
			sqlQuote(payload.Username), sqlQuote(payload.HostPattern), sqlQuote(payload.Password), payload.Privileges, payload.DatabaseName, sqlQuote(payload.Username), sqlQuote(payload.HostPattern),
		)
		if output, err := r.executor.RunWithInput(ctx, sql, "mysql"); err != nil {
			return echo.NewHTTPError(http.StatusFailedDependency, "mysql create user failed: "+strings.TrimSpace(output))
		}
	case "postgres":
		// Random dollar-quote tag: the (already control-char-free) password can
		// never terminate the DO block.
		tag := "$sg" + randomHex(8) + "$"
		sql := fmt.Sprintf(
			"DO %s BEGIN CREATE ROLE %s LOGIN PASSWORD %s; EXCEPTION WHEN duplicate_object THEN ALTER ROLE %s WITH PASSWORD %s; END %s; GRANT ALL PRIVILEGES ON DATABASE %s TO %s;\n",
			tag, payload.Username, sqlQuote(payload.Password), payload.Username, sqlQuote(payload.Password), tag, payload.DatabaseName, payload.Username,
		)
		if output, err := r.executor.RunWithInput(ctx, sql, "psql", "-v", "ON_ERROR_STOP=1", "-d", "postgres"); err != nil {
			return echo.NewHTTPError(http.StatusFailedDependency, "postgres create user failed: "+strings.TrimSpace(output))
		}
	default:
		return echo.NewHTTPError(http.StatusBadRequest, "unsupported database type")
	}

	slog.Info("agent: dbuser.create", "user_id", payload.UserID, "username", payload.Username)
	return c.JSON(http.StatusOK, map[string]any{
		"ok":      true,
		"message": fmt.Sprintf("db user %s created", payload.Username),
	})
}

func (r *Router) dbUserDeleteTask(c echo.Context) error {
	ctx := c.Request().Context()
	var payload struct {
		UserID       int64  `json:"user_id"`
		Username     string `json:"username"`
		DatabaseID   int64  `json:"database_id"`
		DatabaseName string `json:"database_name"`
		Type         string `json:"type"`
		HostPattern  string `json:"host_pattern"`
	}
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid payload")
	}
	if !isValidDBIdentifier(payload.Username) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid username")
	}
	if payload.HostPattern == "" {
		payload.HostPattern = "localhost"
	}
	if !isValidDBHostPattern(payload.HostPattern) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid host_pattern")
	}

	switch payload.Type {
	case "mysql":
		sql := fmt.Sprintf("DROP USER IF EXISTS %s@%s; FLUSH PRIVILEGES;\n", sqlQuote(payload.Username), sqlQuote(payload.HostPattern))
		if output, err := r.executor.RunWithInput(ctx, sql, "mysql"); err != nil {
			return echo.NewHTTPError(http.StatusFailedDependency, "mysql drop user failed: "+strings.TrimSpace(output))
		}
	case "postgres":
		sql := fmt.Sprintf("DROP ROLE IF EXISTS %s;\n", payload.Username)
		if output, err := r.executor.RunWithInput(ctx, sql, "psql", "-v", "ON_ERROR_STOP=1", "-d", "postgres"); err != nil {
			return echo.NewHTTPError(http.StatusFailedDependency, "postgres drop user failed: "+strings.TrimSpace(output))
		}
	default:
		return echo.NewHTTPError(http.StatusBadRequest, "unsupported database type")
	}

	slog.Info("agent: dbuser.delete", "user_id", payload.UserID, "username", payload.Username)
	return c.JSON(http.StatusOK, map[string]any{
		"ok":      true,
		"message": fmt.Sprintf("db user %s deleted", payload.Username),
	})
}

// commandExecTask executes an arbitrary shell command on the server and returns output.
// POST /tasks/command/exec
func (r *Router) commandExecTask(c echo.Context) error {
	ctx := c.Request().Context()
	var payload struct {
		Command string `json:"command"`
		Timeout int    `json:"timeout"`
	}
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid payload")
	}
	if payload.Command == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "command is required")
	}
	if payload.Timeout <= 0 || payload.Timeout > 300 {
		payload.Timeout = 30
	}

	slog.Info("agent: command.exec", "command", payload.Command)
	output, err := r.executor.RunShell(ctx, "bash", "-c", payload.Command)
	if err != nil {
		return c.JSON(http.StatusOK, map[string]any{
			"ok":      false,
			"output":  strings.TrimSpace(output),
			"error":   err.Error(),
			"command": payload.Command,
		})
	}
	return c.JSON(http.StatusOK, map[string]any{
		"ok":      true,
		"output":  strings.TrimSpace(output),
		"command": payload.Command,
	})
}

func notImplementedHandler(action string) echo.HandlerFunc {
	return func(c echo.Context) error {
		return c.JSON(http.StatusNotImplemented, httpjson.ErrorBody{
			Error: httpjson.APIError{
				Code:    "not_implemented",
				Message: action + " not implemented yet",
			},
			RequestID: "",
		})
	}
}

// --- metrics helpers ---

func readCPU() float64 {
	stat1, err := readCPUStat()
	if err != nil {
		return 0
	}
	time.Sleep(200 * time.Millisecond)
	stat2, err := readCPUStat()
	if err != nil {
		return 0
	}
	total1 := stat1.user + stat1.nice + stat1.system + stat1.idle + stat1.iowait + stat1.irq + stat1.softirq + stat1.steal
	total2 := stat2.user + stat2.nice + stat2.system + stat2.idle + stat2.iowait + stat2.irq + stat2.softirq + stat2.steal
	idle1 := stat1.idle + stat1.iowait
	idle2 := stat2.idle + stat2.iowait
	dt := total2 - total1
	di := idle2 - idle1
	if dt == 0 {
		return 0
	}
	return float64(dt-di) / float64(dt) * 100.0
}

type cpuStat struct {
	user, nice, system, idle, iowait, irq, softirq, steal float64
}

func readCPUStat() (cpuStat, error) {
	data, err := exec.Command("sh", "-c", "head -1 /proc/stat").Output()
	if err != nil {
		return cpuStat{}, err
	}
	fields := strings.Fields(string(data))
	if len(fields) < 9 {
		return cpuStat{}, fmt.Errorf("short /proc/stat")
	}
	var s cpuStat
	s.user, _ = strconv.ParseFloat(fields[1], 64)
	s.nice, _ = strconv.ParseFloat(fields[2], 64)
	s.system, _ = strconv.ParseFloat(fields[3], 64)
	s.idle, _ = strconv.ParseFloat(fields[4], 64)
	s.iowait, _ = strconv.ParseFloat(fields[5], 64)
	s.irq, _ = strconv.ParseFloat(fields[6], 64)
	s.softirq, _ = strconv.ParseFloat(fields[7], 64)
	s.steal, _ = strconv.ParseFloat(fields[8], 64)
	return s, nil
}

func readMem() (used, total int64) {
	data, err := exec.Command("sh", "-c", "grep -E '^(MemTotal|MemFree|MemAvailable):' /proc/meminfo | head -3").Output()
	if err != nil {
		return -1, -1
	}
	var memTotal, memFree int64
	for _, line := range strings.Split(string(data), "\n") {
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		val, _ := strconv.ParseInt(parts[1], 10, 64)
		if strings.HasPrefix(line, "MemTotal:") {
			memTotal = val
		} else if strings.HasPrefix(line, "MemFree:") || strings.HasPrefix(line, "MemAvailable:") {
			memFree = val
		}
	}
	if memTotal > 0 && memFree > 0 {
		return (memTotal - memFree) * 1024, memTotal * 1024
	}
	return -1, -1
}

func readDisk() (used, total int64) {
	data, err := exec.Command("sh", "-c", "df -B1 / 2>/dev/null | tail -1").Output()
	if err != nil {
		return -1, -1
	}
	fields := strings.Fields(string(data))
	if len(fields) < 4 {
		return -1, -1
	}
	total, _ = strconv.ParseInt(fields[1], 10, 64)
	used, _ = strconv.ParseInt(fields[2], 10, 64)
	if total > 0 {
		return used, total
	}
	return -1, -1
}

func readSwap() (used, total int64) {
	data, err := exec.Command("sh", "-c", "grep -E '^(SwapTotal|SwapFree):' /proc/meminfo").Output()
	if err != nil {
		return -1, -1
	}
	var swapTotal, swapFree int64
	for _, line := range strings.Split(string(data), "\n") {
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		val, _ := strconv.ParseInt(parts[1], 10, 64)
		if strings.HasPrefix(line, "SwapTotal:") {
			swapTotal = val
		} else if strings.HasPrefix(line, "SwapFree:") {
			swapFree = val
		}
	}
	if swapTotal > 0 && swapFree > 0 {
		return (swapTotal - swapFree) * 1024, swapTotal * 1024
	}
	return -1, -1
}

func readNet() (inBytes, outBytes int64) {
	data, err := exec.Command("sh", "-c", "cat /proc/net/dev").Output()
	if err != nil {
		return -1, -1
	}
	var totalRx, totalTx int64
	lines := strings.Split(string(data), "\n")
	for _, line := range lines[2:] { // skip header lines
		fields := strings.Fields(line)
		if len(fields) < 10 {
			continue
		}
		iface := strings.TrimSuffix(fields[0], ":")
		if iface == "lo" {
			continue
		}
		rx, _ := strconv.ParseInt(fields[1], 10, 64)
		tx, _ := strconv.ParseInt(fields[9], 10, 64)
		totalRx += rx
		totalTx += tx
	}
	if totalRx == 0 && totalTx == 0 {
		return -1, -1
	}
	return totalRx, totalTx
}

type gpuInfo struct {
	Name        string
	Count       int
	Utilization float64
	MemoryUsed  int64
	MemoryTotal int64
	Temperature float64
	PowerDraw   float64
	FanSpeed    float64
}

func readGPU() *gpuInfo {
	if _, err := exec.LookPath("nvidia-smi"); err != nil {
		return nil
	}
	out, err := exec.Command("nvidia-smi", "--query-gpu=name,utilization.gpu,memory.used,memory.total,temperature.gpu,power.draw,fan.speed", "--format=csv,noheader,nounits").Output()
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 0 {
		return nil
	}
	gpu := &gpuInfo{
		Count: len(lines),
	}
	var totalMemUsed, totalMemTotal int64
	var utilSum, tempSum, powerSum, fanSum float64
	var validCount int
	for _, line := range lines {
		fields := strings.Split(strings.TrimSpace(line), ", ")
		if len(fields) < 7 {
			continue
		}
		if validCount == 0 {
			gpu.Name = strings.TrimSpace(fields[0])
		}
		util, _ := strconv.ParseFloat(strings.TrimSpace(fields[1]), 64)
		memUsed, _ := strconv.ParseInt(strings.TrimSpace(fields[2]), 10, 64)
		memTotal, _ := strconv.ParseInt(strings.TrimSpace(fields[3]), 10, 64)
		temp, _ := strconv.ParseFloat(strings.TrimSpace(fields[4]), 64)
		power, _ := strconv.ParseFloat(strings.TrimSpace(fields[5]), 64)
		fan, _ := strconv.ParseFloat(strings.TrimSpace(fields[6]), 64)
		utilSum += util
		tempSum += temp
		powerSum += power
		fanSum += fan
		totalMemUsed += memUsed
		totalMemTotal += memTotal
		validCount++
	}
	if validCount == 0 {
		return nil
	}
	gpu.Utilization = utilSum / float64(validCount)
	gpu.MemoryUsed = totalMemUsed * 1024 * 1024
	gpu.MemoryTotal = totalMemTotal * 1024 * 1024
	gpu.Temperature = tempSum / float64(validCount)
	gpu.PowerDraw = powerSum / float64(validCount)
	gpu.FanSpeed = fanSum / float64(validCount)
	return gpu
}
