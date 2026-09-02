package store

import (
	"context"
	"testing"
)

func TestAlertChannelStore_CreateAndGet(t *testing.T) {
	db := testDB(t)
	s := NewAlertChannelStore(db)
	ctx := context.Background()

	ch := &AlertChannel{Name: "Test Telegram", Type: "telegram", Enabled: true, Config: `{"bot_token":"abc","chat_id":"123"}`}
	if err := s.Create(ctx, ch); err != nil {
		t.Fatalf("create channel: %v", err)
	}
	if ch.ID == 0 {
		t.Fatal("channel ID not set after create")
	}

	got, err := s.GetByID(ctx, ch.ID)
	if err != nil {
		t.Fatalf("get by id: %v", err)
	}
	if got == nil {
		t.Fatal("channel not found")
	}
	if got.Name != "Test Telegram" {
		t.Fatalf("expected name Test Telegram, got %s", got.Name)
	}
	if !got.Enabled {
		t.Fatal("expected channel enabled")
	}
}

func TestAlertChannelStore_GetByID_NotFound(t *testing.T) {
	db := testDB(t)
	s := NewAlertChannelStore(db)
	ctx := context.Background()

	got, err := s.GetByID(ctx, 99999)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatal("expected nil for nonexistent channel")
	}
}

func TestAlertChannelStore_List(t *testing.T) {
	db := testDB(t)
	s := NewAlertChannelStore(db)
	ctx := context.Background()

	_ = createChannel(t, s, ctx, "ch-a", "webhook", true)
	_ = createChannel(t, s, ctx, "ch-b", "discord", false)
	_ = createChannel(t, s, ctx, "ch-c", "email", true)

	channels, err := s.List(ctx)
	if err != nil {
		t.Fatalf("list channels: %v", err)
	}
	if len(channels) != 3 {
		t.Fatalf("expected 3 channels, got %d", len(channels))
	}
	// Should be ordered by id ASC
	if channels[0].Name != "ch-a" {
		t.Fatalf("expected first ch-a, got %s", channels[0].Name)
	}
}

func TestAlertChannelStore_Update(t *testing.T) {
	db := testDB(t)
	s := NewAlertChannelStore(db)
	ctx := context.Background()

	ch := createChannel(t, s, ctx, "original", "telegram", true)
	ch.Name = "updated"
	ch.Enabled = false
	ch.Config = `{"bot_token":"new","chat_id":"456"}`

	if err := s.Update(ctx, ch); err != nil {
		t.Fatalf("update channel: %v", err)
	}

	got, _ := s.GetByID(ctx, ch.ID)
	if got.Name != "updated" {
		t.Fatalf("expected name updated, got %s", got.Name)
	}
	if got.Enabled {
		t.Fatal("expected channel disabled")
	}
}

func TestAlertChannelStore_Delete(t *testing.T) {
	db := testDB(t)
	s := NewAlertChannelStore(db)
	ctx := context.Background()

	ch := createChannel(t, s, ctx, "to-delete", "webhook", true)
	if err := s.Delete(ctx, ch.ID); err != nil {
		t.Fatalf("delete channel: %v", err)
	}

	got, _ := s.GetByID(ctx, ch.ID)
	if got != nil {
		t.Fatal("expected nil after delete")
	}
}

func TestAlertStore_CreateAndList(t *testing.T) {
	db := testDB(t)
	channels := NewAlertChannelStore(db)
	alerts := NewAlertStore(db)
	ctx := context.Background()

	ch := createChannel(t, channels, ctx, "test", "webhook", true)

	alert := &Alert{ChannelID: ch.ID, Severity: "error", Subject: "Test", Message: "Something failed"}
	if err := alerts.Create(ctx, alert); err != nil {
		t.Fatalf("create alert: %v", err)
	}
	if alert.ID == 0 {
		t.Fatal("alert ID not set")
	}
	if alert.Status != "pending" {
		t.Fatalf("expected status pending, got %s", alert.Status)
	}

	list, err := alerts.List(ctx, 10)
	if err != nil {
		t.Fatalf("list alerts: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(list))
	}
}

func TestAlertStore_MarkSent(t *testing.T) {
	db := testDB(t)
	channels := NewAlertChannelStore(db)
	alerts := NewAlertStore(db)
	ctx := context.Background()

	ch := createChannel(t, channels, ctx, "test", "webhook", true)
	alert := &Alert{ChannelID: ch.ID, Severity: "warning", Subject: "Test", Message: "msg"}
	alerts.Create(ctx, alert)

	if err := alerts.MarkSent(ctx, alert.ID); err != nil {
		t.Fatalf("mark sent: %v", err)
	}

	list, _ := alerts.List(ctx, 10)
	if list[0].Status != "sent" {
		t.Fatalf("expected status sent, got %s", list[0].Status)
	}
	if !list[0].SentAt.Valid {
		t.Fatal("expected sent_at to be set")
	}
}

func TestAlertStore_MarkFailed(t *testing.T) {
	db := testDB(t)
	channels := NewAlertChannelStore(db)
	alerts := NewAlertStore(db)
	ctx := context.Background()

	ch := createChannel(t, channels, ctx, "test", "webhook", true)
	alert := &Alert{ChannelID: ch.ID, Severity: "critical", Subject: "Test", Message: "msg"}
	alerts.Create(ctx, alert)

	if err := alerts.MarkFailed(ctx, alert.ID, "network timeout"); err != nil {
		t.Fatalf("mark failed: %v", err)
	}

	list, _ := alerts.List(ctx, 10)
	if list[0].Status != "failed" {
		t.Fatalf("expected status failed, got %s", list[0].Status)
	}
	if !list[0].Error.Valid || list[0].Error.String != "network timeout" {
		t.Fatalf("expected error network timeout, got %v", list[0].Error)
	}
}

func createChannel(t *testing.T, s *SQLiteAlertChannelStore, ctx context.Context, name, chType string, enabled bool) *AlertChannel {
	t.Helper()
	ch := &AlertChannel{Name: name, Type: chType, Enabled: enabled, Config: `{}`}
	if err := s.Create(ctx, ch); err != nil {
		t.Fatalf("create channel: %v", err)
	}
	return ch
}
