#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
failures=0

pass() { printf 'PASS  %s\n' "$1"; }
fail() { printf 'FAIL  %s\n' "$1" >&2; failures=$((failures + 1)); }
contains() {
    local file=$1 pattern=$2 label=$3
    grep -Eq -- "$pattern" "$file" && pass "$label" || fail "$label"
}
not_contains() {
    local file=$1 pattern=$2 label=$3
    if grep -Eq "$pattern" "$file"; then fail "$label"; else pass "$label"; fi
}

panel="$ROOT/deploy/install-panel.sh"
agent="$ROOT/deploy/install-agent.sh"
dashboard="$ROOT/deploy/sigardapanel-info.sh"
generated="$ROOT/internal/apihttp/agents.go"

for file in "$panel" "$agent"; do
    bash -n "$file" && pass "$(basename "$file") syntax" || fail "$(basename "$file") syntax"
done

[[ -x "$dashboard" ]] && pass 'dashboard is packaged and executable' || fail 'dashboard is packaged and executable'
if [[ -f "$dashboard" ]]; then
    bash -n "$dashboard" && pass 'dashboard syntax' || fail 'dashboard syntax'
    contains "$dashboard" 'curl .*127\.0\.0\.1.*\$\{port\}' 'dashboard uses HTTP health probes'
    contains "$dashboard" -- '--watch|-w' 'dashboard supports realtime watch mode'
    contains "$dashboard" 'SIGARDAPANEL_DB_PATH' 'dashboard discovers configured database path'
    not_contains "$dashboard" '/root/SigardaPanel-Enterprise' 'dashboard has no development-only absolute path'
fi

contains "$panel" 'SIGARDAPANEL_API_ADDR=127\.0\.0\.1:7700' 'panel API binds canonical port 7700 to localhost'
contains "$panel" 'AGENT_URL=.*http://127\.0\.0\.1:7710' 'panel defaults to canonical agent port 7710'
contains "$panel" 'PORT=7720' 'web service uses canonical port 7720'
contains "$panel" 'sigardapanel-api' 'panel installs dedicated API service'
contains "$panel" 'sigardapanel-agent' 'panel installs dedicated local agent service'
contains "$panel" 'sigardapanel-web' 'panel installs dedicated web service'
contains "$panel" '/etc/profile\.d/sigardapanel-info\.sh' 'panel installs SSH dashboard'
contains "$panel" 'curl .*127\.0\.0\.1:7700/health' 'panel verifies API health endpoint'
contains "$panel" 'curl .*127\.0\.0\.1:7710/health' 'panel verifies local agent health endpoint'
contains "$panel" 'curl .*127\.0\.0\.1:7720/' 'panel verifies web health endpoint'
not_contains "$panel" 'SIGARDAPANEL_API_ADDR=:8080' 'panel has no stale API port 8080'
not_contains "$panel" 'AGENT_URL=.*9090' 'panel has no stale agent port 9090'

contains "$agent" 'SIGARDAPANEL_AGENT_ADDR=:7710' 'agent uses canonical port 7710'
contains "$agent" 'http://\$SERVER_IP:7710' 'agent reports canonical URL'
contains "$agent" 'curl .*127\.0\.0\.1:7710/health' 'agent verifies health endpoint'
contains "$agent" '/etc/profile\.d/sigardapanel-info\.sh' 'agent installs SSH dashboard'
contains "$agent" "trap rollback_agent_install EXIT" 'agent keeps global rollback trap'
contains "$agent" 'rm -rf.*\.sigardapanel\.previous' 'agent rollback cleans previous binary artifact'
not_contains "$agent" "trap 'rm -f.*DASHBOARD_TMP.*EXIT" 'agent does not overwrite rollback trap'
contains "$agent" "--proto-redir '=https'" 'agent enforces HTTPS redirects'
not_contains "$agent" 'status-dashboard.*\|\|' 'agent dashboard fetch has no fallback'
contains "$agent" "api\.ipify\.org.*--proto-redir '=https'|--proto-redir '=https'.*api\.ipify\.org" 'agent IP discovery restricts redirects to HTTPS'
not_contains "$agent" 'Using existing binary' 'agent has no old-binary fallback'
not_contains "$agent" 'SIGARDAPANEL_AGENT_ADDR=:9090' 'agent has no stale listen port 9090'
not_contains "$agent" 'http://\$SERVER_IP:9090' 'agent has no stale reported port 9090'

contains "$generated" 'SIGARDAPANEL_AGENT_ADDR=:7710' 'generated installer uses canonical agent port'
contains "$generated" '/etc/profile\.d/sigardapanel-info\.sh' 'generated installer installs SSH dashboard'
contains "$generated" 'curl .*localhost:7710/health' 'generated installer verifies agent health'
contains "$generated" 'SIGARDAPANEL_PUBLIC_URL' 'generated installer origin uses configured public URL'
not_contains "$generated" 'Request\(\)\.Host|X-Forwarded-Proto' 'generated installer ignores request-derived origin'
contains "$generated" 'trap rollback_agent_install EXIT' 'generated installer keeps global rollback trap'
contains "$generated" 'rm -rf.*\.sigardapanel\.previous' 'generated rollback cleans previous binary artifact'
not_contains "$generated" "trap 'rm -f.*DASHBOARD_TMP.*EXIT" 'generated installer does not overwrite rollback trap'
not_contains "$generated" 'status-dashboard.*\|\|' 'generated dashboard fetch has no fallback'

if (( failures > 0 )); then
    printf '\n%d installer contract(s) failed\n' "$failures" >&2
    exit 1
fi
printf '\nAll installer contracts passed\n'
