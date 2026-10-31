package apihttp

import (
	"context"
	"log/slog"
	"math"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"sigardapanel/internal/audit"
	"sigardapanel/internal/dbserver"
	"sigardapanel/internal/httpjson"
	"sigardapanel/internal/store"
)

type dbServerHandler struct {
	store *store.Store
	audit *audit.Recorder
}

func newDBServerHandler(s *store.Store, a *audit.Recorder) *dbServerHandler {
	return &dbServerHandler{store: s, audit: a}
}

// getDBServer returns the default database server info (GET /api/v1/database-server).
func (h *dbServerHandler) getDBServer(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)

	svr, err := h.store.DBStore.GetDefault(c.Request().Context())
	if err != nil {
		slog.Error("api: request failed", "route", c.Path(), "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if svr == nil {
		return c.JSON(http.StatusOK, httpjson.Success{
			Data:      map[string]any{"server": nil},
			RequestID: rid,
		})
	}

	decPass, _ := h.store.DBStore.DecryptPassword(svr.PasswordEncrypted)

	// Test connection
	conn, err := dbserver.Connect(svr, decPass)
	status := "disconnected"
	if err == nil {
		status = "connected"
		conn.Close()
	}

	return c.JSON(http.StatusOK, httpjson.Success{
		Data: map[string]any{
			"server": map[string]any{
				"id":         svr.ID,
				"name":       svr.Name,
				"engine":     svr.Engine,
				"host":       svr.Host,
				"port":       svr.Port,
				"username":   svr.Username,
				"is_active":  svr.IsActive,
				"is_default": svr.IsDefault,
				"status":     status,
			},
		},
		RequestID: rid,
	})
}

// importDatabases imports existing MySQL databases into SigardaPanel metadata.
func (h *dbServerHandler) importDatabases(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)

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

	ctx, cancel := context.WithTimeout(c.Request().Context(), 30*time.Second)
	defer cancel()

	// List MySQL databases
	mysqlDBs, err := conn.ListDatabases(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "list databases failed: "+err.Error())
	}

	// List MySQL users
	users, err := conn.ListUsers(ctx)
	if err != nil {
		slog.Warn("import: list users failed", "error", err)
	}

	// Build user-to-database mapping
	userDBMap := make(map[string][]dbserver.DBUserInfo)
	for _, u := range users {
		userDBMap[u.Username] = append(userDBMap[u.Username], u)
	}

	imported := 0
	skipped := 0
	importedUsers := 0
	// Databases that match no site are not imported: databases.site_id is
	// NOT NULL with a foreign key to sites, so they are reported instead.
	unmatched := []string{}

	// Index every site by domain and name once (no page limit) for matching.
	allSites, err := h.store.Sites.List(ctx, store.SiteListFilter{Limit: math.MaxInt32})
	if err != nil {
		slog.Error("api: request failed", "route", c.Path(), "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	siteByName := make(map[string]int64, len(allSites))
	for _, st := range allSites {
		if _, dup := siteByName[st.Name]; !dup {
			siteByName[st.Name] = st.ID
		}
	}

	for _, dbName := range mysqlDBs {
		// Check if already exists in SigardaPanel
		existing, _ := h.store.Databases.GetByName(ctx, dbName)
		var dbID int64
		if existing != nil {
			dbID = existing.ID
			skipped++
		} else {
			// Try to find site by domain matching database name, then by
			// site name.
			var siteID int64
			if site, _ := h.store.Sites.GetByDomain(ctx, dbName); site != nil {
				siteID = site.ID
			} else if id, ok := siteByName[dbName]; ok {
				siteID = id
			}
			if siteID == 0 {
				unmatched = append(unmatched, dbName)
				continue
			}

			db := &store.Database{
				SiteID:    siteID,
				Name:      dbName,
				Type:      "mysql",
				Charset:   "utf8mb4",
				Collation: "utf8mb4_unicode_ci",
			}

			if err := h.store.Databases.Create(ctx, db); err != nil {
				slog.Warn("import: create database record failed", "name", dbName, "error", err)
				continue
			}
			dbID = db.ID
			imported++
		}

		// Import users for this database (also for existing databases)
		for _, u := range userDBMap {
			for _, ui := range u {
				// Only import if username starts with or matches db name
				if ui.Username == dbName || stringsHasPrefix(ui.Username, dbName) {
					// Check if user already imported
					existingUsers, _ := h.store.DBUsers.ListByDatabase(ctx, dbID)
					alreadyImported := false
					for _, eu := range existingUsers {
						if eu.Username == ui.Username && eu.HostPattern == ui.Host {
							alreadyImported = true
							break
						}
					}
					if alreadyImported {
						continue
					}

					// Store user reference (we can't get the password since it's hashed in MySQL)
					dbUser := &store.DBUser{
						DatabaseID:   dbID,
						Username:     ui.Username,
						PasswordHash: "imported",
						HostPattern:  ui.Host,
						Privileges:   "ALL",
					}
					if err := h.store.DBUsers.Create(ctx, dbUser); err != nil {
						slog.Warn("import: create user record failed", "user", ui.Username, "error", err)
						continue
					}
					importedUsers++
				}
			}
		}
	}

	var userID int64
	if sess != nil {
		userID = sess.UserID
	}
	h.audit.RecordAction(ctx, userID, "database_import", "database_server", svr.ID, map[string]any{
		"imported": imported, "skipped": skipped, "unmatched": len(unmatched), "users": len(users), "imported_users": importedUsers,
	})

	return c.JSON(http.StatusOK, httpjson.Success{
		Data: map[string]any{
			"imported":       imported,
			"skipped":        skipped,
			"total":          len(mysqlDBs),
			"users":          len(users),
			"imported_users": importedUsers,
			"unmatched":      unmatched,
		},
		RequestID: rid,
	})
}

func stringsHasPrefix(s, prefix string) bool {
	if len(s) < len(prefix) {
		return false
	}
	return s[:len(prefix)] == prefix
}
