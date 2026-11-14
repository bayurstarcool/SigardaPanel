package agenthttp

import (
	"fmt"
	"net"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"sigardapanel/internal/nginx"
)

// defaultPHPRunDir is where distro / ondrej PHP-FPM pools put their sockets
// (php<version>-fpm.sock). Generated vhosts address PHP-FPM there.
const defaultPHPRunDir = "/run/php"

func (r *Router) phpSocket(version string) string {
	return filepath.Join(r.phpRunDir, "php"+version+"-fpm.sock")
}

// installedPHPFPMVersions lists the versions with a php<version>-fpm.sock.
func (r *Router) installedPHPFPMVersions() []string {
	matches, _ := filepath.Glob(filepath.Join(r.phpRunDir, "php*-fpm.sock"))
	var out []string
	for _, m := range matches {
		v := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(m), "php"), "-fpm.sock")
		if v != "" {
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}

func tcpListening(addr string) bool {
	conn, err := net.DialTimeout("tcp", addr, 300*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// checkPHPFPM verifies, before a PHP site is created, that the PHP-FPM the
// generated vhost will point at exists on this server, mirroring how
// nginx.GenerateVhost picks the upstream: the version's socket, else its
// CloudPanel TCP port; without a version the first default socket, else the
// CloudPanel socket. The error text is meant for the job log.
func (r *Router) checkPHPFPM(version string) error {
	installed := r.installedPHPFPMVersions()
	have := "No PHP-FPM is installed on this server."
	if len(installed) > 0 {
		have = "Installed PHP-FPM versions: " + strings.Join(installed, ", ") + "."
	}

	// The site gets its own pool when the version's pool directory exists
	// (php<version>-fpm installed); the shared socket is not needed then.
	if r.poolVersion(version) != "" {
		return nil
	}
	if version != "" {
		if fileExists(r.phpSocket(version)) {
			return nil
		}
		where := r.phpSocket(version)
		if port, ok := nginx.PHPFPMTCPPort(version); ok {
			addr := "127.0.0.1:" + strconv.Itoa(port)
			if tcpListening(addr) {
				return nil
			}
			where += " and nothing listening on " + addr
		}
		return fmt.Errorf("PHP-FPM %s is not installed on this server (no %s). %s "+
			"Install PHP %s on the server (Servers -> Stack) or pick an installed version, then create the site again",
			version, where, have, version)
	}

	for _, v := range nginx.PHPFPMDefaultVersions {
		if fileExists(r.phpSocket(v)) {
			return nil
		}
	}
	clp := filepath.Join(filepath.Dir(r.phpRunDir), "clp-php-fpm.sock")
	if fileExists(clp) {
		return nil
	}
	return fmt.Errorf("PHP-FPM is not installed on this server (no %s for %s, no %s). %s "+
		"Install PHP on the server (Servers -> Stack) or choose an installed PHP version, then create the site again",
		r.phpSocket("<version>"), strings.Join(nginx.PHPFPMDefaultVersions, "/"), clp, have)
}
