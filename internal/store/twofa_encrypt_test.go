package store

import (
	"strings"
	"testing"

	"sigardapanel/internal/secrets"
)

func twofaUser(t *testing.T) (*SQLiteTwoFactorStore, *SQLiteUserStore, int64) {
	t.Helper()
	t.Setenv("SIGARDAPANEL_ENCRYPTION_KEY", "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f")
	d := testDB(t)
	us := NewUserStore(d)
	u := &User{Username: "totp", Email: "totp@example.com", PasswordHash: "x", Role: "user"}
	if err := us.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	return NewSQLiteTwoFactorStore(d), us, u.ID
}

func rawTOTP(t *testing.T, s *SQLiteTwoFactorStore, id int64) string {
	t.Helper()
	var v string
	if err := s.db.QueryRow(`SELECT COALESCE(totp_secret, '') FROM users WHERE id = ?`, id).Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

const testTOTP = "JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP"

func TestTOTPSecret_EncryptedAtRest(t *testing.T) {
	s, us, id := twofaUser(t)
	if err := s.SetTOTPSecret(ctx, id, testTOTP); err != nil {
		t.Fatal(err)
	}
	raw := rawTOTP(t, s, id)
	if !secrets.IsEncrypted(raw) || strings.Contains(raw, testTOTP) {
		t.Fatalf("stored secret not encrypted: %q", raw)
	}
	if got, err := s.GetTOTPSecret(ctx, id); err != nil || got != testTOTP {
		t.Fatalf("GetTOTPSecret = %q, %v", got, err)
	}
	u, err := us.GetByID(ctx, id)
	if err != nil || u.TOTPSecret != testTOTP {
		t.Fatalf("user.TOTPSecret = %q, %v", u.TOTPSecret, err)
	}
	if err := s.SetTOTPSecret(ctx, id, ""); err != nil {
		t.Fatal(err)
	}
	if raw := rawTOTP(t, s, id); raw != "" {
		t.Fatalf("empty secret stored as %q", raw)
	}
}

func TestTOTPSecret_LegacyPlaintextReencryptedOnRead(t *testing.T) {
	s, us, id := twofaUser(t)
	if _, err := s.db.Exec(`UPDATE users SET totp_secret = ? WHERE id = ?`, testTOTP, id); err != nil {
		t.Fatal(err)
	}
	u, _ := us.GetByID(ctx, id)
	if u.TOTPSecret != testTOTP {
		t.Fatalf("legacy plaintext not readable via user: %q", u.TOTPSecret)
	}
	if got, err := s.GetTOTPSecret(ctx, id); err != nil || got != testTOTP {
		t.Fatalf("GetTOTPSecret legacy = %q, %v", got, err)
	}
	if raw := rawTOTP(t, s, id); !secrets.IsEncrypted(raw) {
		t.Fatalf("legacy secret not re-encrypted on read: %q", raw)
	}
	if got, _ := s.GetTOTPSecret(ctx, id); got != testTOTP {
		t.Fatalf("after re-encrypt = %q", got)
	}
}

func TestEncryptLegacyTOTPSecrets(t *testing.T) {
	s, us, id := twofaUser(t)
	u2 := &User{Username: "enc", Email: "enc@example.com", PasswordHash: "x", Role: "user"}
	if err := us.Create(ctx, u2); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE users SET totp_secret = ? WHERE id = ?`, testTOTP, id); err != nil {
		t.Fatal(err)
	}
	if err := s.SetTOTPSecret(ctx, u2.ID, "ALREADYENCRYPTED"); err != nil {
		t.Fatal(err)
	}
	before := rawTOTP(t, s, u2.ID)

	n, err := s.EncryptLegacyTOTPSecrets(ctx)
	if err != nil || n != 1 {
		t.Fatalf("EncryptLegacyTOTPSecrets = %d, %v; want 1", n, err)
	}
	if raw := rawTOTP(t, s, id); !secrets.IsEncrypted(raw) {
		t.Fatalf("legacy row not encrypted: %q", raw)
	}
	if rawTOTP(t, s, u2.ID) != before {
		t.Fatal("already-encrypted row was rewritten")
	}
	if n, _ := s.EncryptLegacyTOTPSecrets(ctx); n != 0 {
		t.Fatalf("second run converted %d rows, want 0", n)
	}
	if got, _ := s.GetTOTPSecret(ctx, id); got != testTOTP {
		t.Fatalf("decrypted = %q", got)
	}
}

func TestTOTPSecret_WrongKeyFailsClosed(t *testing.T) {
	s, us, id := twofaUser(t)
	if err := s.SetTOTPSecret(ctx, id, testTOTP); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SIGARDAPANEL_ENCRYPTION_KEY", "ff0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f")
	if _, err := s.GetTOTPSecret(ctx, id); err == nil {
		t.Fatal("expected decrypt error with wrong key")
	}
	if u, _ := us.GetByID(ctx, id); u.TOTPSecret != "" {
		t.Fatalf("user.TOTPSecret with wrong key = %q, want empty (fail closed)", u.TOTPSecret)
	}
}
