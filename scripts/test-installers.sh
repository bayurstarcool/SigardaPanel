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

installer="$ROOT/installer/install.sh"
dashboard="$ROOT/deploy/sigardapanel-info.sh"
generated="$ROOT/internal/apihttp/agents.go"

bash -n "$installer" && pass 'install.sh syntax' || fail 'install.sh syntax'
cmp -s "$installer" "$ROOT/install.sh" && pass 'top-level install.sh is identical to installer/install.sh' || fail 'top-level install.sh is identical to installer/install.sh'

[[ -x "$dashboard" ]] && pass 'dashboard is packaged and executable' || fail 'dashboard is packaged and executable'
if [[ -f "$dashboard" ]]; then
    bash -n "$dashboard" && pass 'dashboard syntax' || fail 'dashboard syntax'
    contains "$dashboard" 'curl .*127\.0\.0\.1.*\$\{port\}' 'dashboard uses HTTP health probes'
    contains "$dashboard" -- '--watch|-w' 'dashboard supports realtime watch mode'
    contains "$dashboard" 'SIGARDAPANEL_DB_PATH' 'dashboard discovers configured database path'
    not_contains "$dashboard" '/root/SigardaPanel-Enterprise' 'dashboard has no development-only absolute path'
fi

contains "$installer" 'SIGARDAPANEL_API_ADDR=127\.0\.0\.1:7700' 'installer binds API to localhost:7700'
contains "$installer" 'SIGARDAPANEL_AGENT_ADDR=127\.0\.0\.1:7710' 'installer binds local agent to localhost:7710'
contains "$installer" 'PORT=7720' 'installer web service uses port 7720'
contains "$installer" 'SIGARDAPANEL_ALLOW_REGISTRATION=false' 'installer closes public sign-up'
contains "$installer" "role='super_admin'" 'installer makes the first account super_admin'
contains "$installer" 'ADMIN_PASS_GENERATED=1' 'installer tracks whether it generated the admin password'
contains "$installer" 'password_changed_at=\$\{pw_changed\}' 'installer records the admin password state (generated: change required; ADMIN_PASS: not)'
contains "$installer" 'minisign -Vm' 'installer verifies the release signature'

contains "$generated" 'SIGARDAPANEL_AGENT_ADDR=:7710' 'generated installer uses canonical agent port'
contains "$generated" '/etc/profile\.d/sigardapanel-info\.sh' 'generated installer installs SSH dashboard'
contains "$generated" 'curl .*localhost:7710/health' 'generated installer verifies agent health'
contains "$generated" 'SIGARDAPANEL_PUBLIC_URL' 'generated installer origin uses configured public URL'
not_contains "$generated" 'Request\(\)\.Host|X-Forwarded-Proto' 'generated installer ignores request-derived origin'
contains "$generated" 'trap rollback_agent_install EXIT' 'generated installer keeps global rollback trap'
contains "$generated" 'rm -rf.*\.sigardapanel\.previous' 'generated rollback cleans previous binary artifact'
not_contains "$generated" "trap 'rm -f.*DASHBOARD_TMP.*EXIT" 'generated installer does not overwrite rollback trap'
not_contains "$generated" 'status-dashboard.*\|\|' 'generated dashboard fetch has no fallback'

agent_updater="$ROOT/installer/agent-update.sh"
bash -n "$agent_updater" && pass 'agent-update.sh syntax' || fail 'agent-update.sh syntax'
key_install=$(sed -n 's/^PUBKEY="\(.*\)"$/\1/p' "$installer")
key_agent=$(sed -n 's/^PUBKEY="\(.*\)"$/\1/p' "$agent_updater")
key_api=$(sed -n 's/^const releasePublicKey = "\(.*\)"$/\1/p' "$generated")
[[ -n "$key_install" && "$key_install" == "$key_agent" && "$key_install" == "$key_api" ]] \
    && pass 'install.sh, agent-update.sh and the agent install command pin the same release key' \
    || fail 'install.sh, agent-update.sh and the agent install command pin the same release key'
contains "$installer" 'do_rollback "\$\{1:-\}"' 'installer offers rollback'

# Behaviour: update/rollback and the agent updater against stubbed system tools.
if bash "$ROOT/scripts/test-installer-sandbox.sh" "$installer" > /tmp/sigarda-sandbox.$$ 2>&1; then
    pass 'install.sh update/rollback sandbox'
else
    cat /tmp/sigarda-sandbox.$$ >&2; fail 'install.sh update/rollback sandbox'
fi
if bash "$ROOT/scripts/test-agent-update-sandbox.sh" "$agent_updater" > /tmp/sigarda-sandbox.$$ 2>&1; then
    pass 'agent-update.sh sandbox'
else
    cat /tmp/sigarda-sandbox.$$ >&2; fail 'agent-update.sh sandbox'
fi
rm -f /tmp/sigarda-sandbox.$$

if (( failures > 0 )); then
    printf '\n%d installer contract(s) failed\n' "$failures" >&2
    exit 1
fi
printf '\nAll installer contracts passed\n'
