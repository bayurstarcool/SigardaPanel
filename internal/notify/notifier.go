package notify

import (
	"context"
	"encoding/json"
	"fmt"
)

// Channel represents a notification channel.
type Channel struct {
	ID     int64
	Name   string
	Type   string
	Config json.RawMessage
}

// Alert represents a notification payload.
type Alert struct {
	Severity string
	Subject  string
	Message  string
}

// Notifier is the interface for sending alerts.
type Notifier interface {
	Send(ctx context.Context, channel *Channel, alert *Alert) error
}

// NewNotifier returns a notifier implementation for the given channel type.
func NewNotifier(channelType string) (Notifier, error) {
	switch channelType {
	case "telegram":
		return &TelegramNotifier{}, nil
	case "discord":
		return &DiscordNotifier{}, nil
	case "email":
		return &EmailNotifier{}, nil
	case "webhook":
		return &WebhookNotifier{}, nil
	default:
		return nil, fmt.Errorf("unknown channel type: %s", channelType)
	}
}
