package apihttp

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"sigardapanel/internal/auth"
	"sigardapanel/internal/httpjson"
	"sigardapanel/internal/store"
)

type serviceTokenHandler struct {
	store *store.Store
}

func newServiceTokenHandler(s *store.Store) *serviceTokenHandler {
	return &serviceTokenHandler{store: s}
}

// list returns all service tokens (admin only).
func (h *serviceTokenHandler) list(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	tokens, err := h.store.ServiceTokens.List(c.Request().Context())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	type tokenResp struct {
		ID         int64  `json:"id"`
		Name       string `json:"name"`
		Scopes     string `json:"scopes"`
		CreatedAt  string `json:"created_at"`
		ExpiresAt  string `json:"expires_at,omitempty"`
		LastUsedAt string `json:"last_used_at,omitempty"`
	}
	var out []tokenResp
	for _, t := range tokens {
		resp := tokenResp{
			ID:        t.ID,
			Name:      t.Name,
			Scopes:    t.Scopes,
			CreatedAt: t.CreatedAt.Format(time.RFC3339),
		}
		if t.ExpiresAt.Valid {
			resp.ExpiresAt = t.ExpiresAt.Time.Format(time.RFC3339)
		}
		if t.LastUsedAt.Valid {
			resp.LastUsedAt = t.LastUsedAt.Time.Format(time.RFC3339)
		}
		out = append(out, resp)
	}
	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      map[string]any{"tokens": out, "count": len(out)},
		RequestID: rid,
	})
}

// create generates a new service token (admin only).
// Returns the raw token value exactly once.
func (h *serviceTokenHandler) create(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)

	var body struct {
		Name      string `json:"name"`
		Scopes    string `json:"scopes"`
		ExpiresIn int    `json:"expires_in"` // seconds, 0 = no expiry
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if body.Name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name required")
	}
	if body.Scopes == "" {
		body.Scopes = "*"
	}

	// Generate raw token
	rawToken, err := auth.NewToken()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	token := &store.ServiceToken{
		Name:    body.Name,
		KeyHash: auth.HashServiceToken(rawToken),
		Scopes:  body.Scopes,
	}
	if body.ExpiresIn > 0 {
		token.ExpiresAt = sql.NullTime{
			Time:  time.Now().Add(time.Duration(body.ExpiresIn) * time.Second),
			Valid: true,
		}
	}

	if err := h.store.ServiceTokens.Create(c.Request().Context(), token); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	return c.JSON(http.StatusCreated, httpjson.Success{
		Data: map[string]any{
			"id":     token.ID,
			"name":   token.Name,
			"scopes": token.Scopes,
			"token":  rawToken, // shown once only
		},
		RequestID: rid,
	})
}

// revoke deletes a service token by ID (admin only).
func (h *serviceTokenHandler) revoke(c echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid token id")
	}

	if err := h.store.ServiceTokens.Delete(c.Request().Context(), id); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	return c.JSON(http.StatusOK, httpjson.Success{
		Data:      map[string]any{"deleted": true},
		RequestID: rid,
	})
}
