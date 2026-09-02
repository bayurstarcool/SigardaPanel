package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// TelegramNotifier sends alerts via Telegram Bot API.
type TelegramNotifier struct{}

// Send sends an alert to a Telegram channel.
func (t *TelegramNotifier) Send(ctx context.Context, channel *Channel, alert *Alert) error {
	var config struct {
		BotToken string `json:"bot_token"`
		ChatID   string `json:"chat_id"`
	}
	if err := json.Unmarshal(channel.Config, &config); err != nil {
		return fmt.Errorf("invalid telegram config: %w", err)
	}
	if config.BotToken == "" || config.ChatID == "" {
		return fmt.Errorf("telegram requires bot_token and chat_id")
	}

	text := fmt.Sprintf("*[%s]* %s\n\n%s", alert.Severity, alert.Subject, alert.Message)
	body, _ := json.Marshal(map[string]string{
		"chat_id":    config.ChatID,
		"text":       text,
		"parse_mode": "Markdown",
	})

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", config.BotToken)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("telegram request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("telegram returned %d", resp.StatusCode)
	}
	return nil
}
