package jobs

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"sigardapanel/internal/agentclient"
	"sigardapanel/internal/jobmodel"
	"sigardapanel/internal/notify"
	"sigardapanel/internal/secrets"
	"sigardapanel/internal/store"
)

type backoffConfig struct {
	baseDelay time.Duration
	maxDelay  time.Duration
	jitter    time.Duration
}

var defaultBackoff = backoffConfig{
	baseDelay: 1 * time.Second,
	maxDelay:  5 * time.Minute,
	jitter:    500 * time.Millisecond,
}

func (b backoffConfig) delay(attempts int) time.Duration {
	if attempts < 1 {
		attempts = 1
	}
	shift := attempts - 1
	if shift > 30 {
		shift = 30
	}
	delay := b.baseDelay * time.Duration(1<<uint(shift))
	if delay > b.maxDelay || delay < 0 {
		delay = b.maxDelay
	}
	jitter := time.Duration(rand.Int63n(int64(b.jitter)))
	return delay + jitter
}

// Worker polls the job store and dispatches jobs to the agent.
type Worker struct {
	store        *store.Store
	agent        *agentclient.Client
	pollInterval time.Duration
	wg           sync.WaitGroup
	stopCh       chan struct{}
	stopped      chan struct{}
}

// NewWorker creates a new worker.
func NewWorker(s *store.Store, agent *agentclient.Client) *Worker {
	return &Worker{
		store:        s,
		agent:        agent,
		pollInterval: 2 * time.Second,
		stopCh:       make(chan struct{}),
		stopped:      make(chan struct{}),
	}
}

// Start begins the worker loop in a goroutine.
func (w *Worker) Start(ctx context.Context) {
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		defer close(w.stopped)
		w.loop(ctx)
	}()
}

// Stop signals the worker to stop and waits for it to finish.
func (w *Worker) Stop() {
	close(w.stopCh)
	<-w.stopped
}

func (w *Worker) loop(ctx context.Context) {
	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-w.stopCh:
			return
		case <-ticker.C:
			w.processOne(ctx)
		}
	}
}

func (w *Worker) processOne(ctx context.Context) {
	job, err := w.store.Jobs.ClaimNext(ctx, "")
	if err != nil {
		slog.Error("worker: claim next job", "error", err)
		return
	}
	if job == nil {
		return
	}

	slog.Info("worker: executing job", "id", job.ID, "type", job.Type, "attempts", job.Attempts)

	// Set initial progress for backup jobs
	if job.Type == jobmodel.TypeBackupCreate || job.Type == jobmodel.TypeDBBackupCreate {
		var payloadData struct {
			BackupID int64 `json:"backup_id"`
		}
		if err := json.Unmarshal([]byte(job.Payload), &payloadData); err == nil && payloadData.BackupID > 0 {
			_ = w.store.Backups.UpdateProgress(ctx, payloadData.BackupID, "archiving...")
			_ = w.store.Backups.UpdateStatus(ctx, payloadData.BackupID, "running")
		}

		// Concurrency limit: max 1 concurrent backup job per server to save disk I/O and RAM
		const maxConcurrentBackups = 1
		if job.ServerID.Valid && job.ServerID.Int64 > 0 {
			running, _ := w.store.Jobs.CountRunningByServer(ctx, job.ServerID.Int64)
			if running >= maxConcurrentBackups {
				delay := time.Duration(running-maxConcurrentBackups+1) * 30 * time.Second
				retryAt := time.Now().Add(delay)
				_ = w.store.Jobs.ScheduleRetry(ctx, job.ID, retryAt)
				slog.Info("worker: backup concurrency limit, re-queued",
					"job_id", job.ID, "server_id", job.ServerID.Int64,
					"running", running, "limit", maxConcurrentBackups, "retry_in", delay)
				return
			}
		}
	}

	agent, agentErr := w.agentForJob(ctx, job)
	if agentErr != nil {
		slog.Error("worker: cannot resolve agent", "id", job.ID, "server_id", job.ServerID.Int64, "error", agentErr)
		if job.Attempts < job.MaxAttempts {
			delay := defaultBackoff.delay(job.Attempts)
			retryAt := time.Now().Add(delay)
			_ = w.store.Jobs.ScheduleRetry(ctx, job.ID, retryAt)
			_ = w.store.Jobs.SetError(ctx, job.ID, agentErr.Error())
			return
		}
		_ = w.store.Jobs.MarkCompleted(ctx, job.ID, false, "", agentErr.Error())
		// Mark backup record as failed when job fails permanently
		if job.Type == jobmodel.TypeBackupCreate || job.Type == jobmodel.TypeDBBackupCreate {
			var payloadData struct {
				BackupID int64 `json:"backup_id"`
			}
			if err := json.Unmarshal([]byte(job.Payload), &payloadData); err == nil && payloadData.BackupID > 0 {
				_ = w.store.Backups.MarkFailed(ctx, payloadData.BackupID, agentErr.Error())
			}
		}
		sender := notify.NewSender(w.store)
		sender.Send(ctx, "error", "Job failed", fmt.Sprintf("Job %d (%s): %s", job.ID, job.Type, agentErr.Error()))
		return
	}
	// Decrypt any credential fields that were encrypted at rest before sending
	// the payload to the agent.
	dispatchPayload := job.Payload
	if dec, derr := secrets.DecryptPayload(job.Payload); derr == nil {
		dispatchPayload = dec
	} else {
		slog.Warn("worker: decrypt job payload failed", "job_id", job.ID, "error", derr)
	}
	result, execErr := agent.Execute(ctx, job.Type, dispatchPayload)

	if execErr != nil {
		if job.Attempts < job.MaxAttempts {
			delay := defaultBackoff.delay(job.Attempts)
			retryAt := time.Now().Add(delay)
			if err := w.store.Jobs.ScheduleRetry(ctx, job.ID, retryAt); err != nil {
				slog.Error("worker: schedule retry", "error", err)
			}
			if err := w.store.Jobs.SetError(ctx, job.ID, execErr.Error()); err != nil {
				slog.Error("worker: failed to set error", "id", job.ID, "error", err)
			}
			slog.Warn("worker: job failed, scheduled retry", "id", job.ID, "attempts", job.Attempts, "retry_in", delay)
			return
		}
		resultJSON, _ := json.Marshal(result)
		if err := w.store.Jobs.MarkCompleted(ctx, job.ID, false, string(resultJSON), execErr.Error()); err != nil {
			slog.Error("worker: mark completed", "id", job.ID, "error", err)
		}
		slog.Error("worker: job failed permanently", "id", job.ID, "error", execErr, "attempts", job.Attempts)

		// Failover: if backup.create failed with s3_config, retry without (local backup)
		if job.Type == jobmodel.TypeBackupCreate {
			var payloadData map[string]any
			if err := json.Unmarshal([]byte(job.Payload), &payloadData); err == nil {
				if _, hasS3 := payloadData["s3_config"]; hasS3 {
					failoverPayload := make(map[string]any)
					for k, v := range payloadData {
						failoverPayload[k] = v
					}
					delete(failoverPayload, "s3_config")
					failoverPayload["storage"] = "local"
					failoverPayload["failover"] = true
					failoverJSON, _ := json.Marshal(failoverPayload)
					failoverJob := &jobmodel.Job{
						ServerID:    job.ServerID,
						SiteID:      job.SiteID,
						Type:        jobmodel.TypeBackupCreate,
						Status:      jobmodel.StatusQueued,
						Payload:     string(failoverJSON),
						MaxAttempts: 1,
					}
					if err := w.store.Jobs.Create(ctx, failoverJob); err != nil {
						slog.Error("worker: create failover backup job", "error", err)
					} else {
						slog.Info("worker: created failover backup job (local)", "original_job", job.ID, "failover_job", failoverJob.ID)
					}
				}
			}
		}

		// Mark backup record as failed when job fails permanently.
		// If a failover job was created above, it will re-set the backup
		// to "running" when the worker picks it up (lines 115-122).
		if job.Type == jobmodel.TypeBackupCreate || job.Type == jobmodel.TypeDBBackupCreate {
			var payloadData struct {
				BackupID int64 `json:"backup_id"`
			}
			if err := json.Unmarshal([]byte(job.Payload), &payloadData); err == nil && payloadData.BackupID > 0 {
				_ = w.store.Backups.MarkFailed(ctx, payloadData.BackupID, execErr.Error())
			}
		}

		sender := notify.NewSender(w.store)
		sender.Send(ctx, "error", "Job failed permanently", fmt.Sprintf("Job %d (%s): %s", job.ID, job.Type, execErr.Error()))
		return
	}

	resultJSON, _ := json.Marshal(result)
	if err := w.store.Jobs.MarkCompleted(ctx, job.ID, true, string(resultJSON), ""); err != nil {
		slog.Error("worker: mark completed", "id", job.ID, "error", err)
	}

	// Post-success: update related resource status
	switch job.Type {
	case jobmodel.TypeSiteCreate:
		if job.SiteID.Valid {
			siteID := job.SiteID.Int64
			// Update site status to active
			if err := w.store.Sites.UpdateStatus(ctx, siteID, "active"); err != nil {
				slog.Error("worker: update site status", "id", siteID, "error", err)
			}

			// Update conf_file from agent response
			if result != nil && result.NginxConfig != "" {
				if err := w.store.Sites.UpdateConfFile(ctx, siteID, result.NginxConfig); err != nil {
					slog.Error("worker: update conf_file", "id", siteID, "error", err)
				}
			}
			// Update proxy_pass_url from agent response
			if result != nil && result.ProxyPassURL != "" {
				if err := w.store.Sites.UpdateProxyPassURL(ctx, siteID, result.ProxyPassURL); err != nil {
					slog.Error("worker: update proxy_pass_url", "id", siteID, "error", err)
				}
			}

			// Auto-enqueue SSL issuance for new sites
			site, err := w.store.Sites.GetByID(ctx, siteID)
			if err == nil && site != nil && site.Domain != "" {
				// Get owner email for Let's Encrypt
				email := "admin@sigarda.com" // fallback
				if site.OwnerID.Valid {
					owner, err := w.store.Users.GetByID(ctx, site.OwnerID.Int64)
					if err == nil && owner != nil && owner.Email != "" {
						email = owner.Email
					}
				}
				payload := fmt.Sprintf(`{"site_id":%d,"domain":%q,"email":%q}`, siteID, site.Domain, email)
				sslJob := &jobmodel.Job{
					ServerID: sql.NullInt64{Int64: site.ServerID, Valid: true},
					SiteID:   sql.NullInt64{Int64: site.ID, Valid: true},
					Type:     jobmodel.TypeSSLIssue,
					Status:   jobmodel.StatusQueued,
					Payload:  payload,
				}
				if err := w.store.Jobs.Create(ctx, sslJob); err != nil {
					slog.Error("worker: enqueue ssl.issue", "id", siteID, "error", err)
				} else {
					slog.Info("worker: auto-enqueued ssl.issue", "site_id", siteID, "domain", site.Domain, "job_id", sslJob.ID)
				}
			}
		}

	case jobmodel.TypeSSLIssue:
		if job.SiteID.Valid {
			// Update ssl_status to active
			if err := w.store.Sites.UpdateSSL(ctx, job.SiteID.Int64, "active", "Let's Encrypt", time.Now().Add(90*24*time.Hour)); err != nil {
				slog.Error("worker: update ssl status", "id", job.SiteID.Int64, "error", err)
			}
		}

	case jobmodel.TypeBackupCreate, jobmodel.TypeDBBackupCreate:
		// Parse backup_id from job payload and persist actual local/S3 path from agent result.
		var payloadData struct {
			BackupID int64 `json:"backup_id"`
		}
		if err := json.Unmarshal([]byte(job.Payload), &payloadData); err == nil && payloadData.BackupID > 0 {
			localPath := ""
			s3Path := ""
			if result != nil {
				localPath = result.Path
				s3Path = result.S3Path
				if result.S3UploadError != "" {
					slog.Warn("worker: backup provider upload failed, kept local backup", "backup_id", payloadData.BackupID, "job_id", job.ID, "error", result.S3UploadError)
					if err := w.store.Backups.UpdateProgress(ctx, payloadData.BackupID, "provider upload failed, local backup saved"); err != nil {
						slog.Error("worker: update backup progress", "id", payloadData.BackupID, "error", err)
					}
				}
			}
			size := int64(0)
			if result != nil {
				size = result.SizeBytes
			}
			// Mark completed with local path first
			if err := w.store.Backups.MarkCompleted(ctx, payloadData.BackupID, size, 0, localPath); err != nil {
				slog.Error("worker: mark backup completed", "id", payloadData.BackupID, "error", err)
			} else {
				slog.Info("worker: backup completed", "id", payloadData.BackupID, "job_id", job.ID, "local_path", localPath, "size_bytes", size)
			}
			// Store S3 path separately if present
			if s3Path != "" {
				if err := w.store.Backups.UpdateS3Path(ctx, payloadData.BackupID, s3Path); err != nil {
					slog.Error("worker: update backup s3_path", "id", payloadData.BackupID, "error", err)
				} else {
					slog.Info("worker: backup s3_path saved", "id", payloadData.BackupID, "s3_path", s3Path)
					// Delete local file after successful S3 upload to save disk space
					if localPath != "" && !strings.HasPrefix(localPath, "s3://") {
						if err := os.Remove(localPath); err != nil {
							slog.Warn("worker: failed to remove local backup after S3 upload", "path", localPath, "error", err)
						} else {
							slog.Info("worker: removed local backup after S3 upload", "path", localPath, "backup_id", payloadData.BackupID)
							// Clear storage_path since local file is gone
							if err := w.store.Backups.ClearStoragePath(ctx, payloadData.BackupID); err != nil {
								slog.Error("worker: clear storage_path", "id", payloadData.BackupID, "error", err)
							}
						}
					}
				}
			}

			sender := notify.NewSender(w.store)
			backupTypeStr := "Site Files"
			if job.Type == jobmodel.TypeDBBackupCreate {
				backupTypeStr = "Database"
			}
			sender.Send(ctx, "info", "Backup Completed", fmt.Sprintf("Backup #%d (%s) completed successfully (size: %d bytes)", payloadData.BackupID, backupTypeStr, size))
		}
		// Auto-retention: keep only latest 7 per site+type after successful backup
		if job.SiteID.Valid && result != nil && (result.S3UploadError == "" || result.S3Path != "") {
			deleted, err := w.store.Backups.DeleteOldest(ctx, job.SiteID.Int64, 7)
			if err != nil {
				slog.Warn("worker: cleanup old backups failed", "site_id", job.SiteID.Int64, "error", err)
			} else if len(deleted) > 0 {
				// Clean up S3 files and local files for the deleted backups
				site, _ := w.store.Sites.GetByID(ctx, job.SiteID.Int64)
				var provider *store.BackupStorageProvider
				if site != nil && site.BackupProviderID.Valid && site.BackupProviderID.Int64 > 0 {
					provider, _ = w.store.BackupProviders.GetByID(ctx, site.BackupProviderID.Int64)
				}
				if provider == nil {
					provider, _ = w.store.BackupProviders.GetDefault(ctx)
				}

				for _, b := range deleted {
					if b.S3Path.Valid && b.S3Path.String != "" && provider != nil {
						slog.Info("worker: deleting S3 file for expired backup", "backup_id", b.ID, "s3_path", b.S3Path.String)
						w.deleteS3Object(ctx, provider, b.S3Path.String)
					}
					if b.StoragePath.Valid && b.StoragePath.String != "" && !strings.HasPrefix(b.StoragePath.String, "s3://") {
						_ = os.Remove(b.StoragePath.String)
					}
				}
			}
		}

	case jobmodel.TypeBackupRestore:
		var payloadData struct {
			BackupID int64 `json:"backup_id"`
		}
		if err := json.Unmarshal([]byte(job.Payload), &payloadData); err == nil && payloadData.BackupID > 0 {
			if err := w.store.Backups.UpdateStatus(ctx, payloadData.BackupID, "completed"); err != nil {
				slog.Error("worker: mark backup restore completed", "id", payloadData.BackupID, "error", err)
			}
		}

	case jobmodel.TypeAppStart, jobmodel.TypeAppRestart:
		if job.SiteID.Valid {
			appStatus := "running"
			if result != nil && result.AppStatus != "" {
				appStatus = result.AppStatus
			}
			if err := w.store.Sites.UpdateAppStatus(ctx, job.SiteID.Int64, appStatus); err != nil {
				slog.Error("worker: update app status", "id", job.SiteID.Int64, "error", err)
			}
		}

	case jobmodel.TypeAppStop:
		if job.SiteID.Valid {
			if err := w.store.Sites.UpdateAppStatus(ctx, job.SiteID.Int64, "stopped"); err != nil {
				slog.Error("worker: update app status", "id", job.SiteID.Int64, "error", err)
			}
		}
	}

	slog.Info("worker: job succeeded", "id", job.ID)
}

func (w *Worker) deleteS3Object(ctx context.Context, provider *store.BackupStorageProvider, s3Path string) {
	if provider == nil || s3Path == "" {
		return
	}
	endpoint := provider.Endpoint
	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		endpoint = "https://" + endpoint
	}
	cmd := exec.CommandContext(ctx, "aws", "s3", "rm", s3Path,
		"--endpoint-url", endpoint, "--region", provider.Region)
	cmd.Env = append(os.Environ(),
		"AWS_ACCESS_KEY_ID="+provider.AccessKey,
		"AWS_SECRET_ACCESS_KEY="+provider.SecretKey,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		slog.Warn("worker: failed to delete S3 object", "s3_path", s3Path, "error", err, "output", strings.TrimSpace(string(out)))
	} else {
		slog.Info("worker: deleted S3 object", "s3_path", s3Path)
	}
}

func (w *Worker) agentForJob(ctx context.Context, job *jobmodel.Job) (*agentclient.Client, error) {
	if !job.ServerID.Valid || job.ServerID.Int64 <= 0 {
		return w.agent, nil
	}
	server, err := w.store.Servers.GetByID(ctx, job.ServerID.Int64)
	if err != nil || server == nil {
		return nil, fmt.Errorf("server %d not found", job.ServerID.Int64)
	}
	if server.AgentToken == "" {
		return nil, fmt.Errorf("server %d has no agent token", server.ID)
	}
	if !server.AgentURL.Valid || server.AgentURL.String == "" {
		return nil, fmt.Errorf("server %d (%s) has no agent URL configured — set agent_url on the server", server.ID, server.Name)
	}
	return agentclient.New(server.AgentURL.String, server.AgentToken), nil
}
