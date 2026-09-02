package commands

import (
	"bufio"
	"io"
	"fmt"
	"net"
	"os"
	"os/exec"
	"time"
	"strings"

	"sigardapanel/internal/auth"
	"sigardapanel/internal/config"
	"sigardapanel/internal/db"
	"sigardapanel/internal/migrations"
	"sigardapanel/internal/store"

	"golang.org/x/term"
)

// Install runs the setup wizard.
func (a *App) Install(args []string) int {
	scanner := bufio.NewScanner(a.stdin)

	fmt.Fprintln(a.stdout, "")
	fmt.Fprintln(a.stdout, "╔══════════════════════════════════════════════════╗")
	fmt.Fprintln(a.stdout, "║         SigardaPanel Setup Wizard v0.5.1        ║")
	fmt.Fprintln(a.stdout, "╚══════════════════════════════════════════════════╝")
	fmt.Fprintln(a.stdout, "")

	// Step 1: Database
	fmt.Fprintln(a.stdout, "[1/5] Checking database...")

	database, err := db.Open(config.DBPath())
	if err != nil {
		fmt.Fprintf(a.stderr, "  ✗ Database: %v\n", err)
		return 1
	}
	defer database.Close()

	if err := migrations.Up(database, migrations.AllMigrations); err != nil {
		fmt.Fprintf(a.stderr, "  ✗ Migration: %v\n", err)
		return 1
	}
	fmt.Fprintln(a.stdout, "  ✓ Database ready")

	// Step 2: Admin user
	fmt.Fprintln(a.stdout, "")
	fmt.Fprintln(a.stdout, "[2/5] Admin user...")

	s := store.New(database)
	existing, _ := s.Users.GetByUsername(a.context(), "admin")

	if existing != nil {
		fmt.Fprintf(a.stdout, "  ✓ Admin '%s' exists (id=%d)\n", existing.Username, existing.ID)
	} else {
		username := readLine(scanner, a.stdout, "  Admin username [admin]")
		if username == "" {
			username = "admin"
		}

		email := readLine(scanner, a.stdout, "  Admin email [admin@localhost]")
		if email == "" {
			email = "admin@localhost"
		}

		password := ""
		for len(password) < 8 {
			pw1 := readPassword(scanner, a.stderr, "  Password (min 8 chars)")
			if len(pw1) < 8 {
				fmt.Fprintln(a.stderr, "  ✗ Too short (min 8)")
				continue
			}
			pw2 := readPassword(scanner, a.stderr, "  Confirm password")
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
			PasswordHash: password,
		}
		if err := s.Users.Create(a.context(), user); err != nil {
			fmt.Fprintf(a.stderr, "  ✗ Create user: %v\n", err)
			return 1
		}
		fmt.Fprintf(a.stdout, "  ✓ Admin '%s' created\n", user.Username)
	}

	// Step 3: Detect IP and show access info
	fmt.Fprintln(a.stdout, "")
	fmt.Fprintln(a.stdout, "[3/5] Detecting server IP...")

	publicIP := getPublicIP()
	localIP := getLocalIP()

	fmt.Fprintf(a.stdout, "  Public IP:  %s\n", publicIP)
	fmt.Fprintf(a.stdout, "  Local IP:   %s\n", localIP)

	// Step 4: Start services
	fmt.Fprintln(a.stdout, "")
	fmt.Fprintln(a.stdout, "[4/5] Starting services...")

	// Check if already running
	apiRunning := isPortOpen(7700)
	frontendRunning := isPortOpen(7720)

	if apiRunning && frontendRunning {
		fmt.Fprintln(a.stdout, "  ✓ Services already running")
	} else {
		// Start dev mode in background
		cmd := exec.Command(os.Args[0], "dev")
		cmd.Stdout = a.stdout
		cmd.Stderr = a.stderr
		cmd.Stdin = os.Stdin

		if err := cmd.Start(); err != nil {
			fmt.Fprintf(a.stderr, "  ✗ Failed to start: %v\n", err)
			fmt.Fprintln(a.stdout, "  Start manually: sigardapanel dev")
		} else {
			fmt.Fprintln(a.stdout, "  ✓ Services started (PID:", cmd.Process.Pid, ")")
			// Wait a moment for ports to open
			for i := 0; i < 10; i++ {
				if isPortOpen(7700) {
					break
				}
				fmt.Fprint(a.stdout, ".")
			}
			fmt.Fprintln(a.stdout, "")
		}
	}

	// Step 5: Show access info
	fmt.Fprintln(a.stdout, "")
	fmt.Fprintln(a.stdout, "[5/5] Access information")
	fmt.Fprintln(a.stdout, "")
	fmt.Fprintln(a.stdout, "╔══════════════════════════════════════════════════╗")
	fmt.Fprintln(a.stdout, "║              Setup Complete!                    ║")
	fmt.Fprintln(a.stdout, "╚══════════════════════════════════════════════════╝")
	fmt.Fprintln(a.stdout, "")
	fmt.Fprintln(a.stdout, "  🌐 Web UI:")
	if publicIP != "" && publicIP != "unknown" {
		fmt.Fprintf(a.stdout, "     https://%s\n", publicIP)
	}
	fmt.Fprintf(a.stdout, "     http://%s:7720\n", localIP)
	fmt.Fprintln(a.stdout, "")
	fmt.Fprintln(a.stdout, "  🔑 Credentials:")
	fmt.Fprintln(a.stdout, "     Username: admin")
	fmt.Fprintln(a.stdout, "     Password: (your password)")
	fmt.Fprintln(a.stdout, "")
	fmt.Fprintln(a.stdout, "  📡 Ports:")
	fmt.Fprintf(a.stdout, "     API:      %s:7700\n", localIP)
	fmt.Fprintf(a.stdout, "     Frontend: %s:7720\n", localIP)
	fmt.Fprintf(a.stdout, "     Agent:    %s:7710\n", localIP)
	fmt.Fprintln(a.stdout, "")
	fmt.Fprintln(a.stdout, "  🚀 Quick start:")
	fmt.Fprintln(a.stdout, "     1. Open the Web UI URL above")
	fmt.Fprintln(a.stdout, "     2. Login with your credentials")
	fmt.Fprintln(a.stdout, "     3. Add your first server")
	fmt.Fprintln(a.stdout, "")
	fmt.Fprintln(a.stdout, "  📖 Docs: https://panel.sigarda.dev/docs")
	fmt.Fprintln(a.stdout, "")

	return 0
}

// getPublicIP returns the server's public IP address.
func getPublicIP() string {
	// Try to get public IP from external service
	resp, err := exec.Command("curl", "-s", "-m", "3", "https://api.ipify.org").Output()
	if err == nil {
		ip := strings.TrimSpace(string(resp))
		if ip != "" && net.ParseIP(ip) != nil {
			return ip
		}
	}

	// Fallback: try another service
	resp, err = exec.Command("curl", "-s", "-m", "3", "https://ifconfig.me").Output()
	if err == nil {
		ip := strings.TrimSpace(string(resp))
		if ip != "" && net.ParseIP(ip) != nil {
			return ip
		}
	}

	return "unknown"
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

func readPassword(scanner *bufio.Scanner, err io.Writer, label string) string {
	fmt.Fprint(err, label+": ")

	// If terminal, hide input
	if term.IsTerminal(int(os.Stdin.Fd())) {
		raw, _ := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(err)
		return strings.TrimSpace(string(raw))
	}

	// Fallback: read from scanner
	if !scanner.Scan() {
		fmt.Fprintln(err)
		return ""
	}
	val := strings.TrimSpace(scanner.Text())
	fmt.Fprintln(err)
	return val
}
