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
