package agenthttp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v5"
	"sigardapanel/internal/sitefs"
	"sigardapanel/internal/siteinput"
)

// siteUserMu serializes looking up and creating site owners' accounts.
var siteUserMu sync.Mutex

// minSiteUserUID is the lowest UID treated as a regular (login) account.
// Site files are never handed to system accounts.
const minSiteUserUID = 1000

// siteOwner is the Linux account that owns a site's files.
type siteOwner struct {
	name string
	home string
}

// ensureSiteUser makes sure the site's system_user exists on this server, the
// way panel users map to Linux users on the panel host: a missing account is
// created with `useradd -m -s /bin/bash <name>`. Only names that pass
// siteinput.SystemUser are accepted (never root or well-known service
// accounts). An existing system account (UID < 1000, or nobody) is not used
// as owner: the site is still created, its files stay as they are.
func (r *Router) ensureSiteUser(ctx context.Context, name string) (*siteOwner, error) {
	if err := siteinput.SystemUser(name); err != nil {
		return nil, fmt.Errorf("invalid system_user %q", name)
	}
	// The worker runs several site.create jobs at once. Without this lock,
	// jobs for the same new user each saw it missing and each ran useradd;
	// their /etc/passwd writes overwrote one another, leaving the home and
	// some site roots owned by uids that no longer exist.
	siteUserMu.Lock()
	defer siteUserMu.Unlock()
	u, err := user.Lookup(name)
	if err != nil {
		var unknown user.UnknownUserError
		if !errors.As(err, &unknown) {
			return nil, fmt.Errorf("look up Linux user %s: %v", name, err)
		}
		// Run directly rather than through the executor allowlist, which
		// deliberately does not include useradd; the name is validated above.
		cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		outB, err := exec.CommandContext(cctx, r.useraddCmd, "-m", "-s", "/bin/bash", name).CombinedOutput()
		out := string(outB)
		if err != nil {
			return nil, fmt.Errorf("create Linux user %s for the site (useradd -m -s /bin/bash %s): %s",
				name, name, strings.TrimSpace(out+" "+err.Error()))
		}
		slog.Info("agent: created Linux user for site owner", "user", name)
		home := filepath.Join("/home", name)
		if u, err = user.Lookup(name); err == nil && u.HomeDir != "" {
			home = u.HomeDir
		}
		// Recent useradd defaults (HOME_MODE 0750, e.g. Ubuntu 24.04) make
		// the new home untraversable for nginx/PHP-FPM, and every file under
		// htdocs/ would 404. Grant traverse-only (o+x) on the home we created.
		allowTraverse(home)
		if err != nil {
			// Created but not resolvable through NSS yet: own by name.
			return &siteOwner{name: name, home: home}, nil
		}
	}
	uid, err := strconv.ParseUint(u.Uid, 10, 32)
	if err == nil && uid >= minSiteUserUID && uid != 65534 {
		// An existing account is adopted only when it is an ordinary
		// account: never one with sudo/docker/... or an SFTP jail account
		// (taking such a name would hand its owner's files, or root, to the
		// tenant whose panel username matches it).
		if perr := accountPrivilegedOrJail(u); perr != nil {
			return nil, fmt.Errorf("refusing to use the existing Linux account %s as site owner: %v (choose another panel username, or, if this account is meant to own sites, add it to %s on this server)", name, perr, trustedSiteUsersFile)
		}
	}
	if err != nil || uid < minSiteUserUID || uid == 65534 {
		slog.Warn("agent: system_user is a system account; site files keep their owner", "user", name, "uid", u.Uid)
		return nil, nil
	}
	home := u.HomeDir
	if home == "" {
		home = filepath.Join("/home", name)
	}
	return &siteOwner{name: name, home: home}, nil
}

// allowTraverse adds o+x to dir (a home this agent just created) so the web
// server can reach htdocs/ without being able to list the home.
func allowTraverse(dir string) {
	fi, err := os.Stat(dir)
	if err != nil || !fi.IsDir() || fi.Mode().Perm()&0o001 != 0 {
		return
	}
	if err := os.Chmod(dir, fi.Mode().Perm()|0o001); err != nil {
		slog.Warn("agent: make site owner's home traversable", "dir", dir, "error", err)
	}
}

// chownNewSiteRoot gives a site root the agent just created (and the starter
// files in it) to the owner, plus the owner's htdocs directory when the root
// sits directly in it (Mkdir created that as root). A root that already
// existed keeps its ownership: it may hold files deliberately owned by others.
//
// The site user can rename and replace entries in its own home at any time,
// so nothing here goes through a path the user could swap: the root is the
// pinned directory, entries are Lchown'ed (links are never followed) and
// htdocs is changed through its own pinned descriptor.
func (r *Router) chownNewSiteRoot(ctx context.Context, owner *siteOwner, root *sitefs.PinnedDir, rootPath string, rootExisted bool) {
	if rootExisted {
		slog.Info("agent: site root already existed; ownership left unchanged", "root", rootPath, "user", owner.name)
		return
	}
	uid, gid, ok := siteUserIDs(owner.name)
	if !ok {
		slog.Warn("agent: chown site root: owner account not usable", "root", rootPath, "user", owner.name)
		return
	}
	if err := sitefs.ChownTree(root, int(uid), int(gid)); err != nil {
		slog.Warn("agent: chown site root", "root", rootPath, "user", owner.name, "error", err)
		return
	}
	htdocs := filepath.Join(owner.home, "htdocs")
	if filepath.Dir(filepath.Clean(rootPath)) == htdocs {
		pin, err := sitefs.PinDir(htdocs)
		if err != nil {
			slog.Warn("agent: chown htdocs", "dir", htdocs, "error", err)
			return
		}
		defer pin.Close()
		if err := pin.Chown(int(uid), int(gid)); err != nil {
			slog.Warn("agent: chown htdocs", "dir", htdocs, "error", err)
		}
	}
}

// siteTransferTask answers POST /tasks/site/transfer: move a site root to
// its new owner's home and hand the files to that owner. Nothing changes
// when a step fails before the move; the panel updates its records only
// after this succeeded.
func (r *Router) siteTransferTask(c *echo.Context) error {
	ctx := c.Request().Context()
	var p struct {
		SiteID     int64  `json:"site_id"`
		Domain     string `json:"domain"`
		OldRoot    string `json:"old_root"`
		NewRoot    string `json:"new_root"`
		SystemUser string `json:"system_user"`
	}
	if err := c.Bind(&p); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid payload")
	}
	if !isValidDomain(p.Domain) || !r.isAllowedSitePath(p.OldRoot) || !r.isAllowedSitePath(p.NewRoot) || filepath.Base(p.NewRoot) != p.Domain {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid paths")
	}
	if err := siteinput.SystemUser(p.SystemUser); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid system_user")
	}
	if filepath.Clean(p.OldRoot) == filepath.Clean(p.NewRoot) {
		return c.JSON(http.StatusOK, map[string]any{"ok": true, "message": "already in place", "root_path": p.NewRoot})
	}
	if _, err := os.Lstat(p.NewRoot); err == nil {
		// A retry after the move went through (lost response, worker
		// restart): the files are already in place.
		if _, oldErr := os.Lstat(p.OldRoot); os.IsNotExist(oldErr) {
			return c.JSON(http.StatusOK, map[string]any{"ok": true, "message": "already moved", "root_path": p.NewRoot})
		}
		return echo.NewHTTPError(http.StatusConflict, "the target "+p.NewRoot+" already exists")
	}
	owner, err := r.ensureSiteUser(ctx, p.SystemUser)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if err := os.MkdirAll(filepath.Dir(p.NewRoot), 0o755); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "create target directory failed: "+err.Error())
	}
	if _, err := os.Stat(p.OldRoot); err != nil {
		// Nothing to move (e.g. never deployed): just create the new root.
		if err := os.MkdirAll(p.NewRoot, 0o755); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "create root failed: "+err.Error())
		}
	} else if err := movePath(p.OldRoot, p.NewRoot); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "move files failed: "+err.Error())
	}
	if owner != nil {
		pin, err := sitefs.PinDir(p.NewRoot)
		if err != nil {
			return echo.NewHTTPError(sitefs.StatusOf(err), "new root: "+err.Error())
		}
		r.chownNewSiteRoot(ctx, owner, pin, p.NewRoot, false)
		pin.Close()
	}
	slog.Info("agent: site.transfer", "site_id", p.SiteID, "from", p.OldRoot, "to", p.NewRoot, "user", p.SystemUser)
	return c.JSON(http.StatusOK, map[string]any{"ok": true, "root_path": p.NewRoot})
}
