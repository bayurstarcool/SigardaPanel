package apihttp

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"sigardapanel/internal/jobmodel"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"sigardapanel/internal/audit"
	"sigardapanel/internal/httpjson"
	"sigardapanel/internal/nginx"
	"sigardapanel/internal/store"
)

var sshUsernameRegex = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,30}$`)

// sshAccountsOnServer lists every SSH/FTP account of sites on serverID
// (the agent writes the sshd chroot config for all of them), optionally
// leaving one username out and adding another.
func (h *siteHandler) sshAccountsOnServer(ctx context.Context, serverID int64, without, add string) []map[string]string {
	all, _ := h.store.SSH.ListAll(ctx)
	sites := map[int64]int64{}
	out := []map[string]string{}
	for _, u := range all {
		srv, ok := sites[u.SiteID]
		if !ok {
			if st, err := h.store.Sites.GetByID(ctx, u.SiteID); err == nil && st != nil {
				srv = st.ServerID
			}
			sites[u.SiteID] = srv
		}
		if srv != serverID || u.Username == without || u.Username == "" {
			continue
		}
		out = append(out, map[string]string{"username": u.Username, "home_dir": u.HomeDir})
	}
	if add != "" {
		out = append(out, map[string]string{"username": add, "home_dir": "/home/" + add})
	}
	return out
}

// removeSiteSSHAccounts deprovisions every SSH/FTP account of a site on its
// server (used when the site is deleted; their jails bind-mount the site).
// sshMu serializes SSH/FTP account changes: each one sends the agent the
// full list of accounts (it rewrites the sshd jail config from it), so two
// concurrent changes could each drop the other's account from the jails.
var sshMu sync.Mutex

// removeSiteSSHAccounts removes the site's SSH/FTP accounts on its server.
// It fails if any account could not be removed: deleting the site anyway
// would drop the record while the Linux user and its key stay, and the next
// jail rewrite would leave that user unconfined.
func (h *siteHandler) removeSiteSSHAccounts(ctx context.Context, site *store.Site) error {
	sshMu.Lock()
	defer sshMu.Unlock()
	users, _ := h.store.SSH.ListBySiteID(ctx, site.ID)
	for _, u := range users {
		code, out, err := h.siteAgentCall(ctx, site, http.MethodPost, "/tasks/sshftp/delete",
			map[string]any{"username": u.Username, "accounts": h.sshAccountsOnServer(ctx, site.ServerID, u.Username, "")}, 2*time.Minute)
		if err != nil || code >= 400 {
			slog.Warn("site delete: ssh account not removed", "site_id", site.ID, "user", u.Username, "error", err, "message", agentMessage(out, ""))
			return fmt.Errorf("SSH/FTP account %s could not be removed from the server: %s", u.Username, agentMessage(out, fmt.Sprint(err)))
		}
		_ = h.store.SSH.Delete(ctx, site.ID, u.ID)
	}
	return nil
}

// siteAgentCall calls the agent of the server that hosts site.
func (h *siteHandler) siteAgentCall(ctx context.Context, site *store.Site, method, path string, body any, timeout time.Duration) (int, map[string]any, error) {
	server, err := h.store.Servers.GetByID(ctx, site.ServerID)
	if err != nil || server == nil || !server.AgentURL.Valid || server.AgentURL.String == "" || server.AgentToken == "" {
		return 0, nil, fmt.Errorf("the site's server has no reachable agent")
	}
	return stackAgentRequest(ctx, method, agentBase(server)+path, server.AgentToken, body, timeout)
}

// vhostRootGuard: raw vhost editing can read and write any file through nginx
// directives, i.e. it is root on the site's server; on the panel host that is
// super_admin power.
func (h *siteHandler) vhostRootGuard(ctx context.Context, sess *store.Session, site *store.Site) error {
	server, _ := h.store.Servers.GetByID(ctx, site.ServerID)
	if server != nil && isLocalServer(*server) && !isSessionSuperAdmin(ctx, h.store, sess) {
		return echo.NewHTTPError(http.StatusForbidden, "editing raw vhosts on the panel host requires a super admin")
	}
	return nil
}

func (h *siteHandler) getVhost(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	site, err := h.store.Sites.GetByID(c.Request().Context(), id)
	if err != nil || site == nil || !h.canAccess(c.Request().Context(), sess, site) {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}

	// The live vhost is on the site's server; the DB copy is the fallback.
	config, source, exists, path := "", "db", false, ""
	code, out, err := h.siteAgentCall(c.Request().Context(), site, http.MethodGet, "/tasks/vhost/get?domain="+url.QueryEscape(site.Domain), nil, 15*time.Second)
	if err == nil && code == http.StatusOK {
		path, _ = out["path"].(string)
		if e, _ := out["exists"].(bool); e {
			config, _ = out["config"].(string)
			source, exists = "nginx", true
		}
	}
	if !exists {
		if vhost, dbErr := h.store.Vhosts.GetBySiteID(c.Request().Context(), id); dbErr == nil {
			config, exists = vhost.Config, true
		}
	}
	return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]any{
		"site_id": id, "domain": site.Domain, "vhost_config": config,
		"source": source, "exists": exists, "config_path": path,
	}, RequestID: rid})
}

func (h *siteHandler) updateVhost(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)
	ctx := c.Request().Context()
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	site, err := h.store.Sites.GetByID(ctx, id)
	if err != nil || site == nil || !h.checkSitePermission(ctx, sess, site, "write") {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}
	if err := h.vhostRootGuard(ctx, sess, site); err != nil {
		return err
	}
	if site.Status == "suspended" {
		return echo.NewHTTPError(http.StatusConflict, "the site is suspended; resume it first")
	}
	var body struct {
		Config string `json:"vhost_config"`
	}
	if err := c.Bind(&body); err != nil || strings.TrimSpace(body.Config) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "vhost_config required")
	}

	code, out, err := h.siteAgentCall(ctx, site, http.MethodPost, "/tasks/vhost/put", map[string]any{"domain": site.Domain, "config": body.Config}, 2*time.Minute)
	if err != nil || code >= 400 {
		msg := "the site's agent is unreachable"
		if err == nil {
			msg = agentMessage(out, "agent error")
		}
		return echo.NewHTTPError(http.StatusBadGateway, "vhost not saved: "+msg)
	}
	if status, _ := out["status"].(string); status != "success" {
		nginxErr, _ := out["nginx_error"].(string)
		return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]any{
			"status": "saved_nginx_invalid", "message": "nginx -t failed, previous config kept. Check syntax:\n" + nginxErr,
			"nginx_error": nginxErr, "config_path": out["path"], "reload_attempt": false,
		}, RequestID: rid})
	}
	// Only a vhost nginx accepted is kept as the stored copy.
	if err := h.store.Vhosts.Upsert(ctx, id, body.Config); err != nil {
		slog.Warn("vhost: store copy failed", "site_id", id, "error", err)
	}
	h.store.NotifySiteUsers(ctx, id, "vhost_updated", "Vhost Updated", "Nginx vhost configuration was updated", "info", "site", id)
	if sess != nil {
		h.audit.RecordAction(ctx, sess.UserID, audit.ActionSiteUpdate, audit.ResourceSite, id, map[string]any{"domain": site.Domain, "section": "vhost"})
	}
	reloadOK, _ := out["reload_ok"].(bool)
	msg := "Nginx config saved"
	if reloadOK {
		msg += " and nginx reloaded"
	} else {
		msg += " (nginx reload failed: " + fmt.Sprint(out["reload_output"]) + ")"
	}
	return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]any{
		"status": "success", "message": msg, "config_path": out["path"], "reload_attempt": true, "reload_ok": reloadOK,
	}, RequestID: rid})
}

// validateVhost runs nginx -t with the candidate vhost on the site's server.
func (h *siteHandler) validateVhost(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)
	ctx := c.Request().Context()
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	site, err := h.store.Sites.GetByID(ctx, id)
	// Validation briefly installs the candidate: same rights as saving.
	if err != nil || site == nil || !h.checkSitePermission(ctx, sess, site, "write") {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}
	if err := h.vhostRootGuard(ctx, sess, site); err != nil {
		return err
	}
	var body struct {
		Config string `json:"vhost_config"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid body")
	}
	code, out, err := h.siteAgentCall(ctx, site, http.MethodPost, "/tasks/vhost/validate", map[string]any{"domain": site.Domain, "config": body.Config}, time.Minute)
	if err != nil || code >= 400 {
		return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]any{
			"valid": false, "warning": true, "message": "Could not reach the site's agent to validate the config.",
		}, RequestID: rid})
	}
	valid, _ := out["valid"].(bool)
	return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]any{
		"valid": valid, "warning": false, "message": out["message"],
	}, RequestID: rid})
}

func (h *siteHandler) getVarnish(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}

	site, err := h.store.Sites.GetByID(c.Request().Context(), id)
	if err != nil || site == nil {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}

	// Access check: user must be admin, owner, or assigned to this site
	if !h.canAccess(c.Request().Context(), sess, site) {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}

	v, err := h.store.Varnish.GetBySiteID(c.Request().Context(), id)
	if err == sql.ErrNoRows {
		return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]any{"enabled": false, "cache_time": 3600, "hit_rate": "0%"}, RequestID: rid})
	}
	if err != nil {
		slog.Error("api: request failed", "route", c.Path(), "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]any{"enabled": v.Enabled, "cache_time": v.CacheTime, "hit_rate": v.HitRate}, RequestID: rid})
}

func (h *siteHandler) purgeVarnish(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}

	site, err := h.store.Sites.GetByID(c.Request().Context(), id)
	if err != nil || site == nil {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}

	// Permission check: admin, owner, or user with admin permission on this site
	if !h.checkSitePermission(c.Request().Context(), sess, site, "admin") {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}

	return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]any{"status": "success", "message": "Cache purged"}, RequestID: rid})
}

func (h *siteHandler) getSecurity(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}

	site, err := h.store.Sites.GetByID(c.Request().Context(), id)
	if err != nil || site == nil {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}

	// Access check: user must be admin, owner, or assigned to this site
	if !h.canAccess(c.Request().Context(), sess, site) {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}

	sec, err := h.store.Security.GetBySiteID(c.Request().Context(), id)
	if err == sql.ErrNoRows {
		return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]any{"basic_auth_enabled": false, "basic_auth_username": "", "blocked_ips": []string{}}, RequestID: rid})
	}
	if err != nil {
		slog.Error("api: request failed", "route", c.Path(), "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]any{"basic_auth_enabled": sec.BasicAuthEnabled, "basic_auth_username": sec.BasicAuthUsername, "blocked_ips": sec.BlockedIPs}, RequestID: rid})
}

func (h *siteHandler) updateSecurity(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)
	var userID int64
	if sess != nil {
		userID = sess.UserID
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}

	site, err := h.store.Sites.GetByID(c.Request().Context(), id)
	if err != nil || site == nil {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}

	// Permission check: admin, owner, or user with write+ permission on this site
	if !h.checkSitePermission(c.Request().Context(), sess, site, "write") {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}

	var body struct {
		BasicAuthEnabled  bool     `json:"basic_auth_enabled"`
		BasicAuthUsername string   `json:"basic_auth_username"`
		BasicAuthPassword string   `json:"basic_auth_password"`
		BlockedIPs        []string `json:"blocked_ips"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid body")
	}
	ctx := c.Request().Context()
	if site.Status == "suspended" {
		return echo.NewHTTPError(http.StatusConflict, "the site is suspended; resume it first")
	}
	// Every value ends up in the nginx vhost: only plain IPs/CIDRs and a
	// simple username are accepted.
	blocked, err := nginx.NormalizeBlockedIPs(body.BlockedIPs)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	body.BasicAuthUsername = strings.TrimSpace(body.BasicAuthUsername)
	if body.BasicAuthEnabled {
		if !nginx.ValidBasicAuthUsername(body.BasicAuthUsername) {
			return echo.NewHTTPError(http.StatusBadRequest, "basic auth username must be 1-64 characters of A-Z a-z 0-9 . _ -")
		}
		if len(body.BasicAuthPassword) > 72 {
			return echo.NewHTTPError(http.StatusBadRequest, "basic auth password must be at most 72 characters")
		}
		if body.BasicAuthPassword == "" {
			existing, _ := h.store.Security.GetBySiteID(ctx, id)
			if existing == nil || existing.BasicAuthPassword == "" {
				return echo.NewHTTPError(http.StatusBadRequest, "set a basic auth password")
			}
		} else if len(body.BasicAuthPassword) < 6 {
			return echo.NewHTTPError(http.StatusBadRequest, "basic auth password must be at least 6 characters")
		}
	}

	sec := &store.SiteSecurity{
		SiteID:            id,
		BasicAuthEnabled:  body.BasicAuthEnabled,
		BasicAuthUsername: body.BasicAuthUsername,
		BasicAuthPassword: body.BasicAuthPassword,
		BlockedIPs:        blocked,
	}
	if err := h.store.Security.Upsert(ctx, sec); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	// The site's own agent regenerates the vhost (with these settings, which
	// the worker adds to every site.create payload) on the server that hosts
	// the site, tests it with nginx -t and keeps the old vhost on failure.
	job, err := h.enqueueVhostRegen(ctx, site, "Apply security settings for "+site.Domain)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to enqueue the vhost update")
	}

	h.store.NotifySiteUsers(ctx, id, "security_changed", "Security Updated", "Site security settings were changed", "warning", "site", id)

	h.audit.RecordAction(c.Request().Context(), userID, audit.ActionSiteUpdate, audit.ResourceSite, id, map[string]any{
		"domain":  site.Domain,
		"section": "security",
	})

	return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]any{"status": "queued", "message": "Security settings saved; applying them to the site", "job_id": job.ID}, RequestID: rid})
}

func (h *siteHandler) getSshFtp(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}

	site, err := h.store.Sites.GetByID(c.Request().Context(), id)
	if err != nil || site == nil {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}
	if err := requireSitePermission(c.Request().Context(), h.store, sessionFromEcho(c), site, sitePermRead); err != nil {
		return err
	}

	users, err := h.store.SSH.ListBySiteID(c.Request().Context(), id)
	if err != nil {
		slog.Error("api: request failed", "route", c.Path(), "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if users == nil {
		users = []store.SiteSSHUser{}
	}
	// Private keys are only served by the dedicated (write-guarded) key
	// endpoint, never in the listing.
	for i := range users {
		users[i].SSHPrivateKey = ""
		users[i].Password = ""
	}

	return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]any{"users": users}, RequestID: rid})
}

func (h *siteHandler) createSshFtp(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	sess := sessionFromEcho(c)
	var userID int64
	if sess != nil {
		userID = sess.UserID
	}

	site, err := h.store.Sites.GetByID(c.Request().Context(), id)
	if err != nil || site == nil {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}
	if err := requireSitePermission(c.Request().Context(), h.store, sess, site, sitePermWrite); err != nil {
		return err
	}

	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
		SSHKeys  string `json:"ssh_keys"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid body")
	}
	if body.Username == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "username required")
	}
	if !sshUsernameRegex.MatchString(body.Username) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid username (lowercase, alphanumeric, _, -, 1-31 chars)")
	}

	homeDir := "/home/" + body.Username
	ctx := c.Request().Context()
	if site.RootPath == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "the site has no root path")
	}
	// Panel usernames become site owners' Linux users on the servers their
	// sites land on: an SSH account with such a name would later be reused
	// as that owner's account (its key held by this tenant).
	if u, _ := h.store.Users.GetByUsername(ctx, body.Username); u != nil {
		return echo.NewHTTPError(http.StatusConflict, "this name belongs to a panel user; choose another SSH username")
	}
	var n int
	_ = h.store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM sites WHERE system_user = ?`, body.Username).Scan(&n)
	if n > 0 {
		return echo.NewHTTPError(http.StatusConflict, "this name is a site's system user; choose another SSH username")
	}
	sshMu.Lock()
	defer sshMu.Unlock()
	// The account (a chrooted Linux user) is created on the site's server.
	code, out, err := h.siteAgentCall(ctx, site, http.MethodPost, "/tasks/sshftp/create", map[string]any{
		"username": body.Username, "site_root": site.RootPath, "ssh_keys": body.SSHKeys,
		"accounts": h.sshAccountsOnServer(ctx, site.ServerID, "", body.Username),
	}, 3*time.Minute)
	if err != nil && code == 0 {
		return echo.NewHTTPError(http.StatusBadGateway, "the site's agent is unreachable")
	}
	if code >= 400 {
		return agentHTTPError(code, out)
	}
	pubKey, _ := out["ssh_public_key"].(string)
	privKey, _ := out["ssh_private_key"].(string)

	user := &store.SiteSSHUser{
		SiteID:        id,
		Username:      body.Username,
		Password:      "", // password auth disabled
		SSHKeys:       body.SSHKeys,
		SSHPrivateKey: privKey,
		SSHPublicKey:  pubKey,
	}
	user.HomeDir = homeDir
	if err := h.store.SSH.Create(ctx, user); err != nil {
		// Undo on the server: the account must not exist without its record.
		h.siteAgentCall(ctx, site, http.MethodPost, "/tasks/sshftp/delete",
			map[string]any{"username": body.Username, "accounts": h.sshAccountsOnServer(ctx, site.ServerID, body.Username, "")}, 2*time.Minute)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	h.audit.RecordAction(c.Request().Context(), userID, "ssh_user_create", audit.ResourceSite, id, map[string]any{
		"site_id":  id,
		"username": body.Username,
	})

	// The generated key pair is shown once (the UI displays it right after
	// creation; it used to read these fields from a response without them).
	return c.JSON(http.StatusCreated, httpjson.Success{Data: map[string]any{
		"status": "success", "message": "SSH/FTP user created",
		"username": body.Username, "ssh_public_key": pubKey, "ssh_private_key": privKey,
	}, RequestID: rid})
}

func (h *siteHandler) deleteSshFtp(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)
	var userID int64
	if sess != nil {
		userID = sess.UserID
	}
	sshID, err := strconv.ParseInt(c.Param("ssh_id"), 10, 64)
	if err != nil || sshID <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid user id")
	}

	// The SSH user must belong to the site in the URL, and the caller needs
	// write permission on that site.
	user, err := h.siteSSHUser(c, sshID, sitePermWrite)
	if err != nil {
		return err
	}

	ctx := c.Request().Context()
	site, _ := h.store.Sites.GetByID(ctx, user.SiteID)
	if site == nil {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}
	sshMu.Lock()
	defer sshMu.Unlock()
	code, out, err := h.siteAgentCall(ctx, site, http.MethodPost, "/tasks/sshftp/delete",
		map[string]any{"username": user.Username, "accounts": h.sshAccountsOnServer(ctx, site.ServerID, user.Username, "")}, 2*time.Minute)
	if err != nil && code == 0 {
		return echo.NewHTTPError(http.StatusBadGateway, "the site's agent is unreachable")
	}
	if code >= 400 {
		return agentHTTPError(code, out)
	}

	// Remove from database
	if err := h.store.SSH.Delete(c.Request().Context(), user.SiteID, sshID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "SSH user not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	h.audit.RecordAction(c.Request().Context(), userID, "ssh_user_delete", audit.ResourceSite, user.SiteID, map[string]any{
		"site_id":  user.SiteID,
		"username": user.Username,
	})

	return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]any{"status": "success", "message": "SSH/FTP user deleted"}, RequestID: rid})
}

// siteSSHUser loads SSH/FTP user sshID, verifying it belongs to the site in
// the :id URL parameter and that the caller holds the required permission on
// that site. Users of other sites are reported as not found.
func (h *siteHandler) siteSSHUser(c echo.Context, sshID int64, required string) (*store.SiteSSHUser, error) {
	ctx := c.Request().Context()
	siteID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || siteID <= 0 {
		return nil, echo.NewHTTPError(http.StatusBadRequest, "invalid site id")
	}
	site, err := h.store.Sites.GetByID(ctx, siteID)
	if err != nil || site == nil {
		return nil, echo.NewHTTPError(http.StatusNotFound, "site not found")
	}
	if err := requireSitePermission(ctx, h.store, sessionFromEcho(c), site, required); err != nil {
		return nil, err
	}
	user, err := h.store.SSH.GetByID(ctx, sshID)
	if err != nil {
		slog.Error("api: request failed", "route", c.Path(), "error", err)
		return nil, echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if user == nil || user.SiteID != siteID {
		return nil, echo.NewHTTPError(http.StatusNotFound, "SSH user not found")
	}
	return user, nil
}

// getSSHPrivateKey returns the SSH private key for a given SSH/FTP user.
// GET /api/v1/sites/:id/ssh-ftp/:ssh_id/key
func (h *siteHandler) getSSHPrivateKey(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sshID, err := strconv.ParseInt(c.Param("ssh_id"), 10, 64)
	if err != nil || sshID <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid user id")
	}

	// Downloading a private key grants shell access to the site: needs write
	// permission (or ownership) on the site the key belongs to.
	user, err := h.siteSSHUser(c, sshID, sitePermWrite)
	if err != nil {
		return err
	}

	if user.SSHPrivateKey == "" {
		return echo.NewHTTPError(http.StatusNotFound, "no private key found for this user")
	}

	return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]any{
		"private_key": user.SSHPrivateKey,
		"public_key":  user.SSHPublicKey,
		"username":    user.Username,
	}, RequestID: rid})
}

// syncSiteCron installs the site's complete cron job list on its server
// (/etc/cron.d, run as the site's Linux user from the site root).
func (h *siteHandler) syncSiteCron(ctx context.Context, site *store.Site, jobs []store.SiteCronJob) error {
	if len(jobs) > 0 && (!site.SystemUser.Valid || site.SystemUser.String == "") {
		return echo.NewHTTPError(http.StatusConflict, "this site has no Linux user to run cron jobs as")
	}
	list := make([]map[string]string, 0, len(jobs))
	for _, j := range jobs {
		list = append(list, map[string]string{"schedule": j.Schedule, "command": j.Command})
	}
	code, out, err := h.siteAgentCall(ctx, site, http.MethodPost, "/tasks/cron/sync", map[string]any{
		"site_id": site.ID, "system_user": site.SystemUser.String, "root_path": site.RootPath, "jobs": list,
	}, 30*time.Second)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "could not reach the site's server: "+err.Error())
	}
	if code >= 400 {
		return agentHTTPError(code, out)
	}
	return nil
}

func (h *siteHandler) listCron(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}

	site, err := h.store.Sites.GetByID(c.Request().Context(), id)
	if err != nil || site == nil {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}

	if !h.canAccess(c.Request().Context(), sess, site) {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}

	jobs, err := h.store.Cron.ListBySiteID(c.Request().Context(), id)
	if err != nil {
		slog.Error("api: request failed", "route", c.Path(), "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if jobs == nil {
		jobs = []store.SiteCronJob{}
	}

	return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]any{"jobs": jobs}, RequestID: rid})
}

func (h *siteHandler) createCron(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)
	var userID int64
	if sess != nil {
		userID = sess.UserID
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}

	site, err := h.store.Sites.GetByID(c.Request().Context(), id)
	if err != nil || site == nil {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}

	if !h.checkSitePermission(c.Request().Context(), sess, site, "write") {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}

	var body struct {
		Schedule string `json:"schedule"`
		Command  string `json:"command"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid body")
	}
	if body.Schedule == "" {
		body.Schedule = "* * * * *"
	}
	body.Schedule = strings.Join(strings.Fields(body.Schedule), " ")
	body.Command = strings.TrimSpace(body.Command)
	if err := validateSiteCron(body.Schedule, body.Command); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if site.Status == "suspended" {
		return echo.NewHTTPError(http.StatusConflict, "the site is suspended; resume it first")
	}

	ctx := c.Request().Context()
	existing, err := h.store.Cron.ListBySiteID(ctx, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	job := &store.SiteCronJob{SiteID: id, Schedule: body.Schedule, Command: body.Command}
	// Installed on the server first: a job the server refused is not recorded.
	if err := h.syncSiteCron(ctx, site, append(existing, *job)); err != nil {
		return err
	}
	if err := h.store.Cron.Create(ctx, job); err != nil {
		_ = h.syncSiteCron(ctx, site, existing)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	h.audit.RecordAction(c.Request().Context(), userID, "cron_create", audit.ResourceSite, id, map[string]any{
		"site_id":  id,
		"schedule": body.Schedule,
		"command":  body.Command,
	})

	return c.JSON(http.StatusCreated, httpjson.Success{Data: map[string]any{"status": "success", "message": "Cron job created"}, RequestID: rid})
}

func (h *siteHandler) deleteCron(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)
	var userID int64
	if sess != nil {
		userID = sess.UserID
	}
	cronID, err := strconv.ParseInt(c.Param("cron_id"), 10, 64)
	if err != nil || cronID <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid cron id")
	}

	// Get cron job to find site_id for access check
	siteID, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if siteID <= 0 {
		// Fallback: look up by cron job
		return echo.NewHTTPError(http.StatusBadRequest, "invalid site id")
	}

	site, err := h.store.Sites.GetByID(c.Request().Context(), siteID)
	if err != nil || site == nil {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}

	if !h.checkSitePermission(c.Request().Context(), sess, site, "write") {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}

	ctx := c.Request().Context()
	existing, err := h.store.Cron.ListBySiteID(ctx, siteID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	remaining := make([]store.SiteCronJob, 0, len(existing))
	found := false
	for _, j := range existing {
		if j.ID == cronID {
			found = true
			continue
		}
		remaining = append(remaining, j)
	}
	if !found {
		return echo.NewHTTPError(http.StatusNotFound, "cron job not found")
	}
	if err := h.syncSiteCron(ctx, site, remaining); err != nil {
		return err
	}
	// Scoped to the site in the URL: a cron job of another site is not found.
	if err := h.store.Cron.Delete(ctx, siteID, cronID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "cron job not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	h.audit.RecordAction(c.Request().Context(), userID, "cron_delete", audit.ResourceSite, siteID, map[string]any{
		"site_id": siteID,
		"cron_id": cronID,
	})

	return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]any{"status": "success", "message": "Cron job deleted"}, RequestID: rid})
}

// enqueueVhostRegen queues a site.create job that only regenerates the
// site's vhost on its server. The worker fills the payload with the site's
// stored settings (runtime, PHP, app port, proxy URL, security).
func (h *siteHandler) enqueueVhostRegen(ctx context.Context, site *store.Site, desc string) (*jobmodel.Job, error) {
	return enqueueSiteVhostRegen(ctx, h.store, site, desc)
}

func enqueueSiteVhostRegen(ctx context.Context, st *store.Store, site *store.Site, desc string) (*jobmodel.Job, error) {
	payload, _ := json.Marshal(map[string]any{
		"site_id":     site.ID,
		"name":        site.Name,
		"domain":      site.Domain,
		"runtime":     site.Runtime,
		"root_path":   site.RootPath,
		"vhost_regen": true,
	})
	job := &jobmodel.Job{
		ServerID:    sql.NullInt64{Int64: site.ServerID, Valid: true},
		SiteID:      sql.NullInt64{Int64: site.ID, Valid: true},
		Type:        jobmodel.TypeSiteCreate,
		Description: desc,
		Payload:     string(payload),
	}
	if err := st.Jobs.Create(ctx, job); err != nil {
		return nil, err
	}
	return job, nil
}
