package apihttp

import (
	"database/sql"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/labstack/echo/v5"
	"sigardapanel/internal/audit"
	"sigardapanel/internal/auth"
	"sigardapanel/internal/httpjson"
	"sigardapanel/internal/store"
)

// apiKeyHandler manages personal API keys. Every operation is scoped to the
// calling user: there is no way to list, create or revoke another user's keys.
type apiKeyHandler struct {
	store *store.Store
	audit *audit.Recorder
}

func newAPIKeyHandler(s *store.Store, a *audit.Recorder) *apiKeyHandler {
	return &apiKeyHandler{store: s, audit: a}
}

type apiKeyResp struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Prefix     string `json:"prefix"`
	CreatedAt  string `json:"created_at"`
	ExpiresAt  string `json:"expires_at,omitempty"`
	LastUsedAt string `json:"last_used_at,omitempty"`
	Expired    bool   `json:"expired"`
}

func toAPIKeyResp(k store.UserAPIKey) apiKeyResp {
	r := apiKeyResp{ID: k.ID, Name: k.Name, Prefix: k.Prefix, CreatedAt: k.CreatedAt.UTC().Format(time.RFC3339)}
	if k.ExpiresAt.Valid {
		r.ExpiresAt = k.ExpiresAt.Time.UTC().Format(time.RFC3339)
		r.Expired = time.Now().After(k.ExpiresAt.Time)
	}
	if k.LastUsedAt.Valid {
		r.LastUsedAt = k.LastUsedAt.Time.UTC().Format(time.RFC3339)
	}
	return r
}

// list returns the caller's API keys (never the key material).
func (h *apiKeyHandler) list(c *echo.Context) error {
	sess := sessionFromEcho(c)
	keys, err := h.store.APIKeys.ListByUser(c.Request().Context(), sess.UserID)
	if err != nil {
		slog.Error("api: list api keys", "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	out := make([]apiKeyResp, 0, len(keys))
	for _, k := range keys {
		out = append(out, toAPIKeyResp(k))
	}
	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      map[string]any{"keys": out, "count": len(out), "limit": store.MaxAPIKeysPerUser},
		RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
	})
}

// create mints a key for the caller. The raw key is in the response exactly
// once; only its SHA-256 hash is stored.
func (h *apiKeyHandler) create(c *echo.Context) error {
	sess := sessionFromEcho(c)
	// An impersonating admin must not be able to mint lasting credentials
	// for the user being impersonated.
	if sess.ImpersonatedBy.Valid {
		return echo.NewHTTPError(http.StatusForbidden, "api keys cannot be created while impersonating")
	}
	var body struct {
		Name          string `json:"name"`
		ExpiresInDays int    `json:"expires_in_days"` // 0 = never
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name is required")
	}
	if utf8.RuneCountInString(body.Name) > 64 {
		return echo.NewHTTPError(http.StatusBadRequest, "name must be at most 64 characters")
	}
	if body.ExpiresInDays < 0 || body.ExpiresInDays > 3650 {
		return echo.NewHTTPError(http.StatusBadRequest, "expires_in_days must be between 0 (never) and 3650")
	}
	ctx := c.Request().Context()
	n, err := h.store.APIKeys.CountByUser(ctx, sess.UserID)
	if err != nil {
		slog.Error("api: count api keys", "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if n >= store.MaxAPIKeysPerUser {
		return echo.NewHTTPError(http.StatusConflict, "api key limit reached ("+strconv.Itoa(store.MaxAPIKeysPerUser)+"); revoke an unused key first")
	}

	raw, err := auth.NewToken()
	if err != nil {
		slog.Error("api: generate api key", "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	key := &store.UserAPIKey{
		UserID:  sess.UserID,
		Name:    body.Name,
		KeyHash: auth.HashToken(raw),
		Prefix:  raw[:8],
	}
	if body.ExpiresInDays > 0 {
		key.ExpiresAt = sql.NullTime{Time: time.Now().Add(time.Duration(body.ExpiresInDays) * 24 * time.Hour), Valid: true}
	}
	if err := h.store.APIKeys.Create(ctx, key); err != nil {
		slog.Error("api: create api key", "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	h.audit.RecordAction(ctx, sess.UserID, "api_key.create", "api_key", key.ID, map[string]any{"name": key.Name})

	resp := toAPIKeyResp(*key)
	return c.JSON(http.StatusCreated, httpjson.Success{
		Data:      map[string]any{"key": resp, "token": raw},
		RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
	})
}

// revoke deletes one of the caller's keys. Another user's key is a 404.
func (h *apiKeyHandler) revoke(c *echo.Context) error {
	sess := sessionFromEcho(c)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	ctx := c.Request().Context()
	ok, err := h.store.APIKeys.DeleteForUser(ctx, id, sess.UserID)
	if err != nil {
		slog.Error("api: revoke api key", "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if !ok {
		return echo.NewHTTPError(http.StatusNotFound, "api key not found")
	}
	h.audit.RecordAction(ctx, sess.UserID, "api_key.revoke", "api_key", id, nil)
	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      map[string]any{"revoked": true, "id": id},
		RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
	})
}

// purgeAuditLogs deletes audit entries older than ?older_than_days=N (N >= 1).
// Wiping the whole log is deliberately impossible, and the purge itself is
// recorded as a new audit entry so it can never be hidden.
func purgeAuditLogs(s *store.Store, rec *audit.Recorder) echo.HandlerFunc {
	return func(c *echo.Context) error {
		days, err := strconv.Atoi(c.QueryParam("older_than_days"))
		if err != nil || days < 1 || days > 3650 {
			return echo.NewHTTPError(http.StatusBadRequest, "older_than_days must be between 1 and 3650")
		}
		ctx := c.Request().Context()
		n, err := s.AuditLogs.PurgeOlderThan(ctx, days)
		if err != nil {
			slog.Error("api: purge audit logs", "error", err)
			return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
		}
		sess := sessionFromEcho(c)
		rec.RecordAction(ctx, sess.UserID, "audit.purge", "audit_log", 0, map[string]any{"older_than_days": days, "deleted": n})
		return c.JSON(http.StatusOK, httpjson.Success{
			Data:      map[string]any{"deleted": n, "older_than_days": days},
			RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
		})
	}
}
