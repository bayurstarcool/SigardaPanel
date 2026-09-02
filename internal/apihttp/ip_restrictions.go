package apihttp

import (
	"net/http"
	"strconv"

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

// list returns all IP restrictions for a site.
func (h *ipRestrictionHandler) list(c echo.Context) error {
	siteID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid site id"})
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
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid site id"})
	}

	var body struct {
		IP     string `json:"ip"`
		Action string `json:"action"` // allow, deny
		Reason string `json:"reason"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	if body.IP == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "ip is required"})
	}
	if body.Action != "allow" && body.Action != "deny" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "action must be 'allow' or 'deny'"})
	}

	restriction := &store.IPRestriction{
		SiteID: siteID,
		IP:     body.IP,
		Action: body.Action,
		Reason: body.Reason,
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
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid restriction id"})
	}

	if err := h.store.IPRestrictions.Delete(c.Request().Context(), id); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "IP restriction deleted"})
}
