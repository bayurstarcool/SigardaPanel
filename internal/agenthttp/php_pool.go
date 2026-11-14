package agenthttp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"

	"github.com/labstack/echo/v5"

	"sigardapanel/internal/nginx"
	"sigardapanel/internal/sitefs"
)

// Per-site PHP-FPM pools.
//
// Every PHP site gets its own pool, /etc/php/<ver>/fpm/pool.d/sp-site-<id>.conf,
// running as the site's Linux user (never www-data or root) with open_basedir
// on its root and its own temp/session directories under
// ~/.php/<id>/. One site's PHP therefore cannot read another site's files
// (wp-config.php, .env) and settings cannot leak between sites through a
// shared worker. The socket, /run/php/sp-site-<id>-php<ver>-fpm.sock, is
// owned by the nginx user (0660). Hosts where the agent cannot manage pools
// (no /etc/php/<ver>/fpm/pool.d, e.g. CloudPanel's own PHP) keep the shared
// upstream.

const defaultPHPEtcDir = "/etc/php"

// poolSiteUser resolves a site owner for a pool: a login account (UID >=
// 1000, not nobody, not a service account) and its home directory.
type poolSiteUser struct {
	name     string
	uid, gid uint32
	home     string
}

func defaultPoolUserLookup(name string) (*poolSiteUser, bool) {
	uid, gid, ok := siteUserIDs(name)
	if !ok {
		return nil, false
	}
	u, err := user.Lookup(name)
	if err != nil || u.HomeDir == "" || !filepath.IsAbs(u.HomeDir) {
		return nil, false
	}
	return &poolSiteUser{name: name, uid: uid, gid: gid, home: filepath.Clean(u.HomeDir)}, true
}

func (r *Router) poolUser(name string) (*poolSiteUser, bool) {
	if name == "" {
		return nil, false
	}
	if r.poolUserLookup != nil {
		return r.poolUserLookup(name)
	}
	return defaultPoolUserLookup(name)
}

// poolDir is where a PHP version keeps its pool definitions.
func (r *Router) poolDir(version string) string {
	return filepath.Join(r.phpEtcDir, version, "fpm", "pool.d")
}

// poolVersion picks the PHP version a site's pool uses: the requested one
// when the agent can manage its pools, else (no version) the first default
// version that has a pool directory. "" means pools are not available here.
func (r *Router) poolVersion(requested string) string {
	if requested != "" {
		if dirExists(r.poolDir(requested)) {
			return requested
		}
		return ""
	}
	for _, v := range nginx.PHPFPMDefaultVersions {
		if dirExists(r.poolDir(v)) {
			return v
		}
	}
	return ""
}

func poolName(siteID int64) string { return fmt.Sprintf("sp-site-%d", siteID) }

func (r *Router) poolFile(version string, siteID int64) string {
	return filepath.Join(r.poolDir(version), poolName(siteID)+".conf")
}

var nginxUserRe = regexp.MustCompile(`(?m)^\s*user\s+([a-z_][a-z0-9_-]{0,31})(\s+[a-z_][a-z0-9_-]{0,31})?\s*;`)

// nginxUser is the account nginx workers run as (they connect to the pool
// socket): the "user" directive of nginx.conf, else www-data.
func (r *Router) nginxUser() string {
	if r.nginxRunUser != "" {
		return r.nginxRunUser
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(r.nginxConfDir), "nginx.conf"))
	if err == nil {
		if m := nginxUserRe.FindSubmatch(data); m != nil {
			return string(m[1])
		}
	}
	return "www-data"
}

// sitePHPDir is ~/.php/<id>: the pool's temp, session and error log place.
func sitePHPDir(home string, siteID int64) string {
	return filepath.Join(home, ".php", strconv.FormatInt(siteID, 10))
}

// prepareSitePHPDir creates ~/.php/<id>/{tmp,sessions} owned by the site user
// (0700) without following anything the user planted: the home is pinned and
// every step goes through an os.Root on it (links leaving the home are
// refused, chown never follows a link).
func prepareSitePHPDir(su *poolSiteUser, siteID int64) error {
	pin, err := sitefs.PinDir(su.home)
	if err != nil {
		return fmt.Errorf("site owner's home: %w", err)
	}
	defer pin.Close()
	root, err := os.OpenRoot(pin.Path)
	if err != nil {
		return err
	}
	defer root.Close()
	id := strconv.FormatInt(siteID, 10)
	for _, rel := range []string{".php", filepath.Join(".php", id), filepath.Join(".php", id, "tmp"), filepath.Join(".php", id, "sessions")} {
		if err := root.Mkdir(rel, 0o700); err != nil && !errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("create %s: %w", rel, err)
		}
		fi, err := root.Lstat(rel)
		if err != nil {
			return err
		}
		if !fi.IsDir() {
			return fmt.Errorf("%s is not a directory", filepath.Join(su.home, rel))
		}
		if err := root.Lchown(rel, int(su.uid), int(su.gid)); err != nil {
			return err
		}
		if err := root.Chmod(rel, 0o700); err != nil {
			return err
		}
	}
	return nil
}

// renderPool is the pool definition of a site. Every interpolated value is
// validated by the caller (site id, user names, absolute clean paths without
// spaces or quotes).
func renderPool(siteID int64, su *poolSiteUser, nginxUser, rootPath, socket string) string {
	phpDir := sitePHPDir(su.home, siteID)
	var b strings.Builder
	fmt.Fprintf(&b, "; Managed by SigardaPanel (site %d). Regenerated on every vhost change; do not edit.\n", siteID)
	fmt.Fprintf(&b, "[%s]\n", poolName(siteID))
	fmt.Fprintf(&b, "user = %s\ngroup = %s\n", su.name, su.name)
	fmt.Fprintf(&b, "listen = %s\n", socket)
	fmt.Fprintf(&b, "listen.owner = %s\nlisten.group = %s\nlisten.mode = 0660\n", nginxUser, nginxUser)
	b.WriteString("pm = ondemand\npm.max_children = 5\npm.process_idle_timeout = 10s\npm.max_requests = 500\n")
	b.WriteString("security.limit_extensions = .php\n")
	// Worker output stays out of the server-wide FPM log (other tenants read
	// that one): PHP errors go to the site's own error_log below.
	b.WriteString("catch_workers_output = no\n")
	fmt.Fprintf(&b, "php_admin_value[open_basedir] = %s:%s:/usr/share/php\n", rootPath, phpDir)
	fmt.Fprintf(&b, "php_admin_value[upload_tmp_dir] = %s/tmp\n", phpDir)
	fmt.Fprintf(&b, "php_admin_value[sys_temp_dir] = %s/tmp\n", phpDir)
	fmt.Fprintf(&b, "php_admin_value[session.save_path] = %s/sessions\n", phpDir)
	fmt.Fprintf(&b, "php_admin_value[error_log] = %s/error.log\n", phpDir)
	b.WriteString("php_admin_flag[log_errors] = on\n")
	b.WriteString("php_admin_flag[allow_url_include] = off\n")
	return b.String()
}

var poolPathRe = regexp.MustCompile(`^/[A-Za-z0-9._/-]+$`)

// siteUserNameRe: an account name safe to write into a pool definition.
var siteUserNameRe = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)

// errPoolOwner is a permanent site.create error (the worker does not retry).
func errPoolOwner(name string) error {
	return echo.NewHTTPError(http.StatusUnprocessableEntity, fmt.Sprintf(
		"PHP sites run as their owner's Linux account; %q is not a usable site owner on this server (UID >= 1000, not a service account). Set the site's system user", name))
}

// ensurePHPPool writes (or rewrites) the site's pool for version, removes
// its pools of other versions, checks the PHP-FPM configuration and reloads.
// It returns the socket for the vhost, or "" when pools cannot be managed on
// this host (the vhost keeps the shared upstream).
func (r *Router) ensurePHPPool(ctx context.Context, siteID int64, requestedVersion, systemUser, rootPath string) (string, error) {
	version := r.poolVersion(requestedVersion)
	if version == "" || siteID <= 0 {
		slog.Warn("agent: no manageable PHP-FPM pool directory; site uses the shared pool", "site_id", siteID, "php_version", requestedVersion)
		return "", nil
	}
	su, ok := r.poolUser(systemUser)
	if !ok {
		return "", errPoolOwner(systemUser)
	}
	nu := r.nginxUser()
	socket := nginx.SitePoolSocket(r.phpRunDir, siteID, version)
	root := filepath.Clean(rootPath)
	for _, p := range []string{root, su.home, socket} {
		if !poolPathRe.MatchString(p) {
			return "", echo.NewHTTPError(http.StatusBadRequest, "path not usable in a PHP-FPM pool: "+p)
		}
	}
	if !siteUserNameRe.MatchString(nu) {
		return "", echo.NewHTTPError(http.StatusInternalServerError, "unexpected nginx user "+nu)
	}
	if err := prepareSitePHPDir(su, siteID); err != nil {
		return "", echo.NewHTTPError(http.StatusInternalServerError, "prepare PHP directories: "+err.Error())
	}

	file := r.poolFile(version, siteID)
	content := renderPool(siteID, su, nu, root, socket)
	previous, readErr := os.ReadFile(file)
	hadPrevious := readErr == nil
	changed := !hadPrevious || !bytes.Equal(previous, []byte(content))
	if changed {
		if err := writeFileAtomic(file, []byte(content), 0o644); err != nil {
			return "", echo.NewHTTPError(http.StatusInternalServerError, "write PHP-FPM pool: "+err.Error())
		}
		if out, err := r.executor.RunShell(ctx, "php-fpm"+version, "-t"); err != nil {
			if hadPrevious {
				_ = writeFileAtomic(file, previous, 0o644)
			} else {
				_ = os.Remove(file)
			}
			return "", echo.NewHTTPError(http.StatusInternalServerError, "PHP-FPM rejected the site's pool (not installed): "+strings.TrimSpace(out))
		}
	}
	// A version change leaves the old version's pool behind: remove it.
	r.removePHPPools(ctx, siteID, version)
	if changed {
		if out, err := r.executor.RunShell(ctx, "systemctl", "reload", "php"+version+"-fpm"); err != nil {
			return "", echo.NewHTTPError(http.StatusInternalServerError, "reload php"+version+"-fpm: "+strings.TrimSpace(out))
		}
	}
	return socket, nil
}

// removePHPPools deletes the site's pools of every version except keep and
// reloads the versions that had one.
func (r *Router) removePHPPools(ctx context.Context, siteID int64, keep string) []string {
	if siteID <= 0 {
		return nil
	}
	matches, _ := filepath.Glob(filepath.Join(r.phpEtcDir, "*", "fpm", "pool.d", poolName(siteID)+".conf"))
	var removed []string
	for _, m := range matches {
		version := filepath.Base(filepath.Dir(filepath.Dir(filepath.Dir(m))))
		if version == keep {
			continue
		}
		if err := os.Remove(m); err != nil {
			slog.Warn("agent: remove PHP-FPM pool", "file", m, "error", err)
			continue
		}
		removed = append(removed, version)
		if out, err := r.executor.RunShell(ctx, "systemctl", "reload", "php"+version+"-fpm"); err != nil {
			slog.Warn("agent: reload after pool removal", "version", version, "error", err, "output", strings.TrimSpace(out))
		}
	}
	return removed
}

// readSitePHPErrorLog returns the last lines of ~/.php/<id>/error.log. The
// file is the site user's: it is opened beneath the pinned home without
// following links and only when it is a regular file.
func (r *Router) readSitePHPErrorLog(systemUser string, siteID int64, lines int) (string, []string, bool, error) {
	su, ok := r.poolUser(systemUser)
	if !ok || siteID <= 0 {
		return "", nil, false, errNoSitePool
	}
	path := filepath.Join(sitePHPDir(su.home, siteID), "error.log")
	pin, err := sitefs.PinDir(su.home)
	if err != nil {
		return path, nil, false, err
	}
	defer pin.Close()
	root, err := os.OpenRoot(pin.Path)
	if err != nil {
		return path, nil, false, err
	}
	defer root.Close()
	rel := filepath.Join(".php", strconv.FormatInt(siteID, 10), "error.log")
	f, err := root.OpenFile(rel, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if errors.Is(err, fs.ErrNotExist) {
		return path, []string{}, false, nil
	}
	if err != nil {
		return path, nil, false, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil || !fi.Mode().IsRegular() {
		return path, nil, false, fmt.Errorf("the PHP error log is not a regular file")
	}
	const window = 1 << 20
	off := fi.Size() - window
	if off < 0 {
		off = 0
	}
	buf, err := io.ReadAll(io.NewSectionReader(f, off, fi.Size()-off))
	if err != nil {
		return path, nil, false, err
	}
	all := strings.Split(strings.TrimRight(string(buf), "\n"), "\n")
	if off > 0 && len(all) > 0 {
		all = all[1:] // partial first line
	}
	if len(all) == 1 && all[0] == "" {
		all = nil
	}
	if len(all) > lines {
		all = all[len(all)-lines:]
	}
	if all == nil {
		all = []string{}
	}
	return path, all, true, nil
}

var errNoSitePool = errors.New("site has no PHP pool on this server")

// sitePoolExists reports whether the site has a pool of any PHP version.
func (r *Router) sitePoolExists(siteID int64) bool {
	matches, _ := filepath.Glob(filepath.Join(r.phpEtcDir, "*", "fpm", "pool.d", poolName(siteID)+".conf"))
	return len(matches) > 0
}
