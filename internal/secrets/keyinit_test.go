package secrets

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"sigardapanel/internal/config"
	"sigardapanel/internal/db"
	"sigardapanel/internal/migrations"
)

func keyTestDB(t *testing.T) (string, func() *dbHandle) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "panel.db")
	t.Setenv("SIGARDAPANEL_DB_PATH", dbPath)
	t.Setenv("SIGARDAPANEL_ENCRYPTION_KEY", "")
	t.Setenv("SIGARDAPANEL_ENCRYPTION_KEY_FILE", "")
	t.Setenv("SIGARDAPANEL_AGENT_TOKEN", "agent-token")
	return dir, func() *dbHandle {
		d, err := db.Open(dbPath)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { d.Close() })
		if err := migrations.Up(d, migrations.AllMigrations); err != nil {
			t.Fatal(err)
		}
		return &dbHandle{DB: d}
	}
}

type dbHandle struct{ DB *sql.DB }

func TestInitEncryptionKey_NewInstallGeneratesKeyFile(t *testing.T) {
	dir, open := keyTestDB(t)
	h := open()
	InitEncryptionKey(context.Background(), h.DB)

	path := filepath.Join(dir, "encryption.key")
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("key file not created: %v", err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("key file mode = %o, want 600", fi.Mode().Perm())
	}
	if src := config.EncryptionKeySource(); src != config.KeySourceFile {
		t.Fatalf("key source = %s, want file", src)
	}
	if string(config.EncryptionKey()) == string(config.DerivedEncryptionKey()) {
		t.Fatal("generated key equals the derived fallback")
	}
	enc, err := Encrypt("hello")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := Decrypt(enc); err != nil || got != "hello" {
		t.Fatalf("roundtrip with file key = %q, %v", got, err)
	}
}

func TestInitEncryptionKey_KeepsFallbackWhenDataExists(t *testing.T) {
	dir, open := keyTestDB(t)
	h := open()

	// Something already encrypted with the fallback key.
	enc, err := encryptWith(config.DerivedEncryptionKey(), "legacy")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.DB.Exec(`INSERT INTO jobs (type, payload) VALUES ('x', ?)`, `{"password":"`+enc+`"}`); err != nil {
		t.Fatal(err)
	}

	InitEncryptionKey(context.Background(), h.DB)

	if _, err := os.Stat(filepath.Join(dir, "encryption.key")); !os.IsNotExist(err) {
		t.Fatalf("key file must not be created when fallback-encrypted data exists (err=%v)", err)
	}
	if src := config.EncryptionKeySource(); src != config.KeySourceFallback {
		t.Fatalf("key source = %s, want fallback", src)
	}
	if got, err := Decrypt(enc); err != nil || got != "legacy" {
		t.Fatalf("legacy value no longer decrypts: %q, %v", got, err)
	}
}

func TestInitEncryptionKey_EnvKeyWins(t *testing.T) {
	dir, open := keyTestDB(t)
	t.Setenv("SIGARDAPANEL_ENCRYPTION_KEY", "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f")
	h := open()
	InitEncryptionKey(context.Background(), h.DB)
	if _, err := os.Stat(filepath.Join(dir, "encryption.key")); !os.IsNotExist(err) {
		t.Fatal("key file must not be created when env key is set")
	}
	if config.EncryptionKeySource() != config.KeySourceEnv {
		t.Fatal("env key not used")
	}
}
