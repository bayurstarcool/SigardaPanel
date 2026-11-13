package secrets

import (
	"context"
	"database/sql"
	"log/slog"

	"sigardapanel/internal/config"
)

// encryptedDataQueries count values that were encrypted with the panel's
// encryption key. If any exist while no explicit key is configured, they were
// encrypted with the derived fallback key and switching keys would make them
// unreadable.
var encryptedDataQueries = []string{
	`SELECT COUNT(*) FROM database_servers WHERE COALESCE(password_encrypted, '') != ''`,
	`SELECT COUNT(*) FROM database_servers WHERE COALESCE(backup_password_encrypted, '') != ''`,
	`SELECT COUNT(*) FROM db_users WHERE COALESCE(encrypted_password, '') != ''`,
	`SELECT COUNT(*) FROM jobs WHERE payload LIKE '%` + prefix + `%'`,
	`SELECT COUNT(*) FROM users WHERE COALESCE(totp_secret, '') LIKE '` + prefix + `%'`,
	`SELECT COUNT(*) FROM cloudflare_config WHERE COALESCE(api_token, '') LIKE '` + prefix + `%'`,
	`SELECT COUNT(*) FROM backup_storage_providers WHERE COALESCE(secret_key, '') LIKE '` + prefix + `%'`,
}

// hasEncryptedData reports whether any encrypted value is stored. Any query
// error is treated as "yes" so an unexpected schema never triggers a key
// switch.
func hasEncryptedData(ctx context.Context, db *sql.DB) bool {
	for _, q := range encryptedDataQueries {
		var n int
		if err := db.QueryRowContext(ctx, q).Scan(&n); err != nil {
			slog.Warn("encryption key: cannot inspect existing secrets; keeping current key", "error", err)
			return true
		}
		if n > 0 {
			return true
		}
	}
	return false
}

// InitEncryptionKey runs once at API start-up, after migrations and before
// anything is encrypted.
//
//   - SIGARDAPANEL_ENCRYPTION_KEY or an existing key file: nothing to do.
//   - No key yet and nothing encrypted (new install): generate a random key,
//     persist it to config.EncryptionKeyFile() with mode 0600, and use it.
//   - No key but data already encrypted with the derived fallback: keep the
//     fallback (so that data still decrypts) and log a loud warning.
func InitEncryptionKey(ctx context.Context, db *sql.DB) {
	switch config.EncryptionKeySource() {
	case config.KeySourceEnv:
		return
	case config.KeySourceFile:
		slog.Info("encryption key loaded from file", "path", config.EncryptionKeyFile())
		return
	}

	if hasEncryptedData(ctx, db) {
		slog.Warn("SECURITY WARNING: SIGARDAPANEL_ENCRYPTION_KEY is not set and existing secrets " +
			"(database passwords, job credentials, 2FA secrets) are encrypted with the weak derived " +
			"fallback key (sha256 of DB path + agent token). Keeping it so they still decrypt. " +
			"Changing SIGARDAPANEL_AGENT_TOKEN or SIGARDAPANEL_DB_PATH will make them unreadable. " +
			"Plan a re-encryption with an explicit SIGARDAPANEL_ENCRYPTION_KEY.")
		return
	}

	path, err := config.GenerateEncryptionKeyFile()
	if err != nil {
		slog.Warn("SECURITY WARNING: SIGARDAPANEL_ENCRYPTION_KEY is not set and a key file could not be "+
			"created; falling back to the weak derived key", "path", path, "error", err)
		return
	}
	slog.Warn("generated a new random encryption key; back this file up together with the database "+
		"(secrets cannot be decrypted without it)", "path", path)
}
