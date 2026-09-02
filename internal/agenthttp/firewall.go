package agenthttp

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

// UFWRule represents a UFW firewall rule.
type UFWRule struct {
	Number    int    `json:"number"`
	Port      string `json:"port"`
	Protocol  string `json:"protocol"`
	Action    string `json:"action"`
	Direction string `json:"direction"`
	From      string `json:"from"`
	To        string `json:"to"`
	Comment   string `json:"comment"`
}

// firewallStatusTask returns UFW status.
func (r *Router) firewallStatusTask(c echo.Context) error {
	ctx := c.Request().Context()

	// Check if UFW is installed by trying to get version
	versionOut, err := r.executor.RunShell(ctx, "ufw", "version")
	if err != nil || strings.TrimSpace(versionOut) == "" {
		return c.JSON(http.StatusOK, map[string]any{
			"ok":        true,
			"installed": false,
			"active":    false,
			"message":   "UFW is not installed",
		})
	}

	// Get UFW status
	output, err := r.executor.RunShell(ctx, "ufw", "status")
	if err != nil {
		return c.JSON(http.StatusOK, map[string]any{
			"ok":      false,
			"message": fmt.Sprintf("failed to get status: %s", strings.TrimSpace(output)),
		})
	}

	active := strings.Contains(strings.ToLower(output), "active")
	verbose := strings.Contains(strings.ToLower(output), "verbose")

	return c.JSON(http.StatusOK, map[string]any{
		"ok":        true,
		"installed": true,
		"active":    active,
		"verbose":   verbose,
		"raw":       strings.TrimSpace(output),
	})
}

// firewallEnableTask enables UFW.
func (r *Router) firewallEnableTask(c echo.Context) error {
	ctx := c.Request().Context()

	// Enable UFW with --force to skip confirmation
	output, err := r.executor.RunShell(ctx, "ufw", "--force", "enable")
	if err != nil {
		return c.JSON(http.StatusOK, map[string]any{
			"ok":      false,
			"message": fmt.Sprintf("enable failed: %s", strings.TrimSpace(output)),
		})
	}

	slog.Info("agent: firewall enabled")
	return c.JSON(http.StatusOK, map[string]any{
		"ok":      true,
		"message": "UFW enabled",
		"output":  strings.TrimSpace(output),
	})
}

// firewallDisableTask disables UFW.
func (r *Router) firewallDisableTask(c echo.Context) error {
	ctx := c.Request().Context()

	output, err := r.executor.RunShell(ctx, "ufw", "--force", "disable")
	if err != nil {
		return c.JSON(http.StatusOK, map[string]any{
			"ok":      false,
			"message": fmt.Sprintf("disable failed: %s", strings.TrimSpace(output)),
		})
	}

	slog.Info("agent: firewall disabled")
	return c.JSON(http.StatusOK, map[string]any{
		"ok":      true,
		"message": "UFW disabled",
		"output":  strings.TrimSpace(output),
	})
}

// firewallAllowTask adds a UFW allow rule.
func (r *Router) firewallAllowTask(c echo.Context) error {
	ctx := c.Request().Context()
	var payload struct {
		Port     string `json:"port"`
		Protocol string `json:"protocol"`
		From     string `json:"from"`
		Comment  string `json:"comment"`
	}
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid payload")
	}

	if payload.Port == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "port is required")
	}

	args := []string{"allow"}
	if payload.Port != "" {
		args = append(args, payload.Port)
	}
	if payload.Protocol != "" {
		args = append(args, "/"+payload.Protocol)
	}
	if payload.From != "" {
		args = append(args, "from", payload.From)
	}
	if payload.Comment != "" {
		args = append(args, "comment", payload.Comment)
	}

	output, err := r.executor.RunShell(ctx, "ufw", args...)
	if err != nil {
		return c.JSON(http.StatusOK, map[string]any{
			"ok":      false,
			"message": fmt.Sprintf("allow failed: %s", strings.TrimSpace(output)),
		})
	}

	slog.Info("agent: firewall allow", "port", payload.Port, "protocol", payload.Protocol, "from", payload.From)
	return c.JSON(http.StatusOK, map[string]any{
		"ok":      true,
		"message": fmt.Sprintf("port %s allowed", payload.Port),
		"output":  strings.TrimSpace(output),
	})
}

// firewallDenyTask adds a UFW deny rule.
func (r *Router) firewallDenyTask(c echo.Context) error {
	ctx := c.Request().Context()
	var payload struct {
		Port     string `json:"port"`
		Protocol string `json:"protocol"`
		From     string `json:"from"`
		Comment  string `json:"comment"`
	}
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid payload")
	}

	if payload.Port == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "port is required")
	}

	args := []string{"deny"}
	if payload.Port != "" {
		args = append(args, payload.Port)
	}
	if payload.Protocol != "" {
		args = append(args, "/"+payload.Protocol)
	}
	if payload.From != "" {
		args = append(args, "from", payload.From)
	}
	if payload.Comment != "" {
		args = append(args, "comment", payload.Comment)
	}

	output, err := r.executor.RunShell(ctx, "ufw", args...)
	if err != nil {
		return c.JSON(http.StatusOK, map[string]any{
			"ok":      false,
			"message": fmt.Sprintf("deny failed: %s", strings.TrimSpace(output)),
		})
	}

	slog.Info("agent: firewall deny", "port", payload.Port, "protocol", payload.Protocol, "from", payload.From)
	return c.JSON(http.StatusOK, map[string]any{
		"ok":      true,
		"message": fmt.Sprintf("port %s denied", payload.Port),
		"output":  strings.TrimSpace(output),
	})
}

// firewallDeleteTask deletes a UFW rule by number.
func (r *Router) firewallDeleteTask(c echo.Context) error {
	ctx := c.Request().Context()
	var payload struct {
		Number int `json:"number"`
	}
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid payload")
	}

	if payload.Number <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "rule number is required")
	}

	output, err := r.executor.RunShell(ctx, "ufw", "delete", fmt.Sprintf("%d", payload.Number))
	if err != nil {
		return c.JSON(http.StatusOK, map[string]any{
			"ok":      false,
			"message": fmt.Sprintf("delete failed: %s", strings.TrimSpace(output)),
		})
	}

	slog.Info("agent: firewall delete rule", "number", payload.Number)
	return c.JSON(http.StatusOK, map[string]any{
		"ok":      true,
		"message": fmt.Sprintf("rule %d deleted", payload.Number),
		"output":  strings.TrimSpace(output),
	})
}

// firewallRulesTask lists all UFW rules.
func (r *Router) firewallRulesTask(c echo.Context) error {
	ctx := c.Request().Context()

	output, err := r.executor.RunShell(ctx, "ufw", "status", "numbered")
	if err != nil {
		return c.JSON(http.StatusOK, map[string]any{
			"ok":    false,
			"rules": []UFWRule{},
			"error": err.Error(),
		})
	}

	var rules []UFWRule
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Status") || strings.HasPrefix(line, "To") {
			continue
		}
		// Parse numbered rule:
		// [ 1] 22/tcp                     ALLOW IN    Anywhere                   # SSH
		// [ 2] 80/tcp                     ALLOW IN    Anywhere                   # HTTP
		// [ 3] 443/tcp                    ALLOW IN    Anywhere                   # HTTPS
		// [ 4] 20128/tcp                  ALLOW IN    Anywhere                   # 9router dashboard
		if strings.HasPrefix(line, "[") {
			// Extract number
			numEnd := strings.Index(line, "]")
			if numEnd < 0 {
				continue
			}
			number := 0
			fmt.Sscanf(line[1:numEnd], "%d", &number)

			// Get the rest after "]"
			rest := strings.TrimSpace(line[numEnd+1:])

			// Split by multiple spaces to get fields
			// Format: PORT/PROTO  ACTION  DIRECTION  FROM  TO  # COMMENT
			fields := strings.Fields(rest)
			if len(fields) < 3 {
				continue
			}

			// First field is port/protocol (e.g. "22/tcp" or "22" or "Anywhere")
			portProto := fields[0]
			port := portProto
			protocol := ""
			if idx := strings.Index(portProto, "/"); idx > 0 {
				port = portProto[:idx]
				protocol = portProto[idx+1:]
			}

			// Find ALLOW/DENY
			action := ""
			actionIdx := -1
			for i, f := range fields {
				if f == "ALLOW" || f == "DENY" || f == "REJECT" {
					action = f
					actionIdx = i
					break
				}
			}
			if action == "" {
				continue
			}

			// Direction is next word after action (IN/OUT or empty)
			direction := ""
			from := ""
			to := ""
			comment := ""

			// Check if next word is IN or OUT
			afterAction := ""
			if actionIdx+1 < len(fields) {
				afterAction = fields[actionIdx+1]
			}

			if afterAction == "IN" || afterAction == "OUT" {
				direction = afterAction
				// Remaining fields after direction
				remaining := fields[actionIdx+2:]
				if len(remaining) >= 2 {
					from = remaining[0]
					// Check if second field is comment marker
					if remaining[1] != "#" && !strings.HasPrefix(remaining[1], "#") {
						to = remaining[1]
					}
				} else if len(remaining) == 1 {
					if remaining[0] != "#" && !strings.HasPrefix(remaining[0], "#") {
						to = remaining[0]
					}
				}
			} else {
				// No direction, fields after action are FROM TO
				remaining := fields[actionIdx+1:]
				if len(remaining) >= 2 {
					from = remaining[0]
					if remaining[1] != "#" && !strings.HasPrefix(remaining[1], "#") {
						to = remaining[1]
					}
				} else if len(remaining) == 1 {
					if remaining[0] != "#" && !strings.HasPrefix(remaining[0], "#") {
						to = remaining[0]
					}
				}
			}

			// Extract comment if present
			if idx := strings.Index(line, "#"); idx > 0 {
				comment = strings.TrimSpace(line[idx+1:])
			}

			rules = append(rules, UFWRule{
				Number:    number,
				Port:      port,
				Protocol:  protocol,
				Action:    action,
				Direction: direction,
				From:      from,
				To:        to,
				Comment:   comment,
			})
		}
	}

	return c.JSON(http.StatusOK, map[string]any{
		"ok":    true,
		"rules": rules,
		"total": len(rules),
	})
}

// firewallResetTask resets UFW to defaults.
func (r *Router) firewallResetTask(c echo.Context) error {
	ctx := c.Request().Context()

	output, err := r.executor.RunShell(ctx, "ufw", "--force", "reset")
	if err != nil {
		return c.JSON(http.StatusOK, map[string]any{
			"ok":      false,
			"message": fmt.Sprintf("reset failed: %s", strings.TrimSpace(output)),
		})
	}

	slog.Info("agent: firewall reset")
	return c.JSON(http.StatusOK, map[string]any{
		"ok":      true,
		"message": "UFW reset to defaults",
		"output":  strings.TrimSpace(output),
	})
}
