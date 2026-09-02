package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClient_Get_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"data":       map[string]any{"hello": "world"},
			"request_id": "req_test",
		})
	}))
	defer server.Close()

	client := &Client{
		BaseURL: server.URL,
		HTTP:    &http.Client{Timeout: 5 * time.Second},
	}

	resp, status, err := client.Get("/api/test")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if status != 200 {
		t.Fatalf("expected 200, got %d", status)
	}
	if resp.RequestID != "req_test" {
		t.Errorf("expected request_id, got %q", resp.RequestID)
	}
	var data map[string]string
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data["hello"] != "world" {
		t.Errorf("expected hello=world, got %v", data)
	}
}

func TestClient_Get_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(404)
		json.NewEncoder(w).Encode(map[string]any{
			"error":      map[string]any{"code": "not_found", "message": "missing"},
			"request_id": "req_x",
		})
	}))
	defer server.Close()

	client := &Client{BaseURL: server.URL, HTTP: http.DefaultClient}
	resp, status, err := client.Get("/missing")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if status != 404 {
		t.Errorf("expected 404, got %d", status)
	}
	if resp.Error == nil || resp.Error.Code != "not_found" {
		t.Errorf("expected error code not_found, got %+v", resp.Error)
	}
}

func TestClient_Post_WithBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected application/json, got %s", r.Header.Get("Content-Type"))
		}
		body, _ := io.ReadAll(r.Body)
		var got map[string]string
		json.Unmarshal(body, &got)
		if got["name"] != "test" {
			t.Errorf("expected name=test, got %s", got["name"])
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": 1}, "request_id": "r"})
	}))
	defer server.Close()

	client := &Client{BaseURL: server.URL, HTTP: http.DefaultClient}
	resp, status, err := client.Post("/api/test", map[string]string{"name": "test"})
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	if status != 201 {
		t.Errorf("expected 201, got %d", status)
	}
	if resp == nil {
		t.Fatal("nil response")
	}
}

func TestClient_TokenAttached(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-token" {
			t.Errorf("expected Bearer test-token, got %s", auth)
		}
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]any{"data": nil, "request_id": "r"})
	}))
	defer server.Close()

	client := &Client{BaseURL: server.URL, Token: "test-token", HTTP: http.DefaultClient}
	_, _, _ = client.Get("/api/test")
}

func TestClient_Delete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"deleted": true}, "request_id": "r"})
	}))
	defer server.Close()

	client := &Client{BaseURL: server.URL, HTTP: http.DefaultClient}
	_, status, err := client.Delete("/api/test/1")
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if status != 200 {
		t.Errorf("expected 200, got %d", status)
	}
}