package apihttp

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"sigardapanel/internal/audit"
	"sigardapanel/internal/auth"
	"sigardapanel/internal/httpjson"
	"sigardapanel/internal/jobmodel"
	"sigardapanel/internal/store"
)

type sslHandler struct {
	store *store.Store
	audit *audit.Recorder
}

func newSSLHandler(s *store.Store, a *audit.Recorder) *sslHandler {
	return &sslHandler{store: s, audit: a}
}

var emailRegex = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

// Status returns current SSL info for a site.
func (h *sslHandler) status(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}

	site, err := h.store.Sites.GetByID(c.Request().Context(), id)
	if err != nil {
		slog.Error("api: request failed", "route", c.Path(), "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if site == nil {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}

	if !h.canAccess(c.Request().Context(), sess, site) {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}

	return c.JSON(http.StatusOK, httpjson.Success{
		Data: map[string]any{
			"site_id":        site.ID,
			"domain":         site.Domain,
			"ssl_status":     nullStringValue(site.SSLStatus),
			"ssl_issuer":     nullStringValue(site.SSLIssuer),
			"ssl_expires_at": nullTimeValue(site.SSLExpiresAt),
			"ssl_last_check": nullTimeValue(site.SSLLastCheck),
		},
		RequestID: rid,
	})
}

// Issue enqueues an SSL issue job.
func (h *sslHandler) issue(c echo.Context) error {
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

	var body struct {
		Email string `json:"email"`
	}
	_ = c.Bind(&body) // optional body

	site, err := h.store.Sites.GetByID(c.Request().Context(), id)
	if err != nil {
		slog.Error("api: request failed", "route", c.Path(), "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if site == nil {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}

	if !h.canAccess(c.Request().Context(), sess, site) {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}

	// Resolve email: body override > site owner email > current user email > fallback
	email := h.resolveSSLEmail(c.Request().Context(), body.Email, site, userID)
	if email == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "no email available for SSL certificate; set user email first")
	}

	payload := fmt.Sprintf(`{"site_id":%d,"domain":%q,"email":%q}`, site.ID, site.Domain, email)
	job := &jobmodel.Job{
		ServerID:    sql.NullInt64{Int64: site.ServerID, Valid: true},
		SiteID:      sql.NullInt64{Int64: site.ID, Valid: true},
		Type:        jobmodel.TypeSSLIssue,
		Description: fmt.Sprintf("Issue SSL certificate for %s", site.Domain),
		Status:      jobmodel.StatusQueued,
		Payload:     payload,
	}
	if err := h.store.Jobs.Create(c.Request().Context(), job); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	h.audit.RecordAction(c.Request().Context(), userID, audit.ActionSSLIssue, audit.ResourceSite, site.ID, map[string]any{
		"email": email,
	})

	h.store.NotifySiteUsers(c.Request().Context(), site.ID, "ssl_issued", "SSL Certificate Issued", fmt.Sprintf("SSL issued for %s", site.Domain), "success", "site", site.ID)

	return c.JSON(http.StatusAccepted, httpjson.Success{
		Data: map[string]any{
			"job_id": job.ID,
			"status": jobmodel.StatusQueued,
			"email":  email,
		},
		RequestID: rid,
	})
}

// resolveSSLEmail picks the best email for SSL: explicit override > owner > caller > fallback.
func (h *sslHandler) resolveSSLEmail(ctx context.Context, override string, site *store.Site, callerID int64) string {
	if override != "" && emailRegex.MatchString(override) {
		return override
	}
	// Try site owner email
	if site.OwnerID.Valid {
		owner, err := h.store.Users.GetByID(ctx, site.OwnerID.Int64)
		if err == nil && owner != nil && owner.Email != "" {
			return owner.Email
		}
	}
	// Try current user (caller) email
	if callerID > 0 {
		caller, err := h.store.Users.GetByID(ctx, callerID)
		if err == nil && caller != nil && caller.Email != "" {
			return caller.Email
		}
	}
	return ""
}

// Renew enqueues an SSL renew job.
func (h *sslHandler) renew(c echo.Context) error {
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

	site, err := h.store.Sites.GetByID(c.Request().Context(), id)
	if err != nil {
		slog.Error("api: request failed", "route", c.Path(), "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if site == nil {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}

	if !h.canAccess(c.Request().Context(), sess, site) {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}

	payload := fmt.Sprintf(`{"site_id":%d,"domain":%q}`, site.ID, site.Domain)
	job := &jobmodel.Job{
		ServerID:    sql.NullInt64{Int64: site.ServerID, Valid: true},
		SiteID:      sql.NullInt64{Int64: site.ID, Valid: true},
		Type:        jobmodel.TypeSSLRenew,
		Description: fmt.Sprintf("Renew SSL certificate for %s", site.Domain),
		Status:      jobmodel.StatusQueued,
		Payload:     payload,
	}
	if err := h.store.Jobs.Create(c.Request().Context(), job); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	h.audit.RecordAction(c.Request().Context(), userID, audit.ActionSSLRenew, audit.ResourceSite, site.ID, nil)

	return c.JSON(http.StatusAccepted, httpjson.Success{
		Data: map[string]any{
			"job_id": job.ID,
			"status": jobmodel.StatusQueued,
		},
		RequestID: rid,
	})
}

// renewAll finds all sites with SSL expiring within 30 days and queues renew jobs.
func (h *sslHandler) renewAll(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)

	const renewWindow = 30 * 24 * time.Hour
	sites, err := h.store.Sites.ListSSLEpiring(c.Request().Context(), renewWindow)
	if err != nil {
		slog.Error("api: request failed", "route", c.Path(), "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	var queued int
	var skipped int
	for _, site := range sites {
		if site.Status != "active" {
			skipped++
			continue
		}

		// Check for existing queued/running renew jobs
		queuedJobs, _ := h.store.Jobs.List(c.Request().Context(), store.JobListFilter{
			Type:   jobmodel.TypeSSLRenew,
			SiteID: site.ID,
			Status: jobmodel.StatusQueued,
		})
		runningJobs, _ := h.store.Jobs.List(c.Request().Context(), store.JobListFilter{
			Type:   jobmodel.TypeSSLRenew,
			SiteID: site.ID,
			Status: jobmodel.StatusRunning,
		})
		if len(queuedJobs) > 0 || len(runningJobs) > 0 {
			skipped++
			continue
		}

		payload := fmt.Sprintf(`{"site_id":%d,"domain":%q,"auto":true}`, site.ID, site.Domain)
		job := &jobmodel.Job{
			ServerID:    sql.NullInt64{Int64: site.ServerID, Valid: true},
			SiteID:      sql.NullInt64{Int64: site.ID, Valid: true},
			Type:        jobmodel.TypeSSLRenew,
			Status:      jobmodel.StatusQueued,
			Payload:     payload,
			MaxAttempts: 3,
		}
		if err := h.store.Jobs.Create(c.Request().Context(), job); err != nil {
			continue
		}
		queued++

		h.audit.RecordAction(c.Request().Context(), sess.UserID, audit.ActionSSLRenew, audit.ResourceSite, site.ID, map[string]any{
			"auto": true,
		})

		h.store.NotifySiteUsers(c.Request().Context(), site.ID, "ssl_auto_renew", "SSL Auto-Renewal",
			fmt.Sprintf("SSL for %s expires soon. Auto-renewal queued.", site.Domain), "warning", "site", site.ID)
	}

	return c.JSON(http.StatusOK, httpjson.Success{
		Data: map[string]any{
			"queued":  queued,
			"skipped": skipped,
			"total":   len(sites),
		},
		RequestID: rid,
	})
}

func (h *sslHandler) canAccess(ctx context.Context, sess *store.Session, site *store.Site) bool {
	if sess == nil {
		return false
	}
	user, _ := h.store.Users.GetByID(ctx, sess.UserID)
	if user == nil {
		return false
	}
	if user.Role == auth.RoleSuperAdmin {
		return true
	}
	if site.OwnerID.Valid && site.OwnerID.Int64 == sess.UserID {
		return true
	}
	hasAccess, _ := h.store.UserSites.HasAccess(ctx, sess.UserID, site.ID)
	return hasAccess
}

func nullStringValue(n sql.NullString) any {
	if !n.Valid {
		return nil
	}
	return n.String
}

func nullTimeValue(n sql.NullTime) any {
	if !n.Valid {
		return nil
	}
	return n.Time.Format(time.RFC3339)
}
