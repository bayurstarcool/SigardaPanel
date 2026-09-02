package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

// mockStore is an in-memory implementation of AuditLogStore for testing.
type mockStore []Entry

func (m *mockStore) List(ctx context.Context, filter AuditLogFilter) ([]Entry, error) {
	var out []Entry
	for _, e := range *m {
		if filter.UserID > 0 && e.UserID.Int64 != filter.UserID {
			continue
		}
		if filter.Resource != "" && e.Resource != filter.Resource {
			continue
		}
		if filter.Action != "" && e.Action != filter.Action {
			continue
		}
		out = append(out, e)
	}
	// Apply offset/limit to filtered results
	start := filter.Offset
	if start > len(out) {
		start = len(out)
	}
	end := start + filter.Limit
	if end > len(out) || filter.Limit <= 0 {
		end = len(out)
	}
	return out[start:end], nil
}

func (m *mockStore) NullifyUser(_ context.Context, _ int64) error {
	return nil // no-op for mock
}

func newMockStore(entries []Entry) *mockStore {
	m := mockStore(entries)
	return &m
}

func newTestEcho() *echo.Echo {
	return echo.New()
}

func doAuditRequest(e *echo.Echo, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

type listResponse struct {
	Data struct {
		Entries []map[string]any `json:"entries"`
		Limit   int              `json:"limit"`
		Offset  int              `json:"offset"`
		Count   int              `json:"count"`
	} `json:"data"`
}

func decodeList(t *testing.T, rec *httptest.ResponseRecorder) listResponse {
	t.Helper()
	var result listResponse
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return result
}

func TestList_ReturnsEntries(t *testing.T) {
	entries := []Entry{
		{ID: 3, Action: ActionLogout, Resource: ResourceSession},
		{ID: 2, Action: ActionLogin, Resource: ResourceSession},
		{ID: 1, Action: ActionLogin, Resource: ResourceSession},
	}
	store := newMockStore(entries)

	e := newTestEcho()
	allowAdmin := func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error { return next(c) }
	}
	h := NewHandler(store, allowAdmin)
	h.Register(e.Group("/api/v1"))

	rec := doAuditRequest(e, "/api/v1/audit-logs")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	result := decodeList(t, rec)
	if result.Data.Count != 3 {
		t.Fatalf("expected count 3, got %d", result.Data.Count)
	}
	if len(result.Data.Entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(result.Data.Entries))
	}
}

func TestList_FilterUserID(t *testing.T) {
	entries := []Entry{
		{ID: 1, UserID: sql.NullInt64{Int64: 1, Valid: true}, Action: ActionLogin, Resource: ResourceSession},
		{ID: 2, UserID: sql.NullInt64{Int64: 2, Valid: true}, Action: ActionLogin, Resource: ResourceSession},
		{ID: 3, UserID: sql.NullInt64{Int64: 2, Valid: true}, Action: ActionLogout, Resource: ResourceSession},
	}
	store := newMockStore(entries)

	e := newTestEcho()
	allowAdmin := func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error { return next(c) }
	}
	h := NewHandler(store, allowAdmin)
	h.Register(e.Group("/api/v1"))

	rec := doAuditRequest(e, "/api/v1/audit-logs?user_id=2")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	result := decodeList(t, rec)
	if result.Data.Count != 2 {
		t.Fatalf("expected count 2, got %d", result.Data.Count)
	}
	for _, e := range result.Data.Entries {
		if uid, ok := e["user_id"].(float64); !ok || uid != 2 {
			t.Fatalf("expected user_id 2, got %v", e["user_id"])
		}
	}
}

func TestList_FilterResource(t *testing.T) {
	entries := []Entry{
		{ID: 1, Action: ActionLogin, Resource: ResourceSession},
		{ID: 2, Action: ActionServerCreate, Resource: ResourceServer},
		{ID: 3, Action: ActionLogout, Resource: ResourceSession},
	}
	store := newMockStore(entries)

	e := newTestEcho()
	allowAdmin := func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error { return next(c) }
	}
	h := NewHandler(store, allowAdmin)
	h.Register(e.Group("/api/v1"))

	rec := doAuditRequest(e, "/api/v1/audit-logs?resource=server")

	result := decodeList(t, rec)
	if result.Data.Count != 1 {
		t.Fatalf("expected count 1, got %d", result.Data.Count)
	}
	if result.Data.Entries[0]["resource"] != ResourceServer {
		t.Fatalf("expected resource server, got %s", result.Data.Entries[0]["resource"])
	}
}

func TestList_FilterAction(t *testing.T) {
	entries := []Entry{
		{ID: 1, Action: ActionLogin, Resource: ResourceSession},
		{ID: 2, Action: ActionLogout, Resource: ResourceSession},
		{ID: 3, Action: ActionLogin, Resource: ResourceSession},
	}
	store := newMockStore(entries)

	e := newTestEcho()
	allowAdmin := func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error { return next(c) }
	}
	h := NewHandler(store, allowAdmin)
	h.Register(e.Group("/api/v1"))

	rec := doAuditRequest(e, "/api/v1/audit-logs?action=logout")

	result := decodeList(t, rec)
	if result.Data.Count != 1 {
		t.Fatalf("expected count 1, got %d", result.Data.Count)
	}
	if result.Data.Entries[0]["action"] != ActionLogout {
		t.Fatalf("expected action logout, got %s", result.Data.Entries[0]["action"])
	}
}

func TestList_Pagination(t *testing.T) {
	entries := []Entry{
		{ID: 1, Action: ActionLogin, Resource: ResourceSession},
		{ID: 2, Action: ActionLogin, Resource: ResourceSession},
		{ID: 3, Action: ActionLogin, Resource: ResourceSession},
	}
	store := newMockStore(entries)

	e := newTestEcho()
	allowAdmin := func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error { return next(c) }
	}
	h := NewHandler(store, allowAdmin)
	h.Register(e.Group("/api/v1"))

	rec := doAuditRequest(e, "/api/v1/audit-logs?limit=1&offset=1")

	result := decodeList(t, rec)
	if result.Data.Limit != 1 {
		t.Fatalf("expected limit 1, got %d", result.Data.Limit)
	}
	if result.Data.Offset != 1 {
		t.Fatalf("expected offset 1, got %d", result.Data.Offset)
	}
	if result.Data.Count != 1 {
		t.Fatalf("expected count 1, got %d", result.Data.Count)
	}
	id, _ := result.Data.Entries[0]["id"].(float64)
	if id != 2 {
		t.Fatalf("expected entry ID 2, got %v", result.Data.Entries[0]["id"])
	}
}

func TestList_InvalidLimit(t *testing.T) {
	store := newMockStore(nil)

	e := newTestEcho()
	allowAdmin := func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error { return next(c) }
	}
	h := NewHandler(store, allowAdmin)
	h.Register(e.Group("/api/v1"))

	rec := doAuditRequest(e, "/api/v1/audit-logs?limit=0")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}

	rec = doAuditRequest(e, "/api/v1/audit-logs?limit=1001")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for limit>1000, got %d", rec.Code)
	}

	rec = doAuditRequest(e, "/api/v1/audit-logs?limit=abc")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for non-numeric limit, got %d", rec.Code)
	}
}

func TestList_InvalidUserID(t *testing.T) {
	store := newMockStore(nil)

	e := newTestEcho()
	allowAdmin := func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error { return next(c) }
	}
	h := NewHandler(store, allowAdmin)
	h.Register(e.Group("/api/v1"))

	rec := doAuditRequest(e, "/api/v1/audit-logs?user_id=0")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for user_id=0, got %d", rec.Code)
	}

	rec = doAuditRequest(e, "/api/v1/audit-logs?user_id=-1")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for negative user_id, got %d", rec.Code)
	}

	rec = doAuditRequest(e, "/api/v1/audit-logs?user_id=abc")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for non-numeric user_id, got %d", rec.Code)
	}
}

func TestList_NonAdminBlocked(t *testing.T) {
	store := newMockStore(nil)

	e := newTestEcho()
	blockAdmin := func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			return echo.NewHTTPError(http.StatusForbidden, "admin only")
		}
	}
	h := NewHandler(store, blockAdmin)
	h.Register(e.Group("/api/v1"))

	rec := doAuditRequest(e, "/api/v1/audit-logs")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}
