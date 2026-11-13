package agenthttp

import (
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/labstack/echo/v4"
	"sigardapanel/internal/sitefs"
)

// maxUploadBytes bounds one file manager upload (the API body limit is 32M).
const maxUploadBytes = 32 << 20

// siteFS returns the file manager for a site root on this server, owned by
// the site's system user (or, without one, by the owner of the root).
func (r *Router) siteFS(rootPath, systemUser string) (*sitefs.FS, error) {
	if !r.isAllowedSitePath(rootPath) {
		return nil, &sitefs.Error{Status: http.StatusBadRequest, Msg: "invalid site root"}
	}
	f := &sitefs.FS{Root: filepath.Clean(rootPath)}
	if uid, gid, ok := siteUserIDs(systemUser); ok && uid <= math.MaxInt32 && gid <= math.MaxInt32 {
		f.UID, f.GID = int(uid), int(gid)
	} else if fi, err := os.Stat(f.Root); err == nil {
		if st, ok := fi.Sys().(*syscall.Stat_t); ok && st.Uid != 0 {
			f.UID, f.GID = int(st.Uid), int(st.Gid)
		}
	}
	return f, nil
}

func fsError(c echo.Context, err error) error {
	return c.JSON(sitefs.StatusOf(err), map[string]any{"ok": false, "message": err.Error()})
}

// filesTask answers POST /tasks/files/op: one file manager operation.
func (r *Router) filesTask(c echo.Context) error {
	var p struct {
		Op         string   `json:"op"`
		RootPath   string   `json:"root_path"`
		SystemUser string   `json:"system_user"`
		Path       string   `json:"path"`
		Content    *string  `json:"content"`
		IsDir      bool     `json:"is_dir"`
		OldPath    string   `json:"old_path"`
		NewName    string   `json:"new_name"`
		Paths      []string `json:"paths"`
		Dest       string   `json:"dest"`
		Name       string   `json:"name"`
		Mode       string   `json:"mode"`
	}
	if err := c.Bind(&p); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"ok": false, "message": "invalid payload"})
	}
	f, err := r.siteFS(p.RootPath, p.SystemUser)
	if err != nil {
		return fsError(c, err)
	}
	switch p.Op {
	case "list":
		entries, err := f.List(p.Path)
		if err != nil {
			return fsError(c, err)
		}
		return c.JSON(http.StatusOK, map[string]any{"ok": true, "entries": entries})
	case "read":
		data, info, err := f.Read(p.Path)
		if err != nil {
			return fsError(c, err)
		}
		out := map[string]any{
			"ok": true, "size": info.Size(),
			"mod_time": info.ModTime().Format(time.RFC3339), "mode": info.Mode().String(),
		}
		// Binary or non-UTF-8 content would be mangled by JSON and the
		// editor (and saving it would corrupt the file): download only.
		if sitefs.IsText(data) {
			out["content"] = string(data)
		} else {
			out["binary"], out["content"] = true, ""
		}
		return c.JSON(http.StatusOK, out)
	case "write":
		if p.Content == nil {
			return c.JSON(http.StatusBadRequest, map[string]any{"ok": false, "message": "content required"})
		}
		if err := f.Write(p.Path, []byte(*p.Content)); err != nil {
			return fsError(c, err)
		}
		return c.JSON(http.StatusOK, map[string]any{"ok": true})
	case "create":
		if err := f.Create(p.Path, p.IsDir); err != nil {
			return fsError(c, err)
		}
		return c.JSON(http.StatusOK, map[string]any{"ok": true})
	case "delete":
		isDir, err := f.Delete(p.Path)
		if err != nil {
			return fsError(c, err)
		}
		return c.JSON(http.StatusOK, map[string]any{"ok": true, "is_dir": isDir})
	case "rename":
		isDir, err := f.Rename(p.OldPath, p.NewName)
		if err != nil {
			return fsError(c, err)
		}
		return c.JSON(http.StatusOK, map[string]any{"ok": true, "is_dir": isDir})
	case "move":
		moved, err := f.Move(p.Paths, p.Dest)
		if err != nil {
			return fsError(c, err)
		}
		return c.JSON(http.StatusOK, map[string]any{"ok": true, "moved": moved})
	case "compress":
		name, size, err := f.Compress(p.Paths, p.Name, p.Dest)
		if err != nil {
			return fsError(c, err)
		}
		return c.JSON(http.StatusOK, map[string]any{"ok": true, "archive": name, "size": size})
	case "extract":
		written, skipped, err := f.Extract(p.Path, p.Dest)
		if err != nil {
			return fsError(c, err)
		}
		return c.JSON(http.StatusOK, map[string]any{"ok": true, "extracted": written, "skipped": skipped})
	case "chmod":
		if err := f.Chmod(p.Path, p.Mode); err != nil {
			return fsError(c, err)
		}
		return c.JSON(http.StatusOK, map[string]any{"ok": true})
	default:
		return c.JSON(http.StatusBadRequest, map[string]any{"ok": false, "message": "unknown operation"})
	}
}

// filesUploadTask answers POST /tasks/files/upload?root_path=&system_user=&path=&filename=
// with the file as the raw request body.
func (r *Router) filesUploadTask(c echo.Context) error {
	f, err := r.siteFS(c.QueryParam("root_path"), c.QueryParam("system_user"))
	if err != nil {
		return fsError(c, err)
	}
	rel, size, err := f.SaveUpload(c.QueryParam("path"), c.QueryParam("filename"), c.Request().Body, maxUploadBytes)
	if err != nil {
		return fsError(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{"ok": true, "path": rel, "name": filepath.Base(rel), "size": size})
}

// filesDownloadTask answers GET /tasks/files/download?root_path=&path= with the file.
func (r *Router) filesDownloadTask(c echo.Context) error {
	f, err := r.siteFS(c.QueryParam("root_path"), "")
	if err != nil {
		return fsError(c, err)
	}
	fh, info, err := f.OpenDownload(c.QueryParam("path"))
	if err != nil {
		return fsError(c, err)
	}
	defer fh.Close()
	h := c.Response().Header()
	h.Set("Content-Length", strconv.FormatInt(info.Size(), 10))
	h.Set("X-File-Name", filepath.Base(info.Name()))
	return c.Stream(http.StatusOK, "application/octet-stream", fh)
}
