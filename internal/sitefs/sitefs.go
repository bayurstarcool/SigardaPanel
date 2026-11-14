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
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"
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
	// IsSymlink: the entry is a symbolic link. IsDir then tells whether it
	// points at a directory inside the root (links leaving the root are
	// shown but cannot be followed).
	IsSymlink bool `json:"is_symlink,omitempty"`
}

// clean validates a path relative to the root and returns it cleaned
// ("." for the root itself).
func (f *FS) clean(rel string) (string, error) {
	if f.Root == "" {
		return "", fail(http.StatusBadRequest, "site root path not configured")
	}
	rel = strings.TrimSpace(rel)
	if rel == "" {
		rel = "."
	}
	rel = filepath.Clean(rel)
	if rel == ".." || strings.HasPrefix(rel, "../") || strings.HasPrefix(rel, "/") || strings.ContainsRune(rel, 0) {
		return "", fail(http.StatusBadRequest, "invalid path")
	}
	return rel, nil
}

// open opens the site root. Every operation goes through the returned
// os.Root, which resolves each path component beneath the root at the
// moment of use: a directory swapped for a symlink to /etc between a check
// and the operation cannot redirect it outside the site.
func (f *FS) open() (*os.Root, error) {
	if f.Root == "" {
		return nil, fail(http.StatusBadRequest, "site root path not configured")
	}
	// The root itself must not be reached through a symlink the site user
	// planted (os.Root confines paths beneath the root, not the root path).
	pin, err := PinDir(f.Root)
	if err != nil {
		return nil, err
	}
	defer pin.Close()
	r, err := os.OpenRoot(pin.Path)
	if err != nil {
		return nil, fail(http.StatusNotFound, "site root not found on this server")
	}
	return r, nil
}

// Resolve turns a path relative to the root into an absolute one, refusing
// anything that leaves the root, also through symlinks. Operations do not
// use the result (they go through os.Root); it reports whether a path is
// acceptable.
func (f *FS) Resolve(rel string) (string, error) {
	rel, err := f.clean(rel)
	if err != nil {
		return "", err
	}
	r, err := f.open()
	if err != nil {
		return "", err
	}
	defer r.Close()
	// The deepest existing ancestor must be reachable beneath the root.
	for probe := rel; ; probe = filepath.Dir(probe) {
		if _, err := r.Stat(probe); err == nil {
			break
		} else if !errors.Is(err, fs.ErrNotExist) {
			return "", fail(http.StatusBadRequest, "path traversal not allowed")
		}
		if probe == "." {
			break
		}
	}
	return filepath.Join(filepath.Clean(f.Root), rel), nil
}

// own hands path (relative to the root) to the site owner.
func (f *FS) own(r *os.Root, rel string) {
	if f.UID > 0 {
		_ = r.Lchown(rel, f.UID, f.GID)
	}
}

// mkdirAll creates dir and its missing parents beneath the root, owned by
// the site owner.
func (f *FS) mkdirAll(r *os.Root, dir string) error {
	var missing []string
	for p := dir; p != "."; p = filepath.Dir(p) {
		if _, err := r.Lstat(p); err == nil {
			break
		}
		missing = append(missing, p)
	}
	if err := r.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, p := range missing {
		f.own(r, p)
	}
	return nil
}

// List lists a directory.
func (f *FS) List(rel string) ([]Entry, error) {
	clean, err := f.clean(rel)
	if err != nil {
		return nil, err
	}
	r, err := f.open()
	if err != nil {
		return nil, err
	}
	defer r.Close()
	dir, err := r.Open(clean)
	if err != nil {
		if strings.Contains(err.Error(), "escapes") {
			return nil, fail(http.StatusBadRequest, "path traversal not allowed")
		}
		return nil, fail(http.StatusNotFound, "directory not found")
	}
	defer dir.Close()
	if info, err := dir.Stat(); err != nil || !info.IsDir() {
		return nil, fail(http.StatusNotFound, "directory not found")
	}
	entries, err := dir.ReadDir(-1)
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
		ent := Entry{
			Name: e.Name(), Path: filepath.Join(rel, e.Name()), Size: ei.Size(), IsDir: e.IsDir(),
			Mode: ei.Mode().String(), ModTime: ei.ModTime().Format(time.RFC3339),
		}
		if ei.Mode()&os.ModeSymlink != 0 {
			ent.IsSymlink = true
			// os.Root follows the link only while it stays beneath the root.
			if ti, err := r.Stat(filepath.Join(clean, e.Name())); err == nil {
				ent.IsDir = ti.IsDir()
			}
		}
		out = append(out, ent)
	}
	// Folders first, then by name (case-insensitive): ReadDir returns disk order.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].IsDir != out[j].IsDir {
			return out[i].IsDir
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

// pathErr maps an os.Root error: leaving the root is a client error.
func pathErr(err error, status int, msg string) error {
	if err != nil && strings.Contains(err.Error(), "escapes") {
		return fail(http.StatusBadRequest, "path traversal not allowed")
	}
	return fail(status, msg)
}

// Read returns a text file (at most MaxReadSize).
func (f *FS) Read(rel string) ([]byte, os.FileInfo, error) {
	clean, err := f.clean(rel)
	if err != nil {
		return nil, nil, err
	}
	r, err := f.open()
	if err != nil {
		return nil, nil, err
	}
	defer r.Close()
	// O_NONBLOCK: opening a FIFO the site user planted must not hang the
	// agent (os.Root follows links only beneath the root).
	fh, err := r.OpenFile(clean, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, nil, pathErr(err, http.StatusNotFound, "file not found")
	}
	defer fh.Close()
	info, err := fh.Stat()
	if err != nil {
		return nil, nil, fail(http.StatusNotFound, "file not found")
	}
	if info.IsDir() {
		return nil, nil, fail(http.StatusBadRequest, "is a directory")
	}
	if !info.Mode().IsRegular() {
		return nil, nil, fail(http.StatusBadRequest, "not a regular file")
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
	clean, err := f.clean(rel)
	if err != nil {
		return err
	}
	if clean == "." {
		return fail(http.StatusBadRequest, "invalid path")
	}
	r, err := f.open()
	if err != nil {
		return err
	}
	defer r.Close()
	if err := f.mkdirAll(r, filepath.Dir(clean)); err != nil {
		return pathErr(err, http.StatusInternalServerError, "failed to create directory")
	}
	mode := os.FileMode(0o644)
	existed := false
	if info, err := r.Lstat(clean); err == nil {
		if info.IsDir() {
			return fail(http.StatusBadRequest, "is a directory")
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fail(http.StatusBadRequest, "refusing to write through a symlink")
		}
		mode, existed = info.Mode().Perm(), true
	}
	fh, err := r.OpenFile(clean, os.O_WRONLY|os.O_CREATE|os.O_TRUNC|syscall.O_NOFOLLOW, mode)
	if err != nil {
		return pathErr(err, http.StatusInternalServerError, "failed to write")
	}
	if _, err := fh.Write(content); err != nil {
		fh.Close()
		return fail(http.StatusInternalServerError, "failed to write")
	}
	if err := fh.Close(); err != nil {
		return fail(http.StatusInternalServerError, "failed to write")
	}
	if !existed {
		f.own(r, clean)
	}
	return nil
}

// Create makes an empty file (never truncating an existing one) or a directory.
func (f *FS) Create(rel string, isDir bool) error {
	clean, err := f.clean(rel)
	if err != nil {
		return err
	}
	if clean == "." {
		return fail(http.StatusBadRequest, "invalid path")
	}
	r, err := f.open()
	if err != nil {
		return err
	}
	defer r.Close()
	if isDir {
		if err := f.mkdirAll(r, clean); err != nil {
			return pathErr(err, http.StatusInternalServerError, "failed to create directory")
		}
		return nil
	}
	if err := f.mkdirAll(r, filepath.Dir(clean)); err != nil {
		return pathErr(err, http.StatusInternalServerError, "failed to create directory")
	}
	fh, err := r.OpenFile(clean, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0o644)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			return fail(http.StatusConflict, "already exists")
		}
		return pathErr(err, http.StatusInternalServerError, "failed to create file")
	}
	fh.Close()
	f.own(r, clean)
	return nil
}

// Delete removes a file or directory tree (never the root). It reports
// whether the entry was a directory.
func (f *FS) Delete(rel string) (bool, error) {
	clean, err := f.clean(rel)
	if err != nil {
		return false, err
	}
	if clean == "." {
		return false, fail(http.StatusBadRequest, "cannot delete site root")
	}
	r, err := f.open()
	if err != nil {
		return false, err
	}
	defer r.Close()
	info, err := r.Lstat(clean)
	if err != nil {
		return false, pathErr(err, http.StatusNotFound, "not found")
	}
	if err := r.RemoveAll(clean); err != nil {
		return false, pathErr(err, http.StatusInternalServerError, "failed to delete")
	}
	return info.IsDir(), nil
}

// Rename gives an entry a new name in the same directory.
func (f *FS) Rename(oldRel, newName string) (bool, error) {
	newName = strings.TrimSpace(newName)
	if newName == "" || newName == "." || newName == ".." || strings.ContainsAny(newName, "/\x00") {
		return false, fail(http.StatusBadRequest, "invalid name")
	}
	oldClean, err := f.clean(oldRel)
	if err != nil {
		return false, err
	}
	if oldClean == "." {
		return false, fail(http.StatusBadRequest, "cannot rename site root")
	}
	newClean := filepath.Join(filepath.Dir(oldClean), newName)
	r, err := f.open()
	if err != nil {
		return false, err
	}
	defer r.Close()
	info, err := r.Lstat(oldClean)
	if err != nil {
		return false, pathErr(err, http.StatusNotFound, "not found")
	}
	if _, err := r.Lstat(newClean); err == nil {
		return false, fail(http.StatusConflict, "already exists: "+newName)
	}
	if err := r.Rename(oldClean, newClean); err != nil {
		return false, pathErr(err, http.StatusInternalServerError, "failed to rename")
	}
	return info.IsDir(), nil
}

// Move moves entries into a directory and returns the moved names.
func (f *FS) Move(paths []string, dest string) ([]string, error) {
	if len(paths) == 0 {
		return nil, fail(http.StatusBadRequest, "paths required")
	}
	destClean, err := f.clean(dest)
	if err != nil {
		return nil, err
	}
	r, err := f.open()
	if err != nil {
		return nil, err
	}
	defer r.Close()
	if info, err := r.Stat(destClean); err != nil || !info.IsDir() {
		return nil, pathErr(err, http.StatusBadRequest, "destination is not a directory")
	}
	moved := make([]string, 0, len(paths))
	for _, rel := range paths {
		src, err := f.clean(rel)
		if err != nil {
			return moved, err
		}
		if src == "." {
			return moved, fail(http.StatusBadRequest, "cannot move site root")
		}
		base := filepath.Base(src)
		dst := filepath.Join(destClean, base)
		if src == destClean || strings.HasPrefix(destClean+"/", src+"/") {
			return moved, fail(http.StatusBadRequest, "cannot move a folder into itself")
		}
		if dst == src {
			continue
		}
		if _, err := r.Lstat(dst); err == nil {
			return moved, fail(http.StatusConflict, "already exists at destination: "+base)
		}
		if err := r.Rename(src, dst); err != nil {
			return moved, pathErr(err, http.StatusInternalServerError, "failed to move: "+base)
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
	destClean, err := f.clean(dest)
	if err != nil {
		return "", 0, err
	}
	r, err := f.open()
	if err != nil {
		return "", 0, err
	}
	defer r.Close()
	if info, err := r.Stat(destClean); err != nil || !info.IsDir() {
		return "", 0, pathErr(err, http.StatusBadRequest, "destination is not a directory")
	}
	archive := filepath.Join(destClean, name)
	if _, err := r.Lstat(archive); err == nil {
		return "", 0, fail(http.StatusConflict, "archive already exists: "+name)
	}
	var items []string
	for _, rel := range paths {
		clean, err := f.clean(rel)
		if err != nil {
			return "", 0, err
		}
		if _, err := r.Lstat(clean); err != nil {
			return "", 0, pathErr(err, http.StatusNotFound, "not found: "+rel)
		}
		items = append(items, clean)
	}
	out, err := r.OpenFile(archive, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0o644)
	if err != nil {
		return "", 0, pathErr(err, http.StatusInternalServerError, "failed to create archive")
	}
	zw := zip.NewWriter(out)
	for _, it := range items {
		if err := addRootToZip(zw, r, it, filepath.Base(it), archive); err != nil {
			zw.Close()
			out.Close()
			r.Remove(archive)
			return "", 0, fail(http.StatusInternalServerError, "failed to compress: "+filepath.Base(it))
		}
	}
	if err := zw.Close(); err != nil {
		out.Close()
		r.Remove(archive)
		return "", 0, fail(http.StatusInternalServerError, "failed to finalize archive")
	}
	out.Close()
	f.own(r, archive)
	var size int64
	if info, err := r.Stat(archive); err == nil {
		size = info.Size()
	}
	return name, size, nil
}

// addRootToZip adds src (relative to r) under baseName, skipping symlinks,
// special files and the archive being written.
func addRootToZip(zw *zip.Writer, r *os.Root, src, baseName, skip string) error {
	return fs.WalkDir(r.FS(), src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == skip {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		zipName := baseName
		if rel != "." {
			zipName = filepath.ToSlash(filepath.Join(baseName, rel))
		}
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			return nil
		case info.IsDir():
			if rel == "." && path == src {
				return nil
			}
			_, err := zw.Create(zipName + "/")
			return err
		case !info.Mode().IsRegular():
			return nil
		}
		hdr, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		hdr.Name = zipName
		hdr.Method = zip.Deflate
		w, err := zw.CreateHeader(hdr)
		if err != nil {
			return err
		}
		fh, err := r.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
		if err != nil {
			return err
		}
		defer fh.Close()
		_, err = io.Copy(w, fh)
		return err
	})
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
	fh, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return err
	}
	defer fh.Close()
	_, err = io.Copy(w, fh)
	return err
}

// SaveUpload stores r as relDir/filename (the client-chosen filename is
// reduced to its base name) and returns the relative path and size.
func (f *FS) SaveUpload(relDir, filename string, in io.Reader, maxBytes int64) (string, int64, error) {
	dir, err := f.clean(relDir)
	if err != nil {
		return "", 0, err
	}
	name := filepath.Base(filepath.ToSlash(filename))
	if name == "" || name == "." || name == ".." || name == "/" || strings.ContainsRune(name, 0) {
		return "", 0, fail(http.StatusBadRequest, "invalid filename")
	}
	rel := filepath.Join(dir, name)
	r, err := f.open()
	if err != nil {
		return "", 0, err
	}
	defer r.Close()
	if err := f.mkdirAll(r, dir); err != nil {
		return "", 0, pathErr(err, http.StatusInternalServerError, "failed to create directory")
	}
	if info, err := r.Lstat(rel); err == nil && (info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
		return "", 0, fail(http.StatusConflict, "a folder or link with this name exists")
	}
	tmp := rel + ".sigarda-upload"
	out, err := r.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC|syscall.O_NOFOLLOW, 0o644)
	if err != nil {
		return "", 0, pathErr(err, http.StatusInternalServerError, "failed to create file")
	}
	n, err := io.Copy(out, io.LimitReader(in, maxBytes+1))
	out.Close()
	if err != nil {
		r.Remove(tmp)
		return "", 0, fail(http.StatusInternalServerError, "failed to save file")
	}
	if n > maxBytes {
		r.Remove(tmp)
		return "", 0, fail(http.StatusRequestEntityTooLarge, "file too large")
	}
	if err := r.Rename(tmp, rel); err != nil {
		r.Remove(tmp)
		return "", 0, pathErr(err, http.StatusInternalServerError, "failed to save file")
	}
	f.own(r, rel)
	return rel, n, nil
}

// OpenDownload opens a regular file for download.
func (f *FS) OpenDownload(rel string) (*os.File, os.FileInfo, error) {
	clean, err := f.clean(rel)
	if err != nil {
		return nil, nil, err
	}
	r, err := f.open()
	if err != nil {
		return nil, nil, err
	}
	defer r.Close() // an open file stays usable after its Root is closed
	// Links resolve only beneath the root; O_NONBLOCK keeps a FIFO from
	// blocking the open (it is then refused as not regular).
	fh, err := r.OpenFile(clean, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, nil, pathErr(err, http.StatusNotFound, "file not found")
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

// IsText reports whether data looks like text the editor can round-trip:
// valid UTF-8 without NUL bytes. Anything else is offered as a download only,
// since editing would replace invalid bytes and corrupt the file.
func IsText(data []byte) bool {
	for _, b := range data {
		if b == 0 {
			return false
		}
	}
	return utf8.Valid(data)
}

// Chmod sets the permission bits (octal, e.g. "644" or "0755") of a file or
// directory. setuid/setgid/sticky bits are refused, and symbolic links are
// not followed.
func (f *FS) Chmod(rel, mode string) error {
	clean, err := f.clean(rel)
	if err != nil {
		return err
	}
	if clean == "." {
		return fail(http.StatusBadRequest, "cannot change the site root")
	}
	mode = strings.TrimSpace(mode)
	v, err := strconv.ParseUint(mode, 8, 32)
	if err != nil || len(mode) < 3 || len(mode) > 4 || v > 0o777 {
		return fail(http.StatusBadRequest, "mode must be octal permissions like 644 or 755")
	}
	r, err := f.open()
	if err != nil {
		return err
	}
	defer r.Close()
	info, err := r.Lstat(clean)
	if err != nil {
		return pathErr(err, http.StatusNotFound, "not found")
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fail(http.StatusBadRequest, "cannot change the permissions of a symbolic link")
	}
	if err := r.Chmod(clean, os.FileMode(v)); err != nil {
		return pathErr(err, http.StatusInternalServerError, "failed to change permissions")
	}
	return nil
}

// Extraction limits: a zip bomb must not fill the disk or the inode table.
const (
	MaxExtractEntries = 50000
	MaxExtractBytes   = 4 << 30
)

// Extract unpacks a .zip archive into dest (both relative to the root).
// Every path goes through os.Root, so entries cannot escape the site
// ("zip slip"); symbolic links and special files are skipped, and files that
// already exist are kept (reported as skipped). Extracted files belong to the
// site owner. Returns the number of files written and skipped.
func (f *FS) Extract(archiveRel, dest string) (int, int, error) {
	archive, err := f.clean(archiveRel)
	if err != nil {
		return 0, 0, err
	}
	if !strings.HasSuffix(strings.ToLower(archive), ".zip") {
		return 0, 0, fail(http.StatusBadRequest, "only .zip archives can be extracted")
	}
	destClean, err := f.clean(dest)
	if err != nil {
		return 0, 0, err
	}
	r, err := f.open()
	if err != nil {
		return 0, 0, err
	}
	defer r.Close()
	if info, err := r.Stat(destClean); err != nil || !info.IsDir() {
		return 0, 0, pathErr(err, http.StatusBadRequest, "destination is not a directory")
	}
	fh, err := r.OpenFile(archive, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return 0, 0, pathErr(err, http.StatusNotFound, "archive not found")
	}
	defer fh.Close()
	info, err := fh.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return 0, 0, fail(http.StatusBadRequest, "not a file")
	}
	zr, err := zip.NewReader(fh, info.Size())
	if err != nil {
		return 0, 0, fail(http.StatusBadRequest, "not a valid zip archive")
	}
	if len(zr.File) > MaxExtractEntries {
		return 0, 0, fail(http.StatusBadRequest, "archive has too many entries")
	}
	var total uint64
	for _, zf := range zr.File {
		total += zf.UncompressedSize64
	}
	if total > MaxExtractBytes {
		return 0, 0, fail(http.StatusBadRequest, "archive is too large to extract (over 4 GiB)")
	}
	written, skipped := 0, 0
	var budget int64 = MaxExtractBytes
	for _, zf := range zr.File {
		name := filepath.Clean(filepath.FromSlash(strings.TrimLeft(zf.Name, "/")))
		if name == "." || name == ".." || strings.HasPrefix(name, ".."+string(filepath.Separator)) || filepath.IsAbs(name) {
			skipped++
			continue
		}
		target := filepath.Join(destClean, name)
		mode := zf.Mode()
		switch {
		case mode.IsDir():
			if err := f.mkdirAll(r, target); err != nil {
				return written, skipped, pathErr(err, http.StatusInternalServerError, "failed to create "+name)
			}
			continue
		case !mode.IsRegular():
			skipped++ // symlinks, devices, pipes
			continue
		}
		if err := f.mkdirAll(r, filepath.Dir(target)); err != nil {
			return written, skipped, pathErr(err, http.StatusInternalServerError, "failed to create "+filepath.Dir(name))
		}
		perm := os.FileMode(0o644)
		if mode.Perm()&0o111 != 0 {
			perm = 0o755
		}
		out, err := r.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, perm)
		if err != nil {
			if errors.Is(err, fs.ErrExist) {
				skipped++
				continue
			}
			return written, skipped, pathErr(err, http.StatusInternalServerError, "failed to write "+name)
		}
		in, err := zf.Open()
		if err != nil {
			out.Close()
			r.Remove(target)
			return written, skipped, fail(http.StatusBadRequest, "corrupt archive entry: "+name)
		}
		// The size headers can lie: cap what is actually decompressed.
		n, err := io.Copy(out, io.LimitReader(in, budget+1))
		in.Close()
		out.Close()
		budget -= n
		if err != nil || budget < 0 {
			r.Remove(target)
			if budget < 0 {
				return written, skipped, fail(http.StatusBadRequest, "archive is too large to extract (over 4 GiB)")
			}
			return written, skipped, fail(http.StatusInternalServerError, "failed to extract "+name)
		}
		f.own(r, target)
		written++
	}
	return written, skipped, nil
}
