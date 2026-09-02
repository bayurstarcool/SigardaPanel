package auth

import (
	"strings"
	"testing"
)

func TestNewToken_Length(t *testing.T) {
	token, err := NewToken()
	if err != nil {
		t.Fatalf("NewToken error: %v", err)
	}

	// 32 bytes = 64 hex chars
	if len(token) != 64 {
		t.Fatalf("expected token length 64, got %d", len(token))
	}

	// Must be valid hex
	for _, c := range token {
		if !strings.ContainsRune("0123456789abcdef", c) {
			t.Fatalf("token contains non-hex char: %c", c)
		}
	}
}

func TestNewToken_Uniqueness(t *testing.T) {
	tokens := make(map[string]struct{})
	for i := 0; i < 100; i++ {
		token, _ := NewToken()
		if _, ok := tokens[token]; ok {
			t.Fatal("duplicate token generated")
		}
		tokens[token] = struct{}{}
	}
}

func TestHashToken_Deterministic(t *testing.T) {
	token := "abc123"
	h1 := HashToken(token)
	h2 := HashToken(token)

	if h1 != h2 {
		t.Fatal("HashToken not deterministic")
	}

	// 64 hex chars (SHA-256)
	if len(h1) != 64 {
		t.Fatalf("expected hash length 64, got %d", len(h1))
	}
}

