package agenthttp

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// excludeManifest is stored at the top of an archive made with exclude
// patterns, so a restore knows what the archive deliberately left out.
const excludeManifest = ".sigarda-excludes"

// cleanExcludePatterns validates backup exclude patterns.
func cleanExcludePatterns(in []string) ([]string, error) {
	var out []string
	for _, p := range in {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if len(p) > 200 || strings.ContainsAny(p, "\x00\r\n") || p == "." || p == "*" || p == "/" {
			return nil, fmt.Errorf("invalid exclude pattern %q", p)
		}
		if _, err := path.Match(p, "x"); err != nil {
			return nil, fmt.Errorf("invalid exclude pattern %q", p)
		}
		out = append(out, strings.Trim(p, "/"))
	}
	if len(out) > 50 {
		return nil, fmt.Errorf("too many exclude patterns (max 50)")
	}
	return out, nil
}

// userExcludeArgs are tar arguments for the patterns, matched against any
// part of a member name (the built-in excludes before them are anchored).
func userExcludeArgs(patterns []string) []string {
	if len(patterns) == 0 {
		return nil
	}
	args := []string{"--no-anchored"}
	for _, p := range patterns {
		args = append(args, "--exclude="+p)
	}
	return args
}

// matchesExclude reports whether rel (slash-separated, relative to the site
// root) was left out by the patterns the way tar --no-anchored does: a
// pattern matches any run of path components, and an excluded directory
// takes everything below it.
func matchesExclude(rel string, patterns []string) bool {
	parts := strings.Split(rel, "/")
	for i := range parts {
		for j := i + 1; j <= len(parts); j++ {
			seg := strings.Join(parts[i:j], "/")
			for _, p := range patterns {
				if ok, _ := path.Match(p, seg); ok {
					return true
				}
			}
		}
	}
	return false
}

// carryExcluded moves excluded paths from the previous root into the
// restored one (unless the archive brought its own copy).
//
// The restored tree comes from an archive the site owner controls and may
// contain symlinks (e.g. logs -> /etc/cron.d): every move goes through an
// os.Root on the shared parent directory, which refuses any path that
// leaves it, so nothing can be written outside the site's own directory.
func carryExcluded(oldRoot, newRoot string, patterns []string) {
	parent := filepath.Dir(newRoot)
	if filepath.Dir(oldRoot) != parent {
		return
	}
	pr, err := os.OpenRoot(parent)
	if err != nil {
		return
	}
	defer pr.Close()
	oldBase, newBase := filepath.Base(oldRoot), filepath.Base(newRoot)
	_ = filepath.WalkDir(oldRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil || p == oldRoot {
			return nil
		}
		rel, rerr := filepath.Rel(oldRoot, p)
		if rerr != nil || !matchesExclude(filepath.ToSlash(rel), patterns) {
			return nil
		}
		dst := filepath.Join(newBase, rel)
		if _, err := pr.Lstat(dst); err == nil {
			return nil // the archive has it (a directory: descend for missing parts)
		} else if !errors.Is(err, fs.ErrNotExist) {
			// Unreachable beneath the parent (e.g. through a symlink out).
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if err := pr.MkdirAll(filepath.Dir(dst), 0o755); err == nil {
			if err := pr.Rename(filepath.Join(oldBase, rel), dst); err != nil {
				slog.Warn("agent: backup.restore could not keep excluded path", "path", rel, "error", err)
			}
		}
		if d.IsDir() {
			return filepath.SkipDir
		}
		return nil
	})
}
