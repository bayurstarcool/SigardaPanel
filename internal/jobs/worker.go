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
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"sigardapanel/internal/agentclient"
	"sigardapanel/internal/jobmodel"
	"sigardapanel/internal/nginx"
	"sigardapanel/internal/notify"
	"sigardapanel/internal/secrets"
	"sigardapanel/internal/siteinput"
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

// defaultWorkerConcurrency is how many jobs may run at once. Jobs of the same
// site never overlap and at most one backup runs per server (both enforced
// atomically by store.ClaimNext), so a slow certbot or backup only occupies one
// slot instead of blocking the whole queue.
const defaultWorkerConcurrency = 3

// Worker polls the job store and dispatches jobs to the agent using a small
// bounded pool of goroutines.
type Worker struct {
	store        *store.Store
	agent        *agentclient.Client
	pollInterval time.Duration
	concurrency  int
	wg           sync.WaitGroup
	stopCh       chan struct{}
	stopped      chan struct{}
	stopOnce     sync.Once
}

// NewWorker creates a new worker.
func NewWorker(s *store.Store, agent *agentclient.Client) *Worker {
	return &Worker{
		store:        s,
		agent:        agent,
		pollInterval: 2 * time.Second,
		concurrency:  workerConcurrencyFromEnv(),
		stopCh:       make(chan struct{}),
		stopped:      make(chan struct{}),
	}
}

// workerConcurrencyFromEnv reads SIGARDAPANEL_WORKER_CONCURRENCY (1-16).
func workerConcurrencyFromEnv() int {
	if v := strings.TrimSpace(os.Getenv("SIGARDAPANEL_WORKER_CONCURRENCY")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 && n <= 16 {
			return n
		}
		slog.Warn("worker: ignoring invalid SIGARDAPANEL_WORKER_CONCURRENCY", "value", v)
	}
	return defaultWorkerConcurrency
}

// Start begins the worker pool.
func (w *Worker) Start(ctx context.Context) {
	// The worker runs in-process, so any job still 'running' was orphaned by a
	// previous process. Fail it now: with per-site serialisation it would
	// otherwise block that site's queue until the stuck-job cleanup.
	if n, err := w.store.Jobs.FailOrphanedRunning(ctx, "interrupted: panel restarted while the job was running"); err != nil {
		slog.Warn("worker: fail orphaned running jobs", "error", err)
	} else if n > 0 {
		slog.Warn("worker: marked orphaned running jobs as failed", "count", n)
	}

	n := w.concurrency
	if n < 1 {
		n = 1
	}
	for i := 0; i < n; i++ {
		w.wg.Add(1)
		go func() {
			defer w.wg.Done()
			w.loop(ctx)
		}()
	}
	go func() {
		w.wg.Wait()
		close(w.stopped)
	}()
	slog.Info("worker: started", "concurrency", n)
}

// Stop signals the worker to stop and waits for in-flight jobs to finish.
func (w *Worker) Stop() {
	w.stopOnce.Do(func() { close(w.stopCh) })
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
			// Drain whatever is claimable before sleeping again.
			for w.processOne(ctx) {
				select {
				case <-ctx.Done():
					return
				case <-w.stopCh:
					return
				default:
				}
			}
		}
	}
}

// processOne claims and runs at most one job. It reports whether a job was run.
func (w *Worker) processOne(ctx context.Context) bool {
	job, err := w.store.Jobs.ClaimNext(ctx, "")
	if err != nil {
		slog.Error("worker: claim next job", "error", err)
		return false
	}
	if job == nil {
		return false
	}
	w.runClaimedJob(ctx, job)
	return true
}

// runClaimedJob executes a claimed job. A panic inside any task handler must
// never take the whole panel process down (the worker loop runs in-process),
// so recover here and fail just this job.
func (w *Worker) runClaimedJob(ctx context.Context, job *jobmodel.Job) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("worker: recovered from panic in job execution",
				"job_id", job.ID, "type", job.Type, "panic", r, "stack", string(debug.Stack()))
			_ = w.store.Jobs.MarkCompleted(context.Background(), job.ID, false, "", fmt.Sprintf("panic: %v", r))
			sender := notify.NewSender(w.store)
			sender.Send(context.Background(), "error", "Job panicked",
				fmt.Sprintf("Job %d (%s) panicked: %v", job.ID, job.Type, r))
		}
	}()

	slog.Info("worker: executing job", "id", job.ID, "type", job.Type, "attempts", job.Attempts)

	// Set initial progress for backup jobs
	if job.Type == jobmodel.TypeBackupCreate || job.Type == jobmodel.TypeDBBackupCreate {
		// Concurrency limit: max 1 concurrent backup job per server to save disk I/O and RAM.
		// This job was already claimed (status 'running'), so it is part of the
		// count; only *other* running backups should hold it back. Counting itself
		// re-queued every backup forever without ever calling the agent.
		const maxConcurrentBackups = 1
		if job.ServerID.Valid && job.ServerID.Int64 > 0 {
			running, _ := w.store.Jobs.CountRunningByServer(ctx, job.ServerID.Int64)
			others := running - 1
			if others >= maxConcurrentBackups {
				delay := time.Duration(others-maxConcurrentBackups+1) * 30 * time.Second
				retryAt := time.Now().Add(delay)
				_ = w.store.Jobs.ScheduleRetry(ctx, job.ID, retryAt)
				slog.Info("worker: backup concurrency limit, re-queued",
					"job_id", job.ID, "server_id", job.ServerID.Int64,
					"other_running", others, "limit", maxConcurrentBackups, "retry_in", delay)
				return
			}
		}

		var payloadData struct {
			BackupID int64 `json:"backup_id"`
		}
		if err := json.Unmarshal([]byte(job.Payload), &payloadData); err == nil && payloadData.BackupID > 0 {
			_ = w.store.Backups.UpdateProgress(ctx, payloadData.BackupID, "archiving...")
			_ = w.store.Backups.UpdateStatus(ctx, payloadData.BackupID, "running")
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
	// A database whose creation failed (e.g. the name already existed on
	// the server) has no record any more: never grant on that name.
	if job.Type == jobmodel.TypeDBUserCreate {
		var p struct {
			DatabaseID int64 `json:"database_id"`
		}
		if json.Unmarshal([]byte(job.Payload), &p) == nil && p.DatabaseID > 0 {
			if d, err := w.store.Databases.GetByID(ctx, p.DatabaseID); err == nil && d == nil {
				_ = w.store.Jobs.MarkCompleted(ctx, job.ID, false, "", "database record no longer exists")
				return
			}
		}
	}
	// site.create is also how a site's vhost is regenerated (PHP, app port,
	// security changes): the site's stored settings complete the payload,
	// and the site's status before the job decides what success means.
	var siteBefore *store.Site
	if job.Type == jobmodel.TypeSiteCreate && job.SiteID.Valid {
		siteBefore, _ = w.store.Sites.GetByID(ctx, job.SiteID.Int64)
		enriched, err := w.siteCreatePayload(ctx, siteBefore, dispatchPayload)
		if err != nil {
			_ = w.store.Jobs.MarkCompleted(ctx, job.ID, false, "", err.Error())
			slog.Warn("worker: site.create refused", "id", job.ID, "error", err)
			return
		}
		dispatchPayload = enriched
	}
	result, execErr := agent.Execute(ctx, job.Type, dispatchPayload)

	if execErr != nil {
		permanent := isPermanentJobError(job.Type, execErr, result)
		if permanent && job.Attempts < job.MaxAttempts {
			slog.Warn("worker: deterministic failure, not retrying", "id", job.ID, "type", job.Type, "error", execErr)
		}
		if job.Attempts < job.MaxAttempts && !permanent {
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

		// A site whose creation failed must not stay "pending" forever: show
		// it as failed so the operator sees it and can delete/recreate it.
		if job.Type == jobmodel.TypeSiteCreate && job.SiteID.Valid && siteIsNew(siteBefore) {
			if err := w.store.Sites.UpdateStatus(ctx, job.SiteID.Int64, "failed"); err != nil {
				slog.Error("worker: mark site failed", "id", job.SiteID.Int64, "error", err)
			}
		}

		// A database that could not be created must not stay recorded: its
		// name may belong to someone else on the server (grants, backups and
		// delete would then act on that database).
		// Only a definite "not created" (the name exists on the server or is
		// reserved) drops it; after a timeout the database may exist.
		if job.Type == jobmodel.TypeDatabaseCreate && databaseCreateRefused(execErr, result) {
			var p struct {
				DatabaseID int64 `json:"database_id"`
			}
			if json.Unmarshal([]byte(job.Payload), &p) == nil && p.DatabaseID > 0 {
				if err := w.store.Databases.Delete(ctx, p.DatabaseID); err != nil {
					slog.Error("worker: drop failed database record", "id", p.DatabaseID, "error", err)
				}
			}
		}

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
	case jobmodel.TypeSiteSuspend, jobmodel.TypeSiteResume:
		if job.SiteID.Valid {
			status := "active"
			if job.Type == jobmodel.TypeSiteSuspend {
				status = "suspended"
			}
			if err := w.store.Sites.UpdateStatus(ctx, job.SiteID.Int64, status); err != nil {
				slog.Error("worker: update site status", "id", job.SiteID.Int64, "error", err)
			}
		}
	case jobmodel.TypeSiteCreate:
		if job.SiteID.Valid {
			siteID := job.SiteID.Int64
			isNew := siteIsNew(siteBefore)
			// Only the initial creation activates a site: a vhost
			// regeneration must never change its status.
			if isNew {
				if err := w.store.Sites.UpdateStatus(ctx, siteID, "active"); err != nil {
					slog.Error("worker: update site status", "id", siteID, "error", err)
				}
			}

			// An agent that predates server-level access rules silently
			// ignores the allowlist and leaves PHP URLs open to blocked IPs.
			if result != nil && !result.AccessRules && w.siteHasIPRules(ctx, siteID) {
				msg := "The agent on this site's server is too old to enforce its IP rules (allowlist, blocks on PHP URLs). Update the agent from Servers."
				slog.Warn("worker: agent does not apply IP rules", "site_id", siteID)
				if s, _ := w.store.Sites.GetByID(ctx, siteID); s != nil {
					w.store.NotifyAdmins(ctx, "agent_outdated", "Agent update needed", s.Domain+": "+msg, "warning", "site", siteID)
				}
			}

			// Per-site PHP-FPM pools: remember that this site went through
			// an agent that knows them (see migratePHPPools).
			if result != nil && result.PHPPool != nil {
				v := "0"
				if *result.PHPPool {
					v = "1"
				}
				_ = store.SetSetting(ctx, w.store.DB(), phpPoolSettingKey(siteID), v)
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

			// Auto-enqueue SSL issuance for new sites only (not on every
			// vhost regeneration).
			site, err := w.store.Sites.GetByID(ctx, siteID)
			if isNew && err == nil && site != nil && site.Domain != "" && site.SSLStatus.String != "active" {
				// Get owner email for Let's Encrypt
				// No owner e-mail: the agent registers without a contact
				// address (it used to register every install's Let's
				// Encrypt account with a Sigarda address).
				email := ""
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

	case jobmodel.TypeSiteTransfer:
		// The files are in place on the server: now the panel follows.
		var p struct {
			SiteID     int64  `json:"site_id"`
			NewOwnerID int64  `json:"new_owner_id"`
			Domain     string `json:"domain"`
			SystemUser string `json:"system_user"`
		}
		if json.Unmarshal([]byte(job.Payload), &p) == nil && p.SiteID > 0 && p.NewOwnerID > 0 {
			before, _ := w.store.Sites.GetByID(ctx, p.SiteID)
			if err := w.store.Sites.Transfer(ctx, p.SiteID, p.NewOwnerID); err != nil {
				slog.Error("worker: transfer site record", "site_id", p.SiteID, "error", err)
				break
			}
			if site, _ := w.store.Sites.GetByID(ctx, p.SiteID); site != nil {
				regen, _ := json.Marshal(map[string]any{
					"site_id": site.ID, "name": site.Name, "domain": site.Domain,
					"runtime": site.Runtime, "root_path": site.RootPath, "vhost_regen": true,
				})
				_ = w.store.Jobs.Create(ctx, &jobmodel.Job{
					ServerID: job.ServerID, SiteID: job.SiteID, Type: jobmodel.TypeSiteCreate,
					Description: "Point the vhost of " + site.Domain + " at its new root", Payload: string(regen),
				})
			}
			if before != nil && before.OwnerID.Valid {
				w.store.Notify(ctx, before.OwnerID.Int64, "site_transferred", "Site Transferred", fmt.Sprintf("Transferred %s to %s", p.Domain, p.SystemUser), "info", "site", p.SiteID)
			}
			w.store.Notify(ctx, p.NewOwnerID, "site_received", "Site Received", fmt.Sprintf("You received %s", p.Domain), "success", "site", p.SiteID)
		}

	case jobmodel.TypeSSLIssue, jobmodel.TypeSSLRenew:
		// Both must record the new expiry: if a renewal left the old date in
		// place, the scheduler would keep re-queuing it (and alerting) every minute.
		if job.SiteID.Valid {
			expires := time.Now().Add(90 * 24 * time.Hour)
			if result != nil {
				if t, ok := parseCertEndDate(result.Expires); ok {
					expires = t
				}
			}
			if err := w.store.Sites.UpdateSSL(ctx, job.SiteID.Int64, "active", "Let's Encrypt", expires); err != nil {
				slog.Error("worker: update ssl status", "id", job.SiteID.Int64, "error", err)
			}
		}

	case jobmodel.TypeBackupCreate, jobmodel.TypeDBBackupCreate:
		// Parse backup_id from job payload and persist actual local/S3 path from agent result.
		var payloadData struct {
			BackupID int64 `json:"backup_id"`
			S3Config *struct {
				Bucket    string `json:"bucket"`
				Endpoint  string `json:"endpoint"`
				AccessKey string `json:"access_key"`
			} `json:"s3_config"`
		}
		rawPayload := job.Payload
		if dec, derr := secrets.DecryptPayload(rawPayload); derr == nil {
			rawPayload = dec // s3_config's keys are stored encrypted
		}
		if err := json.Unmarshal([]byte(rawPayload), &payloadData); err == nil && payloadData.BackupID > 0 {
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
					// Remember which provider holds the copy: deleting or
					// restoring it later must not follow a changed default.
					if c := payloadData.S3Config; c != nil {
						if pid := w.store.ProviderIDForS3(ctx, c.Bucket, c.Endpoint, c.AccessKey); pid > 0 {
							_ = w.store.Backups.SetProvider(ctx, payloadData.BackupID, pid)
						}
					}
					// The local copy is no longer needed: the site's agent removes
					// it, and the worker clears storage_path once it is gone.
					if err := w.store.QueueBackupFileDelete(ctx, payloadData.BackupID, job.SiteID.Int64, localPath, true); err != nil {
						slog.Warn("worker: queue removal of local backup after S3 upload", "id", payloadData.BackupID, "error", err)
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
		// Retention is the configured one (expiry per backup config, the
		// global retention settings in the scheduler); a fixed "keep the
		// latest 7" here used to override it, also for "keep forever".

	case jobmodel.TypeBackupDelete:
		var p struct {
			BackupID int64 `json:"backup_id"`
			Clear    bool  `json:"clear_storage_path"`
		}
		if json.Unmarshal([]byte(job.Payload), &p) == nil && p.Clear && p.BackupID > 0 {
			if err := w.store.Backups.ClearStoragePath(ctx, p.BackupID); err != nil {
				slog.Error("worker: clear storage_path", "id", p.BackupID, "error", err)
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
	deleteS3Object(ctx, provider, s3Path)
}

func deleteS3Object(ctx context.Context, provider *store.BackupStorageProvider, s3Path string) {
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

// parseCertEndDate parses openssl's "-enddate" value ("Jan  2 15:04:05 2027 GMT").
func parseCertEndDate(s string) (time.Time, bool) {
	s = strings.Join(strings.Fields(s), " ")
	if s == "" {
		return time.Time{}, false
	}
	t, err := time.Parse("Jan 2 15:04:05 2006 MST", s)
	if err != nil {
		return time.Time{}, false
	}
	return t.UTC(), true
}

// siteIsNew reports whether a site.create job creates the site (rather
// than regenerating the vhost of an existing one).
func siteIsNew(site *store.Site) bool {
	if site == nil {
		return true
	}
	switch site.Status {
	case "", "pending", "creating", "failed":
		return true
	}
	return false
}

// siteHasIPRules reports whether the site has blocked IPs or IP restrictions.
func (w *Worker) siteHasIPRules(ctx context.Context, siteID int64) bool {
	if sec, _ := w.store.Security.GetBySiteID(ctx, siteID); sec != nil && len(sec.BlockedIPs) > 0 {
		return true
	}
	rules, _ := w.store.IPRestrictions.List(ctx, siteID)
	return len(rules) > 0
}

// dedupeIPs canonicalises IPs/CIDRs and drops duplicates and invalid entries.
func dedupeIPs(in []string) []string {
	out := []string{}
	for _, v := range in {
		if norm, err := nginx.NormalizeBlockedIPs([]string{v}); err == nil && len(norm) == 1 && !slices.Contains(out, norm[0]) {
			out = append(out, norm[0])
		}
	}
	return out
}

// siteCreatePayload completes a site.create payload from the stored site:
// fields the caller did not send (app port, proxy URL, PHP settings) and the
// site's security settings, so a regeneration never drops them. Jobs for a
// suspended site are refused: the agent would replace the suspension
// placeholder with the live vhost.
func (w *Worker) siteCreatePayload(ctx context.Context, site *store.Site, raw string) (string, error) {
	if site == nil {
		return raw, nil
	}
	if site.Status == "suspended" {
		return "", fmt.Errorf("site %s is suspended; resume it before changing its configuration", site.Domain)
	}
	var p map[string]any
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return raw, nil
	}
	setIfMissing := func(key string, v any, ok bool) {
		if _, has := p[key]; !has && ok {
			p[key] = v
		}
	}
	setIfMissing("port", site.AppPort.Int64, site.AppPort.Valid && site.AppPort.Int64 > 0)
	setIfMissing("proxy_pass_url", site.ProxyPassURL.String, site.ProxyPassURL.Valid && site.ProxyPassURL.String != "")
	setIfMissing("php_version", site.PHPVersion.String, site.PHPVersion.Valid && site.PHPVersion.String != "")
	setIfMissing("php_ini", site.PHPIni.String, site.PHPIni.Valid && site.PHPIni.String != "")
	// The site's own PHP-FPM pool runs as its owner: regenerations need it
	// too (an existing root keeps its ownership on the agent).
	setIfMissing("system_user", site.SystemUser.String, site.SystemUser.Valid && site.SystemUser.String != "")
	// Settings stored before the allowlist existed are dropped, not sent:
	// the vhost keeps regenerating with the allowed ones.
	if raw, ok := p["php_ini"].(string); ok {
		if kept, _ := siteinput.FilterPHPIni(raw); kept != "" {
			p["php_ini"] = kept
		} else {
			delete(p, "php_ini")
		}
	}
	if !siteIsNew(site) {
		// An existing site: regenerate the vhost only (no starter files, no
		// container re-creation).
		p["vhost_regen"] = true
	}
	// The site's IP restrictions join its security settings: "deny" rules
	// with the blocked IPs, "allow" rules as an allowlist (everyone else
	// gets 403).
	var blocked, allowed []string
	sec, _ := w.store.Security.GetBySiteID(ctx, site.ID)
	if sec != nil {
		blocked = append(blocked, sec.BlockedIPs...)
	}
	if rules, err := w.store.IPRestrictions.List(ctx, site.ID); err == nil {
		for _, r := range rules {
			if r.Action == "allow" {
				allowed = append(allowed, r.IP)
			} else {
				blocked = append(blocked, r.IP)
			}
		}
	}
	if sec != nil || len(blocked) > 0 || len(allowed) > 0 {
		security := map[string]any{"blocked_ips": dedupeIPs(blocked), "allowed_ips": dedupeIPs(allowed)}
		if sec != nil {
			security["basic_auth_enabled"] = sec.BasicAuthEnabled
			security["basic_auth_username"] = sec.BasicAuthUsername
			security["basic_auth_hash"] = sec.BasicAuthPassword
		}
		p["security"] = security
	}
	b, err := json.Marshal(p)
	if err != nil {
		return raw, nil
	}
	return string(b), nil
}

// databaseCreateRefused reports the agent's definite refusals of a
// database.create: the name already exists on the server, or is reserved.
func databaseCreateRefused(execErr error, result *agentclient.TaskResult) bool {
	var b strings.Builder
	if execErr != nil {
		b.WriteString(execErr.Error())
	}
	if result != nil {
		b.WriteString(" " + result.Error + " " + result.Output)
	}
	msg := strings.ToLower(b.String())
	return strings.Contains(msg, "already exists on the server") || strings.Contains(msg, "reserved database name")
}
