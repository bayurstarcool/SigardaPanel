package selfupdate

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewer(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"v1.6.3", "1.6.2", true},
		{"v1.10.0", "1.9.9", true},
		{"v2.0.0", "v1.99.99", true},
		{"v1.6.2", "1.6.2", false},
		{"v1.6.1", "1.6.2", false},
		{"v1.6.3", "dev", false},
		{"latest", "1.6.2", false},
		{"v1.6.3-rc1", "1.6.2", false},
	}
	for _, c := range cases {
		if got := Newer(c.a, c.b); got != c.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestValidTag(t *testing.T) {
	for _, ok := range []string{"v1.0.0", "v12.3.45"} {
		if !ValidTag(ok) {
			t.Errorf("ValidTag(%q) = false", ok)
		}
	}
	for _, bad := range []string{"1.0.0", "v1.0", "v1.0.0;rm -rf /", "v01.0.0", "v1.0.0\n", ""} {
		if ValidTag(bad) {
			t.Errorf("ValidTag(%q) = true", bad)
		}
	}
}

func TestCheckerCachesAndParses(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path != "/repos/o/r/releases/latest" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(`{"tag_name":"v1.7.0","name":"SigardaPanel v1.7.0","body":"notes","html_url":"https://x/y","published_at":"2026-11-01T10:00:00Z"}`))
	}))
	defer srv.Close()
	c := &Checker{HTTP: srv.Client(), APIBase: srv.URL, Repo: "o/r"}
	rel, _, err := c.Latest(context.Background(), time.Hour)
	if err != nil || rel == nil || rel.Tag != "v1.7.0" || rel.Notes != "notes" {
		t.Fatalf("Latest = %+v, %v", rel, err)
	}
	// A fresh request inside MinInterval is served from the cache.
	if _, _, err := c.Latest(context.Background(), 0); err != nil || hits.Load() != 1 {
		t.Fatalf("expected one lookup, got %d (%v)", hits.Load(), err)
	}
}

func TestCheckerRejectsBadTag(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"tag_name":"v1.7.0; echo pwned"}`))
	}))
	defer srv.Close()
	c := &Checker{HTTP: srv.Client(), APIBase: srv.URL, Repo: "o/r"}
	if rel, _, err := c.Latest(context.Background(), time.Hour); err == nil || rel != nil {
		t.Fatalf("expected an error, got %+v", rel)
	}
}
