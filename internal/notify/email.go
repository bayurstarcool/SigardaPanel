package notify

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"os"
	"strings"
	"time"
)

const smtpTimeout = 15 * time.Second

// EmailNotifier delivers alerts over SMTP.
//
// Channel config (JSON): to_address (required, comma-separated allowed) plus
// optional smtp_host, smtp_port, smtp_username, smtp_password, from_address
// and smtp_tls ("starttls" [default], "ssl", "none"). Anything omitted falls
// back to the SIGARDAPANEL_SMTP_{HOST,PORT,USER,PASSWORD,FROM,TLS} environment.
type EmailNotifier struct{}

type emailConfig struct {
	ToAddress string `json:"to_address"`
	Host      string `json:"smtp_host"`
	Port      string `json:"smtp_port"`
	Username  string `json:"smtp_username"`
	Password  string `json:"smtp_password"`
	From      string `json:"from_address"`
	TLS       string `json:"smtp_tls"`
}

func (c *emailConfig) applyEnvDefaults() {
	def := func(v *string, key string) {
		if *v == "" {
			*v = os.Getenv(key)
		}
	}
	def(&c.Host, "SIGARDAPANEL_SMTP_HOST")
	def(&c.Port, "SIGARDAPANEL_SMTP_PORT")
	def(&c.Username, "SIGARDAPANEL_SMTP_USER")
	def(&c.Password, "SIGARDAPANEL_SMTP_PASSWORD")
	def(&c.From, "SIGARDAPANEL_SMTP_FROM")
	def(&c.TLS, "SIGARDAPANEL_SMTP_TLS")
	c.TLS = strings.ToLower(strings.TrimSpace(c.TLS))
	if c.TLS == "" {
		c.TLS = "starttls"
	}
	if c.Port == "" {
		if c.TLS == "ssl" {
			c.Port = "465"
		} else {
			c.Port = "587"
		}
	}
	if c.From == "" {
		c.From = c.Username
	}
}

// Send delivers the alert to every address in to_address.
func (e *EmailNotifier) Send(ctx context.Context, channel *Channel, alert *Alert) error {
	var cfg emailConfig
	if err := json.Unmarshal(channel.Config, &cfg); err != nil {
		return fmt.Errorf("invalid email config: %w", err)
	}
	if strings.TrimSpace(cfg.ToAddress) == "" {
		return fmt.Errorf("email requires to_address")
	}
	cfg.applyEnvDefaults()
	if cfg.Host == "" {
		return fmt.Errorf("email: smtp_host not configured (set it on the channel or SIGARDAPANEL_SMTP_HOST)")
	}
	if cfg.TLS != "starttls" && cfg.TLS != "ssl" && cfg.TLS != "none" {
		return fmt.Errorf("email: smtp_tls must be starttls, ssl or none")
	}

	to, err := mail.ParseAddressList(cfg.ToAddress)
	if err != nil {
		return fmt.Errorf("email: invalid to_address: %w", err)
	}
	from, err := mail.ParseAddress(cfg.From)
	if err != nil {
		return fmt.Errorf("email: invalid from_address: %w", err)
	}
	rcpts := make([]string, len(to))
	for i, a := range to {
		rcpts[i] = a.Address
	}

	msg := buildEmail(from, to, alert)
	return sendSMTP(ctx, &cfg, from.Address, rcpts, msg)
}

// headerSafe removes CR/LF so user-influenced text can never inject headers.
func headerSafe(s string) string {
	return strings.NewReplacer("\r", " ", "\n", " ").Replace(s)
}

func buildEmail(from *mail.Address, to []*mail.Address, alert *Alert) []byte {
	toList := make([]string, len(to))
	for i, a := range to {
		toList[i] = a.String()
	}
	subject := headerSafe(fmt.Sprintf("[SigardaPanel][%s] %s", alert.Severity, alert.Subject))
	body := strings.ReplaceAll(strings.ReplaceAll(alert.Message, "\r\n", "\n"), "\n", "\r\n")

	var b strings.Builder
	b.WriteString("From: " + from.String() + "\r\n")
	b.WriteString("To: " + strings.Join(toList, ", ") + "\r\n")
	b.WriteString("Subject: " + mimeQ(subject) + "\r\n")
	b.WriteString("Date: " + time.Now().Format(time.RFC1123Z) + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n")
	b.WriteString("\r\n")
	b.WriteString(body + "\r\n")
	return []byte(b.String())
}

func mimeQ(s string) string {
	for _, r := range s {
		if r > 127 {
			return mimeEncodeWord(s)
		}
	}
	return s
}

func sendSMTP(ctx context.Context, cfg *emailConfig, from string, rcpts []string, msg []byte) error {
	addr := net.JoinHostPort(cfg.Host, cfg.Port)
	dialer := &net.Dialer{Timeout: smtpTimeout}
	tlsCfg := &tls.Config{ServerName: cfg.Host, MinVersion: tls.VersionTLS12}

	var conn net.Conn
	var err error
	if cfg.TLS == "ssl" {
		conn, err = tls.DialWithDialer(dialer, "tcp", addr, tlsCfg)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("email: connect %s: %w", addr, err)
	}
	deadline := time.Now().Add(2 * smtpTimeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = conn.SetDeadline(deadline)

	c, err := smtp.NewClient(conn, cfg.Host)
	if err != nil {
		conn.Close()
		return fmt.Errorf("email: smtp handshake: %w", err)
	}
	defer c.Close()

	if cfg.TLS == "starttls" {
		if ok, _ := c.Extension("STARTTLS"); !ok {
			return fmt.Errorf("email: server does not offer STARTTLS (use smtp_tls=none only on trusted networks)")
		}
		if err := c.StartTLS(tlsCfg); err != nil {
			return fmt.Errorf("email: starttls: %w", err)
		}
	}
	if cfg.Username != "" {
		if cfg.TLS == "none" {
			return fmt.Errorf("email: refusing to send SMTP credentials without TLS")
		}
		if err := c.Auth(smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)); err != nil {
			return fmt.Errorf("email: auth: %w", err)
		}
	}
	if err := c.Mail(from); err != nil {
		return fmt.Errorf("email: MAIL FROM: %w", err)
	}
	for _, r := range rcpts {
		if err := c.Rcpt(r); err != nil {
			return fmt.Errorf("email: RCPT TO %s: %w", r, err)
		}
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("email: DATA: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("email: write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("email: finish: %w", err)
	}
	return c.Quit()
}

func mimeEncodeWord(s string) string {
	return mime.QEncoding.Encode("UTF-8", s)
}
