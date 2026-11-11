package agenthttp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"sigardapanel/internal/siteinput"
)

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
		if u, err = user.Lookup(name); err != nil {
			// Created but not resolvable through NSS yet: own by name.
			return &siteOwner{name: name, home: filepath.Join("/home", name)}, nil
		}
	}
	uid, err := strconv.ParseUint(u.Uid, 10, 32)
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

// chownNewSiteRoot gives a site root the agent just created (and the starter
// files in it) to the owner, plus the owner's htdocs directory when the root
// sits directly in it (Mkdir created that as root). A root that already
// existed keeps its ownership: it may hold files deliberately owned by others.
func (r *Router) chownNewSiteRoot(ctx context.Context, owner *siteOwner, rootPath string, rootExisted bool) {
	if rootExisted {
		slog.Info("agent: site root already existed; ownership left unchanged", "root", rootPath, "user", owner.name)
		return
	}
	spec := owner.name + ":" // the user and its login group
	if out, err := r.executor.RunShell(ctx, "chown", "-R", spec, rootPath); err != nil {
		slog.Warn("agent: chown site root", "root", rootPath, "user", owner.name, "error", err, "output", strings.TrimSpace(out))
		return
	}
	htdocs := filepath.Join(owner.home, "htdocs")
	if filepath.Dir(filepath.Clean(rootPath)) == htdocs {
		if out, err := r.executor.RunShell(ctx, "chown", spec, htdocs); err != nil {
			slog.Warn("agent: chown htdocs", "dir", htdocs, "error", err, "output", strings.TrimSpace(out))
		}
	}
}
