package apihttp

import (
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v5"

	"sigardapanel/internal/audit"
	"sigardapanel/internal/httpjson"
	"sigardapanel/internal/jobmodel"
	"sigardapanel/internal/store"
)

// suspend takes a site offline (vhost + certbot renewal disabled) without
// deleting files, databases or certificates. Admin only: a suspended owner
// must not be able to resume their own site.
func (h *siteHandler) suspend(c *echo.Context) error {
	return h.toggleSuspend(c, true)
}

// resume reverses suspend.
func (h *siteHandler) resume(c *echo.Context) error {
	return h.toggleSuspend(c, false)
}

func (h *siteHandler) toggleSuspend(c *echo.Context, suspend bool) error {
	ctx := c.Request().Context()
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
	site, err := h.store.Sites.GetByID(ctx, id)
	if err != nil {
		slog.Error("api: request failed", "route", c.Path(), "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if site == nil || !h.checkSitePermission(ctx, sess, site, "admin") {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}

	jobType, verb, action := jobmodel.TypeSiteResume, "Resume", "site_resume"
	if suspend {
		jobType, verb, action = jobmodel.TypeSiteSuspend, "Suspend", "site_suspend"
		if site.Status != "active" {
			return echo.NewHTTPError(http.StatusConflict, fmt.Sprintf("only active sites can be suspended (status is %q)", site.Status))
		}
	} else if site.Status != "suspended" {
		return echo.NewHTTPError(http.StatusConflict, fmt.Sprintf("site is not suspended (status is %q)", site.Status))
	}

	// One toggle at a time per site.
	for _, t := range []string{jobmodel.TypeSiteSuspend, jobmodel.TypeSiteResume} {
		for _, st := range []string{jobmodel.StatusQueued, jobmodel.StatusRunning} {
			pending, err := h.store.Jobs.List(ctx, store.JobListFilter{Type: t, SiteID: site.ID, Status: st, Limit: 1})
			if err == nil && len(pending) > 0 {
				return echo.NewHTTPError(http.StatusConflict, "a suspend/resume job is already in progress for this site")
			}
		}
	}

	job := &jobmodel.Job{
		ServerID:    sql.NullInt64{Int64: site.ServerID, Valid: true},
		SiteID:      sql.NullInt64{Int64: site.ID, Valid: true},
		Type:        jobType,
		Description: fmt.Sprintf("%s site %s", verb, site.Domain),
		Status:      jobmodel.StatusQueued,
		Payload:     fmt.Sprintf(`{"site_id":%d,"domain":%q}`, site.ID, site.Domain),
		MaxAttempts: 1,
	}
	if err := h.store.Jobs.Create(ctx, job); err != nil {
		slog.Error("api: enqueue site toggle", "site_id", site.ID, "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	h.audit.RecordAction(ctx, userID, action, audit.ResourceSite, site.ID, nil)

	return c.JSON(http.StatusAccepted, httpjson.Success{
		Data: map[string]any{
			"job_id": job.ID,
			"status": jobmodel.StatusQueued,
		},
		RequestID: rid,
	})
}
