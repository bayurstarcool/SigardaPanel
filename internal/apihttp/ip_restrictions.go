package apihttp

import (
	"context"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"sigardapanel/internal/auth"
	"sigardapanel/internal/store"
)

// ipRestrictionHandler handles IP restriction endpoints.
type ipRestrictionHandler struct {
	store *store.Store
}

// newIPRestrictionHandler creates a new ipRestrictionHandler.
func newIPRestrictionHandler(s *store.Store) *ipRestrictionHandler {
	return &ipRestrictionHandler{store: s}
}

func (h *ipRestrictionHandler) canAccess(ctx context.Context, sess *store.Session, site *store.Site) bool {
	if sess == nil || site == nil {
		return false
	}
	user, _ := h.store.Users.GetByID(ctx, sess.UserID)
	if user == nil {
		return false
	}
	if user.Role == auth.RoleSuperAdmin {
		return true
	}
	if site.OwnerID.Valid && site.OwnerID.Int64 == sess.UserID {
		return true
	}
	hasAccess, _ := h.store.UserSites.HasAccess(ctx, sess.UserID, site.ID)
	return hasAccess
}

// list returns all IP restrictions for a site.
func (h *ipRestrictionHandler) list(c echo.Context) error {
	siteID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || siteID <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid site id"})
	}

	site, err := h.store.Sites.GetByID(c.Request().Context(), siteID)
	if err != nil || site == nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "site not found"})
	}
	sess := sessionFromEcho(c)
	if !h.canAccess(c.Request().Context(), sess, site) {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "site not found"})
	}

	restrictions, err := h.store.IPRestrictions.List(c.Request().Context(), siteID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"restrictions": restrictions,
	})
}

// create adds a new IP restriction rule.
func (h *ipRestrictionHandler) create(c echo.Context) error {
	siteID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || siteID <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid site id"})
	}

	site, err := h.store.Sites.GetByID(c.Request().Context(), siteID)
	if err != nil || site == nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "site not found"})
	}
	sess := sessionFromEcho(c)
	if !h.canAccess(c.Request().Context(), sess, site) {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "site not found"})
	}

	var body struct {
		IP     string `json:"ip"`
		Action string `json:"action"` // allow, deny
		Reason string `json:"reason"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	body.IP = strings.TrimSpace(body.IP)
	if body.IP == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "ip is required"})
	}

	// Validate valid IP or valid CIDR
	if net.ParseIP(body.IP) == nil {
		if _, _, err := net.ParseCIDR(body.IP); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid IP address or CIDR notation"})
		}
	}

	if body.Action != "allow" && body.Action != "deny" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "action must be 'allow' or 'deny'"})
	}

	restriction := &store.IPRestriction{
		SiteID: siteID,
		IP:     body.IP,
		Action: body.Action,
		Reason: strings.TrimSpace(body.Reason),
	}

	if err := h.store.IPRestrictions.Create(c.Request().Context(), restriction); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusCreated, map[string]any{
		"restriction": restriction,
	})
}

// delete removes an IP restriction rule.
func (h *ipRestrictionHandler) delete(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("restriction_id"), 10, 64)
	if err != nil || id <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid restriction id"})
	}

	siteID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || siteID <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid site id"})
	}

	// Authz is mandatory: if the site cannot be resolved, refuse to delete.
	site, err := h.store.Sites.GetByID(c.Request().Context(), siteID)
	if err != nil || site == nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "site not found"})
	}
	sess := sessionFromEcho(c)
	if !h.canAccess(c.Request().Context(), sess, site) {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "site not found"})
	}

	if err := h.store.IPRestrictions.Delete(c.Request().Context(), id); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "IP restriction deleted"})
}
