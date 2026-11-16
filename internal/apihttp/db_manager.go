package apihttp

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/labstack/echo/v5"

	"sigardapanel/internal/httpjson"
	"sigardapanel/internal/store"
)

// Database manager: structure, row edits, an SQL console and export/import
// for one database. Like the viewer (db_viewer.go) it needs write permission
// on the site and always works through one of the database's own accounts,
// so it can never do more than that account (or phpMyAdmin with it) could.

const maxImportUpload = 30 << 20 // bytes of an uploaded .sql (API body limit is 32 MB)

func (h *dbHandler) auditDB(c *echo.Context, d *store.Database, action string, details map[string]any) {
	var uid int64
	if sess := sessionFromEcho(c); sess != nil {
		uid = sess.UserID
	}
	if details == nil {
		details = map[string]any{}
	}
	details["database"] = d.Name
	h.audit.RecordAction(c.Request().Context(), uid, action, "database", d.ID, details)
}

// manage calls the agent's /tasks/db/manage for the database of the request.
func (h *dbHandler) manage(c *echo.Context, d *store.Database, site *store.Site, user, pass string, extra map[string]any) (map[string]any, error) {
	payload := map[string]any{"type": d.Type, "database": d.Name, "user": user, "password": pass, "system_user": site.SystemUser.String}
	for k, v := range extra {
		payload[k] = v
	}
	sh := &siteHandler{store: h.store}
	code, out, err := sh.siteAgentCall(c.Request().Context(), site, http.MethodPost, "/tasks/db/manage", payload, 70*time.Second)
	if code == http.StatusNotFound && (out == nil || agentMessage(out, "Not Found") == "Not Found") {
		return nil, echo.NewHTTPError(http.StatusBadGateway, "the agent on this server is too old for the database manager; update the agent")
	}
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusBadGateway, "the database's server could not be reached")
	}
	if code >= 400 {
		return nil, agentHTTPError(code, out)
	}
	return out, nil
}

func (h *dbHandler) managerReply(c *echo.Context, d *store.Database, user string, out map[string]any) error {
	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      map[string]any{"database": d.Name, "type": d.Type, "account": user, "result": out["result"]},
		RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
	})
}

// GET /databases/:id/manager/structure?table=&schema=
func (h *dbHandler) managerStructure(c *echo.Context) error {
	d, site, user, pass, err := h.managedDB(c)
	if err != nil {
		return err
	}
	out, err := h.manage(c, d, site, user, pass, map[string]any{"action": "structure", "table": c.QueryParam("table"), "schema": c.QueryParam("schema")})
	if err != nil {
		return err
	}
	return h.managerReply(c, d, user, out)
}

// POST /databases/:id/manager/rows
// body: {table, schema, op: insert|update|delete, keys: [{col: val}], values: {col: val|null}}
func (h *dbHandler) managerRows(c *echo.Context) error {
	d, site, user, pass, err := h.managedDB(c)
	if err != nil {
		return err
	}
	var body struct {
		Table  string               `json:"table"`
		Schema string               `json:"schema"`
		Op     string               `json:"op"`
		Keys   []map[string]*string `json:"keys"`
		Values map[string]*string   `json:"values"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid body")
	}
	switch body.Op {
	case "insert", "update", "delete":
	default:
		return echo.NewHTTPError(http.StatusBadRequest, "op must be insert, update or delete")
	}
	out, err := h.manage(c, d, site, user, pass, map[string]any{
		"action": "rows", "table": body.Table, "schema": body.Schema,
		"op": body.Op, "keys": body.Keys, "values": body.Values,
	})
	if err != nil {
		return err
	}
	affected := 0.0
	if r, ok := out["result"].(map[string]any); ok {
		affected, _ = r["rows_affected"].(float64)
	}
	h.auditDB(c, d, "database_rows", map[string]any{"op": body.Op, "table": body.Table, "schema": body.Schema, "rows": int64(affected)})
	return h.managerReply(c, d, user, out)
}

// POST /databases/:id/manager/sql  body: {sql, read_only}
func (h *dbHandler) managerSQL(c *echo.Context) error {
	d, site, user, pass, err := h.managedDB(c)
	if err != nil {
		return err
	}
	var body struct {
		SQL      string `json:"sql"`
		ReadOnly bool   `json:"read_only"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid body")
	}
	if strings.TrimSpace(body.SQL) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "sql required")
	}
	out, err := h.manage(c, d, site, user, pass, map[string]any{"action": "sql", "sql": body.SQL, "read_only": body.ReadOnly})
	if err != nil {
		// Failed statements are audited too: the console is a powerful tool.
		if !body.ReadOnly {
			h.auditDB(c, d, "database_sql", map[string]any{"sql": auditSQL(body.SQL), "ok": false})
		}
		return err
	}
	if !body.ReadOnly {
		h.auditDB(c, d, "database_sql", map[string]any{"sql": auditSQL(body.SQL), "ok": true})
	}
	return h.managerReply(c, d, user, out)
}

// auditSQL shortens a statement for the audit log. String literals are
// blanked so values (which may be passwords or personal data) stay out of it.
var sqlLiteral = regexp.MustCompile(`'(?:[^'\\]|\\.|'')*'`)

func auditSQL(s string) string {
	s = sqlLiteral.ReplaceAllString(strings.TrimSpace(s), "'…'")
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 500 {
		cut := 500
		for cut > 0 && !utf8.RuneStart(s[cut]) {
			cut--
		}
		s = s[:cut] + "…"
	}
	return s
}

// agentStream opens a streaming request to the agent of the site's server.
func (h *dbHandler) agentStream(c *echo.Context, site *store.Site, path string, body io.Reader, headers map[string]string) (*http.Response, error) {
	fm := &fileManagerHandler{store: h.store, audit: h.audit}
	base, token, err := fm.siteAgent(c.Request().Context(), site)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(c.Request().Context(), http.MethodPost, base+path, body)
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError, "failed to prepare the request")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := agentClient(2 * time.Hour).Do(req)
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusBadGateway, "the database's server could not be reached")
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		var out map[string]any
		_ = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out)
		if out == nil {
			out = map[string]any{}
		}
		if resp.StatusCode == http.StatusNotFound && agentMessage(out, "Not Found") == "Not Found" {
			return nil, echo.NewHTTPError(http.StatusBadGateway, "the agent on this server is too old for the database manager; update the agent")
		}
		return nil, agentHTTPError(resp.StatusCode, out)
	}
	return resp, nil
}

// GET /databases/:id/manager/export — the whole database as a .sql download.
func (h *dbHandler) managerExport(c *echo.Context) error {
	d, site, user, pass, err := h.managedDB(c)
	if err != nil {
		return err
	}
	payload, _ := json.Marshal(map[string]any{"type": d.Type, "database": d.Name, "user": user, "password": pass, "system_user": site.SystemUser.String})
	resp, err := h.agentStream(c, site, "/tasks/db/export", strings.NewReader(string(payload)), map[string]string{"Content-Type": "application/json"})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	h.auditDB(c, d, "database_export", nil)
	name := fmt.Sprintf("%s_%s.sql", d.Name, time.Now().UTC().Format("20060102-150405"))
	c.Response().Header().Set(echo.HeaderContentDisposition, `attachment; filename="`+name+`"`)
	c.Response().Header().Set("Cache-Control", "no-store")
	return c.Stream(http.StatusOK, "application/sql", resp.Body)
}

// POST /databases/:id/manager/import
//   - raw body: the .sql file (at most maxImportUpload), or
//   - ?path=dir/dump.sql: a .sql file already in the site's folder (any size;
//     upload big dumps with the file manager or SFTP first).
func (h *dbHandler) managerImport(c *echo.Context) error {
	d, site, user, pass, err := h.managedDB(c)
	if err != nil {
		return err
	}
	headers := map[string]string{
		"Content-Type":  "application/octet-stream",
		"X-Db-Name":     d.Name,
		"X-Db-Type":     d.Type,
		"X-Db-User":     user,
		"X-Db-Password": pass,
		"X-System-User": site.SystemUser.String,
	}
	var body io.Reader = http.NoBody
	source := "upload"
	if rel := strings.TrimSpace(c.QueryParam("path")); rel != "" {
		if !strings.HasSuffix(strings.ToLower(rel), ".sql") {
			return echo.NewHTTPError(http.StatusBadRequest, "pick a .sql file")
		}
		headers["X-Source-Path"] = rel
		headers["X-Root-Path"] = site.RootPath
		source = rel
	} else {
		if c.Request().ContentLength > maxImportUpload {
			return echo.NewHTTPError(http.StatusRequestEntityTooLarge,
				fmt.Sprintf("the file is larger than %d MB: upload it to the site's folder with the file manager, then import it from there", maxImportUpload>>20))
		}
		body = io.LimitReader(c.Request().Body, maxImportUpload+1)
	}
	resp, err := h.agentStream(c, site, "/tasks/db/import", body, headers)
	if err != nil {
		h.auditDB(c, d, "database_import", map[string]any{"source": source, "ok": false})
		return err
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out)
	h.auditDB(c, d, "database_import", map[string]any{"source": source, "ok": true})
	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      map[string]any{"database": d.Name, "status": "success", "message": "Import finished", "bytes": out["bytes"]},
		RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
	})
}
