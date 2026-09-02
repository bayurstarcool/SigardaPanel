package auth

import "testing"

func TestHashPassword_VerifyPassword_Roundtrip(t *testing.T) {
	password := "correct-horse-battery-staple"

	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword error: %v", err)
	}

	if hash == "" {
		t.Fatal("hash is empty")
	}

	// Must not equal plaintext
	if hash == password {
		t.Fatal("hash equals plaintext")
	}

	// Verify correct password
	ok, err := VerifyPassword(hash, password)
	if err != nil {
		t.Fatalf("VerifyPassword error: %v", err)
	}
	if !ok {
		t.Fatal("correct password did not verify")
	}

	// Reject wrong password
	ok, err = VerifyPassword(hash, "wrong-password")
	if err != nil {
		t.Fatalf("VerifyPassword error: %v", err)
	}
	if ok {
		t.Fatal("wrong password verified")
	}
}

func TestHashPassword_DifferentSalts(t *testing.T) {
	password := "same-password"
	hash1, _ := HashPassword(password)
	hash2, _ := HashPassword(password)

	if hash1 == hash2 {
		t.Fatal("two hashes of the same password should differ (different salts)")
	}

	// Both should still verify
	if ok, _ := VerifyPassword(hash1, password); !ok {
		t.Fatal("hash1 verification failed")
	}
	if ok, _ := VerifyPassword(hash2, password); !ok {
		t.Fatal("hash2 verification failed")
	}
}

func TestVerifyPassword_InvalidHash(t *testing.T) {
	_, err := VerifyPassword("not-a-valid-pck-format", "password")
	if err == nil {
		t.Fatal("expected error for invalid hash format")
	}
}