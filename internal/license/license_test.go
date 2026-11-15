package license

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"
)

func newKey(t *testing.T) ed25519.PrivateKey {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	old := PublicKeys
	PublicKeys = base64.StdEncoding.EncodeToString(pub)
	t.Cleanup(func() { PublicKeys = old })
	return priv
}

func claims() Claims {
	now := time.Now().UTC().Truncate(time.Second)
	return Claims{ID: "lic_1", Licensee: "Acme", Tier: "pro", MaxServers: 10, IssuedAt: now, ExpiresAt: now.AddDate(1, 0, 0)}
}

func TestSignVerify(t *testing.T) {
	priv := newKey(t)
	key, err := Sign(priv, claims())
	if err != nil {
		t.Fatal(err)
	}
	c, err := Verify(key, time.Now())
	if err != nil || c.Tier != "pro" || c.MaxServers != 10 || c.Licensee != "Acme" {
		t.Fatalf("verify: %v %+v", err, c)
	}
	// wrapped / spaced when pasted
	if _, err := Verify(key[:20]+"\n "+key[20:], time.Now()); err != nil {
		t.Fatalf("wrapped key: %v", err)
	}
}

func TestExpired(t *testing.T) {
	priv := newKey(t)
	c := claims()
	key, _ := Sign(priv, c)
	if _, err := Verify(key, c.ExpiresAt.Add(time.Second)); !errors.Is(err, ErrExpired) {
		t.Fatalf("got %v, want expired", err)
	}
	c.ExpiresAt = time.Time{} // never expires
	key, _ = Sign(priv, c)
	if _, err := Verify(key, time.Now().AddDate(50, 0, 0)); err != nil {
		t.Fatalf("perpetual: %v", err)
	}
}

func TestTamperedAndForeign(t *testing.T) {
	priv := newKey(t)
	key, _ := Sign(priv, claims())
	// change the claims (tier) but keep the signature
	c := claims()
	c.Tier = "max"
	other, _ := Sign(priv, c)
	forged := strings.SplitN(other, ".", 3)
	orig := strings.SplitN(key, ".", 3)
	if _, err := Verify(forged[0]+"."+forged[1]+"."+orig[2], time.Now()); !errors.Is(err, ErrSignature) {
		t.Fatalf("tampered: %v", err)
	}
	// signed by another key
	_, foreign, _ := ed25519.GenerateKey(rand.Reader)
	fk, _ := Sign(foreign, claims())
	if _, err := Verify(fk, time.Now()); !errors.Is(err, ErrSignature) {
		t.Fatalf("foreign: %v", err)
	}
}

func TestFormatAndNoKeys(t *testing.T) {
	newKey(t)
	for _, k := range []string{"", "0123456789abcdef0123456789abcdef", "SPL1.x", "SPL1.a.b.c", "SPL2.YQ.YQ"} {
		if _, err := Verify(k, time.Now()); !errors.Is(err, ErrFormat) {
			t.Errorf("%q: %v", k, err)
		}
	}
	priv := newKey(t)
	key, _ := Sign(priv, claims())
	PublicKeys = ""
	if _, err := Verify(key, time.Now()); !errors.Is(err, ErrNoKeys) {
		t.Fatalf("no keys: %v", err)
	}
}

func TestRotation(t *testing.T) {
	oldPriv := newKey(t)
	oldPub := PublicKeys
	newPriv := newKey(t)
	PublicKeys = PublicKeys + "," + oldPub
	for _, p := range []ed25519.PrivateKey{oldPriv, newPriv} {
		k, _ := Sign(p, claims())
		if _, err := Verify(k, time.Now()); err != nil {
			t.Fatalf("rotation: %v", err)
		}
	}
}

func TestSignRejectsBadClaims(t *testing.T) {
	priv := newKey(t)
	c := claims()
	c.Tier = "free"
	if _, err := Sign(priv, c); !errors.Is(err, ErrBadClaims) {
		t.Fatal("free tier signed")
	}
}
