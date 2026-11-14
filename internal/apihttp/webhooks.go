package apihttp

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v5"
	"sigardapanel/internal/store"
	"strings"
)

type webhookHandler struct {
	store *store.Store
}

func newWebhookHandler(s *store.Store) *webhookHandler {
	return &webhookHandler{store: s}
}

func (h *webhookHandler) deploy(c *echo.Context) error {
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

	// Ping events (GitHub sends one when the hook is created) and pushes to
	// other branches or branch deletions do not deploy.
	if ev := c.Request().Header.Get("X-GitHub-Event"); ev == "ping" {
		return c.JSON(http.StatusOK, map[string]any{"ok": true, "message": "pong"})
	}
	push := parsePush(body)
	if push.Ref != "" {
		branch := site.GitBranch.String
		if branch == "" {
			branch = "main"
		}
		if push.Ref != "refs/heads/"+branch {
			return c.JSON(http.StatusOK, map[string]any{"ok": true, "message": "branch mismatch"})
		}
	}
	if push.Deleted {
		return c.JSON(http.StatusOK, map[string]any{"ok": true, "message": "branch deleted; nothing to deploy"})
	}
	if g, _ := h.store.Git.GetSiteGit(c.Request().Context(), site.ID); g != nil && !g.AutoDeploy {
		return c.JSON(http.StatusOK, map[string]any{"ok": true, "message": "auto deploy is off for this site"})
	}

	// Over the owner's disk limit: refuse, and tell the site's users (the git
	// host only shows the failed delivery).
	if qerr := checkSiteDiskQuota(c.Request().Context(), h.store, site, 0); qerr != nil {
		msg := "disk limit reached"
		if he, ok := qerr.(*echo.HTTPError); ok {
			msg = fmt.Sprint(he.Message)
		}
		h.store.NotifySiteUsers(c.Request().Context(), site.ID, "disk_quota", "Webhook deploy refused: "+site.Domain, msg, "error", "site", site.ID)
		return c.JSON(http.StatusForbidden, map[string]any{"ok": false, "message": msg})
	}
	siteDisk.grew(site.ID, 0)

	_, job, err := queueSiteDeploy(c.Request().Context(), h.store, site, deployRequest{Trigger: "push",
		CommitSHA: push.After, CommitMessage: push.Message, CommitAuthor: push.Author})
	if err != nil {
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

// webhookSecret answers GET /api/v1/sites/:id/webhook (site admins): the
// deploy URL path and secret, creating the secret on first use. POST
// /api/v1/sites/:id/webhook/rotate replaces it (the old one stops working).
func (h *siteHandler) webhookSecret(c *echo.Context) error {
	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	ctx := c.Request().Context()
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	site, err := h.store.Sites.GetByID(ctx, id)
	if err != nil || site == nil || !h.checkSitePermission(ctx, sessionFromEcho(c), site, "admin") {
		return echo.NewHTTPError(http.StatusNotFound, "site not found")
	}
	rotate := c.Request().Method == http.MethodPost
	secret := site.WebhookSecret.String
	if rotate || secret == "" {
		if secret, err = h.store.Sites.GenerateWebhookSecret(ctx, id); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
		}
	}
	if rotate {
		var userID int64
		if sess := sessionFromEcho(c); sess != nil {
			userID = sess.UserID
		}
		h.audit.RecordAction(ctx, userID, "webhook_secret_rotate", "site", id, map[string]any{"domain": site.Domain})
	}
	return c.JSON(http.StatusOK, map[string]any{"data": map[string]any{
		"path":   fmt.Sprintf("/api/v1/webhooks/deploy/%d", id),
		"secret": secret,
	}, "request_id": rid})
}
