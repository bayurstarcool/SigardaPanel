package store

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"sigardapanel/internal/secrets"
)

// TwoFactorRepository defines 2FA persistence operations.
type TwoFactorRepository interface {
	GetTOTPSecret(ctx context.Context, userID int64) (string, error)
	SetTOTPSecret(ctx context.Context, userID int64, secret string) error
	EncryptLegacyTOTPSecrets(ctx context.Context) (int, error)
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

// TOTP secrets are encrypted at rest with the panel encryption key
// (secrets.Encrypt, "enc:v1:" prefix). Rows written by older builds hold the
// base32 secret in plaintext; they are still accepted, re-encrypted on the
// next read, and in bulk at start-up by EncryptLegacyTOTPSecrets.

// decodeTOTPSecret returns the plaintext secret for a stored value, plus
// whether the stored value is legacy plaintext.
func decodeTOTPSecret(stored string) (secret string, legacy bool, err error) {
	if stored == "" {
		return "", false, nil
	}
	if !secrets.IsEncrypted(stored) {
		return stored, true, nil
	}
	pt, err := secrets.Decrypt(stored)
	if err != nil {
		return "", false, fmt.Errorf("decrypt totp secret: %w", err)
	}
	return pt, false, nil
}

// totpSecretForUser decodes a stored secret for a scanned user. On a decrypt
// failure it returns "" — login then fails closed because totp_enabled stays
// set and no secret can validate a code.
func totpSecretForUser(userID int64, stored string) string {
	secret, _, err := decodeTOTPSecret(stored)
	if err != nil {
		slog.Error("users: cannot decrypt TOTP secret (encryption key changed?)", "user_id", userID, "error", err)
		return ""
	}
	return secret
}

// GetTOTPSecret retrieves the (decrypted) TOTP secret for a user. A legacy
// plaintext secret is transparently re-encrypted.
func (s *SQLiteTwoFactorStore) GetTOTPSecret(ctx context.Context, userID int64) (string, error) {
	var stored string
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(totp_secret, '') FROM users WHERE id = ?`, userID).Scan(&stored)
	if err != nil {
		return "", fmt.Errorf("get totp secret: %w", err)
	}
	secret, legacy, err := decodeTOTPSecret(stored)
	if err != nil {
		return "", err
	}
	if legacy {
		s.reencrypt(ctx, userID, stored)
	}
	return secret, nil
}

// reencrypt replaces a legacy plaintext secret with its ciphertext, only if
// the row still holds that exact plaintext (no lost update).
func (s *SQLiteTwoFactorStore) reencrypt(ctx context.Context, userID int64, plaintext string) {
	enc, err := secrets.Encrypt(plaintext)
	if err != nil {
		slog.Warn("2fa: encrypt legacy totp secret", "user_id", userID, "error", err)
		return
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE users SET totp_secret = ? WHERE id = ? AND totp_secret = ?`, enc, userID, plaintext); err != nil {
		slog.Warn("2fa: store re-encrypted totp secret", "user_id", userID, "error", err)
	}
}

// SetTOTPSecret encrypts and stores the TOTP secret for a user.
func (s *SQLiteTwoFactorStore) SetTOTPSecret(ctx context.Context, userID int64, secret string) error {
	stored := secret
	if secret != "" {
		enc, err := secrets.Encrypt(secret)
		if err != nil {
			return fmt.Errorf("encrypt totp secret: %w", err)
		}
		stored = enc
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE users SET totp_secret = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		stored, userID)
	if err != nil {
		return fmt.Errorf("set totp secret: %w", err)
	}
	return nil
}

// EncryptLegacyTOTPSecrets encrypts every plaintext TOTP secret in place. Run
// at start-up after the encryption key is settled. Returns how many rows were
// converted.
func (s *SQLiteTwoFactorStore) EncryptLegacyTOTPSecrets(ctx context.Context) (int, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, totp_secret FROM users WHERE COALESCE(totp_secret, '') != '' AND totp_secret NOT LIKE 'enc:v1:%'`)
	if err != nil {
		return 0, fmt.Errorf("list legacy totp secrets: %w", err)
	}
	type row struct {
		id     int64
		secret string
	}
	var legacy []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.secret); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan legacy totp secret: %w", err)
		}
		legacy = append(legacy, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	n := 0
	for _, r := range legacy {
		if secrets.IsEncrypted(r.secret) {
			continue
		}
		enc, err := secrets.Encrypt(r.secret)
		if err != nil {
			return n, fmt.Errorf("encrypt totp secret for user %d: %w", r.id, err)
		}
		res, err := s.db.ExecContext(ctx,
			`UPDATE users SET totp_secret = ? WHERE id = ? AND totp_secret = ?`, enc, r.id, r.secret)
		if err != nil {
			return n, fmt.Errorf("store encrypted totp secret for user %d: %w", r.id, err)
		}
		if c, _ := res.RowsAffected(); c == 1 {
			n++
		}
	}
	return n, nil
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
	// Single atomic UPDATE so two concurrent logins cannot both spend one code.
	res, err := s.db.ExecContext(ctx,
		`UPDATE two_factor_recovery_codes SET used = 1 WHERE user_id = ? AND code_hash = ? AND used = 0`,
		userID, codeHash)
	if err != nil {
		return false, fmt.Errorf("verify recovery code: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("verify recovery code: %w", err)
	}
	return n == 1, nil
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
