package apihttp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v5"
	"sigardapanel/internal/audit"
	"sigardapanel/internal/httpjson"
	"sigardapanel/internal/store"
)

// Stack page: installed software per server and package/service actions.
// Package operations (install, uninstall, update, extensions) run on the agent
// as background operations: the action answers 202 with an op_id and the UI
// polls GET /servers/:id/stack/ops/:op until it is done.

var stackAPIPackages = map[string]bool{
	"nginx": true, "php": true, "node": true, "python": true, "go": true, "docker": true,
	"certbot": true, "mariadb": true, "redis": true, "memcached": true, "supervisor": true,
}

var stackAPIActions = map[string]bool{
	"install": true, "uninstall": true, "update": true,
	"start": true, "stop": true, "restart": true, "reload": true,
	"set-default": true, "install-extension": true, "uninstall-extension": true,
	"list-versions": true, "list-available": true, "list-extensions": true,
}

var stackReadOnlyActions = map[string]bool{"list-versions": true, "list-available": true, "list-extensions": true}

var (
	stackAPIVersionRe = regexp.MustCompile(`^[0-9][0-9A-Za-z.]{0,15}$`)
	stackAPIExtRe     = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,40}$`)
	stackOpIDRe       = regexp.MustCompile(`^op_[0-9a-f]{16}$`)
)

type stackActionBody struct {
	Package   string `json:"package"`
	Action    string `json:"action"`
	Version   string `json:"version,omitempty"`
	Extension string `json:"extension,omitempty"`
}

// stackAgentRequest calls the agent with a bounded timeout.
func stackAgentRequest(ctx context.Context, method, url, token string, body any, timeout time.Duration) (int, map[string]any, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		reader = strings.NewReader(string(b))
	}
	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := agentClient(0).Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return resp.StatusCode, nil, fmt.Errorf("invalid agent response (HTTP %d)", resp.StatusCode)
	}
	return resp.StatusCode, out, nil
}

// agentHTTPError turns an agent error answer into the API's error. A 401/403
// from the agent means it rejected the panel's token; passing that status
// through would look like the user's own session expired.
func agentHTTPError(code int, out map[string]any) error {
	if code == http.StatusUnauthorized || code == http.StatusForbidden {
		return echo.NewHTTPError(http.StatusBadGateway, "the server's agent rejected the panel's token (regenerate it under Servers)")
	}
	if msg, _ := out["message"].(string); code == http.StatusNotFound && (msg == "" || msg == "Not Found") {
		return echo.NewHTTPError(http.StatusBadGateway, "the agent on this server does not support this action; update the agent")
	}
	return echo.NewHTTPError(code, agentMessage(out, "agent error "+strconv.Itoa(code)))
}

// agentMessage extracts a human message from an agent error body.
func agentMessage(m map[string]any, fallback string) string {
	for _, k := range []string{"message", "error"} {
		if s, ok := m[k].(string); ok && strings.TrimSpace(s) != "" {
			return s
		}
	}
	return fallback
}

// unwrapData returns m["data"] when the agent wrapped its answer.
func unwrapData(m map[string]any) any {
	if d, ok := m["data"]; ok && len(m) <= 2 {
		return d
	}
	return m
}

func agentBase(s *store.Server) string { return strings.TrimRight(s.AgentURL.String, "/") }

// stackInfo returns installed software and versions from the agent.
func (h *stackHandler) stackInfo(c *echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	server, err := h.resolveServer(c)
	if err != nil {
		return err
	}
	code, out, err := stackAgentRequest(c.Request().Context(), http.MethodGet, agentBase(server)+"/tasks/stack/info", server.AgentToken, nil, 90*time.Second)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "agent unreachable: "+err.Error())
	}
	if code >= 400 {
		return agentHTTPError(code, out)
	}
	data, _ := unwrapData(out).(map[string]any)
	if data == nil {
		data = map[string]any{}
	}
	data["is_local"] = isLocalServer(*server)
	return c.JSON(http.StatusOK, httpjson.Success{Data: data, RequestID: rid})
}

// stackAction validates, guards and forwards a stack action.
func (h *stackHandler) stackAction(c *echo.Context) error {
	var body stackActionBody
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	return h.runStackAction(c, body)
}

// stackRestart restarts a component's service (kept for API compatibility).
func (h *stackHandler) stackRestart(c *echo.Context) error {
	var body stackActionBody
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	body.Action = "restart"
	return h.runStackAction(c, body)
}

func (h *stackHandler) runStackAction(c *echo.Context, body stackActionBody) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	ctx := c.Request().Context()
	server, err := h.resolveServer(c)
	if err != nil {
		return err
	}
	body.Package = strings.TrimSpace(strings.ToLower(body.Package))
	body.Action = strings.TrimSpace(strings.ToLower(body.Action))
	body.Version = strings.TrimSpace(body.Version)
	body.Extension = strings.TrimSpace(strings.ToLower(body.Extension))
	if body.Package == "mysql" {
		body.Package = "mariadb"
	}
	if !stackAPIPackages[body.Package] {
		return echo.NewHTTPError(http.StatusBadRequest, "unknown package: "+body.Package)
	}
	if !stackAPIActions[body.Action] {
		return echo.NewHTTPError(http.StatusBadRequest, "unknown action: "+body.Action)
	}
	if body.Version != "" && !stackAPIVersionRe.MatchString(body.Version) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid version")
	}
	if body.Extension != "" && !stackAPIExtRe.MatchString(body.Extension) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid extension")
	}

	sess := sessionFromEcho(c)
	var userID int64
	if sess != nil {
		userID = sess.UserID
	}
	mutating := !stackReadOnlyActions[body.Action]
	if mutating {
		if err := h.stackGuard(ctx, server, sess, body); err != nil {
			if userID > 0 {
				h.audit.RecordAction(ctx, userID, "stack_"+body.Action+"_refused", audit.ResourceServer, server.ID, map[string]any{
					"package": body.Package, "version": body.Version, "extension": body.Extension, "reason": err.Error(),
				})
			}
			return err
		}
	}

	timeout := 30 * time.Second
	if body.Action == "start" || body.Action == "stop" || body.Action == "restart" || body.Action == "reload" || body.Action == "set-default" {
		timeout = 4 * time.Minute
	}
	payload := map[string]string{"package": body.Package, "action": body.Action, "version": body.Version, "extension": body.Extension}
	code, out, err := stackAgentRequest(ctx, http.MethodPost, agentBase(server)+"/tasks/stack/install", server.AgentToken, payload, timeout)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "agent unreachable: "+err.Error())
	}
	if mutating && userID > 0 {
		details := map[string]any{"package": body.Package, "version": body.Version}
		if body.Extension != "" {
			details["extension"] = body.Extension
		}
		if op, ok := out["op_id"].(string); ok {
			details["op_id"] = op
		}
		if code >= 400 {
			details["refused"] = agentMessage(out, http.StatusText(code))
		} else if ok, isBool := out["ok"].(bool); isBool && code != http.StatusAccepted {
			details["ok"] = ok
			details["message"] = agentMessage(out, "")
		}
		h.audit.RecordAction(ctx, userID, "stack_"+body.Action, audit.ResourceServer, server.ID, details)
	}
	if code >= 400 {
		return agentHTTPError(code, out)
	}
	if stackReadOnlyActions[body.Action] {
		return c.JSON(http.StatusOK, httpjson.Success{Data: unwrapData(out), RequestID: rid})
	}
	return c.JSON(code, httpjson.Success{Data: out, RequestID: rid})
}

// stackGuard refuses actions that would break sites, the panel itself, or
// that need more privileges than the caller has.
func (h *stackHandler) stackGuard(ctx context.Context, server *store.Server, sess *store.Session, body stackActionBody) error {
	local := isLocalServer(*server)
	destructive := body.Action == "uninstall" || body.Action == "stop" || body.Action == "uninstall-extension"
	if local && destructive && !isSessionSuperAdmin(ctx, h.store, sess) {
		return echo.NewHTTPError(http.StatusForbidden, "only a super_admin can uninstall or stop software on the panel's own server")
	}
	if local && body.Package == "nginx" && (body.Action == "uninstall" || body.Action == "stop") {
		return echo.NewHTTPError(http.StatusConflict, "nginx serves this panel; it cannot be stopped or uninstalled from the panel")
	}
	if local && body.Package == "certbot" && body.Action == "uninstall" {
		return echo.NewHTTPError(http.StatusConflict, "certbot renews this panel's own certificate; it cannot be uninstalled from the panel")
	}
	if body.Action != "uninstall" {
		return nil
	}

	sites, err := h.store.Sites.List(ctx, store.SiteListFilter{ServerID: server.ID, Limit: 100000})
	if err != nil {
		slog.Error("api: stack guard: list sites", "server", server.ID, "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "could not check which sites use this package")
	}
	var users []string
	pick := func(s store.Site) { users = append(users, s.Domain) }
	switch body.Package {
	case "php":
		if body.Version == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "choose the PHP version to uninstall")
		}
		for _, s := range sites {
			if s.Runtime == "php" && s.PHPVersion.Valid && s.PHPVersion.String == body.Version {
				pick(s)
			}
		}
		if len(users) > 0 {
			return echo.NewHTTPError(http.StatusConflict, fmt.Sprintf("PHP %s is used by %d site(s): %s. Switch them to another PHP version (site Settings → PHP) before uninstalling it",
				body.Version, len(users), listSome(users)))
		}
	case "nginx":
		for _, s := range sites {
			pick(s)
		}
		if len(users) > 0 {
			return echo.NewHTTPError(http.StatusConflict, fmt.Sprintf("nginx serves %d site(s) on this server: %s. Delete or move them first", len(users), listSome(users)))
		}
	case "node", "docker":
		for _, s := range sites {
			if s.Runtime == body.Package {
				pick(s)
			}
		}
		if len(users) > 0 {
			return echo.NewHTTPError(http.StatusConflict, fmt.Sprintf("%d %s site(s) run on this server: %s. Delete or move them first", len(users), body.Package, listSome(users)))
		}
	case "certbot":
		for _, s := range sites {
			if st := strings.ToLower(s.SSLStatus.String); s.SSLStatus.Valid && st != "" && st != "none" {
				pick(s)
			}
		}
		if len(users) > 0 {
			return echo.NewHTTPError(http.StatusConflict, fmt.Sprintf("certbot renews the certificates of %d site(s): %s. Uninstalling it would let them expire", len(users), listSome(users)))
		}
	case "mariadb":
		var dbs []string
		for _, s := range sites {
			list, err := h.store.Databases.ListBySite(ctx, s.ID)
			if err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, "could not check the server's databases")
			}
			for _, d := range list {
				dbs = append(dbs, d.Name)
			}
		}
		if len(dbs) > 0 {
			return echo.NewHTTPError(http.StatusConflict, fmt.Sprintf("this server hosts %d database(s) managed by the panel: %s. Delete them first", len(dbs), listSome(dbs)))
		}
	}
	return nil
}

func listSome(items []string) string {
	if len(items) <= 8 {
		return strings.Join(items, ", ")
	}
	return strings.Join(items[:8], ", ") + fmt.Sprintf(" and %d more", len(items)-8)
}

// finishedOps remembers operations whose result was already audited.
var finishedOps sync.Map

// stackOp reports a background stack operation.
func (h *stackHandler) stackOp(c *echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	server, err := h.resolveServer(c)
	if err != nil {
		return err
	}
	opID := c.Param("op")
	if !stackOpIDRe.MatchString(opID) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid operation id")
	}
	code, out, err := stackAgentRequest(c.Request().Context(), http.MethodGet, agentBase(server)+"/tasks/stack/ops/"+opID, server.AgentToken, nil, 20*time.Second)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "agent unreachable: "+err.Error())
	}
	if code >= 400 {
		return agentHTTPError(code, out)
	}
	if done, _ := out["done"].(bool); done {
		key := strconv.FormatInt(server.ID, 10) + "/" + opID
		if _, seen := finishedOps.LoadOrStore(key, true); !seen {
			var uid int64
			if sess := sessionFromEcho(c); sess != nil {
				uid = sess.UserID
			}
			action, _ := out["action"].(string)
			h.audit.RecordAction(c.Request().Context(), uid, "stack_"+action+"_result", audit.ResourceServer, server.ID, map[string]any{
				"op_id": opID, "package": out["package"], "version": out["version"], "extension": out["extension"],
				"status": out["status"], "message": out["message"], "duration_ms": out["duration_ms"],
			})
		}
	}
	return c.JSON(http.StatusOK, httpjson.Success{Data: out, RequestID: rid})
}

// serverPHPVersions asks the server's agent which PHP versions have PHP-FPM.
func serverPHPVersions(ctx context.Context, server *store.Server) ([]map[string]any, error) {
	if !server.AgentURL.Valid || server.AgentURL.String == "" || server.AgentToken == "" {
		return nil, fmt.Errorf("agent not configured")
	}
	code, out, err := stackAgentRequest(ctx, http.MethodPost, agentBase(server)+"/tasks/stack/install", server.AgentToken,
		map[string]string{"package": "php", "action": "list-versions"}, 30*time.Second)
	if err != nil {
		return nil, err
	}
	if code >= 400 {
		return nil, fmt.Errorf("agent: %s", agentMessage(out, http.StatusText(code)))
	}
	data, _ := unwrapData(out).(map[string]any)
	list, _ := data["versions"].([]any)
	res := []map[string]any{}
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		// Older agents do not report fpm; treat their versions as usable.
		if fpm, has := m["fpm"].(bool); has && !fpm {
			continue
		}
		res = append(res, m)
	}
	return res, nil
}

// phpVersions lists the PHP-FPM versions installed on a server, for the site
// forms (any signed-in user; detection only, nothing is installed).
func (h *stackHandler) phpVersions(c *echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	server, err := h.resolveServer(c)
	if err != nil {
		return err
	}
	list, err := serverPHPVersions(c.Request().Context(), server)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "could not read PHP versions: "+err.Error())
	}
	versions := make([]string, 0, len(list))
	for _, m := range list {
		if v, ok := m["version"].(string); ok {
			versions = append(versions, v)
		}
	}
	return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]any{"versions": versions, "details": list}, RequestID: rid})
}

// commandExec executes a shell command on the server via the agent.
// POST /api/v1/servers/:id/command
func (h *stackHandler) commandExec(c *echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	ctx := c.Request().Context()
	server, err := h.resolveServer(c)
	if err != nil {
		return err
	}
	var body struct {
		Command string `json:"command"`
		Timeout int    `json:"timeout"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid body")
	}
	if strings.TrimSpace(body.Command) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "command is required")
	}
	if len(body.Command) > 8192 {
		return echo.NewHTTPError(http.StatusBadRequest, "command is too long")
	}
	sess := sessionFromEcho(c)
	// A root shell on the panel host reaches the panel database and keys:
	// that is super_admin power.
	if isLocalServer(*server) && !isSessionSuperAdmin(ctx, h.store, sess) {
		return echo.NewHTTPError(http.StatusForbidden, "only a super_admin can run commands on the panel's own server")
	}
	if body.Timeout <= 0 || body.Timeout > 300 {
		body.Timeout = 60
	}
	if sess != nil {
		cmd := body.Command
		if len(cmd) > 500 {
			cmd = cmd[:500] + "…"
		}
		h.audit.RecordAction(ctx, sess.UserID, "server_command_exec", audit.ResourceServer, server.ID, map[string]any{
			"command": cmd, "ip": c.RealIP(),
		})
	}
	code, out, err := stackAgentRequest(ctx, http.MethodPost, agentBase(server)+"/tasks/command/exec", server.AgentToken,
		map[string]any{"command": body.Command, "timeout": body.Timeout}, time.Duration(body.Timeout+15)*time.Second)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "agent request failed: "+err.Error())
	}
	if code >= 400 {
		return agentHTTPError(code, out)
	}
	return c.JSON(http.StatusOK, httpjson.Success{Data: out, RequestID: rid})
}

// agentProxyResponse answers a proxied agent call. An agent error (HTTP
// status >= 400) becomes an API error, and so does {"ok": false} on a
// state-changing request; the web UI only treats responses with an "error"
// key as failures, so these used to show up as successes ("Container
// started", "IP banned") although nothing happened. GET answers with
// ok:false are passed through: pages show them as "not installed" states.
func agentProxyResponse(c *echo.Context, rid string, status int, result map[string]any) error {
	if status >= 400 {
		if result == nil {
			result = map[string]any{}
		}
		return agentHTTPError(status, result)
	}
	if c.Request().Method != http.MethodGet {
		if ok, isBool := result["ok"].(bool); isBool && !ok {
			return echo.NewHTTPError(http.StatusUnprocessableEntity, agentMessage(result, "the server's agent reported a failure"))
		}
	}
	return c.JSON(status, httpjson.Success{Data: result, RequestID: rid})
}
