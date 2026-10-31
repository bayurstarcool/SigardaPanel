<script lang="ts">
  import { onMount } from 'svelte';
  import { Menu, X, BookOpen } from 'lucide-svelte';
  import Logo from '$components/Logo.svelte';

  let activeSection = $state('getting-started');
  let sidebarOpen = $state(false);

  // Order matches the sections in the page body.
  const sections = [
    { id: 'getting-started', title: 'Getting Started' },
    { id: 'servers', title: 'Server Management' },
    { id: 'sites', title: 'Site Management' },
    { id: 'databases', title: 'Databases' },
    { id: 'backups', title: 'Backup System' },
    { id: 'security', title: 'Security' },
    { id: 'docker', title: 'Docker' },
    { id: 'firewall', title: 'Firewall' },
    { id: 'redis', title: 'Redis' },
    { id: 'git', title: 'Git Deploy' },
    { id: 'cli', title: 'CLI Reference' },
    { id: 'api', title: 'API Reference' },
    { id: 'troubleshooting', title: 'Troubleshooting' },
  ];

  function scrollTo(id: string) {
    activeSection = id;
    sidebarOpen = false;
    document.getElementById(id)?.scrollIntoView({ behavior: 'smooth', block: 'start' });
  }

  onMount(() => {
    const observer = new IntersectionObserver(
      (entries) => {
        entries.forEach((e) => {
          if (e.isIntersecting) activeSection = e.target.id;
        });
      },
      { rootMargin: '-20% 0px -70% 0px' }
    );
    sections.forEach((s) => {
      const el = document.getElementById(s.id);
      if (el) observer.observe(el);
    });
    return () => observer.disconnect();
  });
</script>

<svelte:head>
  <title>SigardaPanel — Documentation</title>
</svelte:head>

{#snippet toc()}
  <nav aria-label="Table of contents" class="space-y-0.5">
    {#each sections as section (section.id)}
      {@const active = activeSection === section.id}
      <a
        href="#{section.id}"
        onclick={(e) => { e.preventDefault(); scrollTo(section.id); }}
        aria-current={active ? 'location' : undefined}
        class="block rounded-md border-l-2 px-3 py-1.5 text-[13px] transition-colors duration-150
          {active ? 'border-accent bg-surface-2 font-medium text-fg' : 'border-transparent text-muted hover:bg-surface-2 hover:text-fg'}"
      >
        {section.title}
      </a>
    {/each}
  </nav>
{/snippet}

<div class="min-h-dvh bg-bg text-fg">
  <header class="sticky top-0 z-40 border-b border-line bg-bg">
    <div class="mx-auto flex h-14 max-w-7xl items-center justify-between gap-3 px-4 sm:px-6">
      <div class="flex items-center gap-2">
        <button onclick={() => (sidebarOpen = !sidebarOpen)} class="btn btn-ghost btn-sm btn-icon lg:hidden" aria-label="Open table of contents">
          <Menu size={16} />
        </button>
        <a href="/" class="flex items-center gap-2" aria-label="SigardaPanel home">
          <Logo size={26} withText />
        </a>
        <span class="badge max-sm:hidden"><BookOpen size={12} /> Docs</span>
      </div>
      <div class="flex items-center gap-1">
        <a href="/" class="btn btn-ghost btn-sm">Dashboard</a>
        <a href="https://github.com/bayurstarcool/SigardaPanel" target="_blank" rel="noopener" class="btn btn-ghost btn-sm">GitHub</a>
      </div>
    </div>
  </header>

  <div class="mx-auto flex max-w-7xl">
    <aside class="sticky top-14 hidden h-[calc(100dvh-3.5rem)] w-60 shrink-0 overflow-y-auto border-r border-line px-3 py-6 lg:block">
      <p class="mb-2 px-3 text-xs font-medium text-subtle">On this page</p>
      {@render toc()}
    </aside>

    {#if sidebarOpen}
      <div class="fixed inset-0 z-50 lg:hidden">
        <button class="overlay animate-fade-in" aria-label="Close table of contents" onclick={() => (sidebarOpen = false)}></button>
        <aside class="absolute inset-y-0 left-0 z-50 w-72 max-w-[85vw] animate-pop-in overflow-y-auto border-r border-line bg-elevated p-4">
          <div class="mb-4 flex items-center justify-between">
            <span class="text-sm font-semibold text-fg">On this page</span>
            <button onclick={() => (sidebarOpen = false)} class="btn btn-ghost btn-xs btn-icon" aria-label="Close"><X size={14} /></button>
          </div>
          {@render toc()}
        </aside>
      </div>
    {/if}

    <main class="min-w-0 flex-1 px-4 py-8 sm:px-8 lg:px-12 lg:py-10">
      <div class="mx-auto max-w-3xl">
      <div class="mb-10">
        <h1 class="text-3xl font-semibold tracking-tight text-fg">Documentation</h1>
        <p class="mt-2 text-muted">Everything you need to install, operate and automate SigardaPanel.</p>
      </div>

      <!-- Getting Started -->
      <section id="getting-started" class="scroll-mt-20 border-b border-line pb-12 mb-12">
        <h2 class="text-2xl font-semibold tracking-tight text-fg">Getting Started</h2>
        <p class="mt-1.5 mb-2 text-muted">Set up SigardaPanel in minutes.</p>

        <h3 class="mt-8 mb-3 text-base font-semibold text-fg">What is SigardaPanel?</h3>
        <p class="mb-6 leading-relaxed text-muted">SigardaPanel is a modern VPS control panel built with Go + SvelteKit. It manages multiple servers, sites, databases, backups, SSL certificates, Docker, firewall, and monitoring — all from one dashboard.</p>

        <h3 class="mt-8 mb-3 text-base font-semibold text-fg">System Requirements</h3>
        <ul class="mb-6 list-disc space-y-2 pl-5 leading-relaxed text-muted marker:text-subtle">
          <li><strong>Panel Server:</strong> Ubuntu 20.04+, 1GB RAM, 10GB disk</li>
          <li><strong>Target VPS:</strong> Any Linux with systemd</li>
          <li><strong>Network:</strong> Panel must reach agent on port 7710</li>
          <li><strong>Dependencies:</strong> Go 1.22+, Node.js 18+ (for dashboard)</li>
        </ul>

        <h3 class="mt-8 mb-3 text-base font-semibold text-fg">Installation</h3>
        <div class="mb-6 rounded-lg border border-line bg-surface-2 p-4">
          <pre class="overflow-x-auto font-mono text-[13px] leading-relaxed text-fg"><code># Download and install
curl -sSL https://github.com/bayurstarcool/SigardaPanel/releases/latest/download/sigardapanel -o sigardapanel
chmod +x sigardapanel

# Run setup wizard
./sigardapanel install

# Or start directly
./sigardapanel dev</code></pre>
        </div>

        <h3 class="mt-8 mb-3 text-base font-semibold text-fg">Access</h3>
        <div class="rounded-lg border border-line bg-surface-2 p-4">
          <div class="grid grid-cols-1 sm:grid-cols-3 gap-4 text-sm">
            <div>
              <div class="mb-1 text-xs text-subtle">Web UI</div>
              <code class="font-mono text-[13px] text-fg">http://YOUR_IP:7720</code>
            </div>
            <div>
              <div class="mb-1 text-xs text-subtle">API</div>
              <code class="font-mono text-[13px] text-fg">http://YOUR_IP:7700</code>
            </div>
            <div>
              <div class="mb-1 text-xs text-subtle">Agent</div>
              <code class="font-mono text-[13px] text-fg">http://YOUR_IP:7710</code>
            </div>
          </div>
        </div>
      </section>

      <!-- Server Management -->
      <section id="servers" class="scroll-mt-20 border-b border-line pb-12 mb-12">
        <h2 class="text-2xl font-semibold tracking-tight text-fg">Server Management</h2>
        <p class="mt-1.5 mb-2 text-muted">Add, monitor, and manage your VPS fleet.</p>

        <h3 class="mt-8 mb-3 text-base font-semibold text-fg">Adding a Server</h3>
        <div class="mb-4 rounded-lg border border-line bg-surface-2 p-4">
          <pre class="overflow-x-auto font-mono text-[13px] leading-relaxed text-fg"><code># Via CLI
sigardapanel server add --name "Production" --hostname prod.example.com --ip 1.2.3.4

# Output:
# server created (id=1)
# name: Production
# hostname: prod.example.com
# agent_token: abc123def456...
# 
# save this token — it won't be shown again</code></pre>
        </div>

        <h3 class="mt-8 mb-3 text-base font-semibold text-fg">List Servers</h3>
        <div class="mb-4 rounded-lg border border-line bg-surface-2 p-4">
          <pre class="overflow-x-auto font-mono text-[13px] leading-relaxed text-fg"><code>sigardapanel server list

# Output:
# ID  NAME       HOSTNAME     STATUS   
# 1   My Server  prod.example.com  active</code></pre>
        </div>

        <h3 class="mt-8 mb-3 text-base font-semibold text-fg">Update Server</h3>
        <div class="mb-4 rounded-lg border border-line bg-surface-2 p-4">
          <pre class="overflow-x-auto font-mono text-[13px] leading-relaxed text-fg"><code>sigardapanel server update --id 1 --name "Production Server" --status active

# Output:
# server updated (id=1)
#   name: Production Server
#   hostname: prod.example.com</code></pre>
        </div>

        <h3 class="mt-8 mb-3 text-base font-semibold text-fg">Health Monitoring</h3>
        <p class="mb-4 leading-relaxed text-muted">Every 60 seconds, the panel pings each agent and checks response time.</p>
        <div class="flex flex-wrap gap-3">
          <span class="badge badge-success">Online (&lt;1s)</span>
          <span class="badge badge-warning">Degraded (&gt;1s)</span>
          <span class="badge badge-danger">Offline</span>
        </div>
      </section>

      <!-- Site Management -->
      <section id="sites" class="scroll-mt-20 border-b border-line pb-12 mb-12">
        <h2 class="text-2xl font-semibold tracking-tight text-fg">Site Management</h2>
        <p class="mt-1.5 mb-2 text-muted">Create and manage websites across your servers.</p>

        <h3 class="mt-8 mb-3 text-base font-semibold text-fg">Create a Site</h3>
        <div class="mb-4 rounded-lg border border-line bg-surface-2 p-4">
          <pre class="overflow-x-auto font-mono text-[13px] leading-relaxed text-fg"><code>sigardapanel site create --server 1 --name "My Blog" --domain blog.example.com --runtime node

# Output:
# site created (id=1, domain=blog.example.com)
# job_id: 5</code></pre>
        </div>

        <h3 class="mt-8 mb-3 text-base font-semibold text-fg">List Sites</h3>
        <div class="mb-4 rounded-lg border border-line bg-surface-2 p-4">
          <pre class="overflow-x-auto font-mono text-[13px] leading-relaxed text-fg"><code>sigardapanel site list

# Output:
# ID  NAME       DOMAIN              RUNTIME  STATUS
# 1   My Blog    blog.example.com    node     active
# 2   API Server api.example.com     go       active</code></pre>
        </div>

        <h3 class="mt-8 mb-3 text-base font-semibold text-fg">Supported Runtimes</h3>
        <div class="flex flex-wrap gap-2 mb-6">
          {#each ['Static', 'PHP (Multi-Version 7.4 - 8.5)', 'Node.js', 'Python', 'Go', 'Proxy', 'Docker'] as runtime (runtime)}
            <span class="badge">{runtime}</span>
          {/each}
        </div>
      </section>

      <!-- Databases -->
      <section id="databases" class="scroll-mt-20 border-b border-line pb-12 mb-12">
        <h2 class="text-2xl font-semibold tracking-tight text-fg">Databases</h2>
        <p class="mt-1.5 mb-2 text-muted">Manage MariaDB/MySQL & PostgreSQL databases, users, and integrated phpMyAdmin.</p>

        <h3 class="mt-8 mb-3 text-base font-semibold text-fg">phpMyAdmin Web Access</h3>
        <p class="mb-3 text-sm leading-relaxed text-muted">SigardaPanel provides direct integrated phpMyAdmin with single-click authentication.</p>
        <div class="mb-4 rounded-lg border border-line bg-surface-2 p-4">
          <pre class="overflow-x-auto font-mono text-[13px] leading-relaxed text-fg"><code># Access via panel URL:
https://cp.sigarda.dev/pma/

# Or click "phpMyAdmin" on any database in the Databases dashboard for auto-login ticket.</code></pre>
        </div>
      </section>

      <!-- Backup System -->
      <section id="backups" class="scroll-mt-20 border-b border-line pb-12 mb-12">
        <h2 class="text-2xl font-semibold tracking-tight text-fg">Backup System</h2>
        <p class="mt-1.5 mb-2 text-muted">Manual and scheduled backups with local storage and cloud provider support (S3, Cloudflare R2, Backblaze B2, Wasabi).</p>

        <h3 class="mt-8 mb-3 text-base font-semibold text-fg">Manual Backup</h3>
        <div class="mb-4 rounded-lg border border-line bg-surface-2 p-4">
          <pre class="overflow-x-auto font-mono text-[13px] leading-relaxed text-fg"><code># Trigger site backup via CLI:
sigardapanel backup create --site 1

# Or click "+ Create Backup" on the Backups page to select Full, Files-only, or DB-only.</code></pre>
        </div>

        <h3 class="mt-8 mb-3 text-base font-semibold text-fg">Auto-Retention & Cloud Cleanup</h3>
        <p class="mb-3 text-sm leading-relaxed text-muted">Backup retention automatically keeps the latest 7 backups per site, removing expired archives from local disk and cloud S3 buckets simultaneously to prevent storage overflow.</p>
      </section>

      <!-- Security -->
      <section id="security" class="scroll-mt-20 border-b border-line pb-12 mb-12">
        <h2 class="text-2xl font-semibold tracking-tight text-fg">Security & DDoS Defense</h2>
        <p class="mt-1.5 mb-2 text-muted">Hardened OS, Fail2ban intrusion detection, kernel SYN flood defense, and malware monitoring.</p>

        <h3 class="mt-8 mb-3 text-base font-semibold text-fg">Active Protections</h3>
        <ul class="mb-4 list-disc space-y-2 pl-5 text-sm leading-relaxed text-muted marker:text-subtle">
          <li><strong>Firewall (UFW):</strong> Default-deny incoming policy, only ports 22, 80, 443 permitted.</li>
          <li><strong>Anti-DDoS Shield:</strong> Kernel SYN-cookies (<code class="code">net.ipv4.tcp_syncookies=1</code>), Slowloris timeout mitigation (15s client timeouts), per-IP connection limits.</li>
          <li><strong>Fail2ban:</strong> Real-time monitoring and IP ban for SSH bruteforce and Nginx bot searches.</li>
          <li><strong>DB Isolation:</strong> Database ports (3306, 5432, 6379, 8081) bound strictly to <code class="code">127.0.0.1</code> (localhost only).</li>
          <li><strong>Web Integrity:</strong> Automated detection of PHP webshell signatures in site webroots.</li>
        </ul>
      </section>

      <!-- Docker -->
      <section id="docker" class="scroll-mt-20 border-b border-line pb-12 mb-12">
        <h2 class="text-2xl font-semibold tracking-tight text-fg">Docker Management</h2>
        <p class="mt-1.5 mb-2 text-muted">Full Docker container, image, volume, and network management.</p>

        <h3 class="mt-8 mb-3 text-base font-semibold text-fg">List Containers</h3>
        <div class="mb-4 rounded-lg border border-line bg-surface-2 p-4">
          <pre class="overflow-x-auto font-mono text-[13px] leading-relaxed text-fg"><code>sigardapanel docker ps --server 1

# Output:
# ID            NAME          IMAGE          STATE      PORTS
# 2b7b7b89b277  test-nginx    nginx:alpine   running    0.0.0.0:8888->80/tcp</code></pre>
        </div>

        <h3 class="mt-8 mb-3 text-base font-semibold text-fg">Container Actions</h3>
        <div class="mb-4 rounded-lg border border-line bg-surface-2 p-4">
          <pre class="overflow-x-auto font-mono text-[13px] leading-relaxed text-fg"><code># Start a container
sigardapanel docker start --server 1 --id abc123

# Stop a container
sigardapanel docker stop --server 1 --id abc123

# Restart a container
sigardapanel docker restart --server 1 --id abc123

# View logs
sigardapanel docker logs --server 1 --id abc123 --lines 50</code></pre>
        </div>

        <h3 class="mt-8 mb-3 text-base font-semibold text-fg">Pull Image</h3>
        <div class="mb-4 rounded-lg border border-line bg-surface-2 p-4">
          <pre class="overflow-x-auto font-mono text-[13px] leading-relaxed text-fg"><code>sigardapanel docker images --server 1

# Output:
# REPOSITORY   TAG     SIZE      CREATED
# nginx        alpine  93.3MB    2026-06-22</code></pre>
        </div>
      </section>

      <!-- Firewall -->
      <section id="firewall" class="scroll-mt-20 border-b border-line pb-12 mb-12">
        <h2 class="text-2xl font-semibold tracking-tight text-fg">Firewall Management</h2>
        <p class="mt-1.5 mb-2 text-muted">UFW firewall management with presets and quick actions.</p>

        <h3 class="mt-8 mb-3 text-base font-semibold text-fg">Check Status</h3>
        <div class="mb-4 rounded-lg border border-line bg-surface-2 p-4">
          <pre class="overflow-x-auto font-mono text-[13px] leading-relaxed text-fg"><code>sigardapanel firewall status --server 1

# Output:
# 🟢 Firewall: ACTIVE
# 
# Status: active
# 
# To                         Action      From
# --                         ------      ----
# 22/tcp                     ALLOW       Anywhere
# 80/tcp                     ALLOW       Anywhere
# 443/tcp                    ALLOW       Anywhere</code></pre>
        </div>

        <h3 class="mt-8 mb-3 text-base font-semibold text-fg">Allow Port</h3>
        <div class="mb-4 rounded-lg border border-line bg-surface-2 p-4">
          <pre class="overflow-x-auto font-mono text-[13px] leading-relaxed text-fg"><code># Allow HTTPS
sigardapanel firewall allow --server 1 --port 443 --protocol tcp

# Allow from specific IP
sigardapanel firewall allow --server 1 --port 3306 --protocol tcp --from 10.0.0.0/8</code></pre>
        </div>

        <h3 class="mt-8 mb-3 text-base font-semibold text-fg">Block IP</h3>
        <div class="mb-4 rounded-lg border border-line bg-surface-2 p-4">
          <pre class="overflow-x-auto font-mono text-[13px] leading-relaxed text-fg"><code>sigardapanel firewall deny --server 1 --port any --from 1.2.3.4</code></pre>
        </div>
      </section>

      <!-- Redis -->
      <section id="redis" class="scroll-mt-20 border-b border-line pb-12 mb-12">
        <h2 class="text-2xl font-semibold tracking-tight text-fg">Redis Management</h2>
        <p class="mt-1.5 mb-2 text-muted">Monitor and manage Redis instances.</p>

        <h3 class="mt-8 mb-3 text-base font-semibold text-fg">View Stats</h3>
        <div class="mb-4 rounded-lg border border-line bg-surface-2 p-4">
          <pre class="overflow-x-auto font-mono text-[13px] leading-relaxed text-fg"><code>sigardapanel redis stats --server 1

# Output:
# Redis Statistics:
# ─────────────────────────────────────
#   connected_clients              1
#   used_memory                    856.2K
#   uptime_in_seconds              86400</code></pre>
        </div>

        <h3 class="mt-8 mb-3 text-base font-semibold text-fg">Flush Data</h3>
        <div class="mb-4 rounded-lg border border-line bg-surface-2 p-4">
          <pre class="overflow-x-auto font-mono text-[13px] leading-relaxed text-fg"><code># Flush specific database
sigardapanel redis flushdb --server 1 --db 0

# Flush ALL data (with confirmation)
sigardapanel redis flush --server 1</code></pre>
        </div>
      </section>

      <!-- Git Deploy -->
      <section id="git" class="scroll-mt-20 border-b border-line pb-12 mb-12">
        <h2 class="text-2xl font-semibold tracking-tight text-fg">Git Deploy</h2>
        <p class="mt-1.5 mb-2 text-muted">Deploy from Git with branch management and rollback.</p>

        <h3 class="mt-8 mb-3 text-base font-semibold text-fg">View Branches</h3>
        <div class="mb-4 rounded-lg border border-line bg-surface-2 p-4">
          <pre class="overflow-x-auto font-mono text-[13px] leading-relaxed text-fg"><code>sigardapanel git branches --site 1

# Output:
# Current branch: main
# 
#   main
#   develop
#   feature/new-api</code></pre>
        </div>

        <h3 class="mt-8 mb-3 text-base font-semibold text-fg">View Commit History</h3>
        <div class="mb-4 rounded-lg border border-line bg-surface-2 p-4">
          <pre class="overflow-x-auto font-mono text-[13px] leading-relaxed text-fg"><code>sigardapanel git log --site 1 --lines 5

# Output:
# COMMIT   MESSAGE                  AUTHOR      DATE
# abc1234  fix: update dependencies bayur        2026-07-12
# def5678  feat: add new feature    bayur        2026-07-11
# ghi9012  fix: bug fix             bayur        2026-07-10</code></pre>
        </div>

        <h3 class="mt-8 mb-3 text-base font-semibold text-fg">Rollback</h3>
        <div class="mb-4 rounded-lg border border-line bg-surface-2 p-4">
          <pre class="overflow-x-auto font-mono text-[13px] leading-relaxed text-fg"><code>sigardapanel git rollback --site 1 --commit abc123

# Output:
# rolled back to abc123
#   backup branch: backup-before-rollback-abc123</code></pre>
        </div>
      </section>

      <!-- CLI Reference -->
      <section id="cli" class="scroll-mt-20 border-b border-line pb-12 mb-12">
        <h2 class="text-2xl font-semibold tracking-tight text-fg">CLI Reference</h2>
        <p class="mt-1.5 mb-2 text-muted">55+ commands for complete server management from terminal.</p>

        <!-- Core -->
        <h3 class="mt-8 mb-3 text-base font-semibold text-fg">Core Commands</h3>
        <div class="space-y-2 mb-8">
          {#each [
            { cmd: 'sigardapanel api', desc: 'Start API server on :7700', example: '$ sigardapanel api' },
            { cmd: 'sigardapanel agent', desc: 'Start agent on :7710', example: '$ SIGARDAPANEL_AGENT_TOKEN=xxx sigardapanel agent' },
            { cmd: 'sigardapanel dev', desc: 'Run API + agent together', example: '$ sigardapanel dev' },
            { cmd: 'sigardapanel install', desc: 'Setup wizard (interactive)', example: '$ sigardapanel install' },
            { cmd: 'sigardapanel init', desc: 'Create admin user directly', example: '$ sigardapanel init --user admin --password mypass' },
            { cmd: 'sigardapanel login', desc: 'Authenticate CLI', example: '$ sigardapanel login --api-url http://panel:7700' },
            { cmd: 'sigardapanel version', desc: 'Print version', example: '$ sigardapanel version\n0.5.0' },
          ] as item (item.cmd)}
            <div class="rounded-lg border border-line bg-surface p-3">
              <div class="flex items-center gap-2">
                <code class="font-mono text-[13px] font-medium break-all text-fg">{item.cmd}</code>
                <span class="text-xs text-muted">— {item.desc}</span>
              </div>
              <pre class="mt-2 overflow-x-auto rounded-md bg-surface-2 p-2.5 font-mono text-xs leading-relaxed text-muted"><code>{item.example}</code></pre>
            </div>
          {/each}
        </div>

        <!-- Server -->
        <h3 class="mt-8 mb-3 text-base font-semibold text-fg">Server Management</h3>
        <div class="space-y-2 mb-8">
          {#each [
            { cmd: 'server add --name NAME --hostname HOST [--ip IP]', desc: 'Register a new server', example: '$ sigardapanel server add --name "Production" --hostname prod.example.com --ip 1.2.3.4\nserver created (id=1)\nname: Production\nhostname: prod.example.com\nagent_token: abc123...\n\nsave this token — it won\'t be shown again' },
            { cmd: 'server list [--output json]', desc: 'List all servers', example: '$ sigardapanel server list\nID  NAME       HOSTNAME     STATUS\n1   My Server  prod.example.com  active' },
            { cmd: 'server update --id ID [--name N] [--hostname H] [--status S]', desc: 'Update server settings', example: '$ sigardapanel server update --id 1 --name "Prod Server" --status active\nserver updated (id=1)\n  name: Prod Server' },
            { cmd: 'server remove <id>', desc: 'Remove a server', example: '$ sigardapanel server remove 1\nserver 1 deleted' },
            { cmd: 'server doctor', desc: 'Health check', example: '$ sigardapanel server doctor\npanel: ok (url: http://localhost:7700)' },
          ] as item (item.cmd)}
            <div class="rounded-lg border border-line bg-surface p-3">
              <code class="font-mono text-[13px] font-medium break-all text-fg">sigardapanel {item.cmd}</code>
              <p class="mt-1 text-xs text-muted">{item.desc}</p>
              <pre class="mt-2 overflow-x-auto rounded-md bg-surface-2 p-2.5 font-mono text-xs leading-relaxed text-muted"><code>{item.example}</code></pre>
            </div>
          {/each}
        </div>

        <!-- Site -->
        <h3 class="mt-8 mb-3 text-base font-semibold text-fg">Site Management</h3>
        <div class="space-y-2 mb-8">
          {#each [
            { cmd: 'site create --server ID --name X --domain Y [--runtime R]', desc: 'Create a new site', example: '$ sigardapanel site create --server 1 --name "My Blog" --domain blog.example.com --runtime node\nsite created (id=1, domain=blog.example.com)\njob_id: 5' },
            { cmd: 'site list [--server ID] [--output json]', desc: 'List all sites', example: '$ sigardapanel site list\nID  NAME       DOMAIN              RUNTIME  STATUS\n1   My Blog    blog.example.com    node     active\n2   API Server api.example.com     go       active' },
            { cmd: 'site update --id ID [--name N] [--domain D] [--runtime R]', desc: 'Update site settings', example: '$ sigardapanel site update --id 1 --runtime php\nsite updated (id=1)' },
            { cmd: 'site delete <id>', desc: 'Delete a site', example: '$ sigardapanel site delete 1\nsite 1 deleted (cleanup job queued)' },
            { cmd: 'site deploy <id>', desc: 'Deploy site from git', example: '$ sigardapanel site deploy 1\ndeploy job queued (job_id=10)' },
            { cmd: 'site config --id ID --git-repo URL [--git-branch B]', desc: 'Update git config', example: '$ sigardapanel site config --id 1 --git-repo https://github.com/user/repo\nwebhook_secret: abc123...\nconfig updated' },
            { cmd: 'site setup-logrotate --id ID', desc: 'Setup log rotation', example: '$ sigardapanel site setup-logrotate --id 1\nlogrotate config created for blog.example.com' },
          ] as item (item.cmd)}
            <div class="rounded-lg border border-line bg-surface p-3">
              <code class="font-mono text-[13px] font-medium break-all text-fg">sigardapanel {item.cmd}</code>
              <p class="mt-1 text-xs text-muted">{item.desc}</p>
              <pre class="mt-2 overflow-x-auto rounded-md bg-surface-2 p-2.5 font-mono text-xs leading-relaxed text-muted"><code>{item.example}</code></pre>
            </div>
          {/each}
        </div>

        <!-- SSL -->
        <h3 class="mt-8 mb-3 text-base font-semibold text-fg">SSL Management</h3>
        <div class="space-y-2 mb-8">
          {#each [
            { cmd: 'ssl status --site ID', desc: 'Show SSL status', example: '$ sigardapanel ssl status --site 1\n{\n  "site_id": 1,\n  "domain": "blog.example.com",\n  "ssl_status": "active",\n  "ssl_issuer": "Let\'s Encrypt",\n  "ssl_expires_at": "2026-10-10T00:00:00Z"\n}' },
            { cmd: 'ssl issue --site ID --email E', desc: 'Issue Let\'s Encrypt cert', example: '$ sigardapanel ssl issue --site 1 --email admin@example.com\nssl issue job queued (job_id=15)' },
            { cmd: 'ssl renew --site ID', desc: 'Renew certificate', example: '$ sigardapanel ssl renew --site 1\nssl renew job queued (job_id=16)' },
            { cmd: 'ssl renew-all [--dry-run]', desc: 'Renew all expiring certs', example: '$ sigardapanel ssl renew-all --dry-run\nSSL certificates expiring within 30 days (2):\n\nID  DOMAIN              EXPIRES\n1   blog.example.com    2026-07-25 (13 days left)\n2   api.example.com     2026-07-20 (8 days left)\n\nDry run — no jobs queued.' },
          ] as item (item.cmd)}
            <div class="rounded-lg border border-line bg-surface p-3">
              <code class="font-mono text-[13px] font-medium break-all text-fg">sigardapanel {item.cmd}</code>
              <p class="mt-1 text-xs text-muted">{item.desc}</p>
              <pre class="mt-2 overflow-x-auto rounded-md bg-surface-2 p-2.5 font-mono text-xs leading-relaxed text-muted"><code>{item.example}</code></pre>
            </div>
          {/each}
        </div>

        <!-- Database -->
        <h3 class="mt-8 mb-3 text-base font-semibold text-fg">Database Management</h3>
        <div class="space-y-2 mb-8">
          {#each [
            { cmd: 'db create --site ID --name X [--type mysql|postgres]', desc: 'Create database', example: '$ sigardapanel db create --site 1 --name mydb --type mysql\ndatabase queued (id=1, job_id=20)' },
            { cmd: 'db list --site ID', desc: 'List databases', example: '$ sigardapanel db list --site 1\nID  NAME    TYPE    CHARSET  SIZE\n1   mydb    mysql   utf8mb4  0' },
            { cmd: 'db update --id ID [--name N] [--charset C]', desc: 'Update database', example: '$ sigardapanel db update --id 1 --charset utf8mb4\ndatabase updated (id=1)' },
            { cmd: 'db delete <id>', desc: 'Delete database', example: '$ sigardapanel db delete 1\ndatabase 1 deleted (cleanup job queued)' },
            { cmd: 'db user create --database ID --username U [--password P]', desc: 'Create DB user', example: '$ sigardapanel db user create --database 1 --username app_user\ndb user queued (user_id=1, username=app_user, job_id=25)' },
            { cmd: 'db user rotate-password --database DB_ID --user USER_ID', desc: 'Rotate password', example: '$ sigardapanel db user rotate-password --database 1 --user 2\npassword rotated for user 2 on database 1' },
          ] as item (item.cmd)}
            <div class="rounded-lg border border-line bg-surface p-3">
              <code class="font-mono text-[13px] font-medium break-all text-fg">sigardapanel {item.cmd}</code>
              <p class="mt-1 text-xs text-muted">{item.desc}</p>
              <pre class="mt-2 overflow-x-auto rounded-md bg-surface-2 p-2.5 font-mono text-xs leading-relaxed text-muted"><code>{item.example}</code></pre>
            </div>
          {/each}
        </div>

        <!-- Backup -->
        <h3 class="mt-8 mb-3 text-base font-semibold text-fg">Backup Management</h3>
        <div class="space-y-2 mb-8">
          {#each [
            { cmd: 'backup create --site ID', desc: 'Create a new backup', example: '$ sigardapanel backup create --site 1\nbackup queued (backup_id=5, job_id=30)' },
            { cmd: 'backup list [--site ID]', desc: 'List backups', example: '$ sigardapanel backup list --site 1\nID  SITE_ID  STATUS    CREATED\n5   1        success   2026-07-12T10:00:00Z' },
            { cmd: 'backup delete <id>', desc: 'Delete a backup', example: '$ sigardapanel backup delete 5\nbackup 5 deleted' },
            { cmd: 'backup restore <id>', desc: 'Restore from backup', example: '$ sigardapanel backup restore 5\nrestore job queued (backup_id=5, job_id=35)' },
          ] as item (item.cmd)}
            <div class="rounded-lg border border-line bg-surface p-3">
              <code class="font-mono text-[13px] font-medium break-all text-fg">sigardapanel {item.cmd}</code>
              <p class="mt-1 text-xs text-muted">{item.desc}</p>
              <pre class="mt-2 overflow-x-auto rounded-md bg-surface-2 p-2.5 font-mono text-xs leading-relaxed text-muted"><code>{item.example}</code></pre>
            </div>
          {/each}
        </div>

        <!-- Docker -->
        <h3 class="mt-8 mb-3 text-base font-semibold text-fg">Docker Management</h3>
        <div class="space-y-2 mb-8">
          {#each [
            { cmd: 'docker ps --server ID', desc: 'List containers', example: '$ sigardapanel docker ps --server 1\nID            NAME          IMAGE          STATE      PORTS\n2b7b7b89b277  test-nginx    nginx:alpine   running    0.0.0.0:8888->80/tcp' },
            { cmd: 'docker start --server ID --id CID', desc: 'Start container', example: '$ sigardapanel docker start --server 1 --id abc123\ncontainer abc123 started' },
            { cmd: 'docker stop --server ID --id CID', desc: 'Stop container', example: '$ sigardapanel docker stop --server 1 --id abc123\ncontainer abc123 stopped' },
            { cmd: 'docker logs --server ID --id CID [--lines N]', desc: 'View logs', example: '$ sigardapanel docker logs --server 1 --id abc123 --lines 10\n[nginx] 172.17.0.1 - - "GET / HTTP/1.1" 200 612 "-" "curl/7.88.1"' },
            { cmd: 'docker images --server ID', desc: 'List images', example: '$ sigardapanel docker images --server 1\nREPOSITORY   TAG     SIZE      CREATED\nnginx        alpine  93.3MB    2026-06-22' },
            { cmd: 'docker compose up --server ID [--path PATH]', desc: 'Start compose', example: '$ sigardapanel docker compose up --server 1 --path /app/docker-compose.yml\ncompose up completed' },
          ] as item (item.cmd)}
            <div class="rounded-lg border border-line bg-surface p-3">
              <code class="font-mono text-[13px] font-medium break-all text-fg">sigardapanel {item.cmd}</code>
              <p class="mt-1 text-xs text-muted">{item.desc}</p>
              <pre class="mt-2 overflow-x-auto rounded-md bg-surface-2 p-2.5 font-mono text-xs leading-relaxed text-muted"><code>{item.example}</code></pre>
            </div>
          {/each}
        </div>

        <!-- Firewall -->
        <h3 class="mt-8 mb-3 text-base font-semibold text-fg">Firewall Management</h3>
        <div class="space-y-2 mb-8">
          {#each [
            { cmd: 'firewall status --server ID', desc: 'Show firewall status', example: '$ sigardapanel firewall status --server 1\n🟢 Firewall: ACTIVE\n\nStatus: active\n\nTo                         Action      From\n--                         ------      ----\n22/tcp                     ALLOW       Anywhere\n80/tcp                     ALLOW       Anywhere\n443/tcp                    ALLOW       Anywhere' },
            { cmd: 'firewall allow --server ID --port PORT [--protocol PROTO]', desc: 'Allow port', example: '$ sigardapanel firewall allow --server 1 --port 443 --protocol tcp\nport 443 allowed' },
            { cmd: 'firewall deny --server ID --port PORT [--from IP]', desc: 'Deny port', example: '$ sigardapanel firewall deny --server 1 --port 3306 --from 10.0.0.0/8\nport 3306 denied' },
            { cmd: 'firewall rules --server ID', desc: 'List rules', example: '$ sigardapanel firewall rules --server 1\n#  ACTION  FROM        TO\n1  ALLOW   Anywhere    22/tcp\n2  ALLOW   Anywhere    80/tcp\n3  ALLOW   Anywhere    443/tcp' },
            { cmd: 'firewall enable --server ID', desc: 'Enable firewall', example: '$ sigardapanel firewall enable --server 1\nfirewall enabled' },
            { cmd: 'firewall disable --server ID', desc: 'Disable firewall', example: '$ sigardapanel firewall disable --server 1\nfirewall disabled' },
          ] as item (item.cmd)}
            <div class="rounded-lg border border-line bg-surface p-3">
              <code class="font-mono text-[13px] font-medium break-all text-fg">sigardapanel {item.cmd}</code>
              <p class="mt-1 text-xs text-muted">{item.desc}</p>
              <pre class="mt-2 overflow-x-auto rounded-md bg-surface-2 p-2.5 font-mono text-xs leading-relaxed text-muted"><code>{item.example}</code></pre>
            </div>
          {/each}
        </div>

        <!-- Redis -->
        <h3 class="mt-8 mb-3 text-base font-semibold text-fg">Redis Management</h3>
        <div class="space-y-2 mb-8">
          {#each [
            { cmd: 'redis stats --server ID', desc: 'Show stats', example: '$ sigardapanel redis stats --server 1\nRedis Statistics:\n─────────────────────────────────────\n  connected_clients              1\n  used_memory                    856.2K\n  uptime_in_seconds              86400' },
            { cmd: 'redis info --server ID', desc: 'Show detailed info', example: '$ sigardapanel redis info --server 1\n# Server\nredis_version:7.0.12\nuptime_in_seconds:86400\n# Memory\nused_memory_human:856.2K' },
            { cmd: 'redis flush --server ID', desc: 'Flush ALL data', example: '$ sigardapanel redis flush --server 1\nFlush ALL Redis data? [y/N]: y\nRedis data flushed' },
            { cmd: 'redis flushdb --server ID --db N', desc: 'Flush specific db', example: '$ sigardapanel redis flushdb --server 1 --db 0\nRedis database 0 flushed' },
          ] as item (item.cmd)}
            <div class="rounded-lg border border-line bg-surface p-3">
              <code class="font-mono text-[13px] font-medium break-all text-fg">sigardapanel {item.cmd}</code>
              <p class="mt-1 text-xs text-muted">{item.desc}</p>
              <pre class="mt-2 overflow-x-auto rounded-md bg-surface-2 p-2.5 font-mono text-xs leading-relaxed text-muted"><code>{item.example}</code></pre>
            </div>
          {/each}
        </div>

        <!-- System -->
        <h3 class="mt-8 mb-3 text-base font-semibold text-fg">System & User Management</h3>
        <div class="space-y-2 mb-8">
          {#each [
            { cmd: 'system disk-usage --server ID', desc: 'Check disk usage', example: '$ sigardapanel system disk-usage --server 1\nServer Disk Usage: ✅\n\n  Disk:  45.2 GB / 100.0 GB (45.2% used, 54.8 GB free)\n  Mem:   3.2 GB / 8.0 GB\n  CPU:   12.5%' },
            { cmd: 'system updates --server ID', desc: 'Check updates', example: '$ sigardapanel system updates --server 1\nAvailable updates:\n\nnginx/stable 1.24.0 amd64\npostgresql-16/stable 16.3 amd64' },
            { cmd: 'user create [--username U] [--email E] [--password P] [--role R]', desc: 'Create user', example: '$ sigardapanel user create --username john --email john@example.com --role admin\nuser created (id=4)\nusername: john\nemail: john@example.com\nrole: admin' },
            { cmd: 'user list', desc: 'List users', example: '$ sigardapanel user list\nID  USERNAME    EMAIL                     ROLE         CREATED\n1   admin       admin@localhost           admin        2026-07-10\n2   hanyawahyu  hanyawahyusaja@gmail.com  super_admin  2026-07-12\ntotal: 2' },
            { cmd: 'user reset-password <id>', desc: 'Reset password', example: '$ sigardapanel user reset-password 1 --password newpass\npassword updated for user 1' },
          ] as item (item.cmd)}
            <div class="rounded-lg border border-line bg-surface p-3">
              <code class="font-mono text-[13px] font-medium break-all text-fg">sigardapanel {item.cmd}</code>
              <p class="mt-1 text-xs text-muted">{item.desc}</p>
              <pre class="mt-2 overflow-x-auto rounded-md bg-surface-2 p-2.5 font-mono text-xs leading-relaxed text-muted"><code>{item.example}</code></pre>
            </div>
          {/each}
        </div>

        <!-- Env Vars -->
        <h3 class="mt-8 mb-3 text-base font-semibold text-fg">Environment Variables</h3>
        <div class="card table-wrap">
          <table class="table">
            <thead>
              <tr>
                <th>Variable</th>
                <th>Default</th>
                <th>Description</th>
              </tr>
            </thead>
            <tbody>
              {#each [
                { var: 'SIGARDAPANEL_API_ADDR', def: ':7700', desc: 'API listen address' },
                { var: 'SIGARDAPANEL_AGENT_ADDR', def: ':7710', desc: 'Agent listen address' },
                { var: 'SIGARDAPANEL_DB_PATH', def: 'sigardapanel.db', desc: 'Database path' },
                { var: 'SIGARDAPANEL_API_URL', def: '-', desc: 'API base URL for CLI' },
                { var: 'SIGARDAPANEL_TOKEN', def: '-', desc: 'API token from login' },
                { var: 'SIGARDAPANEL_OUTPUT', def: 'table', desc: 'Output format (table|json|yaml)' },
              ] as item (item.var)}
                <tr>
                  <td class="font-mono text-[13px] whitespace-nowrap">{item.var}</td>
                  <td class="font-mono text-[13px] text-subtle">{item.def}</td>
                  <td class="text-muted">{item.desc}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      </section>

      <!-- API Reference -->
      <section id="api" class="scroll-mt-20 border-b border-line pb-12 mb-12">
        <h2 class="text-2xl font-semibold tracking-tight text-fg">API Reference</h2>
        <p class="mt-1.5 mb-2 text-muted">REST API for integrations and automation.</p>

        <h3 class="mt-8 mb-3 text-base font-semibold text-fg">Authentication</h3>
        <div class="mb-6 rounded-lg border border-line bg-surface-2 p-4">
          <pre class="overflow-x-auto font-mono text-[13px] leading-relaxed text-fg"><code>Authorization: Bearer YOUR_TOKEN_HERE</code></pre>
        </div>

        <h3 class="mt-8 mb-3 text-base font-semibold text-fg">Response Format</h3>
        <div class="mb-6 rounded-lg border border-line bg-surface-2 p-4">
          <pre class="overflow-x-auto font-mono text-[13px] leading-relaxed text-fg"><code>&#123;
  "data": &#123; ... &#125;,
  "request_id": "req_xxx"
{'}'}</code></pre>
        </div>

        <h3 class="mt-8 mb-3 text-base font-semibold text-fg">Example: Login</h3>
        <div class="mb-6 rounded-lg border border-line bg-surface-2 p-4">
          <pre class="overflow-x-auto font-mono text-[13px] leading-relaxed text-fg"><code>curl -X POST http://localhost:7700/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '&#123;"username":"admin","password":"mypassword"&#125;'

# Response:
&#123;
  "data": &#123;
    "token": "abc123...",
    "user": &#123;
      "id": 1,
      "username": "admin",
      "role": "admin"
    &#125;
  &#125;,
  "request_id": "req_xxx"
{'}'}</code></pre>
        </div>

        <h3 class="mt-8 mb-3 text-base font-semibold text-fg">Endpoints</h3>
        <div class="space-y-2">
          {#each [
            { method: 'POST', path: '/api/v1/auth/login', desc: 'Login' },
            { method: 'GET', path: '/api/v1/servers', desc: 'List servers' },
            { method: 'POST', path: '/api/v1/servers', desc: 'Create server' },
            { method: 'GET', path: '/api/v1/sites', desc: 'List sites' },
            { method: 'POST', path: '/api/v1/sites', desc: 'Create site' },
            { method: 'GET', path: '/api/v1/databases', desc: 'List databases' },
            { method: 'POST', path: '/api/v1/databases', desc: 'Create database' },
            { method: 'GET', path: '/api/v1/backups', desc: 'List backups' },
            { method: 'POST', path: '/api/v1/backups/:id/restore', desc: 'Restore backup' },
            { method: 'GET', path: '/api/v1/docker/containers', desc: 'List containers' },
            { method: 'GET', path: '/api/v1/firewall/rules', desc: 'List firewall rules' },
          ] as ep (ep.method + ep.path)}
            <div class="flex items-center gap-3 rounded-lg border border-line bg-surface px-3 py-2.5">
              <span class="badge w-14 shrink-0 justify-center font-mono {ep.method === 'GET' ? 'badge-success' : ep.method === 'POST' ? 'badge-info' : ep.method === 'PATCH' ? 'badge-warning' : 'badge-danger'}">{ep.method}</span>
              <code class="min-w-0 truncate font-mono text-[13px] text-fg">{ep.path}</code>
              <span class="ml-auto shrink-0 text-xs text-subtle max-sm:hidden">{ep.desc}</span>
            </div>
          {/each}
        </div>
      </section>

      <!-- Troubleshooting -->
      <section id="troubleshooting" class="scroll-mt-20 border-b border-line pb-12 mb-12">
        <h2 class="text-2xl font-semibold tracking-tight text-fg">Troubleshooting</h2>
        <p class="mt-1.5 mb-2 text-muted">Common issues and solutions.</p>

        <div class="space-y-4">
          <div class="card p-5">
            <h3 class="mb-2 text-sm font-semibold text-fg">Agent shows "Offline"</h3>
            <ul class="space-y-1 text-sm leading-relaxed text-muted">
              <li>1. Check agent is running: <code class="code">systemctl status sigardapanel-agent</code></li>
              <li>2. Check port 7710 is open: <code class="code">ss -tlnp | grep 7710</code></li>
              <li>3. Check agent token matches panel config</li>
              <li>4. Check network connectivity from panel to agent</li>
            </ul>
          </div>

          <div class="card p-5">
            <h3 class="mb-2 text-sm font-semibold text-fg">SSL certificate issue fails</h3>
            <ul class="space-y-1 text-sm leading-relaxed text-muted">
              <li>1. Ensure domain points to this server's IP</li>
              <li>2. Check port 80 is accessible (Let's Encrypt HTTP challenge)</li>
              <li>3. Verify nginx is running: <code class="code">systemctl status nginx</code></li>
              <li>4. Check certbot is installed: <code class="code">certbot --version</code></li>
            </ul>
          </div>

          <div class="card p-5">
            <h3 class="mb-2 text-sm font-semibold text-fg">Docker commands fail</h3>
            <ul class="space-y-1 text-sm leading-relaxed text-muted">
              <li>1. Check Docker is installed: <code class="code">docker --version</code></li>
              <li>2. Check Docker is running: <code class="code">systemctl status docker</code></li>
              <li>3. Check user permissions: <code class="code">usermod -aG docker $USER</code></li>
            </ul>
          </div>

          <div class="card p-5">
            <h3 class="mb-2 text-sm font-semibold text-fg">Firewall blocks access</h3>
            <ul class="space-y-1 text-sm leading-relaxed text-muted">
              <li>1. Check firewall status: <code class="code">sigardapanel firewall status --server 1</code></li>
              <li>2. Allow SSH: <code class="code">sigardapanel firewall allow --server 1 --port 22 --protocol tcp</code></li>
              <li>3. Allow HTTP/HTTPS: <code class="code">sigardapanel firewall allow --server 1 --port 80,443 --protocol tcp</code></li>
            </ul>
          </div>
        </div>
      </section>

      <footer class="pt-2 text-center text-sm text-subtle">
        <p>SigardaPanel Documentation v0.5.3</p>
        <p class="mt-1">Built by <a href="https://github.com/bayurstarcool" class="link">bayurstarcool</a></p>
      </footer>
      </div>
    </main>
  </div>
</div>
