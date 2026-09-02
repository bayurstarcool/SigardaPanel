package apihttp

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"sigardapanel/internal/store"
)

func TestAlertChannelCreate_Success(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	token := newAdminToken(t, s, ctx)

	body := map[string]any{
		"name":   "Test Telegram",
		"type":   "telegram",
		"config": json.RawMessage(`{"bot_token":"abc","chat_id":"123"}`),
	}
	rec := doRequest(t, e, "POST", "/api/v1/alert-channels", body, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d, body: %s", rec.Code, rec.Body.String())
	}

	var result struct {
		Data struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
			Type string `json:"type"`
		} `json:"data"`
	}
	json.NewDecoder(rec.Body).Decode(&result)
	if result.Data.ID == 0 {
		t.Fatal("expected non-zero id")
	}
	if result.Data.Name != "Test Telegram" {
		t.Fatalf("expected name Test Telegram, got %s", result.Data.Name)
	}
}

func TestAlertChannelCreate_InvalidType(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	token := newAdminToken(t, s, ctx)

	body := map[string]any{
		"name":   "Test",
		"type":   "unknown",
		"config": json.RawMessage(`{}`),
	}
	rec := doRequest(t, e, "POST", "/api/v1/alert-channels", body, token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestAlertChannelCreate_MissingConfig(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	token := newAdminToken(t, s, ctx)

	body := map[string]any{
		"name": "Test",
		"type": "telegram",
	}
	rec := doRequest(t, e, "POST", "/api/v1/alert-channels", body, token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestAlertChannelList_Success(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	token := newAdminToken(t, s, ctx)

	s.AlertChannels.Create(ctx, &store.AlertChannel{Name: "ch1", Type: "webhook", Config: `{}`})
	s.AlertChannels.Create(ctx, &store.AlertChannel{Name: "ch2", Type: "email", Config: `{}`})

	rec := doRequest(t, e, "GET", "/api/v1/alert-channels", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var result struct {
		Data struct {
			Channels []map[string]any `json:"channels"`
			Count    int              `json:"count"`
		} `json:"data"`
	}
	json.NewDecoder(rec.Body).Decode(&result)
	if result.Data.Count != 2 {
		t.Fatalf("expected count 2, got %d", result.Data.Count)
	}
}

func TestAlertChannelDelete_Success(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	token := newAdminToken(t, s, ctx)

	ch := &store.AlertChannel{Name: "to-delete", Type: "webhook", Config: `{}`}
	s.AlertChannels.Create(ctx, ch)

	rec := doRequest(t, e, "DELETE", "/api/v1/alert-channels/"+strconv.FormatInt(ch.ID, 10), nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	got, _ := s.AlertChannels.GetByID(ctx, ch.ID)
	if got != nil {
		t.Fatal("expected channel deleted")
	}
}

func TestAlertChannelDelete_NotFound(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	token := newAdminToken(t, s, ctx)

	rec := doRequest(t, e, "DELETE", "/api/v1/alert-channels/99999", nil, token)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestAlertChannelList_UserForbidden(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	userToken := newUserToken(t, s, ctx)

	rec := doRequest(t, e, "GET", "/api/v1/alert-channels", nil, userToken)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestAlertsList_Success(t *testing.T) {
	e, s, ctx := newTestAppWithAllMigrations(t)
	token := newAdminToken(t, s, ctx)

	ch := &store.AlertChannel{Name: "test", Type: "webhook", Config: `{}`}
	s.AlertChannels.Create(ctx, ch)
	s.Alerts.Create(ctx, &store.Alert{ChannelID: ch.ID, Severity: "error", Subject: "S1", Message: "M1"})
	s.Alerts.Create(ctx, &store.Alert{ChannelID: ch.ID, Severity: "warning", Subject: "S2", Message: "M2"})

	rec := doRequest(t, e, "GET", "/api/v1/alerts", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var result struct {
		Data struct {
			Alerts []map[string]any `json:"alerts"`
			Count  int              `json:"count"`
		} `json:"data"`
	}
	json.NewDecoder(rec.Body).Decode(&result)
	if result.Data.Count != 2 {
		t.Fatalf("expected count 2, got %d", result.Data.Count)
	}
}

func TestAlertsList_RequiresAuth(t *testing.T) {
	e, _, _ := newTestAppWithAllMigrations(t)

	rec := doRequest(t, e, "GET", "/api/v1/alerts", nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}
