package apihttp

import (
	"archive/zip"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

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

func safePath(root, rel string) (string, error) {
	if root == "" {
		return "", echo.NewHTTPError(http.StatusBadRequest, "site root path not configured")
	}
	rel = strings.TrimSpace(rel)
	if rel == "" {
		rel = "."
	}
	rel = filepath.Clean(rel)
	if rel == ".." || strings.HasPrefix(rel, "..") || strings.HasPrefix(rel, "/") {
		return "", echo.NewHTTPError(http.StatusBadRequest, "invalid path")
	}
	full := filepath.Join(root, rel)
	// Ensure full path is within root (use trailing separator to avoid prefix collisions
	// e.g., /home/user/htdocs/example.com should NOT match /home/user/htdocs/example.com-backup)
	cleanRoot := filepath.Clean(root)
	if full != cleanRoot && !strings.HasPrefix(full, cleanRoot+string(filepath.Separator)) {
		return "", echo.NewHTTPError(http.StatusBadRequest, "path traversal not allowed")
	}
	// Resolve symlinks so a link inside the site root cannot point outside it.
	if err := ensureNoSymlinkEscape(cleanRoot, full); err != nil {
		return "", err
	}
	return full, nil
}

// ensureNoSymlinkEscape resolves symlinks on the deepest existing ancestor of
// full and verifies the result stays inside the (resolved) root.
func ensureNoSymlinkEscape(root, full string) error {
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "site root not accessible")
	}
	probe := full
	for {
		resolved, err := filepath.EvalSymlinks(probe)
		if err == nil {
			if resolved != realRoot && !strings.HasPrefix(resolved, realRoot+string(filepath.Separator)) {
				return echo.NewHTTPError(http.StatusBadRequest, "path traversal not allowed")
			}
			return nil
		}
		if !os.IsNotExist(err) {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid path")
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid path")
		}
		probe = parent
	}
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
	fullPath, err := safePath(site.RootPath, relPath)
	if err != nil {
		return err
	}

	info, err := os.Stat(fullPath)
	if err != nil || !info.IsDir() {
		return echo.NewHTTPError(http.StatusNotFound, "directory not found")
	}

	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to read directory")
	}

	files := make([]fileEntry, 0, len(entries))
	for _, e := range entries {
		eInfo, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, fileEntry{
			Name:    e.Name(),
			Path:    filepath.Join(relPath, e.Name()),
			Size:    eInfo.Size(),
			IsDir:   e.IsDir(),
			Mode:    eInfo.Mode().String(),
			ModTime: eInfo.ModTime().Format(time.RFC3339),
		})
	}

	return c.JSON(http.StatusOK, httpjson.Success{
		Data: map[string]any{
			"site_id":   site.ID,
			"domain":    site.Domain,
			"root_path": site.RootPath,
			"current":   relPath,
			"entries":   files,
			"total":     len(files),
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
	fullPath, err := safePath(site.RootPath, relPath)
	if err != nil {
		return err
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "file not found")
	}
	if info.Size() > 1024*1024 {
		return echo.NewHTTPError(http.StatusBadRequest, "file too large (>1MB)")
	}
	if info.IsDir() {
		return echo.NewHTTPError(http.StatusBadRequest, "is a directory")
	}

	data, err := os.ReadFile(fullPath)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to read file")
	}

	return c.JSON(http.StatusOK, httpjson.Success{
		Data: map[string]any{
			"path":     relPath,
			"content":  string(data),
			"size":     info.Size(),
			"mod_time": info.ModTime().Format(time.RFC3339),
			"mode":     info.Mode().String(),
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

	fullPath, err := safePath(site.RootPath, body.Path)
	if err != nil {
		return err
	}

	dir := filepath.Dir(fullPath)
	os.MkdirAll(dir, 0755)

	if err := os.WriteFile(fullPath, []byte(body.Content), 0644); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to write")
	}

	sess := sessionFromEcho(c)
	var userID int64
	if sess != nil {
		userID = sess.UserID
	}
	h.audit.RecordAction(c.Request().Context(), userID, audit.ActionSiteUpdate, audit.ResourceSite, site.ID, map[string]any{
		"domain": site.Domain, "action": "file_write", "path": body.Path, "item_type": "file", "is_dir": false,
	})

	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      map[string]any{"status": "success", "message": "File saved"},
		RequestID: rid,
	})
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

	fullPath, err := safePath(site.RootPath, body.Path)
	if err != nil {
		return err
	}

	if body.IsDir {
		os.MkdirAll(fullPath, 0755)
	} else {
		os.MkdirAll(filepath.Dir(fullPath), 0755)
		os.WriteFile(fullPath, []byte(""), 0644)
	}

	sess := sessionFromEcho(c)
	var userID int64
	if sess != nil {
		userID = sess.UserID
	}
	h.audit.RecordAction(c.Request().Context(), userID, audit.ActionSiteUpdate, audit.ResourceSite, site.ID, map[string]any{
		"domain": site.Domain, "action": "file_create", "path": body.Path, "item_type": fileAuditItemType(body.IsDir), "is_dir": body.IsDir,
	})

	return c.JSON(http.StatusCreated, httpjson.Success{
		Data:      map[string]any{"status": "success", "message": "Created"},
		RequestID: rid,
	})
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
	fullPath, err := safePath(site.RootPath, relPath)
	if err != nil {
		return err
	}

	if fullPath == filepath.Clean(site.RootPath) {
		return echo.NewHTTPError(http.StatusBadRequest, "cannot delete site root")
	}
	isDir := false
	if info, err := os.Stat(fullPath); err == nil {
		isDir = info.IsDir()
	}

	os.RemoveAll(fullPath)

	sess := sessionFromEcho(c)
	var userID int64
	if sess != nil {
		userID = sess.UserID
	}
	h.audit.RecordAction(c.Request().Context(), userID, audit.ActionSiteUpdate, audit.ResourceSite, site.ID, map[string]any{
		"domain": site.Domain, "action": "file_delete", "path": relPath, "item_type": fileAuditItemType(isDir), "is_dir": isDir,
	})

	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      map[string]any{"status": "success", "message": "Deleted"},
		RequestID: rid,
	})
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

	oldFull, err := safePath(site.RootPath, body.OldPath)
	if err != nil {
		return err
	}

	// Build new path: parent of old path + new name
	parentDir := filepath.Dir(oldFull)
	newFull := filepath.Join(parentDir, body.NewName)
	newRel := strings.TrimPrefix(newFull, filepath.Clean(site.RootPath)+string(filepath.Separator))

	// Validate new path is still within root
	if _, err := safePath(site.RootPath, newRel); err != nil {
		return err
	}
	isDir := false
	if info, err := os.Stat(oldFull); err == nil {
		isDir = info.IsDir()
	}

	if err := os.Rename(oldFull, newFull); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to rename: "+err.Error())
	}

	sess := sessionFromEcho(c)
	var userID int64
	if sess != nil {
		userID = sess.UserID
	}
	h.audit.RecordAction(c.Request().Context(), userID, audit.ActionSiteUpdate, audit.ResourceSite, site.ID, map[string]any{
		"domain": site.Domain, "action": "file_rename", "old_path": body.OldPath, "new_name": body.NewName, "item_type": fileAuditItemType(isDir), "is_dir": isDir,
	})

	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      map[string]any{"status": "success", "message": "Renamed"},
		RequestID: rid,
	})
}

// POST /sites/:id/files/upload
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
	if _, err := safePath(site.RootPath, relDir); err != nil {
		return err
	}

	src, err := c.FormFile("file")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "file field required")
	}

	// The multipart filename is client-controlled; strip any directory
	// components (ToSlash first — backslashes are not separators on Linux)
	// and re-validate the final path so it can never escape the site root.
	filename := filepath.Base(filepath.ToSlash(src.Filename))
	if filename == "" || filename == "." || filename == ".." || filename == "/" {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid filename")
	}
	dstPath, err := safePath(site.RootPath, filepath.Join(relDir, filename))
	if err != nil {
		return err
	}
	os.MkdirAll(filepath.Dir(dstPath), 0755)

	// Open uploaded file
	srcFile, err := src.Open()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to open upload")
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dstPath)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create file")
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to save file")
	}

	info, _ := os.Stat(dstPath)

	sess := sessionFromEcho(c)
	var userID int64
	if sess != nil {
		userID = sess.UserID
	}
	h.audit.RecordAction(c.Request().Context(), userID, audit.ActionSiteUpdate, audit.ResourceSite, site.ID, map[string]any{
		"domain": site.Domain, "action": "file_upload", "path": filepath.Join(relDir, filename), "size": info.Size(), "item_type": "file", "is_dir": false,
	})

	return c.JSON(http.StatusOK, httpjson.Success{
		Data: map[string]any{
			"status":  "success",
			"message": "Uploaded",
			"name":    filename,
			"size":    info.Size(),
			"path":    filepath.Join(relDir, filename),
		},
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
	fullPath, err := safePath(site.RootPath, relPath)
	if err != nil {
		return err
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "file not found")
	}
	if info.IsDir() {
		return echo.NewHTTPError(http.StatusBadRequest, "cannot download a directory")
	}

	c.Response().Header().Set(echo.HeaderContentDisposition, "attachment; filename=\""+filepath.Base(fullPath)+"\"")
	return c.File(fullPath)
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

	// Resolve destination directory (empty/"." = site root)
	destFull, err := safePath(site.RootPath, body.Dest)
	if err != nil {
		return err
	}
	if info, err := os.Stat(destFull); err != nil || !info.IsDir() {
		return echo.NewHTTPError(http.StatusBadRequest, "destination is not a directory")
	}

	moved := make([]string, 0, len(body.Paths))
	for _, rel := range body.Paths {
		srcFull, err := safePath(site.RootPath, rel)
		if err != nil {
			return err
		}
		if srcFull == filepath.Clean(site.RootPath) {
			return echo.NewHTTPError(http.StatusBadRequest, "cannot move site root")
		}
		base := filepath.Base(srcFull)
		dstFull := filepath.Join(destFull, base)
		// Prevent moving a folder into itself or its own subtree
		if srcFull == destFull || strings.HasPrefix(destFull+string(filepath.Separator), srcFull+string(filepath.Separator)) {
			return echo.NewHTTPError(http.StatusBadRequest, "cannot move a folder into itself")
		}
		if dstFull == srcFull {
			continue // already there
		}
		if _, err := os.Stat(dstFull); err == nil {
			return echo.NewHTTPError(http.StatusConflict, "already exists at destination: "+base)
		}
		if err := os.Rename(srcFull, dstFull); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to move: "+base)
		}
		moved = append(moved, base)
	}

	sess := sessionFromEcho(c)
	var userID int64
	if sess != nil {
		userID = sess.UserID
	}
	h.audit.RecordAction(c.Request().Context(), userID, audit.ActionSiteUpdate, audit.ResourceSite, site.ID, map[string]any{
		"domain": site.Domain, "action": "file_move", "count": len(moved), "dest": body.Dest,
	})

	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      map[string]any{"status": "success", "message": "Moved", "moved": moved},
		RequestID: rid,
	})
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

	// Sanitize archive name
	name := strings.TrimSpace(body.Name)
	if name == "" {
		name = "archive.zip"
	}
	name = filepath.Base(name) // strip any directory components
	if !strings.HasSuffix(strings.ToLower(name), ".zip") {
		name += ".zip"
	}

	destFull, err := safePath(site.RootPath, body.Dest)
	if err != nil {
		return err
	}
	if info, err := os.Stat(destFull); err != nil || !info.IsDir() {
		return echo.NewHTTPError(http.StatusBadRequest, "destination is not a directory")
	}
	archivePath := filepath.Join(destFull, name)
	if _, err := os.Stat(archivePath); err == nil {
		return echo.NewHTTPError(http.StatusConflict, "archive already exists: "+name)
	}

	// Pre-resolve and validate all source paths before creating the archive file
	type srcItem struct {
		full string
		base string
	}
	items := make([]srcItem, 0, len(body.Paths))
	for _, rel := range body.Paths {
		full, err := safePath(site.RootPath, rel)
		if err != nil {
			return err
		}
		if _, err := os.Stat(full); err != nil {
			return echo.NewHTTPError(http.StatusNotFound, "not found: "+rel)
		}
		items = append(items, srcItem{full: full, base: filepath.Base(full)})
	}

	out, err := os.Create(archivePath)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create archive")
	}
	defer out.Close()

	zw := zip.NewWriter(out)
	for _, it := range items {
		if err := addToZip(zw, it.full, it.base); err != nil {
			zw.Close()
			os.Remove(archivePath)
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to compress: "+it.base)
		}
	}
	if err := zw.Close(); err != nil {
		os.Remove(archivePath)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to finalize archive")
	}

	info, _ := os.Stat(archivePath)
	var size int64
	if info != nil {
		size = info.Size()
	}

	sess := sessionFromEcho(c)
	var userID int64
	if sess != nil {
		userID = sess.UserID
	}
	h.audit.RecordAction(c.Request().Context(), userID, audit.ActionSiteUpdate, audit.ResourceSite, site.ID, map[string]any{
		"domain": site.Domain, "action": "file_compress", "count": len(items), "archive": filepath.Join(body.Dest, name), "size": size,
	})

	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      map[string]any{"status": "success", "message": "Compressed", "archive": name, "size": size},
		RequestID: rid,
	})
}

// addToZip walks src (file or dir) and writes entries under baseName inside the zip.
func addToZip(zw *zip.Writer, src, baseName string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return writeFileToZip(zw, src, baseName, info)
	}
	return filepath.Walk(src, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		zipName := baseName
		if rel != "." {
			zipName = filepath.ToSlash(filepath.Join(baseName, rel))
		}
		if fi.IsDir() {
			if rel == "." {
				return nil
			}
			_, err := zw.Create(zipName + "/")
			return err
		}
		return writeFileToZip(zw, path, zipName, fi)
	})
}

func writeFileToZip(zw *zip.Writer, path, zipName string, fi os.FileInfo) error {
	hdr, err := zip.FileInfoHeader(fi)
	if err != nil {
		return err
	}
	hdr.Name = filepath.ToSlash(zipName)
	hdr.Method = zip.Deflate
	w, err := zw.CreateHeader(hdr)
	if err != nil {
		return err
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(w, f)
	return err
}
