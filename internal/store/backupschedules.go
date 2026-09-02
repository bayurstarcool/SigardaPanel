package store

// DEPRECATED: BackupSchedule represents a legacy per-site backup schedule.
// Use BackupConfig (backup_configs.go) instead. This entire file is kept
// for backward compatibility and will be removed in a future release.

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// BackupSchedule represents a scheduled backup configuration.
type BackupSchedule struct {
	ID          int64
	SiteID      int64
	ProviderID  sql.NullInt64
	CronExpr    string
	BackupType  string // files, database, both
	Enabled     bool
	LastRunAt   sql.NullTime
	NextRunAt   sql.NullTime
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Deprecated: BackupScheduleRepository defines persistence operations for
// legacy per-site backup schedules. Use BackupConfigRepository instead.
type BackupScheduleRepository interface {
	Create(ctx context.Context, s *BackupSchedule) error
	GetByID(ctx context.Context, id int64) (*BackupSchedule, error)
	ListBySite(ctx context.Context, siteID int64) ([]BackupSchedule, error)
	Update(ctx context.Context, s *BackupSchedule) error
	Delete(ctx context.Context, id int64) error
	ListDue(ctx context.Context) ([]BackupSchedule, error)
	MarkRun(ctx context.Context, id int64) error
}

// SQLiteBackupScheduleStore implements BackupScheduleRepository.
type SQLiteBackupScheduleStore struct {
	db *sql.DB
}

// NewBackupScheduleStore returns a new store.
func NewBackupScheduleStore(db *sql.DB) *SQLiteBackupScheduleStore {
	return &SQLiteBackupScheduleStore{db: db}
}

func (s *SQLiteBackupScheduleStore) Create(ctx context.Context, sched *BackupSchedule) error {
	if sched.BackupType == "" {
		sched.BackupType = "files"
	}
	enabled := 0
	if sched.Enabled {
		enabled = 1
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO backup_schedules (site_id, provider_id, cron_expr, backup_type, enabled) VALUES (?, ?, ?, ?, ?)`,
		sched.SiteID, sched.ProviderID, sched.CronExpr, sched.BackupType, enabled,
	)
	if err != nil {
		return fmt.Errorf("insert backup schedule: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("last insert id: %w", err)
	}
	sched.ID = id
	return nil
}

func (s *SQLiteBackupScheduleStore) GetByID(ctx context.Context, id int64) (*BackupSchedule, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, site_id, provider_id, cron_expr, backup_type, enabled, last_run_at, next_run_at, created_at, updated_at
		 FROM backup_schedules WHERE id = ?`, id)
	return scanBackupSchedule(row)
}

func (s *SQLiteBackupScheduleStore) ListBySite(ctx context.Context, siteID int64) ([]BackupSchedule, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, site_id, provider_id, cron_expr, backup_type, enabled, last_run_at, next_run_at, created_at, updated_at
		 FROM backup_schedules WHERE site_id = ? ORDER BY id DESC`, siteID)
	if err != nil {
		return nil, fmt.Errorf("query backup schedules by site: %w", err)
	}
	defer rows.Close()
	var out []BackupSchedule
	for rows.Next() {
		var bs BackupSchedule
		if err := scanBackupScheduleRows(rows, &bs); err != nil {
			return nil, err
		}
		out = append(out, bs)
	}
	return out, rows.Err()
}

func (s *SQLiteBackupScheduleStore) Update(ctx context.Context, sched *BackupSchedule) error {
	enabled := 0
	if sched.Enabled {
		enabled = 1
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE backup_schedules SET provider_id = ?, cron_expr = ?, backup_type = ?, enabled = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		sched.ProviderID, sched.CronExpr, sched.BackupType, enabled, sched.ID,
	)
	if err != nil {
		return fmt.Errorf("update backup schedule: %w", err)
	}
	return nil
}

func (s *SQLiteBackupScheduleStore) Delete(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM backup_schedules WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete backup schedule: %w", err)
	}
	return nil
}

func (s *SQLiteBackupScheduleStore) ListDue(ctx context.Context) ([]BackupSchedule, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, site_id, provider_id, cron_expr, backup_type, enabled, last_run_at, next_run_at, created_at, updated_at
		 FROM backup_schedules WHERE enabled = 1 AND (next_run_at IS NULL OR next_run_at <= CURRENT_TIMESTAMP)`)
	if err != nil {
		return nil, fmt.Errorf("query due backup schedules: %w", err)
	}
	defer rows.Close()
	var out []BackupSchedule
	for rows.Next() {
		var bs BackupSchedule
		if err := scanBackupScheduleRows(rows, &bs); err != nil {
			return nil, err
		}
		out = append(out, bs)
	}
	return out, rows.Err()
}

// MarkRun updates last_run_at to now and calculates next_run_at from the cron expression.
func (s *SQLiteBackupScheduleStore) MarkRun(ctx context.Context, id int64) error {
	// Get current cron_expr to calculate next_run_at
	var cronExpr string
	err := s.db.QueryRowContext(ctx, `SELECT cron_expr FROM backup_schedules WHERE id = ?`, id).Scan(&cronExpr)
	if err != nil {
		return fmt.Errorf("get cron expr: %w", err)
	}
	now := time.Now()
	nextRun := NextCronRun(cronExpr, now)
	_, err = s.db.ExecContext(ctx,
		`UPDATE backup_schedules SET last_run_at = ?, next_run_at = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		now, nextRun, id,
	)
	if err != nil {
		return fmt.Errorf("mark run: %w", err)
	}
	return nil
}

// ─── Cron Parser ────────────────────────────────────────────

// NextCronRun calculates the next run time given a 5-field cron expression.
// Fields: minute hour day-of-month month day-of-week
// Supports: exact values, *, */N steps, comma-separated lists.
func NextCronRun(cronExpr string, after time.Time) time.Time {
	fields := strings.Fields(cronExpr)
	if len(fields) != 5 {
		return after.Add(24 * time.Hour) // fallback: 1 day
	}

	minutes := parseCronField(fields[0], 0, 59)
	hours := parseCronField(fields[1], 0, 23)
	daysOfMonth := parseCronField(fields[2], 1, 31)
	months := parseCronField(fields[3], 1, 12)
	daysOfWeek := parseCronField(fields[4], 0, 6) // 0=Sunday

	// Search forward up to ~366 days to find next match
	next := after.Add(1 * time.Minute)
	next = time.Date(next.Year(), next.Month(), next.Day(), next.Hour(), next.Minute(), 0, 0, next.Location())

	for i := 0; i < 525960; i++ { // ~1 year of minutes
		if !containsInt(months, int(next.Month())) {
			// Jump to start of next valid month
			next = time.Date(next.Year(), next.Month()+1, 1, 0, 0, 0, 0, next.Location())
			continue
		}

		dayMatch := containsInt(daysOfMonth, next.Day()) || (containsInt(daysOfMonth, 0) && false)
		// Day-of-week: 0=Sun..6=Sat; time.Weekday: 0=Sun..6=Sat
		weekMatch := containsInt(daysOfWeek, int(next.Weekday()))

		// Standard cron: if both day-of-month and day-of-week are restricted (not *),
		// match either. If one is * and the other isn't, match the restricted one.
		// Both fields default to having all values when set to *, so we check size.
		domAll := len(daysOfMonth) == 31
		dowAll := len(daysOfWeek) == 7

		matched := false
		if domAll && dowAll {
			matched = true
		} else if domAll && !dowAll {
			matched = weekMatch
		} else if !domAll && dowAll {
			matched = dayMatch
		} else {
			matched = dayMatch || weekMatch
		}

		if !matched {
			// Jump to next day
			next = time.Date(next.Year(), next.Month(), next.Day()+1, 0, 0, 0, 0, next.Location())
			continue
		}

		if containsInt(hours, next.Hour()) && containsInt(minutes, next.Minute()) {
			return next
		}

		// Advance by 1 minute
		next = next.Add(1 * time.Minute)
	}

	return after.Add(24 * time.Hour) // fallback
}

func parseCronField(field string, min, max int) []int {
	set := make(map[int]bool)

	if field == "*" {
		for i := min; i <= max; i++ {
			set[i] = true
		}
		return toSlice(set)
	}

	// Handle comma-separated parts
	for _, part := range strings.Split(field, ",") {
		part = strings.TrimSpace(part)
		if strings.Contains(part, "/") {
			parts := strings.SplitN(part, "/", 2)
			var start, end int
			if parts[0] == "*" {
				start = min
				end = max
			} else {
				start, _ = strconv.Atoi(parts[0])
				end = max
			}
			step, _ := strconv.Atoi(parts[1])
			if step <= 0 {
				step = 1
			}
			for i := start; i <= end; i += step {
				if i >= min && i <= max {
					set[i] = true
				}
			}
		} else if strings.Contains(part, "-") {
			bounds := strings.SplitN(part, "-", 2)
			lo, _ := strconv.Atoi(bounds[0])
			hi, _ := strconv.Atoi(bounds[1])
			for i := lo; i <= hi; i++ {
				if i >= min && i <= max {
					set[i] = true
				}
			}
		} else {
			v, _ := strconv.Atoi(part)
			if v >= min && v <= max {
				set[v] = true
			}
		}
	}

	return toSlice(set)
}

func toSlice(m map[int]bool) []int {
	out := make([]int, 0, len(m))
	for v := range m {
		out = append(out, v)
	}
	return out
}

func containsInt(slice []int, val int) bool {
	for _, v := range slice {
		if v == val {
			return true
		}
	}
	return false
}

// ─── Settings helpers ──────────────────────────────────────

// GetSetting retrieves a setting by key.
func GetSetting(ctx context.Context, db *sql.DB, key string) (string, error) {
	var val string
	err := db.QueryRowContext(ctx, `SELECT value FROM app_settings WHERE key = ?`, key).Scan(&val)
	if err != nil {
		return "", err
	}
	return val, nil
}

// SetSetting upserts a setting.
func SetSetting(ctx context.Context, db *sql.DB, key, value string) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO app_settings (key, value, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP`,
		key, value,
	)
	return err
}

// ─── Scanner helpers ──────────────────────────────────────

func scanBackupSchedule(row *sql.Row) (*BackupSchedule, error) {
	var bs BackupSchedule
	err := row.Scan(
		&bs.ID, &bs.SiteID, &bs.ProviderID, &bs.CronExpr, &bs.BackupType,
		&bs.Enabled, &bs.LastRunAt, &bs.NextRunAt, &bs.CreatedAt, &bs.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan backup schedule: %w", err)
	}
	return &bs, nil
}

func scanBackupScheduleRows(rows *sql.Rows, bs *BackupSchedule) error {
	return rows.Scan(
		&bs.ID, &bs.SiteID, &bs.ProviderID, &bs.CronExpr, &bs.BackupType,
		&bs.Enabled, &bs.LastRunAt, &bs.NextRunAt, &bs.CreatedAt, &bs.UpdatedAt,
	)
}
