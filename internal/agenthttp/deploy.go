package agenthttp

// Git deploys (site.deploy) and per-site deploy keys.
//
// Every git, build and script step runs as the site's Linux user inside the
// pinned site root; nothing from the repository ever runs as root. A site
// connected through a git provider clones over SSH with its own read-only
// deploy key (~/.ssh/sigardapanel_site_<id>), with the host keys the panel
// sends (GitHub publishes them) or trust on first use otherwise.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/labstack/echo/v5"
	"sigardapanel/internal/sitefs"
)

// siteHomeDir returns the home directory of uid (tests substitute it).
var siteHomeDir = func(uid uint32) string {
	if u, err := user.LookupId(strconv.FormatUint(uint64(uid), 10)); err == nil {
		return u.HomeDir
	}
	return ""
}

func deployKeyPath(home string, siteID int64) string {
	return filepath.Join(home, ".ssh", fmt.Sprintf("sigardapanel_site_%d", siteID))
}

func knownHostsPath(home string) string {
	return filepath.Join(home, ".ssh", "sigardapanel_known_hosts")
}

// POST /tasks/site/deploy-key {site_id, system_user, action: create|delete}:
// creates (or returns the existing) ed25519 deploy key of the site, as the
// site user; the private key never leaves the server.
func (r *Router) siteDeployKeyTask(c *echo.Context) error {
	ctx := c.Request().Context()
	var p struct {
		SiteID     int64  `json:"site_id"`
		SystemUser string `json:"system_user"`
		Action     string `json:"action"`
	}
	if err := c.Bind(&p); err != nil || p.SiteID <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid payload")
	}
	uid, gid, ok := deployUserIDs(p.SystemUser)
	if !ok {
		return echo.NewHTTPError(http.StatusUnprocessableEntity, "the site has no usable system user")
	}
	home := siteHomeDir(uid)
	if home == "" || !filepath.IsAbs(home) {
		return echo.NewHTTPError(http.StatusUnprocessableEntity, "the site user has no home directory")
	}
	key := deployKeyPath(home, p.SiteID)
	run := func(name string, args ...string) (string, error) {
		return r.runAsSiteUser(ctx, uid, gid, home, name, args...)
	}
	switch p.Action {
	case "delete":
		_, _ = run("rm", "-f", "--", key, key+".pub")
		return c.JSON(http.StatusOK, map[string]any{"ok": true})
	case "create", "":
	default:
		return echo.NewHTTPError(http.StatusBadRequest, "unknown action")
	}
	if out, err := run("mkdir", "-p", "-m", "700", filepath.Join(home, ".ssh")); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "create ~/.ssh: "+strings.TrimSpace(out))
	}
	pub, err := run("cat", "--", key+".pub")
	if err != nil || !strings.HasPrefix(strings.TrimSpace(pub), "ssh-ed25519 ") {
		_, _ = run("rm", "-f", "--", key, key+".pub")
		host, _ := os.Hostname()
		comment := fmt.Sprintf("sigardapanel-site-%d@%s", p.SiteID, host)
		if out, err := run("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-C", comment, "-f", key); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "ssh-keygen failed: "+strings.TrimSpace(out))
		}
		if pub, err = run("cat", "--", key+".pub"); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "read deploy key: "+strings.TrimSpace(pub))
		}
	}
	return c.JSON(http.StatusOK, map[string]any{"ok": true, "public_key": strings.TrimSpace(pub)})
}

var commitRe = regexp.MustCompile(`^[0-9a-fA-F]{7,40}$`)

// deployLog collects the steps of a deploy for the history.
type deployLog struct {
	b     strings.Builder
	start time.Time
}

func (l *deployLog) step(title string) { fmt.Fprintf(&l.b, "\n▸ %s\n", title) }
func (l *deployLog) out(s string) {
	s = strings.TrimRight(s, "\n")
	if len(s) > 64<<10 {
		s = "…\n" + s[len(s)-64<<10:]
	}
	if s != "" {
		l.b.WriteString(s + "\n")
	}
}
func (l *deployLog) String() string {
	return strings.TrimLeft(l.b.String(), "\n") + fmt.Sprintf("\nFinished in %s\n", time.Since(l.start).Round(time.Second))
}

func (r *Router) siteDeployTask(c *echo.Context) error {
	ctx := c.Request().Context()
	var payload struct {
		SiteID       int64  `json:"site_id"`
		Domain       string `json:"domain"`
		Repo         string `json:"repo"`
		Branch       string `json:"branch"`
		RootPath     string `json:"root_path"`
		Runtime      string `json:"runtime"`
		AppStatus    string `json:"app_status"`
		AppPort      int64  `json:"app_port"`
		AppCommand   string `json:"app_command"`
		SystemUser   string `json:"system_user"`
		AppMemoryMax int64  `json:"app_memory_max"`
		// Git provider integration (all optional).
		UseDeployKey bool   `json:"use_deploy_key"`
		KnownHosts   string `json:"known_hosts"`
		TargetCommit string `json:"target_commit"`
		Build        *bool  `json:"build"`
		DeployScript string `json:"deploy_script"`
	}
	if err := c.Bind(&payload); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid payload")
	}
	if !isValidGitURL(payload.Repo) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid repo URL")
	}
	if payload.Branch == "" {
		payload.Branch = "main"
	}
	if !isValidGitRef(payload.Branch) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid branch name")
	}
	if payload.TargetCommit != "" && !commitRe.MatchString(payload.TargetCommit) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid commit")
	}
	if !r.isAllowedSitePath(payload.RootPath) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid root_path")
	}
	if len(payload.DeployScript) > 64<<10 || strings.ContainsRune(payload.KnownHosts, 0) {
		return echo.NewHTTPError(http.StatusBadRequest, "deploy script too long")
	}

	siteUID, siteGID, haveSiteUser := deployUserIDs(payload.SystemUser)
	if !haveSiteUser {
		return echo.NewHTTPError(http.StatusUnprocessableEntity, "deploy needs the site's system user (a regular login account)")
	}
	log := &deployLog{start: time.Now()}
	commit, message, author := "", "", ""
	fail := func(status int, msg string) error {
		log.out("✗ " + msg)
		slog.Warn("agent: site.deploy failed", "site_id", payload.SiteID, "error", msg)
		return c.JSON(status, map[string]any{"ok": false, "error": msg, "message": msg, "deploy_log": log.String(),
			"commit": commit, "commit_message": message, "commit_author": author})
	}

	// git over SSH with the site's deploy key and pinned host keys.
	var gitPrefix []string
	if payload.UseDeployKey {
		home := siteHomeDir(siteUID)
		if home == "" {
			return fail(http.StatusUnprocessableEntity, "the site user has no home directory")
		}
		key := deployKeyPath(home, payload.SiteID)
		kh := knownHostsPath(home)
		strict := "accept-new"
		if hosts := strings.TrimSpace(payload.KnownHosts); hosts != "" {
			if err := r.writeAsSiteUser(ctx, siteUID, siteGID, kh, hosts+"\n"); err != nil {
				return fail(http.StatusInternalServerError, "write known_hosts: "+err.Error())
			}
			strict = "yes"
		}
		ssh := fmt.Sprintf("ssh -i %s -o IdentitiesOnly=yes -o BatchMode=yes -o UserKnownHostsFile=%s -o StrictHostKeyChecking=%s",
			shellQuote(key), shellQuote(kh), strict)
		gitPrefix = []string{"-c", "core.sshCommand=" + ssh}
	}
	git := func(dir string, args ...string) (string, error) {
		return r.runAsSiteUser(ctx, siteUID, siteGID, dir, "git", append(append([]string{}, gitPrefix...), args...)...)
	}
	gitStep := func(dir, title string, args ...string) error {
		log.step(title)
		out, err := git(dir, args...)
		log.out(out)
		if err != nil && strings.TrimSpace(out) == "" {
			log.out(err.Error())
		}
		return err
	}

	// Every git step runs as the site user inside a pinned directory: the
	// site user owns ~/htdocs, so a path-based root rename/checkout/chown
	// could be redirected by a symlink swapped in mid-deploy.
	rootPin, err := sitefs.PinDir(payload.RootPath)
	exists := err == nil
	if err != nil && sitefs.StatusOf(err) != http.StatusNotFound {
		return echo.NewHTTPError(sitefs.StatusOf(err), err.Error())
	}
	if exists {
		defer rootPin.Close()
		if st, ok := rootPin.Info.Sys().(*syscall.Stat_t); !ok || st.Uid != siteUID {
			return echo.NewHTTPError(http.StatusConflict, "the site root is not owned by the site user")
		}
	}
	isGitRepo := false
	if exists {
		if fi, err := os.Lstat(filepath.Join(rootPin.Path, ".git")); err == nil && fi.IsDir() {
			isGitRepo = true
		}
	}
	target := "origin/" + payload.Branch
	if payload.TargetCommit != "" {
		target = payload.TargetCommit
	}
	switch {
	case !exists:
		parent, base, err := sitefs.PinParent(payload.RootPath)
		if err != nil {
			return echo.NewHTTPError(sitefs.StatusOf(err), err.Error())
		}
		defer parent.Close()
		if st, ok := parent.Info.Sys().(*syscall.Stat_t); !ok || st.Uid != siteUID {
			return echo.NewHTTPError(http.StatusConflict, "the site's parent directory is not owned by the site user")
		}
		if err := gitStep(parent.Real, "Clone "+payload.Repo+" ("+payload.Branch+")", "clone", "-b", payload.Branch, "--", payload.Repo, base); err != nil {
			return fail(http.StatusUnprocessableEntity, "git clone failed")
		}
		if rootPin, err = sitefs.PinDir(payload.RootPath); err != nil {
			return echo.NewHTTPError(sitefs.StatusOf(err), err.Error())
		}
		defer rootPin.Close()
	case !isGitRepo:
		// The root exists but is not a repository (site.create writes
		// starter files): clone the history next to the files and check it
		// out in place.
		const tmp = ".sigarda-clone"
		_, _ = r.runAsSiteUser(ctx, siteUID, siteGID, rootPin.Real, "rm", "-rf", "--", tmp)
		if err := gitStep(rootPin.Real, "Clone "+payload.Repo+" ("+payload.Branch+")", "clone", "--no-checkout", "-b", payload.Branch, "--", payload.Repo, tmp); err != nil {
			_, _ = r.runAsSiteUser(ctx, siteUID, siteGID, rootPin.Real, "rm", "-rf", "--", tmp)
			return fail(http.StatusUnprocessableEntity, "git clone failed")
		}
		out, err := r.runAsSiteUser(ctx, siteUID, siteGID, rootPin.Real, "mv", "-T", "--", tmp+"/.git", ".git")
		_, _ = r.runAsSiteUser(ctx, siteUID, siteGID, rootPin.Real, "rm", "-rf", "--", tmp)
		if err != nil {
			log.out(out)
			return fail(http.StatusInternalServerError, "install repository failed")
		}
	default:
		// Follow a changed remote (e.g. https → the deploy key's SSH URL).
		_, _ = git(rootPin.Real, "remote", "set-url", "origin", payload.Repo)
		if err := gitStep(rootPin.Real, "Fetch "+payload.Branch, "fetch", "--prune", "origin", "+refs/heads/"+payload.Branch+":refs/remotes/origin/"+payload.Branch); err != nil {
			return fail(http.StatusUnprocessableEntity, "git fetch failed")
		}
	}
	// The working tree follows the branch (or the requested commit) exactly;
	// tracked files changed on the server are overwritten, untracked ones
	// (uploads, .env) stay.
	if err := gitStep(rootPin.Real, "Check out "+target, "checkout", "-f", "-B", payload.Branch, target, "--"); err != nil {
		if payload.TargetCommit != "" {
			// An older commit may not be in a shallow or pruned history yet.
			if gitStep(rootPin.Real, "Fetch "+payload.TargetCommit, "fetch", "origin", payload.TargetCommit) == nil &&
				gitStep(rootPin.Real, "Check out "+target, "checkout", "-f", "-B", payload.Branch, target, "--") == nil {
				goto checkedOut
			}
		}
		return fail(http.StatusUnprocessableEntity, "git checkout failed")
	}
checkedOut:
	if info, err := git(rootPin.Real, "log", "-1", "--format=%H%n%an%n%s"); err == nil {
		lines := strings.SplitN(strings.TrimSpace(info), "\n", 3)
		if len(lines) == 3 {
			commit, author, message = lines[0], lines[1], lines[2]
		}
	}
	log.out(fmt.Sprintf("HEAD is now %.12s %s (%s)", commit, message, author))

	run := func(title, name string, args ...string) error {
		log.step(title)
		out, err := r.runAsSiteUser(ctx, siteUID, siteGID, rootPin.Real, name, args...)
		log.out(out)
		if err != nil && strings.TrimSpace(out) == "" {
			log.out(err.Error())
		}
		return err
	}
	has := func(rel string) bool {
		fi, err := os.Lstat(filepath.Join(rootPin.Path, rel))
		return err == nil && fi.Mode().IsRegular()
	}

	// Build: dependencies and the project's build script, per what the
	// repository contains.
	if payload.Build == nil || *payload.Build {
		if has("composer.json") {
			if _, err := exec.LookPath("composer"); err != nil {
				log.step("Composer")
				log.out("composer.json found but Composer is not installed on the server (Stack page); skipped")
			} else if err := run("composer install", "composer", "install", "--no-dev", "--no-interaction", "--prefer-dist", "--optimize-autoloader", "--no-progress"); err != nil {
				return fail(http.StatusUnprocessableEntity, "composer install failed")
			}
		}
		if has("package.json") && (payload.Runtime == "node" || packageHasScript(filepath.Join(rootPin.Path, "package.json"), "build")) {
			if _, err := exec.LookPath("npm"); err != nil {
				log.step("npm")
				log.out("package.json found but Node.js/npm is not installed on the server (Stack page); skipped")
			} else {
				args := []string{"install", "--no-audit", "--no-fund"}
				title := "npm install"
				if has("package-lock.json") || has("npm-shrinkwrap.json") {
					args, title = []string{"ci", "--no-audit", "--no-fund"}, "npm ci"
				}
				if err := run(title, "npm", args...); err != nil {
					return fail(http.StatusUnprocessableEntity, title+" failed")
				}
				if packageHasScript(filepath.Join(rootPin.Path, "package.json"), "build") {
					if err := run("npm run build", "npm", "run", "build"); err != nil {
						return fail(http.StatusUnprocessableEntity, "npm run build failed")
					}
				}
			}
		}
		if payload.Runtime == "python" && has("requirements.txt") {
			pip := "pip"
			if has(".venv/bin/pip") {
				pip = filepath.Join(rootPin.Real, ".venv/bin/pip")
			}
			if err := run("pip install", pip, "install", "-r", "requirements.txt"); err != nil {
				return fail(http.StatusUnprocessableEntity, "pip install failed")
			}
		}
		if payload.Runtime == "go" && has("go.mod") {
			gobin := goBinary()
			if gobin == "" {
				return fail(http.StatusUnprocessableEntity, "Go is not installed on this server (Stack page)")
			}
			if err := run("go mod download", gobin, "mod", "download"); err != nil {
				return fail(http.StatusUnprocessableEntity, "go mod download failed")
			}
			// The app runs ./app (the default command): rebuild it.
			if err := run("go build", gobin, "build", "-o", "app", "."); err != nil {
				return fail(http.StatusUnprocessableEntity, "go build failed")
			}
		}
	}

	// The site's own deploy commands (e.g. php artisan migrate --force).
	if script := strings.TrimSpace(payload.DeployScript); script != "" {
		if err := run("Deploy script", "bash", "-e", "-o", "pipefail", "-c", script); err != nil {
			return fail(http.StatusUnprocessableEntity, "deploy script failed")
		}
	}

	// Restart the app if it was running.
	restartMsg := ""
	if payload.AppStatus == "running" && payload.AppPort > 0 {
		svc := appServiceName(payload.SiteID)
		log.step("Restart app")
		if _, err := r.executor.RunShell(ctx, "systemctl", "restart", svc); err == nil {
			statusOut, _ := r.executor.RunShell(ctx, "systemctl", "is-active", svc)
			if strings.TrimSpace(statusOut) == "active" {
				restartMsg = fmt.Sprintf("app restarted on port %d", payload.AppPort)
			} else {
				logs, _ := r.executor.RunShell(ctx, "journalctl", "-u", svc, "-n", "20", "--no-pager")
				log.out(logs)
				return fail(http.StatusUnprocessableEntity, "the app did not start after the deploy")
			}
		} else {
			restartMsg = "app restart skipped (service not found)"
		}
		log.out(restartMsg)
	}

	slog.Info("agent: site.deploy", "site_id", payload.SiteID, "repo", payload.Repo, "branch", payload.Branch, "commit", commit, "runtime", payload.Runtime)
	resp := map[string]any{
		"ok":             true,
		"message":        fmt.Sprintf("deployed %s@%.12s", payload.Branch, commit),
		"commit":         commit,
		"commit_message": message,
		"commit_author":  author,
		"deploy_log":     log.String(),
	}
	if restartMsg != "" {
		resp["restart"] = restartMsg
	}
	return c.JSON(http.StatusOK, resp)
}

// writeAsSiteUser writes content to path as the site user (the path is in
// the user's own home, so it can only affect the user's files).
func (r *Router) writeAsSiteUser(ctx context.Context, uid, gid uint32, path, content string) error {
	if int(uid) == os.Geteuid() && uid != 0 || (uid == 0 && deployAsSelf) {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return err
		}
		return os.WriteFile(path, []byte(content), 0o600)
	}
	if out, err := r.runAsSiteUser(ctx, uid, gid, "/", "mkdir", "-p", "-m", "700", filepath.Dir(path)); err != nil {
		return fmt.Errorf("%s", strings.TrimSpace(out))
	}
	out, err := r.executor.RunAsUserInput(ctx, uid, gid, nil, strings.NewReader(content), "bash", "-c", `umask 077; cat > "$1"`, "_", path)
	if err != nil {
		return fmt.Errorf("%s", strings.TrimSpace(out))
	}
	return nil
}

// packageHasScript reports whether package.json defines scripts[name].
// (Read by root, but only parsed: a symlink was refused by the Lstat above.)
func packageHasScript(path, name string) bool {
	fi, err := os.Lstat(path)
	if err != nil || !fi.Mode().IsRegular() || fi.Size() > 4<<20 {
		return false
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if json.Unmarshal(b, &pkg) != nil {
		return false
	}
	return strings.TrimSpace(pkg.Scripts[name]) != ""
}
