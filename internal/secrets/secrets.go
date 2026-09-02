// Package secrets provides authenticated encryption for sensitive values that
// must be persisted (e.g. credentials embedded in queued job payloads) but
// should never be stored in plaintext at rest.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"strings"

	"sigardapanel/internal/config"
)

// prefix marks a value as encrypted by this package. Values without the prefix
// are treated as plaintext (for backward compatibility and idempotent decrypt).
const prefix = "enc:v1:"

// Encrypt returns an AES-256-GCM ciphertext of plaintext, tagged with prefix
// and base64-encoded. Empty input is returned unchanged.
func Encrypt(plaintext string) (string, error) {
	return encryptWith(config.EncryptionKey(), plaintext)
}

func encryptWith(key []byte, plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ct := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return prefix + base64.StdEncoding.EncodeToString(ct), nil
}

// IsEncrypted reports whether value carries this package's ciphertext prefix.
func IsEncrypted(value string) bool {
	return strings.HasPrefix(value, prefix)
}

// Decrypt reverses Encrypt. If value is not prefixed it is returned unchanged,
// so callers can safely decrypt values that may or may not be encrypted.
func Decrypt(value string) (string, error) {
	return decryptWith(config.EncryptionKey(), value)
}

func decryptWith(key []byte, value string) (string, error) {
	if !strings.HasPrefix(value, prefix) {
		return value, nil
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, prefix))
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("ciphertext too short")
	}
	nonce, ct := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	pt, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", err
	}
	return string(pt), nil
}
