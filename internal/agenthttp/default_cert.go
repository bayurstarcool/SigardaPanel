package agenthttp

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"log/slog"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

// defaultSSLCertDir holds the fallback certificate every generated vhost
// references (ssl_certificate /etc/nginx/ssl/sigardapanel.crt) until the site
// gets its own Let's Encrypt certificate. It must match internal/nginx.
const defaultSSLCertDir = "/etc/nginx/ssl"

// ensureDefaultSSLCert creates a self-signed placeholder certificate in dir
// when sigardapanel.crt/.key are missing. Without it the first vhost on a
// fresh server fails "nginx -t" (cannot load certificate), so no site can be
// created. Existing files (e.g. an operator's wildcard certificate) are never
// touched.
func ensureDefaultSSLCert(dir string) error {
	crtPath := filepath.Join(dir, "sigardapanel.crt")
	keyPath := filepath.Join(dir, "sigardapanel.key")
	_, crtErr := os.Stat(crtPath)
	_, keyErr := os.Stat(keyPath)
	if crtErr == nil && keyErr == nil {
		return nil
	}
	if (crtErr == nil) != (keyErr == nil) {
		// One half exists: do not overwrite what an operator put there.
		return fmt.Errorf("default certificate incomplete: need both %s and %s", crtPath, keyPath)
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("generate key: %w", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
	if err != nil {
		return fmt.Errorf("serial: %w", err)
	}
	host, _ := os.Hostname()
	if host == "" {
		host = "localhost"
	}
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: host, Organization: []string{"SigardaPanel placeholder"}},
		DNSNames:              []string{host},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return fmt.Errorf("create certificate: %w", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return fmt.Errorf("marshal key: %w", err)
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		return fmt.Errorf("write key: %w", err)
	}
	if err := os.WriteFile(crtPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644); err != nil {
		_ = os.Remove(keyPath)
		return fmt.Errorf("write certificate: %w", err)
	}
	slog.Info("agent: created self-signed placeholder certificate for new vhosts", "cert", crtPath)
	return nil
}
