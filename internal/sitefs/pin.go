package sitefs

import (
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// The agent runs as root and site roots live in trees their (unprivileged)
// owners can change: /home/<user>/htdocs/<domain>. A site user who replaces
// a component of that path with a symlink (~/htdocs -> /etc) would make every
// root operation on the site root act on the link's target. Root paths are
// therefore checked and pinned before use.

// CheckNoUserSymlinks refuses a path whose existing components include a
// symbolic link that a non-root user could have planted: a link is accepted
// only when root owns it and its directory is root-owned and not writable by
// others (system layouts such as /home -> /var/home). Components that do not
// exist yet are not checked (callers pin the directory once it exists).
func CheckNoUserSymlinks(path string) error {
	clean := filepath.Clean(path)
	if !filepath.IsAbs(clean) {
		return fail(http.StatusBadRequest, "site root must be an absolute path")
	}
	cur := "/"
	for _, part := range strings.Split(strings.TrimPrefix(clean, "/"), "/") {
		if part == "" {
			continue
		}
		parent := cur
		cur = filepath.Join(cur, part)
		fi, err := os.Lstat(cur)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return fail(http.StatusBadRequest, "site root is not accessible")
		}
		if fi.Mode()&os.ModeSymlink == 0 {
			continue
		}
		if !rootOwned(fi) {
			return fail(http.StatusBadRequest, "site root path contains a symbolic link ("+cur+")")
		}
		pfi, err := os.Stat(parent)
		if err != nil || !rootOwned(pfi) || (pfi.Mode().Perm()&0o002 != 0 && pfi.Mode()&os.ModeSticky == 0) {
			return fail(http.StatusBadRequest, "site root path contains a symbolic link ("+cur+")")
		}
	}
	return nil
}

func rootOwned(fi os.FileInfo) bool {
	st, ok := fi.Sys().(*syscall.Stat_t)
	return ok && st.Uid == 0
}

// PinnedDir is a directory opened by the agent and verified to be the one the
// path names without user-planted symlinks. Path refers to the open
// descriptor (/proc/<pid>/fd/<n>), so later renames or symlink swaps in the
// tree cannot redirect operations that use it, including child processes
// (root can open another root process's fd links).
type PinnedDir struct {
	f    *os.File
	Path string
	Info os.FileInfo
}

// Close releases the descriptor.
func (p *PinnedDir) Close() error { return p.f.Close() }

// PinDir opens the directory at path and verifies it. Opening first and
// checking afterwards closes the race: the directory that was opened must be
// the one a symlink-free walk of path reaches now, and a tree a site user
// controls can never reach another directory's inode without a symlink
// (directories cannot be hard-linked; bind mounts need root).
func PinDir(path string) (*PinnedDir, error) {
	clean := filepath.Clean(path)
	f, err := os.OpenFile(clean, os.O_RDONLY|syscall.O_DIRECTORY, 0)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fail(http.StatusNotFound, "site root not found on this server")
		}
		return nil, fail(http.StatusBadRequest, "site root is not a directory")
	}
	opened, err := f.Stat()
	if err != nil || !opened.IsDir() {
		f.Close()
		return nil, fail(http.StatusBadRequest, "site root is not a directory")
	}
	if err := CheckNoUserSymlinks(clean); err != nil {
		f.Close()
		return nil, err
	}
	// Separate lookups (open, walk, stat) can each see a different tree
	// while the site user swaps a component between a symlink and nothing.
	// The kernel's path of the opened directory is authoritative: it must be
	// exactly where the path leads through trusted links only.
	fdPath := fmt.Sprintf("/proc/%d/fd/%d", os.Getpid(), f.Fd())
	real, err := os.Readlink(fdPath)
	if err != nil {
		f.Close()
		return nil, fail(http.StatusInternalServerError, "cannot verify the site root (is /proc mounted?)")
	}
	want, err := trustedRealPath(clean, 0)
	if err != nil {
		f.Close()
		return nil, err
	}
	if real != want {
		f.Close()
		return nil, fail(http.StatusBadRequest, "site root changed while it was opened")
	}
	p := &PinnedDir{f: f, Path: fdPath, Info: opened}
	return p, nil
}

// trustedRealPath is where path leads when only trusted links (root-owned,
// in a root-owned directory: system layouts such as /home -> /var/home) are
// followed. A user-planted link is refused; a missing component ends the
// walk and the rest of the path is taken as written (it cannot match an
// opened directory that lives elsewhere).
func trustedRealPath(path string, depth int) (string, error) {
	if depth > 8 {
		return "", fail(http.StatusBadRequest, "too many levels of symbolic links in the site root")
	}
	parts := strings.Split(strings.TrimPrefix(filepath.Clean(path), "/"), "/")
	cur := "/"
	for i, part := range parts {
		if part == "" {
			continue
		}
		next := filepath.Join(cur, part)
		fi, err := os.Lstat(next)
		if errors.Is(err, fs.ErrNotExist) {
			return filepath.Join(append([]string{cur}, parts[i:]...)...), nil
		}
		if err != nil {
			return "", fail(http.StatusBadRequest, "site root is not accessible")
		}
		if fi.Mode()&os.ModeSymlink == 0 {
			cur = next
			continue
		}
		pfi, err := os.Stat(cur)
		if !rootOwned(fi) || err != nil || !rootOwned(pfi) || (pfi.Mode().Perm()&0o002 != 0 && pfi.Mode()&os.ModeSticky == 0) {
			return "", fail(http.StatusBadRequest, "site root path contains a symbolic link ("+next+")")
		}
		target, err := os.Readlink(next)
		if err != nil {
			return "", fail(http.StatusBadRequest, "site root is not accessible")
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(cur, target)
		}
		if cur, err = trustedRealPath(target, depth+1); err != nil {
			return "", err
		}
	}
	return cur, nil
}

// PinParent pins the parent directory of path and returns it with the base
// name; operations then name the entry relative to the pinned parent, which
// never follows a symlink planted in place of the parent.
func PinParent(path string) (*PinnedDir, string, error) {
	clean := filepath.Clean(path)
	base := filepath.Base(clean)
	if base == "/" || base == "." || base == ".." {
		return nil, "", fail(http.StatusBadRequest, "invalid site root")
	}
	p, err := PinDir(filepath.Dir(clean))
	if err != nil {
		return nil, "", err
	}
	return p, base, nil
}

// RemoveTree deletes the site root at path without following symlinks: the
// parent is pinned and the entry is removed through an os.Root on it, so a
// root (or parent) swapped for a link to /etc is refused or only the link is
// removed.
func RemoveTree(path string) error {
	parent, base, err := PinParent(path)
	if err != nil {
		if StatusOf(err) == http.StatusNotFound {
			return nil // nothing left to delete
		}
		return err
	}
	defer parent.Close()
	r, err := os.OpenRoot(parent.Path)
	if err != nil {
		return fail(http.StatusInternalServerError, "open site parent failed")
	}
	defer r.Close()
	fi, err := r.Lstat(base)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return fail(http.StatusBadRequest, "site root is a symbolic link; refusing to delete through it")
	}
	return r.RemoveAll(base)
}

// WriteNewFile creates dir/name with content only when nothing exists there
// (O_EXCL, no symlink followed); dir must be a pinned directory path.
func WriteNewFile(dir, name string, content []byte, mode os.FileMode) error {
	if strings.ContainsRune(name, '/') {
		return fail(http.StatusBadRequest, "invalid file name")
	}
	f, err := os.OpenFile(filepath.Join(dir, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, mode)
	if err != nil {
		return err
	}
	if _, err := f.Write(content); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// ChownTree gives the pinned directory and everything beneath it to uid:gid
// without following any symlink (each entry is Lchown'ed through an os.Root).
func ChownTree(p *PinnedDir, uid, gid int) error {
	if err := p.f.Chown(uid, gid); err != nil {
		return err
	}
	r, err := os.OpenRoot(p.Path)
	if err != nil {
		return err
	}
	defer r.Close()
	return fs.WalkDir(r.FS(), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == "." {
			return nil
		}
		return r.Lchown(path, uid, gid)
	})
}

// Chown changes the owner of the pinned directory itself (fchown on the
// open descriptor, never a path).
func (p *PinnedDir) Chown(uid, gid int) error { return p.f.Chown(uid, gid) }
