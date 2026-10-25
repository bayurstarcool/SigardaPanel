package config

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net"
	"os"
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
		ReadTimeout:  durationEnv("SIGARDAPANEL_API_READ_TIMEOUT", 10*time.Second),
		WriteTimeout: durationEnv("SIGARDAPANEL_API_WRITE_TIMEOUT", 30*time.Second),
	}
}

func AgentServer() Server {
	return Server{
		Addr:         env("SIGARDAPANEL_AGENT_ADDR", ":7710"),
		TLSCert:      env("SIGARDAPANEL_AGENT_TLS_CERT", ""),
		TLSKey:       env("SIGARDAPANEL_AGENT_TLS_KEY", ""),
		ReadTimeout:  durationEnv("SIGARDAPANEL_AGENT_READ_TIMEOUT", 10*time.Second),
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

// EncryptionKey returns the AES-256 key for encrypting sensitive data (e.g. DB passwords).
// Must be 32 bytes hex-encoded, or defaults to a hash of the DB path + agent token.
func EncryptionKey() []byte {
	keyHex := env("SIGARDAPANEL_ENCRYPTION_KEY", "")
	if keyHex != "" {
		if key, err := hex.DecodeString(keyHex); err == nil && len(key) == 32 {
			return key
		}
	}
	// The derived key is deployment-local: secrets encrypted with it become
	// unreadable after a reinstall, and the derivation is weaker than an
	// explicitly generated key. Surface the misconfiguration loudly.
	warnDerivedEncryptionKeyOnce.Do(func() {
		slog.Error("SIGARDAPANEL_ENCRYPTION_KEY missing or invalid: using derived fallback key. Set it to 32 bytes hex (openssl rand -hex 32) before storing secrets")
	})
	// Derive from DB path + agent token
	material := DBPath() + ":" + AgentToken()
	h := sha256.Sum256([]byte(material))
	return h[:]
}

// MySQLRootPassword returns the MySQL root password used for direct database management.
func MySQLRootPassword() string {
	return env("SIGARDAPANEL_MYSQL_ROOT_PASSWORD", "")
}

// AllowRegistration reports whether public sign-up is open. Self-hosted
// instances can close it with SIGARDAPANEL_ALLOW_REGISTRATION=false.
func AllowRegistration() bool {
	return boolEnv("SIGARDAPANEL_ALLOW_REGISTRATION", true)
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
