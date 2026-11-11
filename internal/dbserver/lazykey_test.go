package dbserver

import (
	"bytes"
	"testing"
)

// The key is resolved on the first encrypt/decrypt, never at construction,
// so commands that only open the store (e.g. "sigardapanel init") do not
// resolve (and warn about) the panel encryption key.
func TestNewStoreWithKeyFunc_ResolvesKeyLazilyAndOnce(t *testing.T) {
	calls := 0
	s := NewStoreWithKeyFunc(nil, func() []byte {
		calls++
		return bytes.Repeat([]byte{7}, 32)
	})
	if calls != 0 {
		t.Fatalf("key resolved at construction (%d calls)", calls)
	}
	enc, err := s.EncryptPassword("secret")
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.DecryptPassword(enc)
	if err != nil || got != "secret" {
		t.Fatalf("round trip: %q %v", got, err)
	}
	if calls != 1 {
		t.Fatalf("key resolved %d times, want 1", calls)
	}

	// Same key through the eager constructor decrypts it.
	if got, err := NewStore(nil, bytes.Repeat([]byte{7}, 32)).DecryptPassword(enc); err != nil || got != "secret" {
		t.Fatalf("eager store: %q %v", got, err)
	}
}
