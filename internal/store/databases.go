package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Database represents a database belonging to a site.
type Database struct {
	ID        int64
	SiteID    int64
	Name      string
	Type      string
	Charset   string
	Collation string
	SizeBytes int64
	CreatedAt time.Time
}

// DBUser represents a database user with access to a specific database.
type DBUser struct {
	ID                int64
	DatabaseID        int64
	Username          string
	PasswordHash      string
	EncryptedPassword string
	HostPattern       string
	Privileges        string
	CreatedAt         time.Time
}

// DatabaseRepository defines database persistence operations.
type DatabaseRepository interface {
	Create(ctx context.Context, db *Database) error
	GetByID(ctx context.Context, id int64) (*Database, error)
	GetByName(ctx context.Context, name string) (*Database, error)
	ListBySite(ctx context.Context, siteID int64) ([]Database, error)
	ListAll(ctx context.Context) ([]Database, error)
	UpdateCharset(ctx context.Context, id int64, charset, collation string) error
	Delete(ctx context.Context, id int64) error
}

// DBUserRepository defines database user persistence operations.
type DBUserRepository interface {
	Create(ctx context.Context, u *DBUser) error
	GetByID(ctx context.Context, id int64) (*DBUser, error)
	ListByDatabase(ctx context.Context, dbID int64) ([]DBUser, error)
	GetByUsernameAndDatabase(ctx context.Context, username string, dbID int64) (*DBUser, error)
	UpdatePassword(ctx context.Context, id int64, hash string) error
	UpdateEncryptedPassword(ctx context.Context, id int64, encryptedPassword string) error
	UpdateHostAndPrivileges(ctx context.Context, id int64, host, privileges string) error
	Delete(ctx context.Context, id int64) error
}

// SQLiteDatabaseStore implements DatabaseRepository against SQLite.
type SQLiteDatabaseStore struct {
	db *sql.DB
}

// SQLiteDBUserStore implements DBUserRepository against SQLite.
type SQLiteDBUserStore struct {
	db *sql.DB
}

// NewDatabaseStore returns a new SQLite-backed database store.
func NewDatabaseStore(db *sql.DB) *SQLiteDatabaseStore {
	return &SQLiteDatabaseStore{db: db}
}

// NewDBUserStore returns a new SQLite-backed database user store.
func NewDBUserStore(db *sql.DB) *SQLiteDBUserStore {
	return &SQLiteDBUserStore{db: db}
}

// Create inserts a new database. Sets defaults for charset and collation.
func (s *SQLiteDatabaseStore) Create(ctx context.Context, db *Database) error {
	if db.Charset == "" {
		db.Charset = "utf8mb4"
	}
	if db.Collation == "" {
		db.Collation = "utf8mb4_unicode_ci"
	}

	res, err := s.db.ExecContext(ctx,
		`INSERT INTO databases (site_id, name, type, charset, collation, size_bytes) VALUES (?, ?, ?, ?, ?, ?)`,
		db.SiteID, db.Name, db.Type, db.Charset, db.Collation, db.SizeBytes,
	)
	if err != nil {
		return fmt.Errorf("insert database: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("last insert id: %w", err)
	}
	db.ID = id
	return nil
}

// GetByID retrieves a database by ID. Returns (nil, nil) if not found.
func (s *SQLiteDatabaseStore) GetByID(ctx context.Context, id int64) (*Database, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, site_id, name, type, charset, collation, size_bytes, created_at FROM databases WHERE id = ?`, id)
	return scanDatabase(row)
}

// GetByName retrieves a database by name. Returns (nil, nil) if not found.
func (s *SQLiteDatabaseStore) GetByName(ctx context.Context, name string) (*Database, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, site_id, name, type, charset, collation, size_bytes, created_at FROM databases WHERE name = ?`, name)
	return scanDatabase(row)
}

// ListBySite returns databases for a site, ordered by id ASC.
func (s *SQLiteDatabaseStore) ListBySite(ctx context.Context, siteID int64) ([]Database, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, site_id, name, type, charset, collation, size_bytes, created_at FROM databases WHERE site_id = ? ORDER BY id ASC`, siteID)
	if err != nil {
		return nil, fmt.Errorf("query databases: %w", err)
	}
	defer rows.Close()

	var out []Database
	for rows.Next() {
		var db Database
		if err := rows.Scan(&db.ID, &db.SiteID, &db.Name, &db.Type, &db.Charset, &db.Collation, &db.SizeBytes, &db.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan database: %w", err)
		}
		out = append(out, db)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}
	return out, nil
}

// ListAll returns all databases across all sites, ordered by id ASC.
func (s *SQLiteDatabaseStore) ListAll(ctx context.Context) ([]Database, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, site_id, name, type, charset, collation, size_bytes, created_at FROM databases ORDER BY id ASC`)
	if err != nil {
		return nil, fmt.Errorf("query all databases: %w", err)
	}
	defer rows.Close()

	var out []Database
	for rows.Next() {
		var db Database
		if err := rows.Scan(&db.ID, &db.SiteID, &db.Name, &db.Type, &db.Charset, &db.Collation, &db.SizeBytes, &db.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan database: %w", err)
		}
		out = append(out, db)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}
	return out, nil
}

// UpdateCharset updates charset and collation for a database.
func (s *SQLiteDatabaseStore) UpdateCharset(ctx context.Context, id int64, charset, collation string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE databases SET charset = ?, collation = ? WHERE id = ?`, charset, collation, id)
	if err != nil {
		return fmt.Errorf("update database charset: %w", err)
	}
	return nil
}

// Delete removes a database by ID.
func (s *SQLiteDatabaseStore) Delete(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM databases WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete database: %w", err)
	}
	return nil
}

// Create inserts a new database user.
func (s *SQLiteDBUserStore) Create(ctx context.Context, u *DBUser) error {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO db_users (database_id, username, password_hash, encrypted_password, host_pattern, privileges) VALUES (?, ?, ?, ?, ?, ?)`,
		u.DatabaseID, u.Username, u.PasswordHash, u.EncryptedPassword, u.HostPattern, u.Privileges,
	)
	if err != nil {
		return fmt.Errorf("insert db user: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("last insert id: %w", err)
	}
	u.ID = id
	return nil
}

// GetByID retrieves a database user by ID. Returns (nil, nil) if not found.
func (s *SQLiteDBUserStore) GetByID(ctx context.Context, id int64) (*DBUser, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, database_id, username, password_hash, encrypted_password, host_pattern, privileges, created_at FROM db_users WHERE id = ?`, id)
	return scanDBUser(row)
}

// GetByUsernameAndDatabase retrieves a db user by username + database_id.
func (s *SQLiteDBUserStore) GetByUsernameAndDatabase(ctx context.Context, username string, dbID int64) (*DBUser, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, database_id, username, password_hash, encrypted_password, host_pattern, privileges, created_at FROM db_users WHERE username = ? AND database_id = ?`, username, dbID)
	return scanDBUser(row)
}

// ListByDatabase returns database users for a database, ordered by id ASC.
func (s *SQLiteDBUserStore) ListByDatabase(ctx context.Context, dbID int64) ([]DBUser, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, database_id, username, password_hash, encrypted_password, host_pattern, privileges, created_at FROM db_users WHERE database_id = ? ORDER BY id ASC`, dbID)
	if err != nil {
		return nil, fmt.Errorf("query db users: %w", err)
	}
	defer rows.Close()

	var out []DBUser
	for rows.Next() {
		var u DBUser
		if err := rows.Scan(&u.ID, &u.DatabaseID, &u.Username, &u.PasswordHash, &u.EncryptedPassword, &u.HostPattern, &u.Privileges, &u.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan db user: %w", err)
		}
		out = append(out, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}
	return out, nil
}

// UpdatePassword updates the password_hash for a db user.
func (s *SQLiteDBUserStore) UpdatePassword(ctx context.Context, id int64, hash string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE db_users SET password_hash = ? WHERE id = ?`, hash, id)
	if err != nil {
		return fmt.Errorf("update db user password: %w", err)
	}
	return nil
}

// UpdateEncryptedPassword stores the encrypted plaintext password.
func (s *SQLiteDBUserStore) UpdateEncryptedPassword(ctx context.Context, id int64, encryptedPassword string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE db_users SET encrypted_password = ? WHERE id = ?`, encryptedPassword, id)
	if err != nil {
		return fmt.Errorf("update encrypted password: %w", err)
	}
	return nil
}

// UpdateHostAndPrivileges updates host_pattern and privileges for a db user.
func (s *SQLiteDBUserStore) UpdateHostAndPrivileges(ctx context.Context, id int64, host, privileges string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE db_users SET host_pattern = ?, privileges = ? WHERE id = ?`, host, privileges, id)
	if err != nil {
		return fmt.Errorf("update host/privs: %w", err)
	}
	return nil
}

// Delete removes a database user by ID.
func (s *SQLiteDBUserStore) Delete(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM db_users WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete db user: %w", err)
	}
	return nil
}

func scanDatabase(row *sql.Row) (*Database, error) {
	var db Database
	err := row.Scan(&db.ID, &db.SiteID, &db.Name, &db.Type, &db.Charset, &db.Collation, &db.SizeBytes, &db.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan database: %w", err)
	}
	return &db, nil
}

func scanDBUser(row *sql.Row) (*DBUser, error) {
	var u DBUser
	err := row.Scan(&u.ID, &u.DatabaseID, &u.Username, &u.PasswordHash, &u.EncryptedPassword, &u.HostPattern, &u.Privileges, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan db user: %w", err)
	}
	return &u, nil
}
