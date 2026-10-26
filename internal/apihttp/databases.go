package apihttp

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"sigardapanel/internal/audit"
	"sigardapanel/internal/auth"
	"sigardapanel/internal/dbserver"
	"sigardapanel/internal/httpjson"
	"sigardapanel/internal/jobmodel"
	"sigardapanel/internal/siteinput"
	"sigardapanel/internal/store"
)

type dbHandler struct {
	store *store.Store
	audit *audit.Recorder
}

func newDBHandler(s *store.Store, a *audit.Recorder) *dbHandler {
	return &dbHandler{store: s, audit: a}
}

var dbNameRegex = regexp.MustCompile(`^[a-zA-Z0-9_]{1,64}$`)

func canonicalDBHost(host string) string {
	value := strings.ToLower(strings.TrimSpace(host))
	if value == "" {
		return "%"
	}
	if value == "127.0.0.1" || value == "::1" {
		return "localhost"
	}
	return value
}

func canonicalDBUserKey(username, host string) string {
	return username + "@" + canonicalDBHost(host)
}

// createDatabase -- POST /api/v1/sites/:id/databases
func (h *dbHandler) createDatabase(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)
	var userID int64
	if sess != nil {
		userID = sess.UserID
	}

	siteID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || siteID <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid site id")
	}

	site, err := h.store.Sites.GetByID(c.Request().Context(), siteID)
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

	var body struct {
		Name      string `json:"name"`
		Type      string `json:"type"`
		Charset   string `json:"charset"`
		Collation string `json:"collation"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid body")
	}

	if !dbNameRegex.MatchString(body.Name) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid name (alphanumeric + underscore, 1-64)")
	}
	if body.Type != "mysql" && body.Type != "postgres" {
		return echo.NewHTTPError(http.StatusBadRequest, "type must be mysql or postgres")
	}
	if body.Charset == "" {
		body.Charset = "utf8mb4"
	}
	if body.Collation == "" {
		body.Collation = "utf8mb4_unicode_ci"
	}

	database := &store.Database{
		SiteID:    siteID,
		Name:      "sp_" + body.Name,
		Type:      body.Type,
		Charset:   body.Charset,
		Collation: body.Collation,
	}

	// Try direct MySQL first
	mysqlOK := false
	if body.Type == "mysql" {
		if err := h.execMySQL(func(conn *dbserver.MySQLConn) error {
			return conn.CreateDatabase(c.Request().Context(), body.Name, body.Charset, body.Collation)
		}); err == nil {
			mysqlOK = true
		} else {
			slog.Warn("database: direct mysql create failed, will try agent", "name", body.Name, "error", err)
		}
	}

	// Record in SigardaPanel metadata
	if err := h.store.Databases.Create(c.Request().Context(), database); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	// Fallback: enqueue agent job if direct MySQL failed
	jobID := int64(0)
	if !mysqlOK && body.Type == "mysql" {
		payload := fmt.Sprintf(`{"database_id":%d,"site_id":%d,"name":%q,"type":%q}`,
			database.ID, siteID, body.Name, body.Type)
		job := &jobmodel.Job{
			ServerID:    sql.NullInt64{Int64: site.ServerID, Valid: true},
			SiteID:      sql.NullInt64{Int64: site.ID, Valid: true},
			Type:        jobmodel.TypeDatabaseCreate,
			Description: fmt.Sprintf("Create database %s on %s", body.Name, site.Domain),
			Status:      jobmodel.StatusQueued,
			Payload:     payload,
		}
		if err := h.store.Jobs.Create(c.Request().Context(), job); err != nil {
			slog.Error("database: failed to enqueue agent job", "error", err)
		} else {
			jobID = job.ID
		}
	}

	h.audit.RecordAction(c.Request().Context(), userID, "database_create", "database", database.ID, nil)

	h.store.NotifySiteUsers(c.Request().Context(), siteID, "db_created", "Database Created", fmt.Sprintf("Database %s created", body.Name), "info", "database", 0)

	return c.JSON(http.StatusAccepted, httpjson.Success{
		Data: map[string]any{
			"database_id": database.ID,
			"job_id":      jobID,
		},
		RequestID: rid,
	})
}

// listDatabases -- GET /api/v1/sites/:id/databases
func (h *dbHandler) listDatabases(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)

	siteID, _ := strconv.ParseInt(c.Param("id"), 10, 64)

	site, _ := h.store.Sites.GetByID(c.Request().Context(), siteID)
	if site == nil {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}
	if !h.canAccess(c.Request().Context(), sess, site) {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}

	dbs, err := h.store.Databases.ListBySite(c.Request().Context(), siteID)
	if err != nil {
		slog.Error("api: request failed", "route", c.Path(), "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	// Get server host/port from default database server config
	serverHost := "localhost"
	serverPort := 3306
	if svr, _ := h.store.DBStore.GetDefault(c.Request().Context()); svr != nil {
		serverHost = svr.Host
		serverPort = svr.Port
	}

	out := make([]map[string]any, 0, len(dbs))
	for _, db := range dbs {
		out = append(out, map[string]any{
			"id":          db.ID,
			"site_id":     db.SiteID,
			"name":        db.Name,
			"type":        db.Type,
			"charset":     db.Charset,
			"collation":   db.Collation,
			"size_bytes":  db.SizeBytes,
			"created_at":  db.CreatedAt,
			"server_host": serverHost,
			"server_port": serverPort,
		})
	}

	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      map[string]any{"databases": out, "count": len(out)},
		RequestID: rid,
	})
}

// listAllDatabases -- GET /api/v1/databases
func (h *dbHandler) listAllDatabases(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)

	dbs, err := h.store.Databases.ListAll(c.Request().Context())
	if err != nil {
		slog.Error("api: request failed", "route", c.Path(), "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	// Get server host/port from default database server config
	serverHost := "localhost"
	serverPort := 3306
	if svr, _ := h.store.DBStore.GetDefault(c.Request().Context()); svr != nil {
		serverHost = svr.Host
		serverPort = svr.Port
	}

	// Enrich with site domain
	out := make([]map[string]any, 0, len(dbs))
	for _, db := range dbs {
		var domain string
		site, _ := h.store.Sites.GetByID(c.Request().Context(), db.SiteID)
		if site != nil {
			// Only show if user can access
			if !h.canAccess(c.Request().Context(), sess, site) {
				continue
			}
			domain = site.Domain
		}
		out = append(out, map[string]any{
			"id":          db.ID,
			"site_id":     db.SiteID,
			"domain":      domain,
			"name":        db.Name,
			"type":        db.Type,
			"charset":     db.Charset,
			"collation":   db.Collation,
			"size_bytes":  db.SizeBytes,
			"created_at":  db.CreatedAt,
			"server_host": serverHost,
			"server_port": serverPort,
		})
	}

	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      map[string]any{"databases": out, "count": len(out)},
		RequestID: rid,
	})
}

// rotateDBUserPassword -- POST /api/v1/databases/:id/users/:user_id/rotate-password
func (h *dbHandler) rotateDBUserPassword(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)
	var userID int64
	if sess != nil {
		userID = sess.UserID
	}

	dbID, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	uID, _ := strconv.ParseInt(c.Param("user_id"), 10, 64)

	database, _ := h.store.Databases.GetByID(c.Request().Context(), dbID)
	if database == nil {
		return echo.NewHTTPError(http.StatusNotFound, "database not found")
	}
	site, _ := h.store.Sites.GetByID(c.Request().Context(), database.SiteID)
	if !h.canAccess(c.Request().Context(), sess, site) {
		return echo.NewHTTPError(http.StatusNotFound, "database not found")
	}

	dbUser, _ := h.store.DBUsers.GetByID(c.Request().Context(), uID)
	if dbUser == nil || dbUser.DatabaseID != dbID {
		return echo.NewHTTPError(http.StatusNotFound, "user not found")
	}

	// Generate new random password
	newPassword := generateDBPassword()

	// Update password in MySQL directly first
	if database.Type == "mysql" {
		_ = h.execMySQL(func(conn *dbserver.MySQLConn) error {
			if err := conn.ChangePassword(c.Request().Context(), dbUser.Username, dbUser.HostPattern, newPassword); err != nil {
				slog.Warn("dbuser: mysql rotate password failed", "error", err)
				return err
			}
			return nil
		})
	}

	hashed, err := auth.HashPassword(newPassword)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "hash failed")
	}

	// Update in store
	if err := h.store.DBUsers.UpdatePassword(c.Request().Context(), dbUser.ID, hashed); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	// Enqueue job for agent (fallback)
	payload := fmt.Sprintf(`{"user_id":%d,"database_id":%d,"database_name":%q,"type":%q,"username":%q,"password":%q,"host_pattern":%q,"privileges":%q}`,
		dbUser.ID, dbID, database.Name, database.Type, dbUser.Username, newPassword, dbUser.HostPattern, dbUser.Privileges)
	job := &jobmodel.Job{
		ServerID:    sql.NullInt64{Int64: site.ServerID, Valid: true},
		SiteID:      sql.NullInt64{Int64: site.ID, Valid: true},
		Type:        jobmodel.TypeDBUserCreate,
		Description: fmt.Sprintf("Rotate password for %s on %s", dbUser.Username, site.Domain),
		Status:      jobmodel.StatusQueued,
		Payload:     payload,
	}
	if err := h.store.Jobs.Create(c.Request().Context(), job); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	h.audit.RecordAction(c.Request().Context(), userID, "dbuser_password_rotate", "dbuser", dbUser.ID, nil)

	return c.JSON(http.StatusAccepted, httpjson.Success{
		Data: map[string]any{
			"user_id":  dbUser.ID,
			"username": dbUser.Username,
			"password": newPassword,
			"job_id":   job.ID,
		},
		RequestID: rid,
	})
}

// generateDBPassword returns a random 16-char alphanumeric password.
func generateDBPassword() string {
	b := make([]byte, 12)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (h *dbHandler) getDatabase(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}

	database, err := h.store.Databases.GetByID(c.Request().Context(), id)
	if err != nil {
		slog.Error("api: request failed", "route", c.Path(), "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if database == nil {
		return echo.NewHTTPError(http.StatusNotFound, "database not found")
	}

	site, _ := h.store.Sites.GetByID(c.Request().Context(), database.SiteID)
	if site == nil {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}
	if !h.canAccess(c.Request().Context(), sess, site) {
		return echo.NewHTTPError(http.StatusNotFound, "database not found")
	}

	return c.JSON(http.StatusOK, httpjson.Success{
		Data: map[string]any{
			"id":         database.ID,
			"site_id":    database.SiteID,
			"name":       database.Name,
			"type":       database.Type,
			"charset":    database.Charset,
			"collation":  database.Collation,
			"size_bytes": database.SizeBytes,
			"created_at": database.CreatedAt,
		},
		RequestID: rid,
	})
}

// deleteDatabase -- DELETE /api/v1/databases/:id
func (h *dbHandler) deleteDatabase(c echo.Context) error {
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

	database, _ := h.store.Databases.GetByID(c.Request().Context(), id)
	if database == nil {
		return echo.NewHTTPError(http.StatusNotFound, "database not found")
	}

	site, _ := h.store.Sites.GetByID(c.Request().Context(), database.SiteID)
	if site == nil {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}
	if !h.canAccess(c.Request().Context(), sess, site) {
		return echo.NewHTTPError(http.StatusNotFound, "database not found")
	}

	// Drop from MySQL directly first
	if database.Type == "mysql" {
		_ = h.execMySQL(func(conn *dbserver.MySQLConn) error {
			if err := conn.DropDatabase(c.Request().Context(), database.Name); err != nil {
				slog.Warn("database: mysql drop failed", "name", database.Name, "error", err)
				return err
			}
			return nil
		})
	}

	// Enqueue delete job (fallback)
	payload := fmt.Sprintf(`{"database_id":%d,"site_id":%d,"name":%q,"type":%q}`,
		database.ID, database.SiteID, database.Name, database.Type)
	job := &jobmodel.Job{
		ServerID:    sql.NullInt64{Int64: site.ServerID, Valid: true},
		SiteID:      sql.NullInt64{Int64: site.ID, Valid: true},
		Type:        jobmodel.TypeDatabaseDelete,
		Description: fmt.Sprintf("Delete database %s on %s", database.Name, site.Domain),
		Status:      jobmodel.StatusQueued,
		Payload:     payload,
	}
	if err := h.store.Jobs.Create(c.Request().Context(), job); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	h.audit.RecordAction(c.Request().Context(), userID, "database_delete", "database", id, nil)

	// Hard delete from DB (since cleanup is async via agent)
	if err := h.store.Databases.Delete(c.Request().Context(), id); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	h.store.NotifyAdmins(c.Request().Context(), "db_deleted", "Database Deleted", fmt.Sprintf("Database %s deleted", database.Name), "warning", "database", id)

	return c.JSON(http.StatusAccepted, httpjson.Success{
		Data:      map[string]any{"database_id": id, "job_id": job.ID},
		RequestID: rid,
	})
}

// createDBUser -- POST /api/v1/databases/:id/users
func (h *dbHandler) createDBUser(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)
	var userID int64
	if sess != nil {
		userID = sess.UserID
	}

	dbID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || dbID <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid database id")
	}

	database, _ := h.store.Databases.GetByID(c.Request().Context(), dbID)
	if database == nil {
		return echo.NewHTTPError(http.StatusNotFound, "database not found")
	}

	site, _ := h.store.Sites.GetByID(c.Request().Context(), database.SiteID)
	if !h.canAccess(c.Request().Context(), sess, site) {
		return echo.NewHTTPError(http.StatusNotFound, "database not found")
	}

	var body struct {
		Username    string `json:"username"`
		Password    string `json:"password"`
		HostPattern string `json:"host_pattern"`
		Privileges  string `json:"privileges"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid body")
	}

	if !dbNameRegex.MatchString(body.Username) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid username")
	}
	if err := siteinput.DBPassword(body.Password); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	body.HostPattern = canonicalDBHost(body.HostPattern)
	if err := siteinput.DBHost(body.HostPattern); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if body.Privileges == "" {
		body.Privileges = "ALL"
	}
	if err := siteinput.DBPrivileges(body.Privileges); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	// Create user in MySQL directly first
	mysqlOK := false
	if database.Type == "mysql" {
		slog.Info("dbuser: attempting MySQL create", "user", body.Username, "db", database.Name, "host", body.HostPattern)
		if err := h.execMySQL(func(conn *dbserver.MySQLConn) error {
			slog.Info("dbuser: calling CreateUser", "user", body.Username)
			if err := conn.CreateUser(c.Request().Context(), body.Username, body.Password, body.HostPattern, database.Name, body.Privileges); err != nil {
				slog.Warn("dbuser: mysql create failed", "error", err)
				return err
			}
			mysqlOK = true
			slog.Info("dbuser: mysql create success", "user", body.Username)
			return nil
		}); err != nil {
			slog.Error("dbuser: execMySQL failed", "error", err)
		}
		slog.Info("dbuser: mysqlOK", "mysqlOK", mysqlOK)
	}

	hashed, err := auth.HashPassword(body.Password)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "hash failed")
	}

	// Encrypt and store the plaintext password for phpMyAdmin auto-login
	encPassword := ""
	if mysqlOK {
		enc, err := h.store.DBStore.EncryptPassword(body.Password)
		if err == nil {
			encPassword = enc
		}
	}

	dbUser := &store.DBUser{
		DatabaseID:        dbID,
		Username:          body.Username,
		PasswordHash:      hashed,
		EncryptedPassword: encPassword,
		HostPattern:       body.HostPattern,
		Privileges:        body.Privileges,
	}
	if err := h.store.DBUsers.Create(c.Request().Context(), dbUser); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	// Only enqueue agent job if MySQL direct failed
	jobID := int64(0)
	if !mysqlOK {
		payload := fmt.Sprintf(`{"user_id":%d,"database_id":%d,"database_name":%q,"type":%q,"username":%q,"password":%q,"host_pattern":%q,"privileges":%q}`,
			dbUser.ID, dbID, database.Name, database.Type, body.Username, body.Password, body.HostPattern, body.Privileges)
		job := &jobmodel.Job{
			ServerID:    sql.NullInt64{Int64: site.ServerID, Valid: true},
			SiteID:      sql.NullInt64{Int64: site.ID, Valid: true},
			Type:        jobmodel.TypeDBUserCreate,
			Description: fmt.Sprintf("Create user %s for %s", body.Username, site.Domain),
			Status:      jobmodel.StatusQueued,
			Payload:     payload,
		}
		if err := h.store.Jobs.Create(c.Request().Context(), job); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
		}
		jobID = job.ID
	}

	h.audit.RecordAction(c.Request().Context(), userID, "dbuser_create", "dbuser", dbUser.ID, nil)

	h.store.NotifySiteUsers(c.Request().Context(), database.SiteID, "db_user_created", "DB User Created", fmt.Sprintf("User %s created", body.Username), "info", "database", 0)

	return c.JSON(http.StatusAccepted, httpjson.Success{
		Data: map[string]any{
			"user_id":  dbUser.ID,
			"username": body.Username,
			"password": body.Password,
			"job_id":   jobID,
		},
		RequestID: rid,
	})
}

// listDBUsers -- GET /api/v1/databases/:id/users
func (h *dbHandler) listDBUsers(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)

	dbID, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	database, _ := h.store.Databases.GetByID(c.Request().Context(), dbID)
	if database == nil {
		return echo.NewHTTPError(http.StatusNotFound, "database not found")
	}

	site, _ := h.store.Sites.GetByID(c.Request().Context(), database.SiteID)
	if !h.canAccess(c.Request().Context(), sess, site) {
		return echo.NewHTTPError(http.StatusNotFound, "database not found")
	}

	users, err := h.store.DBUsers.ListByDatabase(c.Request().Context(), dbID)
	if err != nil {
		slog.Error("api: request failed", "route", c.Path(), "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	out := make([]map[string]any, 0, len(users))
	for _, u := range users {
		out = append(out, map[string]any{
			"id":           u.ID,
			"database_id":  u.DatabaseID,
			"username":     u.Username,
			"host_pattern": u.HostPattern,
			"privileges":   u.Privileges,
			"created_at":   u.CreatedAt,
		})
	}

	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      map[string]any{"users": out, "count": len(out)},
		RequestID: rid,
	})
}

// deleteDBUser -- DELETE /api/v1/databases/:id/users/:user_id
func (h *dbHandler) deleteDBUser(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)
	var userID int64
	if sess != nil {
		userID = sess.UserID
	}

	dbID, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	userIDParam, _ := strconv.ParseInt(c.Param("user_id"), 10, 64)

	dbUser, _ := h.store.DBUsers.GetByID(c.Request().Context(), userIDParam)
	if dbUser == nil || dbUser.DatabaseID != dbID {
		return echo.NewHTTPError(http.StatusNotFound, "user not found")
	}

	database, _ := h.store.Databases.GetByID(c.Request().Context(), dbID)
	site, _ := h.store.Sites.GetByID(c.Request().Context(), database.SiteID)
	if !h.canAccess(c.Request().Context(), sess, site) {
		return echo.NewHTTPError(http.StatusNotFound, "user not found")
	}

	// Delete from MySQL directly first
	if database.Type == "mysql" {
		_ = h.execMySQL(func(conn *dbserver.MySQLConn) error {
			if err := conn.DeleteUser(c.Request().Context(), dbUser.Username, dbUser.HostPattern); err != nil {
				slog.Warn("dbuser: mysql delete failed", "error", err)
				return err
			}
			return nil
		})
	}

	// Enqueue agent job as fallback
	payload := fmt.Sprintf(`{"user_id":%d,"database_id":%d,"database_name":%q,"type":%q,"username":%q,"host_pattern":%q}`,
		dbUser.ID, dbID, database.Name, database.Type, dbUser.Username, dbUser.HostPattern)
	job := &jobmodel.Job{
		ServerID:    sql.NullInt64{Int64: site.ServerID, Valid: true},
		SiteID:      sql.NullInt64{Int64: site.ID, Valid: true},
		Type:        jobmodel.TypeDBUserDelete,
		Description: fmt.Sprintf("Delete user %s from %s", dbUser.Username, site.Domain),
		Status:      jobmodel.StatusQueued,
		Payload:     payload,
	}
	if err := h.store.Jobs.Create(c.Request().Context(), job); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	h.audit.RecordAction(c.Request().Context(), userID, "dbuser_delete", "dbuser", dbUser.ID, nil)

	if err := h.store.DBUsers.Delete(c.Request().Context(), dbUser.ID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	return c.JSON(http.StatusAccepted, httpjson.Success{
		Data:      map[string]any{"user_id": dbUser.ID, "job_id": job.ID},
		RequestID: rid,
	})
}

func (h *dbHandler) canAccess(ctx context.Context, sess *store.Session, site *store.Site) bool {
	if sess == nil {
		return false
	}
	user, _ := h.store.Users.GetByID(ctx, sess.UserID)
	if user == nil {
		return false
	}
	if auth.HasRole(user.Role, auth.RoleAdmin) {
		return true
	}
	if site.OwnerID.Valid && site.OwnerID.Int64 == sess.UserID {
		return true
	}
	return false
}

// execMySQL connects to the default database server and runs a function.
// Returns nil if no database server configured (graceful skip).
func (h *dbHandler) execMySQL(fn func(conn *dbserver.MySQLConn) error) error {
	return execMySQLConn(h.store, fn)
}

// execMySQLConn opens a short-lived admin connection to the configured
// default database server. Shared by handlers that need direct MySQL access.
func execMySQLConn(s *store.Store, fn func(conn *dbserver.MySQLConn) error) error {
	svr, err := s.DBStore.GetDefault(context.Background())
	if err != nil || svr == nil {
		return fmt.Errorf("no database server")
	}
	pass, err := s.DBStore.DecryptPassword(svr.PasswordEncrypted)
	if err != nil {
		return fmt.Errorf("decrypt: %w", err)
	}
	conn, err := dbserver.Connect(svr, pass)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer conn.Close()
	return fn(conn)
}

// phpMyAdminCredentials creates a dedicated, short-lived-login account for one database.
// Only callable from localhost with a signed request from the web process.
// POST /api/v1/databases/phpmyadmin-credentials
func (h *dbHandler) phpMyAdminCredentials(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)

	var body struct {
		DatabaseID int64  `json:"database_id"`
		Expires    int64  `json:"expires"`
		Signature  string `json:"signature"`
	}
	if err := c.Bind(&body); err != nil || body.DatabaseID <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "valid database_id required")
	}
	secret := os.Getenv("SIGARDAPANEL_PMA_SECRET")
	if len(secret) < 32 || body.Expires <= time.Now().Unix() || body.Expires > time.Now().Add(2*time.Minute).Unix() {
		return echo.NewHTTPError(http.StatusUnauthorized, "invalid signed request")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(mac, "%d:%d", body.DatabaseID, body.Expires)
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(body.Signature)) {
		return echo.NewHTTPError(http.StatusUnauthorized, "invalid signed request")
	}

	database, err := h.store.Databases.GetByID(c.Request().Context(), body.DatabaseID)
	if err != nil || database == nil || database.Type != "mysql" {
		return echo.NewHTTPError(http.StatusNotFound, "mysql database not found")
	}

	// Unique random principal per ticket prevents concurrent sessions from
	// rotating each other's credentials. Host=localhost blocks remote DB login.
	suffix := make([]byte, 6)
	if _, err := rand.Read(suffix); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "credential generation failed")
	}
	username := fmt.Sprintf("sp_pma_%d_%s", database.ID, hex.EncodeToString(suffix))
	password := generateDBPassword()
	const host = "localhost"
	if err := h.execMySQL(func(conn *dbserver.MySQLConn) error {
		if err := conn.CreateUser(c.Request().Context(), username, password, host, database.Name, "ALL"); err != nil {
			return fmt.Errorf("create dedicated user: %w", err)
		}
		if err := conn.ScheduleDeleteUser(c.Request().Context(), username, host, time.Hour+time.Minute); err != nil {
			_ = conn.DeleteUser(context.Background(), username, host)
			return fmt.Errorf("schedule dedicated user cleanup: %w", err)
		}
		return nil
	}); err != nil {
		slog.Error("phpmyadmin: failed to prepare dedicated database login", "database_id", database.ID, "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to prepare phpMyAdmin login")
	}

	return c.JSON(http.StatusOK, httpjson.Success{
		Data: map[string]any{
			"username": username,
			"password": password,
		},
		RequestID: rid,
	})
}

// phpMyAdminResetPasswords resets MySQL passwords for all users that have
// password_hash = "imported" (i.e., imported from CloudPanel without a known password).
// POST /api/v1/databases/phpmyadmin-reset-passwords
func (h *dbHandler) phpMyAdminResetPasswords(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)
	var userID int64
	if sess != nil {
		userID = sess.UserID
	}

	// Get the default database server
	svr, err := h.store.DBStore.GetDefault(c.Request().Context())
	if err != nil || svr == nil {
		return echo.NewHTTPError(http.StatusNotFound, "no database server configured")
	}

	decPass, err := h.store.DBStore.DecryptPassword(svr.PasswordEncrypted)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "decrypt failed")
	}

	conn, err := dbserver.Connect(svr, decPass)
	if err != nil {
		return echo.NewHTTPError(http.StatusFailedDependency, "mysql connection failed: "+err.Error())
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(c.Request().Context(), 60*time.Second)
	defer cancel()

	// Get all DB users that need password reset
	// We need to scan all databases
	dbs, _ := h.store.Databases.ListAll(ctx)
	resetCount := 0
	for _, database := range dbs {
		users, err := h.store.DBUsers.ListByDatabase(ctx, database.ID)
		if err != nil {
			continue
		}
		for _, u := range users {
			if u.PasswordHash != "imported" && u.EncryptedPassword != "" {
				continue // Already has a stored password
			}

			// Generate new password
			newPassword := generateDBPassword()

			// Reset in MySQL
			if err := conn.ChangePassword(ctx, u.Username, u.HostPattern, newPassword); err != nil {
				slog.Warn("phpmyadmin: reset password failed", "user", u.Username, "error", err)
				continue
			}

			// Store encrypted
			encrypted, err := h.store.DBStore.EncryptPassword(newPassword)
			if err != nil {
				slog.Warn("phpmyadmin: encrypt password failed", "user", u.Username, "error", err)
				continue
			}

			if err := h.store.DBUsers.UpdateEncryptedPassword(ctx, u.ID, encrypted); err != nil {
				slog.Warn("phpmyadmin: store encrypted password failed", "user", u.Username, "error", err)
				continue
			}

			// Update password_hash from "imported" to bcrypt so we know it's managed
			hashed, _ := auth.HashPassword(newPassword)
			if hashed != "" {
				_ = h.store.DBUsers.UpdatePassword(ctx, u.ID, hashed)
			}

			resetCount++
		}
	}

	h.audit.RecordAction(ctx, userID, "phpmyadmin_reset_passwords", "database", 0, map[string]any{
		"reset_count": resetCount,
	})

	return c.JSON(http.StatusOK, httpjson.Success{
		Data: map[string]any{
			"reset_count": resetCount,
		},
		RequestID: rid,
	})
}

// listAllDBUsers lists all database users across all databases (global).
// GET /api/v1/db-users
func (h *dbHandler) listAllDBUsers(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)

	allUsers := []map[string]any{}

	dbs, err := h.store.Databases.ListAll(c.Request().Context())
	if err != nil {
		slog.Error("api: request failed", "route", c.Path(), "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	for _, database := range dbs {
		site, _ := h.store.Sites.GetByID(c.Request().Context(), database.SiteID)
		if !h.canAccess(c.Request().Context(), sess, site) {
			continue
		}
		var domain string
		if site != nil {
			domain = site.Domain
		}

		users, err := h.store.DBUsers.ListByDatabase(c.Request().Context(), database.ID)
		if err != nil {
			continue
		}
		for _, u := range users {
			allUsers = append(allUsers, map[string]any{
				"id":            u.ID,
				"database_id":   u.DatabaseID,
				"database_name": database.Name,
				"site_id":       database.SiteID,
				"domain":        domain,
				"username":      u.Username,
				"host_pattern":  u.HostPattern,
				"privileges":    u.Privileges,
				"created_at":    u.CreatedAt,
			})
		}
	}

	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      map[string]any{"users": allUsers, "count": len(allUsers)},
		RequestID: rid,
	})
}

// updateDBUser updates a database user's host_pattern and/or privileges.
// PATCH /api/v1/db-users/:id
func (h *dbHandler) updateDBUser(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)
	var actorID int64
	if sess != nil {
		actorID = sess.UserID
	}

	userID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || userID <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid user id")
	}

	dbUser, _ := h.store.DBUsers.GetByID(c.Request().Context(), userID)
	if dbUser == nil {
		return echo.NewHTTPError(http.StatusNotFound, "user not found")
	}

	database, _ := h.store.Databases.GetByID(c.Request().Context(), dbUser.DatabaseID)
	if database == nil {
		return echo.NewHTTPError(http.StatusNotFound, "database not found")
	}
	site, _ := h.store.Sites.GetByID(c.Request().Context(), database.SiteID)
	if !h.canAccess(c.Request().Context(), sess, site) {
		return echo.NewHTTPError(http.StatusNotFound, "user not found")
	}

	var body struct {
		HostPattern *string `json:"host_pattern"`
		Privileges  *string `json:"privileges"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid body")
	}

	// Build update
	newHost := dbUser.HostPattern
	newPrivs := dbUser.Privileges
	needsMySQL := false

	if body.HostPattern != nil {
		newHost = canonicalDBHost(*body.HostPattern)
		if err := siteinput.DBHost(newHost); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		needsMySQL = true
	}
	if body.Privileges != nil {
		newPrivs = *body.Privileges
		if err := siteinput.DBPrivileges(newPrivs); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		needsMySQL = true
	}

	// Update in MySQL (drop + recreate for host pattern change, or just grant for privs change)
	if needsMySQL && database.Type == "mysql" {
		_ = h.execMySQL(func(conn *dbserver.MySQLConn) error {
			if body.HostPattern != nil && *body.HostPattern != dbUser.HostPattern {
				// Host changed: drop old user by old host, create at new host
				if err := conn.DeleteUser(c.Request().Context(), dbUser.Username, dbUser.HostPattern); err != nil {
					slog.Warn("dbuser: delete old host failed", "error", err)
					return err
				}
				// Recreate with same password — need to get it from encrypted store
				// If encrypted password is available, use it; otherwise skip host change
				if dbUser.EncryptedPassword != "" {
					pass, decErr := h.store.DBStore.DecryptPassword(dbUser.EncryptedPassword)
					if decErr == nil {
						if err := conn.CreateUser(c.Request().Context(), dbUser.Username, pass, newHost, database.Name, newPrivs); err != nil {
							slog.Warn("dbuser: recreate at new host failed", "error", err)
							return err
						}
					}
				}
			} else {
				// Just grant with new privileges
				if err := conn.GrantPrivileges(c.Request().Context(), dbUser.Username, newHost, database.Name, newPrivs); err != nil {
					slog.Warn("dbuser: grant failed", "error", err)
					return err
				}
			}
			return nil
		})
	}

	// Update in store
	if err := h.store.DBUsers.UpdateHostAndPrivileges(c.Request().Context(), dbUser.ID, newHost, newPrivs); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	h.audit.RecordAction(c.Request().Context(), actorID, "dbuser_update", "dbuser", dbUser.ID, map[string]any{
		"host_before": dbUser.HostPattern, "host_after": newHost,
		"privs_before": dbUser.Privileges, "privs_after": newPrivs,
	})

	return c.JSON(http.StatusOK, httpjson.Success{
		Data: map[string]any{
			"id":           dbUser.ID,
			"username":     dbUser.Username,
			"host_pattern": newHost,
			"privileges":   newPrivs,
		},
		RequestID: rid,
	})
}

// updateDatabase updates a database's charset and/or collation.
// PATCH /api/v1/databases/:id
func (h *dbHandler) updateDatabase(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)
	var actorID int64
	if sess != nil {
		actorID = sess.UserID
	}

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}

	database, _ := h.store.Databases.GetByID(c.Request().Context(), id)
	if database == nil {
		return echo.NewHTTPError(http.StatusNotFound, "database not found")
	}
	site, _ := h.store.Sites.GetByID(c.Request().Context(), database.SiteID)
	if !h.canAccess(c.Request().Context(), sess, site) {
		return echo.NewHTTPError(http.StatusNotFound, "database not found")
	}

	var body struct {
		Charset   *string `json:"charset"`
		Collation *string `json:"collation"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid body")
	}

	newCharset := database.Charset
	newCollation := database.Collation
	if body.Charset != nil {
		newCharset = *body.Charset
	}
	if body.Collation != nil {
		newCollation = *body.Collation
	}

	// Alter in MySQL if charset/collation changed
	if (body.Charset != nil || body.Collation != nil) && database.Type == "mysql" {
		_ = h.execMySQL(func(conn *dbserver.MySQLConn) error {
			alterSQL := fmt.Sprintf("ALTER DATABASE `%s` CHARACTER SET %s COLLATE %s",
				database.Name, newCharset, newCollation)
			if _, err := conn.Exec(alterSQL); err != nil {
				slog.Warn("database: alter charset failed", "error", err)
				return err
			}
			return nil
		})
	}

	if err := h.store.Databases.UpdateCharset(c.Request().Context(), id, newCharset, newCollation); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	h.audit.RecordAction(c.Request().Context(), actorID, "database_update", "database", id, map[string]any{
		"charset_before": database.Charset, "charset_after": newCharset,
		"collation_before": database.Collation, "collation_after": newCollation,
	})

	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      map[string]any{"id": id, "charset": newCharset, "collation": newCollation},
		RequestID: rid,
	})
}

// grantExistingUser grants an existing MySQL user access to this database.
// POST /api/v1/databases/:id/grant-user
func (h *dbHandler) grantExistingUser(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)
	var actorID int64
	if sess != nil {
		actorID = sess.UserID
	}

	dbID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || dbID <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid database id")
	}

	database, _ := h.store.Databases.GetByID(c.Request().Context(), dbID)
	if database == nil {
		return echo.NewHTTPError(http.StatusNotFound, "database not found")
	}
	site, _ := h.store.Sites.GetByID(c.Request().Context(), database.SiteID)
	if !h.canAccess(c.Request().Context(), sess, site) {
		return echo.NewHTTPError(http.StatusNotFound, "database not found")
	}

	var body struct {
		Username    string `json:"username"`
		HostPattern string `json:"host_pattern"`
		Privileges  string `json:"privileges"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid body")
	}
	if body.Username == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "username required")
	}
	body.HostPattern = canonicalDBHost(body.HostPattern)
	if body.Privileges == "" {
		body.Privileges = "ALL"
	}

	// Check if this exact user@host is already assigned to this database.
	// Same username can exist with different MySQL hosts (%, localhost, 127.0.0.1),
	// so do NOT match by username only.
	var existing *store.DBUser
	assignedUsers, _ := h.store.DBUsers.ListByDatabase(c.Request().Context(), dbID)
	requestKey := canonicalDBUserKey(body.Username, body.HostPattern)
	for i := range assignedUsers {
		if canonicalDBUserKey(assignedUsers[i].Username, assignedUsers[i].HostPattern) == requestKey {
			existing = &assignedUsers[i]
			break
		}
	}
	if existing != nil {
		// Already assigned — just update privileges for this exact user@host.
		if database.Type == "mysql" {
			_ = h.execMySQL(func(conn *dbserver.MySQLConn) error {
				if err := conn.GrantPrivileges(c.Request().Context(), body.Username, body.HostPattern, database.Name, body.Privileges); err != nil {
					slog.Warn("grant: mysql grant failed", "error", err)
					return err
				}
				return nil
			})
		}
		if err := h.store.DBUsers.UpdateHostAndPrivileges(c.Request().Context(), existing.ID, body.HostPattern, body.Privileges); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
		}
		h.audit.RecordAction(c.Request().Context(), actorID, "dbuser_grant_update", "dbuser", existing.ID, nil)
		return c.JSON(http.StatusOK, httpjson.Success{
			Data:      map[string]any{"user_id": existing.ID, "username": body.Username, "host_pattern": body.HostPattern, "action": "updated"},
			RequestID: rid,
		})
	}

	// Ensure user exists in MySQL
	if database.Type == "mysql" {
		_ = h.execMySQL(func(conn *dbserver.MySQLConn) error {
			if err := conn.GrantPrivileges(c.Request().Context(), body.Username, body.HostPattern, database.Name, body.Privileges); err != nil {
				slog.Warn("grant: mysql grant failed", "error", err)
				// Don't fail — maybe user doesn't exist in MySQL yet; we'll log and continue
			}
			return nil
		})
	}

	// Generate password for phpMyAdmin auto-login
	newPassword := generateDBPassword()
	hashed, _ := auth.HashPassword(newPassword)
	encPassword := ""
	if database.Type == "mysql" {
		enc, err := h.store.DBStore.EncryptPassword(newPassword)
		if err == nil {
			encPassword = enc
		}
		// Set the password in MySQL for the user
		_ = h.execMySQL(func(conn *dbserver.MySQLConn) error {
			return conn.ChangePassword(c.Request().Context(), body.Username, body.HostPattern, newPassword)
		})
	}

	dbUser := &store.DBUser{
		DatabaseID:        dbID,
		Username:          body.Username,
		PasswordHash:      hashed,
		EncryptedPassword: encPassword,
		HostPattern:       body.HostPattern,
		Privileges:        body.Privileges,
	}
	if err := h.store.DBUsers.Create(c.Request().Context(), dbUser); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	h.audit.RecordAction(c.Request().Context(), actorID, "dbuser_grant", "dbuser", dbUser.ID, nil)

	return c.JSON(http.StatusCreated, httpjson.Success{
		Data: map[string]any{
			"user_id":  dbUser.ID,
			"username": body.Username,
			"password": newPassword,
		},
		RequestID: rid,
	})
}

// listAvailableMySQLUsers lists MySQL users NOT yet assigned to this database.
// GET /api/v1/databases/:id/available-users
func (h *dbHandler) listAvailableMySQLUsers(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)

	dbID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || dbID <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid database id")
	}

	database, _ := h.store.Databases.GetByID(c.Request().Context(), dbID)
	if database == nil {
		return echo.NewHTTPError(http.StatusNotFound, "database not found")
	}
	site, _ := h.store.Sites.GetByID(c.Request().Context(), database.SiteID)
	if !h.canAccess(c.Request().Context(), sess, site) {
		return echo.NewHTTPError(http.StatusNotFound, "database not found")
	}

	// Get already-assigned users. Treat localhost, 127.0.0.1, and ::1 as one local host.
	assignedUsers, _ := h.store.DBUsers.ListByDatabase(c.Request().Context(), dbID)
	assigned := make(map[string]bool)
	for _, u := range assignedUsers {
		assigned[canonicalDBUserKey(u.Username, u.HostPattern)] = true
	}

	// Get MySQL users
	var mysqlUsers []dbserver.DBUserInfo
	if database.Type == "mysql" {
		_ = h.execMySQL(func(conn *dbserver.MySQLConn) error {
			var err error
			mysqlUsers, err = conn.ListUsers(c.Request().Context())
			return err
		})
	}

	// Filter: only return unassigned users
	type userEntry struct {
		Username string `json:"username"`
		Host     string `json:"host"`
	}
	available := make([]userEntry, 0)
	seen := make(map[string]bool)
	for _, u := range mysqlUsers {
		host := canonicalDBHost(u.Host)
		key := canonicalDBUserKey(u.Username, host)
		if !assigned[key] && !seen[key] {
			available = append(available, userEntry{Username: u.Username, Host: host})
			seen[key] = true
		}
	}

	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      map[string]any{"users": available, "count": len(available)},
		RequestID: rid,
	})
}

// pmaCredentials returns MySQL credentials for phpMyAdmin per-site.
// Finds the MySQL user that has ALL PRIVILEGES on the database.
// Uses encrypted password from db_users if available, otherwise phpmyadmin control user.
func (h *dbHandler) pmaCredentials(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)
	if sess == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}

	dbID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || dbID <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid database id")
	}

	database, err := h.store.Databases.GetByID(c.Request().Context(), dbID)
	if err != nil || database == nil {
		return echo.NewHTTPError(http.StatusNotFound, "database not found")
	}

	// Check access
	site, _ := h.store.Sites.GetByID(c.Request().Context(), database.SiteID)
	if !h.canAccess(c.Request().Context(), sess, site) {
		return echo.NewHTTPError(http.StatusNotFound, "database not found")
	}

	// Find the MySQL user for this database from db_users
	dbUsers, _ := h.store.DBUsers.ListByDatabase(c.Request().Context(), dbID)
	for _, u := range dbUsers {
		if u.EncryptedPassword != "" {
			pass, err := h.store.DBStore.DecryptPassword(u.EncryptedPassword)
			if err == nil && pass != "" {
				return c.JSON(http.StatusOK, httpjson.Success{
					Data: map[string]any{
						"username": u.Username,
						"password": pass,
						"host":     u.HostPattern,
					},
					RequestID: rid,
				})
			}
		}
	}

	// 2. Fallback: generate a dedicated temporary login for this database
	if database.Type == "mysql" {
		suffix := make([]byte, 4)
		_, _ = rand.Read(suffix)
		username := fmt.Sprintf("pma_%d_%s", database.ID, hex.EncodeToString(suffix))
		password := generateDBPassword()
		if err := h.execMySQL(func(conn *dbserver.MySQLConn) error {
			return conn.CreateUser(c.Request().Context(), username, password, "localhost", database.Name, "ALL")
		}); err == nil {
			return c.JSON(http.StatusOK, httpjson.Success{
				Data: map[string]any{
					"username": username,
					"password": password,
					"host":     "localhost",
				},
				RequestID: rid,
			})
		}
	}

	// 3. Fallback: phpmyadmin control user from config-db.php
	configData, err := os.ReadFile("/etc/phpmyadmin/config-db.php")
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "phpmyadmin config not found")
	}
	cfg := string(configData)

	pmaUser := extractPHPVar(cfg, "dbuser")
	pmaPass := extractPHPVar(cfg, "dbpass")
	if pmaUser == "" || pmaPass == "" {
		return echo.NewHTTPError(http.StatusInternalServerError, "phpmyadmin credentials not configured")
	}

	return c.JSON(http.StatusOK, httpjson.Success{
		Data: map[string]any{
			"username": pmaUser,
			"password": pmaPass,
			"host":     "localhost",
		},
		RequestID: rid,
	})
}

// extractPHPVar extracts a PHP variable value from config-db.php content.
func extractPHPVar(content, varName string) string {
	// Match: $varname='value'; or $varname="value";
	re := regexp.MustCompile(`\$` + varName + `\s*=\s*['"]([^'"]+)['"]`)
	matches := re.FindStringSubmatch(content)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

// execSQL executes raw SQL on the default database server (admin only).
func (h *dbHandler) execSQL(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)

	var body struct {
		SQL string `json:"sql"`
	}
	if err := c.Bind(&body); err != nil || body.SQL == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "sql required")
	}

	var result string
	if err := h.execMySQL(func(conn *dbserver.MySQLConn) error {
		upper := strings.ToUpper(strings.TrimSpace(body.SQL))
		if strings.HasPrefix(upper, "SELECT") || strings.HasPrefix(upper, "SHOW") || strings.HasPrefix(upper, "DESCRIBE") {
			// For SELECT/SHOW/DESCRIBE, get the underlying sql.DB from the connection
			// We need to use the panel's existing MySQL connection
			svr, svrErr := h.store.DBStore.GetDefault(c.Request().Context())
			if svrErr != nil || svr == nil {
				return fmt.Errorf("no database server configured")
			}
			pass, passErr := h.store.DBStore.DecryptPassword(svr.PasswordEncrypted)
			if passErr != nil {
				return fmt.Errorf("decrypt failed: %w", passErr)
			}
			db, err := sql.Open("mysql", fmt.Sprintf("%s:%s@tcp(%s:%d)/?charset=utf8mb4&parseTime=true&timeout=5s",
				svr.Username, pass, svr.Host, svr.Port))
			if err != nil {
				return err
			}
			defer db.Close()
			rows, err := db.QueryContext(c.Request().Context(), body.SQL)
			if err != nil {
				return err
			}
			defer rows.Close()
			cols, _ := rows.Columns()
			var lines []string
			lines = append(lines, strings.Join(cols, "\t"))
			for rows.Next() {
				vals := make([]interface{}, len(cols))
				valPtrs := make([]interface{}, len(cols))
				for i := range vals {
					valPtrs[i] = &vals[i]
				}
				if err := rows.Scan(valPtrs...); err != nil {
					continue
				}
				var row []string
				for _, v := range vals {
					if b, ok := v.([]byte); ok {
						row = append(row, string(b))
					} else if v == nil {
						row = append(row, "NULL")
					} else {
						row = append(row, fmt.Sprintf("%v", v))
					}
				}
				lines = append(lines, strings.Join(row, "\t"))
			}
			result = strings.Join(lines, "\n")
		} else {
			res, err := conn.Exec(body.SQL)
			if err != nil {
				return err
			}
			affected, _ := res.RowsAffected()
			result = fmt.Sprintf("OK (%d rows affected)", affected)
		}
		return nil
	}); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      map[string]any{"result": result},
		RequestID: rid,
	})
}
