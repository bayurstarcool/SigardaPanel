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
		if isAllowedAgentCallbackIP(net.ParseIP(raw)) {
			t.Errorf("sensitive callback IP accepted: %s", raw)
		}
	}
}

func TestAllowedAgentCallbackIPAcceptsGlobalUnicast(t *testing.T) {
	for _, raw := range []string{"8.8.8.8", "1.1.1.1", "2606:4700:4700::1111"} {
		if !isAllowedAgentCallbackIP(net.ParseIP(raw)) {
			t.Errorf("global callback IP rejected: %s", raw)
		}
	}
}
