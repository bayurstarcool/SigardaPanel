package agenthttp

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// passwdEntry is one /etc/passwd line.
type passwdEntry struct {
	Name, Home, Shell string
}

// passwdPath and shellsPath are replaced in tests.
var passwdPath, shellsPath = "/etc/passwd", "/etc/shells"

// passwdByUID reads the account with uid from /etc/passwd.
func passwdByUID(uid uint32) (*passwdEntry, error) {
	f, err := os.Open(passwdPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	want := strconv.FormatUint(uint64(uid), 10)
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		p := strings.Split(sc.Text(), ":")
		if len(p) == 7 && p[2] == want {
			return &passwdEntry{Name: p[0], Home: p[5], Shell: p[6]}, nil
		}
	}
	return nil, fmt.Errorf("account %s not found", want)
}

// loginShellAllowed: the shell is a real login shell listed in /etc/shells
// (not nologin/false), as su would require.
func loginShellAllowed(shell string) bool {
	if shell == "" || !filepath.IsAbs(shell) || strings.HasSuffix(shell, "nologin") || strings.HasSuffix(shell, "/false") {
		return false
	}
	f, err := os.Open(shellsPath)
	if err != nil {
		return false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if strings.TrimSpace(sc.Text()) == shell {
			return true
		}
	}
	return false
}

// siteUserShell returns the login shell of a site user (a real, non-root,
// non-privileged account: siteUserIDs), run with that account's uid, gid
// and groups, in its home, with a clean environment (nothing of the
// agent's, which may hold secrets).
func siteUserShell(name string) (*exec.Cmd, error) {
	uid, gid, ok := siteUserIDs(name)
	if !ok {
		return nil, fmt.Errorf("invalid or unknown user")
	}
	pw, err := passwdByUID(uid)
	if err != nil {
		return nil, err
	}
	if !loginShellAllowed(pw.Shell) {
		return nil, fmt.Errorf("this account has no login shell")
	}
	var groups []uint32
	if u, err := user.LookupId(strconv.FormatUint(uint64(uid), 10)); err == nil {
		if ids, err := u.GroupIds(); err == nil {
			for _, g := range ids {
				if n, err := strconv.ParseUint(g, 10, 32); err == nil {
					groups = append(groups, uint32(n))
				}
			}
		}
	}
	cmd := exec.Command(pw.Shell)
	cmd.Args = []string{"-" + filepath.Base(pw.Shell)} // a login shell, like su -
	cmd.Dir = pw.Home
	if fi, err := os.Stat(pw.Home); err != nil || !fi.IsDir() {
		cmd.Dir = "/"
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: uid, Gid: gid, Groups: groups}}
	cmd.Env = []string{
		"HOME=" + pw.Home, "USER=" + pw.Name, "LOGNAME=" + pw.Name, "SHELL=" + pw.Shell,
		"PATH=/usr/local/bin:/usr/bin:/bin:/usr/local/games:/usr/games",
		"TERM=xterm-256color", "LANG=en_US.UTF-8", "LC_ALL=en_US.UTF-8",
	}
	return cmd, nil
}
