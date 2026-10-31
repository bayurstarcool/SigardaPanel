package apihttp

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"sigardapanel/internal/jobmodel"
	"sigardapanel/internal/store"
	"strings"
)

type webhookHandler struct {
	store *store.Store
}

func newWebhookHandler(s *store.Store) *webhookHandler {
	return &webhookHandler{store: s}
}

func (h *webhookHandler) deploy(c echo.Context) error {
	siteID, err := strconv.ParseInt(c.Param("site_id"), 10, 64)
	if err != nil || siteID <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid site_id")
	}

	site, err := h.store.Sites.GetByID(c.Request().Context(), siteID)
	if err != nil {
		slog.Error("api: request failed", "route", c.Path(), "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if site == nil || !site.GitRepo.Valid {
		return c.JSON(http.StatusOK, map[string]any{"ok": true})
	}

	// Verify signature / token (GitHub / Gitea / GitLab).
	// A site without a configured secret must NEVER accept deploys, regardless
	// of which signature headers are present.
	body, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "read body")
	}

	secret := site.WebhookSecret.String
	if secret == "" {
		return c.JSON(http.StatusUnauthorized, map[string]any{"ok": false, "message": "webhook secret not configured"})
	}

	gitHubSig := c.Request().Header.Get("X-Hub-Signature-256")
	giteaSig := c.Request().Header.Get("X-Gitea-Signature")
	gitlabToken := c.Request().Header.Get("X-Gitlab-Token")

	if gitHubSig != "" {
		expected := computeHMAC(body, secret) // "sha256=" + hex
		if !hmac.Equal([]byte(expected), []byte(gitHubSig)) {
			return c.JSON(http.StatusUnauthorized, map[string]any{"ok": false, "message": "invalid signature"})
		}
	} else if giteaSig != "" {
		// Gitea sends the bare hex digest without the "sha256=" prefix.
		expected := strings.TrimPrefix(computeHMAC(body, secret), "sha256=")
		if !hmac.Equal([]byte(expected), []byte(strings.ToLower(strings.TrimSpace(giteaSig)))) {
			return c.JSON(http.StatusUnauthorized, map[string]any{"ok": false, "message": "invalid signature"})
		}
	} else if gitlabToken != "" {
		if subtle.ConstantTimeCompare([]byte(gitlabToken), []byte(secret)) != 1 {
			return c.JSON(http.StatusUnauthorized, map[string]any{"ok": false, "message": "invalid token"})
		}
	} else {
		return c.JSON(http.StatusUnauthorized, map[string]any{"ok": false, "message": "missing signature"})
	}

	// Verify branch if ref is present in payload
	var payload struct {
		Ref string `json:"ref"`
	}
	if err := json.Unmarshal(body, &payload); err == nil && payload.Ref != "" {
		branch := site.GitBranch.String
		if branch == "" {
			branch = "main"
		}
		if payload.Ref != "refs/heads/"+branch {
			return c.JSON(http.StatusOK, map[string]any{"ok": true, "message": "branch mismatch"})
		}
	}

	branch := site.GitBranch.String
	if branch == "" {
		branch = "main"
	}

	job := &jobmodel.Job{
		ServerID:    sql.NullInt64{Int64: site.ServerID, Valid: true},
		SiteID:      sql.NullInt64{Int64: site.ID, Valid: true},
		Type:        jobmodel.TypeSiteDeploy,
		Status:      jobmodel.StatusQueued,
		Description: fmt.Sprintf("Deploy site %s (webhook)", site.Domain),
		Payload:     deployPayload(site, branch),
	}
	if err := h.store.Jobs.Create(c.Request().Context(), job); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	return c.JSON(http.StatusOK, map[string]any{
		"ok":      true,
		"message": "deploy queued",
		"job_id":  job.ID,
	})
}

func computeHMAC(body []byte, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(body)
	return "sha256=" + hex.EncodeToString(h.Sum(nil))
}
