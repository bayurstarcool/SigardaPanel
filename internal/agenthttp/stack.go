package agenthttp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v5"
	"sigardapanel/internal/httpjson"
)

// Stack management: detection of the server's software and package
// operations (install / uninstall / update / extensions) plus service actions.
//
// Package operations can take many minutes (apt-get update, PPAs, dozens of
// packages), longer than any proxy in front of the panel waits, so they run in
// the background as an "operation": POST /tasks/stack/install answers 202 with
// an op_id at once and GET /tasks/stack/ops/:id reports progress (log tail)
// and the verified result. Only one package operation runs at a time (apt
// cannot run concurrently anyway). Service actions (start/stop/restart/
// reload) and listings are quick and answered synchronously.

// StackRequest is the payload for stack operations.
type StackRequest struct {
	Package   string `json:"package"`
	Version   string `json:"version,omitempty"`
	Action    string `json:"action,omitempty"`
	Extension string `json:"extension,omitempty"`
}

// StackResponse is the result of a synchronous stack action.
type StackResponse struct {
	OK      bool       `json:"ok"`
	Message string     `json:"message"`
	Output  string     `json:"output,omitempty"`
	Version string     `json:"version,omitempty"`
	Info    *StackInfo `json:"info,omitempty"`
}

var stackVersionRe = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z.+:~_\-]{0,63}$`)

// asyncStackActions run as background operations.
var asyncStackActions = map[string]bool{
	"install": true, "uninstall": true, "update": true,
	"install-extension": true, "uninstall-extension": true,
}

var serviceStackActions = map[string]bool{"start": true, "stop": true, "restart": true, "reload": true}

// stackOp is one background package operation.
type stackOp struct {
	mu         sync.Mutex
	ID         string
	Package    string
	Action     string
	Version    string
	Extension  string
	Status     string // running | succeeded | failed
	Message    string
	log        cappedBuffer
	StartedAt  time.Time
	FinishedAt time.Time
	Info       *StackInfo
}

func (o *stackOp) Write(p []byte) (int, error) { return o.log.Write(p) }

func (o *stackOp) snapshot(tail int) map[string]any {
	o.mu.Lock()
	defer o.mu.Unlock()
	snap := map[string]any{
		"op_id":      o.ID,
		"package":    o.Package,
		"action":     o.Action,
		"version":    o.Version,
		"extension":  o.Extension,
		"status":     o.Status,
		"ok":         o.Status == "succeeded",
		"done":       o.Status != "running",
		"message":    o.Message,
		"output":     tailText(o.log.String(), tail),
		"started_at": o.StartedAt.UTC().Format(time.RFC3339),
	}
	if !o.FinishedAt.IsZero() {
		snap["finished_at"] = o.FinishedAt.UTC().Format(time.RFC3339)
		snap["duration_ms"] = o.FinishedAt.Sub(o.StartedAt).Milliseconds()
	} else {
		snap["duration_ms"] = time.Since(o.StartedAt).Milliseconds()
	}
	if o.Info != nil {
		snap["info"] = o.Info
	}
	return snap
}

// stackManager owns detection, the operation registry and the host access.
type stackManager struct {
	sys        stackSystem
	binDir     string
	sbinDir    string
	phpRunDir  string
	vhostDirs  []string
	goDLURL    string
	httpClient *http.Client
	opTimeout  time.Duration

	mu      sync.Mutex
	ops     map[string]*stackOp
	order   []string
	running *stackOp
	wg      sync.WaitGroup
}

func newStackManager(sys stackSystem, phpRunDir string, vhostDirs []string) *stackManager {
	return &stackManager{
		sys:        sys,
		binDir:     "/usr/bin",
		sbinDir:    "/usr/sbin",
		phpRunDir:  phpRunDir,
		vhostDirs:  vhostDirs,
		goDLURL:    "https://go.dev/dl/",
		httpClient: &http.Client{Timeout: 30 * time.Second},
		opTimeout:  45 * time.Minute,
		ops:        map[string]*stackOp{},
	}
}

func newOpID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "op_" + hex.EncodeToString(b)
}

// start registers and launches a background operation; it refuses while
// another one runs.
func (m *stackManager) start(req StackRequest, fn func(ctx context.Context, w io.Writer) (string, error)) (*stackOp, error) {
	m.mu.Lock()
	if m.running != nil {
		r := m.running
		m.mu.Unlock()
		return nil, conflict("another stack operation is running (%s %s %s, %s); wait for it to finish",
			r.Action, r.Package, r.Version, r.ID)
	}
	op := &stackOp{
		ID: newOpID(), Package: req.Package, Action: req.Action, Version: req.Version,
		Extension: req.Extension, Status: "running", StartedAt: time.Now(),
		Message: fmt.Sprintf("%s %s %s in progress", req.Action, req.Package, req.Version),
	}
	op.log.max = 256 << 10
	m.ops[op.ID] = op
	m.order = append(m.order, op.ID)
	for len(m.order) > 30 {
		delete(m.ops, m.order[0])
		m.order = m.order[1:]
	}
	m.running = op
	m.wg.Add(1)
	m.mu.Unlock()

	go func() {
		defer m.wg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), m.opTimeout)
		defer cancel()
		fmt.Fprintf(op, "[sigardapanel] %s %s %s %s started\n", op.Action, op.Package, op.Version, op.Extension)
		var msg string
		var err error
		func() {
			defer func() {
				if p := recover(); p != nil {
					err = fmt.Errorf("internal error: %v", p)
				}
			}()
			msg, err = fn(ctx, op)
		}()
		if err == nil && ctx.Err() != nil {
			err = fmt.Errorf("timed out after %s", m.opTimeout)
		}
		info := m.detect(context.Background(), req.Package)
		op.mu.Lock()
		op.FinishedAt = time.Now()
		op.Info = &info
		if err != nil {
			op.Status = "failed"
			op.Message = err.Error()
		} else {
			op.Status = "succeeded"
			op.Message = msg
		}
		op.mu.Unlock()
		fmt.Fprintf(op, "\n[sigardapanel] %s: %s\n", op.Status, op.Message)
		elapsed := time.Since(op.StartedAt).Milliseconds()
		if err != nil {
			slog.Error("agent: stack operation failed", "op", op.ID, "package", op.Package, "action", op.Action, "version", op.Version, "error", err, "elapsed_ms", elapsed)
		} else {
			slog.Info("agent: stack operation ok", "op", op.ID, "package", op.Package, "action", op.Action, "version", op.Version, "elapsed_ms", elapsed)
		}
		m.mu.Lock()
		m.running = nil
		m.mu.Unlock()
	}()
	return op, nil
}

func (m *stackManager) get(id string) *stackOp {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.ops[id]
}

func (m *stackManager) current() *stackOp {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

// operation returns the background function for a package action, after
// validating it (validation errors are returned before anything starts).
func (m *stackManager) operation(req StackRequest) (func(ctx context.Context, w io.Writer) (string, error), error) {
	p, a, v := req.Package, req.Action, req.Version
	if a == "install-extension" || a == "uninstall-extension" {
		if p != "php" {
			return nil, badRequest("extensions are only managed for php")
		}
		if !phpVersionArgRe.MatchString(v) {
			return nil, badRequest("a PHP version such as 8.3 is required")
		}
		if !phpExtRe.MatchString(req.Extension) {
			return nil, badRequest("invalid extension name")
		}
		if a == "install-extension" {
			return func(ctx context.Context, w io.Writer) (string, error) {
				return m.phpExtensionInstall(ctx, w, v, req.Extension)
			}, nil
		}
		if phpCoreParts[req.Extension] {
			return nil, badRequest("%s is part of PHP %s itself; uninstall the version instead", req.Extension, v)
		}
		return func(ctx context.Context, w io.Writer) (string, error) {
			return m.phpExtensionUninstall(ctx, w, v, req.Extension)
		}, nil
	}

	switch p {
	case "php":
		if v == "" && a != "update" {
			return nil, badRequest("a PHP version such as 8.3 is required to %s PHP", a)
		}
		if v != "" && !phpVersionArgRe.MatchString(v) {
			return nil, badRequest("invalid PHP version %q (expected e.g. 8.3)", v)
		}
		switch a {
		case "install":
			return func(ctx context.Context, w io.Writer) (string, error) { return m.phpInstall(ctx, w, v) }, nil
		case "uninstall":
			if users := m.phpVhostUsers(v); len(users) > 0 {
				return nil, conflict("PHP %s is still used by nginx vhost(s): %s. Switch those sites to another PHP version first", v, strings.Join(users, ", "))
			}
			return func(ctx context.Context, w io.Writer) (string, error) { return m.phpUninstall(ctx, w, v) }, nil
		case "update":
			return func(ctx context.Context, w io.Writer) (string, error) { return m.phpUpdate(ctx, w, v) }, nil
		}
	case "node":
		switch a {
		case "install", "update":
			if v != "" && !nodeMajorRe.MatchString(v) {
				return nil, badRequest("Node.js version must be a major version such as %s", nodeDefaultMajor)
			}
			if a == "update" && v == "" {
				return func(ctx context.Context, w io.Writer) (string, error) {
					info := m.detect(ctx, "node")
					if info.Status == "missing" {
						return "", badRequest("Node.js is not installed")
					}
					return m.nodeInstall(ctx, w, firstMajor(info.Version))
				}, nil
			}
			return func(ctx context.Context, w io.Writer) (string, error) { return m.nodeInstall(ctx, w, v) }, nil
		case "uninstall":
			return func(ctx context.Context, w io.Writer) (string, error) { return m.aptComponentUninstall(ctx, w, "node") }, nil
		}
	case "go":
		switch a {
		case "install", "update":
			if v != "" && !goVersionRe.MatchString(v) {
				return nil, badRequest("invalid Go version %q (expected e.g. 1.25.4)", v)
			}
			return func(ctx context.Context, w io.Writer) (string, error) { return m.goInstall(ctx, w, v) }, nil
		case "uninstall":
			return func(ctx context.Context, w io.Writer) (string, error) {
				if m.detect(ctx, "go").Status == "missing" {
					return "", badRequest("Go is not installed")
				}
				if _, err := m.sys.Run(ctx, w, nil, "sh", "-c", goUninstallScript); err != nil {
					return "", fmt.Errorf("removing /usr/local/go failed")
				}
				if info := m.detect(ctx, "go"); info.Status != "missing" {
					return "", fmt.Errorf("Go is still installed at %s (not managed by the panel: installed from packages?)", info.Path)
				}
				return "Go removed from /usr/local/go", nil
			}, nil
		}
	case "nginx", "python", "docker", "certbot", "mariadb", "mysql", "redis", "memcached", "supervisor":
		name := p
		if name == "mysql" {
			name = "mariadb"
		}
		if v != "" {
			return nil, badRequest("%s is installed from the distribution's packages; a version cannot be chosen", name)
		}
		switch a {
		case "install":
			return func(ctx context.Context, w io.Writer) (string, error) { return m.aptComponentInstall(ctx, w, name) }, nil
		case "uninstall":
			return func(ctx context.Context, w io.Writer) (string, error) { return m.aptComponentUninstall(ctx, w, name) }, nil
		case "update":
			return func(ctx context.Context, w io.Writer) (string, error) { return m.aptComponentUpdate(ctx, w, name) }, nil
		}
	default:
		return nil, badRequest("unknown package: %s", p)
	}
	return nil, badRequest("unknown action %q for %s", a, p)
}

func firstMajor(v string) string {
	for i := 0; i < len(v); i++ {
		if v[i] == '.' {
			return v[:i]
		}
	}
	return v
}

// --- Agent task handlers ---

func stackJSONError(c *echo.Context, err error) error {
	var se *stackError
	if errors.As(err, &se) {
		return c.JSON(se.Code, map[string]any{"ok": false, "message": se.Msg})
	}
	return c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "message": err.Error()})
}

// stackInfoTask detects all installed software and their versions.
func (r *Router) stackInfoTask(c *echo.Context) error {
	ctx := c.Request().Context()
	m := r.stack
	packages := make([]StackInfo, 0, len(stackPackages))
	for _, name := range stackPackages {
		packages = append(packages, m.detect(ctx, name))
	}
	osr := m.osRelease()
	data := map[string]any{
		"packages":        packages,
		"os":              runtime.GOOS,
		"arch":            runtime.GOARCH,
		"distro":          osr.PrettyName,
		"distro_id":       osr.ID,
		"package_manager": detectPackageManager(m.sys),
		"php_versions":    m.phpVersions(ctx),
	}
	if op := m.current(); op != nil {
		data["operation"] = op.snapshot(4096)
	}
	return c.JSON(http.StatusOK, httpjson.Success{Data: data})
}

// stackOpTask reports a background operation.
func (r *Router) stackOpTask(c *echo.Context) error {
	op := r.stack.get(c.Param("id"))
	if op == nil {
		return c.JSON(http.StatusNotFound, map[string]any{"ok": false, "message": "operation not found (the agent may have restarted)"})
	}
	return c.JSON(http.StatusOK, op.snapshot(64<<10))
}

// stackInstallTask handles every stack action.
func (r *Router) stackInstallTask(c *echo.Context) error {
	ctx := c.Request().Context()
	m := r.stack

	var req StackRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"ok": false, "message": "invalid payload"})
	}
	if req.Package == "" {
		return c.JSON(http.StatusBadRequest, map[string]any{"ok": false, "message": "package is required"})
	}
	if req.Package == "mysql" {
		req.Package = "mariadb"
	}
	if req.Action == "" {
		req.Action = "install"
	}
	if req.Version != "" && !stackVersionRe.MatchString(req.Version) {
		return c.JSON(http.StatusBadRequest, map[string]any{"ok": false, "message": "invalid version"})
	}
	known := false
	for _, p := range stackPackages {
		known = known || p == req.Package
	}
	if !known {
		return c.JSON(http.StatusBadRequest, map[string]any{"ok": false, "message": "unknown package: " + req.Package})
	}

	switch req.Action {
	case "list-versions":
		return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]any{"versions": m.listInstalled(ctx, req.Package)}})
	case "list-available":
		return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]any{"versions": m.listAvailable(ctx, req.Package)}})
	case "list-extensions":
		if req.Package != "php" || !phpVersionArgRe.MatchString(req.Version) {
			return c.JSON(http.StatusBadRequest, map[string]any{"ok": false, "message": "list-extensions needs package php and a version"})
		}
		ext, err := m.phpListExtensions(ctx, req.Version)
		if err != nil {
			return stackJSONError(c, err)
		}
		return c.JSON(http.StatusOK, httpjson.Success{Data: ext})
	case "set-default":
		if req.Package != "php" || !phpVersionArgRe.MatchString(req.Version) {
			return c.JSON(http.StatusBadRequest, map[string]any{"ok": false, "message": "set-default needs package php and a version"})
		}
		sctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		msg, err := m.phpSetDefault(sctx, nil, req.Version)
		if err != nil {
			return stackJSONError(c, err)
		}
		info := m.detect(ctx, "php")
		return c.JSON(http.StatusOK, StackResponse{OK: true, Message: msg, Version: info.Version, Info: &info})
	}

	if serviceStackActions[req.Action] {
		if req.Package == "php" && req.Version != "" && !phpVersionArgRe.MatchString(req.Version) {
			return c.JSON(http.StatusBadRequest, map[string]any{"ok": false, "message": "invalid PHP version"})
		}
		if req.Package == "php" && req.Version == "" && req.Action == "stop" {
			return c.JSON(http.StatusBadRequest, map[string]any{"ok": false, "message": "choose the PHP version whose PHP-FPM to stop"})
		}
		sctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
		defer cancel()
		msg, out, err := m.serviceAction(sctx, req.Package, req.Action, req.Version)
		info := m.detect(ctx, req.Package)
		if err != nil {
			var se *stackError
			if errors.As(err, &se) {
				return c.JSON(se.Code, map[string]any{"ok": false, "message": se.Msg, "output": out})
			}
			slog.Error("agent: stack service action failed", "package", req.Package, "action", req.Action, "error", err)
			return c.JSON(http.StatusOK, StackResponse{OK: false, Message: err.Error(), Output: out, Version: info.Version, Info: &info})
		}
		slog.Info("agent: stack service action ok", "package", req.Package, "action", req.Action, "version", req.Version)
		return c.JSON(http.StatusOK, StackResponse{OK: true, Message: msg, Output: out, Version: info.Version, Info: &info})
	}

	if !asyncStackActions[req.Action] {
		return c.JSON(http.StatusBadRequest, map[string]any{"ok": false, "message": "unknown action: " + req.Action})
	}
	if pm := detectPackageManager(m.sys); pm != "apt-get" && req.Package != "go" {
		return c.JSON(http.StatusNotImplemented, map[string]any{"ok": false, "message": "package operations are only supported on Debian/Ubuntu (apt); this server uses " + orUnknown(pm)})
	}
	fn, err := m.operation(req)
	if err != nil {
		return stackJSONError(c, err)
	}
	op, err := m.start(req, fn)
	if err != nil {
		return stackJSONError(c, err)
	}
	return c.JSON(http.StatusAccepted, op.snapshot(4096))
}

func orUnknown(s string) string {
	if s == "" {
		return "an unsupported package manager"
	}
	return s
}

func detectPackageManager(sys stackSystem) string {
	for _, pm := range []string{"apt-get", "dnf", "yum", "apk"} {
		if _, err := sys.LookPath(pm); err == nil {
			return pm
		}
	}
	return ""
}
