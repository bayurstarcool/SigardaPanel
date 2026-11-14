package agenthttp

import (
	"net/http"
	"sync"
	"time"

	"github.com/labstack/echo/v5"
	"sigardapanel/internal/httpjson"
	"sigardapanel/internal/publicip"
)

// detectPublicIP is replaced in tests.
var detectPublicIP = publicip.Detect

var publicIPCache struct {
	sync.Mutex
	res publicip.Result
	at  time.Time
}

// GET /tasks/system/public-ip[?refresh=1]: this server's public IPv4 (what
// a site's DNS A record should point at), cached for 10 minutes.
func (r *Router) publicIPTask(c *echo.Context) error {
	publicIPCache.Lock()
	defer publicIPCache.Unlock()
	if c.QueryParam("refresh") != "1" && publicIPCache.res.IP != "" && time.Since(publicIPCache.at) < 10*time.Minute {
		return c.JSON(http.StatusOK, httpjson.Success{Data: publicIPCache.res})
	}
	res, err := detectPublicIP(c.Request().Context(), nil)
	if err != nil {
		return c.JSON(http.StatusUnprocessableEntity, map[string]any{"ok": false, "message": err.Error()})
	}
	publicIPCache.res, publicIPCache.at = res, time.Now()
	return c.JSON(http.StatusOK, httpjson.Success{Data: res})
}
