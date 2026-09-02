package notify

import (
	"context"
	"encoding/json"
	"log/slog"

	"sigardapanel/internal/store"
)

// Sender orchestrates alert creation and dispatch across enabled channels.
type Sender struct {
	store *store.Store
}

// NewSender creates a new Sender.
func NewSender(s *store.Store) *Sender {
	return &Sender{store: s}
}

// Send creates alert records for each enabled channel and dispatches them.
func (s *Sender) Send(ctx context.Context, severity, subject, message string) {
	channels, err := s.store.AlertChannels.List(ctx)
	if err != nil {
		slog.Error("alerting: list channels", "error", err)
		return
	}

	for _, ch := range channels {
		if !ch.Enabled {
			continue
		}

		alert := &store.Alert{
			ChannelID: ch.ID,
			Severity:  severity,
			Subject:   subject,
			Message:   message,
			Status:    "pending",
		}
		if err := s.store.Alerts.Create(ctx, alert); err != nil {
			slog.Error("alerting: create alert", "error", err)
			continue
		}

		notifier, err := NewNotifier(ch.Type)
		if err != nil {
			s.store.Alerts.MarkFailed(ctx, alert.ID, err.Error())
			continue
		}

		notifyCh := &Channel{
			ID:     ch.ID,
			Name:   ch.Name,
			Type:   ch.Type,
			Config: json.RawMessage(ch.Config),
		}

		if err := notifier.Send(ctx, notifyCh, &Alert{Severity: severity, Subject: subject, Message: message}); err != nil {
			s.store.Alerts.MarkFailed(ctx, alert.ID, err.Error())
			continue
		}

		s.store.Alerts.MarkSent(ctx, alert.ID)
	}
}
