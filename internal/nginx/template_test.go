package nginx

import (
	"strings"
	"testing"

	"sigardapanel/internal/store"
)

func TestGenerateVhost_Static(t *testing.T) {
	site := &store.Site{
		Domain:   "static.example.com",
		RootPath: "/home/static.example.com/htdocs/static.example.com",
		Runtime:  "static",
	}
	config, err := GenerateVhost(site, 0, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(config, "server_name static.example.com") {
		t.Error("expected server_name in config")
	}
	if !strings.Contains(config, "root /home/static.example.com/htdocs/static.example.com") {
		t.Error("expected root in config")
	}
}

func TestGenerateVhost_PHP(t *testing.T) {
	site := &store.Site{
		Domain:   "php.example.com",
		RootPath: "/home/php.example.com/htdocs/php.example.com",
		Runtime:  "php",
	}
	config, err := GenerateVhost(site, 0, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(config, "fastcgi_pass") {
		t.Error("expected fastcgi_pass in config")
	}
	if !strings.Contains(config, "index.php") {
		t.Error("expected index.php in config")
	}
}

func TestGenerateVhost_Node(t *testing.T) {
	site := &store.Site{
		Domain:   "node.example.com",
		RootPath: "/home/node.example.com/htdocs/node.example.com",
		Runtime:  "node",
	}
	config, err := GenerateVhost(site, 3000, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(config, "proxy_pass http://127.0.0.1:3000") {
		t.Error("expected proxy_pass in config")
	}
}

func TestGenerateVhost_UnknownRuntime(t *testing.T) {
	site := &store.Site{
		Domain:  "bad.example.com",
		Runtime: "ruby",
	}
	_, err := GenerateVhost(site, 0, nil)
	if err == nil {
		t.Fatal("expected error for unknown runtime")
	}
	if !strings.Contains(err.Error(), "unsupported runtime") {
		t.Fatalf("expected unsupported runtime error, got %v", err)
	}
}

func TestGenerateVhost_ContainsMarker(t *testing.T) {
	site := &store.Site{
		Domain:   "marker.example.com",
		RootPath: "/home/marker.example.com/htdocs/marker.example.com",
		Runtime:  "static",
	}
	config, err := GenerateVhost(site, 0, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(config, markerBegin) {
		t.Error("expected BEGIN marker in config")
	}
	if !strings.Contains(config, markerEnd) {
		t.Error("expected END marker in config")
	}
}

func TestGenerateVhost_DifferentRuntime(t *testing.T) {
	staticSite := &store.Site{
		Domain:   "test.example.com",
		RootPath: "/home/test-site/htdocs/test.example.com",
		Runtime:  "static",
	}
	phpSite := &store.Site{
		Domain:   "test.example.com",
		RootPath: "/home/test-site/htdocs/test.example.com",
		Runtime:  "php",
	}

	staticConfig, err := GenerateVhost(staticSite, 0, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	phpConfig, err := GenerateVhost(phpSite, 0, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if staticConfig == phpConfig {
		t.Fatal("expected different configs for different runtimes")
	}
	if !strings.Contains(staticConfig, "try_files $uri") {
		t.Error("expected static-specific config")
	}
	if !strings.Contains(phpConfig, "fastcgi_pass") {
		t.Error("expected php-specific config")
	}
}
