package agenthttp

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"

	"sigardapanel/internal/config"

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

	// Keep SSH, HTTP(S) and the agent reachable, then enable (--force skips
	// the confirmation prompt).
	r.ensureProtectedAllows(ctx)
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
	return r.firewallRuleTask(c, "allow")
}

// firewallDenyTask adds a UFW deny rule.
func (r *Router) firewallDenyTask(c echo.Context) error {
	return r.firewallRuleTask(c, "deny")
}

var (
	ufwPortRe    = regexp.MustCompile(`^[0-9]{1,5}(:[0-9]{1,5})?$`)
	ufwCommentRe = regexp.MustCompile(`^[A-Za-z0-9 ._:/()-]{0,64}$`)
)

// ufwRuleArgs builds the ufw arguments for an allow/deny rule after
// validating every value (they go to ufw as separate argv entries, never a
// shell, but ufw itself rejects malformed rules such as "allow 80 /tcp").
func ufwRuleArgs(action, port, protocol, from, comment string) ([]string, error) {
	port, protocol, from, comment = strings.TrimSpace(port), strings.ToLower(strings.TrimSpace(protocol)), strings.TrimSpace(from), strings.TrimSpace(comment)
	if port == "" {
		port = "any"
	}
	if port != "any" {
		if !ufwPortRe.MatchString(port) {
			return nil, fmt.Errorf("invalid port %q (a number 1-65535 or a range like 6000:6010)", port)
		}
		for _, part := range strings.Split(port, ":") {
			if n, _ := strconv.Atoi(part); n < 1 || n > 65535 {
				return nil, fmt.Errorf("invalid port %q", port)
			}
		}
		if strings.Contains(port, ":") && (protocol != "tcp" && protocol != "udp") {
			return nil, fmt.Errorf("a port range needs protocol tcp or udp")
		}
	}
	switch protocol {
	case "", "any":
		protocol = ""
	case "tcp", "udp":
	default:
		return nil, fmt.Errorf("invalid protocol %q (tcp, udp or any)", protocol)
	}
	if from != "" && from != "any" {
		if strings.Contains(from, "/") {
			if _, _, err := net.ParseCIDR(from); err != nil {
				return nil, fmt.Errorf("invalid source %q", from)
			}
		} else if net.ParseIP(from) == nil {
			return nil, fmt.Errorf("invalid source %q", from)
		}
	} else {
		from = ""
	}
	if !ufwCommentRe.MatchString(comment) {
		return nil, fmt.Errorf("invalid comment (letters, digits, spaces and . _ : / ( ) - only, max 64)")
	}

	var args []string
	switch {
	case from == "" && port == "any":
		return nil, fmt.Errorf("a rule needs a port or a source address")
	case from == "":
		// ufw allow 8080/tcp
		spec := port
		if protocol != "" {
			spec += "/" + protocol
		}
		args = []string{action, spec}
	default:
		// ufw deny from 1.2.3.4 [to any port 22 [proto tcp]]
		args = []string{action, "from", from}
		if port != "any" {
			args = append(args, "to", "any", "port", port)
			if protocol != "" {
				args = append(args, "proto", protocol)
			}
		}
	}
	if comment != "" {
		args = append(args, "comment", comment)
	}
	return args, nil
}

// protectedPorts are the ports a firewall change must never close for
// everyone: SSH (22 and any Port from sshd_config), HTTP(S) and the agent.
func (r *Router) protectedPorts() map[string]string {
	ports := map[string]string{"22": "SSH", "80": "HTTP", "443": "HTTPS"}
	if raw, err := os.ReadFile("/etc/ssh/sshd_config"); err == nil {
		for _, line := range strings.Split(string(raw), "\n") {
			f := strings.Fields(line)
			if len(f) == 2 && strings.EqualFold(f[0], "Port") {
				if n, err := strconv.Atoi(f[1]); err == nil && n > 0 && n < 65536 {
					ports[f[1]] = "SSH"
				}
			}
		}
	}
	if _, port, err := net.SplitHostPort(config.AgentServer().Addr); err == nil && port != "" {
		ports[port] = "SigardaPanel agent"
	}
	return ports
}

func (r *Router) firewallRuleTask(c echo.Context, action string) error {
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
	args, err := ufwRuleArgs(action, payload.Port, payload.Protocol, payload.From, payload.Comment)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	port := strings.TrimSpace(payload.Port)
	from := strings.TrimSpace(payload.From)
	if action == "deny" && (from == "" || from == "any") {
		if name, ok := r.protectedPorts()[port]; ok {
			return echo.NewHTTPError(http.StatusConflict, fmt.Sprintf("refusing to deny port %s (%s) for everyone: it would cut off access to this server", port, name))
		}
	}

	output, err := r.executor.RunShell(ctx, "ufw", args...)
	if err != nil {
		return c.JSON(http.StatusOK, map[string]any{
			"ok":      false,
			"message": fmt.Sprintf("%s failed: %s", action, strings.TrimSpace(output)),
		})
	}

	slog.Info("agent: firewall "+action, "args", strings.Join(args, " "))
	what := "port " + port
	if from != "" && from != "any" {
		what = from
		if port != "" && port != "any" {
			what += " on port " + port
		}
	}
	return c.JSON(http.StatusOK, map[string]any{
		"ok":      true,
		"message": fmt.Sprintf("%s %s", what, map[string]string{"allow": "allowed", "deny": "denied"}[action]),
		"output":  strings.TrimSpace(output),
	})
}

// ensureProtectedAllows adds allow rules for the protected ports, so
// enabling a default-deny firewall never locks the operator out.
func (r *Router) ensureProtectedAllows(ctx context.Context) {
	for port, name := range r.protectedPorts() {
		if out, err := r.executor.RunShell(ctx, "ufw", "allow", port+"/tcp", "comment", name+" (kept open by SigardaPanel)"); err != nil {
			slog.Warn("agent: firewall keep-open rule", "port", port, "error", err, "output", strings.TrimSpace(out))
		}
	}
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

	if listing, err := r.executor.RunShell(ctx, "ufw", "status", "numbered"); err == nil {
		rules := parseUFWNumbered(listing)
		protected := r.protectedPorts()
		for _, rule := range rules {
			if rule.Number != payload.Number || rule.Action != "ALLOW" {
				continue
			}
			if name, ok := protected[rule.Port]; ok {
				others := 0
				for _, o := range rules {
					if o.Number != rule.Number && o.Action == "ALLOW" && o.Port == rule.Port && strings.EqualFold(o.From, "Anywhere") == strings.EqualFold(rule.From, "Anywhere") {
						others++
					}
				}
				if others == 0 {
					return echo.NewHTTPError(http.StatusConflict, fmt.Sprintf("refusing to delete the rule that keeps port %s (%s) open", rule.Port, name))
				}
			}
		}
	}
	output, err := r.executor.RunShell(ctx, "ufw", "--force", "delete", fmt.Sprintf("%d", payload.Number))
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

	rules := parseUFWNumbered(output)
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

// parseUFWNumbered parses "ufw status numbered".
func parseUFWNumbered(output string) []UFWRule {
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

	return rules
}
