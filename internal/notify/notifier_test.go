package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewNotifier_Telegram(t *testing.T) {
	n, err := NewNotifier("telegram")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := n.(*TelegramNotifier); !ok {
		t.Fatalf("expected *TelegramNotifier, got %T", n)
	}
}

func TestNewNotifier_Discord(t *testing.T) {
	n, err := NewNotifier("discord")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := n.(*DiscordNotifier); !ok {
		t.Fatalf("expected *DiscordNotifier, got %T", n)
	}
}

func TestNewNotifier_Unknown(t *testing.T) {
	n, err := NewNotifier("unknown")
	if err == nil {
		t.Fatal("expected error for unknown type")
	}
	if n != nil {
		t.Fatal("expected nil notifier")
	}
}

func TestTelegramNotifier_MissingConfig(t *testing.T) {
	n := &TelegramNotifier{}
	ch := &Channel{Type: "telegram", Config: json.RawMessage(`{"bot_token":""}`)}
	err := n.Send(context.Background(), ch, &Alert{Severity: "error", Subject: "Test", Message: "msg"})
	if err == nil {
		t.Fatal("expected error for missing config")
	}
}

func TestDiscordNotifier_InvalidWebhook(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer ts.Close()

	n := &DiscordNotifier{}
	ch := &Channel{Type: "discord", Config: json.RawMessage(`{"webhook_url":"` + ts.URL + `"}`)}
	err := n.Send(context.Background(), ch, &Alert{Severity: "error", Subject: "Test", Message: "msg"})
	if err == nil {
		t.Fatal("expected error for 400 response")
	}
}

func TestWebhookNotifier_InvalidURL(t *testing.T) {
	n := &WebhookNotifier{}
	ch := &Channel{Type: "webhook", Config: json.RawMessage(`{"url":""}`)}
	err := n.Send(context.Background(), ch, &Alert{Severity: "error", Subject: "Test", Message: "msg"})
	if err == nil {
		t.Fatal("expected error for missing url")
	}
}

func TestEmailNotifier_MissingToAddress(t *testing.T) {
	n := &EmailNotifier{}
	ch := &Channel{Type: "email", Config: json.RawMessage(`{"to_address":""}`)}
	err := n.Send(context.Background(), ch, &Alert{Severity: "error", Subject: "Test", Message: "msg"})
	if err == nil {
		t.Fatal("expected error for missing to_address")
	}
}
