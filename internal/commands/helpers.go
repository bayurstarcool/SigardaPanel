package commands

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"sigardapanel/internal/cli"
)

// Exit codes shared by every CLI command.
const (
	exitOK    = 0 // success (also: --help)
	exitError = 1 // API/network/runtime error, or a watched job that failed
	exitUsage = 2 // bad flags, missing/invalid arguments, unknown subcommand
)

// newFlags returns a flag set whose usage line is
// "Usage: sigardapanel <name> <synopsis>". Parse it with a.parseFlags.
func (a *App) newFlags(name, synopsis string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(a.stderr)
	fs.Usage = func() {
		out := fs.Output()
		fmt.Fprintf(out, "Usage: sigardapanel %s %s\n", name, synopsis)
		hasFlags := false
		fs.VisitAll(func(*flag.Flag) { hasFlags = true })
		if hasFlags {
			fmt.Fprintln(out, "\nFlags:")
			fs.PrintDefaults()
		}
	}
	return fs
}

// parseFlags parses args, allowing positional arguments before, between and
// after flags (so both "site get 5 -o json" and "site get -o json 5" work).
// When ok is false the caller must return code: 0 after printing usage to
// stdout for -h/--help, 2 after a flag error (already reported on stderr).
func (a *App) parseFlags(fs *flag.FlagSet, args []string) (pos []string, code int, ok bool) {
	usage := fs.Usage
	fs.Usage = func() {} // printed below, to the right stream
	defer func() { fs.Usage = usage }()

	rest := args
	for {
		before := len(rest)
		err := fs.Parse(rest)
		if err != nil {
			if errors.Is(err, flag.ErrHelp) {
				fs.SetOutput(a.stdout)
				usage()
				fs.SetOutput(a.stderr)
				return nil, exitOK, false
			}
			usage()
			return nil, exitUsage, false
		}
		remaining := fs.Args()
		consumed := rest[:before-len(remaining)]
		if len(consumed) > 0 && consumed[len(consumed)-1] == "--" {
			pos = append(pos, remaining...)
			break
		}
		if len(remaining) == 0 {
			break
		}
		pos = append(pos, remaining[0])
		rest = remaining[1:]
	}
	return pos, exitOK, true
}

// isHelpArg reports whether args asks for help (used by commands that take
// no flag set).
func isHelpArg(args []string) bool {
	for _, a := range args {
		if a == "-h" || a == "--help" || a == "help" {
			return true
		}
	}
	return false
}

// idArg returns the single numeric ID a command operates on: the positional
// argument if present, otherwise the value of an --id style flag. what is
// used in error messages ("site", "job", ...).
func (a *App) idArg(pos []string, flagVal int64, what string) (int64, bool) {
	if len(pos) > 1 {
		fmt.Fprintf(a.stderr, "too many arguments: expected one %s ID, got %q\n", what, strings.Join(pos, " "))
		return 0, false
	}
	if len(pos) == 1 {
		id, err := strconv.ParseInt(pos[0], 10, 64)
		if err != nil || id <= 0 {
			fmt.Fprintf(a.stderr, "invalid %s ID %q\n", what, pos[0])
			return 0, false
		}
		if flagVal > 0 && flagVal != id {
			fmt.Fprintf(a.stderr, "conflicting %s IDs: %d and --id %d\n", what, id, flagVal)
			return 0, false
		}
		return id, true
	}
	if flagVal > 0 {
		return flagVal, true
	}
	fmt.Fprintf(a.stderr, "%s ID is required (pass <id> or --id)\n", what)
	return 0, false
}

// outputMode resolves --output / SIGARDAPANEL_OUTPUT, reporting an invalid
// flag value on stderr.
func (a *App) outputMode(flagVal string) (cli.OutputMode, bool) {
	mode, err := cli.ResolveOutputMode(flagVal)
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return "", false
	}
	return mode, true
}

// addOutputFlag registers --output and its -o shorthand on fs.
func addOutputFlag(fs *flag.FlagSet) *string {
	out := fs.String("output", "", "output format: table|json|yaml (default $SIGARDAPANEL_OUTPUT or table)")
	fs.StringVar(out, "o", "", "shorthand for --output")
	return out
}

// render prints data as JSON/YAML, or calls table() for table mode.
func (a *App) render(mode cli.OutputMode, data any, table func()) int {
	if mode == cli.OutputTable {
		table()
		return exitOK
	}
	if err := cli.PrintStructured(a.stdout, mode, data); err != nil {
		fmt.Fprintf(a.stderr, "encode: %v\n", err)
		return exitError
	}
	return exitOK
}

// printKV prints an object as aligned "key: value" lines in the given key
// order, followed by any remaining keys sorted alphabetically.
func (a *App) printKV(obj map[string]any, order ...string) {
	seen := map[string]bool{}
	rows := [][]string{}
	for _, k := range order {
		if v, ok := obj[k]; ok {
			rows = append(rows, []string{k + ":", kvValue(v)})
			seen[k] = true
		}
	}
	rest := make([]string, 0, len(obj))
	for k := range obj {
		if !seen[k] {
			rest = append(rest, k)
		}
	}
	sort.Strings(rest)
	for _, k := range rest {
		rows = append(rows, []string{k + ":", kvValue(obj[k])})
	}
	printAligned(a.stdout, rows)
}

func kvValue(v any) string {
	switch v.(type) {
	case map[string]any, []any:
		b, _ := json.Marshal(v)
		return string(b)
	}
	return cli.Cell(v)
}

func printAligned(w io.Writer, rows [][]string) {
	width := 0
	for _, r := range rows {
		if len(r[0]) > width {
			width = len(r[0])
		}
	}
	for _, r := range rows {
		fmt.Fprintf(w, "%-*s %s\n", width, r[0], r[1])
	}
}

// apiClient builds the API client (it never fails today; the error is kept
// for API compatibility).
func (a *App) apiClient() (*cli.Client, bool) {
	c, err := cli.NewClient()
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return nil, false
	}
	return c, true
}

// call performs one API request and checks the status. On a transport error
// or an unexpected status it prints the problem and returns ok=false; the
// caller then returns exitError.
func (a *App) call(c *cli.Client, method, path string, body any, want ...int) (*cli.APIResponse, bool) {
	resp, status, err := c.Request(method, path, body)
	if err != nil {
		fmt.Fprintln(a.stderr, err)
		return nil, false
	}
	for _, w := range want {
		if status == w {
			return resp, true
		}
	}
	if len(want) == 0 && status >= 200 && status < 300 {
		return resp, true
	}
	a.printAPIErrorFor(c, resp, status)
	return nil, false
}

// decode unmarshals resp.Data into v, reporting failures.
func (a *App) decode(resp *cli.APIResponse, v any) bool {
	if resp == nil || len(resp.Data) == 0 {
		fmt.Fprintln(a.stderr, "decode: empty response data")
		return false
	}
	if err := json.Unmarshal(resp.Data, v); err != nil {
		fmt.Fprintf(a.stderr, "decode: %v\n", err)
		return false
	}
	return true
}

// printAPIError reports a non-success API response on stderr and returns
// exitError. The API's own message is shown when present.
func (a *App) printAPIError(resp *cli.APIResponse, status int) int {
	return a.printAPIErrorFor(nil, resp, status)
}

func (a *App) printAPIErrorFor(c *cli.Client, resp *cli.APIResponse, status int) int {
	if resp != nil && resp.Error != nil && resp.Error.Message != "" {
		fmt.Fprintf(a.stderr, "API error (%d %s): %s\n", status, resp.Error.Code, resp.Error.Message)
	} else {
		fmt.Fprintf(a.stderr, "API error: HTTP %d %s\n", status, http.StatusText(status))
	}
	switch status {
	case http.StatusUnauthorized:
		if c != nil && c.TokenSource == "env" {
			fmt.Fprintln(a.stderr, "hint: SIGARDAPANEL_TOKEN is not valid (expired or revoked API key?)")
		} else {
			fmt.Fprintln(a.stderr, "hint: not logged in or session expired; run: sigardapanel login")
		}
	case http.StatusForbidden:
		if c != nil && c.TokenSource == "env" && resp != nil && resp.Error != nil && strings.Contains(resp.Error.Message, "api key") {
			fmt.Fprintln(a.stderr, "hint: this endpoint needs an interactive login session; unset SIGARDAPANEL_TOKEN and use: sigardapanel login")
		}
	}
	return exitError
}

// subcommandHelp handles the shared "unknown subcommand" path.
func (a *App) unknownSubcommand(group, sub string, help func()) int {
	fmt.Fprintf(a.stderr, "unknown %s subcommand: %s\n\n", group, sub)
	saved := a.stdout
	a.stdout = a.stderr
	help()
	a.stdout = saved
	return exitUsage
}
