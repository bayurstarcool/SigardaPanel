package agenthttp

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/labstack/echo/v4"
)

// ─── Threat-monitor collectors ──────────────────────────────────────────────
//
// These endpoints give the panel a read-only view of compromise indicators on
// the server the agent runs on: SSH brute-force activity, integrity of
// security-critical files, suspicious outbound connections, persistence
// entries (cron/systemd/authorized_keys) and web-attack patterns in the nginx
// access log. Everything is parsed read-only; nothing is modified here.

const (
	authLogLimitLines  = 20000
	nginxLogLimitLines = 50000
)

// ─── SSH activity ───────────────────────────────────────────────────────────

var (
	reSSHFailed   = regexp.MustCompile(`Failed (\S+) for (?:invalid user )?(\S+) from (\S+) port`)
	reSSHAccepted = regexp.MustCompile(`Accepted (\S+) for (\S+) from (\S+) port`)
	reSSHInvalid  = regexp.MustCompile(`Invalid user (\S+) from (\S+)`)
)

type sshAttacker struct {
	IP        string   `json:"ip"`
	Count     int      `json:"count"`
	Usernames []string `json:"usernames,omitempty"`
}

type sshLogin struct {
	User   string    `json:"user"`
	IP     string    `json:"ip"`
	Method string    `json:"method"`
	Time   time.Time `json:"time"`
}

type sshActivity struct {
	FailedTotal   int           `json:"failed_total"`
	AcceptedTotal int           `json:"accepted_total"`
	InvalidTotal  int           `json:"invalid_user_total"`
	UniqueIPs     int           `json:"unique_attacker_ips"`
	TopAttackers  []sshAttacker `json:"top_attackers"`
	RecentLogins  []sshLogin    `json:"recent_accepted"`
	LogSources    []string      `json:"log_sources"`

	attackers map[string]*sshAttacker
}

// GET /tasks/security/ssh-activity
func (r *Router) securitySSHActivity(c echo.Context) error {
	activity := parseSSHLogs([]string{"/var/log/auth.log", "/var/log/auth.log.1", "/var/log/secure", "/var/log/secure.1"})
	if len(activity.LogSources) == 0 {
		// Minimal Ubuntu 24.04 images run no rsyslog; the journal is the
		// only SSH log source.
		if journal := readSSHJournal(c.Request().Context()); journal != "" {
			parseSSHReader(strings.NewReader(journal), &activity)
			activity.LogSources = []string{"journalctl -u ssh"}
			finalizeSSHActivity(&activity)
		}
	}
	return c.JSON(http.StatusOK, map[string]any{"ok": true, "data": activity})
}

func parseSSHLogs(paths []string) sshActivity {
	act := sshActivity{TopAttackers: []sshAttacker{}, RecentLogins: []sshLogin{}, LogSources: []string{}}

	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		act.LogSources = append(act.LogSources, p)
		parseSSHReader(f, &act)
		f.Close()
	}

	finalizeSSHActivity(&act)
	return act
}

func readSSHJournal(ctx context.Context) string {
	jctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(jctx, "journalctl", "-u", "ssh", "-u", "sshd", "--no-pager", "-n", strconv.Itoa(authLogLimitLines)).Output()
	if err != nil {
		return ""
	}
	return string(out)
}

// parseSSHReader accumulates events from one log source into act.
func parseSSHReader(reader io.Reader, act *sshActivity) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 256*1024)
	lines := 0
	for scanner.Scan() {
		lines++
		if lines > authLogLimitLines {
			break
		}
		line := scanner.Text()
		ts := parseSyslogTime(line)
		if m := reSSHFailed.FindStringSubmatch(line); m != nil {
			act.FailedTotal++
			act.addAttacker(m[3], m[2])
		} else if m := reSSHInvalid.FindStringSubmatch(line); m != nil {
			act.InvalidTotal++
			act.addAttacker(m[2], m[1])
		} else if m := reSSHAccepted.FindStringSubmatch(line); m != nil {
			act.AcceptedTotal++
			act.RecentLogins = append(act.RecentLogins, sshLogin{User: m[2], IP: m[3], Method: m[1], Time: ts})
		}
	}
}

func (a *sshActivity) addAttacker(ip, username string) {
	if net.ParseIP(ip) == nil {
		return
	}
	if a.attackers == nil {
		a.attackers = map[string]*sshAttacker{}
	}
	att, ok := a.attackers[ip]
	if !ok {
		att = &sshAttacker{IP: ip}
		a.attackers[ip] = att
	}
	att.Count++
	if username != "" && len(att.Usernames) < 5 {
		dup := false
		for _, u := range att.Usernames {
			if u == username {
				dup = true
				break
			}
		}
		if !dup {
			att.Usernames = append(att.Usernames, username)
		}
	}
}

func finalizeSSHActivity(act *sshActivity) {
	for _, a := range act.attackers {
		act.TopAttackers = append(act.TopAttackers, *a)
	}
	sort.Slice(act.TopAttackers, func(i, j int) bool { return act.TopAttackers[i].Count > act.TopAttackers[j].Count })
	if len(act.TopAttackers) > 15 {
		act.TopAttackers = act.TopAttackers[:15]
	}
	act.UniqueIPs = len(act.attackers)
	if len(act.RecentLogins) > 10 {
		act.RecentLogins = act.RecentLogins[len(act.RecentLogins)-10:]
	}
}

func addAttacker(m map[string]*sshAttacker, ip, username string) {
	if net.ParseIP(ip) == nil {
		return
	}
	a, ok := m[ip]
	if !ok {
		a = &sshAttacker{IP: ip}
		m[ip] = a
	}
	a.Count++
	if username != "" && len(a.Usernames) < 5 {
		dup := false
		for _, u := range a.Usernames {
			if u == username {
				dup = true
				break
			}
		}
		if !dup {
			a.Usernames = append(a.Usernames, username)
		}
	}
}

var months = map[string]time.Month{"Jan": 1, "Feb": 2, "Mar": 3, "Apr": 4, "May": 5, "Jun": 6, "Jul": 7, "Aug": 8, "Sep": 9, "Oct": 10, "Nov": 11, "Dec": 12}

func parseSyslogTime(line string) time.Time {
	// "Oct 25 14:22:01 host sshd..." — year missing, use current year.
	if len(line) > 15 {
		if mon, ok := months[line[0:3]]; ok {
			day, err1 := strconv.Atoi(line[4:6])
			hh, err2 := strconv.Atoi(line[7:9])
			mm, err3 := strconv.Atoi(line[10:12])
			ss, err4 := strconv.Atoi(line[13:15])
			if err1 == nil && err2 == nil && err3 == nil && err4 == nil {
				return time.Date(time.Now().Year(), mon, day, hh, mm, ss, 0, time.Local)
			}
		}
	}
	return time.Time{}
}

// ─── File integrity ─────────────────────────────────────────────────────────

type integrityEntry struct {
	Path    string `json:"path"`
	Exists  bool   `json:"exists"`
	SHA256  string `json:"sha256,omitempty"`
	Mode    string `json:"mode,omitempty"`
	UID     int    `json:"uid,omitempty"`
	Size    int64  `json:"size,omitempty"`
	ModTime string `json:"mod_time,omitempty"`
}

var integrityWatchList = []string{
	"/etc/passwd",
	"/etc/group",
	"/etc/shadow",
	"/etc/gshadow",
	"/etc/sudoers",
	"/etc/ssh/sshd_config",
	"/etc/crontab",
	"/root/.ssh/authorized_keys",
}

// GET /tasks/security/integrity
func (r *Router) securityIntegrity(c echo.Context) error {
	paths := append([]string{}, integrityWatchList...)

	// Whole watched directories, expanded one level.
	for _, dir := range []string{"/etc/sudoers.d", "/etc/cron.d", "/etc/ssh/sshd_config.d"} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				paths = append(paths, filepath.Join(dir, e.Name()))
			}
		}
	}

	out := make([]integrityEntry, 0, len(paths))
	for _, p := range paths {
		out = append(out, hashFile(p))
	}
	return c.JSON(http.StatusOK, map[string]any{"ok": true, "data": out})
}

func hashFile(path string) integrityEntry {
	entry := integrityEntry{Path: path}
	info, err := os.Stat(path)
	if err != nil {
		return entry
	}
	entry.Exists = true
	entry.Mode = info.Mode().String()
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		entry.UID = int(st.Uid)
	}
	entry.Size = info.Size()
	entry.ModTime = info.ModTime().UTC().Format(time.RFC3339)

	f, err := os.Open(path)
	if err != nil {
		return entry
	}
	defer f.Close()
	h := sha256.New()
	buf := make([]byte, 128*1024)
	for {
		n, err := f.Read(buf)
		if n > 0 {
			h.Write(buf[:n])
		}
		if err != nil {
			break
		}
	}
	entry.SHA256 = hex.EncodeToString(h.Sum(nil))
	return entry
}

// ─── Outbound connections ───────────────────────────────────────────────────

type connEndpoint struct {
	Remote     string `json:"remote"`
	RemoteIP   string `json:"remote_ip"`
	RemotePort int    `json:"remote_port"`
	Count      int    `json:"count"`
	Suspicious bool   `json:"suspicious"`
	Reason     string `json:"reason,omitempty"`
}

type connectionsReport struct {
	Established []connEndpoint `json:"established"`
	Suspicious  []connEndpoint `json:"suspicious_only"`
	Total       int            `json:"total"`
}

// Ports commonly used by cryptominers, reverse shells and known botnet C2s.
var suspiciousPorts = map[int]string{
	3333:  "cryptominer (Stratum)",
	4444:  "Metasploit/reverse shell",
	5555:  "reverse shell / botnet",
	6666:  "botnet C2",
	7777:  "botnet C2",
	8333:  "bitcoin",
	9999:  "reverse shell",
	14444: "cryptominer",
	14433: "cryptominer",
	45700: "Monero miner pool",
}

// GET /tasks/security/connections
func (r *Router) securityConnections(c echo.Context) error {
	report := connectionsReport{Established: []connEndpoint{}, Suspicious: []connEndpoint{}}
	endpoints := map[string]*connEndpoint{}

	parseProcNetTCP("/proc/net/tcp", endpoints)
	parseProcNetTCP("/proc/net/tcp6", endpoints)

	for _, ep := range endpoints {
		report.Established = append(report.Established, *ep)
		if ep.Suspicious {
			report.Suspicious = append(report.Suspicious, *ep)
		}
	}
	sort.Slice(report.Established, func(i, j int) bool {
		if report.Established[i].Suspicious != report.Established[j].Suspicious {
			return report.Established[i].Suspicious
		}
		return report.Established[i].Count > report.Established[j].Count
	})
	if len(report.Established) > 60 {
		report.Established = report.Established[:60]
	}
	report.Total = len(endpoints)
	return c.JSON(http.StatusOK, map[string]any{"ok": true, "data": report})
}

func parseProcNetTCP(path string, endpoints map[string]*connEndpoint) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n")[1:] {
		fields := strings.Fields(line)
		if len(fields) < 4 || fields[3] != "01" { // 01 = ESTABLISHED
			continue
		}
		remote := parseHexSockaddr(fields[2])
		if remote == nil {
			continue
		}
		key := remote.String()
		ep, ok := endpoints[key]
		if !ok {
			ep = &connEndpoint{Remote: key, RemoteIP: remote.ip, RemotePort: remote.port}
			endpoints[key] = ep
		}
		ep.Count++
		if reason, sus := suspiciousPorts[remote.port]; sus {
			ep.Suspicious = true
			ep.Reason = reason
		}
	}
}

type hexSockaddr struct {
	ip   string
	port int
}

func (h hexSockaddr) String() string { return net.JoinHostPort(h.ip, strconv.Itoa(h.port)) }

func parseHexSockaddr(s string) *hexSockaddr {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return nil
	}
	port, err := strconv.ParseInt(parts[1], 16, 32)
	if err != nil {
		return nil
	}
	ip, err := hexToIP(parts[0])
	if err != nil {
		return nil
	}
	// Skip loopback and local side of internal connections.
	if ip.IsLoopback() {
		return nil
	}
	return &hexSockaddr{ip: ip.String(), port: int(port)}
}

func hexToIP(hexStr string) (net.IP, error) {
	b, err := hex.DecodeString(hexStr)
	if err != nil {
		return nil, err
	}
	if len(b) == 4 {
		// /proc/net/tcp is little-endian per component.
		return net.IP{b[3], b[2], b[1], b[0]}, nil
	}
	if len(b) == 16 {
		out := make(net.IP, 16)
		for i := 0; i < 16; i += 4 {
			out[i], out[i+1], out[i+2], out[i+3] = b[i+3], b[i+2], b[i+1], b[i]
		}
		return out, nil
	}
	return nil, fmt.Errorf("unexpected address length %d", len(b))
}

// ─── Persistence audit ──────────────────────────────────────────────────────

type persistenceReport struct {
	CronFiles      []persistenceItem `json:"cron_files"`
	SystemdTimers  []persistenceItem `json:"systemd_timers"`
	AuthorizedKeys []persistenceItem `json:"authorized_keys"`
	UIDZeroUsers   []string          `json:"uid_zero_users"`
}

type persistenceItem struct {
	Owner string `json:"owner,omitempty"`
	Path  string `json:"path"`
	Note  string `json:"note,omitempty"`
}

// GET /tasks/security/persistence
func (r *Router) securityPersistence(c echo.Context) error {
	report := persistenceReport{
		CronFiles:      []persistenceItem{},
		SystemdTimers:  []persistenceItem{},
		AuthorizedKeys: []persistenceItem{},
		UIDZeroUsers:   []string{},
	}

	// System crontabs
	for _, p := range cronFilesIn("/etc/crontab", "/etc/cron.d") {
		report.CronFiles = append(report.CronFiles, persistenceItem{Path: p, Note: "system"})
	}
	// Per-user crontabs (owner from the spool file name)
	if entries, err := os.ReadDir("/var/spool/cron/crontabs"); err == nil {
		for _, e := range entries {
			if !e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
				report.CronFiles = append(report.CronFiles, persistenceItem{Owner: e.Name(), Path: "/var/spool/cron/crontabs/" + e.Name()})
			}
		}
	}

	// systemd timers shipped as units
	for _, dir := range []string{"/etc/systemd/system"} {
		if entries, err := os.ReadDir(dir); err == nil {
			for _, e := range entries {
				n := e.Name()
				if !e.IsDir() && (strings.HasSuffix(n, ".timer")) {
					report.SystemdTimers = append(report.SystemdTimers, persistenceItem{Path: dir + "/" + n})
				}
			}
		}
	}

	// authorized_keys per real user
	if current, err := user.Current(); err == nil {
		report.AuthorizedKeys = append(report.AuthorizedKeys, scanAuthorizedKeys(current.Username, current.HomeDir)...)
	}
	if homes, err := os.ReadDir("/home"); err == nil {
		for _, h := range homes {
			if h.IsDir() {
				report.AuthorizedKeys = append(report.AuthorizedKeys, scanAuthorizedKeys(h.Name(), "/home/"+h.Name())...)
			}
		}
	}

	// Accounts with UID 0 (root-equivalent)
	if passwd, err := os.ReadFile("/etc/passwd"); err == nil {
		for _, line := range strings.Split(string(passwd), "\n") {
			f := strings.Split(line, ":")
			if len(f) >= 3 && f[2] == "0" {
				report.UIDZeroUsers = append(report.UIDZeroUsers, f[0])
			}
		}
	}

	return c.JSON(http.StatusOK, map[string]any{"ok": true, "data": report})
}

func cronFilesIn(paths ...string) []string {
	var out []string
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			continue
		}
		if info.IsDir() {
			if entries, err := os.ReadDir(p); err == nil {
				for _, e := range entries {
					if !e.IsDir() {
						out = append(out, p+"/"+e.Name())
					}
				}
			}
		} else {
			out = append(out, p)
		}
	}
	return out
}

func scanAuthorizedKeys(username, homeDir string) []persistenceItem {
	var out []persistenceItem
	p := filepath.Join(homeDir, ".ssh", "authorized_keys")
	f, err := os.Open(p)
	if err != nil {
		return out
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		flds := strings.Fields(line)
		note := ""
		if len(flds) >= 3 {
			note = flds[0] + " " + flds[2]
		} else if len(flds) == 2 {
			note = flds[0]
		}
		out = append(out, persistenceItem{Owner: username, Path: p, Note: note})
	}
	return out
}

// ─── Web attack patterns (nginx access log) ─────────────────────────────────

type webAttackReport struct {
	LogSources   []string       `json:"log_sources"`
	LinesScanned int            `json:"lines_scanned"`
	Patterns     map[string]int `json:"pattern_hits"`
	TopOffenders []sshAttacker  `json:"top_offenders"`
}

var webAttackPatterns = []struct {
	name    string
	pattern *regexp.Regexp
}{
	{"sqlmap", regexp.MustCompile(`(?i)sqlmap/`)},
	{"nikto", regexp.MustCompile(`(?i)nikto`)},
	{"wpscan/wp-login probe", regexp.MustCompile(`(?i)wp-login\.php|wp-admin|xmlrpc\.php|/\.env\b`)},
	{"path traversal", regexp.MustCompile(`\.\./|\.\.%2f|%2e%2e%2f`)},
	{"sql injection probe", regexp.MustCompile(`(?i)(union%20select|union\+select|or%201=1|'\s*or\s*'1'='1)`)},
	{"shell/script probe", regexp.MustCompile(`(?i)/(phpmyadmin|adminer|shell|cmd|cgi-bin|\.git/config|\.aws/credentials)`)},
	{"rce probe", regexp.MustCompile(`(?i)(eval\(|base64_decode|system\()\s*\?`)},
}

// GET /tasks/security/web-attacks
func (r *Router) securityWebAttacks(c echo.Context) error {
	report := webAttackReport{LogSources: []string{}, Patterns: map[string]int{}, TopOffenders: []sshAttacker{}}
	offenders := map[string]*sshAttacker{}

	for _, p := range []string{"/var/log/nginx/access.log", "/var/log/nginx/access.log.1"} {
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		report.LogSources = append(report.LogSources, p)
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 0, 64*1024), 256*1024)
		lines := 0
		for scanner.Scan() {
			lines++
			if lines > nginxLogLimitLines {
				break
			}
			line := scanner.Text()
			ip := firstLogField(line)
			hit := false
			for _, pat := range webAttackPatterns {
				if pat.pattern.MatchString(line) {
					report.Patterns[pat.name]++
					hit = true
				}
			}
			if hit && ip != "" {
				addAttacker(offenders, ip, "")
			}
		}
		f.Close()
		report.LinesScanned += lines
	}

	for _, a := range offenders {
		report.TopOffenders = append(report.TopOffenders, *a)
	}
	sort.Slice(report.TopOffenders, func(i, j int) bool { return report.TopOffenders[i].Count > report.TopOffenders[j].Count })
	if len(report.TopOffenders) > 15 {
		report.TopOffenders = report.TopOffenders[:15]
	}
	return c.JSON(http.StatusOK, map[string]any{"ok": true, "data": report})
}

func firstLogField(line string) string {
	if i := strings.IndexByte(line, ' '); i > 0 {
		return line[:i]
	}
	return ""
}
