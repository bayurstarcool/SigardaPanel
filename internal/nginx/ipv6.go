package nginx

import (
	"os"
	"regexp"
	"sync"
)

// Some servers run with IPv6 disabled in the kernel (ipv6.disable=1, common
// on VPS images). nginx then refuses to load any "listen [::]:…" directive
// ("Address family not supported by protocol"), which used to break every
// generated vhost and with it site creation, suspension and SSL.

var (
	ipv6Once      sync.Once
	ipv6Supported bool
	// IPv6Probe reports whether the kernel supports IPv6; tests replace it.
	IPv6Probe = func() bool {
		_, err := os.Stat("/proc/net/if_inet6")
		return err == nil
	}
	ipv6ListenRe = regexp.MustCompile(`(?m)^[ \t]*listen[ \t]+\[::\][^;\n]*;[ \t]*\n`)
)

var ipv6Override *bool

// HostIPv6 reports whether this server supports IPv6 (probed once).
func HostIPv6() bool {
	if ipv6Override != nil {
		return *ipv6Override
	}
	ipv6Once.Do(func() { ipv6Supported = IPv6Probe() })
	return ipv6Supported
}

// OverrideHostIPv6 fixes HostIPv6 for tests; call the returned func to undo.
func OverrideHostIPv6(v bool) func() {
	prev := ipv6Override
	ipv6Override = &v
	return func() { ipv6Override = prev }
}

// ForHost returns config unchanged on servers with IPv6, and without its
// "listen [::]…;" lines on servers without it.
func ForHost(config string) string {
	if HostIPv6() {
		return config
	}
	return StripIPv6Listen(config)
}

// StripIPv6Listen removes every "listen [::]…;" line.
func StripIPv6Listen(config string) string {
	return ipv6ListenRe.ReplaceAllString(config, "")
}
