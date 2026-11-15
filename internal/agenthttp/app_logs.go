package agenthttp

import (
	"context"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Every app (Node.js, Python, Go) writes its stdout and stderr to its own
// file, so the site's Logs tab shows that app's output only. The directory
// is root's (logs can hold secrets); systemd opens the file before it
// drops to the site user, and one logrotate rule rotates them all.

const appLogDir = "/var/log/sigardapanel/apps"

const appLogrotatePath = "/etc/logrotate.d/sigardapanel-apps"

const appLogrotateConfig = `# Written by SigardaPanel: output of the sites' apps.
/var/log/sigardapanel/apps/*.log {
    weekly
    maxsize 50M
    rotate 4
    missingok
    notifempty
    compress
    delaycompress
    copytruncate
    su root root
}
`

// appLogRotation matches what logrotate appends to a rotated app log.
var appLogRotation = regexp.MustCompile(`^(\.\d+|-\d{8,10})(\.gz)?$`)

func appLogPath(domain string) string {
	return filepath.Join(appLogDir, domain+".log")
}

// ensureAppLogSetup creates the log directory and the logrotate rule.
func (r *Router) ensureAppLogSetup(ctx context.Context) {
	_, _ = r.executor.RunShell(ctx, "mkdir", "-p", "-m", "0750", appLogDir)
	_, _ = r.executor.RunShell(ctx, "chmod", "0750", appLogDir)
	if ok, _ := r.executor.Stat(ctx, appLogrotatePath); !ok {
		_ = r.executor.WriteFile(ctx, appLogrotatePath, appLogrotateConfig, 0o644)
	}
}

// readAppLog returns the last lines of the app's output: its log file, or
// the journal for apps whose unit predates the file (until the next start).
func (r *Router) readAppLog(ctx context.Context, siteID int64, domain string, lines int) (string, []string, bool, error) {
	path := appLogPath(domain)
	if ok, _ := r.executor.Stat(ctx, path); ok {
		out, err := r.executor.RunShell(ctx, "tail", "-n", strconv.Itoa(lines), path)
		if err != nil {
			return path, nil, true, err
		}
		return path, splitLines(out), true, nil
	}
	if siteID <= 0 {
		return path, []string{}, false, nil
	}
	svc := appServiceName(siteID)
	out, err := r.executor.RunShell(ctx, "journalctl", "-u", svc, "-n", strconv.Itoa(lines), "--no-pager", "-o", "short-iso", "-q")
	if err != nil {
		return path, []string{}, false, nil
	}
	l := splitLines(out)
	return "journal:" + svc, l, len(l) > 0, nil
}

func splitLines(out string) []string {
	if t := strings.TrimRight(out, "\n"); t != "" {
		return strings.Split(t, "\n")
	}
	return []string{}
}
