package agenthttp

import (
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"os"

	"github.com/labstack/echo/v5"
)

// backupDownloadTask answers GET /tasks/backups/download?root_path=&path=:
// a backup file of a site (path relative to the site root, inside backup/ or
// backups/), streamed to the panel. Backups are root-owned (0600) and the
// site's user can rearrange the folders around them, so the file is opened
// through os.Root: a symlink can never lead outside the site's folder.
func (r *Router) backupDownloadTask(c *echo.Context) error {
	root := filepath.Clean(c.QueryParam("root_path"))
	rel := filepath.ToSlash(filepath.Clean(c.QueryParam("path")))
	if !r.isAllowedSitePath(root) {
		return c.JSON(http.StatusBadRequest, map[string]any{"ok": false, "message": "invalid site root"})
	}
	if filepath.IsAbs(rel) || strings.HasPrefix(rel, "../") || rel == ".." ||
		!(strings.HasPrefix(rel, "backup/") || strings.HasPrefix(rel, "backups/")) {
		return c.JSON(http.StatusBadRequest, map[string]any{"ok": false, "message": "not a backup path"})
	}
	dir, err := os.OpenRoot(root)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]any{"ok": false, "message": "site folder not found"})
	}
	defer dir.Close()
	f, err := dir.Open(rel)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]any{"ok": false, "message": "backup file not found on the server"})
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return c.JSON(http.StatusNotFound, map[string]any{"ok": false, "message": "backup file not found on the server"})
	}
	h := c.Response().Header()
	h.Set("Content-Length", strconv.FormatInt(info.Size(), 10))
	h.Set("X-File-Name", filepath.Base(rel))
	return c.Stream(http.StatusOK, "application/octet-stream", f)
}
