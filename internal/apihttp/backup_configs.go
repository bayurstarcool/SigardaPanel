package apihttp

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"sigardapanel/internal/audit"
	"sigardapanel/internal/httpjson"
	"sigardapanel/internal/jobmodel"
	"sigardapanel/internal/store"
)

type backupConfigHandler struct {
	store *store.Store
	audit *audit.Recorder
}

func newBackupConfigHandler(s *store.Store, a *audit.Recorder) *backupConfigHandler {
	return &backupConfigHandler{store: s, audit: a}
}

// GET /api/v1/backup-configs
func (h *backupConfigHandler) list(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)

	configs, err := h.store.BackupConfigs.ListAll(c.Request().Context())
	if err != nil {
		slog.Error("api: request failed", "route", c.Path(), "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	out := make([]map[string]any, 0, len(configs))
	for _, cfg := range configs {
		out = append(out, backupConfigToMap(cfg))
	}

	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      map[string]any{"configs": out, "count": len(out)},
		RequestID: rid,
	})
}

// POST /api/v1/backup-configs
func (h *backupConfigHandler) create(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)
	var userID int64
	if sess != nil {
		userID = sess.UserID
	}

	var body struct {
		Name            string   `json:"name"`
		ScopeType       string   `json:"scope_type"`
		SiteIDs         []int64  `json:"site_ids"`
		ProviderID      *int64   `json:"provider_id"`
		CronExpr        string   `json:"cron_expr"`
		Enabled         *bool    `json:"enabled"`
		RetentionDays   *int     `json:"retention_days"`
		ExcludePatterns []string `json:"exclude_patterns"`
		DatabaseIDs     []int64  `json:"database_ids"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	body.ScopeType = strings.TrimSpace(strings.ToLower(body.ScopeType))
	if body.ScopeType == "" {
		body.ScopeType = "files"
	}
	switch body.ScopeType {
	case "files", "database", "both":
	default:
		return echo.NewHTTPError(http.StatusBadRequest, "invalid scope_type: must be files, database, or both")
	}

	body.CronExpr = strings.TrimSpace(body.CronExpr)
	if body.CronExpr == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "cron_expr required")
	}

	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}

	cfg := &store.BackupConfig{
		Name:            body.Name,
		ScopeType:       body.ScopeType,
		SiteIDs:         body.SiteIDs,
		CronExpr:        body.CronExpr,
		Enabled:         enabled,
		RetentionDays:   30,
		ExcludePatterns: body.ExcludePatterns,
		DatabaseIDs:     body.DatabaseIDs,
	}
	if body.RetentionDays != nil {
		cfg.RetentionDays = *body.RetentionDays
	}
	if body.ProviderID != nil && *body.ProviderID > 0 {
		cfg.ProviderID = sql.NullInt64{Int64: *body.ProviderID, Valid: true}
	}

	if err := h.store.BackupConfigs.Create(c.Request().Context(), cfg); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	h.audit.RecordAction(c.Request().Context(), userID, "backup_config_create", "backup_config", cfg.ID, nil)

	return c.JSON(http.StatusCreated, httpjson.Success{
		Data:      backupConfigToMap(*cfg),
		RequestID: rid,
	})
}

// PATCH /api/v1/backup-configs/:id
func (h *backupConfigHandler) update(c echo.Context) error {
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

	existing, err := h.store.BackupConfigs.GetByID(c.Request().Context(), id)
	if err != nil {
		slog.Error("api: request failed", "route", c.Path(), "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if existing == nil {
		return echo.NewHTTPError(http.StatusNotFound, "backup config not found")
	}

	var body struct {
		Name            *string  `json:"name"`
		ScopeType       *string  `json:"scope_type"`
		SiteIDs         []int64  `json:"site_ids"`
		ProviderID      *int64   `json:"provider_id"`
		CronExpr        *string  `json:"cron_expr"`
		Enabled         *bool    `json:"enabled"`
		RetentionDays   *int     `json:"retention_days"`
		ExcludePatterns []string `json:"exclude_patterns"`
		DatabaseIDs     []int64  `json:"database_ids"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	if body.Name != nil {
		existing.Name = *body.Name
	}
	if body.ScopeType != nil {
		bt := strings.TrimSpace(strings.ToLower(*body.ScopeType))
		switch bt {
		case "files", "database", "both":
			existing.ScopeType = bt
		default:
			return echo.NewHTTPError(http.StatusBadRequest, "invalid scope_type")
		}
	}
	if body.SiteIDs != nil {
		existing.SiteIDs = body.SiteIDs
	}
	if body.ProviderID != nil {
		if *body.ProviderID > 0 {
			existing.ProviderID = sql.NullInt64{Int64: *body.ProviderID, Valid: true}
		} else {
			existing.ProviderID = sql.NullInt64{Valid: false}
		}
	}
	if body.CronExpr != nil {
		existing.CronExpr = strings.TrimSpace(*body.CronExpr)
	}
	if body.Enabled != nil {
		existing.Enabled = *body.Enabled
	}
	if body.ExcludePatterns != nil {
		existing.ExcludePatterns = body.ExcludePatterns
	}
	if body.DatabaseIDs != nil {
		existing.DatabaseIDs = body.DatabaseIDs
	}
	if body.RetentionDays != nil {
		existing.RetentionDays = *body.RetentionDays
	}

	if err := h.store.BackupConfigs.Update(c.Request().Context(), existing); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	h.audit.RecordAction(c.Request().Context(), userID, "backup_config_update", "backup_config", id, nil)

	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      backupConfigToMap(*existing),
		RequestID: rid,
	})
}

// DELETE /api/v1/backup-configs/:id
func (h *backupConfigHandler) delete(c echo.Context) error {
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

	existing, err := h.store.BackupConfigs.GetByID(c.Request().Context(), id)
	if err != nil {
		slog.Error("api: request failed", "route", c.Path(), "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if existing == nil {
		return echo.NewHTTPError(http.StatusNotFound, "backup config not found")
	}

	if err := h.store.BackupConfigs.Delete(c.Request().Context(), id); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	h.audit.RecordAction(c.Request().Context(), userID, "backup_config_delete", "backup_config", id, nil)

	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      map[string]any{"deleted": true},
		RequestID: rid,
	})
}

// POST /api/v1/backup-configs/:id/run
func (h *backupConfigHandler) run(c echo.Context) error {
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

	cfg, err := h.store.BackupConfigs.GetByID(c.Request().Context(), id)
	if err != nil {
		slog.Error("api: request failed", "route", c.Path(), "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if cfg == nil {
		return echo.NewHTTPError(http.StatusNotFound, "backup config not found")
	}

	// Resolve storage provider
	storageType := "local"
	var provider *store.BackupStorageProvider
	if cfg.ProviderID.Valid {
		provider, _ = h.store.BackupProviders.GetByID(c.Request().Context(), cfg.ProviderID.Int64)
	} else {
		provider, _ = h.store.BackupProviders.GetDefault(c.Request().Context())
	}
	if provider != nil {
		storageType = provider.ProviderType
		slog.Info("backup config run: provider resolved", "config_id", id, "provider_id", provider.ID, "provider_type", provider.ProviderType, "bucket", provider.Bucket, "endpoint", provider.Endpoint)
	} else {
		slog.Warn("backup config run: no provider resolved, using local", "config_id", id, "provider_id_valid", cfg.ProviderID.Valid)
	}

	ctx := c.Request().Context()
	var createdFiles, createdDB int
	siteIDs, err := h.store.ResolveBackupConfigSiteIDs(ctx, *cfg)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "resolve backup config sites failed")
	}
	slog.Info("backup config run: sites resolved", "config_id", id, "scope_type", cfg.ScopeType, "site_count", len(siteIDs), "auto_discover", len(cfg.SiteIDs) == 0)

	for _, siteID := range siteIDs {
		if siteID <= 0 {
			continue
		}
		site, err := h.store.Sites.GetByID(ctx, siteID)
		if err != nil || site == nil {
			slog.Warn("backup config run: site unavailable", "config_id", id, "site_id", siteID, "error", err)
			continue
		}

		if cfg.ScopeType == "both" {
			dbPayloads := []map[string]string{}
			databases, _ := h.store.Databases.ListBySite(ctx, siteID)
			for _, db := range databases {
				if len(cfg.DatabaseIDs) > 0 && !int64InSlice(db.ID, cfg.DatabaseIDs) {
					continue
				}
				exists, err := h.store.BackupDatabaseExists(ctx, db)
				if err != nil || !exists {
					continue
				}
				dbUser, dbPassword, err := h.store.DatabaseBackupCredentials(ctx, db)
				if err != nil {
					continue
				}
				dbPayloads = append(dbPayloads, map[string]string{"name": db.Name, "type": db.Type, "user": dbUser, "password": dbPassword})
			}
			backup := &store.Backup{SiteID: siteID, Status: "pending", StorageType: "full_" + storageType, BackupType: "both"}
			if err := h.store.Backups.Create(ctx, backup); err != nil {
				continue
			}
			payloadMap := map[string]any{"backup_id": backup.ID, "site_id": site.ID, "domain": site.Domain, "root_path": site.RootPath, "storage": storageType, "backup_type": "both", "databases": dbPayloads, "scheduled": true}
			if provider != nil {
				payloadMap["s3_config"] = map[string]string{"bucket": provider.Bucket, "endpoint": provider.Endpoint, "region": provider.Region, "access_key": provider.AccessKey, "secret_key": provider.SecretKey}
			}
			payload, _ := json.Marshal(payloadMap)
			job := &jobmodel.Job{ServerID: sql.NullInt64{Int64: site.ServerID, Valid: true}, SiteID: sql.NullInt64{Int64: site.ID, Valid: true}, Type: jobmodel.TypeBackupCreate, Status: jobmodel.StatusQueued, Payload: string(payload), Description: fmt.Sprintf("Scheduled backup for %s", site.Domain)}
			if err := h.store.Jobs.Create(ctx, job); err != nil {
				continue
			}
			h.audit.RecordAction(ctx, userID, "backup_config_run_full", "backup_config", id, nil)
			createdFiles++
			continue
		}

		// Files backup
		if cfg.ScopeType == "files" {
			backup := &store.Backup{SiteID: siteID, Status: "pending", StorageType: storageType, BackupType: "files"}
			if err := h.store.Backups.Create(ctx, backup); err != nil {
				slog.Error("backup config run: create backup record", "config_id", id, "site_id", siteID, "error", err)
				continue
			}
			payloadMap := map[string]any{
				"backup_id": backup.ID, "site_id": site.ID, "domain": site.Domain,
				"root_path": site.RootPath, "storage": storageType, "scheduled": true,
			}
			if provider != nil {
				payloadMap["s3_config"] = map[string]string{
					"bucket": provider.Bucket, "endpoint": provider.Endpoint,
					"region": provider.Region, "access_key": provider.AccessKey, "secret_key": provider.SecretKey,
				}
			}
			payload, _ := json.Marshal(payloadMap)
			job := &jobmodel.Job{
				ServerID: sql.NullInt64{Int64: site.ServerID, Valid: true},
				SiteID:   sql.NullInt64{Int64: site.ID, Valid: true},
				Type:     jobmodel.TypeBackupCreate, Status: jobmodel.StatusQueued, Payload: string(payload),
				Description: fmt.Sprintf("Scheduled files backup for %s", site.Domain),
			}
			if err := h.store.Jobs.Create(ctx, job); err != nil {
				slog.Error("backup config run: create job", "config_id", id, "site_id", siteID, "error", err)
				continue
			}
			slog.Info("backup config run: queued files backup", "config_id", id, "site_id", siteID, "backup_id", backup.ID, "job_id", job.ID, "storage", storageType, "has_s3_config", provider != nil)
			h.audit.RecordAction(ctx, userID, "backup_config_run_files", "backup_config", id, nil)
			createdFiles++
		}

		// Database backup
		if cfg.ScopeType == "database" {
			databases, err := h.store.Databases.ListBySite(ctx, siteID)
			if err != nil || len(databases) == 0 {
				continue
			}
			for _, db := range databases {
				// If specific DB IDs configured, skip if not in list
				if len(cfg.DatabaseIDs) > 0 && !int64InSlice(db.ID, cfg.DatabaseIDs) {
					continue
				}
				exists, err := h.store.BackupDatabaseExists(ctx, db)
				if err != nil || !exists {
					continue
				}
				dbUser, dbPassword, err := h.store.DatabaseBackupCredentials(ctx, db)
				if err != nil {
					continue
				}
				dbStorageType := "database_" + storageType
				backup := &store.Backup{SiteID: siteID, Status: "pending", StorageType: dbStorageType, BackupType: "database"}
				if err := h.store.Backups.Create(ctx, backup); err != nil {
					continue
				}
				payloadMap := map[string]any{
					"backup_id": backup.ID, "site_id": site.ID, "domain": site.Domain,
					"root_path": site.RootPath,
					"db_name":   db.Name, "db_type": db.Type, "db_user": dbUser, "db_password": dbPassword,
				}
				if provider != nil {
					payloadMap["s3_config"] = map[string]string{
						"bucket": provider.Bucket, "endpoint": provider.Endpoint,
						"region": provider.Region, "access_key": provider.AccessKey, "secret_key": provider.SecretKey,
					}
				}
				payload, _ := json.Marshal(payloadMap)
				job := &jobmodel.Job{
					ServerID: sql.NullInt64{Int64: site.ServerID, Valid: true},
					SiteID:   sql.NullInt64{Int64: site.ID, Valid: true},
					Type:     jobmodel.TypeDBBackupCreate, Status: jobmodel.StatusQueued, Payload: string(payload),
					Description: fmt.Sprintf("Scheduled database backup for %s", site.Domain),
				}
				if err := h.store.Jobs.Create(ctx, job); err != nil {
					continue
				}
				h.audit.RecordAction(ctx, userID, "backup_config_run_db", "backup_config", id, nil)
				createdDB++
			}
		}
	}

	// Update last_run_at
	if err := h.store.BackupConfigs.MarkRun(ctx, id); err != nil {
		slog.Error("backup config run: mark run failed", "config_id", id, "error", err)
	}

	return c.JSON(http.StatusAccepted, httpjson.Success{
		Data: map[string]any{
			"config_id":        id,
			"created_files":    createdFiles,
			"created_database": createdDB,
		},
		RequestID: rid,
	})
}

func int64InSlice(val int64, slice []int64) bool {
	for _, v := range slice {
		if v == val {
			return true
		}
	}
	return false
}

func backupConfigToMap(cfg store.BackupConfig) map[string]any {
	m := map[string]any{
		"id":                  cfg.ID,
		"name":                cfg.Name,
		"scope_type":          cfg.ScopeType,
		"site_ids":            cfg.SiteIDs,
		"auto_discover_sites": len(cfg.SiteIDs) == 0 && (cfg.ScopeType == "database" || cfg.ScopeType == "both"),
		"cron_expr":           cfg.CronExpr,
		"enabled":             cfg.Enabled,
		"retention_days":      cfg.RetentionDays,
		"exclude_patterns":    cfg.ExcludePatterns,
		"database_ids":        cfg.DatabaseIDs,
		"created_at":          cfg.CreatedAt,
		"updated_at":          cfg.UpdatedAt,
	}
	if cfg.ProviderID.Valid {
		m["provider_id"] = cfg.ProviderID.Int64
	}
	if cfg.LastRunAt.Valid {
		m["last_run_at"] = cfg.LastRunAt.Time
	}
	if cfg.NextRunAt.Valid {
		m["next_run_at"] = cfg.NextRunAt.Time
	}
	return m
}
