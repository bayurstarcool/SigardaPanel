package commands

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"

	"sigardapanel/internal/auth"
	"sigardapanel/internal/buildinfo"
	"sigardapanel/internal/config"
	"sigardapanel/internal/db"
	"sigardapanel/internal/migrations"
	"sigardapanel/internal/store"

	"golang.org/x/term"
)

// Install runs the setup wizard for development checkouts: it migrates
// SIGARDAPANEL_DB_PATH, creates the first super_admin if there is none and
// tells the developer how to start the services. Production servers use
// install.sh (docs/INSTALL.md).
func (a *App) Install(args []string) int {
	if len(args) > 0 && (args[0] == "-h" || args[0] == "--help" || args[0] == "help") {
		fmt.Fprintln(a.stdout, "usage: sigardapanel install   (interactive setup wizard for development checkouts; production: install.sh)")
		return exitOK
	}
	scanner := bufio.NewScanner(a.stdin)

	fmt.Fprintln(a.stdout, "")
	fmt.Fprintf(a.stdout, "SigardaPanel setup wizard (%s) - development setup\n", buildinfo.Version)
	fmt.Fprintln(a.stdout, "Production servers: use install.sh, see docs/INSTALL.md.")
	fmt.Fprintln(a.stdout, "")

	// Step 1: Database
	fmt.Fprintf(a.stdout, "[1/3] Database %s...\n", config.DBPath())

	database, err := db.Open(config.DBPath())
	if err != nil {
		fmt.Fprintf(a.stderr, "  ✗ Database: %v\n", err)
		return exitError
	}
	defer database.Close()

	if err := migrations.Up(database, migrations.AllMigrations); err != nil {
		fmt.Fprintf(a.stderr, "  ✗ Migration: %v\n", err)
		return exitError
	}
	fmt.Fprintln(a.stdout, "  ✓ Database ready")

	// Step 2: first account
	fmt.Fprintln(a.stdout, "")
	fmt.Fprintln(a.stdout, "[2/3] Admin account...")

	s := store.New(database)
	var existing string
	_ = database.QueryRow("SELECT username FROM users WHERE role = ? ORDER BY id LIMIT 1", auth.RoleSuperAdmin).Scan(&existing)
	username := existing

	if existing != "" {
		fmt.Fprintf(a.stdout, "  ✓ super_admin '%s' already exists\n", existing)
	} else {
		username = readLine(scanner, a.stdout, "  Admin username [admin]")
		if username == "" {
			username = defaultAdminUsername
		}
		if u, _ := s.Users.GetByUsername(a.context(), username); u != nil {
			fmt.Fprintf(a.stderr, "  ✗ user '%s' already exists (not a super_admin); choose another name or use: sigardapanel init --role super_admin\n", username)
			return exitError
		}

		email := readLine(scanner, a.stdout, "  Admin email ["+username+"@localhost]")
		if email == "" {
			email = username + "@localhost"
		}

		password := ""
		for attempt := 0; password == ""; attempt++ {
			if attempt == 3 {
				fmt.Fprintln(a.stderr, "  ✗ no valid password entered")
				return exitError
			}
			pw1, ok := readPassword(scanner, a.stderr, "  Password (min 8 chars)")
			if !ok {
				fmt.Fprintln(a.stderr, "  ✗ no input (stdin closed)")
				return exitError
			}
			if len(pw1) < 8 {
				fmt.Fprintln(a.stderr, "  ✗ Too short (min 8)")
				continue
			}
			pw2, _ := readPassword(scanner, a.stderr, "  Confirm password")
			if pw1 != pw2 {
				fmt.Fprintln(a.stderr, "  ✗ Mismatch")
				continue
			}
			password = pw1
		}

		user := &store.User{
			Username:     username,
			Email:        email,
			Role:         auth.RoleSuperAdmin,
			PasswordHash: password, // Create hashes it
		}
		if err := s.Users.Create(a.context(), user); err != nil {
			fmt.Fprintf(a.stderr, "  ✗ Create user: %v\n", err)
			return exitError
		}
		fmt.Fprintf(a.stdout, "  ✓ super_admin '%s' created\n", user.Username)
	}

	// Step 3: services
	fmt.Fprintln(a.stdout, "")
	fmt.Fprintln(a.stdout, "[3/3] Services")
	localIP := getLocalIP()
	if portOpen(7700) {
		fmt.Fprintln(a.stdout, "  ✓ API is running on :7700")
	} else {
		fmt.Fprintln(a.stdout, "  API + agent:  SIGARDAPANEL_DEV=true sigardapanel dev")
	}
	if portOpen(7720) {
		fmt.Fprintln(a.stdout, "  ✓ Web app is running on :7720")
	} else {
		fmt.Fprintln(a.stdout, "  Web app:      cd web && npx vite dev --host 0.0.0.0 --port 7720")
	}

	fmt.Fprintln(a.stdout, "")
	fmt.Fprintln(a.stdout, "Next:")
	fmt.Fprintf(a.stdout, "  Web UI:  http://%s:7720  (log in as '%s')\n", localIP, username)
	fmt.Fprintf(a.stdout, "  CLI:     sigardapanel login --api-url http://127.0.0.1:7700 --user %s\n", username)
	fmt.Fprintln(a.stdout, "  Docs:    docs/GUIDE.md (development), docs/INSTALL.md (production)")
	fmt.Fprintln(a.stdout, "")

	return exitOK
}

// getLocalIP returns the server's local IP address.
func getLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1"
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}
	return "127.0.0.1"
}

// portOpen is a variable so tests never probe the real ports.
var portOpen = isPortOpen

// isPortOpen checks if a port is listening.
func isPortOpen(port int) bool {
	addr := fmt.Sprintf(":%d", port)
	conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func readLine(scanner *bufio.Scanner, out io.Writer, label string) string {
	fmt.Fprint(out, label+": ")
	if !scanner.Scan() {
		return ""
	}
	return strings.TrimSpace(scanner.Text())
}

// readPassword reads a password (hidden on a terminal). ok is false when
// there is no more input, so callers do not loop forever on a closed stdin.
func readPassword(scanner *bufio.Scanner, err io.Writer, label string) (string, bool) {
	fmt.Fprint(err, label+": ")

	// If terminal, hide input
	if term.IsTerminal(int(os.Stdin.Fd())) {
		raw, rerr := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(err)
		return strings.TrimSpace(string(raw)), rerr == nil
	}

	// Fallback: read from scanner
	if !scanner.Scan() {
		fmt.Fprintln(err)
		return "", false
	}
	val := strings.TrimSpace(scanner.Text())
	fmt.Fprintln(err)
	return val, true
}
