package store

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"sigardapanel/internal/auth"
)

// Site represents a website / application deployed on a server.
type Site struct {
	ID            int64
	ServerID      int64
	Name          string
	Domain        string
	Runtime       string
	RootPath      string
	Status        string
	OwnerID       sql.NullInt64
	SSLStatus     sql.NullString
	SSLIssuer     sql.NullString
	SSLExpiresAt  sql.NullTime
	SSLLastCheck  sql.NullTime
	GitRepo       sql.NullString
	GitBranch     sql.NullString
	GitCommit     sql.NullString
	DeployKey     sql.NullString
	WebhookSecret sql.NullString
	ManagedBy     sql.NullString
	SystemUser    sql.NullString
	ConfFile      sql.NullString
	ProxyPassURL  sql.NullString
	AppPort      sql.NullInt64
	AppCommand   sql.NullString
	AppStatus    sql.NullString
	AppMemoryMax sql.NullInt64
	PHPVersion      sql.NullString
	PHPIni          sql.NullString
	BackupProviderID sql.NullInt64
	CreatedAt       time.Time
	UpdatedAt     time.Time
}

// SiteListFilter provides filtering and pagination for List.
type SiteListFilter struct {
	ServerID int64
	OwnerID  int64
	Runtime  string
	Status   string
	Search   string
	Limit    int
	Offset   int
}

// SiteRepository defines site persistence operations.
type SiteRepository interface {
	Create(ctx context.Context, s *Site) error
	GetByID(ctx context.Context, id int64) (*Site, error)
	GetByDomain(ctx context.Context, domain string) (*Site, error)
	List(ctx context.Context, filter SiteListFilter) ([]Site, error)
	Update(ctx context.Context, s *Site) error
	UpdateStatus(ctx context.Context, id int64, status string) error
	UpdateConfFile(ctx context.Context, id int64, confFile string) error
	UpdateSSL(ctx context.Context, id int64, status, issuer string, expiresAt time.Time) error
	UpdateGitConfig(ctx context.Context, id int64, repo, branch string) error
	UpdateProxyPassURL(ctx context.Context, id int64, url string) error
	UpdateGitCommit(ctx context.Context, id int64, commit string) error
	UpdateAppConfig(ctx context.Context, id int64, port int64, command, status string) error
	UpdateAppStatus(ctx context.Context, id int64, status string) error
	UpdateAppMemory(ctx context.Context, id int64, memoryMax int64) error
	UpdatePHPConfig(ctx context.Context, id int64, version, ini string) error
	CheckPortAvailable(ctx context.Context, excludeSiteID int64, port int64) (bool, string, error)
	GenerateWebhookSecret(ctx context.Context, id int64) (string, error)
	Delete(ctx context.Context, id int64) error
	Count(ctx context.Context, filter SiteListFilter) (int, error)
	CountByOwner(ctx context.Context, ownerID int64) (int, error)
	Transfer(ctx context.Context, siteID, newOwnerID int64) error
	NullifyOwner(ctx context.Context, userID int64) error
	FindFreePort(ctx context.Context, minPort, maxPort int64) (int64, error)
	IsPortAvailable(ctx context.Context, port int64) bool
	ClearAppPort(ctx context.Context, id int64) error
	UpdateBackupProvider(ctx context.Context, id int64, providerID sql.NullInt64) error
	ListSSLEpiring(ctx context.Context, within time.Duration) ([]Site, error)
}

// SQLiteSiteStore implements SiteRepository against SQLite.
type SQLiteSiteStore struct {
	db *sql.DB
}

// NewSiteStore returns a new SQLite-backed site store.
func NewSiteStore(db *sql.DB) *SQLiteSiteStore {
	return &SQLiteSiteStore{db: db}
}

// Create inserts a new site. Sets Status to "pending" and RootPath default.
func (s *SQLiteSiteStore) Create(ctx context.Context, site *Site) error {
	if site.Status == "" {
		site.Status = "pending"
	}
	if site.RootPath == "" {
		user := site.Domain
		if site.SystemUser.Valid && site.SystemUser.String != "" {
			user = site.SystemUser.String
		}
		site.RootPath = "/home/" + user + "/htdocs/" + site.Domain
	}
	if site.SSLStatus.String == "" {
		site.SSLStatus = sql.NullString{String: "none", Valid: true}
	}
	if site.ManagedBy.String == "" {
		site.ManagedBy = sql.NullString{String: "sigardapanel", Valid: true}
	}

	res, err := s.db.ExecContext(ctx,
		`INSERT INTO sites (server_id, name, domain, runtime, root_path, status, owner_id, ssl_status, managed_by, system_user, conf_file, proxy_pass_url, app_port, app_command, app_status, app_memory_max, php_version, php_ini) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		site.ServerID, site.Name, site.Domain, site.Runtime, site.RootPath, site.Status, site.OwnerID, site.SSLStatus,
		site.ManagedBy, site.SystemUser, site.ConfFile, site.ProxyPassURL, site.AppPort, site.AppCommand, site.AppStatus, site.AppMemoryMax, site.PHPVersion, site.PHPIni,
	)
	if err != nil {
		return fmt.Errorf("insert site: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("last insert id: %w", err)
	}
	site.ID = id
	return nil
}

// GetByID retrieves a site by ID. Returns (nil, nil) if not found.
func (s *SQLiteSiteStore) GetByID(ctx context.Context, id int64) (*Site, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, server_id, name, domain, runtime, root_path, status, owner_id, ssl_status, ssl_issuer, ssl_expires_at, ssl_last_check, git_repo, git_branch, git_commit, deploy_key, webhook_secret, managed_by, system_user, conf_file, proxy_pass_url, app_port, app_command, app_status, app_memory_max, php_version, php_ini, backup_provider_id, created_at, updated_at FROM sites WHERE id = ?`, id)
	return scanSite(row)
}

// GetByDomain retrieves a site by domain. Returns (nil, nil) if not found.
func (s *SQLiteSiteStore) GetByDomain(ctx context.Context, domain string) (*Site, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, server_id, name, domain, runtime, root_path, status, owner_id, ssl_status, ssl_issuer, ssl_expires_at, ssl_last_check, git_repo, git_branch, git_commit, deploy_key, webhook_secret, managed_by, system_user, conf_file, proxy_pass_url, app_port, app_command, app_status, app_memory_max, php_version, php_ini, backup_provider_id, created_at, updated_at FROM sites WHERE domain = ?`, domain)
	return scanSite(row)
}

// List returns sites ordered by id DESC with optional filters.
func (s *SQLiteSiteStore) List(ctx context.Context, filter SiteListFilter) ([]Site, error) {
	if filter.Limit <= 0 {
		filter.Limit = 100
	}

	query := `SELECT id, server_id, name, domain, runtime, root_path, status, owner_id, ssl_status, ssl_issuer, ssl_expires_at, ssl_last_check, git_repo, git_branch, git_commit, deploy_key, webhook_secret, managed_by, system_user, conf_file, proxy_pass_url, app_port, app_command, app_status, app_memory_max, php_version, php_ini, backup_provider_id, created_at, updated_at FROM sites WHERE 1=1`
	var args []any

	if filter.ServerID > 0 {
		query += ` AND server_id = ?`
		args = append(args, filter.ServerID)
	}
	if filter.OwnerID > 0 {
		query += ` AND owner_id = ?`
		args = append(args, filter.OwnerID)
	}
	if filter.Runtime != "" {
		query += ` AND runtime = ?`
		args = append(args, filter.Runtime)
	}
	if filter.Status != "" {
		query += ` AND status = ?`
		args = append(args, filter.Status)
	}
	if filter.Search != "" {
		query += ` AND (domain LIKE ? OR name LIKE ?)`
		searchPattern := "%" + filter.Search + "%"
		args = append(args, searchPattern, searchPattern)
	}
	query += ` ORDER BY id DESC LIMIT ? OFFSET ?`
	args = append(args, filter.Limit, filter.Offset)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query sites: %w", err)
	}
	defer rows.Close()

	var out []Site
	for rows.Next() {
		var site Site
		if err := rows.Scan(
			&site.ID, &site.ServerID, &site.Name, &site.Domain, &site.Runtime, &site.RootPath, &site.Status, &site.OwnerID,
			&site.SSLStatus, &site.SSLIssuer, &site.SSLExpiresAt, &site.SSLLastCheck,
			&site.GitRepo, &site.GitBranch, &site.GitCommit, &site.DeployKey, &site.WebhookSecret,
			&site.ManagedBy, &site.SystemUser, &site.ConfFile, &site.ProxyPassURL, &site.AppPort, &site.AppCommand, &site.AppStatus, &site.AppMemoryMax, &site.PHPVersion, &site.PHPIni, &site.BackupProviderID,
 &site.CreatedAt, &site.UpdatedAt,
 ); err != nil {
			return nil, fmt.Errorf("scan site: %w", err)
		}
		out = append(out, site)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}
	return out, nil
}

// Count returns the total number of sites matching the given filter (ignoring pagination).
func (s *SQLiteSiteStore) Count(ctx context.Context, filter SiteListFilter) (int, error) {
	query := `SELECT COUNT(*) FROM sites WHERE 1=1`
	var args []any

	if filter.ServerID > 0 {
		query += ` AND server_id = ?`
		args = append(args, filter.ServerID)
	}
	if filter.OwnerID > 0 {
		query += ` AND owner_id = ?`
		args = append(args, filter.OwnerID)
	}
	if filter.Runtime != "" {
		query += ` AND runtime = ?`
		args = append(args, filter.Runtime)
	}
	if filter.Status != "" {
		query += ` AND status = ?`
		args = append(args, filter.Status)
	}
	if filter.Search != "" {
		query += ` AND (domain LIKE ? OR name LIKE ?)`
		searchPattern := "%" + filter.Search + "%"
		args = append(args, searchPattern, searchPattern)
	}

	var total int
	if err := s.db.QueryRowContext(ctx, query, args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("count sites: %w", err)
	}
	return total, nil
}

// Update modifies name, domain, runtime, root_path, and status.
func (s *SQLiteSiteStore) Update(ctx context.Context, site *Site) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE sites SET name = ?, domain = ?, runtime = ?, root_path = ?, status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		site.Name, site.Domain, site.Runtime, site.RootPath, site.Status, site.ID,
	)
	if err != nil {
		return fmt.Errorf("update site: %w", err)
	}
	return nil
}

// UpdateStatus updates only the status field of a site.
func (s *SQLiteSiteStore) UpdateStatus(ctx context.Context, id int64, status string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE sites SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		status, id,
	)
	if err != nil {
		return fmt.Errorf("update site status: %w", err)
	}
	return nil
}

// UpdateConfFile updates the nginx config file path for a site.
func (s *SQLiteSiteStore) UpdateConfFile(ctx context.Context, id int64, confFile string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE sites SET conf_file = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		confFile, id,
	)
	if err != nil {
		return fmt.Errorf("update conf_file: %w", err)
	}
	return nil
}

// UpdateSSL updates the SSL-related fields of a site.
func (s *SQLiteSiteStore) UpdateSSL(ctx context.Context, id int64, status, issuer string, expiresAt time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE sites SET ssl_status = ?, ssl_issuer = ?, ssl_expires_at = ?, ssl_last_check = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		status, issuer, expiresAt, id,
	)
	if err != nil {
		return fmt.Errorf("update site ssl: %w", err)
	}
	return nil
}

// ListSSLEpiring returns sites whose SSL certificates expire within the given duration.
// Only sites with ssl_status = 'active' and a valid ssl_expires_at are considered.
func (s *SQLiteSiteStore) ListSSLEpiring(ctx context.Context, within time.Duration) ([]Site, error) {
	cutoff := time.Now().Add(within)
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, server_id, name, domain, runtime, root_path, status, owner_id,
		        ssl_status, ssl_issuer, ssl_expires_at, ssl_last_check,
		        git_repo, git_branch, git_commit, deploy_key, webhook_secret,
		        managed_by, system_user, conf_file, proxy_pass_url,
		        app_port, app_command, app_status, app_memory_max,
		        php_version, php_ini,
		        backup_provider_id, created_at, updated_at
		 FROM sites
		 WHERE ssl_status = 'active'
		   AND ssl_expires_at IS NOT NULL
		   AND ssl_expires_at <= ?
		 ORDER BY ssl_expires_at ASC`,
		cutoff,
	)
	if err != nil {
		return nil, fmt.Errorf("list ssl expiring: %w", err)
	}
	defer rows.Close()

	var sites []Site
	for rows.Next() {
		var site Site
		if err := rows.Scan(
			&site.ID, &site.ServerID, &site.Name, &site.Domain, &site.Runtime, &site.RootPath, &site.Status, &site.OwnerID,
			&site.SSLStatus, &site.SSLIssuer, &site.SSLExpiresAt, &site.SSLLastCheck,
			&site.GitRepo, &site.GitBranch, &site.GitCommit, &site.DeployKey, &site.WebhookSecret,
			&site.ManagedBy, &site.SystemUser, &site.ConfFile, &site.ProxyPassURL,
			&site.AppPort, &site.AppCommand, &site.AppStatus, &site.AppMemoryMax,
			&site.PHPVersion, &site.PHPIni,
			&site.BackupProviderID, &site.CreatedAt, &site.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan site: %w", err)
		}
		sites = append(sites, site)
	}
	return sites, rows.Err()
}

// UpdateGitConfig updates git_repo and git_branch for a site.
func (s *SQLiteSiteStore) UpdateGitConfig(ctx context.Context, id int64, repo, branch string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE sites SET git_repo = ?, git_branch = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		repo, branch, id,
	)
	if err != nil {
		return fmt.Errorf("update git config: %w", err)
	}
	return nil
}

// UpdateProxyPassURL updates the proxy_pass_url for a site.
func (s *SQLiteSiteStore) UpdateProxyPassURL(ctx context.Context, id int64, url string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE sites SET proxy_pass_url = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		url, id,
	)
	if err != nil {
		return fmt.Errorf("update proxy pass url: %w", err)
	}
	return nil
}

// UpdateGitCommit updates the last deployed commit hash for a site.
// IsPortAvailable returns true if a specific port is free (not in DB and not in use at OS level).
func (s *SQLiteSiteStore) IsPortAvailable(ctx context.Context, port int64) bool {
	// Check DB
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sites WHERE app_port = ?`, port).Scan(&count)
	if err != nil || count > 0 {
		return false
	}
	// Check OS
	ln, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	ln.Close()
	return true
}

// FindFreePort returns a port in [minPort, maxPort] not currently assigned to any site in DB
// and not in actual OS usage (checked via net.Listen). The OS-level check prevents collisions
// with pre-existing services like PHP-FPM pools, Docker, etc.
func (s *SQLiteSiteStore) FindFreePort(ctx context.Context, minPort, maxPort int64) (int64, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT app_port FROM sites WHERE app_port IS NOT NULL`)
	if err != nil {
		return 0, fmt.Errorf("query used ports: %w", err)
	}
	defer rows.Close()
	used := make(map[int64]bool)
	for rows.Next() {
		var p sql.NullInt64
		if err := rows.Scan(&p); err == nil && p.Valid {
			used[p.Int64] = true
		}
	}
	for port := minPort; port <= maxPort; port++ {
		if used[port] {
			continue
		}
		// Quick OS check: try to listen on the port (SYN only, no accept needed)
		addr := fmt.Sprintf("127.0.0.1:%d", port)
		ln, err := net.Listen("tcp4", addr)
		if err != nil {
			// Port in use at OS level
			used[port] = true
			continue
		}
		ln.Close()
		return port, nil
	}
	return 0, fmt.Errorf("no free port in range %d-%d. Check running apps with: ss -tlnp", minPort, maxPort)
}

// ClearAppPort resets app port/command/status when a site is deleted.
func (s *SQLiteSiteStore) ClearAppPort(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE sites SET app_port = NULL, app_command = NULL, app_status = 'stopped', updated_at = CURRENT_TIMESTAMP WHERE id = ?`, id)
	return err
}

// UpdateAppConfig sets the app port, start command, and status for a site.
func (s *SQLiteSiteStore) UpdateAppConfig(ctx context.Context, id int64, port int64, command, status string) error {
	var portVal any
	if port > 0 {
		portVal = port
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE sites SET app_port = ?, app_command = ?, app_status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		portVal, command, status, id,
	)
	if err != nil {
		return fmt.Errorf("update app config: %w", err)
	}
	return nil
}

// UpdateAppMemory sets the per-site memory limit (MB).
func (s *SQLiteSiteStore) UpdateAppMemory(ctx context.Context, id int64, memoryMax int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE sites SET app_memory_max = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		memoryMax, id,
	)
	if err != nil {
		return fmt.Errorf("update app memory: %w", err)
	}
	return nil
}

// UpdatePHPConfig sets the per-site PHP version and custom php.ini directives.
func (s *SQLiteSiteStore) UpdatePHPConfig(ctx context.Context, id int64, version, ini string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE sites SET php_version = ?, php_ini = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		version, ini, id,
	)
	if err != nil {
		return fmt.Errorf("update php config: %w", err)
	}
	return nil
}

// CheckPortAvailable checks if a port is free — both in DB (other sites) and at OS level (PHP-FPM, Docker, etc).
// Returns (available, reason, error). reason explains what's occupying the port.
func (s *SQLiteSiteStore) CheckPortAvailable(ctx context.Context, excludeSiteID int64, port int64) (bool, string, error) {
	// 1. Check DB: any other site using this port?
	var owner sql.NullInt64
	err := s.db.QueryRowContext(ctx,
		`SELECT id FROM sites WHERE app_port = ? AND id != ?`, port, excludeSiteID,
	).Scan(&owner)
	if err == nil && owner.Valid {
		// Port is assigned to another site — get its domain
		var domain string
		_ = s.db.QueryRowContext(ctx, `SELECT domain FROM sites WHERE id = ?`, owner.Int64).Scan(&domain)
		if domain != "" {
			return false, fmt.Sprintf("port %d is already used by site %s", port, domain), nil
		}
		return false, fmt.Sprintf("port %d is already used by another site (id=%d)", port, owner.Int64), nil
	}
	if err != nil && err != sql.ErrNoRows {
		return false, "", fmt.Errorf("check port db: %w", err)
	}

	// 2. Check OS level: try net.Listen
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	ln, err := net.Listen("tcp4", addr)
	if err != nil {
		// Find what's using it
		hint := fmt.Sprintf("port %d is in use on this server (possibly PHP-FPM, Docker, or another service)", port)
		return false, hint, nil
	}
	ln.Close()

	return true, "", nil
}

// UpdateAppStatus sets only the app_status for a site.
func (s *SQLiteSiteStore) UpdateAppStatus(ctx context.Context, id int64, status string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE sites SET app_status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		status, id,
	)
	if err != nil {
		return fmt.Errorf("update app status: %w", err)
	}
	return nil
}

func (s *SQLiteSiteStore) UpdateGitCommit(ctx context.Context, id int64, commit string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE sites SET git_commit = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		commit, id,
	)
	if err != nil {
		return fmt.Errorf("update git commit: %w", err)
	}
	return nil
}

// GenerateWebhookSecret generates a new random webhook secret for a site.
func (s *SQLiteSiteStore) GenerateWebhookSecret(ctx context.Context, id int64) (string, error) {
	secret, err := auth.NewToken()
	if err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE sites SET webhook_secret = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		secret, id,
	)
	if err != nil {
		return "", fmt.Errorf("update webhook secret: %w", err)
	}
	return secret, nil
}

// Delete removes a site by ID. Handles cleanup: user_sites, jobs, databases, SSH/FTP users, cron jobs.
func (s *SQLiteSiteStore) Delete(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("delete site begin tx: %w", err)
	}
	defer tx.Rollback()

	// Delete related records
	if _, err := tx.ExecContext(ctx, `DELETE FROM user_sites WHERE site_id = ?`, id); err != nil {
		return fmt.Errorf("delete site user_sites: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM jobs WHERE site_id = ?`, id); err != nil {
		return fmt.Errorf("delete site jobs: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM databases WHERE site_id = ?`, id); err != nil {
		return fmt.Errorf("delete site databases: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM site_cron_jobs WHERE site_id = ?`, id); err != nil {
		return fmt.Errorf("delete site cron_jobs: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM site_ssh_ftp WHERE site_id = ?`, id); err != nil {
		return fmt.Errorf("delete site ssh_ftp_users: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM backups WHERE site_id = ?`, id); err != nil {
		return fmt.Errorf("delete site backups: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM audit_logs WHERE resource = 'site' AND resource_id = ?`, id); err != nil {
		return fmt.Errorf("delete site audit_logs: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM sites WHERE id = ?`, id); err != nil {
		return fmt.Errorf("delete site: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("delete site commit: %w", err)
	}
	return nil
}

// CountByOwner returns the number of sites owned by the given user.
func (s *SQLiteSiteStore) CountByOwner(ctx context.Context, ownerID int64) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sites WHERE owner_id = ?`, ownerID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count sites: %w", err)
	}
	return count, nil
}

// NullifyOwner sets owner_id to NULL for all sites owned by the given user.
func (s *SQLiteSiteStore) NullifyOwner(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE sites SET owner_id = NULL, updated_at = CURRENT_TIMESTAMP WHERE owner_id = ?`, userID)
	return err
}

// UpdateBackupProvider sets the backup_provider_id for a site.
func (s *SQLiteSiteStore) UpdateBackupProvider(ctx context.Context, id int64, providerID sql.NullInt64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE sites SET backup_provider_id = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		providerID, id,
	)
	return err
}

// Transfer changes the owner of a site.
func (s *SQLiteSiteStore) Transfer(ctx context.Context, siteID, newOwnerID int64) error {
	// Get site info
	site, err := s.GetByID(ctx, siteID)
	if err != nil {
		return fmt.Errorf("get site: %w", err)
	}
	if site == nil {
		return fmt.Errorf("site not found")
	}

	// Get target user info
	var newUsername string
	err = s.db.QueryRowContext(ctx, `SELECT username FROM users WHERE id = ?`, newOwnerID).Scan(&newUsername)
	if err != nil {
		return fmt.Errorf("target user not found: %w", err)
	}

	// New root path under target user's home
	newRoot := "/home/" + newUsername + "/htdocs/" + site.Domain

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("transfer site begin tx: %w", err)
	}
	defer tx.Rollback()

	// Update owner, root_path, and system_user in sites table
	res, err := tx.ExecContext(ctx,
		`UPDATE sites SET owner_id = ?, root_path = ?, system_user = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		newOwnerID, newRoot, newUsername, siteID,
	)
	if err != nil {
		return fmt.Errorf("transfer site: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("transfer site rows: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("site not found")
	}

	// Update user_sites: remove old owner assignment, add new owner with admin
	if _, err := tx.ExecContext(ctx, `DELETE FROM user_sites WHERE user_id = ? AND site_id = ?`, newOwnerID, siteID); err != nil {
		return fmt.Errorf("transfer site user_sites delete: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO user_sites (user_id, site_id, permission) VALUES (?, ?, 'admin') ON CONFLICT(user_id, site_id) DO UPDATE SET permission = 'admin'`, newOwnerID, siteID); err != nil {
		return fmt.Errorf("transfer site user_sites insert: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("transfer site commit: %w", err)
	}

	// Move files to new owner's home directory
	// This happens outside the transaction since it's filesystem operation
	if site.RootPath != "" && site.RootPath != newRoot {
		go moveSiteFiles(site.RootPath, site.Domain, newUsername)
	}

	return nil
}

// moveSiteFiles moves site files from old path to new owner's home.
func moveSiteFiles(oldRoot, domain, newUsername string) {
	newRoot := "/home/" + newUsername + "/htdocs/" + domain

	// Create target directory
	if err := os.MkdirAll(filepath.Dir(newRoot), 0755); err != nil {
		slog.Error("transfer: create target dir", "error", err)
		return
	}

	// Move files
	if err := os.Rename(oldRoot, newRoot); err != nil {
		slog.Error("transfer: move files failed, copying instead", "error", err)
		// Fallback: copy
		if err := copyDir(oldRoot, newRoot); err != nil {
			slog.Error("transfer: copy files failed", "error", err)
			return
		}
		os.RemoveAll(oldRoot)
	}

	// Chown to new owner
	if err := chownR(newRoot, newUsername); err != nil {
		slog.Error("transfer: chown failed", "error", err)
	}
}

func chownR(path, username string) error {
	// Look up user's UID/GID
	cmd := exec.Command("chown", "-R", username+":"+username, path)
	return cmd.Run()
}

func copyDir(src, dst string) error {
	cmd := exec.Command("cp", "-a", src+"/", dst+"/")
	return cmd.Run()
}

func scanSite(row *sql.Row) (*Site, error) {
	var site Site
	err := row.Scan(
		&site.ID, &site.ServerID, &site.Name, &site.Domain, &site.Runtime, &site.RootPath, &site.Status, &site.OwnerID,
		&site.SSLStatus, &site.SSLIssuer, &site.SSLExpiresAt, &site.SSLLastCheck,
		&site.GitRepo, &site.GitBranch, &site.GitCommit, &site.DeployKey, &site.WebhookSecret,
		&site.ManagedBy, &site.SystemUser, &site.ConfFile, &site.ProxyPassURL, &site.AppPort, &site.AppCommand, &site.AppStatus, &site.AppMemoryMax, &site.PHPVersion, &site.PHPIni, &site.BackupProviderID,
 &site.CreatedAt, &site.UpdatedAt,
 )
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan site: %w", err)
	}
	return &site, nil
}
