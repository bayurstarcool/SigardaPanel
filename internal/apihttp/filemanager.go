package apihttp

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"sigardapanel/internal/sitefs"

	"sigardapanel/internal/audit"
	"sigardapanel/internal/httpjson"
	"sigardapanel/internal/store"

	"github.com/labstack/echo/v4"
)

type fileManagerHandler struct {
	store *store.Store
	audit *audit.Recorder
}

func newFileManagerHandler(s *store.Store, a *audit.Recorder) *fileManagerHandler {
	return &fileManagerHandler{store: s, audit: a}
}

func fileAuditItemType(isDir bool) string {
	if isDir {
		return "folder"
	}
	return "file"
}

type fileEntry struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	IsDir   bool   `json:"is_dir"`
	Mode    string `json:"mode"`
	ModTime string `json:"mod_time"`
}

func (h *fileManagerHandler) resolveSite(c echo.Context) (*store.Site, error) {
	sess := sessionFromEcho(c)
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	site, err := h.store.Sites.GetByID(c.Request().Context(), id)
	if err != nil || site == nil {
		return nil, echo.NewHTTPError(http.StatusNotFound, "site not found")
	}
	siteH := &siteHandler{store: h.store, audit: h.audit}
	if !siteH.canAccess(c.Request().Context(), sess, site) {
		return nil, echo.NewHTTPError(http.StatusNotFound, "site not found")
	}
	return site, nil
}

func (h *fileManagerHandler) resolveSiteWrite(c echo.Context) (*store.Site, error) {
	sess := sessionFromEcho(c)
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	site, err := h.store.Sites.GetByID(c.Request().Context(), id)
	if err != nil || site == nil {
		return nil, echo.NewHTTPError(http.StatusNotFound, "site not found")
	}
	siteH := &siteHandler{store: h.store, audit: h.audit}
	if !siteH.checkSitePermission(c.Request().Context(), sess, site, "write") {
		return nil, echo.NewHTTPError(http.StatusNotFound, "site not found")
	}
	return site, nil
}

// siteAgent returns the agent of the server that hosts site.
func (h *fileManagerHandler) siteAgent(ctx context.Context, site *store.Site) (string, string, error) {
	server, err := h.store.Servers.GetByID(ctx, site.ServerID)
	if err != nil || server == nil || !server.AgentURL.Valid || server.AgentURL.String == "" || server.AgentToken == "" {
		return "", "", echo.NewHTTPError(http.StatusBadGateway, "the site's server has no reachable agent")
	}
	return agentBase(server), server.AgentToken, nil
}

// fileOp runs one file manager operation on the site's server. Files live on
// the server that hosts the site, not on the panel host (the API process is
// sandboxed and cannot see /home there either).
func (h *fileManagerHandler) fileOp(c echo.Context, site *store.Site, op string, args map[string]any) (map[string]any, error) {
	base, token, err := h.siteAgent(c.Request().Context(), site)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{"op": op, "root_path": site.RootPath, "system_user": site.SystemUser.String}
	for k, v := range args {
		payload[k] = v
	}
	code, out, err := stackAgentRequest(c.Request().Context(), http.MethodPost, base+"/tasks/files/op", token, payload, 2*time.Minute)
	if code == http.StatusNotFound && (out == nil || agentMessage(out, "Not Found") == "Not Found") {
		return nil, echo.NewHTTPError(http.StatusBadGateway, "the agent on this server is too old for the file manager; update the agent")
	}
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusBadGateway, "agent unreachable")
	}
	if code >= 400 {
		return nil, agentHTTPError(code, out)
	}
	return out, nil
}

func (h *fileManagerHandler) auditFile(c echo.Context, site *store.Site, details map[string]any) {
	var uid int64
	if sess := sessionFromEcho(c); sess != nil {
		uid = sess.UserID
	}
	details["domain"] = site.Domain
	h.audit.RecordAction(c.Request().Context(), uid, audit.ActionSiteUpdate, audit.ResourceSite, site.ID, details)
}

// GET /sites/:id/files?path=.
func (h *fileManagerHandler) listFiles(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	site, err := h.resolveSite(c)
	if err != nil {
		return err
	}
	relPath := c.QueryParam("path")
	if relPath == "" {
		relPath = "."
	}
	out, err := h.fileOp(c, site, "list", map[string]any{"path": relPath})
	if err != nil {
		return err
	}
	entries, _ := out["entries"].([]any)
	if entries == nil {
		entries = []any{}
	}
	return c.JSON(http.StatusOK, httpjson.Success{
		Data: map[string]any{
			"site_id": site.ID, "domain": site.Domain, "root_path": site.RootPath,
			"current": relPath, "entries": entries, "total": len(entries),
		},
		RequestID: rid,
	})
}

// GET /sites/:id/files/read?path=...
func (h *fileManagerHandler) readFile(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	site, err := h.resolveSite(c)
	if err != nil {
		return err
	}
	relPath := c.QueryParam("path")
	if relPath == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "path required")
	}
	out, err := h.fileOp(c, site, "read", map[string]any{"path": relPath})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, httpjson.Success{
		Data: map[string]any{
			"path": relPath, "content": out["content"], "size": out["size"],
			"mod_time": out["mod_time"], "mode": out["mode"],
		},
		RequestID: rid,
	})
}

// POST /sites/:id/files/write
func (h *fileManagerHandler) writeFile(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	site, err := h.resolveSiteWrite(c)
	if err != nil {
		return err
	}
	var body struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := c.Bind(&body); err != nil || body.Path == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "path required")
	}
	if _, err := h.fileOp(c, site, "write", map[string]any{"path": body.Path, "content": body.Content}); err != nil {
		return err
	}
	h.auditFile(c, site, map[string]any{"action": "file_write", "path": body.Path, "item_type": "file", "is_dir": false})
	return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]any{"status": "success", "message": "File saved"}, RequestID: rid})
}

// POST /sites/:id/files/create
func (h *fileManagerHandler) createEntry(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	site, err := h.resolveSiteWrite(c)
	if err != nil {
		return err
	}
	var body struct {
		Path  string `json:"path"`
		IsDir bool   `json:"is_dir"`
	}
	if err := c.Bind(&body); err != nil || body.Path == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "path required")
	}
	if _, err := h.fileOp(c, site, "create", map[string]any{"path": body.Path, "is_dir": body.IsDir}); err != nil {
		return err
	}
	h.auditFile(c, site, map[string]any{"action": "file_create", "path": body.Path, "item_type": fileAuditItemType(body.IsDir), "is_dir": body.IsDir})
	return c.JSON(http.StatusCreated, httpjson.Success{Data: map[string]any{"status": "success", "message": "Created"}, RequestID: rid})
}

// DELETE /sites/:id/files?path=...
func (h *fileManagerHandler) deleteEntry(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	site, err := h.resolveSiteWrite(c)
	if err != nil {
		return err
	}
	relPath := c.QueryParam("path")
	if relPath == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "path required")
	}
	out, err := h.fileOp(c, site, "delete", map[string]any{"path": relPath})
	if err != nil {
		return err
	}
	isDir, _ := out["is_dir"].(bool)
	h.auditFile(c, site, map[string]any{"action": "file_delete", "path": relPath, "item_type": fileAuditItemType(isDir), "is_dir": isDir})
	return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]any{"status": "success", "message": "Deleted"}, RequestID: rid})
}

// POST /sites/:id/files/rename
func (h *fileManagerHandler) renameEntry(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	site, err := h.resolveSiteWrite(c)
	if err != nil {
		return err
	}
	var body struct {
		OldPath string `json:"old_path"`
		NewName string `json:"new_name"`
	}
	if err := c.Bind(&body); err != nil || body.OldPath == "" || body.NewName == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "old_path and new_name required")
	}
	out, err := h.fileOp(c, site, "rename", map[string]any{"old_path": body.OldPath, "new_name": body.NewName})
	if err != nil {
		return err
	}
	isDir, _ := out["is_dir"].(bool)
	h.auditFile(c, site, map[string]any{"action": "file_rename", "old_path": body.OldPath, "new_name": body.NewName, "item_type": fileAuditItemType(isDir), "is_dir": isDir})
	return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]any{"status": "success", "message": "Renamed"}, RequestID: rid})
}

// POST /sites/:id/files/upload (multipart: path, file)
func (h *fileManagerHandler) uploadFile(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	site, err := h.resolveSiteWrite(c)
	if err != nil {
		return err
	}
	relDir := c.FormValue("path")
	if relDir == "" {
		relDir = "."
	}
	src, err := c.FormFile("file")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "file field required")
	}
	fh, err := src.Open()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to open upload")
	}
	defer fh.Close()
	base, token, err := h.siteAgent(c.Request().Context(), site)
	if err != nil {
		return err
	}
	q := url.Values{"root_path": {site.RootPath}, "system_user": {site.SystemUser.String}, "path": {relDir}, "filename": {src.Filename}}
	req, err := http.NewRequestWithContext(c.Request().Context(), http.MethodPost, base+"/tasks/files/upload?"+q.Encode(), fh)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to prepare upload")
	}
	req.ContentLength = src.Size
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := agentClient(10 * time.Minute).Do(req)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "agent unreachable")
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out)
	if resp.StatusCode == http.StatusNotFound && agentMessage(out, "Not Found") == "Not Found" {
		return echo.NewHTTPError(http.StatusBadGateway, "the agent on this server is too old for the file manager; update the agent")
	}
	if resp.StatusCode >= 400 {
		if out == nil {
			out = map[string]any{}
		}
		return agentHTTPError(resp.StatusCode, out)
	}
	h.auditFile(c, site, map[string]any{"action": "file_upload", "path": out["path"], "size": out["size"], "item_type": "file", "is_dir": false})
	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      map[string]any{"status": "success", "message": "Uploaded", "name": out["name"], "size": out["size"], "path": out["path"]},
		RequestID: rid,
	})
}

// GET /sites/:id/files/download?path=...
func (h *fileManagerHandler) downloadFile(c echo.Context) error {
	site, err := h.resolveSite(c)
	if err != nil {
		return err
	}
	relPath := c.QueryParam("path")
	if relPath == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "path required")
	}
	base, token, err := h.siteAgent(c.Request().Context(), site)
	if err != nil {
		return err
	}
	q := url.Values{"root_path": {site.RootPath}, "path": {relPath}}
	req, err := http.NewRequestWithContext(c.Request().Context(), http.MethodGet, base+"/tasks/files/download?"+q.Encode(), nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to prepare download")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := agentClient(30 * time.Minute).Do(req)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "agent unreachable")
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		var out map[string]any
		_ = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out)
		if out == nil {
			out = map[string]any{}
		}
		if resp.StatusCode == http.StatusNotFound && agentMessage(out, "Not Found") == "Not Found" {
			return echo.NewHTTPError(http.StatusBadGateway, "the agent on this server is too old for the file manager; update the agent")
		}
		return agentHTTPError(resp.StatusCode, out)
	}
	name := filepath.Base(relPath)
	c.Response().Header().Set(echo.HeaderContentDisposition, "attachment; filename=\""+strings.ReplaceAll(name, "\"", "")+"\"")
	if cl := resp.Header.Get("Content-Length"); cl != "" {
		c.Response().Header().Set("Content-Length", cl)
	}
	return c.Stream(http.StatusOK, "application/octet-stream", resp.Body)
}

// POST /sites/:id/files/move  body: { "paths": ["a/b.txt", "c"], "dest": "folder" }
func (h *fileManagerHandler) moveEntries(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	site, err := h.resolveSiteWrite(c)
	if err != nil {
		return err
	}
	var body struct {
		Paths []string `json:"paths"`
		Dest  string   `json:"dest"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid body")
	}
	if len(body.Paths) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "paths required")
	}
	out, err := h.fileOp(c, site, "move", map[string]any{"paths": body.Paths, "dest": body.Dest})
	if err != nil {
		return err
	}
	moved, _ := out["moved"].([]any)
	if moved == nil {
		moved = []any{}
	}
	h.auditFile(c, site, map[string]any{"action": "file_move", "count": len(moved), "dest": body.Dest})
	return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]any{"status": "success", "message": "Moved", "moved": moved}, RequestID: rid})
}

// POST /sites/:id/files/compress  body: { "paths": ["a", "b.txt"], "name": "archive.zip", "dest": "folder" }
func (h *fileManagerHandler) compressEntries(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	site, err := h.resolveSiteWrite(c)
	if err != nil {
		return err
	}
	var body struct {
		Paths []string `json:"paths"`
		Name  string   `json:"name"`
		Dest  string   `json:"dest"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid body")
	}
	if len(body.Paths) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "paths required")
	}
	out, err := h.fileOp(c, site, "compress", map[string]any{"paths": body.Paths, "name": body.Name, "dest": body.Dest})
	if err != nil {
		return err
	}
	h.auditFile(c, site, map[string]any{"action": "file_compress", "count": len(body.Paths), "archive": filepath.Join(body.Dest, fmt.Sprint(out["archive"])), "size": out["size"]})
	return c.JSON(http.StatusOK, httpjson.Success{Data: map[string]any{"status": "success", "message": "Compressed", "archive": out["archive"], "size": out["size"]}, RequestID: rid})
}

// addToZip is kept for callers in this package (see sitefs.AddToZip).
func addToZip(zw *zip.Writer, src, baseName string) error { return sitefs.AddToZip(zw, src, baseName) }
