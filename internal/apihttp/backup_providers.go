package apihttp

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/labstack/echo/v4"
	"net/http"
	"os/exec"
	"sigardapanel/internal/audit"
	"sigardapanel/internal/httpjson"
	"sigardapanel/internal/jobmodel"
	"sigardapanel/internal/store"
	"strconv"
	"strings"
)

type backupProviderHandler struct {
	store *store.Store
	audit *audit.Recorder
}

func newBackupProviderHandler(s *store.Store, a *audit.Recorder) *backupProviderHandler {
	return &backupProviderHandler{store: s, audit: a}
}

var allowedProviderTypes = map[string]struct{}{
	"s3":           {},
	"b2":           {},
	"r2":           {},
	"wasabi":       {},
	"digitalocean": {},
	"generic":      {},
}

func (h *backupProviderHandler) list(c echo.Context) error {
	providers, err := h.store.BackupProviders.List(c.Request().Context())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	out := make([]map[string]any, 0, len(providers))
	for _, p := range providers {
		out = append(out, backupProviderToMap(p))
	}
	return c.JSON(http.StatusOK, httpjson.Success{
		Data: map[string]any{"providers": out, "count": len(out)},
	})
}

func (h *backupProviderHandler) create(c echo.Context) error {
	var body struct {
		Name          string `json:"name"`
		ProviderType  string `json:"provider_type"`
		Bucket        string `json:"bucket"`
		Endpoint      string `json:"endpoint"`
		Region        string `json:"region"`
		AccessKey     string `json:"access_key"`
		SecretKey     string `json:"secret_key"`
		IsDefault     bool   `json:"is_default"`
		RetentionDays int    `json:"retention_days"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	body.Name = strings.TrimSpace(body.Name)
	body.ProviderType = strings.TrimSpace(strings.ToLower(body.ProviderType))
	body.Bucket = strings.TrimSpace(body.Bucket)
	body.Endpoint = strings.TrimSpace(body.Endpoint)
	body.Region = strings.TrimSpace(body.Region)
	body.AccessKey = strings.TrimSpace(body.AccessKey)
	body.SecretKey = strings.TrimSpace(body.SecretKey)

	if body.Name == "" || body.Bucket == "" || body.AccessKey == "" || body.SecretKey == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name, bucket, access_key, secret_key required")
	}
	if _, ok := allowedProviderTypes[body.ProviderType]; !ok {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid provider_type. Allowed: s3, b2, r2, wasabi, digitalocean, generic")
	}
	endpoint, region, err := normalizeProviderEndpointRegion(body.ProviderType, body.Endpoint, body.Region)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	body.Endpoint = endpoint
	body.Region = region

	provider := &store.BackupStorageProvider{
		Name:          body.Name,
		ProviderType:  body.ProviderType,
		Bucket:        body.Bucket,
		Endpoint:      body.Endpoint,
		Region:        body.Region,
		AccessKey:     body.AccessKey,
		SecretKey:     body.SecretKey,
		IsDefault:     body.IsDefault,
		RetentionDays: body.RetentionDays,
	}
	if err := h.store.BackupProviders.Create(c.Request().Context(), provider); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	h.audit.RecordAction(c.Request().Context(), 0, "backup_provider_create", "backup_provider", provider.ID, nil)

	return c.JSON(http.StatusCreated, httpjson.Success{
		Data: backupProviderToMap(*provider),
	})
}

func (h *backupProviderHandler) update(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}

	existing, err := h.store.BackupProviders.GetByID(c.Request().Context(), id)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if existing == nil {
		return echo.NewHTTPError(http.StatusNotFound, "provider not found")
	}

	var body struct {
		Name          *string `json:"name"`
		ProviderType  *string `json:"provider_type"`
		Bucket        *string `json:"bucket"`
		Endpoint      *string `json:"endpoint"`
		Region        *string `json:"region"`
		AccessKey     *string `json:"access_key"`
		SecretKey     *string `json:"secret_key"`
		IsDefault     *bool   `json:"is_default"`
		RetentionDays *int    `json:"retention_days"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	if body.Name != nil {
		existing.Name = strings.TrimSpace(*body.Name)
	}
	if body.ProviderType != nil {
		pt := strings.TrimSpace(strings.ToLower(*body.ProviderType))
		if _, ok := allowedProviderTypes[pt]; !ok {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid provider_type")
		}
		existing.ProviderType = pt
	}
	if body.Bucket != nil {
		existing.Bucket = strings.TrimSpace(*body.Bucket)
	}
	if body.Endpoint != nil {
		existing.Endpoint = strings.TrimSpace(*body.Endpoint)
	}
	if body.Region != nil {
		existing.Region = strings.TrimSpace(*body.Region)
	}
	endpoint, region, err := normalizeProviderEndpointRegion(existing.ProviderType, existing.Endpoint, existing.Region)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	existing.Endpoint = endpoint
	existing.Region = region
	if body.AccessKey != nil {
		existing.AccessKey = strings.TrimSpace(*body.AccessKey)
	}
	if body.SecretKey != nil {
		existing.SecretKey = strings.TrimSpace(*body.SecretKey)
	}
	if body.IsDefault != nil {
		existing.IsDefault = *body.IsDefault
	}
	if body.RetentionDays != nil {
		existing.RetentionDays = *body.RetentionDays
	}

	if err := h.store.BackupProviders.Update(c.Request().Context(), existing); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	h.audit.RecordAction(c.Request().Context(), 0, "backup_provider_update", "backup_provider", id, nil)

	return c.JSON(http.StatusOK, httpjson.Success{
		Data: backupProviderToMap(*existing),
	})
}

func (h *backupProviderHandler) delete(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}

	if err := h.store.BackupProviders.Delete(c.Request().Context(), id); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	h.audit.RecordAction(c.Request().Context(), 0, "backup_provider_delete", "backup_provider", id, nil)

	return c.JSON(http.StatusOK, httpjson.Success{
		Data: map[string]any{"deleted": true},
	})
}

// testConnection tests S3 connectivity by running aws s3 ls on the bucket.
func (h *backupProviderHandler) usage(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	provider, err := h.store.BackupProviders.GetByID(c.Request().Context(), id)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if provider == nil {
		return echo.NewHTTPError(http.StatusNotFound, "provider not found")
	}

	if strings.EqualFold(c.QueryParam("source"), "actual") {
		actual, err := scanProviderActualUsage(c.Request().Context(), provider)
		if err != nil {
			return c.JSON(http.StatusOK, httpjson.Success{
				Data: map[string]any{
					"provider_id":   provider.ID,
					"provider_type": provider.ProviderType,
					"bucket":        provider.Bucket,
					"endpoint":      provider.Endpoint,
					"source":        "actual_provider",
					"connected":     false,
					"error":         err.Error(),
				},
			})
		}
		return c.JSON(http.StatusOK, httpjson.Success{Data: actual})
	}

	backups, err := h.store.Backups.ListAll(c.Request().Context(), 10000, 0)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	bucketPrefix := "s3://" + provider.Bucket + "/"
	var totalBytes int64
	var backupCount int
	var latest any
	byStorageType := map[string]int64{}
	byStatus := map[string]int{}

	for _, b := range backups {
		byStatus[b.Status]++
		if b.Status != "completed" {
			continue
		}
		path := b.StoragePath.String
		matchesProvider := false
		if strings.HasPrefix(path, bucketPrefix) {
			matchesProvider = true
		}
		if path == "" && (b.StorageType == provider.ProviderType || b.StorageType == "database_"+provider.ProviderType || b.StorageType == "full_"+provider.ProviderType) {
			matchesProvider = true
		}
		if !matchesProvider {
			continue
		}
		backupCount++
		totalBytes += b.SizeBytes
		byStorageType[b.StorageType] += b.SizeBytes
		if latest == nil || b.CompletedAt.Valid {
			latest = b.CompletedAt.Time
		}
	}

	return c.JSON(http.StatusOK, httpjson.Success{
		Data: map[string]any{
			"provider_id":       provider.ID,
			"provider_type":     provider.ProviderType,
			"bucket":            provider.Bucket,
			"endpoint":          provider.Endpoint,
			"source":            "database_estimate",
			"total_bytes":       totalBytes,
			"total_mb":          float64(totalBytes) / 1024.0 / 1024.0,
			"backup_count":      backupCount,
			"by_storage_type":   byStorageType,
			"all_backup_status": byStatus,
			"latest_completed":  latest,
		},
	})
}

func (h *backupProviderHandler) testConnection(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	provider, err := h.store.BackupProviders.GetByID(c.Request().Context(), id)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if provider == nil {
		return echo.NewHTTPError(http.StatusNotFound, "provider not found")
	}

	// Validate credentials
	if provider.AccessKey == "" || provider.SecretKey == "" {
		return c.JSON(http.StatusOK, httpjson.Success{
			Data: map[string]any{
				"connected": false,
				"error":     "Access key and secret key are required",
			},
		})
	}
	if provider.Bucket == "" || provider.Endpoint == "" {
		return c.JSON(http.StatusOK, httpjson.Success{
			Data: map[string]any{
				"connected": false,
				"error":     "Bucket and endpoint are required",
			},
		})
	}
	endpoint, region, err := normalizeProviderEndpointRegion(provider.ProviderType, provider.Endpoint, provider.Region)
	if err != nil {
		return c.JSON(http.StatusOK, httpjson.Success{
			Data: map[string]any{
				"connected": false,
				"error":     err.Error(),
			},
		})
	}

	// Try actual S3 connectivity test (aws s3 ls)
	if _, lookErr := exec.LookPath("aws"); lookErr != nil {
		// aws CLI not available — validate config only
		return c.JSON(http.StatusOK, httpjson.Success{
			Data: map[string]any{
				"connected": true,
				"message":   fmt.Sprintf("Provider config valid (aws CLI not available for live test) — %s endpoint: %s", provider.ProviderType, endpoint),
				"provider":  provider.ProviderType,
				"endpoint":  endpoint,
				"region":    region,
			},
		})
	}

	cmd := exec.CommandContext(c.Request().Context(), "aws", "s3", "ls", "s3://"+provider.Bucket,
		"--endpoint-url", "https://"+endpoint, "--region", region)
	cmd.Env = append(cmd.Environ(),
		"AWS_ACCESS_KEY_ID="+provider.AccessKey,
		"AWS_SECRET_ACCESS_KEY="+provider.SecretKey,
	)
	out, cmdErr := cmd.CombinedOutput()
	if cmdErr != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = cmdErr.Error()
		}
		return c.JSON(http.StatusOK, httpjson.Success{
			Data: map[string]any{
				"connected": false,
				"error":     fmt.Sprintf("Connection failed: %s", msg),
				"provider":  provider.ProviderType,
				"endpoint":  endpoint,
				"region":    region,
			},
		})
	}

	return c.JSON(http.StatusOK, httpjson.Success{
		Data: map[string]any{
			"connected": true,
			"message":   fmt.Sprintf("Successfully connected to %s bucket", provider.ProviderType),
			"provider":  provider.ProviderType,
			"endpoint":  endpoint,
			"region":    region,
		},
	})
}

func scanProviderActualUsage(ctx context.Context, provider *store.BackupStorageProvider) (map[string]any, error) {
	endpoint, region, err := normalizeProviderEndpointRegion(provider.ProviderType, provider.Endpoint, provider.Region)
	if err != nil {
		return nil, err
	}
	if _, err := exec.LookPath("aws"); err != nil {
		return nil, fmt.Errorf("aws CLI not installed on server/agent host")
	}

	cmd := exec.CommandContext(ctx, "aws", "s3", "ls", "s3://"+provider.Bucket, "--recursive", "--summarize", "--endpoint-url", "https://"+endpoint, "--region", region)
	cmd.Env = append(cmd.Environ(),
		"AWS_ACCESS_KEY_ID="+provider.AccessKey,
		"AWS_SECRET_ACCESS_KEY="+provider.SecretKey,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("provider scan failed: %s", msg)
	}

	var objectCount int64
	var totalBytes int64
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Total Objects:") {
			fmt.Sscanf(line, "Total Objects: %d", &objectCount)
		}
		if strings.HasPrefix(line, "Total Size:") {
			fmt.Sscanf(line, "Total Size: %d", &totalBytes)
		}
	}

	return map[string]any{
		"provider_id":   provider.ID,
		"provider_type": provider.ProviderType,
		"bucket":        provider.Bucket,
		"endpoint":      endpoint,
		"region":        region,
		"source":        "actual_provider",
		"connected":     true,
		"object_count":  objectCount,
		"total_bytes":   totalBytes,
		"total_mb":      float64(totalBytes) / 1024.0 / 1024.0,
	}, nil
}

func normalizeProviderEndpointRegion(providerType, endpoint, region string) (string, string, error) {
	providerType = strings.TrimSpace(strings.ToLower(providerType))
	endpoint = strings.TrimSpace(endpoint)
	region = strings.TrimSpace(region)

	switch providerType {
	case "s3":
		if region == "" || region == "auto" {
			region = "us-east-1"
		}
		if endpoint == "" {
			endpoint = "s3.amazonaws.com"
		}
	case "r2":
		if region == "" {
			region = "auto"
		}
		if endpoint == "" {
			return "", "", fmt.Errorf("endpoint required for Cloudflare R2 (example: <accountid>.r2.cloudflarestorage.com)")
		}
		if !strings.Contains(endpoint, "r2.cloudflarestorage.com") {
			return "", "", fmt.Errorf("invalid Cloudflare R2 endpoint (expected: <accountid>.r2.cloudflarestorage.com)")
		}
	case "b2":
		if region == "" || region == "auto" {
			return "", "", fmt.Errorf("region required for Backblaze B2 (example: us-west-004)")
		}
		if endpoint == "" {
			endpoint = "s3." + region + ".backblazeb2.com"
		}
		if !strings.Contains(endpoint, "backblazeb2.com") {
			return "", "", fmt.Errorf("invalid Backblaze B2 endpoint (expected: s3.<region>.backblazeb2.com)")
		}
	case "wasabi":
		if region == "" || region == "auto" {
			return "", "", fmt.Errorf("region required for Wasabi (example: us-east-1)")
		}
		if endpoint == "" {
			endpoint = "s3." + region + ".wasabisys.com"
		}
		if !strings.Contains(endpoint, "wasabisys.com") {
			return "", "", fmt.Errorf("invalid Wasabi endpoint (expected: s3.<region>.wasabisys.com)")
		}
	case "digitalocean":
		if region == "" || region == "auto" {
			return "", "", fmt.Errorf("region required for DigitalOcean Spaces (example: sgp1)")
		}
		if endpoint == "" {
			endpoint = region + ".digitaloceanspaces.com"
		}
		if !strings.Contains(endpoint, "digitaloceanspaces.com") {
			return "", "", fmt.Errorf("invalid DigitalOcean Spaces endpoint (expected: <region>.digitaloceanspaces.com)")
		}
	case "generic":
		if endpoint == "" {
			return "", "", fmt.Errorf("endpoint required for generic S3-compatible provider")
		}
		if region == "" {
			region = "auto"
		}
	default:
		return "", "", fmt.Errorf("invalid provider_type")
	}

	endpoint = strings.TrimPrefix(endpoint, "https://")
	endpoint = strings.TrimPrefix(endpoint, "http://")
	endpoint = strings.TrimRight(endpoint, "/")
	if !strings.Contains(endpoint, ".") {
		return "", "", fmt.Errorf("endpoint format appears invalid")
	}
	return endpoint, region, nil
}

func backupProviderToMap(p store.BackupStorageProvider) map[string]any {
	return map[string]any{
		"id":             p.ID,
		"name":           p.Name,
		"provider_type":  p.ProviderType,
		"bucket":         p.Bucket,
		"endpoint":       p.Endpoint,
		"region":         p.Region,
		"access_key":     p.AccessKey,
		"is_default":     p.IsDefault,
		"retention_days": p.RetentionDays,
		"created_at":     p.CreatedAt,
		"updated_at":     p.UpdatedAt,
	}
}

// ─── Update backup create to support S3 ────────────────────

func (h *backupHandler) createWithProvider(c echo.Context) error {
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
	if !h.canAccess(c.Request().Context(), sess, site) {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}

	var body struct {
		StorageProviderID int64 `json:"storage_provider_id"`
	}
	c.Bind(&body) // optional, ignore error

	storageType := "local"
	var provider *store.BackupStorageProvider

	if body.StorageProviderID > 0 {
		provider, err = h.store.BackupProviders.GetByID(c.Request().Context(), body.StorageProviderID)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
		}
		if provider == nil {
			return echo.NewHTTPError(http.StatusBadRequest, "storage provider not found")
		}
		storageType = provider.ProviderType
	} else {
		// Try default provider
		provider, _ = h.store.BackupProviders.GetDefault(c.Request().Context())
		if provider != nil {
			storageType = provider.ProviderType
		}
	}

	backup := &store.Backup{
		SiteID:      siteID,
		Status:      "pending",
		StorageType: storageType,
	}
	if err := h.store.Backups.Create(c.Request().Context(), backup); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	// Build payload with optional S3 config
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
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	h.audit.RecordAction(c.Request().Context(), userID, "backup_create", "backup", backup.ID, nil)

	return c.JSON(http.StatusAccepted, httpjson.Success{
		Data: map[string]any{
			"backup_id": backup.ID,
			"job_id":    job.ID,
			"status":    backup.Status,
			"storage":   storageType,
		},
		RequestID: rid,
	})
}
