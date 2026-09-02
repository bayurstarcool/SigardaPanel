package agenthttp

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

// fail2banStatus returns the overall fail2ban status and list of jails.
func (r *Router) fail2banStatus(c echo.Context) error {
	ctx := c.Request().Context()

	// Check if fail2ban is installed
	output, err := r.executor.RunShell(ctx, "fail2ban-client", "status")
	if err != nil {
		return c.JSON(http.StatusOK, map[string]any{
			"installed": false,
			"active":    false,
			"error":     strings.TrimSpace(output),
		})
	}

	// Parse jail list from output
	jails := []string{}
	lines := strings.Split(output, "\n")
	inJailList := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "Jail list:") {
			inJailList = true
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				jailStr := strings.TrimSpace(parts[1])
				if jailStr != "" {
					for _, j := range strings.Split(jailStr, ",") {
						j = strings.TrimSpace(j)
						if j != "" {
							jails = append(jails, j)
						}
					}
				}
			}
			continue
		}
		if inJailList && strings.HasPrefix(line, "|") {
			// Continued jail list (multi-line format)
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				jailStr := strings.TrimSpace(parts[1])
				if jailStr != "" {
					for _, j := range strings.Split(jailStr, ",") {
						j = strings.TrimSpace(j)
						if j != "" {
							jails = append(jails, j)
						}
					}
				}
			}
		}
	}

	// Get version
	versionOut, _ := r.executor.RunShell(ctx, "fail2ban-client", "--version")
	version := strings.TrimSpace(versionOut)

	return c.JSON(http.StatusOK, map[string]any{
		"installed": true,
		"active":    len(jails) > 0,
		"version":   version,
		"jails":     jails,
	})
}

// fail2banJail returns details about a specific jail.
func (r *Router) fail2banJail(c echo.Context) error {
	ctx := c.Request().Context()
	jailName := c.QueryParam("name")
	if jailName == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "jail name required")
	}

	output, err := r.executor.RunShell(ctx, "fail2ban-client", "status", jailName)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get jail status: "+strings.TrimSpace(output))
	}

	// Parse output
	result := map[string]any{
		"name":             jailName,
		"banned":           []string{},
		"total_banned":     0,
		"failed":           0,
		"currently_failed": 0,
	}

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "Currently banned:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				var n int
				for _, ch := range strings.TrimSpace(parts[1]) {
					if ch >= '0' && ch <= '9' {
						n = n*10 + int(ch-'0')
					}
				}
				result["currently_banned"] = n
			}
		}
		if strings.Contains(line, "Total banned:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				n := 0
				for _, ch := range strings.TrimSpace(parts[1]) {
					if ch >= '0' && ch <= '9' {
						n = n*10 + int(ch-'0')
					}
				}
				result["total_banned"] = n
			}
		}
		if strings.Contains(line, "Currently failed:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				n := 0
				for _, ch := range strings.TrimSpace(parts[1]) {
					if ch >= '0' && ch <= '9' {
						n = n*10 + int(ch-'0')
					}
				}
				result["currently_failed"] = n
			}
		}
		if strings.Contains(line, "Total failed:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				n := 0
				for _, ch := range strings.TrimSpace(parts[1]) {
					if ch >= '0' && ch <= '9' {
						n = n*10 + int(ch-'0')
					}
				}
				result["failed"] = n
			}
		}
		if strings.Contains(line, "Banned IP list:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				ipStr := strings.TrimSpace(parts[1])
				if ipStr != "" {
					ips := []string{}
					for _, ip := range strings.Split(ipStr, " ") {
						ip = strings.TrimSpace(ip)
						if ip != "" {
							ips = append(ips, ip)
						}
					}
					result["banned"] = ips
				}
			}
		}
	}

	return c.JSON(http.StatusOK, result)
}

// fail2banUnban unbans an IP from a jail.
func (r *Router) fail2banUnban(c echo.Context) error {
	ctx := c.Request().Context()

	var body struct {
		Jail string `json:"jail"`
		IP   string `json:"ip"`
	}
	if err := c.Bind(&body); err != nil || body.IP == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "ip required")
	}
	if body.Jail == "" {
		body.Jail = "sshd"
	}

	output, err := r.executor.RunShell(ctx, "fail2ban-client", "set", body.Jail, "unbanip", body.IP)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "unban failed: "+strings.TrimSpace(output))
	}

	return c.JSON(http.StatusOK, map[string]any{
		"ok":      true,
		"message": "Unbanned " + body.IP + " from " + body.Jail,
		"output":  strings.TrimSpace(output),
	})
}

// fail2banBan bans an IP in a jail.
func (r *Router) fail2banBan(c echo.Context) error {
	ctx := c.Request().Context()

	var body struct {
		Jail string `json:"jail"`
		IP   string `json:"ip"`
	}
	if err := c.Bind(&body); err != nil || body.IP == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "ip required")
	}
	if body.Jail == "" {
		body.Jail = "sshd"
	}

	output, err := r.executor.RunShell(ctx, "fail2ban-client", "set", body.Jail, "banip", body.IP)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "ban failed: "+strings.TrimSpace(output))
	}

	return c.JSON(http.StatusOK, map[string]any{
		"ok":      true,
		"message": "Banned " + body.IP + " in " + body.Jail,
		"output":  strings.TrimSpace(output),
	})
}

// fail2banEnable enables a jail.
func (r *Router) fail2banEnable(c echo.Context) error {
	ctx := c.Request().Context()

	var body struct {
		Jail string `json:"jail"`
	}
	if err := c.Bind(&body); err != nil || body.Jail == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "jail name required")
	}

	output, err := r.executor.RunShell(ctx, "fail2ban-client", "start", body.Jail)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "enable failed: "+strings.TrimSpace(output))
	}

	return c.JSON(http.StatusOK, map[string]any{
		"ok":      true,
		"message": "Enabled jail " + body.Jail,
		"output":  strings.TrimSpace(output),
	})
}

// fail2banDisable disables a jail.
func (r *Router) fail2banDisable(c echo.Context) error {
	ctx := c.Request().Context()

	var body struct {
		Jail string `json:"jail"`
	}
	if err := c.Bind(&body); err != nil || body.Jail == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "jail name required")
	}

	output, err := r.executor.RunShell(ctx, "fail2ban-client", "stop", body.Jail)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "disable failed: "+strings.TrimSpace(output))
	}

	return c.JSON(http.StatusOK, map[string]any{
		"ok":      true,
		"message": "Disabled jail " + body.Jail,
		"output":  strings.TrimSpace(output),
	})
}
