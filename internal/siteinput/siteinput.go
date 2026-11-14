// Package siteinput validates user-influenced values that end up in root-run
// commands, nginx/systemd configuration or docker arguments. It is shared by
// the API (reject early) and the agent / nginx generator (defence in depth:
// the agent runs as root and must not trust the API).
package siteinput

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	domainRe       = regexp.MustCompile(`^(\*\.)?[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?)+$`)
	rootPathRe     = regexp.MustCompile(`^/[A-Za-z0-9._@+/\-]*$`)
	proxyPassRe    = regexp.MustCompile(`^https?://[A-Za-z0-9.\-]+(:[0-9]{1,5})?(/[A-Za-z0-9._~/%+\-]*)?$`)
	phpVersionRe   = regexp.MustCompile(`^[0-9]\.[0-9]{1,2}$`)
	systemUserRe   = regexp.MustCompile(`^[a-z_][a-z0-9_\-]{0,31}$`)
	dockerImageRe  = regexp.MustCompile(`^[a-z0-9]+([._\-/][a-z0-9]+)*(:[A-Za-z0-9_][A-Za-z0-9_.\-]{0,127})?$`)
	dockerNameRe   = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.\-]{0,63}$`)
	dockerPortRe   = regexp.MustCompile(`^(127\.0\.0\.1:)?[0-9]{1,5}:[0-9]{1,5}(/(tcp|udp))?$`)
	dockerEnvRe    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=[^\x00-\x1f]*$`)
	dockerTargetRe = regexp.MustCompile(`^/[A-Za-z0-9._/\-]*$`)
	dockerRestarts = map[string]bool{"": true, "no": true, "always": true, "unless-stopped": true, "on-failure": true}
	// reservedUsers are system and service accounts (and the sudo-capable
	// default users of cloud images): a site, an SSH account or a panel
	// sign-up must never run as, or become, one of them.
	reservedUsers = map[string]bool{
		"root": true, "daemon": true, "bin": true, "sys": true, "sync": true, "games": true, "man": true,
		"lp": true, "mail": true, "news": true, "uucp": true, "proxy": true, "www-data": true, "backup": true,
		"list": true, "irc": true, "gnats": true, "nobody": true, "_apt": true, "sigardapanel": true,
		"systemd-network": true, "systemd-resolve": true, "systemd-timesync": true, "messagebus": true,
		"syslog": true, "sshd": true, "uuidd": true, "tcpdump": true, "tss": true, "polkitd": true,
		"landscape": true, "pollinate": true, "fwupd-refresh": true, "usbmux": true, "lxd": true,
		"dnsmasq": true, "ntp": true, "chrony": true, "_chrony": true, "statd": true,
		"mysql": true, "postgres": true, "redis": true, "memcache": true, "memcached": true, "mongodb": true,
		"nginx": true, "apache": true, "httpd": true, "www": true, "varnish": true, "vcache": true,
		"docker": true, "containerd": true, "postfix": true, "dovecot": true, "dovenull": true,
		"opendkim": true, "clamav": true, "debian-exim": true, "bind": true, "ftp": true, "vsftpd": true,
		"elasticsearch": true, "rabbitmq": true, "git": true,
		"ubuntu": true, "debian": true, "centos": true, "ec2-user": true, "fedora": true, "rocky": true,
		"almalinux": true, "azureuser": true, "opc": true,
	}
)

// Domain accepts a hostname (optionally "*.example.com").
func Domain(s string) error {
	if len(s) > 253 || !domainRe.MatchString(s) {
		return fmt.Errorf("invalid domain")
	}
	return nil
}

// RootPath accepts an absolute path made of safe characters only: no spaces,
// quotes, braces, semicolons, newlines or ".." segments.
func RootPath(s string) error {
	if len(s) > 512 || !rootPathRe.MatchString(s) {
		return fmt.Errorf("invalid root_path")
	}
	for _, seg := range strings.Split(s, "/") {
		if seg == ".." {
			return fmt.Errorf("invalid root_path")
		}
	}
	return nil
}

// UnderRoot reports whether p is dir itself or inside dir (lexically).
func UnderRoot(dir, p string) bool {
	dir, p = path.Clean(dir), path.Clean(p)
	return p == dir || strings.HasPrefix(p, dir+"/")
}

// ProxyPassURL accepts http(s)://host[:port][/path] with no query string,
// whitespace, quotes, semicolons or variables, so it can be dropped into an
// nginx proxy_pass directive verbatim.
func ProxyPassURL(s string) error {
	if len(s) > 512 || !proxyPassRe.MatchString(s) {
		return fmt.Errorf("invalid proxy_pass_url (expected http(s)://host[:port][/path])")
	}
	if i := strings.LastIndex(s, ":"); i > strings.Index(s, "//")+1 {
		rest := s[i+1:]
		if j := strings.Index(rest, "/"); j >= 0 {
			rest = rest[:j]
		}
		if n, err := strconv.Atoi(rest); err == nil && (n < 1 || n > 65535) {
			return fmt.Errorf("invalid proxy_pass_url port")
		}
	}
	return nil
}

// PHPVersion accepts e.g. "8.3".
func PHPVersion(s string) error {
	if !phpVersionRe.MatchString(s) {
		return fmt.Errorf("invalid php_version")
	}
	return nil
}

// SystemUser accepts a conventional unprivileged unix account name. root and
// well-known service accounts are refused.
func SystemUser(s string) error {
	if !systemUserRe.MatchString(s) || reservedUsers[s] {
		return fmt.Errorf("invalid system_user")
	}
	return nil
}

// PHPIni validates the already-sanitised value placed inside a quoted nginx
// fastcgi_param PHP_VALUE "...": no double quotes, no stray backslashes (only
// the literal two-character "\n" separator), no control characters and no
// nginx-significant braces/semicolons.
func PHPIni(s string) error {
	if strings.ContainsAny(s, "\"{};\r$") {
		return fmt.Errorf("invalid php_ini")
	}
	rest := strings.ReplaceAll(s, `\n`, "")
	if strings.Contains(rest, `\`) {
		return fmt.Errorf("invalid php_ini")
	}
	for _, r := range s {
		if r < 0x20 && r != '\n' {
			return fmt.Errorf("invalid php_ini")
		}
	}
	return nil
}

// DockerImage accepts a lower-case image reference with an optional tag.
func DockerImage(s string) error {
	if len(s) > 255 || !dockerImageRe.MatchString(s) {
		return fmt.Errorf("invalid docker_image")
	}
	return nil
}

// DockerName accepts a container name.
func DockerName(s string) error {
	if !dockerNameRe.MatchString(s) {
		return fmt.Errorf("invalid docker_name")
	}
	return nil
}

// DockerRestart accepts a docker restart policy name.
func DockerRestart(s string) error {
	if !dockerRestarts[s] {
		return fmt.Errorf("invalid docker_restart")
	}
	return nil
}

// DockerPort accepts "host:container[/proto]" (optionally bound to 127.0.0.1).
func DockerPort(s string) error {
	if !dockerPortRe.MatchString(s) {
		return fmt.Errorf("invalid docker_ports")
	}
	for _, part := range strings.FieldsFunc(strings.TrimPrefix(strings.SplitN(s, "/", 2)[0], "127.0.0.1:"), func(r rune) bool { return r == ':' }) {
		if n, err := strconv.Atoi(part); err != nil || n < 1 || n > 65535 {
			return fmt.Errorf("invalid docker_ports")
		}
	}
	return nil
}

// DockerEnvLine accepts a single KEY=value line without control characters.
func DockerEnvLine(s string) error {
	if len(s) > 4096 || !dockerEnvRe.MatchString(s) {
		return fmt.Errorf("invalid docker_env entry")
	}
	return nil
}

// DockerVolume accepts "hostpath:containerpath[:ro|rw]" where hostpath must be
// inside siteRoot. Host-wide mounts, the docker socket and relative or named
// volumes are refused.
func DockerVolume(v, siteRoot string) error {
	parts := strings.Split(v, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return fmt.Errorf("invalid docker_volume")
	}
	host, target := parts[0], parts[1]
	if len(parts) == 3 && parts[2] != "ro" && parts[2] != "rw" {
		return fmt.Errorf("invalid docker_volume")
	}
	if RootPath(host) != nil || !dockerTargetRe.MatchString(target) || siteRoot == "" || !UnderRoot(siteRoot, host) {
		return fmt.Errorf("docker_volume host path must be inside the site root")
	}
	return nil
}

var protectedTopDirs = map[string]bool{
	"etc": true, "usr": true, "bin": true, "sbin": true, "lib": true, "lib32": true, "lib64": true,
	"boot": true, "dev": true, "proc": true, "sys": true, "run": true, "root": true, "snap": true,
}

// RootPathAllowed refuses roots that would put whole system trees (or a bare
// home directory) under panel control: site.delete runs RemoveAll on them.
func RootPathAllowed(p string) error {
	if err := RootPath(p); err != nil {
		return err
	}
	segs := strings.Split(strings.Trim(path.Clean(p), "/"), "/")
	if len(segs) < 2 || protectedTopDirs[segs[0]] || (segs[0] == "home" && len(segs) < 3) {
		return fmt.Errorf("root_path is not an allowed location")
	}
	return nil
}

// DockerLanguage accepts the short language tag used to pick image defaults.
func DockerLanguage(s string) error {
	if s != "" && !regexp.MustCompile(`^[a-z0-9]{1,16}$`).MatchString(s) {
		return fmt.Errorf("invalid docker_language")
	}
	return nil
}

// DockerOptions validates every user-supplied docker field of a site. volumes
// is comma separated, env newline separated, ports a single mapping.
func DockerOptions(image, name, ports, env, volumes, restart, language, siteRoot string) error {
	if image != "" {
		if err := DockerImage(image); err != nil {
			return err
		}
	}
	if name != "" {
		if err := DockerName(name); err != nil {
			return err
		}
	}
	if ports != "" {
		if err := DockerPort(ports); err != nil {
			return err
		}
	}
	for _, line := range strings.Split(env, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			if err := DockerEnvLine(line); err != nil {
				return err
			}
		}
	}
	for _, v := range strings.Split(volumes, ",") {
		if v = strings.TrimSpace(v); v != "" {
			if err := DockerVolume(v, siteRoot); err != nil {
				return err
			}
		}
	}
	if err := DockerRestart(restart); err != nil {
		return err
	}
	return DockerLanguage(language)
}

var dbPrivileges = map[string]bool{
	"ALL": true, "ALL PRIVILEGES": true, "SELECT": true, "INSERT": true, "UPDATE": true, "DELETE": true,
	"CREATE": true, "DROP": true, "ALTER": true, "INDEX": true, "REFERENCES": true,
	"CREATE TEMPORARY TABLES": true, "LOCK TABLES": true, "EXECUTE": true, "CREATE VIEW": true,
	"SHOW VIEW": true, "CREATE ROUTINE": true, "ALTER ROUTINE": true, "EVENT": true, "TRIGGER": true,
}

// DBPrivileges accepts a comma separated list of per-database privileges. It
// deliberately excludes GRANT OPTION, PROXY, FILE, SUPER and any global right.
func DBPrivileges(s string) error {
	if strings.TrimSpace(s) == "" || len(s) > 256 {
		return fmt.Errorf("invalid privileges")
	}
	for _, p := range strings.Split(s, ",") {
		p = strings.Join(strings.Fields(strings.ToUpper(p)), " ")
		if !dbPrivileges[p] {
			return fmt.Errorf("invalid privileges: %q is not allowed", p)
		}
	}
	return nil
}

// DBPassword accepts 8-128 printable characters excluding backslash, so the
// value can be embedded in a quoted SQL literal without any escaping rules
// (MySQL NO_BACKSLASH_ESCAPES vs. default) or psql meta-command tricks.
func DBPassword(s string) error {
	if len(s) < 8 || len(s) > 128 {
		return fmt.Errorf("password must be 8-128 characters")
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f || r == '\\' {
			return fmt.Errorf("password contains unsupported characters (control characters and backslash are not allowed)")
		}
	}
	return nil
}

var dbHostRe = regexp.MustCompile(`^[a-zA-Z0-9_.%\-]{1,255}$`)

// DBHost accepts the host part of a MySQL account ("localhost", "%", "10.0.0.%").
func DBHost(s string) error {
	if !dbHostRe.MatchString(s) {
		return fmt.Errorf("invalid host_pattern")
	}
	return nil
}

// ReservedDatabaseName reports names that belong to the database server
// itself (MySQL/MariaDB system schemas, PostgreSQL maintenance and template
// databases, phpMyAdmin's storage). A tenant who could register one would get
// GRANT ALL on it, which on mysql.* means every account on the server.
func ReservedDatabaseName(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	switch n {
	case "mysql", "sys", "information_schema", "performance_schema", "phpmyadmin",
		"postgres", "template0", "template1":
		return true
	}
	return strings.HasPrefix(n, "template") || strings.HasPrefix(n, "pg_")
}

// phpIniAllowed are the php.ini settings a site may set through PHP_VALUE.
// PHP-FPM keeps PHP_VALUE settings in the worker process, and the pool is
// shared by every site, so anything that loads code or widens file access
// (auto_prepend_file, open_basedir, extension, include_path, ...) would act
// on other tenants' requests.
var phpIniAllowed = map[string]bool{
	"memory_limit": true, "upload_max_filesize": true, "post_max_size": true,
	"max_execution_time": true, "max_input_time": true, "max_input_vars": true,
	"display_errors": true, "error_reporting": true, "date.timezone": true,
	"session.gc_maxlifetime": true, "short_open_tag": true, "output_buffering": true,
}

var phpIniValueRe = regexp.MustCompile(`^[A-Za-z0-9_./:+&~|^ -]{1,64}$`)

// FilterPHPIni keeps the allowed "key = value" directives of a php_ini value
// (lines separated by real newlines or the literal two-character "\n" used in
// the stored form) and returns them in the stored form, with the rejected
// lines.
func FilterPHPIni(s string) (string, []string) {
	var kept, rejected []string
	for _, part := range strings.Split(strings.ReplaceAll(s, `\n`, "\n"), "\n") {
		line := strings.TrimSpace(part)
		if line == "" {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if !ok || !phpIniAllowed[k] || !phpIniValueRe.MatchString(v) {
			rejected = append(rejected, line)
			continue
		}
		kept = append(kept, k+" = "+v)
	}
	return strings.Join(kept, `\n `), rejected
}

// panelPorts are the panel's own services on a host (API, agent, web).
var panelPorts = map[string]bool{"7700": true, "7710": true, "7720": true}

// ProxyPassTarget checks where a proxy site would send requests (its host,
// resolved when it is a name). Link-local (cloud metadata at 169.254.169.254),
// unspecified and multicast targets are always refused. Without
// allowInternal (the host operator): loopback is allowed for the tenant's own
// app except the panel's ports (API, agent, web), private networks are not.
func ProxyPassTarget(ctx context.Context, raw string, allowInternal bool) error {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return fmt.Errorf("invalid proxy_pass_url")
	}
	host := u.Hostname()
	port := u.Port()
	var ips []net.IP
	if ip := net.ParseIP(host); ip != nil {
		ips = []net.IP{ip}
	} else {
		if strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".localhost") {
			ips = []net.IP{net.IPv4(127, 0, 0, 1)}
		} else {
			rctx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()
			addrs, err := net.DefaultResolver.LookupIPAddr(rctx, host)
			if err != nil || len(addrs) == 0 {
				return fmt.Errorf("proxy_pass_url host does not resolve")
			}
			for _, a := range addrs {
				ips = append(ips, a.IP)
			}
		}
	}
	for _, ip := range ips {
		if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
			return fmt.Errorf("proxy_pass_url may not point to a link-local, unspecified or multicast address")
		}
		if allowInternal {
			continue
		}
		if ip.IsLoopback() && panelPorts[port] {
			return fmt.Errorf("proxy_pass_url may not point to the panel's own services")
		}
		if ip.IsPrivate() {
			return fmt.Errorf("proxy_pass_url may not point to a private network address (ask an administrator)")
		}
	}
	return nil
}
