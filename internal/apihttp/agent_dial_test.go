package apihttp

import (
	"net"
	"testing"
)

func TestAgentIPAllowed(t *testing.T) {
	cases := []struct {
		ip      string
		literal bool
		ok      bool
	}{
		{"127.0.0.1", true, true},        // the panel's own agent, written literally
		{"127.0.0.1", false, false},      // a name that resolves to the panel host
		{"169.254.169.254", true, false}, // cloud metadata
		{"169.254.169.254", false, false},
		{"0.0.0.0", true, false},
		{"203.0.113.10", false, true},
		{"10.0.0.5", false, false}, // private, not allowed by default
	}
	for _, c := range cases {
		if err := agentIPAllowed(net.ParseIP(c.ip), c.literal); (err == nil) != c.ok {
			t.Errorf("%s literal=%v: err=%v", c.ip, c.literal, err)
		}
	}
	if _, err := normalizeAgentURL("http://does-not-resolve.invalid:7710", false); err == nil {
		t.Error("an unresolvable agent host was accepted")
	}
}
