package agenthttp

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"regexp"
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
	defer readNginxLock()()
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
	defer readNginxLock()()
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

// Bot blocker config files (variables so tests can point them elsewhere).
var (
	botAgentsConf = "/etc/nginx/conf.d/bot-blocker.conf"
	botIPsConf    = "/etc/nginx/conf.d/bot-ips.conf"
)

// botAgentRe: user-agent fragments only (they go into an nginx map as a
// quoted case-insensitive regex).
var botAgentRe = regexp.MustCompile(`^[A-Za-z0-9._ /-]{1,64}$`)

// botRule is one user-agent map entry: 1 blocks, 0 allows (the default
// config allowlists curl/wget).
type botRule struct{ pattern, value string }

// botRuleOf parses a map line in either the current quoted or the old
// unquoted format (ok=false when it is not a pattern line).
func botRuleOf(line string) (botRule, bool) {
	t := strings.TrimSpace(line)
	for _, pre := range []string{"\"~*(", "~*("} {
		if !strings.HasPrefix(t, pre) {
			continue
		}
		rest := t[len(pre):]
		end := strings.LastIndex(rest, ")")
		if end < 0 {
			return botRule{}, false
		}
		value := "1"
		if f := strings.Fields(strings.TrimSuffix(strings.TrimSpace(strings.TrimPrefix(rest[end+1:], "\"")), ";")); len(f) > 0 && (f[0] == "0" || f[0] == "1") {
			value = f[0]
		}
		return botRule{pattern: rest[:end], value: value}, true
	}
	return botRule{}, false
}

func renderBotAgents(rules []botRule) string {
	lines := make([]string, 0, len(rules))
	for _, r := range rules {
		lines = append(lines, fmt.Sprintf("    \"~*(%s)\" %s;", r.pattern, r.value))
	}
	return "# SigardaPanel Bot Blocker - User Agent Map\n" +
		"map $http_user_agent $bad_bot {\n" +
		"    default 0;\n" +
		strings.Join(lines, "\n") + "\n" +
		"    \"\" 1;\n" +
		"}\n"
}

func renderBotIPs(ips []string) string {
	lines := make([]string, 0, len(ips))
	for _, ip := range ips {
		lines = append(lines, "deny "+ip+";")
	}
	return "# SigardaPanel Bot Blocker - IP Blocklist\n" + strings.Join(lines, "\n") + "\n"
}

func currentBotAgents() []botRule {
	content, _ := readFileContent(botAgentsConf)
	var out []botRule
	for _, l := range strings.Split(content, "\n") {
		if r, ok := botRuleOf(l); ok {
			out = append(out, r)
		}
	}
	return out
}

func addBotRules(rules []botRule, agents []string) []botRule {
	have := map[string]bool{}
	for _, r := range rules {
		have[r.pattern] = true
	}
	for _, a := range agents {
		if !have[a] {
			have[a] = true
			rules = append(rules, botRule{pattern: a, value: "1"})
		}
	}
	return rules
}

func removeBotRules(rules []botRule, agents []string) []botRule {
	drop := map[string]bool{}
	for _, a := range agents {
		drop[a] = true
	}
	var out []botRule
	for _, r := range rules {
		if !drop[r.pattern] {
			out = append(out, r)
		}
	}
	return out
}

func currentBotIPs() []string {
	content, _ := readFileContent(botIPsConf)
	var out []string
	for _, l := range strings.Split(content, "\n") {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "deny ") && strings.HasSuffix(t, ";") {
			out = append(out, strings.TrimSuffix(strings.TrimPrefix(t, "deny "), ";"))
		}
	}
	return out
}

// applyNginxConf writes the given files, runs nginx -t and restores the
// previous contents when the test fails (a config nginx cannot parse must
// never stay on disk: every later reload would fail).
func (r *Router) applyNginxConf(ctx context.Context, files map[string]string) error {
	defer readNginxLock()()
	type prev struct {
		data   []byte
		exists bool
	}
	old := map[string]prev{}
	for path, content := range files {
		data, err := os.ReadFile(path)
		old[path] = prev{data: data, exists: err == nil}
		if err := writeFileSecure(path, content, 0644); err != nil {
			for p, o := range old {
				if o.exists {
					_ = os.WriteFile(p, o.data, 0644)
				} else {
					_ = os.Remove(p)
				}
			}
			return fmt.Errorf("write %s: %w", path, err)
		}
	}
	if out, err := r.executor.RunShell(ctx, "nginx", "-t"); err != nil {
		for p, o := range old {
			if o.exists {
				_ = os.WriteFile(p, o.data, 0644)
			} else {
				_ = os.Remove(p)
			}
		}
		return fmt.Errorf("nginx config test failed, change reverted: %s", strings.TrimSpace(out))
	}
	r.executor.RunShell(ctx, "systemctl", "reload", "nginx")
	return nil
}

func uniqueAppend(list []string, add ...string) []string {
	seen := map[string]bool{}
	for _, v := range list {
		seen[v] = true
	}
	for _, v := range add {
		if !seen[v] {
			seen[v] = true
			list = append(list, v)
		}
	}
	return list
}

func without(list []string, drop []string) []string {
	rm := map[string]bool{}
	for _, d := range drop {
		rm[d] = true
	}
	var out []string
	for _, v := range list {
		if !rm[v] {
			out = append(out, v)
		}
	}
	return out
}

// addBotAgent adds User-Agent patterns to block.
func (r *Router) addBotAgent(c echo.Context, agents []string) error {
	for _, agent := range agents {
		if !botAgentRe.MatchString(agent) {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid agent pattern (letters, digits, space . _ / - only): "+agent)
		}
	}
	if content, _ := readFileContent(botAgentsConf); content == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot blocker not enabled")
	}
	all := addBotRules(currentBotAgents(), agents)
	if err := r.applyNginxConf(c.Request().Context(), map[string]string{botAgentsConf: renderBotAgents(all)}); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]any{"ok": true, "message": fmt.Sprintf("Added %d bot agent patterns", len(agents))})
}

// removeBotAgent removes User-Agent patterns.
func (r *Router) removeBotAgent(c echo.Context, agents []string) error {
	if content, _ := readFileContent(botAgentsConf); content == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot blocker not enabled")
	}
	rest := removeBotRules(currentBotAgents(), agents)
	if err := r.applyNginxConf(c.Request().Context(), map[string]string{botAgentsConf: renderBotAgents(rest)}); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]any{"ok": true, "message": fmt.Sprintf("Removed %d bot agent patterns", len(agents))})
}

// addBotIP adds IPs to block.
func (r *Router) addBotIP(c echo.Context, ips []string) error {
	for _, ip := range ips {
		if !isValidIP(ip) {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid IP: "+ip)
		}
	}
	all := uniqueAppend(currentBotIPs(), ips...)
	if err := r.applyNginxConf(c.Request().Context(), map[string]string{botIPsConf: renderBotIPs(all)}); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]any{"ok": true, "message": fmt.Sprintf("Added %d IPs to blocklist", len(ips))})
}

// removeBotIP removes IPs from the blocklist (exact matches only).
func (r *Router) removeBotIP(c echo.Context, ips []string) error {
	var valid []string
	for _, ip := range ips {
		if ip = strings.TrimSpace(ip); ip != "" {
			valid = append(valid, ip)
		}
	}
	if len(valid) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "no IP to remove")
	}
	rest := without(currentBotIPs(), valid)
	if err := r.applyNginxConf(c.Request().Context(), map[string]string{botIPsConf: renderBotIPs(rest)}); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]any{"ok": true, "message": fmt.Sprintf("Removed %d IPs from blocklist", len(valid))})
}

// updateBotBlocker replaces the whole bot blocker configuration.
func (r *Router) updateBotBlocker(c echo.Context, agents, ips []string) error {
	var goodAgents []botRule
	var goodIPs []string
	for _, agent := range agents {
		if !botAgentRe.MatchString(agent) {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid agent pattern: "+agent)
		}
	}
	goodAgents = addBotRules(nil, agents)
	for _, ip := range ips {
		if !isValidIP(ip) {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid IP: "+ip)
		}
		goodIPs = uniqueAppend(goodIPs, ip)
	}
	if err := r.applyNginxConf(c.Request().Context(), map[string]string{
		botAgentsConf: renderBotAgents(goodAgents),
		botIPsConf:    renderBotIPs(goodIPs),
	}); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]any{"ok": true, "message": "Bot blocker updated"})
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
