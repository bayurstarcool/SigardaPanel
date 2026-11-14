package agenthttp

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/labstack/echo/v5"
	"sigardapanel/internal/siteinput"
)

// GitCommit represents a git commit.
type GitCommit struct {
	Hash    string `json:"hash"`
	Short   string `json:"short"`
	Message string `json:"message"`
	Author  string `json:"author"`
	Date    string `json:"date"`
}

// gitBranchesTask lists branches in a git repo.
func (r *Router) gitBranchesTask(c *echo.Context) error {
	ctx := c.Request().Context()
	rootPath := c.QueryParam("root_path")
	if rootPath == "" || !r.isAllowedSitePath(rootPath) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid root_path")
	}

	gitDir := filepath.Join(rootPath, ".git")
	if _, err := r.executor.Stat(ctx, gitDir); err != nil {
		return c.JSON(http.StatusOK, map[string]any{
			"ok":       false,
			"branches": []string{},
			"error":    "not a git repository",
		})
	}

	// Get current branch
	currentOut, _ := r.runGit(ctx, rootPath, "branch", "--show-current")
	current := strings.TrimSpace(currentOut)

	// List all branches
	output, err := r.runGit(ctx, rootPath, "branch", "-a", "--format", "%(refname:short)")
	if err != nil {
		return c.JSON(http.StatusOK, map[string]any{
			"ok":       false,
			"branches": []string{},
			"error":    err.Error(),
		})
	}

	var branches []string
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Skip remote tracking branches in output but keep them as options
		branches = append(branches, line)
	}

	return c.JSON(http.StatusOK, map[string]any{
		"ok":       true,
		"branches": branches,
		"current":  current,
	})
}

// gitLogTask shows commit history.
func (r *Router) gitLogTask(c *echo.Context) error {
	ctx := c.Request().Context()
	rootPath := c.QueryParam("root_path")
	lines := c.QueryParam("lines")
	if lines == "" {
		lines = "20"
	}

	if rootPath == "" || !r.isAllowedSitePath(rootPath) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid root_path")
	}

	gitDir := filepath.Join(rootPath, ".git")
	if _, err := r.executor.Stat(ctx, gitDir); err != nil {
		return c.JSON(http.StatusOK, map[string]any{
			"ok":      false,
			"commits": []GitCommit{},
			"error":   "not a git repository",
		})
	}

	// Use custom format for easy parsing
	format := "%H|%h|%s|%an|%ai"
	output, err := r.runGit(ctx, rootPath, "log", "--pretty=format:"+format, "-n", lines)
	if err != nil {
		return c.JSON(http.StatusOK, map[string]any{
			"ok":      false,
			"commits": []GitCommit{},
			"error":   err.Error(),
		})
	}

	var commits []GitCommit
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 5)
		if len(parts) < 5 {
			continue
		}
		commits = append(commits, GitCommit{
			Hash:    parts[0],
			Short:   parts[1],
			Message: parts[2],
			Author:  parts[3],
			Date:    parts[4],
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"ok":      true,
		"commits": commits,
		"total":   len(commits),
	})
}

// gitRollbackTask rolls back to a specific commit.
func (r *Router) gitRollbackTask(c *echo.Context) error {
	ctx := c.Request().Context()
	var payload struct {
		RootPath string `json:"root_path"`
		Commit   string `json:"commit"`
	}
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid payload")
	}

	if payload.RootPath == "" || !r.isAllowedSitePath(payload.RootPath) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid root_path")
	}
	if payload.Commit == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "commit hash is required")
	}
	if !isValidGitRef(payload.Commit) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid commit hash")
	}

	gitDir := filepath.Join(payload.RootPath, ".git")
	if _, err := r.executor.Stat(ctx, gitDir); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "not a git repository")
	}

	// Verify commit exists
	if _, err := r.runGit(ctx, payload.RootPath, "cat-file", "-t", payload.Commit); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid commit hash")
	}

	// Create backup branch before rollback
	branchOut, _ := r.runGit(ctx, payload.RootPath, "branch", "--show-current")
	currentBranch := strings.TrimSpace(branchOut)
	if currentBranch == "" {
		currentBranch = "main"
	}
	shortCommit := payload.Commit
	if len(shortCommit) > 8 {
		shortCommit = shortCommit[:8]
	}
	backupBranch := fmt.Sprintf("backup-before-rollback-%s", shortCommit)
	r.runGit(ctx, payload.RootPath, "branch", backupBranch, currentBranch)

	// Checkout the specific commit
	output, err := r.runGit(ctx, payload.RootPath, "checkout", payload.Commit)
	if err != nil {
		return c.JSON(http.StatusOK, map[string]any{
			"ok":      false,
			"message": fmt.Sprintf("rollback failed: %s", strings.TrimSpace(output)),
		})
	}

	// Get new HEAD info
	headOut, _ := r.runGit(ctx, payload.RootPath, "rev-parse", "--short", "HEAD")
	newHead := strings.TrimSpace(headOut)

	slog.Info("agent: git rollback", "path", payload.RootPath, "commit", payload.Commit, "backup_branch", backupBranch)
	return c.JSON(http.StatusOK, map[string]any{
		"ok":            true,
		"message":       fmt.Sprintf("rolled back to %s", payload.Commit),
		"new_head":      newHead,
		"backup_branch": backupBranch,
	})
}

// gitCheckoutTask switches to a different branch.
func (r *Router) gitCheckoutTask(c *echo.Context) error {
	ctx := c.Request().Context()
	var payload struct {
		RootPath string `json:"root_path"`
		Branch   string `json:"branch"`
	}
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid payload")
	}

	if payload.RootPath == "" || !r.isAllowedSitePath(payload.RootPath) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid root_path")
	}
	if payload.Branch == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "branch is required")
	}
	if !isValidGitRef(payload.Branch) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid branch name")
	}

	gitDir := filepath.Join(payload.RootPath, ".git")
	if _, err := r.executor.Stat(ctx, gitDir); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "not a git repository")
	}

	// Fetch latest first
	r.runGit(ctx, payload.RootPath, "fetch", "--all")

	// Checkout branch
	output, err := r.runGit(ctx, payload.RootPath, "checkout", payload.Branch)
	if err != nil {
		return c.JSON(http.StatusOK, map[string]any{
			"ok":      false,
			"message": fmt.Sprintf("checkout failed: %s", strings.TrimSpace(output)),
		})
	}

	// Pull latest changes
	pullOut, _ := r.runGit(ctx, payload.RootPath, "pull", "origin", payload.Branch)

	slog.Info("agent: git checkout", "path", payload.RootPath, "branch", payload.Branch)
	return c.JSON(http.StatusOK, map[string]any{
		"ok":      true,
		"message": fmt.Sprintf("switched to branch %s", payload.Branch),
		"pull":    strings.TrimSpace(pullOut),
	})
}

// fileOwner returns the uid/gid owning path (not following symlinks).
func fileOwner(path string) (uid, gid uint32, ok bool) {
	fi, err := os.Lstat(path)
	if err != nil {
		return 0, 0, false
	}
	st, isStat := fi.Sys().(*syscall.Stat_t)
	if !isStat {
		return 0, 0, false
	}
	return st.Uid, st.Gid, true
}

// runGit runs git inside rootPath with the privileges of whoever owns the
// repository's .git directory. A site owner controls .git/config and
// .git/hooks, so running git as root there would hand them root (post-merge,
// post-checkout hooks, core.fsmonitor, core.sshCommand ...). Root-owned
// repositories (created by earlier deploys) keep running as root because the
// site user cannot modify them.
func (r *Router) runGit(ctx context.Context, rootPath string, args ...string) (string, error) {
	full := append([]string{"-C", rootPath}, args...)
	gitDir := filepath.Join(rootPath, ".git")
	if fi, err := os.Lstat(gitDir); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("refusing to run git: .git is a symlink")
	}
	// Only a root agent can (and must) drop to the repository's owner.
	if uid, gid, ok := fileOwner(gitDir); ok && uid != 0 && os.Geteuid() == 0 {
		return r.executor.RunAsUser(ctx, uid, gid, rootPath, "git", full...)
	}
	return r.executor.RunShell(ctx, "git", full...)
}

// siteUserIDs resolves a validated, existing regular login account (UID >=
// minSiteUserUID, not nobody). Service accounts (postgres, mysql, redis,
// www-data: UIDs below 1000) are refused: a terminal or file operation as
// postgres is the PostgreSQL superuser, as www-data every site's PHP.
func siteUserIDs(name string) (uid, gid uint32, ok bool) {
	if siteinput.SystemUser(name) != nil {
		return 0, 0, false
	}
	u, err := user.Lookup(name)
	if err != nil || u.Uid == "0" {
		return 0, 0, false
	}
	a, err1 := strconv.ParseUint(u.Uid, 10, 32)
	b, err2 := strconv.ParseUint(u.Gid, 10, 32)
	if err1 != nil || err2 != nil || a < minSiteUserUID || a == 65534 {
		return 0, 0, false
	}
	if accountPrivilegedOrJail(u) != nil {
		return 0, 0, false
	}
	return uint32(a), uint32(b), true
}

// privilegedGroups give root (sudo, wheel, admin, docker, lxd) or read
// access to secrets and raw disks (adm, disk, shadow): an account in one of
// them is never a site owner, whatever its UID (cloud images ship such
// "admin"/"debian"/"ubuntu" users with passwordless sudo).
var privilegedGroups = map[string]bool{
	"root": true, "sudo": true, "wheel": true, "admin": true, "adm": true,
	"docker": true, "lxd": true, "disk": true, "shadow": true,
}

// accountPrivilegedOrJail refuses accounts the panel must never act as: members
// of privileged groups, and SFTP jail accounts (their home is the root-owned
// chroot; they share a site owner's UID and their name is unrelated to it).
func accountPrivilegedOrJail(u *user.User) error {
	if gids, err := u.GroupIds(); err == nil && !trustedSiteUser(u.Username) {
		for _, gid := range gids {
			// The account's own personal group (useradd's default: same name,
			// its primary group) is not the legacy "admin" sudo group.
			if gid == u.Gid {
				if g, err := user.LookupGroupId(gid); err == nil && g.Name == u.Username {
					continue
				}
			}
			if g, err := user.LookupGroupId(gid); err == nil && privilegedGroups[g.Name] {
				return fmt.Errorf("account %s is in the privileged group %s", u.Username, g.Name)
			}
		}
	}
	if u.HomeDir != "" {
		if fi, err := os.Lstat(u.HomeDir); err == nil {
			if st, ok := fi.Sys().(*syscall.Stat_t); ok && st.Uid == 0 && fi.IsDir() {
				return fmt.Errorf("account %s has a root-owned home (an SFTP jail account)", u.Username)
			}
		}
	}
	return nil
}

// trustedSiteUsersFile lists privileged accounts the operator allows as site
// owners (one name per line), e.g. the panel's own super admin "admin" on a
// cloud image where that Linux account has sudo. install.sh adds ADMIN_USER.
// The file must be root-owned and not writable by others.
var trustedSiteUsersFile = "/etc/sigardapanel/trusted-site-users"

func trustedSiteUser(name string) bool {
	fi, err := os.Lstat(trustedSiteUsersFile)
	if err != nil || !fi.Mode().IsRegular() || fi.Mode().Perm()&0o022 != 0 {
		return false
	}
	if st, ok := fi.Sys().(*syscall.Stat_t); !ok || st.Uid != 0 {
		return false
	}
	data, err := os.ReadFile(trustedSiteUsersFile)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == name {
			return true
		}
	}
	return false
}
