package agenthttp

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var errUnsafePath = errors.New("unsafe path")

var (
	domainRegex = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9\-\.]{0,253}[a-zA-Z0-9])?$`)
	pathRegex   = regexp.MustCompile(`^/[a-zA-Z0-9_\-\./]+$`)
	gitURLRegex = regexp.MustCompile(`^(https?://|git@|file://)[a-zA-Z0-9._\-/:]+$`)
	gitRefRegex = regexp.MustCompile(`^[a-zA-Z0-9._/\-]{1,255}$`)
	dbNameRegex = regexp.MustCompile(`^[a-zA-Z0-9_]{1,64}$`)
	dbHostRegex = regexp.MustCompile(`^[a-zA-Z0-9_.%\-]{1,255}$`)
	dbPrivRegex = regexp.MustCompile(`^[A-Z_, ]{1,128}$`)
)

func isValidDomain(d string) bool {
	return domainRegex.MatchString(d)
}

func isValidPath(p string) bool {
	if strings.Contains(p, "..") {
		return false
	}
	return pathRegex.MatchString(p)
}

func isValidGitURL(u string) bool {
	return gitURLRegex.MatchString(u)
}

// isValidGitRef validates a git branch/ref name. It rejects empty values, refs
// beginning with "-" (which git would treat as a flag), and any characters
// outside a conservative allowlist. Prevents git argument injection.
func isValidGitRef(ref string) bool {
	if ref == "" || strings.HasPrefix(ref, "-") {
		return false
	}
	if strings.Contains(ref, "..") {
		return false
	}
	return gitRefRegex.MatchString(ref)
}

func isValidDBIdentifier(value string) bool {
	return dbNameRegex.MatchString(value)
}

func isValidDBHostPattern(value string) bool {
	return dbHostRegex.MatchString(value)
}

func isValidDBPrivileges(value string) bool {
	return dbPrivRegex.MatchString(value)
}

func sqlQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func (r *Router) isAllowedSitePath(path string) bool {
	if !isValidPath(path) {
		return false
	}
	if r.siteRootDir == "" {
		return true
	}
	cleanRoot := filepath.Clean(r.siteRootDir)
	cleanPath := filepath.Clean(path)
	if cleanPath == cleanRoot {
		return false
	}
	return strings.HasPrefix(cleanPath, cleanRoot+string(filepath.Separator))
}

func (r *Router) isAllowedBackupPath(path string) bool {
	if !isValidPath(path) {
		return false
	}
	cleanPath := filepath.Clean(path)
	// Allow paths under the centralized backup directory (old behavior)
	cleanRoot := filepath.Clean(r.backupDir)
	if strings.HasPrefix(cleanPath, cleanRoot+string(filepath.Separator)) {
		return true
	}
	// Also allow paths that contain a /backup/ or /backups/ segment in a
	// non-root position. Files backups are written to <site_root>/backups/ and
	// database backups to <site_root>/backup/db/, so both spellings are valid.
	sep := string(filepath.Separator)
	parts := strings.Split(cleanPath, sep)
	for i, part := range parts {
		if part == ".." {
			return false
		}
		// Must not be the last segment (that is the file itself) and must sit
		// below a real parent directory.
		if (part == "backup" || part == "backups") && i > 0 && i < len(parts)-1 {
			return true
		}
	}
	// Also allow paths under siteRootDir/<domain>/backup(s)/ (backup inside site directory)
	if r.siteRootDir != "" {
		cleanSiteRoot := filepath.Clean(r.siteRootDir)
		if strings.HasPrefix(cleanPath, cleanSiteRoot+sep) {
			// Must be inside a site's backup directory: <siteRoot>/<domain>/backup(s)/
			rel := strings.TrimPrefix(cleanPath, cleanSiteRoot+sep)
			relParts := strings.SplitN(rel, sep, 3)
			if len(relParts) >= 2 && (relParts[1] == "backup" || relParts[1] == "backups") {
				return true
			}
		}
	}
	return false
}

func (r *Router) removeGeneratedFile(ctx context.Context, path string, root string) error {
	cleanRoot := filepath.Clean(root)
	cleanPath := filepath.Clean(path)
	if cleanPath == cleanRoot || !strings.HasPrefix(cleanPath, cleanRoot+string(filepath.Separator)) {
		return errUnsafePath
	}
	return r.executor.RemoveAll(ctx, cleanPath)
}

// movePath moves src to dst. It first tries an atomic os.Rename; if that fails
// because src and dst live on different filesystems (or any rename error), it
// falls back to a recursive copy followed by removing src. This makes restore
// safe when the extraction temp dir and the site root are on separate mounts.
func movePath(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	if err := copyPath(src, dst); err != nil {
		return err
	}
	return os.RemoveAll(src)
}

// copyPath recursively copies src to dst, preserving file modes and symlinks.
func copyPath(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		target, err := os.Readlink(src)
		if err != nil {
			return err
		}
		return os.Symlink(target, dst)
	case info.IsDir():
		if err := os.MkdirAll(dst, info.Mode().Perm()); err != nil {
			return err
		}
		entries, err := os.ReadDir(src)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if err := copyPath(filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())); err != nil {
				return err
			}
		}
		return nil
	default:
		in, err := os.Open(src)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			out.Close()
			return err
		}
		return out.Close()
	}
}

func countFiles(root string) int {
	count := 0
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		count++
		return nil
	})
	return count
}
