package apihttp

import (
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"sigardapanel/internal/jobmodel"
	"sigardapanel/internal/store"
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
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if site == nil || !site.GitRepo.Valid {
		return c.JSON(http.StatusOK, map[string]any{"ok": true})
	}

	// Verify HMAC-SHA256 (GitHub style)
	sig := c.Request().Header.Get("X-Hub-Signature-256")
	if sig != "" {
		body, err := io.ReadAll(c.Request().Body)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "read body")
		}
		expected := computeHMAC(body, site.WebhookSecret.String)
		if !hmac.Equal([]byte(expected), []byte(sig)) {
			return c.JSON(http.StatusOK, map[string]any{"ok": true, "message": "ignored"})
		}
		// Re-bind from body bytes since we already consumed the reader
		var payload struct{ Ref string `json:"ref"` }
		if err := json.Unmarshal(body, &payload); err == nil {
			branch := site.GitBranch.String
			if branch == "" {
				branch = "main"
			}
			if payload.Ref != "" && payload.Ref != "refs/heads/"+branch {
				return c.JSON(http.StatusOK, map[string]any{"ok": true, "message": "branch mismatch"})
			}
		}
	} else {
		return c.JSON(http.StatusUnauthorized, map[string]any{"ok": false, "message": "missing signature"})
	}

	branch := site.GitBranch.String
	if branch == "" {
		branch = "main"
	}

	job := &jobmodel.Job{
		ServerID: sql.NullInt64{Int64: site.ServerID, Valid: true},
		SiteID:   sql.NullInt64{Int64: site.ID, Valid: true},
		Type:     jobmodel.TypeSiteDeploy,
		Status:   jobmodel.StatusQueued,
		Payload:  fmt.Sprintf(`{"site_id":%d,"repo":%q,"branch":%q}`, site.ID, site.GitRepo.String, branch),
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
