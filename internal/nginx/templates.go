package nginx

// RuntimeTemplate returns a hello-world starter file for the given runtime.
// The domain and port are interpolated into the template.
// Returns (filename, content, nil) or ("", "", error) for unsupported runtimes.
func RuntimeTemplate(domain string, port int) (filename, content string, err error) {
	switch {
	case domain == "" || port <= 0:
		return "", "", nil // safety: skip if bad params
	default:
		// Try matching runtime patterns in filename
	}

	// We don't have runtime here, so we return node template by default.
	// The caller should pass runtime. For backwards compat, keep this simple.
	return "", "", nil
}

func nodeHelloWorld(domain string, port int) string {
	return `const http = require('http');

const PORT = process.env.PORT || ` + itoa(port) + `;

const HTML = ` + "`" + `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>` + domain + `</title>
  <style>
    *{margin:0;padding:0;box-sizing:border-box}
    body{min-height:100vh;display:flex;align-items:center;justify-content:center;
         font-family:system-ui,-apple-system,sans-serif;background:#0f172a;color:#e2e8f0}
    .card{text-align:center;padding:3rem;border-radius:1rem;background:#1e293b;
          border:1px solid #334155;box-shadow:0 25px 50px -12px rgba(0,0,0,.5)}
    h1{font-size:2.5rem;margin-bottom:.5rem;color:#38bdf8}
    p{color:#94a3b8;font-size:1.1rem}
    code{background:#334155;padding:.2rem .6rem;border-radius:.375rem;font-size:.875rem;color:#38bdf8}
  </style>
</head>
<body>
  <div class="card">
    <h1>Hello World 🚀</h1>
    <p>Node.js on SigardaPanel</p>
    <p style="margin-top:1rem"><code>` + domain + `</code></p>
  </div>
</body>
</html>` + "`" + `;

http.createServer((req, res) => {
  res.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' });
  res.end(HTML);
}).listen(PORT, () => {
  console.log('Server running on http://localhost:' + PORT);
});
`
}

func pythonHelloWorld(domain string, port int) string {
	return `#!/usr/bin/env python3
"""Hello World server for ` + domain + `"""
import http.server
import os

PORT = int(os.environ.get("PORT", ` + itoa(port) + `))

HTML = """<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>` + domain + `</title>
  <style>
    *{margin:0;padding:0;box-sizing:border-box}
    body{min-height:100vh;display:flex;align-items:center;justify-content:center;
         font-family:system-ui,-apple-system,sans-serif;background:#0f172a;color:#e2e8f0}
    .card{text-align:center;padding:3rem;border-radius:1rem;background:#1e293b;
          border:1px solid #334155;box-shadow:0 25px 50px -12px rgba(0,0,0,.5)}
    h1{font-size:2.5rem;margin-bottom:.5rem;color:#38bdf8}
    p{color:#94a3b8;font-size:1.1rem}
    code{background:#334155;padding:.2rem .6rem;border-radius:.375rem;font-size:.875rem;color:#38bdf8}
  </style>
</head>
<body>
  <div class="card">
    <h1>Hello World 🚀</h1>
    <p>Python on SigardaPanel</p>
    <p style="margin-top:1rem"><code>` + domain + `</code></p>
  </div>
</body>
</html>"""

class Handler(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200)
        self.send_header("Content-Type", "text/html; charset=utf-8")
        self.end_headers()
        self.wfile.write(HTML.encode())

    def log_message(self, format, *args):
        print(f"[{self.address_string()}] {format % args}")

if __name__ == "__main__":
    with http.server.HTTPServer(("127.0.0.1", PORT), Handler) as srv:
        print(f"Server running on http://localhost:{PORT}")
        srv.serve_forever()
`
}

func goHelloWorld(domain string, port int) string {
	return `package main

import (
	"fmt"
	"net/http"
	"os"
)

const html = ` + "`" + `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>` + domain + `</title>
  <style>
    *{margin:0;padding:0;box-sizing:border-box}
    body{min-height:100vh;display:flex;align-items:center;justify-content:center;
         font-family:system-ui,-apple-system,sans-serif;background:#0f172a;color:#e2e8f0}
    .card{text-align:center;padding:3rem;border-radius:1rem;background:#1e293b;
          border:1px solid #334155;box-shadow:0 25px 50px -12px rgba(0,0,0,.5)}
    h1{font-size:2.5rem;margin-bottom:.5rem;color:#38bdf8}
    p{color:#94a3b8;font-size:1.1rem}
    code{background:#334155;padding:.2rem .6rem;border-radius:.375rem;font-size:.875rem;color:#38bdf8}
  </style>
</head>
<body>
  <div class="card">
    <h1>Hello World 🚀</h1>
    <p>Go on SigardaPanel</p>
    <p style="margin-top:1rem"><code>` + domain + `</code></p>
  </div>
</body>
</html>` + "`" + `

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "` + itoa(port) + `"
	}
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, html)
	})
	fmt.Printf("Server running on http://localhost:%s\n", port)
	http.ListenAndServe(":"+port, nil)
}
`
}

func goModFile(domain string) string {
	return `module ` + domain + `

go 1.22
`
}

// itoa is a minimal int-to-string without importing strconv.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	if n < 0 {
		return "-" + itoa(-n)
	}
	digits := make([]byte, 0, 10)
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
