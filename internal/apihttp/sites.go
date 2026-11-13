package apihttp

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"sigardapanel/internal/audit"
	"sigardapanel/internal/auth"
	"sigardapanel/internal/cloudflare"
	"sigardapanel/internal/dbserver"
	"sigardapanel/internal/httpjson"
	"sigardapanel/internal/jobmodel"
	"sigardapanel/internal/siteinput"
	"sigardapanel/internal/store"
)

var (
	siteNameRegex   = regexp.MustCompile(`^[a-zA-Z0-9_\- ]{1,64}$`)
	domainRegex     = regexp.MustCompile(`^(\*\.)?[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?)+$`)
	rootPathRegex   = regexp.MustCompile("^(\\/[^;|&$()<>*?\"]*\\/?)$")
	allowedRuntimes = map[string]struct{}{
		"static": {},
		"php":    {},
		"node":   {},
		"go":     {},
		"python": {},
		"proxy":  {},
		"docker": {},
	}
)

type siteHandler struct {
	store *store.Store
	audit *audit.Recorder
}

func newSiteHandler(s *store.Store, a *audit.Recorder) *siteHandler {
	return &siteHandler{store: s, audit: a}
}

func (h *siteHandler) list(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)
	_ = h.isAdmin(c) // keep for future use if needed

	filter := store.SiteListFilter{}
	if v := c.QueryParam("server_id"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			filter.ServerID = id
		}
	}
	if v := c.QueryParam("owner_id"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil && id > 0 {
			filter.OwnerID = id
		}
	}
	if v := c.QueryParam("runtime"); v != "" {
		filter.Runtime = v
	}
	if v := c.QueryParam("status"); v != "" {
		filter.Status = v
	}
	if v := c.QueryParam("search"); v != "" {
		filter.Search = v
	}
	if v := c.QueryParam("limit"); v != "" {
		if l, err := strconv.Atoi(v); err == nil && l > 0 {
			filter.Limit = l
		}
	}
	if v := c.QueryParam("offset"); v != "" {
		if o, err := strconv.Atoi(v); err == nil && o >= 0 {
			filter.Offset = o
		}
	}

	// Site isolation: only super_admin sees all sites. Everyone else sees the
	// sites they own plus the ones assigned to them; both are resolved in SQL
	// so filters, pagination and total_count apply to the combined set.
	isSuperAdmin := h.isSuperAdmin(c)
	if !isSuperAdmin && sess != nil {
		filter.AccessibleTo = sess.UserID
	}

	sites, err := h.store.Sites.List(c.Request().Context(), filter)
	if err != nil {
		slog.Error("api: request failed", "route", c.Path(), "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	totalCount, err := h.store.Sites.Count(c.Request().Context(), filter)
	if err != nil {
		slog.Error("api: request failed", "route", c.Path(), "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	out := make([]map[string]any, 0, len(sites))
	for _, s := range sites {
		out = append(out, siteToMap(s))
	}

	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      map[string]any{"sites": out, "count": len(out), "total_count": totalCount},
		RequestID: rid,
	})
}

func (h *siteHandler) get(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}

	site, err := h.store.Sites.GetByID(c.Request().Context(), id)
	if err != nil {
		slog.Error("api: request failed", "route", c.Path(), "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if site == nil {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}

	// Access check: hide sites the user cannot access (admin + owner + user_sites)
	if !h.canAccess(c.Request().Context(), sess, site) {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}

	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      siteToMap(*site),
		RequestID: rid,
	})
}

func (h *siteHandler) create(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)
	var userID int64
	if sess != nil {
		userID = sess.UserID
	}

	var body struct {
		ServerID       int64  `json:"server_id"`
		Name           string `json:"name"`
		Domain         string `json:"domain"`
		Runtime        string `json:"runtime"`
		RootPath       string `json:"root_path,omitempty"`
		ManagedBy      string `json:"managed_by,omitempty"`
		SystemUser     string `json:"system_user,omitempty"`
		ConfFile       string `json:"conf_file,omitempty"`
		ProxyPassURL   string `json:"proxy_pass_url,omitempty"`
		PHPVersion     string `json:"php_version,omitempty"`
		DockerImage    string `json:"docker_image,omitempty"`
		DockerLanguage string `json:"docker_language,omitempty"`
		DockerName     string `json:"docker_name,omitempty"`
		DockerPorts    string `json:"docker_ports,omitempty"`
		DockerEnv      string `json:"docker_env,omitempty"`
		DockerVolume   string `json:"docker_volume,omitempty"`
		DockerRestart  string `json:"docker_restart,omitempty"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	body.Name = strings.TrimSpace(body.Name)
	body.Domain = strings.TrimSpace(strings.ToLower(body.Domain))
	body.Runtime = strings.TrimSpace(strings.ToLower(body.Runtime))
	body.RootPath = strings.TrimSpace(body.RootPath)
	body.ManagedBy = strings.TrimSpace(strings.ToLower(body.ManagedBy))
	body.SystemUser = strings.TrimSpace(body.SystemUser)
	body.ConfFile = strings.TrimSpace(body.ConfFile)
	body.ProxyPassURL = strings.TrimSpace(body.ProxyPassURL)
	body.PHPVersion = strings.TrimSpace(body.PHPVersion)
	if body.ManagedBy == "" {
		body.ManagedBy = "sigardapanel"
	}

	if body.ServerID <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "server_id required")
	}
	if !siteNameRegex.MatchString(body.Name) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid name (1-64 chars, alphanumeric/_- )")
	}
	if !domainRegex.MatchString(body.Domain) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid domain")
	}
	if _, ok := allowedRuntimes[body.Runtime]; !ok {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid runtime")
	}
	if body.RootPath == "" {
		// Future-only default: /home/<user>/htdocs/<domain>
		// Existing sites keep their stored root_path unchanged.
		username := ""
		if userID > 0 {
			if owner, err := h.store.Users.GetByID(c.Request().Context(), userID); err == nil && owner != nil {
				username = owner.Username
			}
		}
		body.RootPath = defaultSiteRootPath(username, body.Domain)
	}
	if err := siteinput.RootPathAllowed(body.RootPath); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid root_path")
	}

	// Who is asking? Only admins may pick the unix account / filesystem
	// location; everyone else is pinned to their own account and home.
	var requester *store.User
	if userID > 0 {
		if u, err := h.store.Users.GetByID(c.Request().Context(), userID); err == nil {
			requester = u
		}
	}
	privileged := requester != nil && auth.HasRole(requester.Role, auth.RoleAdmin)
	if privileged {
		if body.SystemUser == "" {
			body.SystemUser = requester.Username
		}
		if err := siteinput.SystemUser(body.SystemUser); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid system_user")
		}
	} else {
		home := "/home/"
		body.SystemUser = ""
		if requester != nil {
			home = "/home/" + requester.Username + "/"
			if siteinput.SystemUser(requester.Username) == nil {
				body.SystemUser = requester.Username
			}
		}
		if !strings.HasPrefix(body.RootPath, home) {
			return echo.NewHTTPError(http.StatusForbidden, "root_path must be inside your home directory")
		}
	}
	if body.ProxyPassURL != "" {
		if err := siteinput.ProxyPassURL(body.ProxyPassURL); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
	}
	if body.PHPVersion != "" {
		if err := siteinput.PHPVersion(body.PHPVersion); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
	}
	if body.ConfFile != "" && !privileged {
		body.ConfFile = "" // informational field, only admins may set it
	}
	if body.Runtime == "docker" {
		if err := siteinput.DockerOptions(body.DockerImage, body.DockerName, body.DockerPorts, body.DockerEnv,
			body.DockerVolume, body.DockerRestart, body.DockerLanguage, body.RootPath); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		// The container is named after the site id (sp-site-<id>): custom
		// names could collide with, and replace, another site's container.
		if body.DockerName != "" {
			return echo.NewHTTPError(http.StatusBadRequest, "docker_name is assigned by the panel (sp-site-<id>); leave it empty")
		}
	}
	if body.ManagedBy != "sigardapanel" && body.ManagedBy != "cloudpanel" && body.ManagedBy != "external" {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid managed_by")
	}

	// Validate server exists
	server, err := h.store.Servers.GetByID(c.Request().Context(), body.ServerID)
	if err != nil {
		slog.Error("api: request failed", "route", c.Path(), "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if server == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "server not found")
	}

	// Validate user max_sites quota
	if userID > 0 {
		owner, err := h.store.Users.GetByID(c.Request().Context(), userID)
		if err != nil {
			slog.Error("api: request failed", "route", c.Path(), "error", err)
			return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
		}
		if owner != nil && owner.MaxSites.Valid {
			count, err := h.store.Sites.CountByOwner(c.Request().Context(), userID)
			if err != nil {
				slog.Error("api: request failed", "route", c.Path(), "error", err)
				return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
			}
			if int64(count) >= owner.MaxSites.Int64 {
				return echo.NewHTTPError(http.StatusForbidden,
					fmt.Sprintf("max sites limit reached (%d/%d)", count, owner.MaxSites.Int64))
			}
		}
	}

	// Validate domain not duplicate
	existing, err := h.store.Sites.GetByDomain(c.Request().Context(), body.Domain)
	if err != nil {
		slog.Error("api: request failed", "route", c.Path(), "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if existing != nil {
		return echo.NewHTTPError(http.StatusConflict, "domain already exists")
	}

	site := &store.Site{
		ServerID:     body.ServerID,
		Name:         body.Name,
		Domain:       body.Domain,
		Runtime:      body.Runtime,
		RootPath:     body.RootPath,
		Status:       "pending",
		OwnerID:      sql.NullInt64{Int64: userID, Valid: userID > 0},
		ManagedBy:    sql.NullString{String: body.ManagedBy, Valid: true},
		SystemUser:   sql.NullString{String: body.SystemUser, Valid: body.SystemUser != ""},
		ConfFile:     sql.NullString{String: body.ConfFile, Valid: body.ConfFile != ""},
		ProxyPassURL: sql.NullString{String: body.ProxyPassURL, Valid: body.ProxyPassURL != ""},
		PHPVersion:   sql.NullString{String: body.PHPVersion, Valid: body.PHPVersion != ""},
	}

	// ── App process management ──
	appPort := int64(0)
	appCmd := ""
	appStatus := "stopped"
	switch body.Runtime {
	case "node":
		appPort, err = h.store.Sites.FindFreePort(c.Request().Context(), 30000, 40000)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "no available port")
		}
		appCmd = "node server.js"
	case "go":
		appPort, err = h.store.Sites.FindFreePort(c.Request().Context(), 30000, 40000)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "no available port")
		}
		appCmd = "./app"
	case "python":
		appPort, err = h.store.Sites.FindFreePort(c.Request().Context(), 30000, 40000)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "no available port")
		}
		appCmd = "python3 app.py"
	case "proxy":
		if body.ProxyPassURL == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "proxy_pass_url required for proxy runtime")
		}
	}
	if appPort > 0 {
		site.AppPort = sql.NullInt64{Int64: appPort, Valid: true}
		site.AppCommand = sql.NullString{String: appCmd, Valid: true}
		site.AppStatus = sql.NullString{String: appStatus, Valid: true}
	}

	if err := h.store.Sites.Create(c.Request().Context(), site); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	// Enqueue site.create job. App runtimes proxy to the assigned app_port.
	port := int(appPort)
	// system_user: the agent creates that Linux account on the target server
	// when it is missing and hands the new site root to it. Only for sites
	// this panel manages (imports keep their existing ownership).
	systemUser := ""
	if body.ManagedBy == "sigardapanel" {
		systemUser = site.SystemUser.String
	}
	payload, _ := json.Marshal(map[string]any{
		"site_id":         site.ID,
		"name":            site.Name,
		"domain":          site.Domain,
		"runtime":         site.Runtime,
		"root_path":       site.RootPath,
		"port":            port,
		"proxy_pass_url":  body.ProxyPassURL,
		"php_version":     body.PHPVersion,
		"docker_image":    body.DockerImage,
		"docker_language": body.DockerLanguage,
		"docker_name":     body.DockerName,
		"docker_ports":    body.DockerPorts,
		"docker_env":      body.DockerEnv,
		"docker_volume":   body.DockerVolume,
		"docker_restart":  body.DockerRestart,
		"system_user":     systemUser,
	})
	job := &jobmodel.Job{
		ServerID:    sql.NullInt64{Int64: site.ServerID, Valid: true},
		SiteID:      sql.NullInt64{Int64: site.ID, Valid: true},
		Type:        jobmodel.TypeSiteCreate,
		Description: fmt.Sprintf("Create site %s", site.Domain),
		Payload:     string(payload),
	}
	if err := h.store.Jobs.Create(c.Request().Context(), job); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	h.audit.RecordAction(c.Request().Context(), userID, audit.ActionSiteCreate, audit.ResourceSite, site.ID, map[string]any{
		"domain":    site.Domain,
		"server_id": site.ServerID,
		"runtime":   site.Runtime,
	})

	h.store.NotifyAdmins(c.Request().Context(), "site_created", "Site Created", fmt.Sprintf("%s created", site.Domain), "info", "site", site.ID)

	// ── Cloudflare DNS auto-provisioning (per-user) ──
	cloudflareDNS := map[string]any{"provisioned": false, "zone": "", "record_id": "", "error": ""}
	cfEnabled := false
	cfCfg, err := h.store.Cloudflare.GetConfig(userID)
	if err == nil && cfCfg.APIToken != "" {
		cfEnabled = true
		cfClient := cloudflare.NewClient(cloudflare.Config{APIToken: cfCfg.APIToken, Email: cfCfg.Email})
		if zones, err := cfClient.ListZones(); err == nil {
			rootDomain := cloudflare.RootDomain(site.Domain)
			if zone := cloudflare.FindMatchingZone(zones, rootDomain); zone != nil {
				// Get server IP for A record
				var ip string
				if server != nil && server.IPAddress.Valid && server.IPAddress.String != "" {
					ip = server.IPAddress.String
				}
				if ip == "" {
					cloudflareDNS["error"] = "server has no IP address"
				} else {
					// Create A record: subdomain (or @ for root) pointing to server IP
					dnsName := site.Domain
					// Remove zone suffix to get subdomain part
					sub := strings.TrimSuffix(site.Domain, "."+zone.Name)
					if sub == "" || sub == site.Domain {
						dnsName = "@"
					}
					record, err := cfClient.CreateDNSRecord(zone.ID, cloudflare.CreateDNSRecordRequest{
						Name:    dnsName,
						Type:    "A",
						Content: ip,
						TTL:     1,
						Proxied: boolPtr(true),
					})
					if err != nil {
						cloudflareDNS["error"] = err.Error()
					} else {
						cloudflareDNS["provisioned"] = true
						cloudflareDNS["zone"] = zone.Name
						cloudflareDNS["record_id"] = record.ID
					}
				}
			} else {
				cloudflareDNS["error"] = "no Cloudflare zone found for " + rootDomain
			}
		} else {
			cloudflareDNS["error"] = "failed to list zones: " + err.Error()
		}
	}
	data := map[string]any{
		"site": siteToMap(*site),
		"job": map[string]any{
			"id":     job.ID,
			"type":   job.Type,
			"status": job.Status,
		},
	}
	if cfEnabled {
		data["cloudflare_dns"] = cloudflareDNS
	}

	return c.JSON(http.StatusCreated, httpjson.Success{
		Data:      data,
		RequestID: rid,
	})
}

func (h *siteHandler) delete(c echo.Context) error {
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
	if err != nil {
		slog.Error("api: request failed", "route", c.Path(), "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if site == nil {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}

	// Only the owner or a super_admin may delete a site (AGENTS.md permission
	// matrix); an assignment — even with "admin" permission — is not enough.
	if !h.isSiteOwnerOrSuperAdmin(c.Request().Context(), sess, site) {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}

	// SSH/FTP jails bind-mount the site root: remove them on the server
	// first (their rows used to vanish with the site while the Linux users,
	// mounts and keys stayed, and lost their chroot on the next sshd sync).
	if err := h.removeSiteSSHAccounts(c.Request().Context(), site); err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, err.Error()+"; the site was not deleted")
	}
	// Its cron jobs would otherwise keep running from a deleted root.
	if jobs, _ := h.store.Cron.ListBySiteID(c.Request().Context(), id); len(jobs) > 0 {
		if err := h.syncSiteCron(c.Request().Context(), site, nil); err != nil {
			slog.Warn("api: remove cron jobs of deleted site", "site_id", id, "error", err)
		}
	}

	// Clean up databases before deleting site. Mirror the dedicated
	// delete-database endpoint: drop the real MySQL database (not just its
	// record), enqueue the agent job as a fallback, then remove the record —
	// otherwise deleted sites leave orphaned databases, users and grants.
	if databases, err := h.store.Databases.ListBySite(c.Request().Context(), id); err == nil {
		for _, database := range databases {
			if database.Type == "mysql" {
				_ = execMySQLConn(h.store, func(conn *dbserver.MySQLConn) error {
					if err := conn.DropDatabase(c.Request().Context(), database.Name); err != nil {
						slog.Warn("site delete: mysql drop failed", "name", database.Name, "error", err)
						return err
					}
					return nil
				})
				payload := fmt.Sprintf(`{"database_id":%d,"site_id":%d,"name":%q,"type":%q}`,
					database.ID, database.SiteID, database.Name, database.Type)
				_ = h.store.Jobs.Create(c.Request().Context(), &jobmodel.Job{
					ServerID:    sql.NullInt64{Int64: site.ServerID, Valid: true},
					SiteID:      sql.NullInt64{Int64: site.ID, Valid: true},
					Type:        jobmodel.TypeDatabaseDelete,
					Description: fmt.Sprintf("Delete database %s (site removal)", database.Name),
					Status:      jobmodel.StatusQueued,
					Payload:     payload,
				})
			}
			h.store.Databases.Delete(c.Request().Context(), database.ID)
		}
	}

	// Clear app port so it can be reused
	h.store.Sites.ClearAppPort(c.Request().Context(), id)

	// Delete Cloudflare DNS record
	if cfCfg, err := h.store.Cloudflare.GetConfig(userID); err == nil && cfCfg.APIToken != "" {
		cfClient := cloudflare.NewClient(cloudflare.Config{APIToken: cfCfg.APIToken, Email: cfCfg.Email})
		if zones, err := cfClient.ListZones(); err == nil {
			rootDomain := cloudflare.RootDomain(site.Domain)
			if zone := cloudflare.FindMatchingZone(zones, rootDomain); zone != nil {
				if records, err := cfClient.ListDNSRecords(zone.ID, "A"); err == nil {
					for _, record := range records {
						if record.Name == site.Domain {
							cfClient.DeleteDNSRecord(zone.ID, record.ID)
							break
						}
					}
				}
			}
		}
	}

	// Send cleanup payload to agent — site.delete job handles files, SSL, logs, services
	payload, _ := json.Marshal(map[string]any{
		"site_id":   site.ID,
		"domain":    site.Domain,
		"root_path": site.RootPath,
	})
	job := &jobmodel.Job{
		ServerID:    sql.NullInt64{Int64: site.ServerID, Valid: true},
		SiteID:      sql.NullInt64{Int64: site.ID, Valid: true},
		Type:        jobmodel.TypeSiteDelete,
		Description: fmt.Sprintf("Delete site %s", site.Domain),
		Payload:     string(payload),
	}
	if err := h.store.Jobs.Create(c.Request().Context(), job); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	h.store.NotifyAdmins(c.Request().Context(), "site_deleted", "Site Deleted", fmt.Sprintf("%s deleted", site.Domain), "warning", "site", id)

	if err := h.store.Sites.Delete(c.Request().Context(), id); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	h.audit.RecordAction(c.Request().Context(), userID, audit.ActionSiteDelete, audit.ResourceSite, id, map[string]any{
		"domain": site.Domain,
	})

	return c.JSON(http.StatusOK, httpjson.Success{
		Data: map[string]any{
			"site_id": id,
			"job_id":  job.ID,
		},
		RequestID: rid,
	})
}

func (h *siteHandler) deploy(c echo.Context) error {
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
	if err != nil {
		slog.Error("api: request failed", "route", c.Path(), "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if site == nil {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}

	// Permission check: admin, owner, or user with write+ permission on this site
	if !h.checkSitePermission(c.Request().Context(), sess, site, "write") {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}

	// Validate git repo is configured
	if !site.GitRepo.Valid || site.GitRepo.String == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "no git repository configured; set git_repo in site config first")
	}
	branch := site.GitBranch.String
	if branch == "" {
		branch = "main"
	}

	job := &jobmodel.Job{
		ServerID:    sql.NullInt64{Int64: site.ServerID, Valid: true},
		SiteID:      sql.NullInt64{Int64: site.ID, Valid: true},
		Type:        jobmodel.TypeSiteDeploy,
		Description: fmt.Sprintf("Deploy site %s", site.Domain),
		Payload:     deployPayload(site, branch),
	}
	if err := h.store.Jobs.Create(c.Request().Context(), job); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	h.audit.RecordAction(c.Request().Context(), userID, audit.ActionSiteDeploy, audit.ResourceSite, id, map[string]any{
		"domain": site.Domain,
	})

	return c.JSON(http.StatusAccepted, httpjson.Success{
		Data: map[string]any{
			"job_id": job.ID,
		},
		RequestID: rid,
	})
}

func (h *siteHandler) updateConfig(c echo.Context) error {
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
	if err != nil {
		slog.Error("api: request failed", "route", c.Path(), "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if site == nil {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}

	// Permission check: admin, owner, or user with write+ permission on this site
	if !h.checkSitePermission(c.Request().Context(), sess, site, "write") {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}

	var body struct {
		GitRepo          string  `json:"git_repo"`
		GitBranch        string  `json:"git_branch"`
		ProxyPassURL     *string `json:"proxy_pass_url"`
		BackupProviderID *int64  `json:"backup_provider_id"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	// Update git config if git_repo is provided
	if body.GitRepo != "" {
		if !strings.HasPrefix(body.GitRepo, "https://") {
			return echo.NewHTTPError(http.StatusBadRequest, "git_repo must be an HTTPS URL")
		}
		branch := body.GitBranch
		if branch == "" {
			branch = "main"
		}
		if err := h.store.Sites.UpdateGitConfig(c.Request().Context(), id, body.GitRepo, branch); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
		}
	}

	// Update proxy_pass_url — use *string to allow clearing (empty string = clear)
	if body.ProxyPassURL != nil {
		if *body.ProxyPassURL != "" {
			if err := siteinput.ProxyPassURL(*body.ProxyPassURL); err != nil {
				return echo.NewHTTPError(http.StatusBadRequest, err.Error())
			}
		}
		changed := *body.ProxyPassURL != site.ProxyPassURL.String
		if changed && site.Runtime == "proxy" {
			if *body.ProxyPassURL == "" {
				return echo.NewHTTPError(http.StatusBadRequest, "a proxy site needs a proxy_pass_url")
			}
			if site.Status == "suspended" {
				return echo.NewHTTPError(http.StatusConflict, "the site is suspended; resume it first")
			}
		}
		if err := h.store.Sites.UpdateProxyPassURL(c.Request().Context(), id, *body.ProxyPassURL); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
		}
		// The vhost carries the upstream: regenerate it, or the change
		// would only show in the panel.
		if changed && site.Runtime == "proxy" {
			fresh, _ := h.store.Sites.GetByID(c.Request().Context(), id)
			if fresh != nil {
				if _, err := h.enqueueVhostRegen(c.Request().Context(), fresh, "Point "+fresh.Domain+" at its new upstream"); err != nil {
					slog.Warn("api: queue vhost regen after proxy change", "site_id", id, "error", err)
				}
			}
		}
	}

	// Update backup provider
	if body.BackupProviderID != nil {
		var pid sql.NullInt64
		if *body.BackupProviderID > 0 {
			pid = sql.NullInt64{Int64: *body.BackupProviderID, Valid: true}
		}
		if err := h.store.Sites.UpdateBackupProvider(c.Request().Context(), id, pid); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
		}
	}

	// Generate webhook secret if not exists
	var secret string
	site, _ = h.store.Sites.GetByID(c.Request().Context(), id)
	if site != nil && !site.WebhookSecret.Valid {
		secret, _ = h.store.Sites.GenerateWebhookSecret(c.Request().Context(), id)
	}

	data := map[string]any{"ok": true}
	if secret != "" {
		data["webhook_secret"] = secret
	}

	h.audit.RecordAction(c.Request().Context(), userID, audit.ActionSiteUpdate, audit.ResourceSite, id, map[string]any{
		"domain": site.Domain,
	})

	h.store.NotifySiteUsers(c.Request().Context(), id, "config_updated", "Config Updated", "Site configuration was updated", "info", "site", id)

	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      data,
		RequestID: rid,
	})
}

func (h *siteHandler) reloadNginx(c echo.Context) error {
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
	if err != nil {
		slog.Error("api: request failed", "route", c.Path(), "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if site == nil {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}

	// Permission check: admin, owner, or user with admin permission on this site
	if !h.checkSitePermission(c.Request().Context(), sess, site, "admin") {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}

	// Enqueue nginx.reload job
	job := &jobmodel.Job{
		ServerID:    sql.NullInt64{Int64: site.ServerID, Valid: true},
		SiteID:      sql.NullInt64{Int64: site.ID, Valid: true},
		Type:        jobmodel.TypeNginxReload,
		Description: fmt.Sprintf("Reload Nginx for %s", site.Domain),
		Status:      jobmodel.StatusQueued,
		Payload:     fmt.Sprintf(`{"site_id":%d,"domain":%q}`, site.ID, site.Domain),
	}
	if err := h.store.Jobs.Create(c.Request().Context(), job); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	h.audit.RecordAction(c.Request().Context(), userID, "site_reload_nginx", audit.ResourceSite, site.ID, nil)

	return c.JSON(http.StatusAccepted, httpjson.Success{
		Data: map[string]any{
			"job_id": job.ID,
			"status": jobmodel.StatusQueued,
		},
		RequestID: rid,
	})
}

// canAccess returns true if the user can access the site.
// super_admin can access everything. admin/user must own or be assigned.
func (h *siteHandler) canAccess(ctx context.Context, sess *store.Session, site *store.Site) bool {
	if sess == nil {
		return false
	}
	user, _ := h.store.Users.GetByID(ctx, sess.UserID)
	if user == nil {
		return false
	}
	// Only super_admin bypasses ownership check
	if user.Role == auth.RoleSuperAdmin {
		return true
	}
	// Owner can always access
	if site.OwnerID.Valid && site.OwnerID.Int64 == sess.UserID {
		return true
	}
	// Check user_sites assignment
	hasAccess, _ := h.store.UserSites.HasAccess(ctx, sess.UserID, site.ID)
	return hasAccess
}

// checkSitePermission returns true if the user has at least the required permission
// level on the site. Only super_admin bypasses. admin/user must own or be assigned.
func (h *siteHandler) checkSitePermission(ctx context.Context, sess *store.Session, site *store.Site, required string) bool {
	if sess == nil {
		return false
	}
	user, _ := h.store.Users.GetByID(ctx, sess.UserID)
	if user == nil {
		return false
	}
	// Only super_admin bypasses permission check
	if user.Role == auth.RoleSuperAdmin {
		return true
	}
	// Owner can do anything with their own site
	if site.OwnerID.Valid && site.OwnerID.Int64 == sess.UserID {
		return true
	}
	// Check user_sites permission level
	perm, err := h.store.UserSites.GetPermission(ctx, sess.UserID, site.ID)
	if err != nil {
		return false
	}
	switch required {
	case "admin":
		return perm == "admin"
	case "write":
		return perm == "admin" || perm == "write"
	case "read":
		return perm == "admin" || perm == "write" || perm == "read"
	default:
		return false
	}
}

// isSiteOwnerOrSuperAdmin reports whether the session user owns the site or is super_admin.
func (h *siteHandler) isSiteOwnerOrSuperAdmin(ctx context.Context, sess *store.Session, site *store.Site) bool {
	if sess == nil || sess.UserID == 0 {
		return false
	}
	if site.OwnerID.Valid && site.OwnerID.Int64 == sess.UserID {
		return true
	}
	user, _ := h.store.Users.GetByID(ctx, sess.UserID)
	return user != nil && user.Role == auth.RoleSuperAdmin
}

func (h *siteHandler) isAdmin(c echo.Context) bool {
	sess := sessionFromEcho(c)
	if sess == nil {
		return false
	}
	user, err := h.store.Users.GetByID(c.Request().Context(), sess.UserID)
	if err != nil || user == nil {
		return false
	}
	return auth.HasRole(user.Role, auth.RoleAdmin)
}

func (h *siteHandler) isSuperAdmin(c echo.Context) bool {
	sess := sessionFromEcho(c)
	if sess == nil {
		return false
	}
	user, err := h.store.Users.GetByID(c.Request().Context(), sess.UserID)
	if err != nil || user == nil {
		return false
	}
	return user.Role == auth.RoleSuperAdmin
}

func (h *siteHandler) transfer(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)
	if sess == nil || sess.UserID == 0 {
		return echo.NewHTTPError(http.StatusUnauthorized, "user session required")
	}
	// The route has no RequireRole, so roleKey is not in the context; read the
	// caller's role from the database (asserting it panicked the request).
	caller, err := h.store.Users.GetByID(c.Request().Context(), sess.UserID)
	if err != nil || caller == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "user session required")
	}
	role := caller.Role

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid site id")
	}

	var body struct {
		NewOwnerEmail string `json:"new_owner_email"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if body.NewOwnerEmail == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "new_owner_email required")
	}

	// Look up target user by email
	target, err := h.store.Users.GetByEmail(c.Request().Context(), body.NewOwnerEmail)
	if err != nil || target == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "user not found with that email")
	}

	site, err := h.store.Sites.GetByID(c.Request().Context(), id)
	if err != nil {
		slog.Error("api: request failed", "route", c.Path(), "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if site == nil {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}

	// Access check: only super_admin, site owner, or admin who created the owner can transfer
	isSuperAdmin := role == auth.RoleSuperAdmin
	isOwner := site.OwnerID.Valid && site.OwnerID.Int64 == sess.UserID
	isAdminOfOwner := false
	if role == auth.RoleAdmin && site.OwnerID.Valid {
		owner, err := h.store.Users.GetByID(c.Request().Context(), site.OwnerID.Int64)
		if err == nil && owner != nil && owner.CreatedBy.Valid && owner.CreatedBy.Int64 == sess.UserID {
			isAdminOfOwner = true
		}
	}
	if !isSuperAdmin && !isOwner && !isAdminOfOwner {
		return echo.NewHTTPError(http.StatusForbidden, "cannot transfer site you do not own")
	}

	if target.ID == sess.UserID {
		return echo.NewHTTPError(http.StatusBadRequest, "cannot transfer to yourself")
	}

	if site.Status == "suspended" {
		return echo.NewHTTPError(http.StatusConflict, "the site is suspended; resume it first")
	}
	// SSH/FTP jails bind-mount the current root path.
	if accts, _ := h.store.SSH.ListBySiteID(c.Request().Context(), id); len(accts) > 0 {
		return echo.NewHTTPError(http.StatusConflict, "remove the site's SSH/FTP accounts before transferring it")
	}
	// A container bind-mounts the current root; it is not re-created by the move.
	if site.Runtime == "docker" {
		return echo.NewHTTPError(http.StatusConflict, "docker sites cannot be transferred")
	}
	// Cron jobs run as the current owner's Linux user from the current root.
	if jobs, _ := h.store.Cron.ListBySiteID(c.Request().Context(), id); len(jobs) > 0 {
		return echo.NewHTTPError(http.StatusConflict, "remove the site's cron jobs before transferring it")
	}
	// The files move on the site's server first (site.transfer job); owner,
	// root path and system user change only when that succeeded, then the
	// vhost is regenerated for the new path.
	newRoot := store.TransferRoot(target.Username, site.Domain)
	payload, _ := json.Marshal(map[string]any{
		"site_id": site.ID, "domain": site.Domain, "old_root": site.RootPath, "new_root": newRoot,
		"system_user": target.Username, "new_owner_id": target.ID,
	})
	job := &jobmodel.Job{
		ServerID:    sql.NullInt64{Int64: site.ServerID, Valid: true},
		SiteID:      sql.NullInt64{Int64: site.ID, Valid: true},
		Type:        jobmodel.TypeSiteTransfer,
		Description: fmt.Sprintf("Transfer %s to %s", site.Domain, target.Username),
		Payload:     string(payload),
	}
	if err := h.store.Jobs.Create(c.Request().Context(), job); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	h.audit.RecordAction(c.Request().Context(), sess.UserID, audit.ActionTransfer, audit.ResourceSite, id, map[string]any{
		"from": site.OwnerID.Int64,
		"to":   target.ID,
	})

	return c.JSON(http.StatusAccepted, httpjson.Success{
		Data:      map[string]any{"transferred": false, "status": "queued", "job_id": job.ID, "site_id": id, "new_owner_email": body.NewOwnerEmail, "new_owner_id": target.ID},
		RequestID: rid,
	})
}

func siteToMap(s store.Site) map[string]any {
	m := map[string]any{
		"id":         s.ID,
		"server_id":  s.ServerID,
		"name":       s.Name,
		"domain":     s.Domain,
		"runtime":    s.Runtime,
		"root_path":  s.RootPath,
		"status":     s.Status,
		"created_at": s.CreatedAt,
		"updated_at": s.UpdatedAt,
	}
	if s.OwnerID.Valid {
		m["owner_id"] = s.OwnerID.Int64
	}
	if s.GitRepo.Valid {
		m["git_repo"] = s.GitRepo.String
	}
	if s.GitBranch.Valid {
		m["git_branch"] = s.GitBranch.String
	}
	if s.GitCommit.Valid {
		m["git_commit"] = s.GitCommit.String
	}
	if s.ManagedBy.Valid {
		m["managed_by"] = s.ManagedBy.String
	}
	if s.SystemUser.Valid {
		m["system_user"] = s.SystemUser.String
	}
	if s.ConfFile.Valid {
		m["conf_file"] = s.ConfFile.String
	}
	if s.ProxyPassURL.Valid {
		m["proxy_pass_url"] = s.ProxyPassURL.String
	}
	if s.AppPort.Valid {
		m["app_port"] = s.AppPort.Int64
	}
	if s.AppCommand.Valid {
		m["app_command"] = s.AppCommand.String
	}
	if s.AppStatus.Valid {
		m["app_status"] = s.AppStatus.String
	}
	if s.AppMemoryMax.Valid {
		m["app_memory_max"] = s.AppMemoryMax.Int64
	}
	if s.PHPVersion.Valid {
		m["php_version"] = s.PHPVersion.String
	}
	if s.SSLStatus.Valid {
		m["ssl_status"] = s.SSLStatus.String
	}
	if s.SSLIssuer.Valid {
		m["ssl_issuer"] = s.SSLIssuer.String
	}
	if s.SSLExpiresAt.Valid {
		m["ssl_expires_at"] = s.SSLExpiresAt.Time
	}
	if s.SSLLastCheck.Valid {
		m["ssl_last_check"] = s.SSLLastCheck.Time
	}
	if s.PHPIni.Valid {
		m["php_ini"] = s.PHPIni.String
	}
	if s.BackupProviderID.Valid {
		m["backup_provider_id"] = s.BackupProviderID.Int64
	}
	if s.SSLStatus.Valid {
		m["ssl_status"] = s.SSLStatus.String
	}
	if s.SSLIssuer.Valid {
		m["ssl_issuer"] = s.SSLIssuer.String
	}
	if s.SSLExpiresAt.Valid {
		m["ssl_expires_at"] = s.SSLExpiresAt.Time
	}
	return m
}

// boolPtr returns a pointer to a bool value.
func boolPtr(v bool) *bool { return &v }

// defaultSiteRootPath returns the conventional root path for a new site.
// Convention: /home/<user>/htdocs/<domain> (matches existing CloudPanel layout).
// One user can own many sites, all grouped under their home dir.
// Falls back to the domain as the home segment when no username is available.
// Existing sites keep their stored paths unchanged; this only applies to new creations.
func defaultSiteRootPath(username, domain string) string {
	home := username
	if home == "" {
		home = domain
	}
	return "/home/" + home + "/htdocs/" + domain
}

// usage returns real disk usage for a specific site.
func (h *siteHandler) usage(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid site id")
	}

	site, err := h.store.Sites.GetByID(c.Request().Context(), id)
	if err != nil {
		slog.Error("api: request failed", "route", c.Path(), "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if site == nil {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}

	if !h.canAccess(c.Request().Context(), sess, site) {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}

	// du runs on the site's server (the API host may not have the files).
	var diskUsedBytes int64
	if site.RootPath != "" {
		code, out, err := h.siteAgentCall(c.Request().Context(), site, http.MethodPost, "/tasks/site/usage", map[string]any{"root_path": site.RootPath}, 2*time.Minute)
		if err != nil || code >= 400 {
			return echo.NewHTTPError(http.StatusBadGateway, "could not get the disk usage from the site's server")
		}
		if v, ok := out["disk_used_bytes"].(float64); ok {
			diskUsedBytes = int64(v)
		}
	}

	return c.JSON(http.StatusOK, httpjson.Success{
		Data: map[string]any{
			"site_id":         id,
			"domain":          site.Domain,
			"root_path":       site.RootPath,
			"disk_used_bytes": diskUsedBytes,
		},
		RequestID: rid,
	})
}

// ─── Git Deploy v2 Proxy Handlers ──────────────────────────────────────

// gitBranches proxies to agent GET /tasks/git/branches.
func (h *siteHandler) gitBranches(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	site, server, err := h.resolveSiteAndServer(c)
	if err != nil {
		return err
	}
	agentURL := strings.TrimRight(server.AgentURL.String, "/") + "/tasks/git/branches?root_path=" + url.QueryEscape(site.RootPath)
	resp, err := agentRequest("GET", agentURL, server.AgentToken, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "agent unreachable")
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]any
	json.Unmarshal(respBody, &result)
	return agentProxyResponse(c, rid, resp.StatusCode, result)
}

// gitLog proxies to agent GET /tasks/git/log.
func (h *siteHandler) gitLog(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	site, server, err := h.resolveSiteAndServer(c)
	if err != nil {
		return err
	}
	lines := c.QueryParam("lines")
	if lines == "" {
		lines = "20"
	}
	agentURL := strings.TrimRight(server.AgentURL.String, "/") + "/tasks/git/log?root_path=" + url.QueryEscape(site.RootPath) + "&lines=" + url.QueryEscape(lines)
	resp, err := agentRequest("GET", agentURL, server.AgentToken, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "agent unreachable")
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]any
	json.Unmarshal(respBody, &result)
	return agentProxyResponse(c, rid, resp.StatusCode, result)
}

// gitRollback proxies to agent POST /tasks/git/rollback.
func (h *siteHandler) gitRollback(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	// Changing the deployed code needs write permission on the site (read
	// assignees could do it, owners with the user role could not).
	site, server, err := h.resolveSiteAndServerPerm(c, "write")
	if err != nil {
		return err
	}
	var req struct {
		Commit string `json:"commit"`
	}
	if err := c.Bind(&req); err != nil || strings.TrimSpace(req.Commit) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "commit is required")
	}
	body := map[string]any{"commit": strings.TrimSpace(req.Commit), "root_path": site.RootPath}
	agentURL := strings.TrimRight(server.AgentURL.String, "/") + "/tasks/git/rollback"
	resp, err := agentRequest("POST", agentURL, server.AgentToken, body)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "agent unreachable")
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]any
	json.Unmarshal(respBody, &result)
	return agentProxyResponse(c, rid, resp.StatusCode, result)
}

// gitCheckout proxies to agent POST /tasks/git/checkout.
func (h *siteHandler) gitCheckout(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	// Changing the deployed code needs write permission on the site (read
	// assignees could do it, owners with the user role could not).
	site, server, err := h.resolveSiteAndServerPerm(c, "write")
	if err != nil {
		return err
	}
	var req struct {
		Branch string `json:"branch"`
	}
	if err := c.Bind(&req); err != nil || strings.TrimSpace(req.Branch) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "branch is required")
	}
	body := map[string]any{"branch": strings.TrimSpace(req.Branch), "root_path": site.RootPath}
	agentURL := strings.TrimRight(server.AgentURL.String, "/") + "/tasks/git/checkout"
	resp, err := agentRequest("POST", agentURL, server.AgentToken, body)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "agent unreachable")
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]any
	json.Unmarshal(respBody, &result)
	return agentProxyResponse(c, rid, resp.StatusCode, result)
}

// resolveSiteAndServer resolves site and its server from URL params.
func (h *siteHandler) resolveSiteAndServer(c echo.Context) (*store.Site, *store.Server, error) {
	return h.resolveSiteAndServerPerm(c, "read")
}

// resolveSiteAndServerPerm is resolveSiteAndServer requiring a permission
// level on the site, checked before anything about the server is revealed.
func (h *siteHandler) resolveSiteAndServerPerm(c echo.Context, perm string) (*store.Site, *store.Server, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return nil, nil, echo.NewHTTPError(http.StatusBadRequest, "invalid site id")
	}
	site, err := h.store.Sites.GetByID(c.Request().Context(), id)
	if err != nil || site == nil {
		return nil, nil, echo.NewHTTPError(http.StatusNotFound, "site not found")
	}
	// Ownership / assignment check: without it any logged-in user could read
	// another tenant's repository (branches, history) or roll it back.
	if !h.canAccess(c.Request().Context(), sessionFromEcho(c), site) ||
		(perm != "read" && !h.checkSitePermission(c.Request().Context(), sessionFromEcho(c), site, perm)) {
		return nil, nil, echo.NewHTTPError(http.StatusNotFound, "site not found")
	}
	server, err := h.store.Servers.GetByID(c.Request().Context(), site.ServerID)
	if err != nil || server == nil {
		return nil, nil, echo.NewHTTPError(http.StatusNotFound, "server not found")
	}
	if !server.AgentURL.Valid || server.AgentURL.String == "" {
		return nil, nil, echo.NewHTTPError(http.StatusBadRequest, "agent URL not configured")
	}
	if server.AgentToken == "" {
		return nil, nil, echo.NewHTTPError(http.StatusBadRequest, "agent token not configured")
	}
	return site, server, nil
}

// ─── Process Resource Management ──────────────────────────────────────

// appResourceUsage returns current resource usage for a site's app process.
func (h *siteHandler) appResourceUsage(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid site id")
	}

	_ = id
	site, err := h.getSiteForUserPerm(c, sitePermRead)
	if err != nil {
		return err
	}

	server, err := h.store.Servers.GetByID(c.Request().Context(), site.ServerID)
	if err != nil || server == nil || !server.AgentURL.Valid || server.AgentToken == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "agent not configured")
	}

	// Get process status via systemctl
	svcName := fmt.Sprintf("sigarda-app-%d", site.ID)
	agentURL := strings.TrimRight(server.AgentURL.String, "/") + "/tasks/command/exec"
	payload := map[string]string{
		"command": fmt.Sprintf("systemctl show %s --property=MemoryCurrent,MemoryMax,CPUUsageNSec,TaskCurrent,MainPID,ActiveState,SubState,RestartUSec,NRestarts", svcName),
	}
	resp, err := agentRequest("POST", agentURL, server.AgentToken, payload)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "agent unreachable")
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]any
	json.Unmarshal(respBody, &result)

	// Parse the output
	output, _ := result["output"].(string)
	resourceMap := make(map[string]string)
	for _, line := range strings.Split(output, "\n") {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			resourceMap[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}

	return c.JSON(http.StatusOK, httpjson.Success{
		Data: map[string]any{
			"site_id":        id,
			"domain":         site.Domain,
			"service":        svcName,
			"memory_current": resourceMap["MemoryCurrent"],
			"memory_max":     resourceMap["MemoryMax"],
			"cpu_usage":      resourceMap["CPUUsageNSec"],
			"task_current":   resourceMap["TaskCurrent"],
			"main_pid":       resourceMap["MainPID"],
			"active_state":   resourceMap["ActiveState"],
			"sub_state":      resourceMap["SubState"],
			"restart_usec":   resourceMap["RestartUSec"],
			"n_restarts":     resourceMap["NRestarts"],
			"memory_limit":   site.AppMemoryMax.Int64,
		},
		RequestID: rid,
	})
}

// appUpdateResourceLimits updates resource limits for a site's app process.
func (h *siteHandler) appUpdateResourceLimits(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid site id")
	}

	site, err := h.getSiteForUserPerm(c, sitePermWrite)
	if err != nil {
		return err
	}

	var body struct {
		MemoryMax int64 `json:"memory_max"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid body")
	}

	if body.MemoryMax < 64 || body.MemoryMax > 65536 {
		return echo.NewHTTPError(http.StatusBadRequest, "memory_max must be 64-65536 (MB)")
	}

	// Update memory limit in DB
	if err := h.store.Sites.UpdateAppMemory(c.Request().Context(), id, body.MemoryMax); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "update failed")
	}

	// Apply the new limit: app.restart rewrites the systemd unit with
	// MemoryMax (a bare "systemctl restart" kept the old limit).
	if site.AppStatus.String == "running" {
		payload, _ := json.Marshal(map[string]any{
			"site_id": site.ID, "domain": site.Domain, "root_path": site.RootPath,
			"system_user": site.SystemUser.String, "app_port": site.AppPort.Int64,
			"app_command": site.AppCommand.String, "app_memory_max": body.MemoryMax,
		})
		if err := h.store.Jobs.Create(c.Request().Context(), &jobmodel.Job{
			ServerID: sql.NullInt64{Int64: site.ServerID, Valid: true},
			SiteID:   sql.NullInt64{Int64: site.ID, Valid: true},
			Type:     jobmodel.TypeAppRestart,
			Payload:  string(payload),
		}); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to enqueue the restart")
		}
	}

	h.audit.RecordAction(c.Request().Context(), sessionFromEcho(c).UserID, audit.ActionSiteUpdate, "site", id, map[string]any{
		"memory_max": body.MemoryMax,
	})

	return c.JSON(http.StatusOK, httpjson.Success{
		Data: map[string]any{
			"site_id":    id,
			"memory_max": body.MemoryMax,
			"message":    fmt.Sprintf("Memory limit updated to %dMB. Restarting app.", body.MemoryMax),
		},
		RequestID: rid,
	})
}
