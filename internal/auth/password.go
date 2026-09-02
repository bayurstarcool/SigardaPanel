package auth

import (
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2id parameters used for password hashing.
const (
	argon2Time    = 1
	argon2Memory  = 64 * 1024 // 64 MB
	argon2Threads = 2
	argon2SaltLen = 16
	argon2KeyLen  = 32
)

// HashPassword hashes a plaintext password using Argon2id.
// Returns the hash in PHC string format.
func HashPassword(plain string) (string, error) {
	salt, err := generateSalt(argon2SaltLen)
	if err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}

	hash := argon2.IDKey([]byte(plain), salt, argon2Time, argon2Memory, argon2Threads, argon2KeyLen)

	// PHC format: $argon2id$v=19$m=65536,t=1,p=2$<salt>$<hash>
	encoded := fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		argon2Memory,
		argon2Time,
		argon2Threads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	)
	return encoded, nil
}

// VerifyPassword checks whether the plaintext password matches the PHC hash.
func VerifyPassword(hash, plain string) (bool, error) {
	params, salt, storedHash, err := parsePHC(hash)
	if err != nil {
		return false, fmt.Errorf("parse hash: %w", err)
	}

	computed := argon2.IDKey([]byte(plain), salt, params.time, params.memory, params.threads, argon2KeyLen)

	if subtle.ConstantTimeCompare(computed, storedHash) != 1 {
		return false, nil
	}
	return true, nil
}

type phcParams struct {
	time    uint32
	memory  uint32
	threads uint8
}

// parsePHC extracts parameters, salt, and hash from a PHC string.
// Expected format: $argon2id$v=19$m=65536,t=1,p=2$<salt>$<hash>
func parsePHC(phc string) (phcParams, []byte, []byte, error) {
	// Remove leading $ and split by $
	parts := strings.Split(phc, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || !strings.HasPrefix(parts[2], "v=") {
		return phcParams{}, nil, nil, fmt.Errorf("invalid PHC format")
	}

	// Parse params part: v=19$m=65536,t=1,p=2
	var version int
	var mem, t uint32
	var p uint8
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return phcParams{}, nil, nil, fmt.Errorf("parse version: %w", err)
	}
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &mem, &t, &p); err != nil {
		return phcParams{}, nil, nil, fmt.Errorf("parse params: %w", err)
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return phcParams{}, nil, nil, fmt.Errorf("decode salt: %w", err)
	}
	storedHash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return phcParams{}, nil, nil, fmt.Errorf("decode hash: %w", err)
	}

	return phcParams{
		time:    t,
		memory:  mem,
		threads: p,
	}, salt, storedHash, nil
}