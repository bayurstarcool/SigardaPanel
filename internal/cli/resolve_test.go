package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResolveToken_EnvOverridesFile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("SIGARDAPANEL_TOKEN", "")
	if tok, src := ResolveToken(); tok != "" || src != "" {
		t.Fatalf("expected no token, got %q from %q", tok, src)
	}
	if err := SaveToken("file-token\n"); err != nil {
		t.Fatal(err)
	}
	if tok, src := ResolveToken(); tok != "file-token" || src != "file" {
		t.Fatalf("expected file-token from file, got %q from %q", tok, src)
	}
	t.Setenv("SIGARDAPANEL_TOKEN", "  env-key ")
	if tok, src := ResolveToken(); tok != "env-key" || src != "env" {
		t.Fatalf("expected env-key from env, got %q from %q", tok, src)
	}
}

func TestResolveAPIURL_Precedence(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("SIGARDAPANEL_API_URL", "")
	if got := ResolveAPIURL(); got != DefaultAPIURL {
		t.Fatalf("default: got %q", got)
	}
	if err := SaveConfig(&Config{APIURL: "https://file.example/"}); err != nil {
		t.Fatal(err)
	}
	if got := ResolveAPIURL(); got != "https://file.example" {
		t.Fatalf("file: got %q", got)
	}
	t.Setenv("SIGARDAPANEL_API_URL", "https://env.example")
	if got := ResolveAPIURL(); got != "https://env.example" {
		t.Fatalf("env should win over file: got %q", got)
	}
}

func TestNewClient_SendsEnvToken(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{}})
	}))
	defer srv.Close()
	SaveToken("file-token")
	t.Setenv("SIGARDAPANEL_API_URL", srv.URL)
	t.Setenv("SIGARDAPANEL_TOKEN", "ci-api-key")
	c, _ := NewClient()
	if c.TokenSource != "env" {
		t.Fatalf("expected env token source, got %q", c.TokenSource)
	}
	if _, _, err := c.Get("/api/v1/auth/me"); err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer ci-api-key" {
		t.Fatalf("expected env key to be sent, got %q", gotAuth)
	}
}

func TestClient_NonJSONErrorBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(502)
		w.Write([]byte("<html>Bad Gateway</html>"))
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL}
	resp, status, err := c.Get("/x")
	if err != nil {
		t.Fatal(err)
	}
	if status != 502 || resp.Error == nil || !strings.Contains(resp.Error.Message, "Bad Gateway") {
		t.Fatalf("expected synthesized error, got %d %+v", status, resp.Error)
	}
}

func TestResolveOutputMode(t *testing.T) {
	t.Setenv("SIGARDAPANEL_OUTPUT", "yaml")
	if m, err := ResolveOutputMode(""); err != nil || m != OutputYAML {
		t.Fatalf("env yaml: %v %v", m, err)
	}
	if m, err := ResolveOutputMode("json"); err != nil || m != OutputJSON {
		t.Fatalf("flag json: %v %v", m, err)
	}
	if _, err := ResolveOutputMode("xml"); err == nil {
		t.Fatal("expected error for xml")
	}
	t.Setenv("SIGARDAPANEL_OUTPUT", "bogus")
	if m, _ := ResolveOutputMode(""); m != OutputTable {
		t.Fatalf("bogus env should fall back to table, got %v", m)
	}
}

func TestPrintYAML(t *testing.T) {
	var buf bytes.Buffer
	data := []map[string]any{
		{"id": 1, "domain": "a.test", "status": "active", "ssl_expires_at": "2026-12-01T00:00:00Z",
			"note": "yes", "empty": "", "nested": map[string]any{"k": []any{"x", 2}}, "none": nil, "list": []any{}},
		{"id": 2, "message": "line1\nline2: x"},
	}
	if err := PrintYAML(&buf, data); err != nil {
		t.Fatal(err)
	}
	want := `- domain: a.test
  empty: ""
  id: 1
  list: []
  nested:
    k:
      - x
      - 2
  none: null
  note: "yes"
  ssl_expires_at: "2026-12-01T00:00:00Z"
  status: active
- id: 2
  message: "line1\nline2: x"
`
	if buf.String() != want {
		t.Fatalf("yaml mismatch:\n%s\nwant:\n%s", buf.String(), want)
	}
}

func TestPrintYAML_EmptyList(t *testing.T) {
	var buf bytes.Buffer
	PrintYAML(&buf, []any{})
	if buf.String() != "[]\n" {
		t.Fatalf("got %q", buf.String())
	}
}

func TestCell(t *testing.T) {
	cases := map[string]any{"-": nil, "5": float64(5), "1.5": 1.5, "yes": true, "abc": "abc"}
	for want, in := range cases {
		if got := Cell(in); got != want {
			t.Errorf("Cell(%v)=%q want %q", in, got, want)
		}
	}
	if Cell("") != "-" {
		t.Error("empty string should be -")
	}
}
