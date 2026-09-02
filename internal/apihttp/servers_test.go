package apihttp

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"sigardapanel/internal/auth"
	"sigardapanel/internal/store"
)

func newAdminToken(t *testing.T, s *store.Store, ctx context.Context) string {
	t.Helper()
	user := &store.User{
		Username:     "admin",
		Email:        "admin@example.com",
		PasswordHash: "password123",
		Role:         auth.RoleAdmin,
	}
	if err := s.Users.Create(ctx, user); err != nil {
		t.Fatalf("create admin user: %v", err)
	}
	token, err := auth.NewToken()
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	if err := s.Sessions.Create(ctx, &store.Session{
		UserID:    user.ID,
		TokenHash: auth.HashToken(token),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	return token
}

func newUserToken(t *testing.T, s *store.Store, ctx context.Context) string {
	t.Helper()
	user := &store.User{
		Username:     "regular",
		Email:        "user@example.com",
		PasswordHash: "password123",
		Role:         auth.RoleUser,
	}
	if err := s.Users.Create(ctx, user); err != nil {
		t.Fatalf("create regular user: %v", err)
	}
	token, err := auth.NewToken()
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	if err := s.Sessions.Create(ctx, &store.Session{
		UserID:    user.ID,
		TokenHash: auth.HashToken(token),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	return token
}

func TestServerList_Empty(t *testing.T) {
	e, s, ctx := newTestApp(t)
	token := newAdminToken(t, s, ctx)

	rec := doRequest(t, e, "GET", "/api/v1/servers", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var result struct {
		Data struct {
			Servers []map[string]any `json:"servers"`
			Count   int              `json:"count"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Data.Count != 0 {
		t.Fatalf("expected count 0, got %d", result.Data.Count)
	}
	if len(result.Data.Servers) != 0 {
		t.Fatalf("expected empty servers, got %d", len(result.Data.Servers))
	}
}

func TestServerList_WithServers(t *testing.T) {
	e, s, ctx := newTestApp(t)
	token := newAdminToken(t, s, ctx)

	for i := 0; i < 3; i++ {
		server := &store.Server{
			Name:       "server-" + string(rune('a'+i)),
			Hostname:   "host-" + string(rune('a'+i)) + ".example.com",
			Status:     "active",
			AgentToken: "token-" + string(rune('a'+i)),
		}
		if err := s.Servers.Create(ctx, server); err != nil {
			t.Fatalf("create server %d: %v", i, err)
		}
	}

	rec := doRequest(t, e, "GET", "/api/v1/servers", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var result struct {
		Data struct {
			Servers []map[string]any `json:"servers"`
			Count   int              `json:"count"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Data.Count != 3 {
		t.Fatalf("expected count 3, got %d", result.Data.Count)
	}
	if len(result.Data.Servers) != 3 {
		t.Fatalf("expected 3 servers, got %d", len(result.Data.Servers))
	}
	for _, srv := range result.Data.Servers {
		if _, ok := srv["agent_token"]; ok {
			t.Fatal("agent_token should not be present in list response")
		}
	}
}

func TestServerCreate_Success(t *testing.T) {
	e, s, ctx := newTestApp(t)
	token := newAdminToken(t, s, ctx)

	body := map[string]string{
		"name":       "prod-01",
		"hostname":   "prod-01.example.com",
		"ip_address": "192.168.1.1",
	}
	rec := doRequest(t, e, "POST", "/api/v1/servers", body, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var result struct {
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Data["name"] != "prod-01" {
		t.Fatalf("expected name prod-01, got %v", result.Data["name"])
	}
	if result.Data["agent_token"] == "" {
		t.Fatal("expected agent_token in create response")
	}
}

func TestServerCreate_TokenOnlyInCreate(t *testing.T) {
	e, s, ctx := newTestApp(t)
	token := newAdminToken(t, s, ctx)

	body := map[string]string{
		"name":     "token-test",
		"hostname": "token.example.com",
	}
	rec := doRequest(t, e, "POST", "/api/v1/servers", body, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}

	var createResult struct {
		Data map[string]any `json:"data"`
	}
	json.NewDecoder(rec.Body).Decode(&createResult)
	if createResult.Data["agent_token"] == "" {
		t.Fatal("expected agent_token in create response")
	}

	id := int64(createResult.Data["id"].(float64))
	rec2 := doRequest(t, e, "GET", "/api/v1/servers/"+strconv.FormatInt(id, 10), nil, token)
	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec2.Code)
	}

	var getResult struct {
		Data map[string]any `json:"data"`
	}
	json.NewDecoder(rec2.Body).Decode(&getResult)
	if _, ok := getResult.Data["agent_token"]; ok {
		t.Fatal("agent_token should not be present in get response")
	}
}

func TestServerCreate_InvalidName(t *testing.T) {
	e, s, ctx := newTestApp(t)
	token := newAdminToken(t, s, ctx)

	body := map[string]string{
		"name":     "prod@01!",
		"hostname": "prod-01.example.com",
	}
	rec := doRequest(t, e, "POST", "/api/v1/servers", body, token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestServerCreate_InvalidHostname(t *testing.T) {
	e, s, ctx := newTestApp(t)
	token := newAdminToken(t, s, ctx)

	body := map[string]string{
		"name":     "prod-01",
		"hostname": "not a valid hostname!",
	}
	rec := doRequest(t, e, "POST", "/api/v1/servers", body, token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestServerCreate_InvalidIP(t *testing.T) {
	e, s, ctx := newTestApp(t)
	token := newAdminToken(t, s, ctx)

	body := map[string]string{
		"name":       "prod-01",
		"hostname":   "prod-01.example.com",
		"ip_address": "not-an-ip",
	}
	rec := doRequest(t, e, "POST", "/api/v1/servers", body, token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestServerCreate_NonAdmin(t *testing.T) {
	e, s, ctx := newTestApp(t)
	token := newUserToken(t, s, ctx)

	body := map[string]string{
		"name":     "prod-01",
		"hostname": "prod-01.example.com",
	}
	rec := doRequest(t, e, "POST", "/api/v1/servers", body, token)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestServerCreate_RequiresAuth(t *testing.T) {
	e, _, _ := newTestApp(t)

	body := map[string]string{
		"name":     "prod-01",
		"hostname": "prod-01.example.com",
	}
	rec := doRequest(t, e, "POST", "/api/v1/servers", body, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestServerGet_Found(t *testing.T) {
	e, s, ctx := newTestApp(t)
	token := newAdminToken(t, s, ctx)

	server := &store.Server{
		Name:       "get-test",
		Hostname:   "get.example.com",
		Status:     "active",
		AgentToken: "token",
	}
	if err := s.Servers.Create(ctx, server); err != nil {
		t.Fatalf("create server: %v", err)
	}

	rec := doRequest(t, e, "GET", "/api/v1/servers/"+strconv.FormatInt(server.ID, 10), nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var result struct {
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Data["name"] != "get-test" {
		t.Fatalf("expected name get-test, got %v", result.Data["name"])
	}
	if _, ok := result.Data["agent_token"]; ok {
		t.Fatal("agent_token should not be present in get response")
	}
}

func TestServerGet_NotFound(t *testing.T) {
	e, s, ctx := newTestApp(t)
	token := newAdminToken(t, s, ctx)

	rec := doRequest(t, e, "GET", "/api/v1/servers/99999", nil, token)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestServerGet_InvalidID(t *testing.T) {
	e, s, ctx := newTestApp(t)
	token := newAdminToken(t, s, ctx)

	rec := doRequest(t, e, "GET", "/api/v1/servers/abc", nil, token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestServerDelete_Success(t *testing.T) {
	e, s, ctx := newTestApp(t)
	token := newAdminToken(t, s, ctx)

	server := &store.Server{
		Name:       "delete-test",
		Hostname:   "delete.example.com",
		Status:     "active",
		AgentToken: "token",
	}
	if err := s.Servers.Create(ctx, server); err != nil {
		t.Fatalf("create server: %v", err)
	}

	rec := doRequest(t, e, "DELETE", "/api/v1/servers/"+strconv.FormatInt(server.ID, 10), nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var result struct {
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Data["deleted"] != true {
		t.Fatalf("expected deleted true, got %v", result.Data["deleted"])
	}
}

func TestServerDelete_NotFound(t *testing.T) {
	e, s, ctx := newTestApp(t)
	token := newAdminToken(t, s, ctx)

	rec := doRequest(t, e, "DELETE", "/api/v1/servers/99999", nil, token)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestServerDelete_NonAdmin(t *testing.T) {
	e, s, ctx := newTestApp(t)
	_ = newAdminToken(t, s, ctx)
	userToken := newUserToken(t, s, ctx)

	server := &store.Server{
		Name:       "delete-test",
		Hostname:   "delete.example.com",
		Status:     "active",
		AgentToken: "token",
	}
	if err := s.Servers.Create(ctx, server); err != nil {
		t.Fatalf("create server: %v", err)
	}

	rec := doRequest(t, e, "DELETE", "/api/v1/servers/"+strconv.FormatInt(server.ID, 10), nil, userToken)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestServerList_NoToken(t *testing.T) {
	e, s, ctx := newTestApp(t)
	token := newAdminToken(t, s, ctx)

	server := &store.Server{
		Name:       "list-test",
		Hostname:   "list.example.com",
		Status:     "active",
		AgentToken: "secret-token-123",
	}
	if err := s.Servers.Create(ctx, server); err != nil {
		t.Fatalf("create server: %v", err)
	}

	rec := doRequest(t, e, "GET", "/api/v1/servers", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var result struct {
		Data struct {
			Servers []map[string]any `json:"servers"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(result.Data.Servers) == 0 {
		t.Fatal("expected at least one server in list")
	}
	for _, srv := range result.Data.Servers {
		if _, ok := srv["agent_token"]; ok {
			t.Fatal("agent_token should not be present in list response")
		}
	}
}
