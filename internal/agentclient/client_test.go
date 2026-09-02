package agentclient

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExecute_Success(t *testing.T) {
	var gotPath string
	var gotBody string
	var gotAuth string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(TaskResult{OK: true, Output: "done"})
	}))
	defer ts.Close()

	c := New(ts.URL, "test-token")
	result, err := c.Execute(context.Background(), "site.create", `{"domain":"x.com"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.OK {
		t.Fatal("expected OK result")
	}
	if result.Output != "done" {
		t.Fatalf("expected output done, got %s", result.Output)
	}
	if gotPath != "/tasks/site/create" {
		t.Fatalf("unexpected path: %s", gotPath)
	}
	if gotBody != `{"domain":"x.com"}` {
		t.Fatalf("unexpected body: %s", gotBody)
	}
	if gotAuth != "Bearer test-token" {
		t.Fatalf("unexpected auth header: %s", gotAuth)
	}
}

func TestExecute_NotImplemented(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotImplemented)
		json.NewEncoder(w).Encode(TaskResult{OK: false})
	}))
	defer ts.Close()

	c := New(ts.URL, "token")
	result, err := c.Execute(context.Background(), "site.delete", `{}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.OK {
		t.Fatal("expected not OK")
	}
	if result.Error == "" {
		t.Fatal("expected error message")
	}
}

func TestExecute_ServerError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(TaskResult{OK: false, Error: "boom"})
	}))
	defer ts.Close()

	c := New(ts.URL, "token")
	result, err := c.Execute(context.Background(), "ssl.issue", `{}`)
	if err == nil {
		t.Fatal("expected error for 500")
	}
	if result == nil {
		t.Fatal("expected result even on error")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected agent error detail, got %v", err)
	}
}

func TestExecute_ServerError_EchoMessageRedactsSecrets(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"postgres dump failed: password=top-secret"}`))
	}))
	defer ts.Close()

	c := New(ts.URL, "token")
	_, err := c.Execute(context.Background(), "db_backup.create", `{}`)
	if err == nil {
		t.Fatal("expected error for 500")
	}
	if !strings.Contains(err.Error(), "postgres dump failed") {
		t.Fatalf("expected Echo message, got %v", err)
	}
	if strings.Contains(err.Error(), "top-secret") {
		t.Fatalf("expected secret redaction, got %v", err)
	}
	if !strings.Contains(err.Error(), "password=[REDACTED]") {
		t.Fatalf("expected redacted secret, got %v", err)
	}
}

func TestExecute_EndpointMapping(t *testing.T) {
	tests := []struct {
		jobType  string
		expected string
	}{
		{"site.create", "/tasks/site/create"},
		{"site.delete", "/tasks/site/delete"},
		{"site.deploy", "/tasks/site/deploy"},
		{"ssl.issue", "/tasks/ssl/issue"},
		{"ssl.renew", "/tasks/ssl/renew"},
		{"backup.create", "/tasks/backup/create"},
	}

	for _, tt := range tests {
		var gotPath string
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			json.NewEncoder(w).Encode(TaskResult{OK: true})
		}))

		c := New(ts.URL, "token")
		_, err := c.Execute(context.Background(), tt.jobType, `{}`)
		if err != nil {
			t.Fatalf("unexpected error for %s: %v", tt.jobType, err)
		}
		if gotPath != tt.expected {
			t.Fatalf("type %s: expected path %s, got %s", tt.jobType, tt.expected, gotPath)
		}
		ts.Close()
	}
}
