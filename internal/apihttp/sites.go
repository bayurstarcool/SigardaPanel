package apihttp

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"sigardapanel/internal/audit"
	"sigardapanel/internal/auth"
	"sigardapanel/internal/cloudflare"
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

	// Site isolation: only super_admin sees all sites.
	// admin and user see only sites they own + sites assigned to them.
	isSuperAdmin := h.isSuperAdmin(c)
	if !isSuperAdmin && sess != nil {
		filter.OwnerID = sess.UserID
	}

	sites, err := h.store.Sites.List(c.Request().Context(), filter)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	// Also merge sites from user_sites assignments (for all non-super_admin users)
	if !isSuperAdmin && sess != nil {
		assigned, assignErr := h.store.UserSites.ListByUser(c.Request().Context(), sess.UserID)
		if assignErr == nil && len(assigned) > 0 {
			// Build set of owned site IDs (already in sites list)
			ownedIDs := make(map[int64]bool, len(sites))
			for _, s := range sites {
				ownedIDs[s.ID] = true
			}
			// Fetch each assigned site that isn't already owned
			for _, us := range assigned {
				if !ownedIDs[us.SiteID] {
					assignedSite, err := h.store.Sites.GetByID(c.Request().Context(), us.SiteID)
					if err == nil && assignedSite != nil {
						sites = append(sites, *assignedSite)
					}
				}
			}
		}
	}

	totalCount := len(sites)

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
		ServerID      int64  `json:"server_id"`
		Name          string `json:"name"`
		Domain        string `json:"domain"`
		Runtime       string `json:"runtime"`
		RootPath      string `json:"root_path,omitempty"`
		ManagedBy     string `json:"managed_by,omitempty"`
		SystemUser    string `json:"system_user,omitempty"`
		ConfFile      string `json:"conf_file,omitempty"`
		ProxyPassURL  string `json:"proxy_pass_url,omitempty"`
		PHPVersion    string `json:"php_version,omitempty"`
		DockerImage   string `json:"docker_image,omitempty"`
		DockerLanguage string `json:"docker_language,omitempty"`
		DockerName    string `json:"docker_name,omitempty"`
		DockerPorts   string `json:"docker_ports,omitempty"`
		DockerEnv     string `json:"docker_env,omitempty"`
		DockerVolume  string `json:"docker_volume,omitempty"`
		DockerRestart string `json:"docker_restart,omitempty"`
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
	}
	if body.ManagedBy != "sigardapanel" && body.ManagedBy != "cloudpanel" && body.ManagedBy != "external" {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid managed_by")
	}

	// Validate server exists
	server, err := h.store.Servers.GetByID(c.Request().Context(), body.ServerID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if server == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "server not found")
	}

	// Validate user max_sites quota
	if userID > 0 {
		owner, err := h.store.Users.GetByID(c.Request().Context(), userID)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
		}
		if owner != nil && owner.MaxSites.Valid {
			count, err := h.store.Sites.CountByOwner(c.Request().Context(), userID)
			if err != nil {
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
	payload, _ := json.Marshal(map[string]any{
		"site_id":        site.ID,
		"name":           site.Name,
		"domain":         site.Domain,
		"runtime":        site.Runtime,
		"root_path":      site.RootPath,
		"port":           port,
		"proxy_pass_url": body.ProxyPassURL,
		"php_version":    body.PHPVersion,
		"docker_image":    body.DockerImage,
		"docker_language": body.DockerLanguage,
		"docker_name":     body.DockerName,
		"docker_ports":    body.DockerPorts,
		"docker_env":      body.DockerEnv,
		"docker_volume":  body.DockerVolume,
		"docker_restart": body.DockerRestart,
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
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if site == nil {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}

	// Permission check: admin, owner, or user with admin permission on this site
	if !h.checkSitePermission(c.Request().Context(), sess, site, "admin") {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}

	// Clean up databases before deleting site
	if databases, err := h.store.Databases.ListBySite(c.Request().Context(), id); err == nil {
		for _, db := range databases {
			h.store.Databases.Delete(c.Request().Context(), db.ID)
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

	payload, _ := json.Marshal(map[string]any{
		"site_id":        site.ID,
		"domain":         site.Domain,
		"root_path":      site.RootPath,
		"repo":           site.GitRepo.String,
		"branch":         branch,
		"runtime":        site.Runtime,
		"app_status":     site.AppStatus,
		"app_port":       site.AppPort,
		"app_command":    site.AppCommand,
		"system_user":    site.SystemUser,
		"app_memory_max": site.AppMemoryMax,
	})
	job := &jobmodel.Job{
		ServerID:    sql.NullInt64{Int64: site.ServerID, Valid: true},
		SiteID:      sql.NullInt64{Int64: site.ID, Valid: true},
		Type:        jobmodel.TypeSiteDeploy,
		Description: fmt.Sprintf("Deploy site %s", site.Domain),
		Payload:     string(payload),
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
		if err := h.store.Sites.UpdateProxyPassURL(c.Request().Context(), id, *body.ProxyPassURL); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
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
	role := c.Get(roleKey).(string)

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

	if err := h.store.Sites.Transfer(c.Request().Context(), id, target.ID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	h.audit.RecordAction(c.Request().Context(), sess.UserID, audit.ActionTransfer, audit.ResourceSite, id, map[string]any{
		"from": site.OwnerID.Int64,
		"to":   target.ID,
	})

	h.store.Notify(c.Request().Context(), site.OwnerID.Int64, "site_transferred", "Site Transferred", fmt.Sprintf("Transferred %s to %s", site.Domain, target.Username), "info", "site", id)
	h.store.Notify(c.Request().Context(), target.ID, "site_received", "Site Received", fmt.Sprintf("You received %s", site.Domain), "success", "site", id)

	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      map[string]any{"transferred": true, "site_id": id, "new_owner_email": body.NewOwnerEmail, "new_owner_id": target.ID},
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
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if site == nil {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}

	if !h.canAccess(c.Request().Context(), sess, site) {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}

	var diskUsedBytes int64
	if site.RootPath != "" {
		out, err := execCommand("du", "-sb", site.RootPath)
		if err == nil {
			parts := strings.SplitN(out, "\t", 2)
			if len(parts) >= 1 {
				if sz, parseErr := strconv.ParseInt(parts[0], 10, 64); parseErr == nil {
					diskUsedBytes = sz
				}
			}
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
	return c.JSON(resp.StatusCode, httpjson.Success{Data: result, RequestID: rid})
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
	return c.JSON(resp.StatusCode, httpjson.Success{Data: result, RequestID: rid})
}

// gitRollback proxies to agent POST /tasks/git/rollback.
func (h *siteHandler) gitRollback(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	site, server, err := h.resolveSiteAndServer(c)
	if err != nil {
		return err
	}
	reqBody, _ := io.ReadAll(c.Request().Body)
	var body map[string]any
	json.Unmarshal(reqBody, &body)
	body["root_path"] = site.RootPath
	agentURL := strings.TrimRight(server.AgentURL.String, "/") + "/tasks/git/rollback"
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

// gitCheckout proxies to agent POST /tasks/git/checkout.
func (h *siteHandler) gitCheckout(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	site, server, err := h.resolveSiteAndServer(c)
	if err != nil {
		return err
	}
	reqBody, _ := io.ReadAll(c.Request().Body)
	var body map[string]any
	json.Unmarshal(reqBody, &body)
	body["root_path"] = site.RootPath
	agentURL := strings.TrimRight(server.AgentURL.String, "/") + "/tasks/git/checkout"
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

// resolveSiteAndServer resolves site and its server from URL params.
func (h *siteHandler) resolveSiteAndServer(c echo.Context) (*store.Site, *store.Server, error) {
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
	if !h.canAccess(c.Request().Context(), sessionFromEcho(c), site) {
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

	site, err := h.store.Sites.GetByID(c.Request().Context(), id)
	if err != nil || site == nil {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
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

	site, err := h.store.Sites.GetByID(c.Request().Context(), id)
	if err != nil || site == nil {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}

	var body struct {
		MemoryMax int64 `json:"memory_max"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid body")
	}

	if body.MemoryMax <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "memory_max is required (MB)")
	}

	// Update memory limit in DB
	if err := h.store.Sites.UpdateAppMemory(c.Request().Context(), id, body.MemoryMax); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "update failed")
	}

	// Restart the app to apply new limits
	server, err := h.store.Servers.GetByID(c.Request().Context(), site.ServerID)
	if err != nil || server == nil || !server.AgentURL.Valid || server.AgentToken == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "agent not configured")
	}

	svcName := fmt.Sprintf("sigarda-app-%d", site.ID)
	agentURL := strings.TrimRight(server.AgentURL.String, "/") + "/tasks/command/exec"
	restartPayload := map[string]string{
		"command": fmt.Sprintf("systemctl restart %s", svcName),
	}
	agentRequest("POST", agentURL, server.AgentToken, restartPayload)

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
