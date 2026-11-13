package config

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

type Server struct {
	Addr         string
	TLSCert      string
	TLSKey       string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

func BackendServer() Server {
	return Server{
		Addr:         env("SIGARDAPANEL_API_ADDR", ":7700"),
		TLSCert:      env("SIGARDAPANEL_API_TLS_CERT", ""),
		TLSKey:       env("SIGARDAPANEL_API_TLS_KEY", ""),
		ReadTimeout:  durationEnv("SIGARDAPANEL_API_READ_TIMEOUT", 5*time.Minute),
		WriteTimeout: durationEnv("SIGARDAPANEL_API_WRITE_TIMEOUT", 15*time.Minute),
	}
}

func AgentServer() Server {
	return Server{
		Addr:         env("SIGARDAPANEL_AGENT_ADDR", ":7710"),
		TLSCert:      env("SIGARDAPANEL_AGENT_TLS_CERT", ""),
		TLSKey:       env("SIGARDAPANEL_AGENT_TLS_KEY", ""),
		ReadTimeout:  durationEnv("SIGARDAPANEL_AGENT_READ_TIMEOUT", 5*time.Minute),
		WriteTimeout: durationEnv("SIGARDAPANEL_AGENT_WRITE_TIMEOUT", 15*time.Minute),
	}
}

// BackendAgentURL returns the default agent endpoint used by the backend worker.
func BackendAgentURL() string {
	return env("SIGARDAPANEL_AGENT_URL", "http://localhost:7710")
}

// LocalServerIP returns the address shown for the auto-registered panel host.
// Operators can override private/NAT addresses with SIGARDAPANEL_LOCAL_IP.
func LocalServerIP() string {
	return env("SIGARDAPANEL_LOCAL_IP", "")
}

// AgentServerURL returns the local agent endpoint registered for this panel host.
func AgentServerURL() string {
	addr := env("SIGARDAPANEL_AGENT_ADDR", "127.0.0.1:7710")
	if strings.HasPrefix(addr, ":") {
		addr = "127.0.0.1" + addr
	}
	if host, port, err := net.SplitHostPort(addr); err == nil {
		if host == "" || host == "0.0.0.0" || host == "::" {
			host = "127.0.0.1"
		}
		return "http://" + net.JoinHostPort(host, port)
	}
	return "http://127.0.0.1:7710"
}

// BackendAgentToken returns the bearer token used by the backend worker.
func BackendAgentToken() string {
	return env("SIGARDAPANEL_AGENT_TOKEN", "")
}

// AgentToken returns the bearer token required by agent task endpoints.
func AgentToken() string {
	return env("SIGARDAPANEL_AGENT_TOKEN", "")
}

// AgentSiteRoot returns the optional allowed root for site file operations.
func AgentSiteRoot() string {
	return env("SIGARDAPANEL_AGENT_SITE_ROOT", "")
}

// AgentAllowStubSSL reports whether SSL tasks may return stub certificates.
func AgentAllowStubSSL() bool {
	return boolEnv("SIGARDAPANEL_AGENT_ALLOW_STUB_SSL", false)
}

// DBPath returns the database file path from environment or default.
func DBPath() string {
	return env("SIGARDAPANEL_DB_PATH", "sigardapanel.db")
}

// warnDerivedEncryptionKeyOnce makes the fallback-key warning fire once per
// process instead of on every encrypt/decrypt call.
var warnDerivedEncryptionKeyOnce sync.Once

// EncryptionKeySource values reported by EncryptionKeySource.
const (
	KeySourceEnv      = "env"
	KeySourceFile     = "file"
	KeySourceFallback = "fallback"
)

var (
	keyFileMu    sync.Mutex
	keyFileCache = map[string][]byte{}
)

// EncryptionKeyFile is where a generated encryption key is persisted:
// SIGARDAPANEL_ENCRYPTION_KEY_FILE, or "encryption.key" next to the database.
func EncryptionKeyFile() string {
	if p := env("SIGARDAPANEL_ENCRYPTION_KEY_FILE", ""); p != "" {
		return p
	}
	return filepath.Join(filepath.Dir(DBPath()), "encryption.key")
}

func envEncryptionKey() ([]byte, bool) {
	keyHex := env("SIGARDAPANEL_ENCRYPTION_KEY", "")
	if keyHex == "" {
		return nil, false
	}
	key, err := hex.DecodeString(strings.TrimSpace(keyHex))
	if err != nil || len(key) != 32 {
		return nil, false
	}
	return key, true
}

// fileEncryptionKey loads (and caches) the 32-byte hex key from the key file.
func fileEncryptionKey() ([]byte, bool) {
	path := EncryptionKeyFile()
	keyFileMu.Lock()
	defer keyFileMu.Unlock()
	if k, ok := keyFileCache[path]; ok {
		return k, true
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	key, err := hex.DecodeString(strings.TrimSpace(string(data)))
	if err != nil || len(key) != 32 {
		slog.Error("encryption key file is invalid (want 32 bytes hex); ignoring it", "path", path)
		return nil, false
	}
	keyFileCache[path] = key
	return key, true
}

// derivedEncryptionKey is the legacy fallback: sha256(DB path + ":" + agent
// token). Anyone who knows (or guesses) the DB path and can read the agent
// token can derive it. Kept only so data already encrypted with it still
// decrypts.
func derivedEncryptionKey() []byte {
	material := DBPath() + ":" + AgentToken()
	h := sha256.Sum256([]byte(material))
	return h[:]
}

// EncryptionKeySource reports which key EncryptionKey will return.
func EncryptionKeySource() string {
	if _, ok := envEncryptionKey(); ok {
		return KeySourceEnv
	}
	if _, ok := fileEncryptionKey(); ok {
		return KeySourceFile
	}
	return KeySourceFallback
}

// EncryptionKey returns the AES-256 key for encrypting sensitive data (e.g. DB
// passwords, TOTP secrets). In order of preference:
//  1. SIGARDAPANEL_ENCRYPTION_KEY (32 bytes hex),
//  2. the key file (EncryptionKeyFile), generated on new installs by
//     GenerateEncryptionKeyFile,
//  3. the legacy derived fallback (weak; logged loudly).
func EncryptionKey() []byte {
	if key, ok := envEncryptionKey(); ok {
		return key
	}
	if key, ok := fileEncryptionKey(); ok {
		return key
	}
	// The derived key is deployment-local: secrets encrypted with it become
	// unreadable after a reinstall, and the derivation is weaker than an
	// explicitly generated key. Surface the misconfiguration loudly.
	warnDerivedEncryptionKeyOnce.Do(func() {
		slog.Error("SIGARDAPANEL_ENCRYPTION_KEY missing or invalid: using derived fallback key. Set it to 32 bytes hex (openssl rand -hex 32) before storing secrets")
	})
	return derivedEncryptionKey()
}

// DerivedEncryptionKey exposes the legacy fallback key (for re-encryption
// tooling and tests).
func DerivedEncryptionKey() []byte { return derivedEncryptionKey() }

// GenerateEncryptionKeyFile creates EncryptionKeyFile with a fresh random key
// (mode 0600). It never overwrites an existing file.
func GenerateEncryptionKeyFile() (string, error) {
	path := EncryptionKeyFile()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return path, fmt.Errorf("generate encryption key: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return path, fmt.Errorf("create key dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return path, fmt.Errorf("create key file: %w", err)
	}
	if _, err := f.WriteString(hex.EncodeToString(key) + "\n"); err != nil {
		f.Close()
		os.Remove(path)
		return path, fmt.Errorf("write key file: %w", err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(path)
		return path, fmt.Errorf("sync key file: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(path)
		return path, fmt.Errorf("close key file: %w", err)
	}
	keyFileMu.Lock()
	delete(keyFileCache, path)
	keyFileMu.Unlock()
	return path, nil
}

// MySQLRootPassword returns the MySQL root password used for direct database management.
func MySQLRootPassword() string {
	return env("SIGARDAPANEL_MYSQL_ROOT_PASSWORD", "")
}

// AllowRegistration reports whether public sign-up is open. It is closed by
// default; panels that sell hosting open it with
// SIGARDAPANEL_ALLOW_REGISTRATION=true.
func AllowRegistration() bool {
	return boolEnv("SIGARDAPANEL_ALLOW_REGISTRATION", false)
}

// LandingPage reports whether the web serves the Sigarda SaaS landing and
// marketing pages (/, /pricing). Off by default: a self-hosted panel sends
// visitors straight to /login (or /dashboard when signed in). Enable with
// SIGARDAPANEL_LANDING_PAGE=true.
func LandingPage() bool {
	return boolEnv("SIGARDAPANEL_LANDING_PAGE", false)
}

// AllowMemberSignup reports whether POST /api/v1/member/register accepts new
// member (SaaS customer) accounts. Off by default; the member portal also
// needs its database to be configured. Enable with
// SIGARDAPANEL_ALLOW_MEMBER_SIGNUP=true.
func AllowMemberSignup() bool {
	return boolEnv("SIGARDAPANEL_ALLOW_MEMBER_SIGNUP", false)
}

// AllowPrivateAgents lets self-registering agents (one-line installer) use a
// private (RFC 1918 / ULA) callback address, for panels that reach their
// servers over a VPC or VPN. Off by default.
func AllowPrivateAgents() bool {
	return boolEnv("SIGARDAPANEL_ALLOW_PRIVATE_AGENTS", false)
}

func env(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func durationEnv(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	seconds, err := strconv.Atoi(value)
	if err != nil || seconds <= 0 {
		return fallback
	}
	return time.Duration(seconds) * time.Second
}

func boolEnv(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

// CheckPortWarn checks if a port is available. If not, logs a warning.
// Returns the original addr if available, or an empty string if port is occupied.
func CheckPortWarn(name, addr string) string {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	ln, err := net.Listen("tcp", ":"+port)
	if err != nil {
		slog.Error(fmt.Sprintf("%s: port %s already in use", name, port),
			"addr", addr,
			"fix", fmt.Sprintf("set %s_ADDR env to a different port", name),
		)
		return ""
	}
	ln.Close()
	return addr
}

// FindFreePort finds a free port starting from the given port.
func FindFreePort(startPort int) int {
	for port := startPort; port < startPort+100; port++ {
		ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err == nil {
			ln.Close()
			return port
		}
	}
	return startPort
}
