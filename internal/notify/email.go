package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
)

// EmailNotifier is a stub implementation that logs only.
type EmailNotifier struct{}

// Send logs the email alert without sending actual email.
func (e *EmailNotifier) Send(ctx context.Context, channel *Channel, alert *Alert) error {
	var config struct {
		ToAddress string `json:"to_address"`
	}
	if err := json.Unmarshal(channel.Config, &config); err != nil {
		return fmt.Errorf("invalid email config: %w", err)
	}
	if config.ToAddress == "" {
		return fmt.Errorf("email requires to_address")
	}
	slog.Info("email: stub send", "to", config.ToAddress, "subject", alert.Subject)
	return nil
}
