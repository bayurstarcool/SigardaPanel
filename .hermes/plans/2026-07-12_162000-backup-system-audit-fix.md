# Backup System — Full Audit Fix & Multi-Site Efficiency Plan

> **For Hermes:** Use subagent-driven-development skill to implement this plan task-by-task.

**Goal:** Fix all backup bugs (stuck states, missing retries, no cleanup) and add efficiency features for 50+ site backups with minimal failures.

**Architecture:** Backup flow is API → Worker → Agent. The worker polls jobs every 2s, claims next queued job, executes via agent, and updates backup record status. The scheduler runs periodically to trigger scheduled backups and cleanup.

**Tech Stack:** Go (Echo), SQLite (WAL), SvelteKit frontend

---

## Current Bug Summary

| # | Bug | Impact | Fix Complexity |
|---|-----|--------|----------------|
| 1 | Backup record stuck "running" when job fails | ✅ ALREADY FIXED (worker.go) | Done |
| 2 | No `MaxAttempts` on backup jobs → no retries | HIGH — single failure = permanent loss | Low |
| 3 | No stuck job/backup cleanup mechanism | HIGH — stuck "running" forever | Medium |
| 4 | No deduplication — concurrent backups for same site | MEDIUM — disk/bandwidth waste | Medium |
| 5 | No concurrency limit — 50 backups simultaneously | MEDIUM — server overload | Medium |
| 6 | Orphaned backup records when job creation fails | LOW — DB clutter | Low |

---

## Task 1: Add MaxAttempts to All Backup Job Creations

**Objective:** Ensure all backup jobs have retry capability (MaxAttempts: 2)

**Files:**
- Modify: `internal/jobs/scheduler.go:320-326` (createBackupJob)
- Modify: `internal/jobs/scheduler.go:386` (processBackupConfig both branch)
- Modify: `internal/apihttp/backups.go:332-338` (bulkCreate files)
- Modify: `internal/apihttp/backups.go:423-429` (bulkCreate database)
- Modify: `internal/apihttp/backup_configs.go:316` (run handler)
- Modify: `internal/apihttp/backup_configs.go:393-397` (run handler db)

**Step 1: Fix createBackupJob helper**

In `internal/jobs/scheduler.go`, the `createBackupJob` function at line 320 creates a job without MaxAttempts:

```go
// CURRENT (line 320-326):
job := &jobmodel.Job{
    ServerID: sql.NullInt64{Int64: site.ServerID, Valid: true},
    SiteID:   sql.NullInt64{Int64: site.ID, Valid: true},
    Type:     jobmodel.TypeBackupCreate,
    Status:   jobmodel.StatusQueued,
    Payload:  string(payload),
}

// FIX — add MaxAttempts: 2:
job := &jobmodel.Job{
    ServerID:    sql.NullInt64{Int64: site.ServerID, Valid: true},
    SiteID:      sql.NullInt64{Int64: site.ID, Valid: true},
    Type:        jobmodel.TypeBackupCreate,
    Status:      jobmodel.StatusQueued,
    Payload:     string(payload),
    MaxAttempts: 2,
}
```

**Step 2: Fix processBackupConfig `both` branch**

In `scheduler.go` line 386, same fix:

```go
// CURRENT:
job := &jobmodel.Job{ServerID: ..., SiteID: ..., Type: jobmodel.TypeBackupCreate, Status: jobmodel.StatusQueued, Payload: string(payload)}

// FIX:
job := &jobmodel.Job{ServerID: ..., SiteID: ..., Type: jobmodel.TypeBackupCreate, Status: jobmodel.StatusQueued, Payload: string(payload), MaxAttempts: 2}
```

**Step 3: Fix processSchedule DB backup jobs**

In `scheduler.go` lines 227-233 and 280-286, add MaxAttempts: 2 to TypeDBBackupCreate jobs.

**Step 4: Fix bulkCreate in backups.go**

In `backups.go` lines 332-338 (files) and 423-429 (database), add MaxAttempts: 2.

**Step 5: Fix backupConfigs.run handler**

In `backup_configs.go` line 316 (both/files) and line 393-397 (database), add MaxAttempts: 2.

**Step 6: Verify build**

```bash
cd /root/SigardaPanel-Enterprise && go build ./...
```

**Step 7: Run tests**

```bash
go test ./internal/jobs/ -v -count=1
go test ./internal/store/ -run "TestBackup" -v -count=1
```

---

## Task 2: Add Stuck Job/Backup Cleanup in Scheduler

**Objective:** Automatically mark jobs and backups stuck "running" for > 30 minutes as "failed"

**Files:**
- Modify: `internal/jobs/scheduler.go` — add `runStuckCleanup()` function
- Modify: `internal/jobs/scheduler.go:40` — call from `runTick()`
- Modify: `internal/store/jobs.go` — add `CleanupStuckRunning()` method
- Modify: `internal/store/backups.go` — add `CleanupStuckRunning()` method

**Step 1: Add CleanupStuckRunning to jobs store**

In `internal/store/jobs.go`, add to `JobRepository` interface:

```go
CleanupStuckRunning(ctx context.Context, staleAfter time.Duration) (int64, error)
```

Implementation:

```go
func (s *SQLiteJobStore) CleanupStuckRunning(ctx context.Context, staleAfter time.Duration) (int64, error) {
    cutoff := time.Now().Add(-staleAfter)
    res, err := s.db.ExecContext(ctx,
        `UPDATE jobs SET status = 'failed', error = 'cleanup: stuck in running state'
         WHERE status = 'running' AND updated_at < ?`, cutoff)
    if err != nil {
        return 0, fmt.Errorf("cleanup stuck jobs: %w", err)
    }
    return res.RowsAffected()
}
```

**Step 2: Add CleanupStuckRunning to backups store**

In `internal/store/backups.go`, add to `BackupRepository` interface:

```go
CleanupStuckRunning(ctx context.Context, staleAfter time.Duration) (int64, error)
```

Implementation:

```go
func (s *SQLiteBackupStore) CleanupStuckRunning(ctx context.Context, staleAfter time.Duration) (int64, error) {
    cutoff := time.Now().Add(-staleAfter)
    res, err := s.db.ExecContext(ctx,
        `UPDATE backups SET status = 'failed', error = 'cleanup: stuck in running state', progress = 'failed', completed_at = CURRENT_TIMESTAMP
         WHERE status = 'running' AND created_at < ?`, cutoff)
    if err != nil {
        return 0, fmt.Errorf("cleanup stuck backups: %w", err)
    }
    return res.RowsAffected()
}
```

**Step 3: Add runStuckCleanup to scheduler**

In `scheduler.go`, add function:

```go
func runStuckCleanup(ctx context.Context, s *store.Store) {
    staleDuration := 30 * time.Minute

    // Clean up stuck jobs
    if n, err := s.Jobs.CleanupStuckRunning(ctx, staleDuration); err != nil {
        slog.Warn("scheduler: cleanup stuck jobs failed", "error", err)
    } else if n > 0 {
        slog.Warn("scheduler: cleaned up stuck jobs", "count", n)
        sender := notify.NewSender(s)
        sender.Send(ctx, "warning", "Stuck Jobs Cleaned",
            fmt.Sprintf("%d job(s) stuck in 'running' for >30min were marked as failed.", n))
    }

    // Clean up stuck backups
    if n, err := s.Backups.CleanupStuckRunning(ctx, staleDuration); err != nil {
        slog.Warn("scheduler: cleanup stuck backups failed", "error", err)
    } else if n > 0 {
        slog.Warn("scheduler: cleaned up stuck backups", "count", n)
        sender := notify.NewSender(s)
        sender.Send(ctx, "warning", "Stuck Backups Cleaned",
            fmt.Sprintf("%d backup(s) stuck in 'running' for >30min were marked as failed.", n))
    }
}
```

**Step 4: Call from runTick**

In `scheduler.go:runTick()`, add after `runCleanup()`:

```go
// Clean up stuck jobs and backups
runStuckCleanup(ctx, s)
```

**Step 5: Verify build and tests**

```bash
go build ./...
go test ./internal/store/ -run "TestBackup|TestJob" -v -count=1
```

---

## Task 3: Add Backup Deduplication

**Objective:** Skip creating backup job if one is already queued/running for the same site

**Files:**
- Modify: `internal/store/jobs.go` — add `HasActiveJob()` method
- Modify: `internal/jobs/scheduler.go` — check before creating backup jobs

**Step 1: Add HasActiveJob to jobs store**

```go
// In JobRepository interface:
HasActiveJob(ctx context.Context, jobType string, siteID int64) (bool, error)

// Implementation:
func (s *SQLiteJobStore) HasActiveJob(ctx context.Context, jobType string, siteID int64) (bool, error) {
    var count int
    err := s.db.QueryRowContext(ctx,
        `SELECT COUNT(*) FROM jobs WHERE type = ? AND site_id = ? AND status IN ('queued', 'running')`,
        jobType, siteID).Scan(&count)
    if err != nil {
        return false, fmt.Errorf("check active job: %w", err)
    }
    return count > 0, nil
}
```

**Step 2: Add dedup check in processBackupConfig**

In `scheduler.go:processBackupConfig()`, before creating each backup job:

```go
// Deduplicate: skip if backup job already active for this site
active, _ := s.Jobs.HasActiveJob(ctx, jobmodel.TypeBackupCreate, siteID)
if active {
    slog.Info("scheduler: skip backup (already active)", "config_id", cfg.ID, "site_id", siteID)
    continue
}
```

**Step 3: Verify build**

```bash
go build ./...
```

---

## Task 4: Add Concurrency Limit for Backup Jobs

**Objective:** Max 5 concurrent backup jobs per server to prevent I/O/network overload

**Files:**
- Modify: `internal/store/jobs.go` — add `CountRunningByServer()` method
- Modify: `internal/jobs/worker.go` — check concurrency before executing

**Step 1: Add CountRunningByServer to jobs store**

```go
// In JobRepository interface:
CountRunningByServer(ctx context.Context, serverID int64) (int, error)

// Implementation:
func (s *SQLiteJobStore) CountRunningByServer(ctx context.Context, serverID int64) (int, error) {
    var count int
    err := s.db.QueryRowContext(ctx,
        `SELECT COUNT(*) FROM jobs WHERE server_id = ? AND status = 'running'
         AND type IN ('backup.create', 'backup.database')`, serverID).Scan(&count)
    if err != nil {
        return 0, fmt.Errorf("count running backups: %w", err)
    }
    return count, nil
}
```

**Step 2: Add concurrency check in worker**

In `worker.go:processOne()`, after claiming the job but before executing:

```go
const maxConcurrentBackups = 5

// Check concurrency limit for backup jobs
if (job.Type == jobmodel.TypeBackupCreate || job.Type == jobmodel.TypeDBBackupCreate) &&
   job.ServerID.Valid {
    running, _ := w.store.Jobs.CountRunningByServer(ctx, job.ServerID.Int64)
    if running >= maxConcurrentBackups {
        // Re-queue with delay instead of executing
        delay := time.Duration(running-maxConcurrentBackups+1) * 30 * time.Second
        retryAt := time.Now().Add(delay)
        _ = w.store.Jobs.ScheduleRetry(ctx, job.ID, retryAt)
        slog.Info("worker: backup concurrency limit, re-queued",
            "job_id", job.ID, "server_id", job.ServerID.Int64,
            "running", running, "limit", maxConcurrentBackups, "retry_in", delay)
        return
    }
}
```

**Step 3: Verify build**

```bash
go build ./...
```

---

## Task 5: Add Orphaned Backup Cleanup

**Objective:** Clean up backup records stuck in "pending" (orphaned when job creation failed)

**Files:**
- Modify: `internal/store/backups.go` — add `CleanupOrphanedPending()` method
- Modify: `internal/jobs/scheduler.go` — call from `runCleanup()`

**Step 1: Add CleanupOrphanedPending**

```go
// In BackupRepository interface:
CleanupOrphanedPending(ctx context.Context, olderThan time.Duration) (int64, error)

// Implementation:
func (s *SQLiteBackupStore) CleanupOrphanedPending(ctx context.Context, olderThan time.Duration) (int64, error) {
    cutoff := time.Now().Add(-olderThan)
    res, err := s.db.ExecContext(ctx,
        `DELETE FROM backups WHERE status = 'pending' AND created_at < ?`, cutoff)
    if err != nil {
        return 0, fmt.Errorf("cleanup orphaned pending backups: %w", err)
    }
    return res.RowsAffected()
}
```

**Step 2: Call from runCleanup**

In `scheduler.go:runCleanup()`, add at the end:

```go
// Clean up orphaned pending backups (no job created for them)
if n, err := s.Backups.CleanupOrphanedPending(ctx, 1*time.Hour); err != nil {
    slog.Warn("scheduler: cleanup orphaned pending backups failed", "error", err)
} else if n > 0 {
    slog.Info("scheduler: cleaned up orphaned pending backups", "count", n)
}
```

**Step 3: Verify build**

```bash
go build ./...
```

---

## Task 6: Update Backup Record Progress During Execution

**Objective:** Update backup progress from "archiving..." to "uploading..." when S3 upload starts

**Files:**
- Modify: `internal/agenthttp/router.go:882-910` — update progress before S3 upload

**Step 1: Update progress before S3 upload**

In `router.go`, after the tar succeeds and before S3 upload:

```go
// After line 872 (after stat), before S3 upload:
if payload.S3Config != nil && payload.S3Config.Bucket != "" {
    // Progress is updated by worker, but we can log the phase
    slog.Info("agent: backup archived, starting S3 upload", "backup_id", payload.BackupID, "size", info.Size())
}
```

Note: The worker already updates progress. The agent doesn't have direct DB access — it returns the result to the worker which updates the DB. So this is more of a logging improvement.

**Step 2: Update worker to set "uploading" progress**

In `worker.go`, after agent returns result but before MarkCompleted:

```go
// After line 262 (after result is parsed), before MarkCompleted:
if result != nil && result.S3Path != "" {
    _ = w.store.Backups.UpdateProgress(ctx, payloadData.BackupID, "uploading to cloud...")
}
```

**Step 3: Verify build**

```bash
go build ./...
```

---

## Task 7: Build, Test, and Deploy

**Objective:** Full build, test suite, and restart service with new binary

**Step 1: Full build**

```bash
cd /root/SigardaPanel-Enterprise && export PATH=/usr/local/go/bin:$PATH
go build ./...
go build -o sigardapanel ./cmd/sigardapanel
```

**Step 2: Run all backup-related tests**

```bash
go test ./internal/store/ -run "TestBackup|TestJob" -v -count=1
go test ./internal/jobs/ -v -count=1
```

**Step 3: Fix any stuck records in DB**

```bash
sqlite3 sigardapanel.db "UPDATE backups SET status='failed', progress='failed', error='manual cleanup' WHERE status='running';"
sqlite3 sigardapanel.db "UPDATE jobs SET status='failed', error='manual cleanup' WHERE status='running';"
```

**Step 4: Restart service**

```bash
tmux send-keys -t sigarda:0 C-c
sleep 1
tmux send-keys -t sigarda:0 './sigardapanel dev' Enter
sleep 3
ss -tlnp | grep -E "7700|7710"
```

**Step 5: Verify no stuck records**

```bash
sqlite3 sigardapanel.db "SELECT COUNT(*) FROM backups WHERE status='running';"
sqlite3 sigardapanel.db "SELECT COUNT(*) FROM jobs WHERE status='running';"
```

---

## Files Changed Summary

| File | Changes |
|------|---------|
| `internal/jobs/worker.go` | Add concurrency limit check, progress update |
| `internal/jobs/scheduler.go` | Add stuck cleanup, dedup check, MaxAttempts |
| `internal/store/jobs.go` | Add CleanupStuckRunning, HasActiveJob, CountRunningByServer |
| `internal/store/backups.go` | Add CleanupStuckRunning, CleanupOrphanedPending |
| `internal/apihttp/backups.go` | Add MaxAttempts to bulkCreate jobs |
| `internal/apihttp/backup_configs.go` | Add MaxAttempts to run handler jobs |
| `internal/agenthttp/router.go` | Logging improvement for S3 upload phase |

---

## Verification Checklist

- [ ] `go build ./...` succeeds
- [ ] `go test ./internal/store/ -run "TestBackup"` — all pass
- [ ] `go test ./internal/jobs/` — all pass
- [ ] No stuck "running" jobs after 30 minutes
- [ ] Backup retry works (MaxAttempts: 2)
- [ ] Deduplication prevents concurrent backups for same site
- [ ] Concurrency limit prevents > 5 simultaneous backups per server
- [ ] Orphaned pending backups cleaned up after 1 hour
- [ ] Service restarted with new binary
- [ ] Frontend shows correct status (completed/failed/running)

---

## Risk Assessment

| Risk | Mitigation |
|------|-----------|
| MaxAttempts too high → wasted resources | 2 is conservative, 30s retry delay |
| Stuck cleanup too aggressive | 30min threshold is generous for real backups |
| Dedup too strict → missed backups | Only skips if same site has queued/running |
| Concurrency limit too low | 5 is reasonable for single server I/O |
| SQLite WAL contention | All operations are short UPDATE/SELECT, low risk |

---

## Open Questions

1. **Should backup configs have per-site staggering?** — Currently all sites in a config run simultaneously. Could add configurable delay (e.g., 30s between sites).
2. **Should we add backup size limits?** — Warn if site > 10GB before starting backup.
3. **Should we add backup duration tracking?** — Track how long each backup takes for capacity planning.
4. **Should we add S3 upload progress tracking?** — Currently agent returns only success/failure, no progress percentage.
