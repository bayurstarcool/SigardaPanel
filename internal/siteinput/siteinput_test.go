package siteinput

import "testing"

func check(t *testing.T, name string, fn func(string) error, good, bad []string) {
	t.Helper()
	for _, s := range good {
		if err := fn(s); err != nil {
			t.Errorf("%s(%q) should pass: %v", name, s, err)
		}
	}
	for _, s := range bad {
		if err := fn(s); err == nil {
			t.Errorf("%s(%q) should be rejected", name, s)
		}
	}
}

func TestProxyPassURL(t *testing.T) {
	check(t, "ProxyPassURL", ProxyPassURL,
		[]string{"http://127.0.0.1:30005", "https://app.internal:8443/api/", "http://backend"},
		[]string{"http://x;\n}\nlocation /p/ {alias /home/;autoindex on;\n", "http://a b", "http://x;", "http://x$host", "ftp://x", "http://x\"", "http://x:99999", "http://x/?q=1", ""})
}

func TestRootPath(t *testing.T) {
	check(t, "RootPath", RootPath,
		[]string{"/home/admin/htdocs/sigarda.com", "/opt/gps-tracker", "/var/www/a_b-c+d@e"},
		[]string{"home/x", "/home/x/../etc", "/x\n}\nserver{", "/x y", "/x;y", "/x\"y", "/x{y}", "/x$y", "/x`id`"})
}

func TestSystemUser(t *testing.T) {
	check(t, "SystemUser", SystemUser,
		[]string{"hanyawahyu", "whmcs", "user_1", "a-b"},
		[]string{"root", "", "Root", "x\nExecStartPre=/bin/evil", "a b", "1abc", "nobody", "www-data", "sigardapanel"})
}

func TestPHPVersionAndIni(t *testing.T) {
	check(t, "PHPVersion", PHPVersion, []string{"8.3", "7.4", "8.10"}, []string{"8", "8.3; evil", "../8.3", ""})
	check(t, "PHPIni", PHPIni,
		[]string{"", "memory_limit=256M", `memory_limit=256M\n max_execution_time=60`},
		[]string{`a" ; fastcgi_param X "b`, "a;b", "a{b}", `trailing\`, `x\"y`, "a\rb"})
}

func TestDocker(t *testing.T) {
	check(t, "DockerImage", DockerImage, []string{"nginx", "php:8.3-apache", "ghcr.io/org/app:1.2"}, []string{"-v/:/h x", "Nginx", "a b", "img;rm", "--privileged", ""})
	check(t, "DockerName", DockerName, []string{"web-1", "a_b.c"}, []string{"-x", "a b", "", "a;b"})
	check(t, "DockerRestart", DockerRestart, []string{"", "always", "unless-stopped"}, []string{"--privileged", "x"})
	check(t, "DockerPort", DockerPort, []string{"8080:80", "127.0.0.1:8080:80", "443:443/tcp"}, []string{"80", "8080:80;x", "0:80", "70000:80", "8080:80 --privileged", "a:b"})
	check(t, "DockerEnvLine", DockerEnvLine, []string{"FOO=bar", "A_B=x y"}, []string{"--privileged", "FOO", "A=b\nC=d", "=x", "1A=b"})

	root := "/home/u/htdocs/site"
	good := []string{root + ":/var/www", root + "/data:/data:ro"}
	bad := []string{"/:/host", "/var/run/docker.sock:/s", "/home/u/htdocs/other:/x", root + "/../x:/y", "relative:/x", root + ":relative", root, "named_vol:/x", root + ":/x:evil", root + ":/x;y"}
	for _, v := range good {
		if err := DockerVolume(v, root); err != nil {
			t.Errorf("DockerVolume(%q) should pass: %v", v, err)
		}
	}
	for _, v := range bad {
		if err := DockerVolume(v, root); err == nil {
			t.Errorf("DockerVolume(%q) should be rejected", v)
		}
	}
	if DockerVolume(root+":/x", "") == nil {
		t.Error("empty siteRoot must reject")
	}
}

func TestRootPathAllowed(t *testing.T) {
	check(t, "RootPathAllowed", RootPathAllowed,
		[]string{"/home/u/htdocs/a.com", "/opt/gps-tracker", "/var/www/site", "/srv/app"},
		[]string{"/", "/etc", "/etc/nginx", "/usr/share/x", "/root/x", "/home", "/home/u", "/opt", "/boot/x", "/home/u/../../etc"})
}

func TestDockerOptions(t *testing.T) {
	root := "/home/u/htdocs/a.com"
	if err := DockerOptions("php:8.3-apache", "web", "8080:80", "A=1\nB=2", root+":/var/www/html", "unless-stopped", "php", root); err != nil {
		t.Fatalf("valid options rejected: %v", err)
	}
	if err := DockerOptions("nginx", "", "", "", "/:/host", "", "", root); err == nil {
		t.Fatal("host bind mount must be rejected")
	}
	if err := DockerOptions("nginx", "", "", "", "", "--privileged", "", root); err == nil {
		t.Fatal("bad restart must be rejected")
	}
}

func TestDBPrivilegesAndPassword(t *testing.T) {
	check(t, "DBPrivileges", DBPrivileges,
		[]string{"ALL", "select, insert,update", "ALL PRIVILEGES", "create temporary tables"},
		[]string{"", "ALL ON *.* TO 'x'@'%' WITH GRANT OPTION --", "GRANT OPTION", "SUPER", "FILE", "ALL;DROP", "SELECT,"})
	check(t, "DBPassword", DBPassword,
		[]string{"abcdefgh", "p@ss'w0rd$$!", "päss-wörd-ok"},
		[]string{"short", `back\slash12`, "new\nline123", "nul\x00byte12", "\\! id 12345"})
}
