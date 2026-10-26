package apihttp

import (
	"encoding/json"
	"net/http"
	"testing"

	"sigardapanel/internal/buildinfo"
)

// The API must report the version stamped into the binary at release time,
// not a constant that silently drifts from it.
func TestVersionEndpointReportsBuildVersion(t *testing.T) {
	orig := buildinfo.Version
	buildinfo.Version = "9.8.7-test"
	t.Cleanup(func() { buildinfo.Version = orig })

	e, _, _ := newTestApp(t)
	rec := doRequest(t, e, http.MethodGet, "/api/v1/version", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Data struct {
			Service string `json:"service"`
			Version string `json:"version"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Data.Service != "sigardapanel-api" {
		t.Fatalf("expected service sigardapanel-api, got %q", resp.Data.Service)
	}
	if resp.Data.Version != "9.8.7-test" {
		t.Fatalf("expected version 9.8.7-test, got %q", resp.Data.Version)
	}
}
