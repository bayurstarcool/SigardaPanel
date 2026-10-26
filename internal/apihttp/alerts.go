package apihttp

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/mail"
	"net/url"
	"strconv"

	"github.com/labstack/echo/v4"
	"sigardapanel/internal/audit"
	"sigardapanel/internal/httpjson"
	"sigardapanel/internal/notify"
	"sigardapanel/internal/store"
)

type alertHandler struct {
	store  *store.Store
	audit  *audit.Recorder
	sender *notify.Sender
}

func newAlertHandler(s *store.Store, a *audit.Recorder) *alertHandler {
	return &alertHandler{store: s, audit: a, sender: notify.NewSender(s)}
}

func (h *alertHandler) createChannel(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)
	var userID int64
	if sess != nil {
		userID = sess.UserID
	}

	var req struct {
		Name   string          `json:"name"`
		Type   string          `json:"type"`
		Config json.RawMessage `json:"config"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if req.Name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name is required")
	}
	if !isValidChannelType(req.Type) {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("invalid channel type: %s", req.Type))
	}
	if len(req.Config) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "config is required")
	}

	if err := validateChannelConfig(req.Type, req.Config); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	ch := &store.AlertChannel{
		Name:   req.Name,
		Type:   req.Type,
		Config: string(req.Config),
	}
	if err := h.store.AlertChannels.Create(c.Request().Context(), ch); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	h.audit.RecordAction(c.Request().Context(), userID, "alert_channel_create", "alert_channel", ch.ID, map[string]any{
		"type": req.Type,
		"name": req.Name,
	})

	return c.JSON(http.StatusCreated, httpjson.Success{
		Data:      alertChannelToMap(ch),
		RequestID: rid,
	})
}

func (h *alertHandler) listChannels(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)

	channels, err := h.store.AlertChannels.List(c.Request().Context())
	if err != nil {
		slog.Error("api: request failed", "route", c.Path(), "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	out := make([]map[string]any, 0, len(channels))
	for _, ch := range channels {
		out = append(out, alertChannelToMap(&ch))
	}

	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      map[string]any{"channels": out, "count": len(out)},
		RequestID: rid,
	})
}

func (h *alertHandler) deleteChannel(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	sess := sessionFromEcho(c)
	var userID int64
	if sess != nil {
		userID = sess.UserID
	}

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}

	ch, err := h.store.AlertChannels.GetByID(c.Request().Context(), id)
	if err != nil {
		slog.Error("api: request failed", "route", c.Path(), "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if ch == nil {
		return echo.NewHTTPError(http.StatusNotFound, "channel not found")
	}

	if err := h.store.AlertChannels.Delete(c.Request().Context(), id); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	h.audit.RecordAction(c.Request().Context(), userID, "alert_channel_delete", "alert_channel", id, map[string]any{
		"type": ch.Type,
		"name": ch.Name,
	})

	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      map[string]any{"deleted": true},
		RequestID: rid,
	})
}

func (h *alertHandler) listAlerts(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)

	limit := 100
	if v := c.QueryParam("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}

	alerts, err := h.store.Alerts.List(c.Request().Context(), limit)
	if err != nil {
		slog.Error("api: request failed", "route", c.Path(), "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	out := make([]map[string]any, 0, len(alerts))
	for _, a := range alerts {
		out = append(out, alertToMap(a))
	}

	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      map[string]any{"alerts": out, "count": len(out)},
		RequestID: rid,
	})
}

func isValidChannelType(t string) bool {
	switch t {
	case "telegram", "discord", "email", "webhook":
		return true
	}
	return false
}

func validateChannelConfig(channelType string, config json.RawMessage) error {
	switch channelType {
	case "telegram":
		var cfg struct {
			BotToken string `json:"bot_token"`
			ChatID   string `json:"chat_id"`
		}
		if err := json.Unmarshal(config, &cfg); err != nil {
			return fmt.Errorf("invalid config: %w", err)
		}
		if cfg.BotToken == "" || cfg.ChatID == "" {
			return fmt.Errorf("telegram config requires bot_token and chat_id")
		}
	case "discord":
		var cfg struct {
			WebhookURL string `json:"webhook_url"`
		}
		if err := json.Unmarshal(config, &cfg); err != nil {
			return fmt.Errorf("invalid config: %w", err)
		}
		if cfg.WebhookURL == "" {
			return fmt.Errorf("discord config requires webhook_url")
		}
	case "email":
		var cfg struct {
			ToAddress string `json:"to_address"`
			From      string `json:"from_address"`
			TLS       string `json:"smtp_tls"`
		}
		if err := json.Unmarshal(config, &cfg); err != nil {
			return fmt.Errorf("invalid config: %w", err)
		}
		if cfg.ToAddress == "" {
			return fmt.Errorf("email config requires to_address")
		}
		if _, err := mail.ParseAddressList(cfg.ToAddress); err != nil {
			return fmt.Errorf("email config: invalid to_address")
		}
		if cfg.From != "" {
			if _, err := mail.ParseAddress(cfg.From); err != nil {
				return fmt.Errorf("email config: invalid from_address")
			}
		}
		switch cfg.TLS {
		case "", "starttls", "ssl", "none":
		default:
			return fmt.Errorf("email config: smtp_tls must be starttls, ssl or none")
		}
	case "webhook":
		var cfg struct {
			URL string `json:"url"`
		}
		if err := json.Unmarshal(config, &cfg); err != nil {
			return fmt.Errorf("invalid config: %w", err)
		}
		if cfg.URL == "" {
			return fmt.Errorf("webhook config requires url")
		}
	}
	return nil
}

func alertChannelToMap(ch *store.AlertChannel) map[string]any {
	m := map[string]any{
		"id":         ch.ID,
		"name":       ch.Name,
		"type":       ch.Type,
		"enabled":    ch.Enabled,
		"config":     redactChannelConfig(ch.Config),
		"created_at": ch.CreatedAt,
		"updated_at": ch.UpdatedAt,
	}
	return m
}

// channelSecretKeys are config fields that must never be echoed back to the UI.
var channelSecretKeys = map[string]bool{
	"bot_token":     true,
	"smtp_password": true,
	"password":      true,
	"secret":        true,
	"token":         true,
	"webhook_url":   true,
	"url":           true,
}

// redactChannelConfig masks secret values in a stored channel config. A
// webhook URL usually embeds its credential, so it is masked down to the host.
func redactChannelConfig(raw string) string {
	var cfg map[string]any
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return "{}"
	}
	for k, v := range cfg {
		if !channelSecretKeys[k] {
			continue
		}
		str, _ := v.(string)
		if str == "" {
			continue
		}
		if k == "webhook_url" || k == "url" {
			if u, err := url.Parse(str); err == nil && u.Host != "" {
				cfg[k] = u.Scheme + "://" + u.Host + "/********"
				continue
			}
		}
		cfg[k] = "********"
	}
	out, err := json.Marshal(cfg)
	if err != nil {
		return "{}"
	}
	return string(out)
}

func alertToMap(a store.Alert) map[string]any {
	m := map[string]any{
		"id":         a.ID,
		"channel_id": a.ChannelID,
		"severity":   a.Severity,
		"subject":    a.Subject,
		"message":    a.Message,
		"status":     a.Status,
		"created_at": a.CreatedAt,
	}
	if a.Error.Valid {
		m["error"] = a.Error.String
	}
	if a.SentAt.Valid {
		m["sent_at"] = a.SentAt.Time
	}
	return m
}
