package agenthttp

import (
	"strings"
	"testing"
)

func TestUFWRuleArgs(t *testing.T) {
	ok := []struct{ action, port, proto, from, comment, want string }{
		{"allow", "8080", "tcp", "", "Web", "allow 8080/tcp comment Web"},
		{"allow", "53", "any", "", "", "allow 53"},
		{"allow", "22", "tcp", "10.0.0.1", "", "allow from 10.0.0.1 to any port 22 proto tcp"},
		{"deny", "any", "", "1.2.3.4", "Blocked via panel", "deny from 1.2.3.4 comment Blocked via panel"},
		{"deny", "", "", "10.0.0.0/8", "", "deny from 10.0.0.0/8"},
		{"allow", "6000:6010", "udp", "", "", "allow 6000:6010/udp"},
	}
	for _, c := range ok {
		got, err := ufwRuleArgs(c.action, c.port, c.proto, c.from, c.comment)
		if err != nil || strings.Join(got, " ") != c.want {
			t.Errorf("ufwRuleArgs(%q,%q,%q,%q) = %q, %v; want %q", c.port, c.proto, c.from, c.comment, strings.Join(got, " "), err, c.want)
		}
	}
	bad := []struct{ port, proto, from, comment string }{
		{"any", "", "", ""}, {"70000", "tcp", "", ""}, {"80; rm", "", "", ""}, {"6000:6010", "", "", ""},
		{"80", "icmp", "", ""}, {"80", "tcp", "not-an-ip", ""}, {"80", "tcp", "", "x\" quote"},
	}
	for _, c := range bad {
		if got, err := ufwRuleArgs("allow", c.port, c.proto, c.from, c.comment); err == nil {
			t.Errorf("accepted %+v → %v", c, got)
		}
	}
}

func TestParseUFWNumbered(t *testing.T) {
	out := `Status: active

     To                         Action      From
     --                         ------      ----
[ 1] 22/tcp                     ALLOW IN    Anywhere                   # SSH
[ 2] 80/tcp                     ALLOW IN    Anywhere
[ 3] Anywhere                   DENY IN     1.2.3.4`
	rules := parseUFWNumbered(out)
	if len(rules) != 3 || rules[0].Port != "22" || rules[0].Comment != "SSH" || rules[2].Action != "DENY" {
		t.Fatalf("rules = %+v", rules)
	}
}
