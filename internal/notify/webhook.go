package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// WebhookNotifier sends alerts via generic HTTP webhook.
type WebhookNotifier struct{}

// Send sends an alert to a generic webhook URL.
func (w *WebhookNotifier) Send(ctx context.Context, channel *Channel, alert *Alert) error {
	var config struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(channel.Config, &config); err != nil {
		return fmt.Errorf("invalid webhook config: %w", err)
	}
	if config.URL == "" {
		return fmt.Errorf("webhook requires url")
	}

	payload := map[string]any{
		"severity": alert.Severity,
		"subject":  alert.Subject,
		"message":  alert.Message,
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, "POST", config.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("webhook request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned %d", resp.StatusCode)
	}
	return nil
}
