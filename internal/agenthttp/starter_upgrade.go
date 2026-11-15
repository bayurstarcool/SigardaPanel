package agenthttp

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"sigardapanel/internal/nginx"
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
	path := filepath.Join(rootPath, file)
	fi, err := os.Lstat(path)
	if err != nil || !fi.Mode().IsRegular() || fi.Size() > 4096 {
		return
	}
	cur, err := os.ReadFile(path)
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
	if err := r.writeAsSiteUser(ctx, uid, gid, path, content); err != nil {
		slog.Warn("agent: upgrade starter", "path", path, "error", err)
		return
	}
	if runtime == "go" { // rebuild ./app from the new main.go
		_, _ = r.runAsSiteUser(ctx, uid, gid, rootPath, "rm", "-f", "--", "app")
	}
	slog.Info("agent: replaced the old starter (listened on all interfaces)", "path", path)
}
