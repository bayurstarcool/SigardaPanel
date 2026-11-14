package siteinput

import (
	"context"
	"testing"
)

func TestFilterPHPIni(t *testing.T) {
	kept, rejected := FilterPHPIni(`memory_limit = 256M\n auto_prepend_file = /home/x/a.php\n date.timezone = Asia/Jakarta` + "\nerror_reporting = E_ALL & ~E_NOTICE\nmax_execution_time = $uri")
	if kept != `memory_limit = 256M\n date.timezone = Asia/Jakarta\n error_reporting = E_ALL & ~E_NOTICE` {
		t.Fatalf("kept = %q", kept)
	}
	if len(rejected) != 2 {
		t.Fatalf("rejected = %v", rejected)
	}
	if err := PHPIni(kept); err != nil {
		t.Fatalf("kept value invalid for the vhost: %v", err)
	}
}

func TestProxyPassTarget(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		url      string
		internal bool
		ok       bool
	}{
		{"http://169.254.169.254/latest/meta-data/", true, false},
		{"http://[fe80::1]:80", true, false},
		{"http://0.0.0.0:80", true, false},
		{"http://127.0.0.1:3000", false, true},
		{"http://localhost:3000", false, true},
		{"http://127.0.0.1:7710", false, false},
		{"http://localhost:7700", false, false},
		{"http://10.0.0.5:8080", false, false},
		{"http://10.0.0.5:8080", true, true},
		{"http://127.0.0.1:7710", true, true},
		{"http://203.0.113.10:8080", false, true},
	}
	for _, c := range cases {
		err := ProxyPassTarget(ctx, c.url, c.internal)
		if (err == nil) != c.ok {
			t.Errorf("%s internal=%v: err=%v", c.url, c.internal, err)
		}
	}
}

func TestReservedServiceAccounts(t *testing.T) {
	for _, n := range []string{"postgres", "mysql", "redis", "ubuntu", "www-data", "nginx"} {
		if SystemUser(n) == nil {
			t.Errorf("%s accepted", n)
		}
	}
	if SystemUser("alice") != nil {
		t.Fatal("alice refused")
	}
}

func TestRootPathForOwner(t *testing.T) {
	ok := []string{"/home/admin/htdocs/shop.test", "/home/admin/apps/api", "/var/www/shop"}
	for _, p := range ok {
		if err := RootPathForOwner(p, "admin"); err != nil {
			t.Errorf("%s: %v", p, err)
		}
	}
	bad := []string{"/home/budi/htdocs/shop.test", "/home/admin", "/home/admin/", "/home/admin/htdocs", "/home/admin/.ssh/x", "/home/budi"}
	for _, p := range bad {
		if RootPathForOwner(p, "admin") == nil {
			t.Errorf("%s accepted", p)
		}
	}
}

func TestRootPathsOverlap(t *testing.T) {
	cases := map[[2]string]bool{
		{"/home/a/htdocs/x", "/home/a/htdocs/x"}:     true,
		{"/home/a/htdocs/x", "/home/a/htdocs/x/sub"}: true,
		{"/home/a/htdocs/x/", "/home/a/htdocs/x"}:    true,
		{"/home/a/htdocs/x", "/home/a/htdocs/xy"}:    false,
		{"/home/a/htdocs/x", "/home/b/htdocs/x"}:     false,
	}
	for c, want := range cases {
		if got := RootPathsOverlap(c[0], c[1]); got != want {
			t.Errorf("%v = %v, want %v", c, got, want)
		}
	}
}
