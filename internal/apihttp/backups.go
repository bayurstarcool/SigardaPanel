package apihttp

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"sigardapanel/internal/audit"
	"sigardapanel/internal/auth"
	"sigardapanel/internal/httpjson"
	"sigardapanel/internal/jobmodel"
	"sigardapanel/internal/store"
)

type backupHandler struct {
	store *store.Store
	audit *audit.Recorder
}

func newBackupHandler(s *store.Store, a *audit.Recorder) *backupHandler {
	return &backupHandler{store: s, audit: a}
}

// hasPendingBackup checks if a site already has a pending or running backup to prevent duplicates.
func (h *backupHandler) hasPendingBackup(ctx context.Context, siteID int64) bool {
	backups, err := h.store.Backups.ListBySite(ctx, siteID, 50, 0)
	if err != nil {
		return false
	}
	for _, b := range backups {
		if b.Status == "pending" || b.Status == "running" || b.Status == "archiving" {
			return true
		}
	}
	return false
}

func (h *backupHandler) create(c echo.Context) error {
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
		slog.Error("api: request failed", "route", c.Path(), "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if site == nil {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}
	if !h.canAccess(c.Request().Context(), sess, site) {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}

	// Prevent duplicate backups — check if one is already running
	if h.hasPendingBackup(c.Request().Context(), siteID) {
		return c.JSON(http.StatusConflict, httpjson.Success{
			Data:      map[string]any{"status": "already_running", "message": "A backup for this site is already in progress"},
			RequestID: rid,
		})
	}

	backup := &store.Backup{
		SiteID:      siteID,
		BackupType:  "files",
		Status:      "pending",
		StorageType: "local",
		Progress:    "queued",
	}
	if err := h.store.Backups.Create(c.Request().Context(), backup); err != nil {
		if errors.Is(err, store.ErrDuplicateBackup) {
			return c.JSON(http.StatusConflict, httpjson.Success{
				Data:      map[string]any{"status": "already_running", "message": "A backup for this site is already in progress"},
				RequestID: rid,
			})
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	payload := fmt.Sprintf(`{"backup_id":%d,"site_id":%d,"domain":%q,"root_path":%q}`,
		backup.ID, site.ID, site.Domain, site.RootPath)
	job := &jobmodel.Job{
		ServerID:    sql.NullInt64{Int64: site.ServerID, Valid: true},
		SiteID:      sql.NullInt64{Int64: site.ID, Valid: true},
		Type:        jobmodel.TypeBackupCreate,
		Description: fmt.Sprintf("Backup files for %s", site.Domain),
		Status:      jobmodel.StatusQueued,
		Payload:     payload,
	}
	if err := h.store.Jobs.Create(c.Request().Context(), job); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	h.audit.RecordAction(c.Request().Context(), userID, "backup_create", "backup", backup.ID, nil)

	return c.JSON(http.StatusAccepted, httpjson.Success{
		Data: map[string]any{
			"backup_id": backup.ID,
			"job_id":    job.ID,
			"status":    backup.Status,
		},
		RequestID: rid,
	})
}

func (h *backupHandler) list(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)

	var siteID int64
	// Try path param first (/sites/:id/backups), then query param (?site_id=)
	if v := c.Param("id"); v != "" {
		n, _ := strconv.ParseInt(v, 10, 64)
		siteID = n
	}
	if siteID <= 0 {
		if v := c.QueryParam("site_id"); v != "" {
			n, _ := strconv.ParseInt(v, 10, 64)
			siteID = n
		}
	}

	sess := sessionFromEcho(c)
	var backups []store.Backup
	var err error
	if siteID > 0 {
		site, siteErr := h.store.Sites.GetByID(c.Request().Context(), siteID)
		if siteErr != nil || site == nil {
			return echo.NewHTTPError(http.StatusNotFound, "site not found")
		}
		if !h.canAccess(c.Request().Context(), sess, site) {
			return echo.NewHTTPError(http.StatusNotFound, "site not found")
		}
		backups, err = h.store.Backups.ListBySite(c.Request().Context(), siteID, 100, 0)
	} else {
		backups, err = h.store.Backups.ListAll(c.Request().Context(), 100, 0)
	}
	if err != nil {
		slog.Error("api: request failed", "route", c.Path(), "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	out := make([]map[string]any, 0, len(backups))
	for _, b := range backups {
		out = append(out, backupToMap(b))
	}

	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      map[string]any{"backups": out, "count": len(out)},
		RequestID: rid,
	})
}

func (h *backupHandler) restore(c echo.Context) error {
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

	backup, err := h.store.Backups.GetByID(c.Request().Context(), id)
	if err != nil {
		slog.Error("api: request failed", "route", c.Path(), "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if backup == nil {
		return echo.NewHTTPError(http.StatusNotFound, "backup not found")
	}

	site, _ := h.store.Sites.GetByID(c.Request().Context(), backup.SiteID)
	if site == nil {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}
	if !h.canAccess(c.Request().Context(), sess, site) {
		return echo.NewHTTPError(http.StatusNotFound, "backup not found")
	}

	if backup.Status != "completed" {
		return echo.NewHTTPError(http.StatusConflict, "backup not completed (status="+backup.Status+")")
	}
	// Build payload with optional S3 config
	backupPath := backup.StoragePath.String
	if backup.S3Path.Valid && backup.S3Path.String != "" {
		backupPath = backup.S3Path.String
	}
	payloadMap := map[string]any{
		"backup_id":   backup.ID,
		"site_id":     site.ID,
		"domain":      site.Domain,
		"backup_path": backupPath,
		"root_path":   site.RootPath,
	}
	// If backup is on S3, include s3_config so agent can download first
	if strings.HasPrefix(backupPath, "s3://") && backup.StorageType != "local" {
		var provider *store.BackupStorageProvider
		if site.BackupProviderID.Valid && site.BackupProviderID.Int64 > 0 {
			provider, _ = h.store.BackupProviders.GetByID(c.Request().Context(), site.BackupProviderID.Int64)
		}
		if provider == nil {
			provider, _ = h.store.BackupProviders.GetDefault(c.Request().Context())
		}
		if provider != nil {
			payloadMap["s3_config"] = map[string]string{
				"bucket":     provider.Bucket,
				"endpoint":   provider.Endpoint,
				"region":     provider.Region,
				"access_key": provider.AccessKey,
				"secret_key": provider.SecretKey,
			}
		}
	}
	payload, _ := json.Marshal(payloadMap)
	job := &jobmodel.Job{
		ServerID: sql.NullInt64{Int64: site.ServerID, Valid: true},
		SiteID:   sql.NullInt64{Int64: site.ID, Valid: true},
		Type:     jobmodel.TypeBackupRestore,
		Status:   jobmodel.StatusQueued,
		Payload:  string(payload),
	}
	if err := h.store.Jobs.Create(c.Request().Context(), job); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	h.audit.RecordAction(c.Request().Context(), userID, "backup_restore", "backup", backup.ID, nil)

	return c.JSON(http.StatusAccepted, httpjson.Success{
		Data: map[string]any{
			"backup_id": backup.ID,
			"job_id":    job.ID,
		},
		RequestID: rid,
	})
}

func (h *backupHandler) canAccess(ctx context.Context, sess *store.Session, site *store.Site) bool {
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

// ─── Bulk Create handler ──────────────────────────────────

func (h *backupHandler) bulkCreate(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)
	var userID int64
	if sess != nil {
		userID = sess.UserID
	}

	var body struct {
		SiteIDs    []int64 `json:"site_ids"`
		ProviderID int64   `json:"provider_id"`
		BackupType string  `json:"backup_type"`
	}
	if err := c.Bind(&body); err != nil || len(body.SiteIDs) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "site_ids required and must be non-empty")
	}
	// Validate backup_type
	backupType := body.BackupType
	if backupType == "" {
		backupType = "files"
	}
	if backupType != "files" && backupType != "database" && backupType != "both" {
		return echo.NewHTTPError(http.StatusBadRequest, "backup_type must be 'files', 'database', or 'both'")
	}

	// Resolve storage provider once for all sites
	storageType := "local"
	var provider *store.BackupStorageProvider
	if body.ProviderID > 0 {
		var err error
		provider, err = h.store.BackupProviders.GetByID(c.Request().Context(), body.ProviderID)
		if err != nil {
			slog.Error("api: request failed", "route", c.Path(), "error", err)
			return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
		}
		if provider == nil {
			return echo.NewHTTPError(http.StatusBadRequest, "storage provider not found")
		}
		storageType = provider.ProviderType
	} else {
		provider, _ = h.store.BackupProviders.GetDefault(c.Request().Context())
		if provider != nil {
			storageType = provider.ProviderType
		}
	}

	type bulkResult struct {
		SiteID  int64            `json:"site_id"`
		Backups []map[string]any `json:"backups,omitempty"`
		Error   string           `json:"error,omitempty"`
	}

	var results []bulkResult
	var createdCount, failedCount int

	for _, siteID := range body.SiteIDs {
		if siteID <= 0 {
			results = append(results, bulkResult{SiteID: siteID, Error: "invalid site id"})
			failedCount++
			continue
		}

		site, err := h.store.Sites.GetByID(c.Request().Context(), siteID)
		if err != nil {
			results = append(results, bulkResult{SiteID: siteID, Error: "internal error"})
			failedCount++
			continue
		}
		if site == nil {
			results = append(results, bulkResult{SiteID: siteID, Error: "site not found"})
			failedCount++
			continue
		}
		if !h.canAccess(c.Request().Context(), sess, site) {
			results = append(results, bulkResult{SiteID: siteID, Error: "access denied"})
			failedCount++
			continue
		}

		var siteBackups []map[string]any

		// Files backup
		if backupType == "files" || backupType == "both" {
			backup := &store.Backup{
				SiteID:      siteID,
				Status:      "pending",
				BackupType:  "files",
				StorageType: storageType,
				Progress:    "queued",
			}
			if err := h.store.Backups.Create(c.Request().Context(), backup); err != nil {
				results = append(results, bulkResult{SiteID: siteID, Error: "failed to create backup record"})
				failedCount++
				continue
			}

			payloadMap := map[string]any{
				"backup_id": backup.ID,
				"site_id":   site.ID,
				"domain":    site.Domain,
				"root_path": site.RootPath,
				"storage":   storageType,
				"failover":  false,
			}
			if provider != nil {
				payloadMap["s3_config"] = map[string]string{
					"bucket":     provider.Bucket,
					"endpoint":   provider.Endpoint,
					"region":     provider.Region,
					"access_key": provider.AccessKey,
					"secret_key": provider.SecretKey,
				}
			}
			payload, _ := json.Marshal(payloadMap)
			job := &jobmodel.Job{
				ServerID:    sql.NullInt64{Int64: site.ServerID, Valid: true},
				SiteID:      sql.NullInt64{Int64: site.ID, Valid: true},
				Type:        jobmodel.TypeBackupCreate,
				Description: fmt.Sprintf("Backup files for %s", site.Domain),
				Status:      jobmodel.StatusQueued,
				Payload:     string(payload),
			}
			if err := h.store.Jobs.Create(c.Request().Context(), job); err != nil {
				results = append(results, bulkResult{SiteID: siteID, Error: "failed to create backup job"})
				failedCount++
				continue
			}

			h.audit.RecordAction(c.Request().Context(), userID, "backup_create", "backup", backup.ID, nil)
			siteBackups = append(siteBackups, map[string]any{
				"type":      "files",
				"backup_id": backup.ID,
				"job_id":    job.ID,
				"status":    backup.Status,
				"storage":   storageType,
			})
		}

		// Database backup
		if backupType == "database" || backupType == "both" {
			databases, err := h.store.Databases.ListBySite(c.Request().Context(), siteID)
			if err != nil || len(databases) == 0 {
				if backupType == "database" {
					results = append(results, bulkResult{SiteID: siteID, Error: "no databases found for site"})
					failedCount++
					continue
				}
				// For "both", skip database backups if no databases exist
			} else {
				for _, db := range databases {
					dbUser, dbPassword, err := h.store.DatabaseBackupCredentials(c.Request().Context(), db)
					if err != nil {
						siteBackups = append(siteBackups, map[string]any{
							"type":     "database",
							"database": db.Name,
							"error":    "database backup credential unavailable",
						})
						continue
					}

					dbStorageType := "database_" + storageType
					backup := &store.Backup{
						SiteID:      siteID,
						Status:      "pending",
						StorageType: dbStorageType,
						BackupType:  "database",
						Progress:    "queued",
					}
					if err := h.store.Backups.Create(c.Request().Context(), backup); err != nil {
						siteBackups = append(siteBackups, map[string]any{
							"type":     "database",
							"database": db.Name,
							"error":    "failed to create backup record",
						})
						continue
					}

					payloadMap := map[string]any{
						"backup_id":   backup.ID,
						"site_id":     site.ID,
						"domain":      site.Domain,
						"root_path":   site.RootPath,
						"db_name":     db.Name,
						"db_type":     db.Type,
						"db_user":     dbUser,
						"db_password": dbPassword,
					}
					if provider != nil {
						payloadMap["s3_config"] = map[string]string{
							"bucket":     provider.Bucket,
							"endpoint":   provider.Endpoint,
							"region":     provider.Region,
							"access_key": provider.AccessKey,
							"secret_key": provider.SecretKey,
						}
					}
					payload, _ := json.Marshal(payloadMap)
					job := &jobmodel.Job{
						ServerID:    sql.NullInt64{Int64: site.ServerID, Valid: true},
						SiteID:      sql.NullInt64{Int64: site.ID, Valid: true},
						Type:        jobmodel.TypeDBBackupCreate,
						Description: fmt.Sprintf("Backup database %s", site.Domain),
						Status:      jobmodel.StatusQueued,
						Payload:     string(payload),
					}
					if err := h.store.Jobs.Create(c.Request().Context(), job); err != nil {
						siteBackups = append(siteBackups, map[string]any{
							"type":     "database",
							"database": db.Name,
							"error":    "failed to create backup job",
						})
						continue
					}

					h.audit.RecordAction(c.Request().Context(), userID, "db_backup_create", "backup", backup.ID, nil)
					siteBackups = append(siteBackups, map[string]any{
						"type":      "database",
						"database":  db.Name,
						"backup_id": backup.ID,
						"job_id":    job.ID,
						"status":    backup.Status,
					})
				}
			}
		}

		createdCount++
		results = append(results, bulkResult{SiteID: siteID, Backups: siteBackups})
	}

	return c.JSON(http.StatusAccepted, httpjson.Success{
		Data: map[string]any{
			"total":   len(body.SiteIDs),
			"created": createdCount,
			"failed":  failedCount,
			"backups": results,
		},
		RequestID: rid,
	})
}

func backupToMap(b store.Backup) map[string]any {
	siteInfo := map[string]string{
		"name":   b.SiteName,
		"domain": b.SiteDomain,
	}
	m := map[string]any{
		"id":           b.ID,
		"site_id":      b.SiteID,
		"site":         siteInfo,
		"status":       b.Status,
		"backup_type":  b.BackupType,
		"storage_type": b.StorageType,
		"size_bytes":   b.SizeBytes,
		"file_count":   b.FileCount,
		"created_at":   b.CreatedAt,
		"progress":     b.Progress,
	}
	if b.StoragePath.Valid {
		m["storage_path"] = b.StoragePath.String
	}
	if b.S3Path.Valid && b.S3Path.String != "" {
		m["s3_path"] = b.S3Path.String
	}
	if b.CompletedAt.Valid {
		m["completed_at"] = b.CompletedAt.Time
	}
	if b.Error.Valid {
		m["error"] = b.Error.String
	}
	return m
}

// ─── Download handler ──────────────────────────────────────

func (h *backupHandler) download(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}

	backup, err := h.store.Backups.GetByID(c.Request().Context(), id)
	if err != nil {
		slog.Error("api: request failed", "route", c.Path(), "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if backup == nil {
		return echo.NewHTTPError(http.StatusNotFound, "backup not found")
	}

	site, _ := h.store.Sites.GetByID(c.Request().Context(), backup.SiteID)
	if site == nil {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}
	if !h.canAccess(c.Request().Context(), sess, site) {
		return echo.NewHTTPError(http.StatusNotFound, "backup not found")
	}

	if backup.Status != "completed" {
		return echo.NewHTTPError(http.StatusConflict, "backup not completed (status="+backup.Status+")")
	}

	if !backup.StoragePath.Valid || backup.StoragePath.String == "" {
		// Fall back to s3_path if storage_path is empty
		if !backup.S3Path.Valid || backup.S3Path.String == "" {
			return echo.NewHTTPError(http.StatusNotFound, "backup file not available")
		}
	}

	path := backup.StoragePath.String
	if path == "" && backup.S3Path.Valid {
		path = backup.S3Path.String
	}

	// S3 path — return S3 config for frontend to construct download URL
	if strings.HasPrefix(path, "s3://") {
		var provider *store.BackupStorageProvider
		if site.BackupProviderID.Valid && site.BackupProviderID.Int64 > 0 {
			provider, _ = h.store.BackupProviders.GetByID(c.Request().Context(), site.BackupProviderID.Int64)
		}
		if provider == nil {
			provider, _ = h.store.BackupProviders.GetDefault(c.Request().Context())
		}
		if provider == nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "S3 provider not configured")
		}
		return c.JSON(http.StatusOK, httpjson.Success{
			Data: map[string]any{
				"type":      "s3",
				"s3_path":   path,
				"bucket":    provider.Bucket,
				"endpoint":  provider.Endpoint,
				"region":    provider.Region,
				"file_name": path[strings.LastIndex(path, "/")+1:],
			},
			RequestID: rid,
		})
	}

	// Local path — serve file
	f, err := os.Open(path)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "backup file not found on disk")
	}
	defer f.Close()

	finfo, err := f.Stat()
	if err != nil || finfo.IsDir() {
		return echo.NewHTTPError(http.StatusNotFound, "backup file not found on disk")
	}

	fileName := path[strings.LastIndex(path, "/")+1:]
	c.Response().Header().Set("Content-Disposition", "attachment; filename="+fileName)
	c.Response().Header().Set("Content-Type", "application/octet-stream")
	c.Response().Header().Set("Content-Length", strconv.FormatInt(finfo.Size(), 10))
	return c.Stream(http.StatusOK, "application/octet-stream", f)
}

// ─── Delete handler ────────────────────────────────────────

func (h *backupHandler) delete(c echo.Context) error {
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

	backup, err := h.store.Backups.GetByID(c.Request().Context(), id)
	if err != nil {
		slog.Error("api: request failed", "route", c.Path(), "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if backup == nil {
		return echo.NewHTTPError(http.StatusNotFound, "backup not found")
	}

	site, _ := h.store.Sites.GetByID(c.Request().Context(), backup.SiteID)
	if site == nil {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}
	if !h.canAccess(c.Request().Context(), sess, site) {
		return echo.NewHTTPError(http.StatusNotFound, "backup not found")
	}

	// Clean up S3 object if the backup was stored on S3. Use the aws CLI to
	// stay consistent with the upload/list paths (previously rclone was used
	// here, which took incompatible flags and silently failed to delete).
	if backup.S3Path.Valid && backup.S3Path.String != "" {
		s3Path := backup.S3Path.String
		provider, _ := h.store.BackupProviders.GetDefault(c.Request().Context())
		if provider != nil {
			endpoint, region, nerr := normalizeProviderEndpointRegion(provider.ProviderType, provider.Endpoint, provider.Region)
			if nerr != nil {
				slog.Warn("backup: S3 object orphaned — invalid provider config", "s3_path", s3Path, "error", nerr)
			} else if _, lookErr := exec.LookPath("aws"); lookErr != nil {
				slog.Warn("backup: S3 object orphaned — aws CLI not installed", "s3_path", s3Path)
			} else {
				cmd := exec.CommandContext(c.Request().Context(), "aws", "s3", "rm", s3Path,
					"--endpoint-url", "https://"+endpoint, "--region", region)
				cmd.Env = append(os.Environ(),
					"AWS_ACCESS_KEY_ID="+provider.AccessKey,
					"AWS_SECRET_ACCESS_KEY="+provider.SecretKey,
				)
				if out, err := cmd.CombinedOutput(); err != nil {
					slog.Warn("backup: failed to delete S3 object", "s3_path", s3Path, "error", err, "output", strings.TrimSpace(string(out)))
				} else {
					slog.Info("backup: deleted S3 object", "s3_path", s3Path)
				}
			}
		} else {
			slog.Warn("backup: S3 object orphaned — no storage provider configured", "s3_path", s3Path)
		}
	}

	// Remove the local file if it exists (best-effort). Note: for backups whose
	// file lives on a remote agent, panel-side removal cannot reach it; the
	// retention worker dispatches backup.delete jobs for those cases.
	if backup.StoragePath.Valid && backup.StoragePath.String != "" && !strings.HasPrefix(backup.StoragePath.String, "s3://") {
		os.Remove(backup.StoragePath.String)
	}

	if err := h.store.Backups.Delete(c.Request().Context(), id); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	h.audit.RecordAction(c.Request().Context(), userID, "backup_delete", "backup", id, nil)

	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      map[string]any{"deleted": true},
		RequestID: rid,
	})
}

// ─── Database Backup handler ───────────────────────────────

func (h *backupHandler) createDatabaseBackup(c echo.Context) error {
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
		slog.Error("api: request failed", "route", c.Path(), "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if site == nil {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}
	if !h.canAccess(c.Request().Context(), sess, site) {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}

	var body struct {
		DatabaseID        int64 `json:"database_id"`
		StorageProviderID int64 `json:"storage_provider_id"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	// Determine which databases to backup
	var targetDatabases []struct {
		DB     *store.Database
		DBUser *store.DBUser
	}

	if body.DatabaseID > 0 {
		// Backup a specific database
		db, err := h.store.Databases.GetByID(c.Request().Context(), body.DatabaseID)
		if err != nil {
			slog.Error("api: request failed", "route", c.Path(), "error", err)
			return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
		}
		if db == nil {
			return echo.NewHTTPError(http.StatusNotFound, "database not found")
		}
		if db.SiteID != siteID {
			return echo.NewHTTPError(http.StatusBadRequest, "database does not belong to this site")
		}

		targetDatabases = append(targetDatabases, struct {
			DB     *store.Database
			DBUser *store.DBUser
		}{db, &store.DBUser{DatabaseID: db.ID}})
	} else {
		// Backup ALL databases for this site
		allDBs, err := h.store.Databases.ListBySite(c.Request().Context(), siteID)
		if err != nil {
			slog.Error("api: request failed", "route", c.Path(), "error", err)
			return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
		}
		if len(allDBs) == 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "no databases found for this site")
		}
		for i := range allDBs {
			db := &allDBs[i]
			targetDatabases = append(targetDatabases, struct {
				DB     *store.Database
				DBUser *store.DBUser
			}{db, &store.DBUser{DatabaseID: db.ID}})
		}
		if len(targetDatabases) == 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "no databases with users found for this site")
		}
	}

	// Prevent duplicate backups — skip databases that already have pending/running backups
	var filteredTargets []struct {
		DB     *store.Database
		DBUser *store.DBUser
	}
	for _, td := range targetDatabases {
		// Check if this database already has a pending backup
		hasPending := false
		existing, _ := h.store.Backups.ListBySite(c.Request().Context(), siteID, 10, 0)
		for _, eb := range existing {
			if eb.StoragePath.Valid && eb.StoragePath.String == td.DB.Name && (eb.Status == "pending" || eb.Status == "running" || eb.Status == "archiving") {
				hasPending = true
				break
			}
		}
		if !hasPending {
			filteredTargets = append(filteredTargets, td)
		}
	}
	if len(filteredTargets) == 0 {
		return c.JSON(http.StatusConflict, httpjson.Success{
			Data:      map[string]any{"status": "already_running", "message": "All databases already have a backup in progress"},
			RequestID: rid,
		})
	}
	targetDatabases = filteredTargets

	// Process each database backup
	var jobIDs []int64
	var backupIDs []int64
	for _, td := range targetDatabases {
		db := td.DB
		exists, err := h.store.BackupDatabaseExists(c.Request().Context(), *db)
		if err != nil || !exists {
			continue
		}
		dbUserName, dbPassword, err := h.store.DatabaseBackupCredentials(c.Request().Context(), *db)
		if err != nil {
			continue
		}

		// Determine storage type/provider
		storageType := "local"
		var provider *store.BackupStorageProvider

		if body.StorageProviderID > 0 {
			provider, err = h.store.BackupProviders.GetByID(c.Request().Context(), body.StorageProviderID)
			if err == nil && provider != nil {
				storageType = provider.ProviderType
			}
		} else if site.BackupProviderID.Valid && site.BackupProviderID.Int64 > 0 {
			provider, _ = h.store.BackupProviders.GetByID(c.Request().Context(), site.BackupProviderID.Int64)
			if provider != nil {
				storageType = provider.ProviderType
			}
		} else {
			provider, _ = h.store.BackupProviders.GetDefault(c.Request().Context())
			if provider != nil {
				storageType = provider.ProviderType
			}
		}

		backup := &store.Backup{
			SiteID:      siteID,
			Status:      "pending",
			BackupType:  "database",
			StorageType: storageType,
			StoragePath: sql.NullString{String: db.Name, Valid: true},
			Progress:    "queued",
		}
		if err := h.store.Backups.Create(c.Request().Context(), backup); err != nil {
			continue
		}

		var s3Config string
		if provider != nil {
			s3Config = fmt.Sprintf(`,"s3_config":{"bucket":%q,"endpoint":%q,"region":%q,"access_key":%q,"secret_key":%q}`,
				provider.Bucket, provider.Endpoint, provider.Region, provider.AccessKey, provider.SecretKey)
		}

		payload := fmt.Sprintf(`{"backup_id":%d,"site_id":%d,"domain":%q,"root_path":%q,"db_name":%q,"db_type":%q,"db_user":%q,"db_password":%q%s}`,
			backup.ID, site.ID, site.Domain, site.RootPath, db.Name, db.Type, dbUserName, dbPassword, s3Config)

		job := &jobmodel.Job{
			ServerID:    sql.NullInt64{Int64: site.ServerID, Valid: true},
			SiteID:      sql.NullInt64{Int64: site.ID, Valid: true},
			Type:        jobmodel.TypeDBBackupCreate,
			Description: fmt.Sprintf("Backup database for %s", site.Domain),
			Status:      jobmodel.StatusQueued,
			Payload:     payload,
		}
		if err := h.store.Jobs.Create(c.Request().Context(), job); err != nil {
			continue
		}
		jobIDs = append(jobIDs, job.ID)
		backupIDs = append(backupIDs, backup.ID)
	}

	h.audit.RecordAction(c.Request().Context(), userID, "database_backup_create", "backup", siteID, nil)

	msg := fmt.Sprintf("Database backup started for %d database(s)", len(targetDatabases))
	if len(targetDatabases) == 1 {
		msg = fmt.Sprintf("Database backup started for %s", targetDatabases[0].DB.Name)
	}

	return c.JSON(http.StatusAccepted, httpjson.Success{
		Data: map[string]any{
			"backup_ids": backupIDs,
			"job_ids":    jobIDs,
			"status":     "pending",
			"message":    msg,
			"count":      len(targetDatabases),
		},
		RequestID: rid,
	})
}

// ─── Retention Settings Handlers ───────────────────────────

func getRetention(db *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		rid := c.Response().Header().Get(echo.HeaderXRequestID)
		val, err := store.GetSetting(c.Request().Context(), db, "retention_days")
		if err != nil || val == "" || val == "0" {
			val = "30"
		}
		var days int
		fmt.Sscanf(val, "%d", &days)
		val2, err2 := store.GetSetting(c.Request().Context(), db, "keep_last_per_site")
		var keepLast int
		if err2 == nil && val2 != "" {
			fmt.Sscanf(val2, "%d", &keepLast)
		}
		if keepLast <= 0 {
			keepLast = 3
		}
		return c.JSON(http.StatusOK, httpjson.Success{
			Data:      map[string]any{"retention_days": days, "keep_last_per_site": keepLast},
			RequestID: rid,
		})
	}
}

func setRetention(db *sql.DB) echo.HandlerFunc {
	return func(c echo.Context) error {
		rid := c.Response().Header().Get(echo.HeaderXRequestID)
		var body struct {
			RetentionDays   int `json:"retention_days"`
			KeepLastPerSite int `json:"keep_last_per_site"`
		}
		if err := c.Bind(&body); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
		}
		if body.RetentionDays != 0 && body.RetentionDays < 7 {
			return echo.NewHTTPError(http.StatusBadRequest, "retention_days must be 0 (disabled) or at least 7 days")
		}
		if body.RetentionDays > 3650 {
			return echo.NewHTTPError(http.StatusBadRequest, "retention_days must be <= 3650")
		}
		val := strconv.Itoa(body.RetentionDays)
		if err := store.SetSetting(c.Request().Context(), db, "retention_days", val); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to save setting")
		}
		if body.KeepLastPerSite < 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "keep_last_per_site must be >= 0")
		}
		if body.KeepLastPerSite > 0 {
			if err := store.SetSetting(c.Request().Context(), db, "keep_last_per_site", strconv.Itoa(body.KeepLastPerSite)); err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, "failed to save setting")
			}
		}
		// Read back current value for response
		if body.KeepLastPerSite <= 0 {
			v, _ := store.GetSetting(c.Request().Context(), db, "keep_last_per_site")
			fmt.Sscanf(v, "%d", &body.KeepLastPerSite)
			if body.KeepLastPerSite <= 0 {
				body.KeepLastPerSite = 3
			}
		}
		return c.JSON(http.StatusOK, httpjson.Success{
			Data:      map[string]any{"retention_days": body.RetentionDays, "keep_last_per_site": body.KeepLastPerSite},
			RequestID: rid,
		})
	}
}
