package agenthttp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type poolFixture struct {
	opts    RouterOptions
	etc     string
	run     string
	home    string
	fpmLog  string
	nginx   string
	siteDir string
}

// newPoolFixture: PHP 8.3 with a pool directory, a fake php-fpm8.3 that
// records its calls (and fails when FPM_FAIL exists), and a site owner whose
// home is a temp dir (owned by the test's own uid).
func newPoolFixture(t *testing.T) *poolFixture {
	t.Helper()
	bin := setupTestBinaries(t)
	f := &poolFixture{etc: t.TempDir(), run: t.TempDir(), home: t.TempDir(), nginx: t.TempDir()}
	f.siteDir = filepath.Join(f.home, "htdocs", "shop.example.com")
	if err := os.MkdirAll(filepath.Join(f.etc, "8.3", "fpm", "pool.d"), 0o755); err != nil {
		t.Fatal(err)
	}
	f.fpmLog = filepath.Join(t.TempDir(), "fpm.log")
	fail := filepath.Join(filepath.Dir(f.fpmLog), "FPM_FAIL")
	script := "#!/bin/sh\necho \"$@\" >> " + f.fpmLog + "\nif [ -e " + fail + " ]; then echo 'ERROR: bad pool'; exit 1; fi\necho ok\n"
	if err := os.WriteFile(filepath.Join(bin, "php-fpm8.3"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	uid, gid := uint32(os.Getuid()), uint32(os.Getgid())
	home := f.home
	f.opts = RouterOptions{
		NginxConfDir: f.nginx, PHPRunDir: f.run, PHPEtcDir: f.etc, NginxUser: "www-data",
		UseraddCommand: "true", // no real account is created
		PoolUserLookup: func(name string) (uint32, uint32, string, bool) {
			if name != "alice" {
				return 0, 0, "", false
			}
			return uid, gid, home, true
		},
	}
	return f
}

func (f *poolFixture) create(t *testing.T, extra map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	p := map[string]any{"site_id": 7, "domain": "shop.example.com", "runtime": "php", "root_path": f.siteDir, "php_version": "8.3", "system_user": "alice"}
	for k, v := range extra {
		p[k] = v
	}
	return postSiteCreate(t, f.opts, p)
}

func TestPHPPool_SiteGetsOwnPoolAsItsUser(t *testing.T) {
	f := newPoolFixture(t)
	rec := f.create(t, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("create = %d %s", rec.Code, rec.Body.String())
	}
	var res map[string]any
	json.Unmarshal(rec.Body.Bytes(), &res)
	if res["php_pool"] != true {
		t.Fatalf("php_pool = %v", res["php_pool"])
	}
	pool, err := os.ReadFile(filepath.Join(f.etc, "8.3", "fpm", "pool.d", "sp-site-7.conf"))
	if err != nil {
		t.Fatal(err)
	}
	sock := filepath.Join(f.run, "sp-site-7-php8.3-fpm.sock")
	phpDir := filepath.Join(f.home, ".php", "7")
	for _, want := range []string{
		"[sp-site-7]", "user = alice", "group = alice", "listen = " + sock,
		"listen.owner = www-data", "listen.mode = 0660", "pm = ondemand",
		"php_admin_value[open_basedir] = " + f.siteDir + ":" + phpDir + ":/usr/share/php",
		"php_admin_value[session.save_path] = " + phpDir + "/sessions",
		"php_admin_value[upload_tmp_dir] = " + phpDir + "/tmp",
		"php_admin_flag[allow_url_include] = off", "catch_workers_output = no",
	} {
		if !strings.Contains(string(pool), want) {
			t.Fatalf("pool lacks %q:\n%s", want, pool)
		}
	}
	vhost, _ := os.ReadFile(filepath.Join(f.nginx, "shop.example.com.conf"))
	if !strings.Contains(string(vhost), "fastcgi_pass unix:"+sock+";") {
		t.Fatalf("vhost does not use the site's pool:\n%s", vhost)
	}
	for _, d := range []string{phpDir, phpDir + "/tmp", phpDir + "/sessions"} {
		fi, err := os.Stat(d)
		if err != nil || !fi.IsDir() || fi.Mode().Perm() != 0o700 {
			t.Fatalf("%s: %v %v", d, fi, err)
		}
	}
	calls, _ := os.ReadFile(f.fpmLog)
	if !strings.Contains(string(calls), "-t") {
		t.Fatalf("php-fpm -t not run: %q", calls)
	}
}

func TestPHPPool_RefusesSitesWithoutAUsableOwner(t *testing.T) {
	for _, user := range []string{"", "root", "www-data", "postgres", "bob"} {
		f := newPoolFixture(t)
		rec := f.create(t, map[string]any{"system_user": user})
		if rec.Code != http.StatusUnprocessableEntity && rec.Code != http.StatusBadRequest {
			t.Fatalf("owner %q = %d %s", user, rec.Code, rec.Body.String())
		}
		if _, err := os.Stat(filepath.Join(f.etc, "8.3", "fpm", "pool.d", "sp-site-7.conf")); !os.IsNotExist(err) {
			t.Fatalf("owner %q: a pool was written", user)
		}
		if _, err := os.Stat(filepath.Join(f.nginx, "shop.example.com.conf")); !os.IsNotExist(err) {
			t.Fatalf("owner %q: a vhost was written", user)
		}
	}
}

func TestPHPPool_RejectedConfigIsRolledBack(t *testing.T) {
	f := newPoolFixture(t)
	if rec := f.create(t, nil); rec.Code != http.StatusOK {
		t.Fatalf("create = %d %s", rec.Code, rec.Body.String())
	}
	poolPath := filepath.Join(f.etc, "8.3", "fpm", "pool.d", "sp-site-7.conf")
	before, _ := os.ReadFile(poolPath)
	os.WriteFile(filepath.Join(filepath.Dir(f.fpmLog), "FPM_FAIL"), nil, 0o644)
	// A different root changes the pool; php-fpm -t fails: the old one stays.
	other := filepath.Join(f.home, "htdocs", "other.example.com")
	rec := f.create(t, map[string]any{"root_path": other, "vhost_regen": true})
	if rec.Code < 400 {
		t.Fatalf("expected failure, got %d", rec.Code)
	}
	after, _ := os.ReadFile(poolPath)
	if !bytes.Equal(before, after) {
		t.Fatalf("pool not restored:\n%s", after)
	}
}

func TestPHPPool_VersionChangeAndDeleteRemovePools(t *testing.T) {
	f := newPoolFixture(t)
	if rec := f.create(t, nil); rec.Code != http.StatusOK {
		t.Fatalf("create = %d %s", rec.Code, rec.Body.String())
	}
	// 8.4 appears; the site moves there and the 8.3 pool goes away.
	os.MkdirAll(filepath.Join(f.etc, "8.4", "fpm", "pool.d"), 0o755)
	fpm84 := "#!/bin/sh\necho ok\n"
	path84 := filepath.Join(strings.Split(os.Getenv("PATH"), string(os.PathListSeparator))[0], "php-fpm8.4")
	os.WriteFile(path84, []byte(fpm84), 0o755)
	if rec := f.create(t, map[string]any{"php_version": "8.4", "vhost_regen": true}); rec.Code != http.StatusOK {
		t.Fatalf("regen 8.4 = %d %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(f.etc, "8.3", "fpm", "pool.d", "sp-site-7.conf")); !os.IsNotExist(err) {
		t.Fatal("8.3 pool left behind")
	}
	if _, err := os.Stat(filepath.Join(f.etc, "8.4", "fpm", "pool.d", "sp-site-7.conf")); err != nil {
		t.Fatal("8.4 pool missing")
	}
	// Switching the runtime away from PHP removes it too.
	if rec := f.create(t, map[string]any{"runtime": "static", "vhost_regen": true}); rec.Code != http.StatusOK {
		t.Fatalf("regen static = %d %s", rec.Code, rec.Body.String())
	}
	if m, _ := filepath.Glob(filepath.Join(f.etc, "*", "fpm", "pool.d", "sp-site-7.conf")); len(m) != 0 {
		t.Fatalf("pool left after runtime change: %v", m)
	}
	// Delete removes a pool as well.
	if rec := f.create(t, nil); rec.Code != http.StatusOK {
		t.Fatalf("create again = %d", rec.Code)
	}
	body, _ := json.Marshal(map[string]any{"site_id": 7, "domain": "shop.example.com", "root_path": f.siteDir})
	req := httptest.NewRequest(http.MethodPost, "/tasks/site/delete", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	NewRouterWithOptions(f.opts).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("delete = %d %s", rr.Code, rr.Body.String())
	}
	if m, _ := filepath.Glob(filepath.Join(f.etc, "*", "fpm", "pool.d", "sp-site-7.conf")); len(m) != 0 {
		t.Fatalf("pool left after delete: %v", m)
	}
}

func TestPHPPool_ErrorLogReadDoesNotFollowLinks(t *testing.T) {
	f := newPoolFixture(t)
	if rec := f.create(t, nil); rec.Code != http.StatusOK {
		t.Fatalf("create = %d %s", rec.Code, rec.Body.String())
	}
	read := func() *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]any{"site_id": 7, "domain": "shop.example.com", "type": "php", "lines": 10, "system_user": "alice"})
		req := httptest.NewRequest(http.MethodPost, "/tasks/logs/read", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		NewRouterWithOptions(f.opts).ServeHTTP(rr, req)
		return rr
	}
	logPath := filepath.Join(f.home, ".php", "7", "error.log")
	os.WriteFile(logPath, []byte("[x] PHP Warning: one\n[x] PHP Fatal error: two\n"), 0o600)
	rr := read()
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "Fatal error: two") {
		t.Fatalf("read = %d %s", rr.Code, rr.Body.String())
	}
	secret := filepath.Join(t.TempDir(), "shadow")
	os.WriteFile(secret, []byte("root:secret-hash\n"), 0o600)
	os.Remove(logPath)
	os.Symlink(secret, logPath)
	rr = read()
	if strings.Contains(rr.Body.String(), "secret-hash") {
		t.Fatalf("followed a planted symlink: %s", rr.Body.String())
	}
}

func TestPHPPool_HostWithoutPoolDirKeepsSharedUpstream(t *testing.T) {
	setupTestBinaries(t)
	nginxDir := t.TempDir()
	root := filepath.Join(t.TempDir(), "site")
	rec := postSiteCreate(t, RouterOptions{NginxConfDir: nginxDir, PHPRunDir: phpSocketDir(t, "8.3")}, map[string]any{
		"site_id": 9, "domain": "legacy.example.com", "runtime": "php", "root_path": root, "php_version": "8.3",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("create = %d %s", rec.Code, rec.Body.String())
	}
	var res map[string]any
	json.Unmarshal(rec.Body.Bytes(), &res)
	if res["php_pool"] != false {
		t.Fatalf("php_pool = %v", res["php_pool"])
	}
}
