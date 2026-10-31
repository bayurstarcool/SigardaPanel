package apihttp

import (
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
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

// checkSite enforces the site permission model: listing needs read, creating
// or deleting rules needs write. Returns a JSON error response (in this
// handler's response format). ok is false when the request was refused and
// the response has already been written.
func (h *ipRestrictionHandler) checkSite(c echo.Context, site *store.Site, required string) (ok bool, err error) {
	if err := requireSitePermission(c.Request().Context(), h.store, sessionFromEcho(c), site, required); err != nil {
		code, msg := http.StatusNotFound, "site not found"
		if he, isHTTP := err.(*echo.HTTPError); isHTTP {
			code, msg = he.Code, fmt.Sprint(he.Message)
		}
		return false, c.JSON(code, map[string]string{"error": msg})
	}
	return true, nil
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
	if ok, err := h.checkSite(c, site, sitePermRead); !ok {
		return err
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
	if ok, err := h.checkSite(c, site, sitePermWrite); !ok {
		return err
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
	if ok, err := h.checkSite(c, site, sitePermWrite); !ok {
		return err
	}

	if err := h.store.IPRestrictions.Delete(c.Request().Context(), siteID, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "ip restriction not found"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "IP restriction deleted"})
}
