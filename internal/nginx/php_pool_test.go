package nginx

import (
	"database/sql"
	"strings"
	"testing"

	"sigardapanel/internal/store"
)

func TestVhostUsesTheSitePoolSocket(t *testing.T) {
	site := &store.Site{Domain: "shop.example.com", Runtime: "php", RootPath: "/home/alice/htdocs/shop.example.com",
		PHPVersion: sql.NullString{String: "8.3", Valid: true}}
	sock := SitePoolSocket("/run/php", 7, "8.3")
	if sock != "/run/php/sp-site-7-php8.3-fpm.sock" {
		t.Fatalf("socket = %s", sock)
	}
	cfg, err := GenerateVhostWithOptions(site, 0, nil, VhostOptions{PHPFPMSocket: sock})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(cfg, "fastcgi_pass unix:"+sock+";") {
		t.Fatalf("vhost:\n%s", cfg)
	}
	// The Stack page's "version in use" check still finds it.
	if !strings.Contains(cfg, "php8.3-fpm.sock") {
		t.Fatal("version not visible in the socket name")
	}
	for _, bad := range []string{"/run/php/x.sock", "/run/php/sp-site-7-php8.3-fpm.sock; include /etc/passwd", "relative/sp-site-1-php8.3-fpm.sock"} {
		if _, err := GenerateVhostWithOptions(site, 0, nil, VhostOptions{PHPFPMSocket: bad}); err == nil {
			t.Fatalf("socket %q accepted", bad)
		}
	}
}
