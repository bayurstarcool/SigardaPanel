package apihttp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"sigardapanel/internal/audit"
	"sigardapanel/internal/auth"
	"sigardapanel/internal/httpjson"
	"sigardapanel/internal/store"
)

type stackHandler struct {
	store *store.Store
	audit *audit.Recorder
}

func newStackHandler(s *store.Store, a *audit.Recorder) *stackHandler {
	return &stackHandler{store: s, audit: a}
}

// stackInfo returns installed software and versions from the agent.
func (h *stackHandler) stackInfo(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid server id")
	}

	server, err := h.store.Servers.GetByID(c.Request().Context(), id)
	if err != nil || server == nil {
		return echo.NewHTTPError(http.StatusNotFound, "server not found")
	}
	if !server.AgentURL.Valid || server.AgentURL.String == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "agent URL not configured")
	}
	if server.AgentToken == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "agent token not configured")
	}

	// Call agent GET /tasks/stack/info
	agentURL := strings.TrimRight(server.AgentURL.String, "/") + "/tasks/stack/info"
	resp, err := agentRequest("GET", agentURL, server.AgentToken, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "agent unreachable")
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return echo.NewHTTPError(resp.StatusCode, "agent error: "+string(body))
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "invalid agent response")
	}

	return c.JSON(http.StatusOK, httpjson.Success{Data: result, RequestID: rid})
}

// stackAction performs an action (install, uninstall, restart, etc.) on a package.
func (h *stackHandler) stackAction(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)
	var userID int64
	if sess != nil {
		userID = sess.UserID
	}

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid server id")
	}

	server, err := h.store.Servers.GetByID(c.Request().Context(), id)
	if err != nil || server == nil {
		return echo.NewHTTPError(http.StatusNotFound, "server not found")
	}
	if !server.AgentURL.Valid || server.AgentURL.String == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "agent URL not configured")
	}
	if server.AgentToken == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "agent token not configured")
	}

	// Parse request body
	var body struct {
		Package string `json:"package"`
		Action  string `json:"action"`
		Version string `json:"version,omitempty"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if body.Package == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "package is required")
	}
	if body.Action == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "action is required")
	}

	// Call agent POST /tasks/stack/install
	agentURL := strings.TrimRight(server.AgentURL.String, "/") + "/tasks/stack/install"
	payload := map[string]string{
		"package": body.Package,
		"action":  body.Action,
		"version": body.Version,
	}
	resp, err := agentRequest("POST", agentURL, server.AgentToken, payload)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "agent unreachable")
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return echo.NewHTTPError(resp.StatusCode, "agent error: "+string(respBody))
	}

	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "invalid agent response")
	}

	// Audit log
	h.audit.RecordAction(c.Request().Context(), userID, audit.ActionServerUpdate, audit.ResourceServer, id, map[string]any{
		"action":  "stack_" + body.Action,
		"package": body.Package,
		"version": body.Version,
	})

	return c.JSON(http.StatusOK, httpjson.Success{Data: result, RequestID: rid})
}

// stackRestart restarts a specific service on the server.
func (h *stackHandler) stackRestart(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)
	var userID int64
	if sess != nil {
		userID = sess.UserID
	}

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid server id")
	}

	server, err := h.store.Servers.GetByID(c.Request().Context(), id)
	if err != nil || server == nil {
		return echo.NewHTTPError(http.StatusNotFound, "server not found")
	}
	if !server.AgentURL.Valid || server.AgentURL.String == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "agent URL not configured")
	}
	if server.AgentToken == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "agent token not configured")
	}

	var body struct {
		Package string `json:"package"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if body.Package == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "package is required")
	}

	agentURL := strings.TrimRight(server.AgentURL.String, "/") + "/tasks/stack/install"
	payload := map[string]string{
		"package": body.Package,
		"action":  "restart",
	}
	resp, err := agentRequest("POST", agentURL, server.AgentToken, payload)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "agent unreachable")
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "invalid agent response")
	}

	h.audit.RecordAction(c.Request().Context(), userID, audit.ActionServerUpdate, audit.ResourceServer, id, map[string]any{
		"action":  "stack_restart",
		"package": body.Package,
	})

	return c.JSON(http.StatusOK, httpjson.Success{Data: result, RequestID: rid})
}

// commandExec executes a shell command on the server via the agent.
// POST /api/v1/servers/:id/command
func (h *stackHandler) commandExec(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid server id")
	}
	server, err := h.store.Servers.GetByID(c.Request().Context(), id)
	if err != nil || server == nil {
		return echo.NewHTTPError(http.StatusNotFound, "server not found")
	}
	if !server.AgentURL.Valid || server.AgentURL.String == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "agent URL not configured")
	}
	if server.AgentToken == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "agent token not configured")
	}
	var body struct {
		Command string `json:"command"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid body")
	}
	if body.Command == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "command is required")
	}
	if sess := sessionFromEcho(c); sess != nil {
		cmd := body.Command
		if len(cmd) > 500 {
			cmd = cmd[:500] + "…"
		}
		h.audit.RecordAction(c.Request().Context(), sess.UserID, "server_command_exec", audit.ResourceServer, id, map[string]any{
			"command": cmd, "ip": c.RealIP(),
		})
	}
	agentURL := server.AgentURL.String + "/tasks/command/exec"
	resp, err := agentRequest("POST", agentURL, server.AgentToken, map[string]string{"command": body.Command})
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "agent request failed: "+err.Error())
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]any
	json.Unmarshal(respBody, &result)
	return c.JSON(resp.StatusCode, httpjson.Success{Data: result, RequestID: rid})
}

// ─── Docker Container Management Proxy Handlers ──────────────────────

// dockerContainers proxies to agent GET /tasks/docker/containers.
func (h *stackHandler) dockerContainers(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	server, err := h.resolveServer(c)
	if err != nil {
		return err
	}
	agentURL := strings.TrimRight(server.AgentURL.String, "/") + "/tasks/docker/containers"
	resp, err := agentRequest("GET", agentURL, server.AgentToken, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "agent unreachable")
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]any
	json.Unmarshal(respBody, &result)
	if allowed, restricted := h.tenantContainerNames(c); restricted {
		result = filterContainers(result, allowed)
	}
	return c.JSON(resp.StatusCode, httpjson.Success{Data: result, RequestID: rid})
}

// tenantContainerNames returns the docker container names a non-admin user may
// see (the containers of sites they own or are assigned to). restricted is
// false for admins, who see everything.
func (h *stackHandler) tenantContainerNames(c echo.Context) (allowed map[string]bool, restricted bool) {
	sess := sessionFromEcho(c)
	ctx := c.Request().Context()
	if sess == nil {
		return map[string]bool{}, true
	}
	user, _ := h.store.Users.GetByID(ctx, sess.UserID)
	if user != nil && auth.HasRole(user.Role, auth.RoleAdmin) {
		return nil, false
	}
	allowed = map[string]bool{}
	add := func(site *store.Site) {
		if site != nil {
			allowed["site-"+strings.ReplaceAll(site.Name, " ", "-")] = true
		}
	}
	if owned, err := h.store.Sites.List(ctx, store.SiteListFilter{OwnerID: sess.UserID}); err == nil {
		for i := range owned {
			add(&owned[i])
		}
	}
	if assigned, err := h.store.UserSites.ListByUser(ctx, sess.UserID); err == nil {
		for _, us := range assigned {
			if site, err := h.store.Sites.GetByID(ctx, us.SiteID); err == nil {
				add(site)
			}
		}
	}
	return allowed, true
}

func filterContainers(result map[string]any, allowed map[string]bool) map[string]any {
	list, _ := result["containers"].([]any)
	kept := make([]any, 0, len(list))
	for _, item := range list {
		if m, ok := item.(map[string]any); ok {
			if name, _ := m["name"].(string); allowed[strings.TrimPrefix(name, "/")] {
				kept = append(kept, item)
			}
		}
	}
	return map[string]any{"containers": kept}
}

// containerBelongsTo asks the agent for the container list and checks that
// containerID (id or name) resolves to one of the allowed names.
func (h *stackHandler) containerBelongsTo(server *store.Server, containerID string, allowed map[string]bool) bool {
	resp, err := agentRequest("GET", strings.TrimRight(server.AgentURL.String, "/")+"/tasks/docker/containers", server.AgentToken, nil)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	var result map[string]any
	body, _ := io.ReadAll(resp.Body)
	if json.Unmarshal(body, &result) != nil {
		return false
	}
	list, _ := result["containers"].([]any)
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		id, _ := m["id"].(string)
		name, _ := m["name"].(string)
		name = strings.TrimPrefix(name, "/")
		if allowed[name] && containerID != "" && (containerID == name || (id != "" && (strings.HasPrefix(id, containerID) || strings.HasPrefix(containerID, id)))) {
			return true
		}
	}
	return false
}

// dockerContainerStart proxies to agent POST /tasks/docker/container/start.
func (h *stackHandler) dockerContainerStart(c echo.Context) error {
	return h.dockerContainerProxy(c, "start")
}

// dockerContainerStop proxies to agent POST /tasks/docker/container/stop.
func (h *stackHandler) dockerContainerStop(c echo.Context) error {
	return h.dockerContainerProxy(c, "stop")
}

// dockerContainerRestart proxies to agent POST /tasks/docker/container/restart.
func (h *stackHandler) dockerContainerRestart(c echo.Context) error {
	return h.dockerContainerProxy(c, "restart")
}

// dockerContainerRemove proxies to agent POST /tasks/docker/container/remove.
func (h *stackHandler) dockerContainerRemove(c echo.Context) error {
	return h.dockerContainerProxy(c, "remove")
}

// dockerContainerCreate proxies to agent POST /tasks/docker/container/create.
func (h *stackHandler) dockerContainerCreate(c echo.Context) error {
	return h.dockerContainerProxy(c, "create")
}

// dockerContainerLogs proxies to agent GET /tasks/docker/container/logs.
func (h *stackHandler) dockerContainerLogs(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	server, err := h.resolveServer(c)
	if err != nil {
		return err
	}
	containerID := c.QueryParam("container_id")
	lines := c.QueryParam("lines")
	if lines == "" {
		lines = "100"
	}
	if n, err := strconv.Atoi(lines); err != nil || n < 1 || n > 5000 {
		lines = "100"
	}
	if allowed, restricted := h.tenantContainerNames(c); restricted {
		if !h.containerBelongsTo(server, containerID, allowed) {
			return echo.NewHTTPError(http.StatusNotFound, "container not found")
		}
	}
	agentURL := strings.TrimRight(server.AgentURL.String, "/") + "/tasks/docker/container/logs?container_id=" + url.QueryEscape(containerID) + "&lines=" + lines
	resp, err := agentRequest("GET", agentURL, server.AgentToken, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "agent unreachable")
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]any
	json.Unmarshal(respBody, &result)
	return c.JSON(resp.StatusCode, httpjson.Success{Data: result, RequestID: rid})
}

// dockerContainerExec proxies to agent POST /tasks/docker/container/exec.
func (h *stackHandler) dockerContainerExec(c echo.Context) error {
	return h.dockerContainerProxy(c, "exec")
}

// dockerImages proxies to agent GET /tasks/docker/images.
func (h *stackHandler) dockerImages(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	server, err := h.resolveServer(c)
	if err != nil {
		return err
	}
	agentURL := strings.TrimRight(server.AgentURL.String, "/") + "/tasks/docker/images"
	resp, err := agentRequest("GET", agentURL, server.AgentToken, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "agent unreachable")
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]any
	json.Unmarshal(respBody, &result)
	return c.JSON(resp.StatusCode, httpjson.Success{Data: result, RequestID: rid})
}

// dockerSearchImages queries Docker Hub for image suggestions.
func (h *stackHandler) dockerSearchImages(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	query := c.QueryParam("q")
	if query == "" {
		return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]any{"results": []any{}}, RequestID: rid})
	}
	pageSize := c.QueryParam("page_size")
	if pageSize == "" {
		pageSize = "10"
	}

	hubURL := fmt.Sprintf("https://hub.docker.com/v2/search/repositories/?query=%s&page_size=%s", url.QueryEscape(query), pageSize)
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", hubURL, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to build request")
	}
	req.Header.Set("User-Agent", "SigardaPanel/0.5")

	resp, err := client.Do(req)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "docker hub unreachable: "+err.Error())
	}
	defer resp.Body.Close()

	var hubResp struct {
		Results []struct {
			RepoName    string `json:"repo_name"`
			ShortDesc   string `json:"short_description"`
			StarCount   int64  `json:"star_count"`
			PullCount   int64  `json:"pull_count"`
			IsOfficial  bool   `json:"is_official"`
			IsAutomated bool   `json:"is_automated"`
			Owner       string `json:"owner"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&hubResp); err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "failed to parse docker hub response")
	}

	type imageResult struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Stars       int64  `json:"stars"`
		Pulls       int64  `json:"pulls"`
		Official    bool   `json:"official"`
		Automated   bool   `json:"automated"`
	}
	results := make([]imageResult, 0, len(hubResp.Results))
	for _, r := range hubResp.Results {
		name := r.RepoName
		if r.IsOfficial {
			name = r.RepoName
		}
		results = append(results, imageResult{
			Name:        name,
			Description: r.ShortDesc,
			Stars:       r.StarCount,
			Pulls:       r.PullCount,
			Official:    r.IsOfficial,
			Automated:   r.IsAutomated,
		})
	}
	return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]any{"results": results}, RequestID: rid})
}

// dockerComposeUp proxies to agent POST /tasks/docker/compose/up.
func (h *stackHandler) dockerComposeUp(c echo.Context) error {
	return h.dockerContainerProxy(c, "compose-up")
}

// dockerComposeDown proxies to agent POST /tasks/docker/compose/down.
func (h *stackHandler) dockerComposeDown(c echo.Context) error {
	return h.dockerContainerProxy(c, "compose-down")
}

// dockerImagePull proxies to agent POST /tasks/docker/image/pull.
func (h *stackHandler) dockerImagePull(c echo.Context) error {
	return h.dockerContainerProxy(c, "image/pull")
}

// dockerImageRemove proxies to agent POST /tasks/docker/image/remove.
func (h *stackHandler) dockerImageRemove(c echo.Context) error {
	return h.dockerContainerProxy(c, "image/remove")
}

// dockerVolumes proxies to agent GET /tasks/docker/volumes.
func (h *stackHandler) dockerVolumes(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	server, err := h.resolveServer(c)
	if err != nil {
		return err
	}
	agentURL := strings.TrimRight(server.AgentURL.String, "/") + "/tasks/docker/volumes"
	resp, err := agentRequest("GET", agentURL, server.AgentToken, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "agent unreachable")
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]any
	json.Unmarshal(respBody, &result)
	return c.JSON(resp.StatusCode, httpjson.Success{Data: result, RequestID: rid})
}

// dockerVolumeCreate proxies to agent POST /tasks/docker/volume/create.
func (h *stackHandler) dockerVolumeCreate(c echo.Context) error {
	return h.dockerContainerProxy(c, "volume/create")
}

// dockerVolumeRemove proxies to agent POST /tasks/docker/volume/remove.
func (h *stackHandler) dockerVolumeRemove(c echo.Context) error {
	return h.dockerContainerProxy(c, "volume/remove")
}

// dockerNetworks proxies to agent GET /tasks/docker/networks.
func (h *stackHandler) dockerNetworks(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	server, err := h.resolveServer(c)
	if err != nil {
		return err
	}
	agentURL := strings.TrimRight(server.AgentURL.String, "/") + "/tasks/docker/networks"
	resp, err := agentRequest("GET", agentURL, server.AgentToken, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "agent unreachable")
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]any
	json.Unmarshal(respBody, &result)
	return c.JSON(resp.StatusCode, httpjson.Success{Data: result, RequestID: rid})
}

// dockerNetworkCreate proxies to agent POST /tasks/docker/network/create.
func (h *stackHandler) dockerNetworkCreate(c echo.Context) error {
	return h.dockerContainerProxy(c, "network/create")
}

// dockerNetworkRemove proxies to agent POST /tasks/docker/network/remove.
func (h *stackHandler) dockerNetworkRemove(c echo.Context) error {
	return h.dockerContainerProxy(c, "network/remove")
}

// dockerInfo proxies to agent GET /tasks/docker/info.
func (h *stackHandler) dockerInfo(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	server, err := h.resolveServer(c)
	if err != nil {
		return err
	}
	agentURL := strings.TrimRight(server.AgentURL.String, "/") + "/tasks/docker/info"
	resp, err := agentRequest("GET", agentURL, server.AgentToken, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "agent unreachable")
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]any
	json.Unmarshal(respBody, &result)
	return c.JSON(resp.StatusCode, httpjson.Success{Data: result, RequestID: rid})
}

// dockerDiskUsage proxies to agent GET /tasks/docker/disk-usage.
func (h *stackHandler) dockerDiskUsage(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	server, err := h.resolveServer(c)
	if err != nil {
		return err
	}
	agentURL := strings.TrimRight(server.AgentURL.String, "/") + "/tasks/docker/disk-usage"
	resp, err := agentRequest("GET", agentURL, server.AgentToken, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "agent unreachable")
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]any
	json.Unmarshal(respBody, &result)
	return c.JSON(resp.StatusCode, httpjson.Success{Data: result, RequestID: rid})
}

// pm2Manage proxies to agent POST /tasks/pm2/manage.
func (h *stackHandler) pm2Manage(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	server, err := h.resolveServer(c)
	if err != nil {
		return err
	}

	// Read and forward body
	reqBody, _ := io.ReadAll(c.Request().Body)
	var body map[string]any
	json.Unmarshal(reqBody, &body)

	agentURL := strings.TrimRight(server.AgentURL.String, "/") + "/tasks/pm2/manage"
	resp, err := agentRequest("POST", agentURL, server.AgentToken, body)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "agent unreachable")
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]any
	json.Unmarshal(respBody, &result)
	return c.JSON(resp.StatusCode, httpjson.Success{Data: result, RequestID: rid})
}

// emailManage proxies mail-server actions (status, list, install, …) to the
// agent's /tasks/email/manage. Admin only, like pm2 and bot-blocker.
func (h *stackHandler) emailManage(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	server, err := h.resolveServer(c)
	if err != nil {
		return err
	}

	var body map[string]any
	if err := json.NewDecoder(c.Request().Body).Decode(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid JSON body")
	}

	agentURL := strings.TrimRight(server.AgentURL.String, "/") + "/tasks/email/manage"
	resp, err := agentRequest("POST", agentURL, server.AgentToken, body)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "agent unreachable")
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]any
	json.Unmarshal(respBody, &result)
	return c.JSON(resp.StatusCode, httpjson.Success{Data: result, RequestID: rid})
}

// dockerContainerProxy is a generic proxy for Docker container actions.
func (h *stackHandler) dockerContainerProxy(c echo.Context, action string) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	server, err := h.resolveServer(c)
	if err != nil {
		return err
	}

	// Read and forward body
	reqBody, _ := io.ReadAll(c.Request().Body)
	var body map[string]any
	json.Unmarshal(reqBody, &body)

	agentPath := "/tasks/docker/container/" + action
	if strings.Contains(action, "/") && !strings.HasPrefix(action, "compose-") {
		// volume/create, volume/remove, network/create, network/remove,
		// image/pull, image/remove → all map to /tasks/docker/<action>
		agentPath = "/tasks/docker/" + action
	} else if action == "compose-up" {
		agentPath = "/tasks/docker/compose/up"
	} else if action == "compose-down" {
		agentPath = "/tasks/docker/compose/down"
	}

	agentURL := strings.TrimRight(server.AgentURL.String, "/") + agentPath
	resp, err := agentRequest("POST", agentURL, server.AgentToken, body)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "agent unreachable")
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]any
	json.Unmarshal(respBody, &result)
	return c.JSON(resp.StatusCode, httpjson.Success{Data: result, RequestID: rid})
}

// resolveServer extracts server ID from URL params and resolves agent info.
func (h *stackHandler) resolveServer(c echo.Context) (*store.Server, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return nil, echo.NewHTTPError(http.StatusBadRequest, "invalid server id")
	}
	server, err := h.store.Servers.GetByID(c.Request().Context(), id)
	if err != nil || server == nil {
		return nil, echo.NewHTTPError(http.StatusNotFound, "server not found")
	}
	if !server.AgentURL.Valid || server.AgentURL.String == "" {
		return nil, echo.NewHTTPError(http.StatusBadRequest, "agent URL not configured")
	}
	if server.AgentToken == "" {
		return nil, echo.NewHTTPError(http.StatusBadRequest, "agent token not configured")
	}
	return server, nil
}

// ─── Firewall (UFW) Proxy Handlers ──────────────────────────────────────

// firewallStatus proxies to agent GET /tasks/firewall/status.
func (h *stackHandler) firewallStatus(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	server, err := h.resolveServer(c)
	if err != nil {
		return err
	}
	agentURL := strings.TrimRight(server.AgentURL.String, "/") + "/tasks/firewall/status"
	resp, err := agentRequest("GET", agentURL, server.AgentToken, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "agent unreachable")
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]any
	json.Unmarshal(respBody, &result)
	return c.JSON(resp.StatusCode, httpjson.Success{Data: result, RequestID: rid})
}

// firewallEnable proxies to agent POST /tasks/firewall/enable.
func (h *stackHandler) firewallEnable(c echo.Context) error {
	return h.firewallProxy(c, "enable")
}

// firewallDisable proxies to agent POST /tasks/firewall/disable.
func (h *stackHandler) firewallDisable(c echo.Context) error {
	return h.firewallProxy(c, "disable")
}

// firewallAllow proxies to agent POST /tasks/firewall/allow.
func (h *stackHandler) firewallAllow(c echo.Context) error {
	return h.firewallProxy(c, "allow")
}

// firewallDeny proxies to agent POST /tasks/firewall/deny.
func (h *stackHandler) firewallDeny(c echo.Context) error {
	return h.firewallProxy(c, "deny")
}

// firewallDelete proxies to agent POST /tasks/firewall/delete.
func (h *stackHandler) firewallDelete(c echo.Context) error {
	return h.firewallProxy(c, "delete")
}

// firewallRules proxies to agent GET /tasks/firewall/rules.
func (h *stackHandler) firewallRules(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	server, err := h.resolveServer(c)
	if err != nil {
		return err
	}
	agentURL := strings.TrimRight(server.AgentURL.String, "/") + "/tasks/firewall/rules"
	resp, err := agentRequest("GET", agentURL, server.AgentToken, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "agent unreachable")
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]any
	json.Unmarshal(respBody, &result)
	return c.JSON(resp.StatusCode, httpjson.Success{Data: result, RequestID: rid})
}

// firewallReset proxies to agent POST /tasks/firewall/reset.
func (h *stackHandler) firewallReset(c echo.Context) error {
	return h.firewallProxy(c, "reset")
}

// firewallProxy is a generic proxy for firewall actions.
func (h *stackHandler) firewallProxy(c echo.Context, action string) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	server, err := h.resolveServer(c)
	if err != nil {
		return err
	}

	reqBody, _ := io.ReadAll(c.Request().Body)
	var body map[string]any
	json.Unmarshal(reqBody, &body)

	agentURL := strings.TrimRight(server.AgentURL.String, "/") + "/tasks/firewall/" + action
	resp, err := agentRequest("POST", agentURL, server.AgentToken, body)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "agent unreachable")
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]any
	json.Unmarshal(respBody, &result)
	return c.JSON(resp.StatusCode, httpjson.Success{Data: result, RequestID: rid})
}

// ─── Redis Management Proxy Handlers ──────────────────────────────────────

// redisStats proxies to agent GET /tasks/redis/stats.
func (h *stackHandler) redisStats(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	server, err := h.resolveServer(c)
	if err != nil {
		return err
	}
	agentURL := strings.TrimRight(server.AgentURL.String, "/") + "/tasks/redis/stats"
	resp, err := agentRequest("GET", agentURL, server.AgentToken, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "agent unreachable")
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]any
	json.Unmarshal(respBody, &result)
	return c.JSON(resp.StatusCode, httpjson.Success{Data: result, RequestID: rid})
}

// redisFlush proxies to agent POST /tasks/redis/flush.
func (h *stackHandler) redisFlush(c echo.Context) error {
	return h.redisProxy(c, "flush")
}

// redisInfo proxies to agent GET /tasks/redis/info.
func (h *stackHandler) redisInfo(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	server, err := h.resolveServer(c)
	if err != nil {
		return err
	}
	agentURL := strings.TrimRight(server.AgentURL.String, "/") + "/tasks/redis/info"
	resp, err := agentRequest("GET", agentURL, server.AgentToken, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "agent unreachable")
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]any
	json.Unmarshal(respBody, &result)
	return c.JSON(resp.StatusCode, httpjson.Success{Data: result, RequestID: rid})
}

// redisFlushDB proxies to agent POST /tasks/redis/flushdb.
func (h *stackHandler) redisFlushDB(c echo.Context) error {
	return h.redisProxy(c, "flushdb")
}

// redisProxy is a generic proxy for Redis actions.
func (h *stackHandler) redisProxy(c echo.Context, action string) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	server, err := h.resolveServer(c)
	if err != nil {
		return err
	}

	reqBody, _ := io.ReadAll(c.Request().Body)
	var body map[string]any
	json.Unmarshal(reqBody, &body)

	agentURL := strings.TrimRight(server.AgentURL.String, "/") + "/tasks/redis/" + action
	resp, err := agentRequest("POST", agentURL, server.AgentToken, body)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "agent unreachable")
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]any
	json.Unmarshal(respBody, &result)
	return c.JSON(resp.StatusCode, httpjson.Success{Data: result, RequestID: rid})
}

// agentRequest makes an HTTP request to the agent.
func agentRequest(method, url, token string, body any) (*http.Response, error) {
	client := &http.Client{Timeout: 5 * time.Minute}

	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal body: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return client.Do(req)
}

// ─── Fail2ban Management Proxy Handlers ──────────────────────────────────────

// securityOverview proxies to agent GET /tasks/security/overview.
func (h *stackHandler) securityOverview(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid server id")
	}

	server, err := h.store.Servers.GetByID(c.Request().Context(), id)
	if err != nil || server == nil {
		return echo.NewHTTPError(http.StatusNotFound, "server not found")
	}
	if !server.AgentURL.Valid || server.AgentURL.String == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "agent URL not configured")
	}

	agentURL := strings.TrimRight(server.AgentURL.String, "/") + "/tasks/security/overview"
	resp, err := agentRequest("GET", agentURL, server.AgentToken, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "agent unreachable")
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return echo.NewHTTPError(resp.StatusCode, "agent error: "+string(body))
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "invalid agent response")
	}

	return c.JSON(http.StatusOK, httpjson.Success{Data: result, RequestID: rid})
}

// fail2banStatus proxies to agent GET /tasks/fail2ban/status.
func (h *stackHandler) fail2banStatus(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	server, err := h.resolveServer(c)
	if err != nil {
		return err
	}
	agentURL := strings.TrimRight(server.AgentURL.String, "/") + "/tasks/fail2ban/status"
	resp, err := agentRequest("GET", agentURL, server.AgentToken, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "agent unreachable")
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]any
	json.Unmarshal(respBody, &result)
	return c.JSON(resp.StatusCode, httpjson.Success{Data: result, RequestID: rid})
}

// fail2banJail proxies to agent GET /tasks/fail2ban/jail?name=...
func (h *stackHandler) fail2banJail(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	server, err := h.resolveServer(c)
	if err != nil {
		return err
	}
	jailName := c.QueryParam("name")
	agentURL := strings.TrimRight(server.AgentURL.String, "/") + "/tasks/fail2ban/jail?name=" + jailName
	resp, err := agentRequest("GET", agentURL, server.AgentToken, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "agent unreachable")
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]any
	json.Unmarshal(respBody, &result)
	return c.JSON(resp.StatusCode, httpjson.Success{Data: result, RequestID: rid})
}

// fail2banUnban proxies to agent POST /tasks/fail2ban/unban.
func (h *stackHandler) fail2banUnban(c echo.Context) error {
	return h.fail2banProxy(c, "unban")
}

// fail2banBan proxies to agent POST /tasks/fail2ban/ban.
func (h *stackHandler) fail2banBan(c echo.Context) error {
	return h.fail2banProxy(c, "ban")
}

// fail2banEnable proxies to agent POST /tasks/fail2ban/enable.
func (h *stackHandler) fail2banEnable(c echo.Context) error {
	return h.fail2banProxy(c, "enable")
}

// fail2banDisable proxies to agent POST /tasks/fail2ban/disable.
func (h *stackHandler) fail2banDisable(c echo.Context) error {
	return h.fail2banProxy(c, "disable")
}

// botBlockerManage proxies to agent POST /tasks/bot-blocker/manage.
func (h *stackHandler) botBlockerManage(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	server, err := h.resolveServer(c)
	if err != nil {
		return err
	}

	reqBody, _ := io.ReadAll(c.Request().Body)
	var body map[string]any
	json.Unmarshal(reqBody, &body)

	agentURL := strings.TrimRight(server.AgentURL.String, "/") + "/tasks/bot-blocker/manage"
	resp, err := agentRequest("POST", agentURL, server.AgentToken, body)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "agent unreachable")
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]any
	json.Unmarshal(respBody, &result)
	return c.JSON(resp.StatusCode, httpjson.Success{Data: result, RequestID: rid})
}

func (h *stackHandler) fail2banProxy(c echo.Context, action string) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	server, err := h.resolveServer(c)
	if err != nil {
		return err
	}

	reqBody, _ := io.ReadAll(c.Request().Body)
	var body map[string]any
	json.Unmarshal(reqBody, &body)

	agentURL := strings.TrimRight(server.AgentURL.String, "/") + "/tasks/fail2ban/" + action
	resp, err := agentRequest("POST", agentURL, server.AgentToken, body)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "agent unreachable")
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]any
	json.Unmarshal(respBody, &result)
	return c.JSON(resp.StatusCode, httpjson.Success{Data: result, RequestID: rid})
}

// ─── Server Specs & Benchmarks ─────────────────────────────────────────────

// serverSpecs proxies to agent GET /tasks/system/spec.
func (h *stackHandler) serverSpecs(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	server, err := h.resolveServer(c)
	if err != nil {
		return err
	}

	agentURL := strings.TrimRight(server.AgentURL.String, "/") + "/tasks/system/spec"
	resp, err := agentRequest("GET", agentURL, server.AgentToken, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "agent unreachable")
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return echo.NewHTTPError(resp.StatusCode, "agent error: "+string(body))
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "invalid agent response")
	}

	return c.JSON(http.StatusOK, httpjson.Success{Data: result["data"], RequestID: rid})
}

// serverBenchmark proxies to agent GET /tasks/system/benchmark.
func (h *stackHandler) serverBenchmark(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	server, err := h.resolveServer(c)
	if err != nil {
		return err
	}

	// Use longer timeout for benchmark (up to 60 seconds)
	client := &http.Client{Timeout: 60 * time.Second}

	agentURL := strings.TrimRight(server.AgentURL.String, "/") + "/tasks/system/benchmark"
	req, err := http.NewRequest("GET", agentURL, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create request")
	}
	req.Header.Set("Authorization", "Bearer "+server.AgentToken)

	resp, err := client.Do(req)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "agent unreachable")
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return echo.NewHTTPError(resp.StatusCode, "agent error: "+string(body))
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "invalid agent response")
	}

	return c.JSON(http.StatusOK, httpjson.Success{Data: result["data"], RequestID: rid})
}
