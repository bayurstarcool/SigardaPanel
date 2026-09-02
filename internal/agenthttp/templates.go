package agenthttp

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"github.com/labstack/echo/v4"
)

// WordPressTemplate handles WordPress installation via WP-CLI.
type WordPressTemplate struct {
	Domain     string `json:"domain"`
	RootPath   string `json:"root_path"`
	Title      string `json:"title"`
	AdminUser  string `json:"admin_user"`
	AdminPass  string `json:"admin_pass"`
	AdminEmail string `json:"admin_email"`
	DBName     string `json:"db_name"`
	DBUser     string `json:"db_user"`
	DBPass     string `json:"db_pass"`
	DBHost     string `json:"db_host"`
	Locale     string `json:"locale"`
}

// installWordPress installs WordPress using WP-CLI.
func (r *Router) installWordPress(c echo.Context) error {
	ctx := c.Request().Context()

	var payload WordPressTemplate
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid payload")
	}

	// Validate required fields
	if payload.Domain == "" || payload.RootPath == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "domain and root_path are required")
	}
	if payload.DBName == "" || payload.DBUser == "" || payload.DBPass == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "database credentials are required")
	}

	// Set defaults
	if payload.Title == "" {
		payload.Title = payload.Domain
	}
	if payload.AdminUser == "" {
		payload.AdminUser = "admin"
	}
	if payload.AdminEmail == "" {
		payload.AdminEmail = "admin@" + payload.Domain
	}
	if payload.DBHost == "" {
		payload.DBHost = "localhost"
	}
	if payload.Locale == "" {
		payload.Locale = "en_US"
	}

	// Check if WP-CLI is installed
	if _, err := r.executor.RunShell(ctx, "which", "wp"); err != nil {
		// Install WP-CLI
		slog.Info("wordpress: installing WP-CLI")
		if _, err := r.executor.RunShell(ctx, "bash", "-c",
			"curl -sO https://raw.githubusercontent.com/wp-cli/builds/gh-pages/phar/wp-cli.phar && chmod +x wp-cli.phar && mv wp-cli.phar /usr/local/bin/wp"); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to install WP-CLI: "+err.Error())
		}
	}

	// Create root directory if it doesn't exist
	if err := os.MkdirAll(payload.RootPath, 0755); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create root directory: "+err.Error())
	}

	// Download WordPress core
	slog.Info("wordpress: downloading core", "path", payload.RootPath)
	if _, err := r.executor.RunShell(ctx, "wp", "core", "download",
		"--path="+payload.RootPath,
		"--locale="+payload.Locale,
		"--force"); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to download WordPress: "+err.Error())
	}

	// Create wp-config.php
	slog.Info("wordpress: creating config")
	if _, err := r.executor.RunShell(ctx, "wp", "config", "create",
		"--path="+payload.RootPath,
		"--dbname="+payload.DBName,
		"--dbuser="+payload.DBUser,
		"--dbpass="+payload.DBPass,
		"--dbhost="+payload.DBHost,
		"--force"); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create wp-config: "+err.Error())
	}

	// Install WordPress
	slog.Info("wordpress: installing", "domain", payload.Domain)
	if _, err := r.executor.RunShell(ctx, "wp", "core", "install",
		"--path="+payload.RootPath,
		"--url=https://"+payload.Domain,
		"--title="+payload.Title,
		"--admin_user="+payload.AdminUser,
		"--admin_password="+payload.AdminPass,
		"--admin_email="+payload.AdminEmail,
		"--skip-email"); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to install WordPress: "+err.Error())
	}

	// Set correct permissions
	r.executor.RunShell(ctx, "chown", "-R", "www-data:www-data", payload.RootPath)
	r.executor.RunShell(ctx, "find", payload.RootPath, "-type", "d", "-exec", "chmod", "755", "{}", ";")
	r.executor.RunShell(ctx, "find", payload.RootPath, "-type", "f", "-exec", "chmod", "644", "{}", ";")

	// Install essential plugins
	slog.Info("wordpress: installing plugins")
	plugins := []string{"akismet", "wordfence", "wp-super-cache"}
	for _, plugin := range plugins {
		r.executor.RunShell(ctx, "wp", "plugin", "install", plugin,
			"--path="+payload.RootPath, "--activate")
	}

	slog.Info("wordpress: installation complete", "domain", payload.Domain)

	return c.JSON(http.StatusOK, map[string]any{
		"ok":      true,
		"message": fmt.Sprintf("WordPress installed successfully at %s", payload.Domain),
		"url":     "https://" + payload.Domain,
		"admin":   "https://" + payload.Domain + "/wp-admin",
	})
}

// installLaravel installs a Laravel project using Composer.
func (r *Router) installLaravel(c echo.Context) error {
	ctx := c.Request().Context()

	var payload struct {
		Domain      string `json:"domain"`
		RootPath    string `json:"root_path"`
		ProjectName string `json:"project_name"`
	}
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid payload")
	}

	if payload.Domain == "" || payload.RootPath == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "domain and root_path are required")
	}
	if payload.ProjectName == "" {
		payload.ProjectName = payload.Domain
	}

	// Check if Composer is installed
	if _, err := r.executor.RunShell(ctx, "which", "composer"); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Composer is not installed")
	}

	// Create Laravel project
	slog.Info("laravel: creating project", "path", payload.RootPath)
	if _, err := r.executor.RunShell(ctx, "composer", "create-project",
		"laravel/laravel", payload.RootPath, "--prefer-dist"); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create Laravel project: "+err.Error())
	}

	// Set permissions
	r.executor.RunShell(ctx, "chown", "-R", "www-data:www-data", payload.RootPath)
	r.executor.RunShell(ctx, "chmod", "-R", "775", filepath.Join(payload.RootPath, "storage"))
	r.executor.RunShell(ctx, "chmod", "-R", "775", filepath.Join(payload.RootPath, "bootstrap", "cache"))

	slog.Info("laravel: installation complete", "domain", payload.Domain)

	return c.JSON(http.StatusOK, map[string]any{
		"ok":      true,
		"message": fmt.Sprintf("Laravel project created at %s", payload.Domain),
		"url":     "https://" + payload.Domain,
	})
}
