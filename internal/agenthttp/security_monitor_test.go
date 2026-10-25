package agenthttp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseSSHLogsCounts(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "auth.log")
	content := `Oct 20 01:00:01 host sshd[1]: Failed password for root from 203.0.113.9 port 40000 ssh2
Oct 20 01:00:05 host sshd[2]: Failed password for invalid user admin from 203.0.113.9 port 40001 ssh2
Oct 20 01:00:09 host sshd[3]: Invalid user oracle from 198.51.100.7 port 40002
Oct 20 01:00:12 host sshd[4]: Accepted publickey for deploy from 192.0.2.10 port 40003 ssh2
Oct 20 01:00:20 host sshd[5]: Failed password for root from 198.51.100.7 port 40004 ssh2
`
	if err := os.WriteFile(log, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	act := parseSSHLogs([]string{log})
	if act.FailedTotal != 3 {
		t.Errorf("FailedTotal = %d, want 3", act.FailedTotal)
	}
	if act.InvalidTotal != 1 {
		t.Errorf("InvalidTotal = %d, want 1", act.InvalidTotal)
	}
	if act.AcceptedTotal != 1 {
		t.Errorf("AcceptedTotal = %d, want 1", act.AcceptedTotal)
	}
	if act.UniqueIPs != 2 {
		t.Errorf("UniqueIPs = %d, want 2", act.UniqueIPs)
	}
	if len(act.RecentLogins) != 1 || act.RecentLogins[0].User != "deploy" || act.RecentLogins[0].IP != "192.0.2.10" {
		t.Errorf("RecentLogins = %+v", act.RecentLogins)
	}
	byIP := map[string]int{}
	for _, a := range act.TopAttackers {
		byIP[a.IP] = a.Count
	}
	if byIP["203.0.113.9"] != 2 || byIP["198.51.100.7"] != 2 {
		t.Errorf("top attackers = %+v", act.TopAttackers)
	}
}

func TestParseSSHLogsMissingFiles(t *testing.T) {
	act := parseSSHLogs([]string{"/nonexistent/auth.log"})
	if act.FailedTotal != 0 || len(act.TopAttackers) != 0 {
		t.Errorf("expected empty activity, got %+v", act)
	}
}

func TestHexToIPLittleEndian(t *testing.T) {
	// /proc/net/tcp stores IPv4 little-endian per 4-byte group:
	// 0A0200C0 -> C0.00.02.0A -> 192.0.2.10.
	ip := hexToIPLittle(t, "0A0200C0")
	if ip != "192.0.2.10" {
		t.Fatalf("hexToIP = %s, want 192.0.2.10", ip)
	}
}

func hexToIPLittle(t *testing.T, h string) string {
	t.Helper()
	ip, err := hexToIP(h)
	if err != nil {
		t.Fatalf("hexToIP(%s): %v", h, err)
	}
	return ip.String()
}

func TestParseProcNetTCPEstablishedOnly(t *testing.T) {
	endpoints := map[string]*connEndpoint{}
	dir := t.TempDir()
	p := filepath.Join(dir, "tcp")
	// 01 = ESTABLISHED to 1.2.3.4:4444, 0A = LISTEN on 0.0.0.0:22
	content := "  sl local rem st\n" +
		"   0: 0100007F:0016 00000000:0000 0A\n" +
		"   1: 0100007F:9C40 04030201:115C 01\n" +
		"   2: 0100007F:9C41 04030201:115C 01\n"
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	parseProcNetTCP(p, endpoints)

	ep, ok := endpoints["1.2.3.4:4444"]
	if !ok {
		t.Fatalf("expected 1.2.3.4:4444, got %+v", endpoints)
	}
	if ep.Count != 2 {
		t.Errorf("count = %d, want 2", ep.Count)
	}
	if !ep.Suspicious || ep.Reason == "" {
		t.Errorf("port 4444 must be flagged suspicious, got %+v", ep)
	}
	if len(endpoints) != 1 {
		t.Errorf("LISTEN socket must be skipped, got %+v", endpoints)
	}
}

func TestWebAttackPatternsCompile(t *testing.T) {
	if !webAttackPatterns[0].pattern.MatchString("GET /?id=1 sqlmap/1.7 HTTP/1.1") {
		t.Error("sqlmap pattern should match")
	}
	if webAttackPatterns[0].pattern.MatchString("GET / HTTP/1.1") {
		t.Error("sqlmap pattern should not match clean traffic")
	}
}
