package executor

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// Executor provides safe OS-level operations with timeout and command whitelisting.
type Executor struct {
	Timeout time.Duration // default 60s per command
}

// RunWithInput executes an allowlisted command with stdin and timeout.
func (e *Executor) RunWithInput(ctx context.Context, input string, name string, args ...string) (string, error) {
	if !isAllowedCommand(name) {
		return "", fmt.Errorf("command not allowed: %s", name)
	}

	timeout := e.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = bytes.NewBufferString(input)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("%s failed: %w", name, err)
	}
	return string(output), nil
}

// RunShell executes a shell command with timeout and validation against an allowlist.
func (e *Executor) RunShell(ctx context.Context, name string, args ...string) (string, error) {
	if !isAllowedCommand(name) {
		return "", fmt.Errorf("command not allowed: %s", name)
	}

	timeout := e.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("%s failed: %w", name, err)
	}
	return string(output), nil
}

// RunShellEnv executes a shell command with custom environment variables.
func (e *Executor) RunShellEnv(ctx context.Context, env []string, name string, args ...string) (string, error) {
	if !isAllowedCommand(name) {
		return "", fmt.Errorf("command not allowed: %s", name)
	}

	timeout := e.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(cmd.Environ(), env...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("%s failed: %w", name, err)
	}
	return string(output), nil
}

// RunToFileEnv executes an allowlisted command with custom environment
// variables, writing stdout to the file at outPath. No shell is involved,
// so command arguments and environment values are never interpolated into a
// shell string. Stderr is captured and returned for diagnostics.
func (e *Executor) RunToFileEnv(ctx context.Context, env []string, outPath, name string, args ...string) (string, error) {
	if !isAllowedCommand(name) {
		return "", fmt.Errorf("command not allowed: %s", name)
	}

	timeout := e.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	f, err := os.OpenFile(outPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return "", fmt.Errorf("open output file: %w", err)
	}
	defer f.Close()

	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(cmd.Environ(), env...)
	cmd.Stdout = f
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return stderr.String(), fmt.Errorf("%s failed: %w", name, err)
	}
	return stderr.String(), nil
}

// RunShellDir executes an allowlisted command with the working directory set to
// dir, using an argument array (no shell). This avoids fragile "cd X && cmd"
// shell concatenation for path-scoped commands.
func (e *Executor) RunShellDir(ctx context.Context, dir, name string, args ...string) (string, error) {
	if !isAllowedCommand(name) {
		return "", fmt.Errorf("command not allowed: %s", name)
	}

	timeout := e.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("%s failed: %w", name, err)
	}
	return string(output), nil
}

// Mkdir creates a directory recursively.
func (e *Executor) Mkdir(ctx context.Context, path string) error {
	return os.MkdirAll(path, 0755)
}

// WriteFile writes content to a file with the given mode.
func (e *Executor) WriteFile(ctx context.Context, path, content string, mode os.FileMode) error {
	return os.WriteFile(path, []byte(content), mode)
}

// RemoveAll removes a path recursively.
func (e *Executor) RemoveAll(ctx context.Context, path string) error {
	return os.RemoveAll(path)
}

// Stat checks if a path exists.
func (e *Executor) Stat(ctx context.Context, path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// EnsureDir ensures a directory exists, creating it if necessary.
func (e *Executor) EnsureDir(ctx context.Context, path string) error {
	exists, err := e.Stat(ctx, path)
	if err != nil {
		return err
	}
	if !exists {
		return e.Mkdir(ctx, path)
	}
	return nil
}

var allowedCommands = map[string]bool{
	"systemctl":       true,
	"service":         true,
	"nginx":           true,
	"certbot":         true,
	"git":             true,
	"tar":             true,
	"tail":            true,
	"openssl":         true,
	"mysql":           true,
	"mysqldump":       true,
	"psql":            true,
	"pg_dump":         true,
	"stat":            true,
	"mkdir":           true,
	"echo":            true,
	"aws":             true,
	"sh":              true,
	"bash":            true,
	"cp":              true,
	"mv":              true,
	"find":            true,
	"sysctl":          true,
	"du":              true,
	"cat":             true,
	"wc":              true,
	"head":            true,
	"sleep":           true,
	"curl":            true,
	"apt-get":         true,
	"apt-cache":       true,
	"apt":             true,
	"yum":             true,
	"dnf":             true,
	"apk":             true,
	"docker":          true,
	"node":            true,
	"npm":             true,
	"pip":             true,
	"pip3":            true,
	"go":              true,
	"redis-server":    true,
	"redis-cli":       true,
	"memcached":       true,
	"ufw":             true,
	"supervisord":     true,
	"mariadb":         true,
	"php":             true,
	"php8.4":          true,
	"php8.3":          true,
	"php8.2":          true,
	"php8.1":          true,
	"php8.0":          true,
	"make":            true,
	"gcc":             true,
	"python3":         true,
	"python":          true,
	"fail2ban-client": true,
	"which":           true,
	"test":            true,
}

func isAllowedCommand(name string) bool {
	base := filepath.Base(name)
	return allowedCommands[base]
}
