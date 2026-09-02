package agenthttp

import (
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/labstack/echo/v4"
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
func (r *Router) gitBranchesTask(c echo.Context) error {
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
	currentOut, _ := r.executor.RunShell(ctx, "git", "-C", rootPath, "branch", "--show-current")
	current := strings.TrimSpace(currentOut)

	// List all branches
	output, err := r.executor.RunShell(ctx, "git", "-C", rootPath, "branch", "-a", "--format", "%(refname:short)")
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
func (r *Router) gitLogTask(c echo.Context) error {
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
	output, err := r.executor.RunShell(ctx, "git", "-C", rootPath, "log", "--pretty=format:"+format, "-n", lines)
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
func (r *Router) gitRollbackTask(c echo.Context) error {
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
	if _, err := r.executor.RunShell(ctx, "git", "-C", payload.RootPath, "cat-file", "-t", payload.Commit); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid commit hash")
	}

	// Create backup branch before rollback
	branchOut, _ := r.executor.RunShell(ctx, "git", "-C", payload.RootPath, "branch", "--show-current")
	currentBranch := strings.TrimSpace(branchOut)
	if currentBranch == "" {
		currentBranch = "main"
	}
	shortCommit := payload.Commit
	if len(shortCommit) > 8 {
		shortCommit = shortCommit[:8]
	}
	backupBranch := fmt.Sprintf("backup-before-rollback-%s", shortCommit)
	r.executor.RunShell(ctx, "git", "-C", payload.RootPath, "branch", backupBranch, currentBranch)

	// Checkout the specific commit
	output, err := r.executor.RunShell(ctx, "git", "-C", payload.RootPath, "checkout", payload.Commit)
	if err != nil {
		return c.JSON(http.StatusOK, map[string]any{
			"ok":      false,
			"message": fmt.Sprintf("rollback failed: %s", strings.TrimSpace(output)),
		})
	}

	// Get new HEAD info
	headOut, _ := r.executor.RunShell(ctx, "git", "-C", payload.RootPath, "rev-parse", "--short", "HEAD")
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
func (r *Router) gitCheckoutTask(c echo.Context) error {
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
	r.executor.RunShell(ctx, "git", "-C", payload.RootPath, "fetch", "--all")

	// Checkout branch
	output, err := r.executor.RunShell(ctx, "git", "-C", payload.RootPath, "checkout", payload.Branch)
	if err != nil {
		return c.JSON(http.StatusOK, map[string]any{
			"ok":      false,
			"message": fmt.Sprintf("checkout failed: %s", strings.TrimSpace(output)),
		})
	}

	// Pull latest changes
	pullOut, _ := r.executor.RunShell(ctx, "git", "-C", payload.RootPath, "pull", "origin", payload.Branch)

	slog.Info("agent: git checkout", "path", payload.RootPath, "branch", payload.Branch)
	return c.JSON(http.StatusOK, map[string]any{
		"ok":      true,
		"message": fmt.Sprintf("switched to branch %s", payload.Branch),
		"pull":    strings.TrimSpace(pullOut),
	})
}
