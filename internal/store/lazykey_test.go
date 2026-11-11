package store

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"sigardapanel/internal/dbserver"
)

// store.New must not resolve the encryption key: "sigardapanel init" opens
// the store on a fresh database before the API ever generated a key file,
// and resolving it there logged a misleading "derived fallback key" ERROR.
// A key file that appears after store.New is still the one used.
func TestStoreNew_DoesNotResolveEncryptionKeyEagerly(t *testing.T) {
	dir := t.TempDir()
	keyFile := filepath.Join(dir, "late.key")
	t.Setenv("SIGARDAPANEL_ENCRYPTION_KEY", "")
	t.Setenv("SIGARDAPANEL_ENCRYPTION_KEY_FILE", keyFile)

	s := New(testDB(t))

	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	if err := os.WriteFile(keyFile, []byte(hex.EncodeToString(key)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	enc, err := s.DBStore.EncryptPassword("pw")
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(key)
	if got, err := dbserver.NewStore(nil, sum[:]).DecryptPassword(enc); err != nil || got != "pw" {
		t.Fatalf("DBStore did not use the key file written after store.New: %q %v", got, err)
	}
}
