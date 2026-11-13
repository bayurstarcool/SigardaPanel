package nginx

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"

	"sigardapanel/internal/siteinput"
	"sigardapanel/internal/store"
)

const markerBegin = "# BEGIN SIGARDAPANEL MANAGED BLOCK - DO NOT EDIT"
const markerEnd = "# END SIGARDAPANEL MANAGED BLOCK"

// HTTP + SSL templates (wildcard cert covers *.sigarda.dev and *.sigarda.com)
const staticTemplate = `server {
    listen 80;
    listen [::]:80;
    listen 443 ssl http2;
    listen [::]:443 ssl http2;
    server_name {{.Domain}};
    client_max_body_size 100M;

    ssl_certificate {{.SSLCertificate}};
    ssl_certificate_key {{.SSLCertificateKey}};
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers HIGH:!aNULL:!MD5;

    # ACME challenge for Let's Encrypt (future use)
    location /.well-known/acme-challenge/ {
        root /var/www/certbot/{{.Domain}};
    }

    # Panel backups are stored under the site root: never serve them.
    location ^~ /backups/ { deny all; }
    location ^~ /backup/ { deny all; }

    root {{.RootPath}};
    index index.html;

    # SigardaPanel marker
    {{.MarkerBegin}}
    access_log /var/log/nginx/{{.Domain}}.access.log;
    error_log /var/log/nginx/{{.Domain}}.error.log;
    {{.MarkerEnd}}
{{if .BasicAuthEnabled}}
    auth_basic "Restricted";
    auth_basic_user_file {{.BasicAuthPasswordPath}};
{{end}}

    location / {
{{range .BlockedIPs}}        deny {{.}};
{{end}}{{range .AllowedIPs}}        allow {{.}};
{{end}}{{if .AllowedIPs}}        deny all;
{{end}}        try_files $uri $uri/ =404;
    }

    # SigardaPanel marker
    {{.MarkerBegin}}
    location ~* \.(jpg|jpeg|png|gif|ico|css|js)$ {
        expires 7d;
        access_log off;
    }
    {{.MarkerEnd}}
}
`

const phpTemplate = `server {
    listen 80;
    listen [::]:80;
    listen 443 ssl http2;
    listen [::]:443 ssl http2;
    server_name {{.Domain}};
    client_max_body_size 100M;

    ssl_certificate {{.SSLCertificate}};
    ssl_certificate_key {{.SSLCertificateKey}};
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers HIGH:!aNULL:!MD5;

    # ACME challenge for Let's Encrypt
    location /.well-known/acme-challenge/ {
        root /var/www/certbot/{{.Domain}};
    }

    # Panel backups are stored under the site root: never serve them.
    location ^~ /backups/ { deny all; }
    location ^~ /backup/ { deny all; }

    root {{.RootPath}};
    index index.php index.html;

    # SigardaPanel marker
    {{.MarkerBegin}}
    access_log /var/log/nginx/{{.Domain}}.access.log;
    error_log /var/log/nginx/{{.Domain}}.error.log;
    {{.MarkerEnd}}
{{if .BasicAuthEnabled}}
    auth_basic "Restricted";
    auth_basic_user_file {{.BasicAuthPasswordPath}};
{{end}}

    location / {
{{range .BlockedIPs}}        deny {{.}};
{{end}}{{range .AllowedIPs}}        allow {{.}};
{{end}}{{if .AllowedIPs}}        deny all;
{{end}}        try_files $uri $uri/ /index.php?$query_string;
    }

    location ~ \.php$ {
        fastcgi_split_path_info ^(.+\.php)(/.+)$;
        fastcgi_pass {{.PHPFpmPass}};
        fastcgi_index index.php;
        include fastcgi_params;
        fastcgi_param SCRIPT_FILENAME $document_root$fastcgi_script_name;
        fastcgi_param PATH_INFO $fastcgi_path_info;
        {{if .PHPIniDirectives}}
        fastcgi_param PHP_VALUE "{{.PHPIniDirectives}}";
        {{end}}
    }
}
`

const nodeTemplate = `server {
    listen 80;
    listen [::]:80;
    listen 443 ssl http2;
    listen [::]:443 ssl http2;
    server_name {{.Domain}};
    client_max_body_size 100M;

    ssl_certificate {{.SSLCertificate}};
    ssl_certificate_key {{.SSLCertificateKey}};
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers HIGH:!aNULL:!MD5;

    # ACME challenge for Let's Encrypt
    location /.well-known/acme-challenge/ {
        root /var/www/certbot/{{.Domain}};
    }

    # Panel backups are stored under the site root: never serve them.
    location ^~ /backups/ { deny all; }
    location ^~ /backup/ { deny all; }

    # SigardaPanel marker
    {{.MarkerBegin}}
    access_log /var/log/nginx/{{.Domain}}.access.log;
    error_log /var/log/nginx/{{.Domain}}.error.log;
    {{.MarkerEnd}}
{{if .BasicAuthEnabled}}
    auth_basic "Restricted";
    auth_basic_user_file {{.BasicAuthPasswordPath}};
{{end}}

    location / {
{{range .BlockedIPs}}        deny {{.}};
{{end}}{{range .AllowedIPs}}        allow {{.}};
{{end}}{{if .AllowedIPs}}        deny all;
{{end}}        proxy_pass http://127.0.0.1:{{.Port}};
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
`

const proxyTemplate = `server {
    listen 80;
    listen [::]:80;
    listen 443 ssl http2;
    listen [::]:443 ssl http2;
    server_name {{.Domain}};
    client_max_body_size 100M;

    ssl_certificate {{.SSLCertificate}};
    ssl_certificate_key {{.SSLCertificateKey}};
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers HIGH:!aNULL:!MD5;

    # ACME challenge for Let's Encrypt
    location /.well-known/acme-challenge/ {
        root /var/www/certbot/{{.Domain}};
    }

    # Panel backups are stored under the site root: never serve them.
    location ^~ /backups/ { deny all; }
    location ^~ /backup/ { deny all; }

    # SigardaPanel marker
    {{.MarkerBegin}}
    access_log /var/log/nginx/{{.Domain}}.access.log;
    error_log /var/log/nginx/{{.Domain}}.error.log;
    {{.MarkerEnd}}
{{if .BasicAuthEnabled}}
    auth_basic "Restricted";
    auth_basic_user_file {{.BasicAuthPasswordPath}};
{{end}}

    location / {
{{range .BlockedIPs}}        deny {{.}};
{{end}}{{range .AllowedIPs}}        allow {{.}};
{{end}}{{if .AllowedIPs}}        deny all;
{{end}}        proxy_pass {{.ProxyPassURL}};
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }
}
`

// SSL templates (with ACME support - no HTTPS redirect, Cloudflare handles that)
const staticSSLTemplate = `server {
    listen 80;
    listen [::]:80;
    server_name {{.Domain}};
    client_max_body_size 100M;

    # ACME challenge for Let's Encrypt
    location /.well-known/acme-challenge/ {
        root /var/www/certbot/{{.Domain}};
    }

    # Panel backups are stored under the site root: never serve them.
    location ^~ /backups/ { deny all; }
    location ^~ /backup/ { deny all; }

    root {{.RootPath}};
    index index.html;

    # SigardaPanel marker
    {{.MarkerBegin}}
    access_log /var/log/nginx/{{.Domain}}.access.log;
    error_log /var/log/nginx/{{.Domain}}.error.log;
    {{.MarkerEnd}}
{{if .BasicAuthEnabled}}
    auth_basic "Restricted";
    auth_basic_user_file {{.BasicAuthPasswordPath}};
{{end}}

    location / {
{{range .BlockedIPs}}        deny {{.}};
{{end}}{{range .AllowedIPs}}        allow {{.}};
{{end}}{{if .AllowedIPs}}        deny all;
{{end}}        try_files $uri $uri/ =404;
    }

    # SigardaPanel marker
    {{.MarkerBegin}}
    location ~* \.(jpg|jpeg|png|gif|ico|css|js)$ {
        expires 7d;
        access_log off;
    }
    {{.MarkerEnd}}
}
`

const phpSSLTemplate = `server {
    listen 80;
    listen [::]:80;
    server_name {{.Domain}};
    client_max_body_size 100M;

    # ACME challenge for Let's Encrypt
    location /.well-known/acme-challenge/ {
        root /var/www/certbot/{{.Domain}};
    }

    # Panel backups are stored under the site root: never serve them.
    location ^~ /backups/ { deny all; }
    location ^~ /backup/ { deny all; }

    root {{.RootPath}};
    index index.php index.html;

    # SigardaPanel marker
    {{.MarkerBegin}}
    access_log /var/log/nginx/{{.Domain}}.access.log;
    error_log /var/log/nginx/{{.Domain}}.error.log;
    {{.MarkerEnd}}
{{if .BasicAuthEnabled}}
    auth_basic "Restricted";
    auth_basic_user_file {{.BasicAuthPasswordPath}};
{{end}}

    location / {
{{range .BlockedIPs}}        deny {{.}};
{{end}}{{range .AllowedIPs}}        allow {{.}};
{{end}}{{if .AllowedIPs}}        deny all;
{{end}}        try_files $uri $uri/ /index.php?$query_string;
    }

    location ~ \.php$ {
        fastcgi_split_path_info ^(.+\.php)(/.+)$;
        fastcgi_pass {{.PHPFpmPass}};
        fastcgi_index index.php;
        include fastcgi_params;
        fastcgi_param SCRIPT_FILENAME $document_root$fastcgi_script_name;
        fastcgi_param PATH_INFO $fastcgi_path_info;
        {{if .PHPIniDirectives}}
        fastcgi_param PHP_VALUE "{{.PHPIniDirectives}}";
        {{end}}
    }
}
`

const nodeSSLTemplate = `server {
    listen 80;
    listen [::]:80;
    server_name {{.Domain}};
    client_max_body_size 100M;

    # ACME challenge for Let's Encrypt
    location /.well-known/acme-challenge/ {
        root /var/www/certbot/{{.Domain}};
    }

    # Panel backups are stored under the site root: never serve them.
    location ^~ /backups/ { deny all; }
    location ^~ /backup/ { deny all; }

    # SigardaPanel marker
    {{.MarkerBegin}}
    access_log /var/log/nginx/{{.Domain}}.access.log;
    error_log /var/log/nginx/{{.Domain}}.error.log;
    {{.MarkerEnd}}
{{if .BasicAuthEnabled}}
    auth_basic "Restricted";
    auth_basic_user_file {{.BasicAuthPasswordPath}};
{{end}}

    location / {
{{range .BlockedIPs}}        deny {{.}};
{{end}}{{range .AllowedIPs}}        allow {{.}};
{{end}}{{if .AllowedIPs}}        deny all;
{{end}}        proxy_pass http://127.0.0.1:{{.Port}};
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
`

const proxySSLTemplate = `server {
    listen 80;
    listen [::]:80;
    server_name {{.Domain}};
    client_max_body_size 100M;

    # ACME challenge for Let's Encrypt
    location /.well-known/acme-challenge/ {
        root /var/www/certbot/{{.Domain}};
    }

    # Panel backups are stored under the site root: never serve them.
    location ^~ /backups/ { deny all; }
    location ^~ /backup/ { deny all; }

    # SigardaPanel marker
    {{.MarkerBegin}}
    access_log /var/log/nginx/{{.Domain}}.access.log;
    error_log /var/log/nginx/{{.Domain}}.error.log;
    {{.MarkerEnd}}
{{if .BasicAuthEnabled}}
    auth_basic "Restricted";
    auth_basic_user_file {{.BasicAuthPasswordPath}};
{{end}}

    location / {
{{range .BlockedIPs}}        deny {{.}};
{{end}}{{range .AllowedIPs}}        allow {{.}};
{{end}}{{if .AllowedIPs}}        deny all;
{{end}}        proxy_pass {{.ProxyPassURL}};
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
`

var templates = map[string]string{
	"static": staticTemplate,
	"php":    phpTemplate,
	"node":   nodeTemplate,
	"python": nodeTemplate,
	"go":     nodeTemplate,
	"proxy":  proxyTemplate,
	"docker": proxyTemplate,
}

// SiteConfig holds data for nginx vhost template rendering.
type SiteConfig struct {
	Domain                string
	RootPath              string
	Runtime               string
	Port                  int
	ProxyPassURL          string
	PHPFpmPass            string
	PHPIniDirectives      string
	MarkerBegin           string
	MarkerEnd             string
	BasicAuthEnabled      bool
	BasicAuthUsername     string
	BasicAuthPasswordPath string
	BlockedIPs            []string
	AllowedIPs            []string
	SSLCertificate        string
	SSLCertificateKey     string
}

// Placeholder certificate every vhost serves until Let's Encrypt issued one.
const (
	DefaultSSLCertificate    = "/etc/nginx/ssl/sigardapanel.crt"
	DefaultSSLCertificateKey = "/etc/nginx/ssl/sigardapanel.key"
)

var (
	basicAuthUserRe = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)
	bcryptHashRe    = regexp.MustCompile(`^\$2[aby]\$[0-9]{2}\$[./A-Za-z0-9]{53}$`)
	certPathRe      = regexp.MustCompile(`^/[A-Za-z0-9._/-]+\.pem$`)
)

// NormalizeBlockedIPs validates blocked IP entries (an IP or a CIDR each)
// and returns them in canonical form. Anything else is rejected, so the
// values can never carry nginx syntax into a vhost.
func NormalizeBlockedIPs(in []string) ([]string, error) {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, raw := range in {
		v := strings.TrimSpace(raw)
		if v == "" {
			continue
		}
		var norm string
		if strings.Contains(v, "/") {
			_, n, err := net.ParseCIDR(v)
			if err != nil {
				return nil, fmt.Errorf("invalid blocked IP or CIDR %q", raw)
			}
			norm = n.String()
		} else {
			ip := net.ParseIP(v)
			if ip == nil {
				return nil, fmt.Errorf("invalid blocked IP or CIDR %q", raw)
			}
			norm = ip.String()
		}
		if !seen[norm] {
			seen[norm] = true
			out = append(out, norm)
		}
	}
	if len(out) > 500 {
		return nil, fmt.Errorf("too many blocked IPs (max 500)")
	}
	return out, nil
}

// ValidBasicAuthUsername reports whether name may be used in an htpasswd file.
func ValidBasicAuthUsername(name string) bool { return basicAuthUserRe.MatchString(name) }

// validateSecurity is the vhost-side check of the per-site security settings.
func validateSecurity(sec *store.SiteSecurity) error {
	if sec == nil {
		return nil
	}
	if _, err := NormalizeBlockedIPs(sec.BlockedIPs); err != nil {
		return err
	}
	if _, err := NormalizeBlockedIPs(sec.AllowedIPs); err != nil {
		return err
	}
	for _, v := range append(append([]string{}, sec.BlockedIPs...), sec.AllowedIPs...) {
		if strings.TrimSpace(v) != v || v == "" {
			return fmt.Errorf("invalid blocked IP %q", v)
		}
	}
	if sec.BasicAuthEnabled {
		if !basicAuthUserRe.MatchString(sec.BasicAuthUsername) {
			return fmt.Errorf("invalid basic auth username")
		}
		if !bcryptHashRe.MatchString(sec.BasicAuthPassword) {
			return fmt.Errorf("basic auth is enabled but no valid password hash is stored")
		}
	}
	return nil
}

// phpFpmTCPPorts maps PHP versions to the TCP port a CloudPanel-style
// PHP-FPM pool listens on when there is no unix socket.
var phpFpmTCPPorts = map[string]int{
	"7.1": 11000, "7.2": 12000, "7.3": 13000, "7.4": 14000,
	"8.0": 15000, "8.1": 16000, "8.2": 17000, "8.3": 18000,
	"8.4": 19000, "8.5": 20000,
}

// PHPFPMTCPPort returns the TCP port generated vhosts fall back to for a PHP
// version without a /run/php/php<version>-fpm.sock socket.
func PHPFPMTCPPort(version string) (int, bool) {
	p, ok := phpFpmTCPPorts[version]
	return p, ok
}

// PHPFPMDefaultVersions are the sockets a vhost without an explicit (or with
// an unknown) PHP version uses, in order of preference.
var PHPFPMDefaultVersions = []string{"8.3", "8.4", "8.2", "8.1", "8.0"}

// phpFpmAddr returns the TCP address for a specific PHP version.
func phpFpmAddr(version string) string {
	if p, ok := phpFpmTCPPorts[version]; ok {
		// Check if PHP-FPM is actually listening on TCP, otherwise use Unix socket
		socketPath := fmt.Sprintf("/run/php/php%s-fpm.sock", version)
		if _, err := os.Stat(socketPath); err == nil {
			return fmt.Sprintf("unix:%s", socketPath)
		}
		return fmt.Sprintf("127.0.0.1:%d", p)
	}
	// Default: try Unix socket for common versions
	for _, v := range PHPFPMDefaultVersions {
		socketPath := fmt.Sprintf("/run/php/php%s-fpm.sock", v)
		if _, err := os.Stat(socketPath); err == nil {
			return fmt.Sprintf("unix:%s", socketPath)
		}
	}
	return "unix:/run/clp-php-fpm.sock"
}

// writeHtpasswd generates an nginx htpasswd file.
func WriteHtpasswd(domain, username, bcryptHash string) error {
	path := htpasswdPath(domain)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("mkdir htpasswd dir: %w", err)
	}
	line := fmt.Sprintf("%s:%s\n", username, bcryptHash)
	return os.WriteFile(path, []byte(line), 0644)
}

// RemoveHtpasswd removes the htpasswd file for a domain.
func RemoveHtpasswd(domain string) error {
	path := htpasswdPath(domain)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func htpasswdPath(domain string) string {
	return filepath.Join("/etc/nginx", ".htpasswd_"+domain)
}

// GenerateVhost renders an nginx vhost configuration for a site.
// SSL is enabled by default via wildcard cert.
func GenerateVhost(s *store.Site, port int, security *store.SiteSecurity) (string, error) {
	return GenerateVhostWithCert(s, port, security, "", "")
}

// GenerateVhostWithCert is GenerateVhost serving the given certificate
// (e.g. Let's Encrypt's live/<domain>/fullchain.pem) instead of the
// placeholder. Empty paths mean the placeholder.
func GenerateVhostWithCert(s *store.Site, port int, security *store.SiteSecurity, certPath, keyPath string) (string, error) {
	if certPath == "" || keyPath == "" {
		certPath, keyPath = DefaultSSLCertificate, DefaultSSLCertificateKey
	}
	if !certPathRe.MatchString(certPath) && certPath != DefaultSSLCertificate || !certPathRe.MatchString(keyPath) && keyPath != DefaultSSLCertificateKey {
		return "", fmt.Errorf("invalid certificate path")
	}
	if err := validateSecurity(security); err != nil {
		return "", err
	}
	tmplStr := templates[s.Runtime]
	if tmplStr == "" {
		return "", fmt.Errorf("unsupported runtime %q", s.Runtime)
	}
	// Every value interpolated into the vhost is validated here, the single
	// choke point, so no caller can inject nginx directives.
	if err := validateSiteForVhost(s); err != nil {
		return "", err
	}

	t, err := template.New("vhost").Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}

	proxyPassURL := ""
	if s.ProxyPassURL.Valid {
		proxyPassURL = s.ProxyPassURL.String
	}
	if proxyPassURL == "" && s.Runtime == "proxy" {
		proxyPassURL = "http://127.0.0.1:7700"
	}

	// Determine PHP-FPM address (auto-detect socket, fallback to TCP)
	phpFpmPass := phpFpmAddr(s.PHPVersion.String)
	if s.PHPVersion.Valid && s.PHPVersion.String != "" {
		phpFpmPass = phpFpmAddr(s.PHPVersion.String)
	}
	var phpIni string
	if s.PHPIni.Valid {
		phpIni = s.PHPIni.String
	}

	data := SiteConfig{
		Domain:            s.Domain,
		RootPath:          s.RootPath,
		Runtime:           s.Runtime,
		Port:              port,
		ProxyPassURL:      proxyPassURL,
		PHPFpmPass:        phpFpmPass,
		PHPIniDirectives:  phpIni,
		MarkerBegin:       markerBegin,
		MarkerEnd:         markerEnd,
		SSLCertificate:    certPath,
		SSLCertificateKey: keyPath,
	}

	if security != nil && security.BasicAuthEnabled {
		data.BasicAuthEnabled = true
		data.BasicAuthUsername = security.BasicAuthUsername
		data.BasicAuthPasswordPath = htpasswdPath(s.Domain)
		if err := WriteHtpasswd(s.Domain, security.BasicAuthUsername, security.BasicAuthPassword); err != nil {
			return "", fmt.Errorf("write htpasswd: %w", err)
		}
	}
	if security != nil && len(security.BlockedIPs) > 0 {
		data.BlockedIPs = security.BlockedIPs
	}
	if security != nil && len(security.AllowedIPs) > 0 {
		data.AllowedIPs = security.AllowedIPs
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}

	return buf.String(), nil
}

// TemplateForRuntime returns starter file templates for a runtime (e.g., package.json for node).
func TemplateForRuntime(runtime, domain string, port int) map[string]string {
	switch runtime {
	case "node":
		return map[string]string{
			"package.json": fmt.Sprintf(`{
  "name": %q,
  "version": "1.0.0",
  "scripts": {
    "start": "node server.js"
  }
}`, domain),
			"server.js": fmt.Sprintf(`const http = require('http');
const PORT = %d;
const server = http.createServer((req, res) => {
  res.writeHead(200, { 'Content-Type': 'text/html' });
  res.end('<h1>%s</h1><p>Site created via SigardaPanel</p>');
});
server.listen(PORT, () => console.log('Server running on port ' + PORT));
`, port, domain),
		}
	case "python":
		return map[string]string{
			"app.py": fmt.Sprintf(`from http.server import HTTPServer, SimpleHTTPRequestHandler
import os

PORT = %d

class Handler(SimpleHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200)
        self.send_header('Content-type', 'text/html')
        self.end_headers()
        self.wfile.write(b'<h1>%s</h1><p>Site created via SigardaPanel</p>')

if __name__ == '__main__':
    os.chdir(os.path.dirname(os.path.abspath(__file__)))
    HTTPServer(('127.0.0.1', PORT), Handler).serve_forever()
`, port, domain),
		}
	case "go":
		return map[string]string{
			"go.mod": goModFile(domain),
			"main.go": fmt.Sprintf(`package main

import (
    "fmt"
    "net/http"
)

func main() {
    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        fmt.Fprintf(w, "<h1>%s</h1><p>Site created via SigardaPanel</p>")
    })
    fmt.Println("Server running on port %d")
    http.ListenAndServe(":%d", nil)
}
`, domain, port, port),
		}
	case "docker", "static", "proxy":
		return nil
	default:
		return nil
	}
}

// validateSiteForVhost rejects site values that could break out of their
// nginx directive (newlines, ';', '{', '}', quotes, '$' ...).
func validateSiteForVhost(s *store.Site) error {
	if err := siteinput.Domain(s.Domain); err != nil {
		return err
	}
	if s.RootPath != "" {
		if err := siteinput.RootPath(s.RootPath); err != nil {
			return err
		}
	}
	if s.ProxyPassURL.Valid && s.ProxyPassURL.String != "" {
		if err := siteinput.ProxyPassURL(s.ProxyPassURL.String); err != nil {
			return err
		}
	}
	if s.PHPVersion.Valid && s.PHPVersion.String != "" {
		if err := siteinput.PHPVersion(s.PHPVersion.String); err != nil {
			return err
		}
	}
	if s.PHPIni.Valid {
		if err := siteinput.PHPIni(s.PHPIni.String); err != nil {
			return err
		}
	}
	return nil
}
