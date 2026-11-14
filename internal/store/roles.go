package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"time"

	"sigardapanel/internal/rbac"
)

// Role is a built-in or custom role. Built-in roles' permissions come from
// internal/rbac; custom roles' from role_permissions.
type Role struct {
	ID          int64           `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	BuiltIn     bool            `json:"built_in"`
	CreatedBy   sql.NullInt64   `json:"-"`
	CreatedAt   time.Time       `json:"created_at"`
	Permissions map[string]bool `json:"-"`
}

// ErrRoleInUse is returned when deleting a role users still have.
var ErrRoleInUse = errors.New("role is assigned to users")

// customRoleNameRe: lowercase name, never a built-in or reserved name.
var customRoleNameRe = regexp.MustCompile(`^[a-z][a-z0-9_-]{1,31}$`)

// ValidCustomRoleName reports whether name may be used for a custom role.
func ValidCustomRoleName(name string) bool {
	return customRoleNameRe.MatchString(name) && !rbac.IsBuiltinRole(name) && name != "service_token"
}

// SQLiteRoleStore persists roles.
type SQLiteRoleStore struct {
	db *sql.DB
}

// NewRoleStore returns a SQLite-backed role store.
func NewRoleStore(db *sql.DB) *SQLiteRoleStore { return &SQLiteRoleStore{db: db} }

func (s *SQLiteRoleStore) scan(ctx context.Context, row interface{ Scan(...any) error }) (*Role, error) {
	r := &Role{}
	var builtIn int
	if err := row.Scan(&r.ID, &r.Name, &r.Description, &builtIn, &r.CreatedBy, &r.CreatedAt); err != nil {
		return nil, err
	}
	r.BuiltIn = builtIn == 1
	if r.BuiltIn || rbac.IsBuiltinRole(r.Name) {
		r.BuiltIn = true
		r.Permissions = rbac.BuiltinPermissions(r.Name)
		return r, nil
	}
	perms, err := s.permissions(ctx, r.ID)
	if err != nil {
		return nil, err
	}
	r.Permissions = perms
	return r, nil
}

func (s *SQLiteRoleStore) permissions(ctx context.Context, roleID int64) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT permission FROM role_permissions WHERE role_id = ?`, roleID)
	if err != nil {
		return nil, fmt.Errorf("role permissions: %w", err)
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		// A permission removed from the registry is ignored.
		if _, ok := rbac.Get(p); ok {
			out[p] = true
		}
	}
	return out, rows.Err()
}

const roleCols = `id, name, description, built_in, created_by, created_at`

// List returns every role, built-in first.
func (s *SQLiteRoleStore) List(ctx context.Context) ([]Role, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+roleCols+` FROM roles ORDER BY built_in DESC, id`)
	if err != nil {
		return nil, fmt.Errorf("list roles: %w", err)
	}
	var raw []*Role
	for rows.Next() {
		r := &Role{}
		var builtIn int
		if err := rows.Scan(&r.ID, &r.Name, &r.Description, &builtIn, &r.CreatedBy, &r.CreatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		r.BuiltIn = builtIn == 1 || rbac.IsBuiltinRole(r.Name)
		raw = append(raw, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]Role, 0, len(raw))
	for _, r := range raw {
		if r.BuiltIn {
			r.Permissions = rbac.BuiltinPermissions(r.Name)
		} else if r.Permissions, err = s.permissions(ctx, r.ID); err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	return out, nil
}

// GetByID returns the role or nil.
func (s *SQLiteRoleStore) GetByID(ctx context.Context, id int64) (*Role, error) {
	r, err := s.scan(ctx, s.db.QueryRowContext(ctx, `SELECT `+roleCols+` FROM roles WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return r, err
}

// GetByName returns the role or nil.
func (s *SQLiteRoleStore) GetByName(ctx context.Context, name string) (*Role, error) {
	r, err := s.scan(ctx, s.db.QueryRowContext(ctx, `SELECT `+roleCols+` FROM roles WHERE name = ?`, name))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return r, err
}

// PermissionsOf returns the permission set of a role name: built-ins from
// the registry, custom roles from the database, nil for an unknown role.
func (s *SQLiteRoleStore) PermissionsOf(ctx context.Context, name string) (map[string]bool, error) {
	if p := rbac.BuiltinPermissions(name); p != nil {
		return p, nil
	}
	r, err := s.GetByName(ctx, name)
	if err != nil || r == nil {
		return nil, err
	}
	return r.Permissions, nil
}

// Exists reports whether name is a built-in or stored role.
func (s *SQLiteRoleStore) Exists(ctx context.Context, name string) (bool, error) {
	if rbac.IsBuiltinRole(name) {
		return true, nil
	}
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM roles WHERE name = ?`, name).Scan(&n); err != nil {
		return false, err
	}
	return n > 0, nil
}

func setPermissions(ctx context.Context, tx *sql.Tx, roleID int64, perms map[string]bool) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM role_permissions WHERE role_id = ?`, roleID); err != nil {
		return err
	}
	for _, p := range rbac.Sorted(perms) {
		if _, ok := rbac.Get(p); !ok {
			return fmt.Errorf("unknown permission %q", p)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO role_permissions (role_id, permission) VALUES (?, ?)`, roleID, p); err != nil {
			return err
		}
	}
	return nil
}

// Create stores a custom role with its permissions.
func (s *SQLiteRoleStore) Create(ctx context.Context, r *Role) error {
	if !ValidCustomRoleName(r.Name) {
		return fmt.Errorf("invalid role name %q", r.Name)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `INSERT INTO roles (name, description, built_in, created_by) VALUES (?, ?, 0, ?)`,
		r.Name, r.Description, r.CreatedBy)
	if err != nil {
		return fmt.Errorf("create role: %w", err)
	}
	if r.ID, err = res.LastInsertId(); err != nil {
		return err
	}
	if err := setPermissions(ctx, tx, r.ID, r.Permissions); err != nil {
		return err
	}
	return tx.Commit()
}

// Update changes a custom role's description and permissions.
func (s *SQLiteRoleStore) Update(ctx context.Context, id int64, description string, perms map[string]bool) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE roles SET description = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND built_in = 0`, description, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("role %d is not a custom role", id)
	}
	if err := setPermissions(ctx, tx, id, perms); err != nil {
		return err
	}
	return tx.Commit()
}

// Delete removes a custom role nobody has.
func (s *SQLiteRoleStore) Delete(ctx context.Context, id int64) error {
	r, err := s.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if r == nil || r.BuiltIn {
		return fmt.Errorf("role %d is not a custom role", id)
	}
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE role = ?`, r.Name).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return ErrRoleInUse
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM roles WHERE id = ? AND built_in = 0`, id)
	return err
}

// CountUsers returns how many users have the role.
func (s *SQLiteRoleStore) CountUsers(ctx context.Context, name string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE role = ?`, name).Scan(&n)
	return n, err
}
