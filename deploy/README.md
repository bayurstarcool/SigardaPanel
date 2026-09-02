# SigardaPanel Deployment

## Quick Install — Agent

On target VPS (Ubuntu/Debian/CentOS):

```bash
curl -fsSL https://raw.githubusercontent.com/bayurstarcool/SigardaPanel/main/deploy/install-agent.sh | \
  sudo bash -s -- --panel-url https://panel.example.com --token YOUR_AGENT_TOKEN
```

Agent task endpoints require `SIGARDAPANEL_AGENT_TOKEN` when installed with this script. Keep port `7710` behind firewall/VPN and allow only the panel server.

## Quick Install — Panel

On panel server:

```bash
curl -fsSL https://raw.githubusercontent.com/bayurstarcool/SigardaPanel/main/deploy/install-panel.sh | \
  sudo PANEL_DOMAIN=panel.example.com SIGARDAPANEL_ADMIN_USER=admin SIGARDAPANEL_ADMIN_EMAIL=admin@example.com SIGARDAPANEL_ADMIN_PASSWORD='CHANGE_ME_STRONG' SIGARDAPANEL_AGENT_TOKEN=YOUR_AGENT_TOKEN bash
```

Set `SIGARDAPANEL_AGENT_URL` when the worker talks to a remote agent instead of `http://127.0.0.1:7710`.

## Trial Production Guardrails

- Use HTTPS or private network between panel and agent.
- Set same `SIGARDAPANEL_AGENT_TOKEN` on panel and agent.
- Set `SIGARDAPANEL_AGENT_SITE_ROOT=/var/www` or another controlled root.
- Keep `SIGARDAPANEL_AGENT_ALLOW_STUB_SSL=false` outside dev/test.
- Do not expose agent port `7710` publicly.

## Agent Task Support

- Site create/delete writes/removes generated Nginx config and validates `nginx -t` before reload.
- Backup create/restore uses `.tar.gz` archives under the configured backup directory.
- Database create/delete and user create/delete call local `mysql` or `psql`; install and configure DB admin access on the agent host first.
- Database names and users are restricted to alphanumeric plus underscore.

## Dashboard Live Data

The production installer deploys the SvelteKit adapter-node bundle on port `7720`. For a custom deployment, set:

```bash
SIGARDAPANEL_API_URL=https://panel.example.com
SIGARDAPANEL_TOKEN=YOUR_API_TOKEN
```

Without them, dashboard renders empty states instead of mock data.

## Manual Install

Download binary, init, run:

```bash
curl -fsSL -o /usr/local/bin/sigardapanel \
  https://github.com/bayurstarcool/SigardaPanel/releases/latest/download/sigardapanel
chmod +x /usr/local/bin/sigardapanel
sigardapanel api   # panel mode
sigardapanel agent # agent mode
```
