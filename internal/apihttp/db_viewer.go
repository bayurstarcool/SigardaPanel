package apihttp

import (
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/labstack/echo/v4"

	"sigardapanel/internal/httpjson"
	"sigardapanel/internal/store"
)

// Database viewer: tables, columns, rows and read-only queries for one
// database, run by the agent of the site's server with one of the
// database's own accounts (see agenthttp/dbviewer.go). Viewing data needs
// write permission on the site, like opening phpMyAdmin.

// viewerAccount picks the database account the viewer connects with: one
// with ALL privileges if possible, else any account with a stored password.
func viewerAccount(h *dbHandler, c echo.Context, d *store.Database) (string, string) {
	bh := &backupHandler{store: h.store}
	return bh.restoreAccount(c.Request().Context(), *d)
}

func (h *dbHandler) viewer(c echo.Context, action string, extra map[string]any) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)
	if sess == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "not authenticated")
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid database id")
	}
	ctx := c.Request().Context()
	d, err := h.store.Databases.GetByID(ctx, id)
	if err != nil || d == nil {
		return echo.NewHTTPError(http.StatusNotFound, "database not found")
	}
	site, _ := h.store.Sites.GetByID(ctx, d.SiteID)
	if site == nil {
		return echo.NewHTTPError(http.StatusNotFound, "database not found")
	}
	if !h.canManage(ctx, sess, site) {
		return h.denyDB(ctx, sess, site, "database not found")
	}
	user, pass := viewerAccount(h, c, d)
	if user == "" {
		return echo.NewHTTPError(http.StatusConflict,
			"this database has no user with a password stored in the panel: create a database user, or reset an existing user's password, then open the viewer again")
	}
	payload := map[string]any{"type": d.Type, "database": d.Name, "user": user, "password": pass, "action": action}
	for k, v := range extra {
		payload[k] = v
	}
	sh := &siteHandler{store: h.store}
	code, out, err := sh.siteAgentCall(ctx, site, http.MethodPost, "/tasks/db/view", payload, 30*time.Second)
	if code == http.StatusNotFound && (out == nil || agentMessage(out, "Not Found") == "Not Found") {
		return echo.NewHTTPError(http.StatusBadGateway, "the agent on this server is too old for the database viewer; update the agent")
	}
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "the database's server could not be reached")
	}
	if code >= 400 {
		return agentHTTPError(code, out)
	}
	if action == "query" {
		q, _ := extra["sql"].(string)
		if utf8.RuneCountInString(q) > 500 {
			q = string([]rune(q)[:500]) + "…"
		}
		h.audit.RecordAction(ctx, sess.UserID, "database_query", "database", d.ID, map[string]any{"database": d.Name, "sql": q})
	}
	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      map[string]any{"database": d.Name, "type": d.Type, "account": user, "site_id": site.ID, "domain": site.Domain, "result": out["result"]},
		RequestID: rid,
	})
}

// GET /databases/:id/viewer/tables
func (h *dbHandler) viewerTables(c echo.Context) error { return h.viewer(c, "tables", nil) }

// GET /databases/:id/viewer/columns?table=&schema=
func (h *dbHandler) viewerColumns(c echo.Context) error {
	return h.viewer(c, "columns", map[string]any{"table": c.QueryParam("table"), "schema": c.QueryParam("schema")})
}

// GET /databases/:id/viewer/rows?table=&schema=&limit=&offset=&order_by=&desc=1
func (h *dbHandler) viewerRows(c echo.Context) error {
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	offset, _ := strconv.Atoi(c.QueryParam("offset"))
	return h.viewer(c, "rows", map[string]any{
		"table": c.QueryParam("table"), "schema": c.QueryParam("schema"),
		"limit": limit, "offset": offset, "order_by": c.QueryParam("order_by"),
		"desc": c.QueryParam("desc") == "1" || strings.EqualFold(c.QueryParam("desc"), "true"),
	})
}

// POST /databases/:id/viewer/query {sql, limit}
func (h *dbHandler) viewerQuery(c echo.Context) error {
	var body struct {
		SQL   string `json:"sql"`
		Limit int    `json:"limit"`
	}
	if err := c.Bind(&body); err != nil || strings.TrimSpace(body.SQL) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "sql required")
	}
	return h.viewer(c, "query", map[string]any{"sql": body.SQL, "limit": body.Limit})
}
