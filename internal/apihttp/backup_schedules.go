package apihttp

// DEPRECATED: This file implements the legacy per-site backup schedule API.
// Use backup_configs.go (BackupConfigHandler) instead.
// These routes are kept for backward compatibility and will be removed in
// a future release. All new backup scheduling should go through
// /api/v1/backup-configs.

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"sigardapanel/internal/audit"
	"sigardapanel/internal/httpjson"
	"sigardapanel/internal/store"
)

type backupScheduleHandler struct {
	store *store.Store
	audit *audit.Recorder
}

func newBackupScheduleHandler(s *store.Store, a *audit.Recorder) *backupScheduleHandler {
	return &backupScheduleHandler{store: s, audit: a}
}

// Deprecated: POST /api/v1/sites/:id/schedules
// Use POST /api/v1/backup-configs instead.
func (h *backupScheduleHandler) create(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)
	var userID int64
	if sess != nil {
		userID = sess.UserID
	}

	siteID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || siteID <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid site id")
	}

	site, err := h.store.Sites.GetByID(c.Request().Context(), siteID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if site == nil {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}

	var body struct {
		CronExpr   string `json:"cron_expr"`
		ProviderID *int64 `json:"provider_id"`
		BackupType string `json:"backup_type"`
		Enabled    *bool  `json:"enabled"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	body.CronExpr = strings.TrimSpace(body.CronExpr)
	body.BackupType = strings.TrimSpace(strings.ToLower(body.BackupType))

	if body.CronExpr == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "cron_expr required")
	}
	if body.BackupType == "" {
		body.BackupType = "files"
	}
	switch body.BackupType {
	case "files", "database", "both":
	default:
		return echo.NewHTTPError(http.StatusBadRequest, "invalid backup_type: must be files, database, or both")
	}

	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}

	sched := &store.BackupSchedule{
		SiteID:     siteID,
		CronExpr:   body.CronExpr,
		BackupType: body.BackupType,
		Enabled:    enabled,
	}
	if body.ProviderID != nil && *body.ProviderID > 0 {
		sched.ProviderID = sql.NullInt64{Int64: *body.ProviderID, Valid: true}
	}

	if err := h.store.BackupSchedules.Create(c.Request().Context(), sched); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	h.audit.RecordAction(c.Request().Context(), userID, "backup_schedule_create", "backup_schedule", sched.ID, nil)

	return c.JSON(http.StatusCreated, httpjson.Success{
		Data:      backupScheduleToMap(*sched),
		RequestID: rid,
	})
}

// Deprecated: GET /api/v1/sites/:id/schedules
// Use GET /api/v1/backup-configs instead.
func (h *backupScheduleHandler) listBySite(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)

	siteID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || siteID <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid site id")
	}

	schedules, err := h.store.BackupSchedules.ListBySite(c.Request().Context(), siteID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	out := make([]map[string]any, 0, len(schedules))
	for _, s := range schedules {
		out = append(out, backupScheduleToMap(s))
	}

	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      map[string]any{"schedules": out, "count": len(out)},
		RequestID: rid,
	})
}

// Deprecated: PATCH /api/v1/schedules/:id
// Use PATCH /api/v1/backup-configs/:id instead.
func (h *backupScheduleHandler) update(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)
	var userID int64
	if sess != nil {
		userID = sess.UserID
	}

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}

	existing, err := h.store.BackupSchedules.GetByID(c.Request().Context(), id)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if existing == nil {
		return echo.NewHTTPError(http.StatusNotFound, "schedule not found")
	}

	var body struct {
		CronExpr   *string `json:"cron_expr"`
		ProviderID *int64  `json:"provider_id"`
		BackupType *string `json:"backup_type"`
		Enabled    *bool   `json:"enabled"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	if body.CronExpr != nil {
		existing.CronExpr = strings.TrimSpace(*body.CronExpr)
	}
	if body.ProviderID != nil {
		if *body.ProviderID > 0 {
			existing.ProviderID = sql.NullInt64{Int64: *body.ProviderID, Valid: true}
		} else {
			existing.ProviderID = sql.NullInt64{Valid: false}
		}
	}
	if body.BackupType != nil {
		bt := strings.TrimSpace(strings.ToLower(*body.BackupType))
		switch bt {
		case "files", "database", "both":
			existing.BackupType = bt
		default:
			return echo.NewHTTPError(http.StatusBadRequest, "invalid backup_type")
		}
	}
	if body.Enabled != nil {
		existing.Enabled = *body.Enabled
	}

	if err := h.store.BackupSchedules.Update(c.Request().Context(), existing); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	h.audit.RecordAction(c.Request().Context(), userID, "backup_schedule_update", "backup_schedule", id, nil)

	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      backupScheduleToMap(*existing),
		RequestID: rid,
	})
}

// Deprecated: DELETE /api/v1/schedules/:id
// Use DELETE /api/v1/backup-configs/:id instead.
func (h *backupScheduleHandler) delete(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)
	var userID int64
	if sess != nil {
		userID = sess.UserID
	}

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}

	existing, err := h.store.BackupSchedules.GetByID(c.Request().Context(), id)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if existing == nil {
		return echo.NewHTTPError(http.StatusNotFound, "schedule not found")
	}

	if err := h.store.BackupSchedules.Delete(c.Request().Context(), id); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	h.audit.RecordAction(c.Request().Context(), userID, "backup_schedule_delete", "backup_schedule", id, nil)

	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      map[string]any{"deleted": true},
		RequestID: rid,
	})
}

func backupScheduleToMap(s store.BackupSchedule) map[string]any {
	m := map[string]any{
		"id":          s.ID,
		"site_id":     s.SiteID,
		"cron_expr":   s.CronExpr,
		"backup_type": s.BackupType,
		"enabled":     s.Enabled,
		"created_at":  s.CreatedAt,
		"updated_at":  s.UpdatedAt,
	}
	if s.ProviderID.Valid {
		m["provider_id"] = s.ProviderID.Int64
	}
	if s.LastRunAt.Valid {
		m["last_run_at"] = s.LastRunAt.Time
	}
	if s.NextRunAt.Valid {
		m["next_run_at"] = s.NextRunAt.Time
	}
	return m
}
