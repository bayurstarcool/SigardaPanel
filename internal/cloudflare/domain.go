package cloudflare

import (
	"strings"
)

// RootDomain extracts the root domain (registrable domain) from a full domain.
// e.g. "sub.example.com" -> "example.com"
// e.g. "example.co.uk" -> "example.co.uk"
// This is a simple implementation; for production use a PSL library.
func RootDomain(domain string) string {
	domain = strings.TrimSpace(strings.ToLower(domain))
	parts := strings.Split(domain, ".")
	n := len(parts)
	if n < 2 {
		return domain
	}

	// Common multi-part TLDs
	multiTLD := map[string]bool{
		"co.uk": true, "org.uk": true, "ac.uk": true, "gov.uk": true,
		"net.uk": true, "me.uk": true, "ltd.uk": true, "plc.uk": true,
		"com.au": true, "net.au": true, "org.au": true, "edu.au": true,
		"gov.au": true, "co.nz": true, "net.nz": true, "org.nz": true,
		"co.jp": true, "ne.jp": true, "or.jp": true,
		"co.kr": true, "or.kr": true, "ne.kr": true,
		"com.br": true, "org.br": true, "net.br": true,
		"com.cn": true, "net.cn": true, "org.cn": true,
		"co.in": true, "net.in": true, "org.in": true, "gen.in": true,
		"com.mx": true, "org.mx": true,
		"co.za": true, "org.za": true, "net.za": true, "gov.za": true,
		"co.il": true, "org.il": true, "ac.il": true, "gov.il": true,
		"com.sg": true, "org.sg": true, "edu.sg": true, "gov.sg": true,
		"com.hk": true, "org.hk": true, "edu.hk": true, "gov.hk": true,
		"com.ar": true, "org.ar": true,
		"co.th": true, "or.th": true, "go.th": true,
		"com.tr": true, "org.tr": true, "net.tr": true, "gov.tr": true,
	}

	if n >= 3 {
		lastTwo := parts[n-2] + "." + parts[n-1]
		if multiTLD[lastTwo] {
			if n >= 4 {
				return strings.Join(parts[n-3:], ".")
			}
			return domain
		}
	}

	// Standard: return last 2 parts (e.g., example.com)
	if n >= 2 {
		return strings.Join(parts[n-2:], ".")
	}
	return domain
}

// FindMatchingZone checks if any zone in the list matches the given domain.
// Returns the matching zone, or nil if not found.
func FindMatchingZone(zones []Zone, domain string) *Zone {
	domain = strings.TrimSpace(strings.ToLower(domain))
	for i := range zones {
		if strings.HasSuffix(domain, zones[i].Name) || domain == zones[i].Name {
			return &zones[i]
		}
	}
	return nil
}
