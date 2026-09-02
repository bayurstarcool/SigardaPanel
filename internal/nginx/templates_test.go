package nginx

import (
	"strings"
	"testing"
)

func TestTemplateForRuntime_Node(t *testing.T) {
	templates := TemplateForRuntime("node", "example.com", 30001)
	if templates == nil {
		t.Fatal("expected non-nil templates for node runtime")
	}
	if _, ok := templates["server.js"]; !ok {
		t.Fatal("expected server.js in templates")
	}
	content := templates["server.js"]
	if !strings.Contains(content, "30001") {
		t.Error("expected port 30001 in server.js")
	}
	if !strings.Contains(content, "example.com") {
		t.Error("expected domain in server.js")
	}
	if !strings.Contains(content, "require('http')") {
		t.Error("expected http module import")
	}
}

func TestTemplateForRuntime_Python(t *testing.T) {
	templates := TemplateForRuntime("python", "myapp.com", 30002)
	if templates == nil {
		t.Fatal("expected non-nil templates for python runtime")
	}
	if _, ok := templates["app.py"]; !ok {
		t.Fatal("expected app.py in templates")
	}
	content := templates["app.py"]
	if !strings.Contains(content, "30002") {
		t.Error("expected port 30002 in app.py")
	}
	if !strings.Contains(content, "myapp.com") {
		t.Error("expected domain in app.py")
	}
}

func TestTemplateForRuntime_Go(t *testing.T) {
	templates := TemplateForRuntime("go", "goservice.io", 30003)
	if templates == nil {
		t.Fatal("expected non-nil templates for go runtime")
	}
	if _, ok := templates["main.go"]; !ok {
		t.Fatal("expected main.go in templates")
	}
	if _, ok := templates["go.mod"]; !ok {
		t.Fatal("expected go.mod in templates")
	}
	content := templates["main.go"]
	if !strings.Contains(content, "30003") {
		t.Error("expected port 30003 in main.go")
	}
	if !strings.Contains(content, "goservice.io") {
		t.Error("expected domain in main.go")
	}
	modContent := templates["go.mod"]
	if !strings.Contains(modContent, "goservice.io") {
		t.Error("expected domain in go.mod")
	}
}

func TestTemplateForRuntime_Static(t *testing.T) {
	templates := TemplateForRuntime("static", "static.com", 8080)
	if templates != nil {
		t.Error("expected nil templates for static runtime")
	}
}

func TestTemplateForRuntime_Proxy(t *testing.T) {
	templates := TemplateForRuntime("proxy", "proxy.com", 8080)
	if templates != nil {
		t.Error("expected nil templates for proxy runtime")
	}
}

func TestItoa(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{0, "0"},
		{1, "1"},
		{5009, "5009"},
		{30000, "30000"},
		{-1, "-1"},
	}
	for _, tt := range tests {
		if got := itoa(tt.input); got != tt.expected {
			t.Errorf("itoa(%d) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}
