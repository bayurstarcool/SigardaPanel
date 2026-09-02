package auth

import (
	"crypto/rand"
	"encoding/base64"
)

func generateSalt(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	return b, nil
}

func saltB64(b []byte) string {
	return base64.RawStdEncoding.EncodeToString(b)
}

func b64Decode(s string) ([]byte, error) {
	return base64.RawStdEncoding.DecodeString(s)
}