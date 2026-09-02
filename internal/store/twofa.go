package store

import (
	"context"
	"database/sql"
	"fmt"
)

// TwoFactorRepository defines 2FA persistence operations.
type TwoFactorRepository interface {
	GetTOTPSecret(ctx context.Context, userID int64) (string, error)
	SetTOTPSecret(ctx context.Context, userID int64, secret string) error
	EnableTOTP(ctx context.Context, userID int64) error
	DisableTOTP(ctx context.Context, userID int64) error
	IsTOTPEnabled(ctx context.Context, userID int64) (bool, error)
	StoreRecoveryCodes(ctx context.Context, userID int64, codeHashes []string) error
	VerifyRecoveryCode(ctx context.Context, userID int64, codeHash string) (bool, error)
	DeleteRecoveryCodes(ctx context.Context, userID int64) error
}

// SQLiteTwoFactorStore implements TwoFactorRepository with SQLite.
type SQLiteTwoFactorStore struct {
	db *sql.DB
}

// NewSQLiteTwoFactorStore creates a new SQLiteTwoFactorStore.
func NewSQLiteTwoFactorStore(db *sql.DB) *SQLiteTwoFactorStore {
	return &SQLiteTwoFactorStore{db: db}
}

// GetTOTPSecret retrieves the TOTP secret for a user.
func (s *SQLiteTwoFactorStore) GetTOTPSecret(ctx context.Context, userID int64) (string, error) {
	var secret string
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(totp_secret, '') FROM users WHERE id = ?`, userID).Scan(&secret)
	if err != nil {
		return "", fmt.Errorf("get totp secret: %w", err)
	}
	return secret, nil
}

// SetTOTPSecret sets the TOTP secret for a user.
func (s *SQLiteTwoFactorStore) SetTOTPSecret(ctx context.Context, userID int64, secret string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE users SET totp_secret = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		secret, userID)
	if err != nil {
		return fmt.Errorf("set totp secret: %w", err)
	}
	return nil
}

// EnableTOTP enables TOTP 2FA for a user.
func (s *SQLiteTwoFactorStore) EnableTOTP(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE users SET totp_enabled = 1, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		userID)
	if err != nil {
		return fmt.Errorf("enable totp: %w", err)
	}
	return nil
}

// DisableTOTP disables TOTP 2FA for a user.
func (s *SQLiteTwoFactorStore) DisableTOTP(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE users SET totp_enabled = 0, totp_secret = '', updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		userID)
	if err != nil {
		return fmt.Errorf("disable totp: %w", err)
	}
	return nil
}

// IsTOTPEnabled checks if TOTP 2FA is enabled for a user.
func (s *SQLiteTwoFactorStore) IsTOTPEnabled(ctx context.Context, userID int64) (bool, error) {
	var enabled int
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(totp_enabled, 0) FROM users WHERE id = ?`, userID).Scan(&enabled)
	if err != nil {
		return false, fmt.Errorf("check totp enabled: %w", err)
	}
	return enabled == 1, nil
}

// StoreRecoveryCodes stores hashed recovery codes for a user, replacing any existing codes.
func (s *SQLiteTwoFactorStore) StoreRecoveryCodes(ctx context.Context, userID int64, codeHashes []string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM two_factor_recovery_codes WHERE user_id = ?`, userID); err != nil {
		return fmt.Errorf("delete old recovery codes: %w", err)
	}

	stmt, err := tx.PrepareContext(ctx, `INSERT INTO two_factor_recovery_codes (user_id, code_hash) VALUES (?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare insert: %w", err)
	}
	defer stmt.Close()

	for _, h := range codeHashes {
		if _, err := stmt.ExecContext(ctx, userID, h); err != nil {
			return fmt.Errorf("insert recovery code: %w", err)
		}
	}

	return tx.Commit()
}

// VerifyRecoveryCode checks if a recovery code hash is valid and unused, then marks it used.
func (s *SQLiteTwoFactorStore) VerifyRecoveryCode(ctx context.Context, userID int64, codeHash string) (bool, error) {
	var id int64
	err := s.db.QueryRowContext(ctx,
		`SELECT id FROM two_factor_recovery_codes WHERE user_id = ? AND code_hash = ? AND used = 0`,
		userID, codeHash).Scan(&id)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, fmt.Errorf("verify recovery code: %w", err)
	}

	_, err = s.db.ExecContext(ctx,
		`UPDATE two_factor_recovery_codes SET used = 1 WHERE id = ?`, id)
	if err != nil {
		return false, fmt.Errorf("mark recovery code used: %w", err)
	}

	return true, nil
}

// DeleteRecoveryCodes removes all recovery codes for a user.
func (s *SQLiteTwoFactorStore) DeleteRecoveryCodes(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM two_factor_recovery_codes WHERE user_id = ?`, userID)
	if err != nil {
		return fmt.Errorf("delete recovery codes: %w", err)
	}
	return nil
}
