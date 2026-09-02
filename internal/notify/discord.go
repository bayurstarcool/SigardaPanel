package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// DiscordNotifier sends alerts via Discord webhook.
type DiscordNotifier struct{}

// Send sends an alert to a Discord webhook.
func (d *DiscordNotifier) Send(ctx context.Context, channel *Channel, alert *Alert) error {
	var config struct {
		WebhookURL string `json:"webhook_url"`
	}
	if err := json.Unmarshal(channel.Config, &config); err != nil {
		return fmt.Errorf("invalid discord config: %w", err)
	}
	if config.WebhookURL == "" {
		return fmt.Errorf("discord requires webhook_url")
	}

	body, _ := json.Marshal(map[string]any{
		"content": fmt.Sprintf("**[%s] %s**\n%s", alert.Severity, alert.Subject, alert.Message),
	})

	req, err := http.NewRequestWithContext(ctx, "POST", config.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("discord request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("discord returned %d", resp.StatusCode)
	}
	return nil
}
