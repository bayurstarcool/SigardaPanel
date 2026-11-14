package agenthttp

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"sigardapanel/internal/sitefs"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/labstack/echo/v5"
)

// SSH/FTP accounts of a site: a chrooted Linux user whose jail
// (/home/<user>) bind-mounts the site root at htdocs/ and the system
// directories read-only. Run by the agent of the server hosting the site.

var (
	sshAccountRe = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,30}$`)
	sshPubKeyRe  = regexp.MustCompile(`^(ssh-ed25519|ssh-rsa|ecdsa-sha2-nistp(256|384|521)|sk-ssh-ed25519@openssh\.com|sk-ecdsa-sha2-nistp256@openssh\.com) [A-Za-z0-9+/=]{40,}( [^\r\n]{0,200})?$`)
)

type sshAccountRef struct {
	Username string `json:"username"`
	HomeDir  string `json:"home_dir"`
}

func (r *Router) sshHome(username string) string { return filepath.Join(r.sshHomeBase, username) }

// sshMarker is the fstab comment of a mount owned by username.
func sshMarker(username string) string { return "# sigardapanel:" + username }

func (r *Router) readFstabLines() []string {
	raw, err := os.ReadFile(r.fstabPath)
	if err != nil {
		return nil
	}
	var out []string
	for _, l := range strings.Split(string(raw), "\n") {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}

func (r *Router) writeFstabLines(lines []string) error {
	tmp := r.fstabPath + ".sigarda-tmp"
	if err := os.WriteFile(tmp, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, r.fstabPath)
}

// privateMountNS reports whether the agent runs in its own mount namespace
// (systemd's ProtectKernelTunables/ProtectControlGroups and similar give a
// service one). Bind mounts made there are invisible to sshd, so jails
// looked empty and logins failed until a reboot applied fstab.
func privateMountNS() bool {
	self, err1 := os.Readlink("/proc/self/ns/mnt")
	host, err2 := os.Readlink("/proc/1/ns/mnt")
	return err1 == nil && err2 == nil && self != host
}

// hostMount runs mount/umount in the host's mount namespace.
func (r *Router) hostMount(ctx context.Context, cmd string, args ...string) (string, error) {
	if privateMountNS() {
		return r.executor.RunShell(ctx, "nsenter", append([]string{"-t", "1", "-m", "--", cmd}, args...)...)
	}
	return r.executor.RunShell(ctx, cmd, args...)
}

// mountedUnder lists the mount points at or below dir, as the host (and
// sshd) sees them.
func mountedUnder(dir string) []string {
	mounts := "/proc/self/mounts"
	if privateMountNS() {
		mounts = "/proc/1/mounts"
	}
	f, err := os.Open(mounts)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) >= 2 {
			target := strings.ReplaceAll(fields[1], `\040`, " ")
			if target == dir || strings.HasPrefix(target, dir+"/") {
				out = append(out, target)
			}
		}
	}
	return out
}

// syncSSHDConfig writes the chroot Match blocks for every account on this
// server, checks sshd's configuration and reloads it (the previous file is
// restored when sshd -t fails).
func (r *Router) syncSSHDConfig(ctx context.Context, accounts []sshAccountRef) error {
	var sb strings.Builder
	sb.WriteString("# Managed by SigardaPanel - do not edit manually\n# ChrootDirectory jails for SSH/FTP users\n\n")
	for _, a := range accounts {
		if !sshAccountRe.MatchString(a.Username) || a.HomeDir != r.sshHome(a.Username) {
			continue
		}
		fmt.Fprintf(&sb, "Match User %s\n    ChrootDirectory %s\n    X11Forwarding no\n    AllowTcpForwarding no\n    PasswordAuthentication no\n    AuthenticationMethods publickey\n\n", a.Username, a.HomeDir)
	}
	if err := os.MkdirAll(filepath.Dir(r.sshdConfigPath), 0o755); err != nil {
		return err
	}
	prev, prevErr := os.ReadFile(r.sshdConfigPath)
	if err := os.WriteFile(r.sshdConfigPath, []byte(sb.String()), 0o644); err != nil {
		return err
	}
	if out, err := r.executor.RunShell(ctx, "sshd", "-t"); err != nil {
		if prevErr == nil {
			_ = os.WriteFile(r.sshdConfigPath, prev, 0o644)
		} else {
			_ = os.Remove(r.sshdConfigPath)
		}
		return fmt.Errorf("sshd -t rejected the configuration: %s", strings.TrimSpace(out))
	}
	if _, err := r.executor.RunShell(ctx, "systemctl", "reload", "ssh"); err != nil {
		if out, err := r.executor.RunShell(ctx, "systemctl", "reload", "sshd"); err != nil {
			slog.Warn("agent: sshd reload failed", "output", strings.TrimSpace(out))
		}
	}
	return nil
}

// sshCreateTask answers POST /tasks/sshftp/create.
// sshTaskMu serializes account changes: each rewrites fstab and the sshd
// jail config.
var sshTaskMu sync.Mutex

func (r *Router) sshCreateTask(c *echo.Context) error {
	sshTaskMu.Lock()
	defer sshTaskMu.Unlock()
	ctx := c.Request().Context()
	var p struct {
		Username string `json:"username"`
		SiteRoot string `json:"site_root"`
		SSHKeys  string `json:"ssh_keys"`
		// The site's Linux user: the account shares its uid/gid.
		SystemUser string          `json:"system_user"`
		Accounts   []sshAccountRef `json:"accounts"` // every account on this server after the change
	}
	if err := c.Bind(&p); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"ok": false, "message": "invalid payload"})
	}
	if !sshAccountRe.MatchString(p.Username) {
		return c.JSON(http.StatusBadRequest, map[string]any{"ok": false, "message": "invalid username"})
	}
	if !r.isAllowedSitePath(p.SiteRoot) || !dirExists(p.SiteRoot) {
		return c.JSON(http.StatusBadRequest, map[string]any{"ok": false, "message": "the site root does not exist on this server"})
	}
	var extraKeys []string
	for _, line := range strings.Split(strings.TrimSpace(p.SSHKeys), "\n") {
		if line = strings.TrimSpace(line); line == "" {
			continue
		}
		if !sshPubKeyRe.MatchString(line) {
			return c.JSON(http.StatusBadRequest, map[string]any{"ok": false, "message": "not an SSH public key: " + truncate(line, 40)})
		}
		extraKeys = append(extraKeys, line)
	}
	if _, err := user.Lookup(p.Username); err == nil {
		return c.JSON(http.StatusConflict, map[string]any{"ok": false, "message": "a Linux user named " + p.Username + " already exists on this server"})
	}
	home := r.sshHome(p.Username)

	// The jail is created here, as root:root 0755, before the account exists
	// (useradd -M). useradd -m used to create it owned by the site user's
	// uid (the account shares it) and it was handed to root only later, so
	// the site user could plant symlinks in it (tmp -> /etc made the agent
	// chmod /etc 1777; .ssh/authorized_keys -> /root/... got the tenant's
	// keys). Mkdir fails on anything already there, a symlink included, and
	// nothing inside the jail is writable by the account until the site is
	// bind-mounted at the very end.
	if err := os.Mkdir(home, 0o755); err != nil {
		return c.JSON(http.StatusConflict, map[string]any{"ok": false, "message": "the jail directory " + home + " already exists on this server"})
	}
	if err := os.Chown(home, 0, 0); err != nil {
		_ = os.Remove(home)
		return c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "message": "prepare jail failed"})
	}
	_ = os.Chmod(home, 0o755) // not subject to the umask

	// The account shares the uid/gid of the site's owner (a second name for
	// the same Linux user), so it can edit the site's files and what it
	// uploads belongs to the site user. As a separate user it could read the
	// site but not write it. The chroot keeps it inside this site.
	// The ids come from the site user's account (siteUserIDs refuses root and
	// service accounts), never from a path in the request.
	useradd := []string{"-M", "-d", home, "-s", "/bin/bash"}
	if uid, gid, ok := siteUserIDs(p.SystemUser); ok && uid >= 1000 {
		useradd = append(useradd, "-o", "-u", strconv.FormatUint(uint64(uid), 10), "-g", strconv.FormatUint(uint64(gid), 10))
	}
	useradd = append(useradd, p.Username)
	siteUserMu.Lock() // /etc/passwd is shared with site owners' accounts
	out, err := r.executor.RunShell(ctx, r.useraddCmd, useradd...)
	siteUserMu.Unlock()
	if err != nil {
		_ = os.Remove(home)
		return c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "message": "create user failed: " + strings.TrimSpace(out)})
	}
	cleanup := func() { r.removeSSHAccount(ctx, p.Username, home) }
	_, _ = r.executor.RunShell(ctx, "passwd", "-l", p.Username)
	u, err := user.Lookup(p.Username)
	if err != nil {
		cleanup()
		return c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "message": "the new user cannot be looked up"})
	}

	// Key pair: generated in a private temp dir; the private key is returned
	// once and not kept on the server.
	keyDir, err := os.MkdirTemp("", "sigarda-sshkey-")
	if err != nil {
		cleanup()
		return c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "message": "key generation failed"})
	}
	defer os.RemoveAll(keyDir)
	keyFile := filepath.Join(keyDir, "id_ed25519")
	if out, err := r.executor.RunShell(ctx, "ssh-keygen", "-q", "-t", "ed25519", "-f", keyFile, "-N", "", "-C", p.Username+"@sigardapanel"); err != nil {
		cleanup()
		return c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "message": "key generation failed: " + strings.TrimSpace(out)})
	}
	priv, _ := os.ReadFile(keyFile)
	pub, _ := os.ReadFile(keyFile + ".pub")

	// Everything below is created fresh in the root-owned jail: Mkdir and
	// O_EXCL|O_NOFOLLOW fail on an entry that already exists.
	// .ssh and authorized_keys stay root-owned (sshd's StrictModes accepts
	// that); the keys are managed by the panel.
	sshDir := filepath.Join(home, ".ssh")
	keys := append([]string{strings.TrimSpace(string(pub))}, extraKeys...)
	if err := jailMkdir(sshDir, 0o755); err != nil {
		cleanup()
		return c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "message": "prepare jail failed: " + err.Error()})
	}
	if err := jailWriteFile(filepath.Join(sshDir, "authorized_keys"), []byte(strings.Join(keys, "\n")+"\n"), 0o644); err != nil {
		cleanup()
		return c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "message": "write authorized_keys failed"})
	}

	fstab := r.readFstabLines()
	for _, d := range []string{"/bin", "/lib", "/lib64", "/usr"} {
		if !dirExists(d) {
			continue
		}
		dst := filepath.Join(home, d)
		if err := jailMkdir(dst, 0o755); err != nil {
			cleanup()
			return c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "message": "prepare jail failed: " + err.Error()})
		}
		_, _ = r.hostMount(ctx, "mount", "--bind", d, dst)
		_, _ = r.hostMount(ctx, "mount", "-o", "remount,bind,ro", dst)
		fstab = append(fstab, fmt.Sprintf("%s %s none bind,ro 0 0  %s", d, dst, sshMarker(p.Username)))
	}
	// Shells and tools (git, composer, php) need a few devices and /tmp.
	devDir := filepath.Join(home, "dev")
	if err := jailMkdir(devDir, 0o755); err != nil {
		cleanup()
		return c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "message": "prepare jail failed: " + err.Error()})
	}
	for _, dev := range []string{"null", "zero", "urandom", "random", "tty"} {
		src := filepath.Join("/dev", dev)
		if _, err := os.Stat(src); err != nil {
			continue
		}
		dst := filepath.Join(devDir, dev)
		if err := jailWriteFile(dst, nil, 0o644); err != nil {
			continue
		}
		_, _ = r.hostMount(ctx, "mount", "--bind", src, dst)
		fstab = append(fstab, fmt.Sprintf("%s %s none bind 0 0  %s", src, dst, sshMarker(p.Username)))
	}
	tmpDir := filepath.Join(home, "tmp")
	if err := jailMkdir(tmpDir, 0o1777|os.ModeSticky); err != nil {
		cleanup()
		return c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "message": "prepare jail failed: " + err.Error()})
	}
	etc := filepath.Join(home, "etc")
	if err := jailMkdir(etc, 0o755); err != nil {
		cleanup()
		return c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "message": "prepare jail failed: " + err.Error()})
	}
	_ = jailWriteFile(filepath.Join(etc, "passwd"), []byte(fmt.Sprintf("root:x:0:0:root:/root:/sbin/nologin\n%s:x:%s:%s::/:/bin/bash\n", p.Username, u.Uid, u.Gid)), 0o644)
	_ = jailWriteFile(filepath.Join(etc, "group"), []byte(fmt.Sprintf("root:x:0:\n%s:x:%s:\n", p.Username, u.Gid)), 0o644)
	if resolv, err := os.ReadFile("/etc/resolv.conf"); err == nil {
		_ = jailWriteFile(filepath.Join(etc, "resolv.conf"), resolv, 0o644)
	}
	htdocs := filepath.Join(home, "htdocs")
	if err := jailMkdir(htdocs, 0o755); err != nil {
		cleanup()
		return c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "message": "create mount point failed"})
	}
	// The site root is pinned, and after the bind mount (which may run in
	// the host's mount namespace, where the pinned descriptor's path is not
	// usable) the mounted directory must be the pinned one: a root swapped
	// for a symlink (to /etc, say) meanwhile is unmounted and refused.
	sitePin, perr := sitefs.PinDir(p.SiteRoot)
	if perr != nil {
		cleanup()
		return c.JSON(sitefs.StatusOf(perr), map[string]any{"ok": false, "message": perr.Error()})
	}
	defer sitePin.Close()
	mountPoint, _ := os.Stat(htdocs) // the empty directory created above
	out, err = r.hostMount(ctx, "mount", "--bind", p.SiteRoot, htdocs)
	if err == nil {
		fi, serr := os.Stat(htdocs)
		mountedOther := serr == nil && !os.SameFile(fi, sitePin.Info) && (mountPoint == nil || !os.SameFile(fi, mountPoint))
		if mountedOther {
			_, _ = r.hostMount(ctx, "umount", htdocs)
			cleanup()
			return c.JSON(http.StatusBadRequest, map[string]any{"ok": false, "message": "the site root changed while the jail was created"})
		}
	}
	if err != nil {
		cleanup()
		return c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "message": "bind mount of the site failed: " + strings.TrimSpace(out)})
	}
	fstab = append(fstab, fmt.Sprintf("%s %s none bind 0 0  %s", p.SiteRoot, htdocs, sshMarker(p.Username)))
	if err := r.writeFstabLines(fstab); err != nil {
		slog.Warn("agent: fstab not updated (mounts will not survive a reboot)", "error", err)
	}

	if err := r.syncSSHDConfig(ctx, p.Accounts); err != nil {
		cleanup()
		return c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "message": err.Error()})
	}
	slog.Info("agent: ssh/ftp account created", "user", p.Username, "site_root", p.SiteRoot)
	return c.JSON(http.StatusOK, map[string]any{
		"ok": true, "username": p.Username, "home_dir": home,
		"ssh_public_key": string(pub), "ssh_private_key": string(priv),
	})
}

// removeSSHAccount unmounts the jail, removes its fstab lines, deletes the
// Linux user and finally the jail, but only when nothing is mounted in it
// any more (otherwise the site's files would be deleted through htdocs/).
func (r *Router) removeSSHAccount(ctx context.Context, username, home string) error {
	marker := sshMarker(username)
	var kept []string
	for _, l := range r.readFstabLines() {
		f := strings.Fields(l)
		if strings.HasSuffix(strings.TrimSpace(l), marker) || (len(f) >= 2 && (f[1] == home || strings.HasPrefix(f[1], home+"/"))) {
			continue
		}
		kept = append(kept, l)
	}
	_ = r.writeFstabLines(kept)

	targets := []string{filepath.Join(home, "htdocs"), filepath.Join(home, "usr"), filepath.Join(home, "lib64"), filepath.Join(home, "lib"), filepath.Join(home, "bin")}
	for _, dev := range []string{"null", "zero", "urandom", "random", "tty"} {
		targets = append(targets, filepath.Join(home, "dev", dev))
	}
	targets = append(targets, mountedUnder(home)...)
	for _, t := range targets {
		if _, err := r.hostMount(ctx, "umount", t); err != nil {
			_, _ = r.hostMount(ctx, "umount", "-l", t)
		}
	}
	// -f: the account shares the site owner's uid, whose processes (cron,
	// the app) would otherwise block the removal. Without -r no files are
	// removed, and no group is (the account has none of its own).
	siteUserMu.Lock()
	_, _ = r.executor.RunShell(ctx, "userdel", "-f", username)
	siteUserMu.Unlock()
	if left := mountedUnder(home); len(left) > 0 {
		return fmt.Errorf("still mounted, %s kept: %s", home, strings.Join(left, ", "))
	}
	if filepath.Dir(home) != r.sshHomeBase || filepath.Base(home) != username {
		return fmt.Errorf("refusing to remove %s", home)
	}
	return os.RemoveAll(home)
}

// sshDeleteTask answers POST /tasks/sshftp/delete.
func (r *Router) sshDeleteTask(c *echo.Context) error {
	sshTaskMu.Lock()
	defer sshTaskMu.Unlock()
	var p struct {
		Username string          `json:"username"`
		Accounts []sshAccountRef `json:"accounts"`
	}
	if err := c.Bind(&p); err != nil || !sshAccountRe.MatchString(p.Username) {
		return c.JSON(http.StatusBadRequest, map[string]any{"ok": false, "message": "invalid payload"})
	}
	ctx := c.Request().Context()
	removeErr := r.removeSSHAccount(ctx, p.Username, r.sshHome(p.Username))
	if err := r.syncSSHDConfig(ctx, p.Accounts); err != nil {
		slog.Warn("agent: sshd config sync after delete", "error", err)
	}
	if removeErr != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "message": removeErr.Error()})
	}
	return c.JSON(http.StatusOK, map[string]any{"ok": true})
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// jailMkdir creates one directory of a new jail; an existing entry (a
// symlink included) is an error, never reused.
func jailMkdir(path string, mode os.FileMode) error {
	if err := os.Mkdir(path, mode.Perm()); err != nil {
		return err
	}
	return os.Chmod(path, mode) // exact bits (umask, sticky)
}

// jailWriteFile creates a file of a new jail: O_EXCL|O_NOFOLLOW, so nothing
// already there (a planted symlink in particular) is followed or reused.
func jailWriteFile(path string, data []byte, mode os.FileMode) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, mode)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
