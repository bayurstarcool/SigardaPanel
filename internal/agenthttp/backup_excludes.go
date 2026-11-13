package agenthttp

import (
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
func carryExcluded(oldRoot, newRoot string, patterns []string) {
	_ = filepath.WalkDir(oldRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil || p == oldRoot {
			return nil
		}
		rel, rerr := filepath.Rel(oldRoot, p)
		if rerr != nil || !matchesExclude(filepath.ToSlash(rel), patterns) {
			return nil
		}
		dst := filepath.Join(newRoot, rel)
		if _, err := os.Lstat(dst); err == nil {
			if d.IsDir() {
				return nil // descend: parts of it may be missing
			}
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err == nil {
			if err := movePath(p, dst); err != nil {
				slog.Warn("agent: backup.restore could not keep excluded path", "path", rel, "error", err)
			}
		}
		if d.IsDir() {
			return filepath.SkipDir
		}
		return nil
	})
}
