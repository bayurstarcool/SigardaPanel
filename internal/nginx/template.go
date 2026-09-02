package nginx

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"text/template"

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

    ssl_certificate /etc/nginx/ssl/sigardapanel.crt;
    ssl_certificate_key /etc/nginx/ssl/sigardapanel.key;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers HIGH:!aNULL:!MD5;

    # ACME challenge for Let's Encrypt (future use)
    location /.well-known/acme-challenge/ {
        root /var/www/certbot/{{.Domain}};
    }

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

    ssl_certificate /etc/nginx/ssl/sigardapanel.crt;
    ssl_certificate_key /etc/nginx/ssl/sigardapanel.key;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers HIGH:!aNULL:!MD5;

    # ACME challenge for Let's Encrypt
    location /.well-known/acme-challenge/ {
        root /var/www/certbot/{{.Domain}};
    }

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

    ssl_certificate /etc/nginx/ssl/sigardapanel.crt;
    ssl_certificate_key /etc/nginx/ssl/sigardapanel.key;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers HIGH:!aNULL:!MD5;

    # ACME challenge for Let's Encrypt
    location /.well-known/acme-challenge/ {
        root /var/www/certbot/{{.Domain}};
    }

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

    ssl_certificate /etc/nginx/ssl/sigardapanel.crt;
    ssl_certificate_key /etc/nginx/ssl/sigardapanel.key;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers HIGH:!aNULL:!MD5;

    # ACME challenge for Let's Encrypt
    location /.well-known/acme-challenge/ {
        root /var/www/certbot/{{.Domain}};
    }

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
}

// phpFpmAddr returns the TCP address for a specific PHP version.
func phpFpmAddr(version string) string {
	// Map PHP versions to TCP ports (ondrej/php PPA convention)
	portMap := map[string]int{
		"7.1": 11000, "7.2": 12000, "7.3": 13000, "7.4": 14000,
		"8.0": 15000, "8.1": 16000, "8.2": 17000, "8.3": 18000,
		"8.4": 19000, "8.5": 20000,
	}
	if p, ok := portMap[version]; ok {
		// Check if PHP-FPM is actually listening on TCP, otherwise use Unix socket
		socketPath := fmt.Sprintf("/run/php/php%s-fpm.sock", version)
		if _, err := os.Stat(socketPath); err == nil {
			return fmt.Sprintf("unix:%s", socketPath)
		}
		return fmt.Sprintf("127.0.0.1:%d", p)
	}
	// Default: try Unix socket for common versions
	for _, v := range []string{"8.3", "8.4", "8.2", "8.1", "8.0"} {
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
	tmplStr := templates[s.Runtime]
	if tmplStr == "" {
		return "", fmt.Errorf("unsupported runtime %q", s.Runtime)
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
		Domain:           s.Domain,
		RootPath:         s.RootPath,
		Runtime:          s.Runtime,
		Port:             port,
		ProxyPassURL:     proxyPassURL,
		PHPFpmPass:       phpFpmPass,
		PHPIniDirectives: phpIni,
		MarkerBegin:      markerBegin,
		MarkerEnd:        markerEnd,
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
