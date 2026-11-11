package agenthttp

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

type suspendFixture struct {
	e        *echo.Echo
	enabled  string // sites-enabled/<domain>.conf
	disabled string // sites-disabled/<domain>.conf
	renewal  string
	le       string
}

const suspendDomain = "sus.example.com"

const originalVhost = `server {
    listen 80;
    server_name sus.example.com www.sus.example.com;
    root /home/u/htdocs/sus.example.com;
}
server {
    listen 443 ssl;
    server_name sus.example.com ~^weird$ _;
}
`

func newSuspendFixture(t *testing.T) *suspendFixture {
	t.Helper()
	setupTestBinaries(t)
	base := t.TempDir()
	enabledDir := filepath.Join(base, "nginx", "sites-enabled")
	le := filepath.Join(base, "letsencrypt")
	for _, d := range []string{enabledDir, filepath.Join(le, "renewal")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	f := &suspendFixture{
		e:        NewRouterWithOptions(RouterOptions{NginxConfDir: enabledDir, LetsEncryptDir: le}),
		enabled:  filepath.Join(enabledDir, suspendDomain+".conf"),
		disabled: filepath.Join(base, "nginx", "sites-disabled", suspendDomain+".conf"),
		renewal:  filepath.Join(le, "renewal", suspendDomain+".conf"),
		le:       le,
	}
	if err := os.WriteFile(f.enabled, []byte(originalVhost), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.renewal, []byte("[renewalparams]"), 0o644); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *suspendFixture) call(t *testing.T, action string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/tasks/site/"+action,
		bytes.NewReader([]byte(`{"site_id":7,"domain":"`+suspendDomain+`"}`)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	f.e.ServeHTTP(rec, req)
	return rec
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	return string(b)
}

// failNginxTest puts an nginx in front of the fake one whose "-t" fails.
func failNginxTest(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\nif [ \"$1\" = \"-t\" ]; then echo 'nginx: [emerg] test failure'; exit 1; fi\nexit 0\n"
	if err := os.WriteFile(filepath.Join(dir, "nginx"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestSuspend_WritesPlaceholderReturning503(t *testing.T) {
	f := newSuspendFixture(t)
	if rec := f.call(t, "suspend"); rec.Code != http.StatusOK {
		t.Fatalf("suspend: %d %s", rec.Code, rec.Body.String())
	}
	if got := readFile(t, f.disabled); got != originalVhost {
		t.Fatalf("original vhost not kept in sites-disabled: %q", got)
	}
	p := readFile(t, f.enabled)
	if !strings.HasPrefix(p, suspendedPlaceholderMarker+"\n") {
		t.Fatalf("placeholder must start with the marker:\n%s", p)
	}
	for _, want := range []string{
		"listen 80;",
		"listen [::]:80;",
		"server_name sus.example.com www.sus.example.com;",
		"default_type text/html;",
		"return 503 \"<!doctype html>",
		"This site is temporarily suspended",
	} {
		if !strings.Contains(p, want) {
			t.Fatalf("placeholder lacks %q:\n%s", want, p)
		}
	}
	// No certificate: no HTTPS server; regex/catch-all names never copied.
	for _, bad := range []string{"443", "ssl_certificate", "~^weird", " _;"} {
		if strings.Contains(p, bad) {
			t.Fatalf("placeholder must not contain %q:\n%s", bad, p)
		}
	}
	// The inline page must be a valid nginx string: no quotes, variables or
	// backslashes inside it.
	if strings.ContainsAny(suspendedPageHTML, "\"$\\") {
		t.Fatal("suspended page contains characters that break the nginx string")
	}
}

func TestSuspend_PlaceholderServesHTTPSWithExistingCertificate(t *testing.T) {
	f := newSuspendFixture(t)
	live := filepath.Join(f.le, "live", suspendDomain)
	if err := os.MkdirAll(live, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"fullchain.pem", "privkey.pem"} {
		if err := os.WriteFile(filepath.Join(live, n), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if rec := f.call(t, "suspend"); rec.Code != http.StatusOK {
		t.Fatalf("suspend: %d %s", rec.Code, rec.Body.String())
	}
	p := readFile(t, f.enabled)
	for _, want := range []string{
		"listen 443 ssl;",
		"listen [::]:443 ssl;",
		"ssl_certificate " + filepath.Join(live, "fullchain.pem") + ";",
		"ssl_certificate_key " + filepath.Join(live, "privkey.pem") + ";",
	} {
		if !strings.Contains(p, want) {
			t.Fatalf("placeholder lacks %q:\n%s", want, p)
		}
	}
}

func TestSuspend_IdempotentAndResumeRestoresOriginal(t *testing.T) {
	f := newSuspendFixture(t)
	for i := 0; i < 2; i++ {
		if rec := f.call(t, "suspend"); rec.Code != http.StatusOK {
			t.Fatalf("suspend #%d: %d %s", i+1, rec.Code, rec.Body.String())
		}
	}
	// The second suspend must not have moved the placeholder over the original.
	if got := readFile(t, f.disabled); got != originalVhost {
		t.Fatalf("sites-disabled no longer holds the original: %q", got)
	}
	if rec := f.call(t, "resume"); rec.Code != http.StatusOK {
		t.Fatalf("resume: %d %s", rec.Code, rec.Body.String())
	}
	if got := readFile(t, f.enabled); got != originalVhost {
		t.Fatalf("original vhost not restored: %q", got)
	}
	if _, err := os.Stat(f.disabled); !os.IsNotExist(err) {
		t.Fatal("sites-disabled copy should be gone after resume")
	}
	if got := readFile(t, f.renewal); got != "[renewalparams]" {
		t.Fatalf("renewal not restored: %q", got)
	}
	// Resuming again is a no-op.
	if rec := f.call(t, "resume"); rec.Code != http.StatusOK {
		t.Fatalf("second resume: %d %s", rec.Code, rec.Body.String())
	}
	if got := readFile(t, f.enabled); got != originalVhost {
		t.Fatalf("second resume changed the vhost: %q", got)
	}
}

func TestResume_NeverRemovesAVhostWithoutTheMarker(t *testing.T) {
	f := newSuspendFixture(t)
	if rec := f.call(t, "suspend"); rec.Code != http.StatusOK {
		t.Fatalf("suspend: %d", rec.Code)
	}
	// Someone replaced the placeholder with a real config meanwhile.
	custom := "server { server_name sus.example.com; }\n"
	if err := os.WriteFile(f.enabled, []byte(custom), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := f.call(t, "resume")
	if rec.Code == http.StatusOK {
		t.Fatalf("resume must refuse to overwrite a foreign vhost, got 200 %s", rec.Body.String())
	}
	if got := readFile(t, f.enabled); got != custom {
		t.Fatalf("foreign vhost was touched: %q", got)
	}
	if got := readFile(t, f.disabled); got != originalVhost {
		t.Fatalf("original lost: %q", got)
	}
}

func TestSuspend_RollsBackWhenNginxTestFails(t *testing.T) {
	f := newSuspendFixture(t)
	failNginxTest(t)
	rec := f.call(t, "suspend")
	if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), "rolled back") {
		t.Fatalf("expected a rolled-back 500, got %d %s", rec.Code, rec.Body.String())
	}
	if got := readFile(t, f.enabled); got != originalVhost {
		t.Fatalf("original vhost not back in sites-enabled: %q", got)
	}
	if _, err := os.Stat(f.disabled); !os.IsNotExist(err) {
		t.Fatal("sites-disabled copy left behind")
	}
	if got := readFile(t, f.renewal); got != "[renewalparams]" {
		t.Fatalf("renewal not rolled back: %q", got)
	}
}

func TestResume_RollsBackWhenNginxTestFails(t *testing.T) {
	f := newSuspendFixture(t)
	if rec := f.call(t, "suspend"); rec.Code != http.StatusOK {
		t.Fatalf("suspend: %d", rec.Code)
	}
	placeholder := readFile(t, f.enabled)
	failNginxTest(t)
	rec := f.call(t, "resume")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d %s", rec.Code, rec.Body.String())
	}
	if got := readFile(t, f.enabled); got != placeholder {
		t.Fatalf("placeholder not restored after failed resume: %q", got)
	}
	if got := readFile(t, f.disabled); got != originalVhost {
		t.Fatalf("original not back in sites-disabled: %q", got)
	}
}

func TestSuspend_SiteWithoutVhostStillGetsPlaceholder(t *testing.T) {
	f := newSuspendFixture(t)
	if err := os.Remove(f.enabled); err != nil {
		t.Fatal(err)
	}
	if rec := f.call(t, "suspend"); rec.Code != http.StatusOK {
		t.Fatalf("suspend: %d %s", rec.Code, rec.Body.String())
	}
	p := readFile(t, f.enabled)
	if !isSuspendedPlaceholder([]byte(p)) || !strings.Contains(p, "server_name sus.example.com;") {
		t.Fatalf("unexpected placeholder:\n%s", p)
	}
	// Resume removes it and leaves nothing behind.
	if rec := f.call(t, "resume"); rec.Code != http.StatusOK {
		t.Fatalf("resume: %d %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(f.enabled); !os.IsNotExist(err) {
		t.Fatal("placeholder left behind after resume")
	}
}
