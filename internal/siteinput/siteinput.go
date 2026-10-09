// Package siteinput validates user-influenced values that end up in root-run
// commands, nginx/systemd configuration or docker arguments. It is shared by
// the API (reject early) and the agent / nginx generator (defence in depth:
// the agent runs as root and must not trust the API).
package siteinput

import (
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"
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
	reservedUsers  = map[string]bool{"root": true, "daemon": true, "bin": true, "sys": true, "nobody": true, "www-data": true, "sigardapanel": true}
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
	if strings.ContainsAny(s, "\"{};\r") {
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
