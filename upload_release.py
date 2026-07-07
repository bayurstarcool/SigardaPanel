#!/usr/bin/env python3
import json, subprocess, sys, os

REPO = "bayurstarcool/SigardaPanel"
TAG = "v0.1.4"

token = input("Paste PAT: ").strip()

auth = "Authorization: token " + token

r = subprocess.run(
    ["curl", "-s", "-o", "/dev/null", "-w", "%{http_code}",
     "-H", auth,
     "https://api.github.com/user"],
    capture_output=True, text=True, timeout=15
)
if r.stdout.strip() != "200":
    print("Invalid token")
    sys.exit(1)
print("Token OK!")

body = (
    "## v0.1.1 - Bug Fix\n\n"
    "### Fix\n"
    "- Added all missing migrations (022-041) to init command\n"
    "- Fixes: table users has no column named disk_limit\n"
    "- Also fixed missing migrations in server startup\n\n"
    "### Binaries\n"
    "| Platform | File |\n"
    "|----------|------|\n"
    "| Linux amd64 | sigardapanel |\n"
    "| macOS Intel | sigardapanel-darwin-amd64 |\n"
    "| macOS Apple Silicon | sigardapanel-darwin-arm64 |\n\n"
    "### Quick Start\n"
    "```bash\n"
    "curl -sSL https://github.com/bayurstarcool/SigardaPanel/releases/download/v0.1.1/sigardapanel -o sigardapanel\n"
    "chmod +x sigardapanel\n"
    "sudo mv sigardapanel /usr/local/bin/\n"
    "sigardapanel init\n"
    "```"
)

data = {"tag_name": TAG, "name": TAG + " - Bug Fix", "body": body, "draft": False, "prerelease": False}

r = subprocess.run(
    ["curl", "-s", "-X", "POST",
     "https://api.github.com/repos/" + REPO + "/releases",
     "-H", auth,
     "-H", "Content-Type: application/json",
     "-d", json.dumps(data)],
    capture_output=True, text=True, timeout=30
)
resp = json.loads(r.stdout)
rid = resp.get("id")
if not rid:
    print("ERROR:", resp)
    sys.exit(1)
print("Release created:", resp.get("html_url"))

for fname in ["sigardapanel", "sigardapanel-darwin-amd64", "sigardapanel-darwin-arm64"]:
    fpath = os.path.join("/root/SigardaPanel", fname)
    if not os.path.isfile(fpath):
        print("  SKIP", fname)
        continue
    url = "https://uploads.github.com/repos/" + REPO + "/releases/" + str(rid) + "/assets?name=" + fname
    r = subprocess.run(
        ["curl", "-s", "-X", "POST", url,
         "-H", auth,
         "-H", "Content-Type: application/octet-stream",
         "--data-binary", "@" + fpath],
        capture_output=True, text=True, timeout=300
    )
    a = json.loads(r.stdout)
    print("  " + fname + ":", a.get("state"), a.get("size", 0), "bytes")

print("DONE:", resp.get("html_url"))
