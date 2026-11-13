package nginx

import (
	"strings"
	"testing"

	"sigardapanel/internal/store"
)

func TestNormalizeBlockedIPs(t *testing.T) {
	got, err := NormalizeBlockedIPs([]string{" 1.2.3.4 ", "10.0.0.0/8", "2001:db8::1", "1.2.3.4", ""})
	if err != nil || strings.Join(got, ",") != "1.2.3.4,10.0.0.0/8,2001:db8::1" {
		t.Fatalf("NormalizeBlockedIPs = %v, %v", got, err)
	}
	for _, bad := range []string{
		"1.2.3.4; } location /leak/ { alias /etc/; autoindex on; } location /zz/ { deny 5.6.7.8",
		"all", "1.2.3.4 5.6.7.8", "10.0.0.0/33", "example.com",
	} {
		if _, err := NormalizeBlockedIPs([]string{bad}); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
}

func TestGenerateVhostRejectsInjectedSecurity(t *testing.T) {
	site := &store.Site{Domain: "ex.example.com", Runtime: "static", RootPath: "/home/u/htdocs/ex.example.com"}
	evil := &store.SiteSecurity{BlockedIPs: []string{"1.2.3.4; } location /leak/ { alias /etc/; } location /z/ { deny all"}}
	if out, err := GenerateVhost(site, 0, evil); err == nil {
		t.Fatalf("injected blocked IP rendered:\n%s", out)
	}
	badUser := &store.SiteSecurity{BasicAuthEnabled: true, BasicAuthUsername: "a b", BasicAuthPassword: "$2a$10$" + strings.Repeat("a", 53)}
	if _, err := GenerateVhost(site, 0, badUser); err == nil {
		t.Fatal("invalid basic auth username accepted")
	}
	noHash := &store.SiteSecurity{BasicAuthEnabled: true, BasicAuthUsername: "admin"}
	if _, err := GenerateVhost(site, 0, noHash); err == nil {
		t.Fatal("basic auth without a password hash accepted (it would lock everyone out)")
	}
	ok := &store.SiteSecurity{BlockedIPs: []string{"1.2.3.4", "10.0.0.0/8"}}
	out, err := GenerateVhost(site, 0, ok)
	if err != nil || !strings.Contains(out, "deny 1.2.3.4;") || !strings.Contains(out, "deny 10.0.0.0/8;") {
		t.Fatalf("valid blocked IPs: %v\n%s", err, out)
	}
}

func TestGenerateVhostCertAndBackupsDenied(t *testing.T) {
	for _, rt := range []string{"static", "php", "node", "proxy"} {
		site := &store.Site{Domain: "ex.example.com", Runtime: rt, RootPath: "/home/u/htdocs/ex.example.com"}
		out, err := GenerateVhost(site, 3000, nil)
		if err != nil {
			t.Fatalf("%s: %v", rt, err)
		}
		if !strings.Contains(out, "ssl_certificate "+DefaultSSLCertificate+";") {
			t.Errorf("%s: placeholder certificate missing", rt)
		}
		if !strings.Contains(out, "location ^~ /backups/ { deny all; }") {
			t.Errorf("%s: backups directory is not denied", rt)
		}
		le := "/etc/letsencrypt/live/ex.example.com/"
		out, err = GenerateVhostWithCert(site, 3000, nil, le+"fullchain.pem", le+"privkey.pem")
		if err != nil || !strings.Contains(out, "ssl_certificate "+le+"fullchain.pem;") || !strings.Contains(out, "ssl_certificate_key "+le+"privkey.pem;") {
			t.Errorf("%s: Let's Encrypt certificate not used: %v", rt, err)
		}
	}
	site := &store.Site{Domain: "ex.example.com", Runtime: "static", RootPath: "/home/u/htdocs/ex.example.com"}
	if _, err := GenerateVhostWithCert(site, 0, nil, "/x;\nfoo.pem", "/y.pem"); err == nil {
		t.Fatal("unsafe certificate path accepted")
	}
}

func TestGenerateVhostAllowlist(t *testing.T) {
	for _, rt := range []string{"static", "php", "node", "proxy"} {
		site := &store.Site{Domain: "ex.example.com", Runtime: rt, RootPath: "/home/u/htdocs/ex.example.com"}
		sec := &store.SiteSecurity{BlockedIPs: []string{"10.0.0.5"}, AllowedIPs: []string{"10.0.0.0/8", "2001:db8::/32"}}
		out, err := GenerateVhost(site, 3000, sec)
		if err != nil {
			t.Fatal(err)
		}
		// nginx applies the first matching rule: the blocked address before
		// the allowed range, and "deny all" last.
		d, a, all := strings.Index(out, "deny 10.0.0.5;"), strings.Index(out, "allow 10.0.0.0/8;"), strings.Index(out, "deny all;\n        ")
		if d < 0 || a < 0 || all < 0 || !(d < a && a < all) || !strings.Contains(out, "allow 2001:db8::/32;") {
			t.Fatalf("%s: allowlist not rendered in order:\n%s", rt, out)
		}
		if _, err := GenerateVhost(site, 3000, &store.SiteSecurity{AllowedIPs: []string{"1.2.3.4; deny all"}}); err == nil {
			t.Fatalf("%s: injected allowed IP accepted", rt)
		}
		// Without allow rules nobody is denied wholesale.
		plain, _ := GenerateVhost(site, 3000, &store.SiteSecurity{BlockedIPs: []string{"1.2.3.4"}})
		if strings.Contains(plain, "allow ") || strings.Count(plain, "deny all;") != strings.Count(out, "deny all;")-1 {
			t.Fatalf("%s: rules without an allowlist:\n%s", rt, plain)
		}
	}
}
