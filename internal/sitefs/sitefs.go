// Package sitefs implements the file manager operations on one site root.
// The agent of the server that hosts the site runs them (as root), so every
// path is confined to the root (no "..", no symlink escapes) and everything
// it creates is handed to the site owner.
package sitefs

import (
	"archive/zip"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// MaxReadSize is the largest file Read returns (the editor is for text).
const MaxReadSize = 1 << 20

// Error carries the HTTP status an operation failed with.
type Error struct {
	Status int
	Msg    string
}

func (e *Error) Error() string { return e.Msg }

func fail(status int, msg string) error {
	return &Error{Status: status, Msg: msg}
}

// StatusOf returns the HTTP status for err (500 for unexpected errors).
func StatusOf(err error) int {
	var e *Error
	if errors.As(err, &e) {
		return e.Status
	}
	return http.StatusInternalServerError
}

// FS is one site root. UID/GID own what it creates (0 = leave as is).
type FS struct {
	Root     string
	UID, GID int
}

// Entry is one directory entry.
type Entry struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	IsDir   bool   `json:"is_dir"`
	Mode    string `json:"mode"`
	ModTime string `json:"mod_time"`
}

// Resolve turns a path relative to the root into an absolute one, refusing
// anything that leaves the root, also through symlinks.
func (f *FS) Resolve(rel string) (string, error) {
	if f.Root == "" {
		return "", fail(http.StatusBadRequest, "site root path not configured")
	}
	rel = strings.TrimSpace(rel)
	if rel == "" {
		rel = "."
	}
	rel = filepath.Clean(rel)
	if rel == ".." || strings.HasPrefix(rel, "../") || strings.HasPrefix(rel, "/") {
		return "", fail(http.StatusBadRequest, "invalid path")
	}
	root := filepath.Clean(f.Root)
	full := filepath.Join(root, rel)
	if full != root && !strings.HasPrefix(full, root+string(filepath.Separator)) {
		return "", fail(http.StatusBadRequest, "path traversal not allowed")
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fail(http.StatusNotFound, "site root not found on this server")
	}
	probe := full
	for {
		resolved, err := filepath.EvalSymlinks(probe)
		if err == nil {
			if resolved != realRoot && !strings.HasPrefix(resolved, realRoot+string(filepath.Separator)) {
				return "", fail(http.StatusBadRequest, "path traversal not allowed")
			}
			return full, nil
		}
		if !os.IsNotExist(err) {
			return "", fail(http.StatusBadRequest, "invalid path")
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			return "", fail(http.StatusBadRequest, "invalid path")
		}
		probe = parent
	}
}

func (f *FS) isRoot(full string) bool { return full == filepath.Clean(f.Root) }

// own hands path to the site owner.
func (f *FS) own(path string) {
	if f.UID > 0 {
		_ = os.Lchown(path, f.UID, f.GID)
	}
}

// mkdirAll creates dir and its missing parents, owned by the site owner.
func (f *FS) mkdirAll(dir string) error {
	var missing []string
	for p := dir; ; p = filepath.Dir(p) {
		if _, err := os.Lstat(p); err == nil {
			break
		}
		missing = append(missing, p)
		if filepath.Dir(p) == p {
			break
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, p := range missing {
		f.own(p)
	}
	return nil
}

// List lists a directory.
func (f *FS) List(rel string) ([]Entry, error) {
	full, err := f.Resolve(rel)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(full)
	if err != nil || !info.IsDir() {
		return nil, fail(http.StatusNotFound, "directory not found")
	}
	entries, err := os.ReadDir(full)
	if err != nil {
		return nil, fail(http.StatusInternalServerError, "failed to read directory")
	}
	if rel == "" {
		rel = "."
	}
	out := make([]Entry, 0, len(entries))
	for _, e := range entries {
		ei, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, Entry{
			Name: e.Name(), Path: filepath.Join(rel, e.Name()), Size: ei.Size(), IsDir: e.IsDir(),
			Mode: ei.Mode().String(), ModTime: ei.ModTime().Format(time.RFC3339),
		})
	}
	return out, nil
}

// Read returns a text file (at most MaxReadSize).
func (f *FS) Read(rel string) ([]byte, os.FileInfo, error) {
	full, err := f.Resolve(rel)
	if err != nil {
		return nil, nil, err
	}
	fh, err := os.OpenFile(full, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, nil, fail(http.StatusNotFound, "file not found")
	}
	defer fh.Close()
	info, err := fh.Stat()
	if err != nil {
		return nil, nil, fail(http.StatusNotFound, "file not found")
	}
	if info.IsDir() {
		return nil, nil, fail(http.StatusBadRequest, "is a directory")
	}
	if info.Size() > MaxReadSize {
		return nil, nil, fail(http.StatusBadRequest, "file too large (>1MB)")
	}
	data, err := io.ReadAll(io.LimitReader(fh, MaxReadSize+1))
	if err != nil {
		return nil, nil, fail(http.StatusInternalServerError, "failed to read file")
	}
	return data, info, nil
}

// Write replaces (or creates) a file. Existing files keep their mode and
// owner; new ones belong to the site owner.
func (f *FS) Write(rel string, content []byte) error {
	full, err := f.Resolve(rel)
	if err != nil {
		return err
	}
	if f.isRoot(full) {
		return fail(http.StatusBadRequest, "invalid path")
	}
	if err := f.mkdirAll(filepath.Dir(full)); err != nil {
		return fail(http.StatusInternalServerError, "failed to create directory")
	}
	mode := os.FileMode(0o644)
	existed := false
	if info, err := os.Lstat(full); err == nil {
		if info.IsDir() {
			return fail(http.StatusBadRequest, "is a directory")
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fail(http.StatusBadRequest, "refusing to write through a symlink")
		}
		mode, existed = info.Mode().Perm(), true
	}
	fh, err := os.OpenFile(full, os.O_WRONLY|os.O_CREATE|os.O_TRUNC|syscall.O_NOFOLLOW, mode)
	if err != nil {
		return fail(http.StatusInternalServerError, "failed to write")
	}
	if _, err := fh.Write(content); err != nil {
		fh.Close()
		return fail(http.StatusInternalServerError, "failed to write")
	}
	if err := fh.Close(); err != nil {
		return fail(http.StatusInternalServerError, "failed to write")
	}
	if !existed {
		f.own(full)
	}
	return nil
}

// Create makes an empty file (never truncating an existing one) or a directory.
func (f *FS) Create(rel string, isDir bool) error {
	full, err := f.Resolve(rel)
	if err != nil {
		return err
	}
	if f.isRoot(full) {
		return fail(http.StatusBadRequest, "invalid path")
	}
	if isDir {
		if err := f.mkdirAll(full); err != nil {
			return fail(http.StatusInternalServerError, "failed to create directory")
		}
		return nil
	}
	if err := f.mkdirAll(filepath.Dir(full)); err != nil {
		return fail(http.StatusInternalServerError, "failed to create directory")
	}
	fh, err := os.OpenFile(full, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			return fail(http.StatusConflict, "already exists")
		}
		return fail(http.StatusInternalServerError, "failed to create file")
	}
	fh.Close()
	f.own(full)
	return nil
}

// Delete removes a file or directory tree (never the root). It reports
// whether the entry was a directory.
func (f *FS) Delete(rel string) (bool, error) {
	full, err := f.Resolve(rel)
	if err != nil {
		return false, err
	}
	if f.isRoot(full) {
		return false, fail(http.StatusBadRequest, "cannot delete site root")
	}
	info, err := os.Lstat(full)
	if err != nil {
		return false, fail(http.StatusNotFound, "not found")
	}
	if err := os.RemoveAll(full); err != nil {
		return false, fail(http.StatusInternalServerError, "failed to delete")
	}
	return info.IsDir(), nil
}

// Rename gives an entry a new name in the same directory.
func (f *FS) Rename(oldRel, newName string) (bool, error) {
	newName = strings.TrimSpace(newName)
	if newName == "" || newName == "." || newName == ".." || strings.ContainsAny(newName, "/\x00") {
		return false, fail(http.StatusBadRequest, "invalid name")
	}
	oldFull, err := f.Resolve(oldRel)
	if err != nil {
		return false, err
	}
	if f.isRoot(oldFull) {
		return false, fail(http.StatusBadRequest, "cannot rename site root")
	}
	newFull := filepath.Join(filepath.Dir(oldFull), newName)
	rel, _ := filepath.Rel(filepath.Clean(f.Root), newFull)
	if _, err := f.Resolve(rel); err != nil {
		return false, err
	}
	info, err := os.Lstat(oldFull)
	if err != nil {
		return false, fail(http.StatusNotFound, "not found")
	}
	if _, err := os.Lstat(newFull); err == nil {
		return false, fail(http.StatusConflict, "already exists: "+newName)
	}
	if err := os.Rename(oldFull, newFull); err != nil {
		return false, fail(http.StatusInternalServerError, "failed to rename")
	}
	return info.IsDir(), nil
}

// Move moves entries into a directory and returns the moved names.
func (f *FS) Move(paths []string, dest string) ([]string, error) {
	if len(paths) == 0 {
		return nil, fail(http.StatusBadRequest, "paths required")
	}
	destFull, err := f.Resolve(dest)
	if err != nil {
		return nil, err
	}
	if info, err := os.Stat(destFull); err != nil || !info.IsDir() {
		return nil, fail(http.StatusBadRequest, "destination is not a directory")
	}
	moved := make([]string, 0, len(paths))
	for _, rel := range paths {
		src, err := f.Resolve(rel)
		if err != nil {
			return moved, err
		}
		if f.isRoot(src) {
			return moved, fail(http.StatusBadRequest, "cannot move site root")
		}
		base := filepath.Base(src)
		dst := filepath.Join(destFull, base)
		if src == destFull || strings.HasPrefix(destFull+string(filepath.Separator), src+string(filepath.Separator)) {
			return moved, fail(http.StatusBadRequest, "cannot move a folder into itself")
		}
		if dst == src {
			continue
		}
		if _, err := os.Lstat(dst); err == nil {
			return moved, fail(http.StatusConflict, "already exists at destination: "+base)
		}
		if err := os.Rename(src, dst); err != nil {
			return moved, fail(http.StatusInternalServerError, "failed to move: "+base)
		}
		moved = append(moved, base)
	}
	return moved, nil
}

// Compress zips entries into dest/name and returns the archive name and size.
// Symlinks and special files are skipped (never followed).
func (f *FS) Compress(paths []string, name, dest string) (string, int64, error) {
	if len(paths) == 0 {
		return "", 0, fail(http.StatusBadRequest, "paths required")
	}
	name = filepath.Base(strings.TrimSpace(name))
	if name == "" || name == "." || name == "/" {
		name = "archive.zip"
	}
	if !strings.HasSuffix(strings.ToLower(name), ".zip") {
		name += ".zip"
	}
	destFull, err := f.Resolve(dest)
	if err != nil {
		return "", 0, err
	}
	if info, err := os.Stat(destFull); err != nil || !info.IsDir() {
		return "", 0, fail(http.StatusBadRequest, "destination is not a directory")
	}
	archive := filepath.Join(destFull, name)
	if _, err := os.Lstat(archive); err == nil {
		return "", 0, fail(http.StatusConflict, "archive already exists: "+name)
	}
	type item struct{ full, base string }
	var items []item
	for _, rel := range paths {
		full, err := f.Resolve(rel)
		if err != nil {
			return "", 0, err
		}
		if _, err := os.Lstat(full); err != nil {
			return "", 0, fail(http.StatusNotFound, "not found: "+rel)
		}
		items = append(items, item{full, filepath.Base(full)})
	}
	out, err := os.OpenFile(archive, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return "", 0, fail(http.StatusInternalServerError, "failed to create archive")
	}
	zw := zip.NewWriter(out)
	for _, it := range items {
		if err := AddToZip(zw, it.full, it.base); err != nil {
			zw.Close()
			out.Close()
			os.Remove(archive)
			return "", 0, fail(http.StatusInternalServerError, "failed to compress: "+it.base)
		}
	}
	if err := zw.Close(); err != nil {
		out.Close()
		os.Remove(archive)
		return "", 0, fail(http.StatusInternalServerError, "failed to finalize archive")
	}
	out.Close()
	f.own(archive)
	var size int64
	if info, err := os.Stat(archive); err == nil {
		size = info.Size()
	}
	return name, size, nil
}

// AddToZip walks src (file or dir) into the zip under baseName, skipping
// symlinks and special files.
func AddToZip(zw *zip.Writer, src, baseName string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
		return nil
	}
	if !info.IsDir() {
		return writeZipFile(zw, src, baseName, info)
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
		if !fi.Mode().IsRegular() {
			return nil
		}
		return writeZipFile(zw, path, zipName, fi)
	})
}

func writeZipFile(zw *zip.Writer, path, zipName string, fi os.FileInfo) error {
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
	fh, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	defer fh.Close()
	_, err = io.Copy(w, fh)
	return err
}

// SaveUpload stores r as relDir/filename (the client-chosen filename is
// reduced to its base name) and returns the relative path and size.
func (f *FS) SaveUpload(relDir, filename string, r io.Reader, maxBytes int64) (string, int64, error) {
	if relDir == "" {
		relDir = "."
	}
	if _, err := f.Resolve(relDir); err != nil {
		return "", 0, err
	}
	name := filepath.Base(filepath.ToSlash(filename))
	if name == "" || name == "." || name == ".." || name == "/" || strings.ContainsRune(name, 0) {
		return "", 0, fail(http.StatusBadRequest, "invalid filename")
	}
	rel := filepath.Join(relDir, name)
	full, err := f.Resolve(rel)
	if err != nil {
		return "", 0, err
	}
	if err := f.mkdirAll(filepath.Dir(full)); err != nil {
		return "", 0, fail(http.StatusInternalServerError, "failed to create directory")
	}
	if info, err := os.Lstat(full); err == nil && (info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
		return "", 0, fail(http.StatusConflict, "a folder or link with this name exists")
	}
	tmp := full + ".sigarda-upload"
	out, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC|syscall.O_NOFOLLOW, 0o644)
	if err != nil {
		return "", 0, fail(http.StatusInternalServerError, "failed to create file")
	}
	n, err := io.Copy(out, io.LimitReader(r, maxBytes+1))
	out.Close()
	if err != nil {
		os.Remove(tmp)
		return "", 0, fail(http.StatusInternalServerError, "failed to save file")
	}
	if n > maxBytes {
		os.Remove(tmp)
		return "", 0, fail(http.StatusRequestEntityTooLarge, "file too large")
	}
	if err := os.Rename(tmp, full); err != nil {
		os.Remove(tmp)
		return "", 0, fail(http.StatusInternalServerError, "failed to save file")
	}
	f.own(full)
	return rel, n, nil
}

// OpenDownload opens a regular file for download.
func (f *FS) OpenDownload(rel string) (*os.File, os.FileInfo, error) {
	full, err := f.Resolve(rel)
	if err != nil {
		return nil, nil, err
	}
	fh, err := os.OpenFile(full, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, nil, fail(http.StatusNotFound, "file not found")
	}
	info, err := fh.Stat()
	if err != nil || !info.Mode().IsRegular() {
		fh.Close()
		if err == nil && info.IsDir() {
			return nil, nil, fail(http.StatusBadRequest, "cannot download a directory")
		}
		return nil, nil, fail(http.StatusNotFound, "file not found")
	}
	return fh, info, nil
}
