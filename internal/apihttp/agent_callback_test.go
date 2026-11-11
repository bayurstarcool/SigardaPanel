package apihttp

import (
	"net"
	"testing"
)

func TestAllowedAgentCallbackIPRejectsSensitiveRanges(t *testing.T) {
	for _, raw := range []string{
		"127.0.0.1",
		"10.0.0.1",
		"172.16.0.1",
		"192.168.1.1",
		"169.254.169.254",
		"224.0.0.1",
		"::1",
		"fe80::1",
		"fc00::1",
	} {
		if isAllowedAgentCallbackIP(net.ParseIP(raw), false) {
			t.Errorf("sensitive callback IP accepted: %s", raw)
		}
	}
}

func TestAllowedAgentCallbackIPAcceptsGlobalUnicast(t *testing.T) {
	for _, raw := range []string{"8.8.8.8", "1.1.1.1", "2606:4700:4700::1111"} {
		if !isAllowedAgentCallbackIP(net.ParseIP(raw), false) {
			t.Errorf("global callback IP rejected: %s", raw)
		}
	}
}

// With SIGARDAPANEL_ALLOW_PRIVATE_AGENTS only RFC 1918 / ULA addresses are
// added; loopback, link-local (metadata) and multicast stay refused.
func TestAllowedAgentCallbackIPPrivateOptIn(t *testing.T) {
	for _, raw := range []string{"10.0.0.1", "172.16.0.1", "192.168.1.1", "fd00::1", "8.8.8.8"} {
		if !isAllowedAgentCallbackIP(net.ParseIP(raw), true) {
			t.Errorf("opt-in: private/global IP rejected: %s", raw)
		}
	}
	for _, raw := range []string{"127.0.0.1", "169.254.169.254", "224.0.0.1", "0.0.0.0", "::1", "fe80::1"} {
		if isAllowedAgentCallbackIP(net.ParseIP(raw), true) {
			t.Errorf("opt-in: sensitive IP accepted: %s", raw)
		}
	}
}
