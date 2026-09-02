package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// HashServiceToken returns the SHA-256 hex digest of a service token raw value.
// Same algorithm as HashToken — suitable for DB storage.
func HashServiceToken(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

// ValidateScopes checks whether the token's scopes allow the required scope.
// A token with scope "*" allows everything. Otherwise, scopes are comma-separated
// and the required scope must appear in the list.
func ValidateScopes(tokenScopes, requiredScope string) bool {
	if tokenScopes == "*" {
		return true
	}
	for _, s := range strings.Split(tokenScopes, ",") {
		if strings.TrimSpace(s) == requiredScope {
			return true
		}
	}
	return false
}
