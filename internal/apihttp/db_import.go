package apihttp

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v5"

	"sigardapanel/internal/auth"
	"sigardapanel/internal/httpjson"
	"sigardapanel/internal/siteinput"
	"sigardapanel/internal/store"
)

// Importing existing databases: a super admin lists the databases that exist
// on a server (created outside the panel, e.g. by an application installed
// by hand) and attaches one to a site as an "external" database. The panel
// then shows it and uses it with the credentials it was given (viewer,
// manager, backups, phpMyAdmin), but never changes it on the server: it is
// not dropped when removed from the panel or with its site, its character
// set is not altered, and its accounts are never created, re-granted,
// rotated or dropped (see jobs/external_database.go for the worker side).

// errExternalDatabase answers a request that would change an external
// database on the server.
func errExternalDatabase() error {
	return echo.NewHTTPError(http.StatusConflict, "external database: the panel does not change it on the server (it was imported; manage it where it was created)")
}

// discoveredDB is one entry of the agent's /tasks/databases/discover answer.
type discoveredDB struct {
	Engine    string `json:"engine"`
	Name      string `json:"name"`
	Owner     string `json:"owner"`
	SizeBytes int64  `json:"size_bytes"`
}

// discoverOnServer asks the server's agent for the databases that exist on it.
func (h *dbHandler) discoverOnServer(ctx context.Context, server *store.Server) ([]discoveredDB, map[string]any, error) {
	if server == nil || !server.AgentURL.Valid || server.AgentURL.String == "" || server.AgentToken == "" {
		return nil, nil, echo.NewHTTPError(http.StatusConflict, "this server has no reachable agent")
	}
	code, out, err := stackAgentRequest(ctx, http.MethodPost, agentBase(server)+"/tasks/databases/discover", server.AgentToken, map[string]any{}, time.Minute)
	if code == http.StatusNotFound && (out == nil || agentMessage(out, "Not Found") == "Not Found") {
		return nil, nil, echo.NewHTTPError(http.StatusBadGateway, "the agent on this server is too old to list its databases; update the agent")
	}
	if err != nil {
		return nil, nil, echo.NewHTTPError(http.StatusBadGateway, "the server's agent could not be reached")
	}
	if code >= 400 {
		return nil, nil, agentHTTPError(code, out)
	}
	var list []discoveredDB
	raw, _ := out["databases"].([]any)
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		d := discoveredDB{}
		d.Engine, _ = m["engine"].(string)
		d.Name, _ = m["name"].(string)
		d.Owner, _ = m["owner"].(string)
		if f, ok := m["size_bytes"].(float64); ok {
			d.SizeBytes = int64(f)
		}
		if (d.Engine == "mysql" || d.Engine == "postgres") && d.Name != "" {
			list = append(list, d)
		}
	}
	errs, _ := out["errors"].(map[string]any)
	return list, errs, nil
}

// registeredOnServer returns "type/name" of every panel database whose site
// is on the server.
func (h *dbHandler) registeredOnServer(ctx context.Context, serverID int64) (map[string]bool, error) {
	all, err := h.store.Databases.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(all))
	siteServer := map[int64]int64{}
	for _, d := range all {
		sid, ok := siteServer[d.SiteID]
		if !ok {
			if site, _ := h.store.Sites.GetByID(ctx, d.SiteID); site != nil {
				sid = site.ServerID
			}
			siteServer[d.SiteID] = sid
		}
		if sid == serverID {
			out[d.Type+"/"+d.Name] = true
		}
	}
	return out, nil
}

// importableName reports why a database name cannot be imported ("" when it
// can): the panel's tools (agent tasks, backups) only accept plain
// identifiers, and the server's own databases are never attached to a site.
func importableName(engine, name string) string {
	if !dbNameRegex.MatchString(name) {
		return "the name has characters the panel does not support (letters, digits and _ only, up to 64)"
	}
	if siteinput.ReservedDatabaseName(name) {
		return "this database belongs to the database server"
	}
	if engine == "postgres" && name != strings.ToLower(name) {
		return "PostgreSQL database names with upper-case letters are not supported"
	}
	return ""
}

// discoverDatabases -- GET /api/v1/servers/:id/databases/discover (super admin)
func (h *dbHandler) discoverDatabases(c *echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	ctx := c.Request().Context()
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid server id")
	}
	server, err := h.store.Servers.GetByID(ctx, id)
	if err != nil || server == nil {
		return echo.NewHTTPError(http.StatusNotFound, "server not found")
	}
	list, errs, err := h.discoverOnServer(ctx, server)
	if err != nil {
		return err
	}
	registered, err := h.registeredOnServer(ctx, server.ID)
	if err != nil {
		slog.Error("api: request failed", "route", c.Path(), "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	out := make([]map[string]any, 0, len(list))
	for _, d := range list {
		reason := importableName(d.Engine, d.Name)
		out = append(out, map[string]any{
			"engine":     d.Engine,
			"name":       d.Name,
			"owner":      d.Owner,
			"size_bytes": d.SizeBytes,
			"registered": registered[d.Engine+"/"+d.Name],
			"importable": reason == "",
			"reason":     reason,
		})
	}
	data := map[string]any{"server_id": server.ID, "databases": out, "count": len(out)}
	if len(errs) > 0 {
		data["errors"] = errs
	}
	return c.JSON(http.StatusOK, httpjson.Success{Data: data, RequestID: rid})
}

// importDatabase -- POST /api/v1/databases/import (super admin)
// Body: {site_id, type, name, username?, password?}. The database must exist
// on the site's server (fresh discovery) and not be in the panel yet. Given
// credentials are checked by signing in with them on the site's server (the
// viewer's own connection) before anything is stored.
func (h *dbHandler) importDatabase(c *echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	ctx := c.Request().Context()
	sess := sessionFromEcho(c)
	var userID int64
	if sess != nil {
		userID = sess.UserID
	}

	var body struct {
		SiteID   int64  `json:"site_id"`
		Type     string `json:"type"`
		Name     string `json:"name"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid body")
	}
	body.Name = strings.TrimSpace(body.Name)
	body.Username = strings.TrimSpace(body.Username)
	if body.Type != "mysql" && body.Type != "postgres" {
		return echo.NewHTTPError(http.StatusBadRequest, "type must be mysql or postgres")
	}
	if reason := importableName(body.Type, body.Name); reason != "" {
		return echo.NewHTTPError(http.StatusBadRequest, reason)
	}
	if (body.Username == "") != (body.Password == "") {
		return echo.NewHTTPError(http.StatusBadRequest, "give both a username and a password, or neither")
	}
	if body.Username != "" {
		if !dbNameRegex.MatchString(body.Username) {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid username (letters, digits and _ only, up to 64)")
		}
		if isReservedMySQLUser(body.Username) || body.Username == "postgres" {
			return echo.NewHTTPError(http.StatusBadRequest, "server administrator accounts cannot be stored in the panel; use the application's own database account")
		}
		// The account's password must never be changed by the panel: refuse
		// one that panel-managed databases already use (rotating it there
		// would break the imported database's application).
		if rows, _ := h.store.DBUsers.ListByUsername(ctx, body.Username); len(rows) > 0 {
			for _, r := range rows {
				if d, _ := h.store.Databases.GetByID(ctx, r.DatabaseID); d != nil && !d.IsExternal() {
					return echo.NewHTTPError(http.StatusConflict, "this database user is already managed by the panel for another database; import with the application's own account")
				}
			}
		}
	}

	if body.SiteID <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "site_id is required")
	}
	site, err := h.store.Sites.GetByID(ctx, body.SiteID)
	if err != nil {
		slog.Error("api: request failed", "route", c.Path(), "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if site == nil {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}
	server, _ := h.store.Servers.GetByID(ctx, site.ServerID)
	if server == nil {
		return echo.NewHTTPError(http.StatusConflict, "the site's server is not in the panel")
	}

	registered, err := h.registeredOnServer(ctx, server.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if registered[body.Type+"/"+body.Name] {
		return echo.NewHTTPError(http.StatusConflict, "this database is already in the panel")
	}
	list, _, err := h.discoverOnServer(ctx, server)
	if err != nil {
		return err
	}
	found := false
	for _, d := range list {
		if d.Engine == body.Type && d.Name == body.Name {
			found = true
			break
		}
	}
	if !found {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("no %s database named %q on the site's server", engineLabel(body.Type), body.Name))
	}

	if body.Username != "" {
		if err := h.verifyDBLogin(ctx, site, body.Type, body.Name, body.Username, body.Password); err != nil {
			return err
		}
	}

	database := &store.Database{
		SiteID:    site.ID,
		Name:      body.Name,
		Type:      body.Type,
		ManagedBy: store.DatabaseManagedByExternal,
	}
	if err := h.store.Databases.Create(ctx, database); err != nil {
		slog.Error("api: request failed", "route", c.Path(), "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	var dbUserID int64
	if body.Username != "" {
		hashed, err := auth.HashPassword(body.Password)
		if err != nil {
			_ = h.store.Databases.Delete(ctx, database.ID)
			return echo.NewHTTPError(http.StatusInternalServerError, "hash failed")
		}
		enc, err := h.store.DBStore.EncryptPassword(body.Password)
		if err != nil {
			_ = h.store.Databases.Delete(ctx, database.ID)
			return echo.NewHTTPError(http.StatusInternalServerError, "could not encrypt the password")
		}
		u := &store.DBUser{
			DatabaseID:        database.ID,
			Username:          body.Username,
			PasswordHash:      hashed,
			EncryptedPassword: enc,
			HostPattern:       "localhost",
			// The panel never re-grants an external account; "ALL" only marks
			// it as the account the viewer, manager and backups use.
			Privileges: "ALL",
		}
		if err := h.store.DBUsers.Create(ctx, u); err != nil {
			_ = h.store.Databases.Delete(ctx, database.ID)
			return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
		}
		dbUserID = u.ID
	}

	h.audit.RecordAction(ctx, userID, "database_import", "database", database.ID, map[string]any{
		"database": body.Name, "type": body.Type, "site_id": site.ID, "domain": site.Domain,
		"server_id": server.ID, "account": body.Username, "external": true,
	})

	return c.JSON(http.StatusCreated, httpjson.Success{
		Data: map[string]any{
			"database_id": database.ID,
			"user_id":     dbUserID,
			"external":    true,
		},
		RequestID: rid,
	})
}

// verifyDBLogin signs in to the database with the given account on the
// site's server, through the agent's viewer (the same connection the viewer
// and manager use later), and lists its tables. A refused login is a 400.
func (h *dbHandler) verifyDBLogin(ctx context.Context, site *store.Site, dbType, name, user, pass string) error {
	sh := &siteHandler{store: h.store}
	payload := map[string]any{"type": dbType, "database": name, "user": user, "password": pass, "action": "tables"}
	code, out, err := sh.siteAgentCall(ctx, site, http.MethodPost, "/tasks/db/view", payload, 30*time.Second)
	if code == http.StatusNotFound && (out == nil || agentMessage(out, "Not Found") == "Not Found") {
		return echo.NewHTTPError(http.StatusBadGateway, "the agent on this server is too old to check database logins; update the agent")
	}
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "the site's server could not be reached to check the login")
	}
	if code == http.StatusUnauthorized || code == http.StatusForbidden {
		return agentHTTPError(code, out)
	}
	if code >= 400 {
		return echo.NewHTTPError(http.StatusBadRequest, "the database refused this login: "+agentMessage(out, "login failed"))
	}
	return nil
}

func engineLabel(t string) string {
	if t == "postgres" {
		return "PostgreSQL"
	}
	return "MySQL/MariaDB"
}

// unlinkExternalDatabase removes an external database from the panel only:
// its record and its accounts' records (with the stored passwords). Nothing
// is sent to the server: the database and its accounts stay as they are.
func (h *dbHandler) unlinkExternalDatabase(c *echo.Context, rid string, userID int64, database *store.Database) error {
	ctx := c.Request().Context()
	if err := removeDatabaseRecords(ctx, h.store, database.ID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	h.audit.RecordAction(ctx, userID, "database_unlink", "database", database.ID, map[string]any{
		"database": database.Name, "type": database.Type, "external": true,
	})
	h.store.NotifyAdmins(ctx, "db_deleted", "Database Removed",
		fmt.Sprintf("Database %s removed from the panel (external database: kept on the server)", database.Name), "info", "database", database.ID)
	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      map[string]any{"database_id": database.ID, "external": true},
		RequestID: rid,
	})
}

// removeDatabaseRecords deletes a database's panel records: its accounts'
// rows, then the database row. It never touches the database server.
func removeDatabaseRecords(ctx context.Context, s *store.Store, databaseID int64) error {
	if users, err := s.DBUsers.ListByDatabase(ctx, databaseID); err == nil {
		for _, u := range users {
			if err := s.DBUsers.Delete(ctx, u.ID); err != nil {
				return err
			}
		}
	}
	return s.Databases.Delete(ctx, databaseID)
}

// usedByExternalDatabase reports whether an account is stored for an external
// database (its password must then never be changed by the panel).
func (h *dbHandler) usedByExternalDatabase(ctx context.Context, username string) bool {
	rows, _ := h.store.DBUsers.ListByUsername(ctx, username)
	for _, r := range rows {
		if d, _ := h.store.Databases.GetByID(ctx, r.DatabaseID); d.IsExternal() {
			return true
		}
	}
	return false
}
