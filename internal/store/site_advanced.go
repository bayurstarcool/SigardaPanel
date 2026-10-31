package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// ─── Vhost ─────────────────────────────────────────────────

type SiteVhost struct {
	ID        int64     `json:"id"`
	SiteID    int64     `json:"site_id"`
	Config    string    `json:"vhost_config"`
	UpdatedAt time.Time `json:"updated_at"`
}

type VhostRepository interface {
	GetBySiteID(ctx context.Context, siteID int64) (*SiteVhost, error)
	Upsert(ctx context.Context, siteID int64, config string) error
}

type vhostStore struct{ db *sql.DB }

func NewVhostStore(db *sql.DB) VhostRepository { return &vhostStore{db} }

func (s *vhostStore) GetBySiteID(ctx context.Context, siteID int64) (*SiteVhost, error) {
	v := &SiteVhost{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, site_id, config, updated_at FROM site_vhost WHERE site_id = ?`, siteID,
	).Scan(&v.ID, &v.SiteID, &v.Config, &v.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return v, nil
}

func (s *vhostStore) Upsert(ctx context.Context, siteID int64, config string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO site_vhost (site_id, config, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(site_id) DO UPDATE SET config = ?, updated_at = CURRENT_TIMESTAMP`,
		siteID, config, config,
	)
	return err
}

// ─── Varnish ────────────────────────────────────────────────

type SiteVarnish struct {
	ID          int64     `json:"id"`
	SiteID      int64     `json:"site_id"`
	Enabled     bool      `json:"enabled"`
	CacheTime   int       `json:"cache_time"`
	HitRate     string    `json:"hit_rate"`
	BackendHost string    `json:"backend_host"`
	BackendPort int       `json:"backend_port"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type VarnishRepository interface {
	GetBySiteID(ctx context.Context, siteID int64) (*SiteVarnish, error)
	Upsert(ctx context.Context, v *SiteVarnish) error
}

type varnishStore struct{ db *sql.DB }

func NewVarnishStore(db *sql.DB) VarnishRepository { return &varnishStore{db} }

func (s *varnishStore) GetBySiteID(ctx context.Context, siteID int64) (*SiteVarnish, error) {
	v := &SiteVarnish{}
	var enabled int
	err := s.db.QueryRowContext(ctx,
		`SELECT id, site_id, enabled, cache_time, hit_rate, backend_host, backend_port, updated_at FROM site_varnish WHERE site_id = ?`, siteID,
	).Scan(&v.ID, &v.SiteID, &enabled, &v.CacheTime, &v.HitRate, &v.BackendHost, &v.BackendPort, &v.UpdatedAt)
	if err != nil {
		return nil, err
	}
	v.Enabled = enabled == 1
	return v, nil
}

func (s *varnishStore) Upsert(ctx context.Context, v *SiteVarnish) error {
	enabled := 0
	if v.Enabled {
		enabled = 1
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO site_varnish (site_id, enabled, cache_time, hit_rate, backend_host, backend_port, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(site_id) DO UPDATE SET enabled = ?, cache_time = ?, hit_rate = ?, backend_host = ?, backend_port = ?, updated_at = CURRENT_TIMESTAMP`,
		v.SiteID, enabled, v.CacheTime, v.HitRate, v.BackendHost, v.BackendPort,
		enabled, v.CacheTime, v.HitRate, v.BackendHost, v.BackendPort,
	)
	return err
}

// ─── Security ───────────────────────────────────────────────

type SiteSecurity struct {
	ID                int64     `json:"id"`
	SiteID            int64     `json:"site_id"`
	BasicAuthEnabled  bool      `json:"basic_auth_enabled"`
	BasicAuthUsername string    `json:"basic_auth_username"`
	BasicAuthPassword string    `json:"basic_auth_password"`
	BlockedIPs        []string  `json:"blocked_ips"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type SecurityRepository interface {
	GetBySiteID(ctx context.Context, siteID int64) (*SiteSecurity, error)
	Upsert(ctx context.Context, s *SiteSecurity) error
}

type securityStore struct{ db *sql.DB }

func NewSecurityStore(db *sql.DB) SecurityRepository { return &securityStore{db} }

func (s *securityStore) GetBySiteID(ctx context.Context, siteID int64) (*SiteSecurity, error) {
	sec := &SiteSecurity{}
	var enabled int
	var blockedRaw string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, site_id, basic_auth_enabled, basic_auth_username, basic_auth_password, blocked_ips, updated_at FROM site_security WHERE site_id = ?`, siteID,
	).Scan(&sec.ID, &sec.SiteID, &enabled, &sec.BasicAuthUsername, &sec.BasicAuthPassword, &blockedRaw, &sec.UpdatedAt)
	if err != nil {
		return nil, err
	}
	sec.BasicAuthEnabled = enabled == 1
	json.Unmarshal([]byte(blockedRaw), &sec.BlockedIPs)
	if sec.BlockedIPs == nil {
		sec.BlockedIPs = []string{}
	}
	return sec, nil
}

func (s *securityStore) Upsert(ctx context.Context, sec *SiteSecurity) error {
	enabled := 0
	if sec.BasicAuthEnabled {
		enabled = 1
	}
	// Hash password with bcrypt if provided
	hashedPassword := sec.BasicAuthPassword
	if sec.BasicAuthPassword != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(sec.BasicAuthPassword), bcrypt.DefaultCost)
		if err == nil {
			hashedPassword = string(hash)
		}
	}
	blockedRaw, _ := json.Marshal(sec.BlockedIPs)
	if sec.BlockedIPs == nil {
		blockedRaw = []byte("[]")
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO site_security (site_id, basic_auth_enabled, basic_auth_username, basic_auth_password, blocked_ips, updated_at)
	 VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	 ON CONFLICT(site_id) DO UPDATE SET basic_auth_enabled = ?, basic_auth_username = ?, basic_auth_password = ?, blocked_ips = ?, updated_at = CURRENT_TIMESTAMP`,
		sec.SiteID, enabled, sec.BasicAuthUsername, hashedPassword, string(blockedRaw),
		enabled, sec.BasicAuthUsername, hashedPassword, string(blockedRaw),
	)
	return err
}

// ─── SSH/FTP ────────────────────────────────────────────────

type SiteSSHUser struct {
	ID            int64     `json:"id"`
	SiteID        int64     `json:"site_id"`
	Username      string    `json:"username"`
	Password      string    `json:"password,omitempty"`
	SSHKeys       string    `json:"ssh_keys,omitempty"`
	SSHPrivateKey string    `json:"ssh_private_key,omitempty"`
	SSHPublicKey  string    `json:"ssh_public_key,omitempty"`
	HomeDir       string    `json:"home_dir"`
	CreatedAt     time.Time `json:"created_at"`
}

type SSHRepository interface {
	GetByID(ctx context.Context, id int64) (*SiteSSHUser, error)
	ListBySiteID(ctx context.Context, siteID int64) ([]SiteSSHUser, error)
	ListAll(ctx context.Context) ([]SiteSSHUser, error)
	Create(ctx context.Context, u *SiteSSHUser) error
	// Delete removes the SSH/FTP user id of siteID; sql.ErrNoRows if the
	// user does not belong to that site.
	Delete(ctx context.Context, siteID, id int64) error
}

type sshStore struct{ db *sql.DB }

func NewSSHStore(db *sql.DB) SSHRepository { return &sshStore{db} }

func (s *sshStore) ListBySiteID(ctx context.Context, siteID int64) ([]SiteSSHUser, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, site_id, username, ssh_keys, home_dir, ssh_private_key, ssh_public_key, created_at FROM site_ssh_ftp WHERE site_id = ? ORDER BY created_at DESC`, siteID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []SiteSSHUser
	for rows.Next() {
		var u SiteSSHUser
		if err := rows.Scan(&u.ID, &u.SiteID, &u.Username, &u.SSHKeys, &u.HomeDir, &u.SSHPrivateKey, &u.SSHPublicKey, &u.CreatedAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (s *sshStore) GetByID(ctx context.Context, id int64) (*SiteSSHUser, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, site_id, username, ssh_keys, home_dir, ssh_private_key, ssh_public_key, created_at FROM site_ssh_ftp WHERE id = ?`, id)
	var u SiteSSHUser
	if err := row.Scan(&u.ID, &u.SiteID, &u.Username, &u.SSHKeys, &u.HomeDir, &u.SSHPrivateKey, &u.SSHPublicKey, &u.CreatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &u, nil
}

func (s *sshStore) ListAll(ctx context.Context) ([]SiteSSHUser, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, site_id, username, ssh_keys, home_dir, created_at FROM site_ssh_ftp ORDER BY site_id, username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []SiteSSHUser
	for rows.Next() {
		var u SiteSSHUser
		if err := rows.Scan(&u.ID, &u.SiteID, &u.Username, &u.SSHKeys, &u.HomeDir, &u.CreatedAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (s *sshStore) Create(ctx context.Context, u *SiteSSHUser) error {
	homeDir := u.HomeDir
	if homeDir == "" {
		homeDir = "/home/" + u.Username
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO site_ssh_ftp (site_id, username, password, ssh_keys, home_dir, ssh_private_key, ssh_public_key) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		u.SiteID, u.Username, u.Password, u.SSHKeys, homeDir, u.SSHPrivateKey, u.SSHPublicKey,
	)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	u.ID = id
	u.HomeDir = homeDir
	return nil
}

func (s *sshStore) Delete(ctx context.Context, siteID, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM site_ssh_ftp WHERE id = ? AND site_id = ?`, id, siteID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ─── Cron Jobs ──────────────────────────────────────────────

type SiteCronJob struct {
	ID        int64     `json:"id"`
	SiteID    int64     `json:"site_id"`
	Schedule  string    `json:"schedule"`
	Command   string    `json:"command"`
	CreatedAt time.Time `json:"created_at"`
}

type CronRepository interface {
	ListBySiteID(ctx context.Context, siteID int64) ([]SiteCronJob, error)
	Create(ctx context.Context, j *SiteCronJob) error
	// Delete removes cron job id of siteID; sql.ErrNoRows if the job does
	// not belong to that site.
	Delete(ctx context.Context, siteID, id int64) error
}

type cronStore struct{ db *sql.DB }

func NewCronStore(db *sql.DB) CronRepository { return &cronStore{db} }

func (s *cronStore) ListBySiteID(ctx context.Context, siteID int64) ([]SiteCronJob, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, site_id, schedule, command, created_at FROM site_cron_jobs WHERE site_id = ? ORDER BY created_at DESC`, siteID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var jobs []SiteCronJob
	for rows.Next() {
		var j SiteCronJob
		if err := rows.Scan(&j.ID, &j.SiteID, &j.Schedule, &j.Command, &j.CreatedAt); err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

func (s *cronStore) Create(ctx context.Context, j *SiteCronJob) error {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO site_cron_jobs (site_id, schedule, command) VALUES (?, ?, ?)`,
		j.SiteID, j.Schedule, j.Command,
	)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	j.ID = id
	return nil
}

func (s *cronStore) Delete(ctx context.Context, siteID, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM site_cron_jobs WHERE id = ? AND site_id = ?`, id, siteID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
