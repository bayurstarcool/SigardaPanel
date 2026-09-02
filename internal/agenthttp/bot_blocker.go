package agenthttp

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/labstack/echo/v4"
)

// manageBotBlocker handles bot blocking via nginx.
func (r *Router) manageBotBlocker(c echo.Context) error {

	var payload struct {
		Action string   `json:"action"`
		Agents []string `json:"agents"`
		IPs    []string `json:"ips"`
	}
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid payload")
	}

	switch payload.Action {
	case "enable":
		return r.enableBotBlocker(c)
	case "disable":
		return r.disableBotBlocker(c)
	case "list":
		return r.listBotBlockerRules(c)
	case "add-agent":
		return r.addBotAgent(c, payload.Agents)
	case "remove-agent":
		return r.removeBotAgent(c, payload.Agents)
	case "add-ip":
		return r.addBotIP(c, payload.IPs)
	case "remove-ip":
		return r.removeBotIP(c, payload.IPs)
	case "update":
		return r.updateBotBlocker(c, payload.Agents, payload.IPs)
	default:
		return echo.NewHTTPError(http.StatusBadRequest, "unknown action: "+payload.Action)
	}
}

// enableBotBlocker enables bot blocking by injecting checks into server blocks.
func (r *Router) enableBotBlocker(c echo.Context) error {
	ctx := c.Request().Context()

	// 1. Create map config for user-agent blocking
	mapConfig := "# SigardaPanel Bot Blocker - User Agent Map\n" +
		"map $http_user_agent $bad_bot {\n" +
		"    default 0;\n" +
		"    ~*(bot|crawl|spider|scraper|harvest|collect|grab|extract|email|siphon) 1;\n" +
		"    ~*(nikto|sqlmap|nmap|masscan|zgrab|dirbuster|gobuster|wfuzz) 1;\n" +
		"    ~*(python-requests|python-urllib|go-http-client|java/|perl|ruby) 1;\n" +
		"    ~*(curl|wget|libwww|lwp-trivial|urllib) 0;\n" +
		"    \"\" 1;\n" +
		"}\n"

	if err := writeFileSecure("/etc/nginx/conf.d/bot-blocker.conf", mapConfig, 0644); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to write bot blocker config")
	}

	// 2. Inject if ($bad_bot) into ALL existing site configs
	siteFiles, _ := r.executor.RunShell(ctx, "sh", "-c", "ls /etc/nginx/sites-enabled/ 2>/dev/null")
	if siteFiles != "" {
		files := strings.Split(strings.TrimSpace(string(siteFiles)), "\n")
		for _, f := range files {
			if f == "" || f == "default" {
				continue
			}
			injectBotBlockInclude("/etc/nginx/sites-enabled/" + f)
		}
	}

	// 3. Test nginx config
	if _, err := r.executor.RunShell(ctx, "nginx", "-t"); err != nil {
		r.executor.RunShell(ctx, "rm", "-f", "/etc/nginx/conf.d/bot-blocker.conf")
		for _, f := range strings.Split(strings.TrimSpace(string(siteFiles)), "\n") {
			if f != "" {
				removeBotBlockInclude("/etc/nginx/sites-enabled/" + f)
			}
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "nginx config test failed: "+err.Error())
	}

	r.executor.RunShell(ctx, "systemctl", "reload", "nginx")
	slog.Info("bot-blocker: enabled")

	return c.JSON(http.StatusOK, map[string]any{
		"ok":      true,
		"message": "Bot blocker enabled",
	})
}

// disableBotBlocker disables the bot blocker.
func (r *Router) disableBotBlocker(c echo.Context) error {
	ctx := c.Request().Context()

	r.executor.RunShell(ctx, "rm", "-f", "/etc/nginx/conf.d/bot-blocker.conf")

	siteFiles, _ := r.executor.RunShell(ctx, "sh", "-c", "ls /etc/nginx/sites-enabled/ 2>/dev/null")
	if siteFiles != "" {
		files := strings.Split(strings.TrimSpace(string(siteFiles)), "\n")
		for _, f := range files {
			if f != "" {
				removeBotBlockInclude("/etc/nginx/sites-enabled/" + f)
			}
		}
	}

	if _, err := r.executor.RunShell(ctx, "nginx", "-t"); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "nginx config test failed: "+err.Error())
	}
	r.executor.RunShell(ctx, "systemctl", "reload", "nginx")
	slog.Info("bot-blocker: disabled")

	return c.JSON(http.StatusOK, map[string]any{
		"ok":      true,
		"message": "Bot blocker disabled",
	})
}

// listBotBlockerRules lists current bot blocker rules.
func (r *Router) listBotBlockerRules(c echo.Context) error {
	content, _ := readFileContent("/etc/nginx/conf.d/bot-blocker.conf")
	return c.JSON(http.StatusOK, map[string]any{
		"enabled": content != "",
		"config":  content,
	})
}

// addBotAgent adds User-Agent patterns to block.
func (r *Router) addBotAgent(c echo.Context, agents []string) error {
	ctx := c.Request().Context()

	for _, agent := range agents {
		if !isShellSafe(agent) {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid agent pattern: "+agent)
		}
	}

	currentConfig, _ := readFileContent("/etc/nginx/conf.d/bot-blocker.conf")
	if currentConfig == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot blocker not enabled")
	}

	var newLines []string
	for _, agent := range agents {
		newLines = append(newLines, fmt.Sprintf("    ~*(%s) 1;", agent))
	}

	// Find the LAST closing brace to insert before it
	lastBrace := strings.LastIndex(currentConfig, "}")
	if lastBrace < 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid nginx config format")
	}
	config := currentConfig[:lastBrace] + strings.Join(newLines, "\n") + "\n" + currentConfig[lastBrace:]

	if err := writeFileSecure("/etc/nginx/conf.d/bot-blocker.conf", config, 0644); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to update config")
	}

	if _, err := r.executor.RunShell(ctx, "nginx", "-t"); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "nginx config test failed")
	}
	r.executor.RunShell(ctx, "systemctl", "reload", "nginx")

	return c.JSON(http.StatusOK, map[string]any{
		"ok":      true,
		"message": fmt.Sprintf("Added %d bot agent patterns", len(agents)),
	})
}

// removeBotAgent removes User-Agent patterns.
func (r *Router) removeBotAgent(c echo.Context, agents []string) error {
	ctx := c.Request().Context()

	currentConfig, _ := readFileContent("/etc/nginx/conf.d/bot-blocker.conf")
	if currentConfig == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot blocker not enabled")
	}

	config := currentConfig
	for _, agent := range agents {
		config = strings.ReplaceAll(config, fmt.Sprintf("    ~*(%s) 1;", agent), "")
	}

	if err := writeFileSecure("/etc/nginx/conf.d/bot-blocker.conf", config, 0644); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to update config")
	}

	if _, err := r.executor.RunShell(ctx, "nginx", "-t"); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "nginx config test failed")
	}
	r.executor.RunShell(ctx, "systemctl", "reload", "nginx")

	return c.JSON(http.StatusOK, map[string]any{
		"ok":      true,
		"message": fmt.Sprintf("Removed %d bot agent patterns", len(agents)),
	})
}

// addBotIP adds IPs to block.
func (r *Router) addBotIP(c echo.Context, ips []string) error {
	ctx := c.Request().Context()

	for _, ip := range ips {
		if !isValidIP(ip) {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid IP: "+ip)
		}
	}

	var denyLines []string
	for _, ip := range ips {
		denyLines = append(denyLines, fmt.Sprintf("deny %s;", ip))
	}

	config := "# SigardaPanel Bot Blocker - IP Blocklist\n" + strings.Join(denyLines, "\n") + "\n"

	if err := appendFileSecure("/etc/nginx/conf.d/bot-ips.conf", config, 0644); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to add IPs")
	}

	if _, err := r.executor.RunShell(ctx, "nginx", "-t"); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "nginx config test failed")
	}
	r.executor.RunShell(ctx, "systemctl", "reload", "nginx")

	return c.JSON(http.StatusOK, map[string]any{
		"ok":      true,
		"message": fmt.Sprintf("Added %d IPs to blocklist", len(ips)),
	})
}

// removeBotIP removes IPs from blocklist.
func (r *Router) removeBotIP(c echo.Context, ips []string) error {
	ctx := c.Request().Context()

	for _, ip := range ips {
		removeLineFromFile("/etc/nginx/conf.d/bot-ips.conf", ip)
	}

	if _, err := r.executor.RunShell(ctx, "nginx", "-t"); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "nginx config test failed")
	}
	r.executor.RunShell(ctx, "systemctl", "reload", "nginx")

	return c.JSON(http.StatusOK, map[string]any{
		"ok":      true,
		"message": fmt.Sprintf("Removed %d IPs from blocklist", len(ips)),
	})
}

// updateBotBlocker updates the entire bot blocker configuration.
func (r *Router) updateBotBlocker(c echo.Context, agents, ips []string) error {
	ctx := c.Request().Context()

	var agentLines []string
	for _, agent := range agents {
		if isShellSafe(agent) {
			agentLines = append(agentLines, fmt.Sprintf("    ~*(%s) 1;", agent))
		}
	}

	config := "# SigardaPanel Bot Blocker - User Agent Map\n" +
		"map $http_user_agent $bad_bot {\n" +
		"    default 0;\n" +
		strings.Join(agentLines, "\n") + "\n" +
		"    \"\" 1;\n" +
		"}\n"

	if err := writeFileSecure("/etc/nginx/conf.d/bot-blocker.conf", config, 0644); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to write config")
	}

	var ipLines []string
	for _, ip := range ips {
		if isValidIP(ip) {
			ipLines = append(ipLines, fmt.Sprintf("deny %s;", ip))
		}
	}

	ipConfig := "# SigardaPanel Bot Blocker - IP Blocklist\n" + strings.Join(ipLines, "\n") + "\n"
	if err := writeFileSecure("/etc/nginx/conf.d/bot-ips.conf", ipConfig, 0644); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to write IP config")
	}

	if _, err := r.executor.RunShell(ctx, "nginx", "-t"); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "nginx config test failed")
	}
	r.executor.RunShell(ctx, "systemctl", "reload", "nginx")

	return c.JSON(http.StatusOK, map[string]any{
		"ok":      true,
		"message": "Bot blocker updated",
	})
}

// --- Helper functions ---

// injectBotBlockInclude adds bot-block checking to a server block.
func injectBotBlockInclude(configPath string) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return
	}
	content := string(data)

	// Skip if already injected
	if strings.Contains(content, "SigardaPanel Bot Blocker") {
		return
	}

	// Find the last closing brace and insert before it
	lastBrace := strings.LastIndex(content, "}")
	if lastBrace < 0 {
		return
	}

	inject := "    # SigardaPanel Bot Blocker\n" +
		"    if ($bad_bot) {\n" +
		"        return 403;\n" +
		"    }\n\n"

	newContent := content[:lastBrace] + inject + content[lastBrace:]
	os.WriteFile(configPath, []byte(newContent), 0644)
}

// removeBotBlockInclude removes bot-block checking from a server block.
func removeBotBlockInclude(configPath string) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return
	}
	content := string(data)

	startMarker := "    # SigardaPanel Bot Blocker"
	start := strings.Index(content, startMarker)
	if start < 0 {
		return
	}

	// Find the end of the if block
	endBlock := strings.Index(content[start:], "}")
	if endBlock < 0 {
		return
	}
	end := start + endBlock + 1

	// Consume trailing newlines
	for end < len(content) && content[end] == '\n' {
		end++
	}

	newContent := content[:start] + content[end:]
	os.WriteFile(configPath, []byte(newContent), 0644)
}

func readFileContent(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// isValidIP validates an IPv4 or IPv6 address.
func isValidIP(ip string) bool {
	return net.ParseIP(ip) != nil
}
