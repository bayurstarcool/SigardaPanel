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
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/labstack/echo/v5"

	"sigardapanel/internal/agenttls"
)

// Remote agents serve HTTPS with a self-signed certificate the panel pins by
// fingerprint (internal/agenttls). New installs get the certificate from the
// install script; agents installed before that are switched by the panel:
// POST /tasks/agent/tls/enable (token-authenticated, over the existing
// connection) creates the certificate here, records it in agent.env and
// restarts the agent with TLS; the panel stores the returned fingerprint and
// the https URL before the restart.

const (
	defaultAgentTLSDir = "/etc/sigardapanel/agent-tls"
	agentServiceName   = "sigardapanel-agent"
)

type agentTLSManager struct {
	dir          string
	agentEnvFile string
	panelEnvFile string
	// active reports the certificate the running process serves ("" = http).
	active  func() string
	restart func()
}

func newAgentTLSManager(opts *RouterOptions) *agentTLSManager {
	m := &agentTLSManager{
		dir:          defaultAgentTLSDir,
		agentEnvFile: defaultAgentEnvFile,
		panelEnvFile: defaultPanelEnvFile,
		active:       func() string { return os.Getenv("SIGARDAPANEL_AGENT_TLS_CERT") },
		restart: func() {
			// After the response is written: the unit restarts with the new
			// environment (systemd kills this process).
			time.Sleep(2 * time.Second)
			if out, err := exec.Command("systemctl", "restart", agentServiceName).CombinedOutput(); err != nil {
				slog.Error("agent: restart for TLS failed", "error", err, "output", strings.TrimSpace(string(out)))
			}
		},
	}
	if opts != nil {
		// Injected routers (tests) never touch the real host.
		m.dir = filepath.Join(os.TempDir(), "sigardapanel-agent-test-tls-not-configured")
		m.agentEnvFile = filepath.Join(os.TempDir(), "sigardapanel-agent-test-tls-agent.env-not-configured")
		m.panelEnvFile = filepath.Join(os.TempDir(), "sigardapanel-agent-test-tls-panel.env-not-configured")
		m.restart = func() {}
		if opts.AgentTLSDir != "" {
			m.dir = opts.AgentTLSDir
		}
		if opts.AgentTLSEnvFile != "" {
			m.agentEnvFile = opts.AgentTLSEnvFile
		}
		if opts.AgentTLSActive != nil {
			m.active = opts.AgentTLSActive
		}
		if opts.AgentTLSRestart != nil {
			m.restart = opts.AgentTLSRestart
		}
	}
	return m
}

func (m *agentTLSManager) certPath() string { return filepath.Join(m.dir, "agent.crt") }
func (m *agentTLSManager) keyPath() string  { return filepath.Join(m.dir, "agent.key") }

func (m *agentTLSManager) fingerprint() string {
	raw, err := os.ReadFile(m.certPath())
	if err != nil {
		return ""
	}
	block, _ := pem.Decode(raw)
	if block == nil || block.Type != "CERTIFICATE" {
		return ""
	}
	return agenttls.Fingerprint(block.Bytes)
}

// agentTLSStatusTask answers GET /tasks/agent/tls.
func (r *Router) agentTLSStatusTask(c *echo.Context) error {
	m := r.agentTLS
	return c.JSON(http.StatusOK, map[string]any{
		"supported":   true,
		"enabled":     m.active() != "",
		"fingerprint": m.fingerprint(),
	})
}

// agentTLSEnableTask answers POST /tasks/agent/tls/enable {"ip": "..."}.
func (r *Router) agentTLSEnableTask(c *echo.Context) error {
	m := r.agentTLS
	if _, err := os.Stat(m.panelEnvFile); err == nil {
		return echo.NewHTTPError(http.StatusConflict, "this is the panel host: its agent stays on loopback http")
	}
	if _, err := os.Stat(m.agentEnvFile); err != nil {
		return echo.NewHTTPError(http.StatusConflict, "agent.env not found: this agent was not installed by the agent installer")
	}
	var body struct {
		IP string `json:"ip"`
	}
	_ = c.Bind(&body)
	if fp := m.fingerprint(); fp == "" {
		if err := m.generate(net.ParseIP(strings.TrimSpace(body.IP))); err != nil {
			slog.Error("agent: TLS certificate generation failed", "error", err)
			return echo.NewHTTPError(http.StatusInternalServerError, "certificate generation failed")
		}
	}
	fp := m.fingerprint()
	if fp == "" {
		return echo.NewHTTPError(http.StatusInternalServerError, "certificate unreadable")
	}
	if err := m.writeEnv(); err != nil {
		slog.Error("agent: TLS env update failed", "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "agent.env update failed")
	}
	restart := m.active() == ""
	if restart {
		go m.restart()
	}
	slog.Info("agent: TLS enabled", "fingerprint", fp, "restart", restart)
	return c.JSON(http.StatusOK, map[string]any{"ok": true, "fingerprint": fp, "restarting": restart})
}

// generate writes a self-signed ECDSA P-256 certificate (10 years; the pin,
// not the validity period, is the trust).
func (m *agentTLSManager) generate(ip net.IP) error {
	if err := os.MkdirAll(m.dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(m.dir, 0o700); err != nil {
		return err
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
	if err != nil {
		return err
	}
	host, _ := os.Hostname()
	if host == "" {
		host = "sigardapanel-agent"
	}
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: host},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{host},
	}
	if ip != nil {
		tmpl.IPAddresses = []net.IP{ip}
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return err
	}
	if err := writeFileAtomic(m.keyPath(), pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		return err
	}
	return writeFileAtomic(m.certPath(), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644)
}

// writeEnv sets SIGARDAPANEL_AGENT_TLS_CERT/KEY in agent.env (other lines kept).
func (m *agentTLSManager) writeEnv() error {
	raw, err := os.ReadFile(m.agentEnvFile)
	if err != nil {
		return err
	}
	var out []string
	for _, line := range strings.Split(strings.TrimRight(string(raw), "\n"), "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "SIGARDAPANEL_AGENT_TLS_CERT=") || strings.HasPrefix(t, "SIGARDAPANEL_AGENT_TLS_KEY=") {
			continue
		}
		out = append(out, line)
	}
	out = append(out, "SIGARDAPANEL_AGENT_TLS_CERT="+m.certPath(), "SIGARDAPANEL_AGENT_TLS_KEY="+m.keyPath())
	return writeFileAtomic(m.agentEnvFile, []byte(strings.Join(out, "\n")+"\n"), 0o600)
}

func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err := f.Chmod(mode); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("install %s: %w", path, err)
	}
	return nil
}
