package apihttp

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/labstack/echo/v4"
	"net/http"
	"sigardapanel/internal/jobmodel"
	"sigardapanel/internal/store"
	"strconv"
)

// appStart enqueues an app.start job for the site.
func (h *siteHandler) appStart(c echo.Context) error {
	site, err := h.getSiteForUserPerm(c, sitePermWrite)
	if err != nil {
		return err
	}
	if site.Runtime == "static" || site.Runtime == "php" {
		return echo.NewHTTPError(http.StatusBadRequest, "runtime does not support app process management")
	}
	if !site.AppPort.Valid || site.AppPort.Int64 == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "no app port configured; update app settings first")
	}

	// Pre-flight port check: ensure port is still available
	avail, reason, err := h.store.Sites.CheckPortAvailable(c.Request().Context(), site.ID, site.AppPort.Int64)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "port check failed")
	}
	if !avail {
		suggested, _ := findSuggestedPort(h, c.Request().Context(), site.AppPort.Int64)
		msg := reason
		if suggested > 0 {
			msg += fmt.Sprintf(". Suggest using port %d instead", suggested)
		}
		return echo.NewHTTPError(http.StatusConflict, msg)
	}

	var body struct {
		Command string `json:"command"`
	}
	c.Bind(&body)
	cmd := site.AppCommand.String
	if body.Command != "" {
		cmd = body.Command
		// Save updated command
		h.store.Sites.UpdateAppConfig(c.Request().Context(), site.ID, site.AppPort.Int64, cmd, "starting")
	} else {
		h.store.Sites.UpdateAppStatus(c.Request().Context(), site.ID, "starting")
	}

	memMax := site.AppMemoryMax.Int64
	if memMax <= 0 {
		memMax = 512
	}

	payload, _ := json.Marshal(map[string]any{
		"site_id":        site.ID,
		"domain":         site.Domain,
		"root_path":      site.RootPath,
		"system_user":    site.SystemUser.String,
		"app_port":       site.AppPort.Int64,
		"app_command":    cmd,
		"app_memory_max": memMax,
	})
	job := &jobmodel.Job{
		ServerID:    sql.NullInt64{Int64: site.ServerID, Valid: true},
		SiteID:      sql.NullInt64{Int64: site.ID, Valid: true},
		Type:        jobmodel.TypeAppStart,
		Description: fmt.Sprintf("Start app on %s", site.Domain),
		Payload:     string(payload),
	}
	if err := h.store.Jobs.Create(c.Request().Context(), job); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to enqueue job")
	}
	return c.JSON(http.StatusAccepted, map[string]any{
		"data": map[string]any{"job_id": job.ID, "status": "queued", "app_status": "starting"},
	})
}

// appStop enqueues an app.stop job for the site.
func (h *siteHandler) appStop(c echo.Context) error {
	site, err := h.getSiteForUserPerm(c, sitePermWrite)
	if err != nil {
		return err
	}
	if site.Runtime == "static" || site.Runtime == "php" {
		return echo.NewHTTPError(http.StatusBadRequest, "runtime does not support app process management")
	}

	h.store.Sites.UpdateAppStatus(c.Request().Context(), site.ID, "stopping")

	payload, _ := json.Marshal(map[string]any{
		"site_id": site.ID,
		"domain":  site.Domain,
	})
	job := &jobmodel.Job{
		ServerID:    sql.NullInt64{Int64: site.ServerID, Valid: true},
		SiteID:      sql.NullInt64{Int64: site.ID, Valid: true},
		Type:        jobmodel.TypeAppStop,
		Description: fmt.Sprintf("Stop app on %s", site.Domain),
		Payload:     string(payload),
	}
	if err := h.store.Jobs.Create(c.Request().Context(), job); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to enqueue job")
	}
	return c.JSON(http.StatusAccepted, map[string]any{
		"data": map[string]any{"job_id": job.ID, "status": "queued", "app_status": "stopping"},
	})
}

// appRestart enqueues an app.restart job for the site.
func (h *siteHandler) appRestart(c echo.Context) error {
	site, err := h.getSiteForUserPerm(c, sitePermWrite)
	if err != nil {
		return err
	}
	if site.Runtime == "static" || site.Runtime == "php" {
		return echo.NewHTTPError(http.StatusBadRequest, "runtime does not support app process management")
	}

	// Pre-flight port check
	avail, reason, err := h.store.Sites.CheckPortAvailable(c.Request().Context(), site.ID, site.AppPort.Int64)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "port check failed")
	}
	if !avail {
		suggested, _ := findSuggestedPort(h, c.Request().Context(), site.AppPort.Int64)
		msg := reason
		if suggested > 0 {
			msg += fmt.Sprintf(". Suggest using port %d instead", suggested)
		}
		return echo.NewHTTPError(http.StatusConflict, msg)
	}

	h.store.Sites.UpdateAppStatus(c.Request().Context(), site.ID, "restarting")

	payload, _ := json.Marshal(map[string]any{
		"site_id":        site.ID,
		"domain":         site.Domain,
		"root_path":      site.RootPath,
		"system_user":    site.SystemUser.String,
		"app_port":       site.AppPort.Int64,
		"app_command":    site.AppCommand.String,
		"app_memory_max": appMemoryOrDefault(site.AppMemoryMax.Int64, 512),
	})
	job := &jobmodel.Job{
		ServerID:    sql.NullInt64{Int64: site.ServerID, Valid: true},
		SiteID:      sql.NullInt64{Int64: site.ID, Valid: true},
		Type:        jobmodel.TypeAppRestart,
		Description: fmt.Sprintf("Restart app on %s", site.Domain),
		Payload:     string(payload),
	}
	if err := h.store.Jobs.Create(c.Request().Context(), job); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to enqueue job")
	}
	return c.JSON(http.StatusAccepted, map[string]any{
		"data": map[string]any{"job_id": job.ID, "status": "queued", "app_status": "restarting"},
	})
}

// appStatus returns the current app process status.
func (h *siteHandler) appStatus(c echo.Context) error {
	site, err := h.getSiteForUser(c)
	if err != nil {
		return err
	}
	if site.Runtime == "static" || site.Runtime == "php" {
		return echo.NewHTTPError(http.StatusBadRequest, "runtime does not support app process management")
	}
	return c.JSON(http.StatusOK, map[string]any{
		"data": map[string]any{
			"app_status":     site.AppStatus.String,
			"app_port":       site.AppPort.Int64,
			"app_command":    site.AppCommand.String,
			"app_memory_max": appMemoryOrDefault(site.AppMemoryMax.Int64, 512),
		},
	})
}

// appUpdateConfig allows updating app_command and app_port.
func (h *siteHandler) appUpdateConfig(c echo.Context) error {
	site, err := h.getSiteForUserPerm(c, sitePermWrite)
	if err != nil {
		return err
	}
	if site.Runtime == "static" || site.Runtime == "php" {
		return echo.NewHTTPError(http.StatusBadRequest, "runtime does not support app process management")
	}
	var body struct {
		Command   string `json:"command"`
		Port      int64  `json:"port"`
		MemoryMax int64  `json:"memory_max_mb"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid body")
	}
	port := site.AppPort.Int64
	// Validate port: check if it's available
	if body.Port > 0 {
		avail, reason, err := h.store.Sites.CheckPortAvailable(c.Request().Context(), site.ID, body.Port)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "port check failed")
		}
		if !avail {
			// Suggest next available port
			suggested, _ := findSuggestedPort(h, c.Request().Context(), body.Port)
			msg := reason
			if suggested > 0 {
				msg += fmt.Sprintf(". Suggest using port %d instead", suggested)
			}
			return echo.NewHTTPError(http.StatusConflict, msg)
		}
		port = body.Port
	}
	cmd := site.AppCommand.String
	if body.Command != "" {
		cmd = body.Command
	}
	status := site.AppStatus.String
	if status == "" {
		status = "stopped"
	}

	// Update memory if provided
	memMax := site.AppMemoryMax.Int64
	if body.MemoryMax > 0 {
		memMax = body.MemoryMax
		h.store.Sites.UpdateAppMemory(c.Request().Context(), site.ID, memMax)
	}

	portChanged := port != site.AppPort.Int64
	if err := h.store.Sites.UpdateAppConfig(c.Request().Context(), site.ID, port, cmd, status); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to update app config")
	}

	// If the port changed, regenerate the nginx vhost so the reverse proxy targets the new port.
	if portChanged && port > 0 {
		payload, _ := json.Marshal(map[string]any{
			"site_id":   site.ID,
			"name":      site.Name,
			"domain":    site.Domain,
			"runtime":   site.Runtime,
			"root_path": site.RootPath,
			"port":      int(port),
		})
		job := &jobmodel.Job{
			ServerID: sql.NullInt64{Int64: site.ServerID, Valid: true},
			SiteID:   sql.NullInt64{Int64: site.ID, Valid: true},
			Type:     jobmodel.TypeSiteCreate,
			Payload:  string(payload),
		}
		h.store.Jobs.Create(c.Request().Context(), job)
	}

	return c.JSON(http.StatusOK, map[string]any{
		"data": map[string]any{"app_port": port, "app_command": cmd, "app_status": status, "app_memory_max": memMax},
	})
}

// findSuggestedPort finds the next available port near startPort, preferring app range 30000-40000.
func findSuggestedPort(h *siteHandler, ctx context.Context, startPort int64) (int64, error) {
	if startPort < 30000 || startPort > 40000 {
		startPort = 30000
	}
	for p := startPort + 1; p <= 40000; p++ {
		avail, _, err := h.store.Sites.CheckPortAvailable(ctx, 0, p)
		if err != nil {
			return 0, err
		}
		if avail {
			return p, nil
		}
	}
	for p := int64(30000); p < startPort; p++ {
		avail, _, err := h.store.Sites.CheckPortAvailable(ctx, 0, p)
		if err != nil {
			return 0, err
		}
		if avail {
			return p, nil
		}
	}
	return 0, fmt.Errorf("no free port in 30000-40000")
}

// appMemoryOrDefault returns the configured memory limit, or fallback if unset (<= 0).
func appMemoryOrDefault(v, fallback int64) int64 {
	if v > 0 {
		return v
	}
	return fallback
}

// getSiteForUser fetches site and verifies the caller can at least read it.
// Use it only for read-only endpoints; mutations must use getSiteForUserPerm
// with sitePermWrite (or higher).
func (h *siteHandler) getSiteForUser(c echo.Context) (*store.Site, error) {
	return h.getSiteForUserPerm(c, sitePermRead)
}

// getSiteForUserPerm fetches site and verifies the caller holds the required
// site permission (404 when the site is not visible, 403 when visible but the
// level is insufficient).
func (h *siteHandler) getSiteForUserPerm(c echo.Context, required string) (*store.Site, error) {
	siteID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusBadRequest, "invalid site id")
	}
	site, err := h.store.Sites.GetByID(c.Request().Context(), siteID)
	if err != nil || site == nil {
		return nil, echo.NewHTTPError(http.StatusNotFound, "site not found")
	}
	if err := requireSitePermission(c.Request().Context(), h.store, sessionFromEcho(c), site, required); err != nil {
		return nil, err
	}
	return site, nil
}
