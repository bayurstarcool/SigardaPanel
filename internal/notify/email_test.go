package notify

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"testing"
)

// fakeSMTP accepts one plain-text SMTP session and returns the DATA payload.
func fakeSMTP(t *testing.T) (host, port string, got chan string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	got = make(chan string, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		r := bufio.NewReader(conn)
		fmt.Fprint(conn, "220 fake ESMTP\r\n")
		var data strings.Builder
		inData := false
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			if inData {
				if line == ".\r\n" {
					inData = false
					got <- data.String()
					fmt.Fprint(conn, "250 queued\r\n")
					continue
				}
				data.WriteString(line)
				continue
			}
			switch cmd := strings.ToUpper(strings.TrimSpace(line)); {
			case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
				fmt.Fprint(conn, "250 fake\r\n")
			case strings.HasPrefix(cmd, "MAIL"), strings.HasPrefix(cmd, "RCPT"):
				fmt.Fprint(conn, "250 ok\r\n")
			case cmd == "DATA":
				inData = true
				fmt.Fprint(conn, "354 go\r\n")
			case cmd == "QUIT":
				fmt.Fprint(conn, "221 bye\r\n")
				return
			default:
				fmt.Fprint(conn, "250 ok\r\n")
			}
		}
	}()
	h, p, _ := net.SplitHostPort(ln.Addr().String())
	return h, p, got
}

func emailChannel(t *testing.T, cfg map[string]string) *Channel {
	b, _ := json.Marshal(cfg)
	return &Channel{Type: "email", Config: b}
}

func TestEmailNotifier_Delivers(t *testing.T) {
	host, port, got := fakeSMTP(t)
	ch := emailChannel(t, map[string]string{
		"to_address": "ops@example.com", "smtp_host": host, "smtp_port": port,
		"smtp_tls": "none", "from_address": "panel@example.com",
	})
	err := (&EmailNotifier{}).Send(context.Background(), ch,
		&Alert{Severity: "critical", Subject: "Disk full\r\nBcc: evil@example.com", Message: "line1\nline2"})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	msg := <-got
	if strings.Contains(msg, "\r\nBcc:") {
		t.Fatalf("header injection succeeded:\n%s", msg)
	}
	for _, want := range []string{"To: <ops@example.com>", "Subject: [SigardaPanel][critical] Disk full", "line1\r\nline2"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q:\n%s", want, msg)
		}
	}
}

func TestEmailNotifier_Errors(t *testing.T) {
	cases := map[string]map[string]string{
		"no recipient": {"smtp_host": "127.0.0.1"},
		"no host":      {"to_address": "a@example.com", "from_address": "b@example.com"},
		"bad tls mode": {"to_address": "a@example.com", "smtp_host": "127.0.0.1", "from_address": "b@example.com", "smtp_tls": "weird"},
		"bad address":  {"to_address": "not-an-address", "smtp_host": "127.0.0.1", "from_address": "b@example.com"},
		"creds w/o tls": {"to_address": "a@example.com", "smtp_host": "127.0.0.1", "smtp_port": "1",
			"from_address": "b@example.com", "smtp_tls": "none", "smtp_username": "u"},
	}
	for name, cfg := range cases {
		if err := (&EmailNotifier{}).Send(context.Background(), emailChannel(t, cfg), &Alert{}); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}
