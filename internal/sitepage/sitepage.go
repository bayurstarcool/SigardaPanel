// Package sitepage renders the default page of a new site: a placeholder
// that says the site is live and how to replace it, styled with Tailwind
// (compiled and inlined by scripts/build-site-page.sh, so it makes no
// external requests).
package sitepage

import (
	"bytes"
	_ "embed"
	"html/template"
	"strings"
)

//go:embed page.tmpl
var pageSource string

var page = template.Must(template.New("page").Parse(pageSource))

type runtimeInfo struct {
	RuntimeLabel string
	Replace      string // "own <...>"
	File         string // file shown in the terminal card's title bar
	Files        string // what `ls` shows
	Hint         string
	Watch        string
}

var runtimes = map[string]runtimeInfo{
	"static": {"Static", "HTML, CSS and assets", "index.html", "index.html",
		"replace index.html with your site; nginx serves the files as they are",
		"See every request and error in the site's Logs tab: nginx access and error logs."},
	"php": {"PHP", "PHP app (index.php is served first)", "index.php", "index.html",
		"upload your app; index.php takes over from this page",
		"PHP-FPM errors and nginx logs are in the site's Logs tab."},
	"node": {"Node.js", "Node.js app", "server.js", "index.html  package.json  server.js",
		"edit server.js (it listens on $PORT), then restart from the App tab",
		"Your app's output is in the site's Logs tab; start, stop and restart it from the App tab."},
	"python": {"Python", "Python app", "app.py", "app.py  index.html",
		"edit app.py (it listens on $PORT), then restart from the App tab",
		"Your app's output is in the site's Logs tab; start, stop and restart it from the App tab."},
	"go": {"Go", "Go app", "main.go", "app  go.mod  index.html  main.go",
		"edit main.go; it is rebuilt to ./app when you deploy (it listens on $PORT)",
		"Your app's output is in the site's Logs tab; start, stop and restart it from the App tab."},
	"docker": {"Docker", "app in the container", "index.html", "index.html",
		"the container serves this folder; replace it or change the image",
		"Container output and nginx logs are in the site's Logs tab."},
}

// HTML returns the default page for a site with this domain and runtime.
func HTML(domain, runtime string) string {
	info, ok := runtimes[runtime]
	if !ok {
		info = runtimes["static"]
	}
	data := struct {
		runtimeInfo
		Domain string
	}{info, domain}
	var b bytes.Buffer
	if err := page.Execute(&b, data); err != nil {
		// Cannot happen with the embedded template; keep a usable page.
		return "<!DOCTYPE html><title>" + template.HTMLEscapeString(domain) + "</title><h1>" + template.HTMLEscapeString(domain) + "</h1><p>Site is live.</p>"
	}
	return strings.TrimPrefix(b.String(), "\n")
}
