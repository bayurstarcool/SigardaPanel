// Package license verifies SigardaPanel license keys offline.
//
// A key is issued (signed) by the master panel with its ed25519 private key
// and verified here with the matching public key built into the panel, so
// no license server is contacted and a key cannot be made up or edited:
//
//	SPL1.<base64url(JSON claims)>.<base64url(ed25519 signature)>
//
// The signature covers "SPL1." followed by the encoded claims. See
// docs/LICENSING.md for issuing keys.
package license

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const prefix = "SPL1."

// Claims is what a license grants.
type Claims struct {
	ID         string    `json:"id"`       // license id (from the master)
	Licensee   string    `json:"licensee"` // customer name
	Email      string    `json:"email,omitempty"`
	Tier       string    `json:"tier"`        // free, basic, pro, enterprise, max
	MaxServers int       `json:"max_servers"` // 0 = tier default
	IssuedAt   time.Time `json:"issued_at"`
	ExpiresAt  time.Time `json:"expires_at,omitempty"` // zero = never
	// InstallID binds the key to one panel installation (its install id):
	// another panel refuses it. Keys the master delivers through check-in
	// (free and upgraded licenses) are bound; keys issued by hand may not be.
	InstallID string `json:"install_id,omitempty"`
}

var (
	ErrFormat       = errors.New("not a SigardaPanel license key")
	ErrSignature    = errors.New("license key signature is not valid (not issued by SigardaPanel, or altered)")
	ErrExpired      = errors.New("license has expired")
	ErrNoKeys       = errors.New("this build has no license public key, so it cannot verify licenses")
	ErrBadClaims    = errors.New("license key has missing or invalid fields")
	ErrOtherInstall = errors.New("license key was issued for another installation of SigardaPanel")
	validTiers      = map[string]bool{"free": true, "basic": true, "pro": true, "enterprise": true, "max": true}
	base64Encoder   = base64.RawURLEncoding
)

// PublicKeys are the trusted ed25519 public keys (base64, comma separated).
// Set at build time by the release (-ldflags "-X
// sigardapanel/internal/license.PublicKeys=..."); several allow a key
// rotation without invalidating issued licenses.
var PublicKeys = defaultPublicKeys

func trusted() ([]ed25519.PublicKey, error) {
	var out []ed25519.PublicKey
	for _, k := range strings.Split(PublicKeys, ",") {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		raw, err := base64.StdEncoding.DecodeString(k)
		if err != nil || len(raw) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("invalid license public key in this build")
		}
		out = append(out, ed25519.PublicKey(raw))
	}
	if len(out) == 0 {
		return nil, ErrNoKeys
	}
	return out, nil
}

// Parse checks the key's format and signature and returns its claims,
// without looking at the expiry (Verify does both).
func Parse(key string) (*Claims, error) {
	key = strings.Join(strings.Fields(key), "") // pasted keys may wrap
	if !strings.HasPrefix(key, prefix) || len(key) > 4096 {
		return nil, ErrFormat
	}
	parts := strings.Split(strings.TrimPrefix(key, prefix), ".")
	if len(parts) != 2 {
		return nil, ErrFormat
	}
	payload, err1 := base64Encoder.DecodeString(parts[0])
	sig, err2 := base64Encoder.DecodeString(parts[1])
	if err1 != nil || err2 != nil || len(sig) != ed25519.SignatureSize {
		return nil, ErrFormat
	}
	keys, err := trusted()
	if err != nil {
		return nil, err
	}
	signed := []byte(prefix + parts[0])
	ok := false
	for _, pk := range keys {
		if ed25519.Verify(pk, signed, sig) {
			ok = true
			break
		}
	}
	if !ok {
		return nil, ErrSignature
	}
	var c Claims
	dec := json.NewDecoder(strings.NewReader(string(payload)))
	if err := dec.Decode(&c); err != nil {
		return nil, ErrBadClaims
	}
	if c.ID == "" || !validTiers[c.Tier] || c.MaxServers < 0 || c.IssuedAt.IsZero() {
		return nil, ErrBadClaims
	}
	return &c, nil
}

// Verify parses key and checks that it has not expired at now.
func Verify(key string, now time.Time) (*Claims, error) {
	c, err := Parse(key)
	if err != nil {
		return nil, err
	}
	if !c.ExpiresAt.IsZero() && !now.Before(c.ExpiresAt) {
		return c, ErrExpired
	}
	return c, nil
}

// VerifyFor is Verify plus the installation check: a key bound to an
// install id is valid only on that installation.
func VerifyFor(key, installID string, now time.Time) (*Claims, error) {
	c, err := Verify(key, now)
	if err != nil {
		return c, err
	}
	if c.InstallID != "" && c.InstallID != installID {
		return c, ErrOtherInstall
	}
	return c, nil
}

// Sign issues a key for c with priv (the master panel's private key).
func Sign(priv ed25519.PrivateKey, c Claims) (string, error) {
	if c.ID == "" || !validTiers[c.Tier] || c.IssuedAt.IsZero() {
		return "", ErrBadClaims
	}
	payload, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	body := prefix + base64Encoder.EncodeToString(payload)
	sig := ed25519.Sign(priv, []byte(body))
	return body + "." + base64Encoder.EncodeToString(sig), nil
}
