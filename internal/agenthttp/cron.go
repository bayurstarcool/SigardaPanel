package agenthttp

import (
	"fmt"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"sigardapanel/internal/siteinput"
)

type siteCronJob struct {
	Schedule string `json:"schedule"`
	Command  string `json:"command"`
}

// cronFileName is the /etc/cron.d entry of one site. cron ignores files in
// cron.d whose names contain anything but letters, digits, '-' and '_'.
func cronFileName(siteID int64) string {
	return "sigardapanel-site-" + strconv.FormatInt(siteID, 10)
}

// validCronSchedule checks the five schedule fields loosely (the panel
// validates them strictly): only the characters cron accepts, so a schedule
// can never smuggle extra fields or lines into the file.
func validCronSchedule(s string) bool {
	f := strings.Fields(s)
	if len(f) != 5 {
		return false
	}
	for _, x := range f {
		if strings.Trim(x, "0123456789*,-/") != "" {
			return false
		}
	}
	return true
}

// renderSiteCron builds the cron.d file of a site: every job runs as the
// site's own user from the site root. '%' is a newline to cron, so it is
// escaped; a command may not contain line breaks.
func renderSiteCron(siteID int64, systemUser, rootPath string, jobs []siteCronJob) (string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "# Managed by SigardaPanel for site %d. Edits are overwritten.\nSHELL=/bin/bash\nPATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin\n", siteID)
	for _, j := range jobs {
		if !validCronSchedule(j.Schedule) {
			return "", fmt.Errorf("invalid schedule %q", j.Schedule)
		}
		cmd := strings.TrimSpace(j.Command)
		if cmd == "" || len(cmd) > 2000 || strings.ContainsAny(cmd, "\r\n\x00") {
			return "", fmt.Errorf("invalid command")
		}
		cmd = strings.ReplaceAll(cmd, "%", `\%`)
		fmt.Fprintf(&b, "%s %s cd '%s' && %s\n", strings.Join(strings.Fields(j.Schedule), " "), systemUser, rootPath, cmd)
	}
	return b.String(), nil
}

// cronUserAllowed: jobs never run as root or as a system account.
func cronUserAllowed(name string) bool {
	if siteinput.SystemUser(name) != nil {
		return false
	}
	u, err := user.Lookup(name)
	if err != nil {
		return false
	}
	uid, err := strconv.Atoi(u.Uid)
	return err == nil && uid >= minSiteUserUID
}

// cronSyncTask answers POST /tasks/cron/sync {site_id, system_user,
// root_path, jobs}: replace the site's cron file (none when jobs is empty).
func (r *Router) cronSyncTask(c echo.Context) error {
	var p struct {
		SiteID     int64         `json:"site_id"`
		SystemUser string        `json:"system_user"`
		RootPath   string        `json:"root_path"`
		Jobs       []siteCronJob `json:"jobs"`
	}
	if err := c.Bind(&p); err != nil || p.SiteID <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]any{"ok": false, "message": "invalid payload"})
	}
	path := filepath.Join(r.cronDir, cronFileName(p.SiteID))
	suspended := path + ".suspended"
	if len(p.Jobs) == 0 {
		for _, f := range []string{path, suspended} {
			if err := os.Remove(f); err != nil && !os.IsNotExist(err) {
				return c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "message": err.Error()})
			}
		}
		return c.JSON(http.StatusOK, map[string]any{"ok": true, "jobs": 0})
	}
	// A suspended site keeps its jobs paused.
	target := path
	if fileExists(suspended) {
		target = suspended
	}
	if !r.isAllowedSitePath(p.RootPath) || strings.ContainsAny(p.RootPath, "'%") {
		return c.JSON(http.StatusBadRequest, map[string]any{"ok": false, "message": "invalid root_path"})
	}
	if !r.cronUserOK(p.SystemUser) {
		return c.JSON(http.StatusBadRequest, map[string]any{"ok": false, "message": "cron jobs run as the site's own Linux user; " + strconv.Quote(p.SystemUser) + " is not a regular user on this server"})
	}
	content, err := renderSiteCron(p.SiteID, p.SystemUser, p.RootPath, p.Jobs)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"ok": false, "message": err.Error()})
	}
	if err := os.MkdirAll(r.cronDir, 0o755); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "message": err.Error()})
	}
	// cron reads files in cron.d only when root-owned and not group/world
	// writable; write atomically so cron never sees half a file.
	tmp := filepath.Join(r.cronDir, "."+cronFileName(p.SiteID)+".tmp")
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "message": err.Error()})
	}
	if err := os.Rename(tmp, target); err != nil {
		os.Remove(tmp)
		return c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "message": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]any{"ok": true, "jobs": len(p.Jobs), "path": target})
}
