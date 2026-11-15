package agenthttp

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"sigardapanel/internal/nginx"
	"sigardapanel/internal/sitefs"
)

// Starters written by panels before v1.12.3 listened on every interface
// (Node.js and Go), so the app answered on its port directly, bypassing
// nginx, its TLS and access rules. When a site's starter is still exactly
// what the panel wrote (unchanged by its owner), it is replaced by the
// current one, which listens on 127.0.0.1 and reads $PORT.

func oldNodeStarter(domain string, port int64) string {
	return fmt.Sprintf(`const http = require('http');
const PORT = %d;
const server = http.createServer((req, res) => {
  res.writeHead(200, { 'Content-Type': 'text/html' });
  res.end('<h1>%s</h1><p>Site created via SigardaPanel</p>');
});
server.listen(PORT, () => console.log('Server running on port ' + PORT));
`, port, domain)
}

func oldGoStarter(domain string, port int64) string {
	return fmt.Sprintf(`package main

import (
    "fmt"
    "net/http"
)

func main() {
    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        fmt.Fprintf(w, "<h1>%s</h1><p>Site created via SigardaPanel</p>")
    })
    fmt.Println("Server running on port %d")
    http.ListenAndServe(":%d", nil)
}
`, domain, port, port)
}

// upgradeOldStarter replaces an unchanged pre-v1.12.3 starter. The port in
// the old file may differ from the current one (the port was changed).
func (r *Router) upgradeOldStarter(ctx context.Context, systemUser, rootPath, domain, command string, port int64) {
	var file, runtime string
	switch strings.Fields(command + " x")[0] {
	case "node":
		file, runtime = "server.js", "node"
	case "./app":
		file, runtime = "main.go", "go"
	default:
		return
	}
	pin, err := sitefs.PinDir(rootPath)
	if err != nil {
		return
	}
	defer pin.Close()
	root, err := os.OpenRoot(pin.Path)
	if err != nil {
		return
	}
	defer root.Close()
	fi, err := root.Lstat(file)
	if err != nil || !fi.Mode().IsRegular() || fi.Size() > 4096 {
		return
	}
	cur, err := root.ReadFile(file)
	if err != nil {
		return
	}
	text := string(cur)
	var oldPort int64
	if runtime == "node" {
		if _, err := fmt.Sscanf(strings.SplitN(text, "\n", 3)[1], "const PORT = %d;", &oldPort); err != nil || text != oldNodeStarter(domain, oldPort) {
			return
		}
	} else {
		i := strings.Index(text, `http.ListenAndServe(":`)
		if i < 0 {
			return
		}
		if _, err := fmt.Sscanf(text[i:], `http.ListenAndServe(":%d"`, &oldPort); err != nil || text != oldGoStarter(domain, oldPort) {
			return
		}
	}
	uid, gid, ok := deployUserIDs(systemUser)
	if !ok {
		return
	}
	content := nginx.TemplateForRuntime(runtime, domain, int(port))[file]
	if err := r.writeAsSiteUser(ctx, uid, gid, filepath.Join(pin.Real, file), content); err != nil {
		slog.Warn("agent: upgrade starter", "site", domain, "file", file, "error", err)
		return
	}
	// The new starters serve index.html (the default page).
	if _, err := root.Lstat("index.html"); err != nil {
		page := nginx.TemplateForRuntime(runtime, domain, int(port))["index.html"]
		_ = r.writeAsSiteUser(ctx, uid, gid, filepath.Join(pin.Real, "index.html"), page)
	}
	if runtime == "go" { // rebuild ./app from the new main.go
		_, _ = r.runAsSiteUser(ctx, uid, gid, pin.Real, "rm", "-f", "--", "app")
	}
	slog.Info("agent: replaced the old starter (listened on all interfaces)", "site", domain, "file", file)
}
