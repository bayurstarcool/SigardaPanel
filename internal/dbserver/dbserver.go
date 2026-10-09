// Package dbserver provides storage and direct MySQL/MariaDB connection management.
package dbserver

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"sigardapanel/internal/siteinput"

	_ "github.com/go-sql-driver/mysql"
)

// ─── Entity ───────────────────────────────────────────────

// Server represents a database server (MySQL/MariaDB) configuration.
type Server struct {
	ID                      int64     `json:"id"`
	Name                    string    `json:"name"`
	Engine                  string    `json:"engine"`
	Host                    string    `json:"host"`
	Port                    int       `json:"port"`
	Username                string    `json:"username"`
	PasswordEncrypted       string    `json:"-"`
	BackupUsername          string    `json:"backup_username,omitempty"`
	BackupPasswordEncrypted string    `json:"-"`
	IsActive                bool      `json:"is_active"`
	IsDefault               bool      `json:"is_default"`
	CreatedAt               time.Time `json:"created_at"`
	UpdatedAt               time.Time `json:"updated_at"`
}

// ─── Store ─────────────────────────────────────────────────

// Store handles persistence of database server configurations.
type Store struct {
	db     *sql.DB
	encKey []byte // AES key for password encryption
}

// NewStore creates a new dbserver store with the given encryption key.
// The key should be 32 bytes (AES-256) — pass a hash of APP_SECRET or equivalent.
func NewStore(db *sql.DB, encKey []byte) *Store {
	key := encKey
	if len(key) != 32 {
		// Derive 32-byte key from whatever we have
		h := sha256Hash(key)
		key = []byte(h[:32])
	}
	return &Store{db: db, encKey: key}
}

// GetDefault returns the active default database server (if any).
func (s *Store) GetDefault(ctx context.Context) (*Server, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, engine, host, port, username, password_encrypted, backup_username, backup_password_encrypted, is_active, is_default, created_at, updated_at
		 FROM database_servers WHERE is_default = 1 AND is_active = 1 LIMIT 1`)
	return scanServer(row)
}

// GetBackupCredentials returns the dedicated least-privilege backup credential.
// Root credentials remain reserved for database administration.
func (s *Store) GetBackupCredentials(ctx context.Context) (string, string, error) {
	var username, encrypted string
	err := s.db.QueryRowContext(ctx,
		`SELECT backup_username, backup_password_encrypted
		 FROM database_servers WHERE is_default = 1 AND is_active = 1 LIMIT 1`).
		Scan(&username, &encrypted)
	if err != nil {
		return "", "", err
	}
	if strings.TrimSpace(username) == "" || strings.TrimSpace(encrypted) == "" {
		return "", "", errors.New("dedicated database backup credential is not configured")
	}
	return username, encrypted, nil
}

// SetServer saves or updates a database server configuration.
func (s *Store) SetServer(ctx context.Context, svr *Server) error {
	if svr.ID > 0 {
		_, err := s.db.ExecContext(ctx,
			`UPDATE database_servers SET name=?, engine=?, host=?, port=?, username=?, password_encrypted=?, backup_username=?, backup_password_encrypted=?, is_active=?, is_default=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`,
			svr.Name, svr.Engine, svr.Host, svr.Port, svr.Username, svr.PasswordEncrypted, svr.BackupUsername, svr.BackupPasswordEncrypted, boolToInt(svr.IsActive), boolToInt(svr.IsDefault), svr.ID)
		return err
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO database_servers (name, engine, host, port, username, password_encrypted, backup_username, backup_password_encrypted, is_active, is_default, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		svr.Name, svr.Engine, svr.Host, svr.Port, svr.Username, svr.PasswordEncrypted, svr.BackupUsername, svr.BackupPasswordEncrypted, boolToInt(svr.IsActive), boolToInt(svr.IsDefault))
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	svr.ID = id
	return nil
}

// ─── Encryption ────────────────────────────────────────────

// EncryptPassword encrypts a plaintext password using AES-256-GCM.
func (s *Store) EncryptPassword(plaintext string) (string, error) {
	block, err := aes.NewCipher(s.encKey)
	if err != nil {
		return "", err
	}
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aesGCM.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := aesGCM.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// DecryptPassword decrypts an encrypted password.
func (s *Store) DecryptPassword(encrypted string) (string, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(s.encKey)
	if err != nil {
		return "", err
	}
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonceSize := aesGCM.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", errors.New("ciphertext too short")
	}
	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := aesGCM.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

// ─── MySQL Driver ──────────────────────────────────────────

// MySQLConn holds a live MySQL connection.
type MySQLConn struct {
	db     *sql.DB
	engine string
}

// Connect opens a direct MySQL connection using the server credentials.
func Connect(svr *Server, password string) (*MySQLConn, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/?charset=utf8mb4&parseTime=true&timeout=5s",
		svr.Username, password, svr.Host, svr.Port)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("mysql open: %w", err)
	}
	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("mysql ping: %w", err)
	}

	// Detect engine
	engine := "MySQL"
	var ver string
	if err := db.QueryRow("SELECT VERSION()").Scan(&ver); err == nil {
		if strings.Contains(strings.ToLower(ver), "mariadb") || strings.Contains(strings.ToLower(ver), "mari") {
			engine = "MariaDB"
		}
	}

	return &MySQLConn{db: db, engine: engine}, nil
}

// Close closes the MySQL connection.
func (m *MySQLConn) Close() error {
	return m.db.Close()
}

// Engine returns the detected database engine.
func (m *MySQLConn) Engine() string { return m.engine }

// ListDatabases returns all non-system databases.
func (m *MySQLConn) ListDatabases(ctx context.Context) ([]string, error) {
	rows, err := m.db.QueryContext(ctx,
		`SELECT SCHEMA_NAME FROM information_schema.SCHEMATA
		 WHERE SCHEMA_NAME NOT IN ('information_schema','performance_schema','mysql','sys')
		 ORDER BY SCHEMA_NAME`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, nil
}

// DatabaseExists reports whether a schema exists on this server.
func (m *MySQLConn) DatabaseExists(ctx context.Context, name string) (bool, error) {
	var count int
	err := m.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.SCHEMATA WHERE SCHEMA_NAME = ?`, name).Scan(&count)
	return count > 0, err
}

// DatabaseAccessible verifies schema exists and current user can select it.
// information_schema may list schemas hidden from user's actual privileges.
func (m *MySQLConn) DatabaseAccessible(ctx context.Context, name string) (bool, error) {
	_, err := m.db.ExecContext(ctx, "USE `"+safeIdent(name)+"`")
	return err == nil, err
}

// ListUsers returns all non-system MySQL users.
func (m *MySQLConn) ListUsers(ctx context.Context) ([]DBUserInfo, error) {
	rows, err := m.db.QueryContext(ctx,
		`SELECT User, Host FROM mysql.user
		 WHERE User NOT IN ('root','mariadb.sys','mysql')
		 ORDER BY User`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DBUserInfo
	for rows.Next() {
		var u DBUserInfo
		if err := rows.Scan(&u.Username, &u.Host); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, nil
}

// DBUserInfo holds MySQL user metadata.
type DBUserInfo struct {
	Username string `json:"username"`
	Host     string `json:"host"`
}

// CreateDatabase creates a new database.
func (m *MySQLConn) CreateDatabase(ctx context.Context, name string, charset, collation string) error {
	if charset == "" {
		charset = "utf8mb4"
	}
	if collation == "" {
		collation = "utf8mb4_unicode_ci"
	}
	q := fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s` CHARACTER SET %s COLLATE %s", safeIdent(name), safeIdent(charset), safeIdent(collation))
	_, err := m.db.ExecContext(ctx, q)
	return err
}

// DropDatabase drops a database.
func (m *MySQLConn) DropDatabase(ctx context.Context, name string) error {
	q := fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", safeIdent(name))
	_, err := m.db.ExecContext(ctx, q)
	return err
}

// HasDatabase checks if a database exists.
func (m *MySQLConn) HasDatabase(ctx context.Context, name string) (bool, error) {
	var count int
	err := m.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM information_schema.SCHEMATA WHERE SCHEMA_NAME = ?`, name).Scan(&count)
	return count > 0, err
}

// CreateUser creates a MySQL/MariaDB user and grants privileges.
func (m *MySQLConn) CreateUser(ctx context.Context, username, password, host, dbName, privileges string) error {
	if host == "" {
		host = "%"
	}
	if privileges == "" {
		privileges = "ALL"
	}
	if err := siteinput.DBPrivileges(privileges); err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	if err := siteinput.DBHost(host); err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	if err := siteinput.DBPassword(password); err != nil {
		return fmt.Errorf("create user: %w", err)
	}

	// Determine auth plugin
	// MariaDB 10.x uses mysql_native_password by default and only supports IDENTIFIED BY.
	// MySQL 8.0+ uses caching_sha2_password by default; we use mysql_native_password for compatibility.
	authClause := fmt.Sprintf("IDENTIFIED BY '%s'", safeSQLString(password))
	if !strings.Contains(strings.ToLower(m.engine), "mariadb") {
		authClause = fmt.Sprintf("IDENTIFIED WITH mysql_native_password BY '%s'", safeSQLString(password))
	}

	// Create user
	_, err := m.db.ExecContext(ctx,
		fmt.Sprintf("CREATE USER IF NOT EXISTS '%s'@'%s' %s", safeIdent(username), safeIdent(host), authClause))
	if err != nil {
		return fmt.Errorf("create user: %w", err)
	}

	// Grant privileges
	if dbName != "" {
		grantSQL := fmt.Sprintf("GRANT %s ON `%s`.* TO '%s'@'%s'", privileges, safeIdent(dbName), safeIdent(username), safeIdent(host))
		_, err = m.db.ExecContext(ctx, grantSQL)
		if err != nil {
			return fmt.Errorf("grant: %w", err)
		}
	}

	// For phpmyadmin control user, also grant global privileges
	if username == "phpmyadmin" {
		globalGrant := fmt.Sprintf("GRANT ALL PRIVILEGES ON *.* TO '%s'@'%s' WITH GRANT OPTION", safeIdent(username), safeIdent(host))
		_, _ = m.db.ExecContext(ctx, globalGrant)
	}

	// Apply resource limits (MariaDB style, safe for MySQL too)
	_, _ = m.db.ExecContext(ctx,
		fmt.Sprintf("ALTER USER '%s'@'%s' REQUIRE NONE WITH MAX_QUERIES_PER_HOUR 0 MAX_CONNECTIONS_PER_HOUR 0 MAX_UPDATES_PER_HOUR 0 MAX_USER_CONNECTIONS 0",
			safeIdent(username), safeIdent(host)))

	_, err = m.db.ExecContext(ctx, "FLUSH PRIVILEGES")
	return err
}

// ScheduleDeleteUser creates a durable MariaDB/MySQL event that removes a
// temporary account after ttl. Unlike an in-process timer, this survives API restarts.
func (m *MySQLConn) ScheduleDeleteUser(ctx context.Context, username, host string, ttl time.Duration) error {
	if host == "" {
		host = "%"
	}
	seconds := int64(ttl.Seconds())
	if seconds < 60 {
		seconds = 60
	}
	eventName := "cleanup_" + safeIdent(username)
	if _, err := m.db.ExecContext(ctx, "SET GLOBAL event_scheduler = ON"); err != nil {
		return fmt.Errorf("enable event scheduler: %w", err)
	}
	query := fmt.Sprintf(
		"CREATE EVENT `mysql`.`%s` ON SCHEDULE AT CURRENT_TIMESTAMP + INTERVAL %d SECOND ON COMPLETION NOT PRESERVE DO DROP USER IF EXISTS '%s'@'%s'",
		eventName, seconds, safeIdent(username), safeIdent(host),
	)
	if _, err := m.db.ExecContext(ctx, query); err != nil {
		return fmt.Errorf("schedule user cleanup: %w", err)
	}
	return nil
}

// DeleteUser drops a database user.
func (m *MySQLConn) DeleteUser(ctx context.Context, username, host string) error {
	if host == "" {
		host = "%"
	}
	_, err := m.db.ExecContext(ctx,
		fmt.Sprintf("DROP USER IF EXISTS '%s'@'%s'", safeIdent(username), safeIdent(host)))
	if err != nil {
		return err
	}
	_, err = m.db.ExecContext(ctx, "FLUSH PRIVILEGES")
	return err
}

// ChangePassword changes a user's password.
func (m *MySQLConn) ChangePassword(ctx context.Context, username, host, newPassword string) error {
	if host == "" {
		host = "%"
	}
	if err := siteinput.DBHost(host); err != nil {
		return err
	}
	if err := siteinput.DBPassword(newPassword); err != nil {
		return err
	}
	// Use simple IDENTIFIED BY syntax (works on both MySQL 5.7/8.0 and MariaDB 10.x)
	q := fmt.Sprintf("ALTER USER '%s'@'%s' IDENTIFIED BY '%s'",
		safeIdent(username), safeIdent(host), safeSQLString(newPassword))
	_, err := m.db.ExecContext(ctx, q)
	if err != nil {
		return err
	}
	_, err = m.db.ExecContext(ctx, "FLUSH PRIVILEGES")
	return err
}

// ─── Helpers ───────────────────────────────────────────────

func safeIdent(s string) string {
	return strings.NewReplacer("`", "", "'", "", "\\", "", ";", "", "\n", "", "\r", "").Replace(s)
}

// safeSQLString quotes s for use inside a single-quoted SQL literal by doubling
// quotes, which behaves identically with and without NO_BACKSLASH_ESCAPES.
// Callers must have rejected backslashes and control characters beforehand
// (siteinput.DBPassword); nothing is silently dropped from the value.
func safeSQLString(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func sha256Hash(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// func sha256Hash is replaced below to avoid import cycle

func scanServer(row *sql.Row) (*Server, error) {
	var s Server
	var isActive, isDefault int
	err := row.Scan(&s.ID, &s.Name, &s.Engine, &s.Host, &s.Port, &s.Username, &s.PasswordEncrypted, &s.BackupUsername, &s.BackupPasswordEncrypted, &isActive, &isDefault, &s.CreatedAt, &s.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	s.IsActive = isActive == 1
	s.IsDefault = isDefault == 1
	return &s, nil
}

// GrantPrivileges grants privileges on a database to an existing user.
func (m *MySQLConn) GrantPrivileges(ctx context.Context, username, host, dbName, privileges string) error {
	q := fmt.Sprintf("GRANT %s ON `%s`.* TO '%s'@'%s'", privileges, safeIdent(dbName), safeIdent(username), safeIdent(host))
	if _, err := m.db.ExecContext(ctx, q); err != nil {
		return err
	}
	_, err := m.db.ExecContext(ctx, "FLUSH PRIVILEGES")
	return err
}

// Exec executes a raw SQL statement (for ALTER DATABASE etc).
func (m *MySQLConn) Exec(query string) (sql.Result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return m.db.ExecContext(ctx, query)
}
