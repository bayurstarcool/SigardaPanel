<script lang="ts">
  import { onMount } from 'svelte';

  let activeSection = $state('getting-started');
  let searchQuery = $state('');
  let sidebarOpen = $state(false);

  const sections = [
    { id: 'getting-started', title: 'Getting Started', icon: '🚀' },
    { id: 'servers', title: 'Server Management', icon: '🖥️' },
    { id: 'sites', title: 'Site Management', icon: '🌐' },
    { id: 'databases', title: 'Databases', icon: '🗄️' },
    { id: 'backups', title: 'Backup System', icon: '💾' },
    { id: 'monitoring', title: 'Monitoring & Logs', icon: '📊' },
    { id: 'security', title: 'Security', icon: '🔒' },
    { id: 'cli', title: 'CLI Reference', icon: '⌨️' },
    { id: 'api', title: 'API Reference', icon: '🔌' },
    { id: 'docker', title: 'Docker', icon: '🐳' },
    { id: 'firewall', title: 'Firewall', icon: '🛡️' },
    { id: 'redis', title: 'Redis', icon: '⚡' },
    { id: 'git', title: 'Git Deploy', icon: '📦' },
    { id: 'troubleshooting', title: 'Troubleshooting', icon: '🔧' },
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

<div class="min-h-screen bg-[#060d1f] text-slate-200">
  <!-- Header -->
  <header class="sticky top-0 z-50 bg-[#0a1122]/90 backdrop-blur-xl border-b border-white/[0.06]">
    <div class="max-w-7xl mx-auto px-4 py-3 flex items-center justify-between">
      <div class="flex items-center gap-3">
        <button onclick={() => sidebarOpen = !sidebarOpen} class="lg:hidden p-2 rounded-lg hover:bg-white/[0.05]">
          <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 12h16M4 18h16"/></svg>
        </button>
        <a href="/" class="flex items-center gap-2">
          <div class="w-8 h-8 rounded-lg bg-gradient-to-br from-sky-400 to-indigo-500 flex items-center justify-center text-white text-xs font-bold">S</div>
          <span class="font-semibold text-white hidden sm:block">SigardaPanel</span>
          <span class="text-xs text-slate-500 hidden sm:block">Docs</span>
        </a>
      </div>
      <div class="flex items-center gap-2">
        <a href="/" class="px-3 py-1.5 text-xs text-slate-400 hover:text-white transition">Dashboard</a>
        <a href="https://github.com/bayurstarcool/SigardaPanel" target="_blank" class="px-3 py-1.5 text-xs text-slate-400 hover:text-white transition">GitHub</a>
      </div>
    </div>
  </header>

  <div class="max-w-7xl mx-auto flex">
    <!-- Sidebar -->
    <aside class="w-64 shrink-0 border-r border-white/[0.06] min-h-[calc(100vh-56px)] sticky top-14 hidden lg:block overflow-y-auto">
      <nav class="p-4 space-y-1">
        {#each sections as section}
          <button onclick={() => scrollTo(section.id)}
            class="w-full flex items-center gap-2 px-3 py-2 rounded-lg text-sm transition {activeSection === section.id ? 'bg-white/[0.08] text-white' : 'text-slate-400 hover:text-white hover:bg-white/[0.04]'}">
            <span>{section.icon}</span>
            <span>{section.title}</span>
          </button>
        {/each}
      </nav>
    </aside>

    <!-- Mobile sidebar -->
    {#if sidebarOpen}
      <div class="fixed inset-0 z-50 lg:hidden" onclick={() => sidebarOpen = false}>
        <div class="absolute inset-0 bg-black/50"></div>
        <aside class="absolute left-0 top-0 bottom-0 w-72 bg-[#0a1122] border-r border-white/[0.06] overflow-y-auto" onclick={(e) => e.stopPropagation()}>
          <div class="p-4">
            <div class="flex items-center justify-between mb-4">
              <span class="font-semibold text-white">Navigation</span>
              <button onclick={() => sidebarOpen = false} class="p-1 rounded hover:bg-white/[0.05]">
                <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"/></svg>
              </button>
            </div>
            <nav class="space-y-1">
              {#each sections as section}
                <button onclick={() => scrollTo(section.id)}
                  class="w-full flex items-center gap-2 px-3 py-2 rounded-lg text-sm transition {activeSection === section.id ? 'bg-white/[0.08] text-white' : 'text-slate-400 hover:text-white hover:bg-white/[0.04]'}">
                  <span>{section.icon}</span>
                  <span>{section.title}</span>
                </button>
              {/each}
            </nav>
          </div>
        </aside>
      </div>
    {/if}

    <!-- Main content -->
    <main class="flex-1 min-w-0 p-6 lg:p-10">
      <!-- Getting Started -->
      <section id="getting-started" class="mb-16">
        <h1 class="text-3xl font-bold text-white mb-2">Getting Started</h1>
        <p class="text-slate-400 mb-8">Set up SigardaPanel in minutes.</p>

        <h2 class="text-xl font-semibold text-white mb-4">What is SigardaPanel?</h2>
        <p class="text-slate-300 mb-6">SigardaPanel is a modern VPS control panel built with Go + SvelteKit. It manages multiple servers, sites, databases, backups, SSL certificates, Docker, firewall, and monitoring — all from one dashboard.</p>

        <h2 class="text-xl font-semibold text-white mb-4">System Requirements</h2>
        <ul class="space-y-2 text-slate-300 mb-6">
          <li>• <strong>Panel Server:</strong> Ubuntu 20.04+, 1GB RAM, 10GB disk</li>
          <li>• <strong>Target VPS:</strong> Any Linux with systemd</li>
          <li>• <strong>Network:</strong> Panel must reach agent on port 7710</li>
          <li>• <strong>Dependencies:</strong> Go 1.22+, Node.js 18+ (for dashboard)</li>
        </ul>

        <h2 class="text-xl font-semibold text-white mb-4">Installation</h2>
        <div class="bg-[#0d1526] rounded-xl border border-white/[0.08] p-4 mb-6">
          <pre class="text-sm font-mono text-slate-300 overflow-x-auto"><code># Download and install
curl -sSL https://github.com/bayurstarcool/SigardaPanel/releases/latest/download/sigardapanel -o sigardapanel
chmod +x sigardapanel

# Run setup wizard
./sigardapanel install

# Or start directly
./sigardapanel dev</code></pre>
        </div>

        <h2 class="text-xl font-semibold text-white mb-4">Access</h2>
        <div class="bg-[#0d1526] rounded-xl border border-white/[0.08] p-4">
          <div class="grid grid-cols-1 sm:grid-cols-3 gap-4 text-sm">
            <div>
              <div class="text-slate-500 mb-1">Web UI</div>
              <code class="text-sky-300">http://YOUR_IP:7720</code>
            </div>
            <div>
              <div class="text-slate-500 mb-1">API</div>
              <code class="text-sky-300">http://YOUR_IP:7700</code>
            </div>
            <div>
              <div class="text-slate-500 mb-1">Agent</div>
              <code class="text-sky-300">http://YOUR_IP:7710</code>
            </div>
          </div>
        </div>
      </section>

      <!-- Server Management -->
      <section id="servers" class="mb-16">
        <h1 class="text-3xl font-bold text-white mb-2">Server Management</h1>
        <p class="text-slate-400 mb-8">Add, monitor, and manage your VPS fleet.</p>

        <h2 class="text-xl font-semibold text-white mb-4">Adding a Server</h2>
        <div class="bg-[#0d1526] rounded-xl border border-white/[0.08] p-4 mb-4">
          <pre class="text-sm font-mono text-slate-300 overflow-x-auto"><code># Via CLI
sigardapanel server add --name "Production" --hostname prod.example.com --ip 1.2.3.4

# Output:
# server created (id=1)
# name: Production
# hostname: prod.example.com
# agent_token: abc123def456...
# 
# save this token — it won't be shown again</code></pre>
        </div>

        <h2 class="text-xl font-semibold text-white mb-4">List Servers</h2>
        <div class="bg-[#0d1526] rounded-xl border border-white/[0.08] p-4 mb-4">
          <pre class="text-sm font-mono text-slate-300 overflow-x-auto"><code>sigardapanel server list

# Output:
# ID  NAME       HOSTNAME     STATUS   
# 1   My Server  prod.example.com  active</code></pre>
        </div>

        <h2 class="text-xl font-semibold text-white mb-4">Update Server</h2>
        <div class="bg-[#0d1526] rounded-xl border border-white/[0.08] p-4 mb-4">
          <pre class="text-sm font-mono text-slate-300 overflow-x-auto"><code>sigardapanel server update --id 1 --name "Production Server" --status active

# Output:
# server updated (id=1)
#   name: Production Server
#   hostname: prod.example.com</code></pre>
        </div>

        <h2 class="text-xl font-semibold text-white mb-4">Health Monitoring</h2>
        <p class="text-slate-300 mb-4">Every 60 seconds, the panel pings each agent and checks response time.</p>
        <div class="flex flex-wrap gap-3">
          <span class="px-3 py-1 rounded-full text-xs bg-emerald-500/10 text-emerald-400">Online (&lt;1s)</span>
          <span class="px-3 py-1 rounded-full text-xs bg-amber-500/10 text-amber-400">Degraded (&gt;1s)</span>
          <span class="px-3 py-1 rounded-full text-xs bg-red-500/10 text-red-400">Offline</span>
        </div>
      </section>

      <!-- Site Management -->
      <section id="sites" class="mb-16">
        <h1 class="text-3xl font-bold text-white mb-2">Site Management</h1>
        <p class="text-slate-400 mb-8">Create and manage websites across your servers.</p>

        <h2 class="text-xl font-semibold text-white mb-4">Create a Site</h2>
        <div class="bg-[#0d1526] rounded-xl border border-white/[0.08] p-4 mb-4">
          <pre class="text-sm font-mono text-slate-300 overflow-x-auto"><code>sigardapanel site create --server 1 --name "My Blog" --domain blog.example.com --runtime node

# Output:
# site created (id=1, domain=blog.example.com)
# job_id: 5</code></pre>
        </div>

        <h2 class="text-xl font-semibold text-white mb-4">List Sites</h2>
        <div class="bg-[#0d1526] rounded-xl border border-white/[0.08] p-4 mb-4">
          <pre class="text-sm font-mono text-slate-300 overflow-x-auto"><code>sigardapanel site list

# Output:
# ID  NAME       DOMAIN              RUNTIME  STATUS
# 1   My Blog    blog.example.com    node     active
# 2   API Server api.example.com     go       active</code></pre>
        </div>

        <h2 class="text-xl font-semibold text-white mb-4">Supported Runtimes</h2>
        <div class="flex flex-wrap gap-2 mb-6">
          {#each ['Static', 'PHP (Multi-Version 7.4 - 8.5)', 'Node.js', 'Python', 'Go', 'Proxy', 'Docker'] as runtime}
            <span class="px-3 py-1 rounded-lg text-xs bg-white/[0.05] text-slate-300">{runtime}</span>
          {/each}
        </div>
      </section>

      <!-- Databases -->
      <section id="databases" class="mb-16">
        <h1 class="text-3xl font-bold text-white mb-2">Databases</h1>
        <p class="text-slate-400 mb-8">Manage MariaDB/MySQL & PostgreSQL databases, users, and integrated phpMyAdmin.</p>

        <h2 class="text-xl font-semibold text-white mb-4">phpMyAdmin Web Access</h2>
        <p class="text-sm text-slate-300 mb-3">SigardaPanel provides direct integrated phpMyAdmin with single-click authentication.</p>
        <div class="bg-[#0d1526] rounded-xl border border-white/[0.08] p-4 mb-4">
          <pre class="text-sm font-mono text-slate-300 overflow-x-auto"><code># Access via panel URL:
https://cp.sigarda.dev/pma/

# Or click "phpMyAdmin" on any database in the Databases dashboard for auto-login ticket.</code></pre>
        </div>
      </section>

      <!-- Backup System -->
      <section id="backups" class="mb-16">
        <h1 class="text-3xl font-bold text-white mb-2">Backup System</h1>
        <p class="text-slate-400 mb-8">Manual and scheduled backups with local storage and cloud provider support (S3, Cloudflare R2, Backblaze B2, Wasabi).</p>

        <h2 class="text-xl font-semibold text-white mb-4">Manual Backup</h2>
        <div class="bg-[#0d1526] rounded-xl border border-white/[0.08] p-4 mb-4">
          <pre class="text-sm font-mono text-slate-300 overflow-x-auto"><code># Trigger site backup via CLI:
sigardapanel backup create --site 1

# Or click "+ Create Backup" on the Backups page to select Full, Files-only, or DB-only.</code></pre>
        </div>

        <h2 class="text-xl font-semibold text-white mb-4">Auto-Retention & Cloud Cleanup</h2>
        <p class="text-sm text-slate-300 mb-3">Backup retention automatically keeps the latest 7 backups per site, removing expired archives from local disk and cloud S3 buckets simultaneously to prevent storage overflow.</p>
      </section>

      <!-- Security -->
      <section id="security" class="mb-16">
        <h1 class="text-3xl font-bold text-white mb-2">Security & DDoS Defense</h1>
        <p class="text-slate-400 mb-8">Hardened OS, Fail2ban intrusion detection, kernel SYN flood defense, and malware monitoring.</p>

        <h2 class="text-xl font-semibold text-white mb-4">Active Protections</h2>
        <ul class="list-disc pl-5 space-y-2 text-sm text-slate-300 mb-4">
          <li><strong>Firewall (UFW):</strong> Default-deny incoming policy, only ports 22, 80, 443 permitted.</li>
          <li><strong>Anti-DDoS Shield:</strong> Kernel SYN-cookies (<code>net.ipv4.tcp_syncookies=1</code>), Slowloris timeout mitigation (15s client timeouts), per-IP connection limits.</li>
          <li><strong>Fail2ban:</strong> Real-time monitoring and IP ban for SSH bruteforce and Nginx bot searches.</li>
          <li><strong>DB Isolation:</strong> Database ports (3306, 5432, 6379, 8081) bound strictly to <code>127.0.0.1</code> (localhost only).</li>
          <li><strong>Web Integrity:</strong> Automated detection of PHP webshell signatures in site webroots.</li>
        </ul>
      </section>

      <!-- Docker -->
      <section id="docker" class="mb-16">
        <h1 class="text-3xl font-bold text-white mb-2">Docker Management</h1>
        <p class="text-slate-400 mb-8">Full Docker container, image, volume, and network management.</p>

        <h2 class="text-xl font-semibold text-white mb-4">List Containers</h2>
        <div class="bg-[#0d1526] rounded-xl border border-white/[0.08] p-4 mb-4">
          <pre class="text-sm font-mono text-slate-300 overflow-x-auto"><code>sigardapanel docker ps --server 1

# Output:
# ID            NAME          IMAGE          STATE      PORTS
# 2b7b7b89b277  test-nginx    nginx:alpine   running    0.0.0.0:8888->80/tcp</code></pre>
        </div>

        <h2 class="text-xl font-semibold text-white mb-4">Container Actions</h2>
        <div class="bg-[#0d1526] rounded-xl border border-white/[0.08] p-4 mb-4">
          <pre class="text-sm font-mono text-slate-300 overflow-x-auto"><code># Start a container
sigardapanel docker start --server 1 --id abc123

# Stop a container
sigardapanel docker stop --server 1 --id abc123

# Restart a container
sigardapanel docker restart --server 1 --id abc123

# View logs
sigardapanel docker logs --server 1 --id abc123 --lines 50</code></pre>
        </div>

        <h2 class="text-xl font-semibold text-white mb-4">Pull Image</h2>
        <div class="bg-[#0d1526] rounded-xl border border-white/[0.08] p-4 mb-4">
          <pre class="text-sm font-mono text-slate-300 overflow-x-auto"><code>sigardapanel docker images --server 1

# Output:
# REPOSITORY   TAG     SIZE      CREATED
# nginx        alpine  93.3MB    2026-06-22</code></pre>
        </div>
      </section>

      <!-- Firewall -->
      <section id="firewall" class="mb-16">
        <h1 class="text-3xl font-bold text-white mb-2">Firewall Management</h1>
        <p class="text-slate-400 mb-8">UFW firewall management with presets and quick actions.</p>

        <h2 class="text-xl font-semibold text-white mb-4">Check Status</h2>
        <div class="bg-[#0d1526] rounded-xl border border-white/[0.08] p-4 mb-4">
          <pre class="text-sm font-mono text-slate-300 overflow-x-auto"><code>sigardapanel firewall status --server 1

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

        <h2 class="text-xl font-semibold text-white mb-4">Allow Port</h2>
        <div class="bg-[#0d1526] rounded-xl border border-white/[0.08] p-4 mb-4">
          <pre class="text-sm font-mono text-slate-300 overflow-x-auto"><code># Allow HTTPS
sigardapanel firewall allow --server 1 --port 443 --protocol tcp

# Allow from specific IP
sigardapanel firewall allow --server 1 --port 3306 --protocol tcp --from 10.0.0.0/8</code></pre>
        </div>

        <h2 class="text-xl font-semibold text-white mb-4">Block IP</h2>
        <div class="bg-[#0d1526] rounded-xl border border-white/[0.08] p-4 mb-4">
          <pre class="text-sm font-mono text-slate-300 overflow-x-auto"><code>sigardapanel firewall deny --server 1 --port any --from 1.2.3.4</code></pre>
        </div>
      </section>

      <!-- Redis -->
      <section id="redis" class="mb-16">
        <h1 class="text-3xl font-bold text-white mb-2">Redis Management</h1>
        <p class="text-slate-400 mb-8">Monitor and manage Redis instances.</p>

        <h2 class="text-xl font-semibold text-white mb-4">View Stats</h2>
        <div class="bg-[#0d1526] rounded-xl border border-white/[0.08] p-4 mb-4">
          <pre class="text-sm font-mono text-slate-300 overflow-x-auto"><code>sigardapanel redis stats --server 1

# Output:
# Redis Statistics:
# ─────────────────────────────────────
#   connected_clients              1
#   used_memory                    856.2K
#   uptime_in_seconds              86400</code></pre>
        </div>

        <h2 class="text-xl font-semibold text-white mb-4">Flush Data</h2>
        <div class="bg-[#0d1526] rounded-xl border border-white/[0.08] p-4 mb-4">
          <pre class="text-sm font-mono text-slate-300 overflow-x-auto"><code># Flush specific database
sigardapanel redis flushdb --server 1 --db 0

# Flush ALL data (with confirmation)
sigardapanel redis flush --server 1</code></pre>
        </div>
      </section>

      <!-- Git Deploy -->
      <section id="git" class="mb-16">
        <h1 class="text-3xl font-bold text-white mb-2">Git Deploy</h1>
        <p class="text-slate-400 mb-8">Deploy from Git with branch management and rollback.</p>

        <h2 class="text-xl font-semibold text-white mb-4">View Branches</h2>
        <div class="bg-[#0d1526] rounded-xl border border-white/[0.08] p-4 mb-4">
          <pre class="text-sm font-mono text-slate-300 overflow-x-auto"><code>sigardapanel git branches --site 1

# Output:
# Current branch: main
# 
#   main
#   develop
#   feature/new-api</code></pre>
        </div>

        <h2 class="text-xl font-semibold text-white mb-4">View Commit History</h2>
        <div class="bg-[#0d1526] rounded-xl border border-white/[0.08] p-4 mb-4">
          <pre class="text-sm font-mono text-slate-300 overflow-x-auto"><code>sigardapanel git log --site 1 --lines 5

# Output:
# COMMIT   MESSAGE                  AUTHOR      DATE
# abc1234  fix: update dependencies bayur        2026-07-12
# def5678  feat: add new feature    bayur        2026-07-11
# ghi9012  fix: bug fix             bayur        2026-07-10</code></pre>
        </div>

        <h2 class="text-xl font-semibold text-white mb-4">Rollback</h2>
        <div class="bg-[#0d1526] rounded-xl border border-white/[0.08] p-4 mb-4">
          <pre class="text-sm font-mono text-slate-300 overflow-x-auto"><code>sigardapanel git rollback --site 1 --commit abc123

# Output:
# rolled back to abc123
#   backup branch: backup-before-rollback-abc123</code></pre>
        </div>
      </section>

      <!-- CLI Reference -->
      <section id="cli" class="mb-16">
        <h1 class="text-3xl font-bold text-white mb-2">CLI Reference</h1>
        <p class="text-slate-400 mb-8">55+ commands for complete server management from terminal.</p>

        <!-- Core -->
        <h2 class="text-xl font-semibold text-white mb-4">Core Commands</h2>
        <div class="space-y-2 mb-8">
          {#each [
            { cmd: 'sigardapanel api', desc: 'Start API server on :7700', example: '$ sigardapanel api' },
            { cmd: 'sigardapanel agent', desc: 'Start agent on :7710', example: '$ SIGARDAPANEL_AGENT_TOKEN=xxx sigardapanel agent' },
            { cmd: 'sigardapanel dev', desc: 'Run API + agent together', example: '$ sigardapanel dev' },
            { cmd: 'sigardapanel install', desc: 'Setup wizard (interactive)', example: '$ sigardapanel install' },
            { cmd: 'sigardapanel init', desc: 'Create admin user directly', example: '$ sigardapanel init --user admin --password mypass' },
            { cmd: 'sigardapanel login', desc: 'Authenticate CLI', example: '$ sigardapanel login --api-url http://panel:7700' },
            { cmd: 'sigardapanel version', desc: 'Print version', example: '$ sigardapanel version\n0.5.0' },
          ] as item}
            <div class="bg-[#0d1526] rounded-lg border border-white/[0.08] p-3">
              <div class="flex items-center gap-2">
                <code class="text-sm font-mono text-sky-300">{item.cmd}</code>
                <span class="text-xs text-slate-500">— {item.desc}</span>
              </div>
              <pre class="text-xs font-mono text-emerald-400/70 mt-2 bg-black/20 rounded p-2 overflow-x-auto"><code>{item.example}</code></pre>
            </div>
          {/each}
        </div>

        <!-- Server -->
        <h2 class="text-xl font-semibold text-white mb-4">Server Management</h2>
        <div class="space-y-2 mb-8">
          {#each [
            { cmd: 'server add --name NAME --hostname HOST [--ip IP]', desc: 'Register a new server', example: '$ sigardapanel server add --name "Production" --hostname prod.example.com --ip 1.2.3.4\nserver created (id=1)\nname: Production\nhostname: prod.example.com\nagent_token: abc123...\n\nsave this token — it won\'t be shown again' },
            { cmd: 'server list [--output json]', desc: 'List all servers', example: '$ sigardapanel server list\nID  NAME       HOSTNAME     STATUS\n1   My Server  prod.example.com  active' },
            { cmd: 'server update --id ID [--name N] [--hostname H] [--status S]', desc: 'Update server settings', example: '$ sigardapanel server update --id 1 --name "Prod Server" --status active\nserver updated (id=1)\n  name: Prod Server' },
            { cmd: 'server remove <id>', desc: 'Remove a server', example: '$ sigardapanel server remove 1\nserver 1 deleted' },
            { cmd: 'server doctor', desc: 'Health check', example: '$ sigardapanel server doctor\npanel: ok (url: http://localhost:7700)' },
          ] as item}
            <div class="bg-[#0d1526] rounded-lg border border-white/[0.08] p-3">
              <code class="text-sm font-mono text-sky-300">sigardapanel {item.cmd}</code>
              <p class="text-xs text-slate-400 mt-1">{item.desc}</p>
              <pre class="text-xs font-mono text-emerald-400/70 mt-2 bg-black/20 rounded p-2 overflow-x-auto"><code>{item.example}</code></pre>
            </div>
          {/each}
        </div>

        <!-- Site -->
        <h2 class="text-xl font-semibold text-white mb-4">Site Management</h2>
        <div class="space-y-2 mb-8">
          {#each [
            { cmd: 'site create --server ID --name X --domain Y [--runtime R]', desc: 'Create a new site', example: '$ sigardapanel site create --server 1 --name "My Blog" --domain blog.example.com --runtime node\nsite created (id=1, domain=blog.example.com)\njob_id: 5' },
            { cmd: 'site list [--server ID] [--output json]', desc: 'List all sites', example: '$ sigardapanel site list\nID  NAME       DOMAIN              RUNTIME  STATUS\n1   My Blog    blog.example.com    node     active\n2   API Server api.example.com     go       active' },
            { cmd: 'site update --id ID [--name N] [--domain D] [--runtime R]', desc: 'Update site settings', example: '$ sigardapanel site update --id 1 --runtime php\nsite updated (id=1)' },
            { cmd: 'site delete <id>', desc: 'Delete a site', example: '$ sigardapanel site delete 1\nsite 1 deleted (cleanup job queued)' },
            { cmd: 'site deploy <id>', desc: 'Deploy site from git', example: '$ sigardapanel site deploy 1\ndeploy job queued (job_id=10)' },
            { cmd: 'site config --id ID --git-repo URL [--git-branch B]', desc: 'Update git config', example: '$ sigardapanel site config --id 1 --git-repo https://github.com/user/repo\nwebhook_secret: abc123...\nconfig updated' },
            { cmd: 'site setup-logrotate --id ID', desc: 'Setup log rotation', example: '$ sigardapanel site setup-logrotate --id 1\nlogrotate config created for blog.example.com' },
          ] as item}
            <div class="bg-[#0d1526] rounded-lg border border-white/[0.08] p-3">
              <code class="text-sm font-mono text-sky-300">sigardapanel {item.cmd}</code>
              <p class="text-xs text-slate-400 mt-1">{item.desc}</p>
              <pre class="text-xs font-mono text-emerald-400/70 mt-2 bg-black/20 rounded p-2 overflow-x-auto"><code>{item.example}</code></pre>
            </div>
          {/each}
        </div>

        <!-- SSL -->
        <h2 class="text-xl font-semibold text-white mb-4">SSL Management</h2>
        <div class="space-y-2 mb-8">
          {#each [
            { cmd: 'ssl status --site ID', desc: 'Show SSL status', example: '$ sigardapanel ssl status --site 1\n{\n  "site_id": 1,\n  "domain": "blog.example.com",\n  "ssl_status": "active",\n  "ssl_issuer": "Let\'s Encrypt",\n  "ssl_expires_at": "2026-10-10T00:00:00Z"\n}' },
            { cmd: 'ssl issue --site ID --email E', desc: 'Issue Let\'s Encrypt cert', example: '$ sigardapanel ssl issue --site 1 --email admin@example.com\nssl issue job queued (job_id=15)' },
            { cmd: 'ssl renew --site ID', desc: 'Renew certificate', example: '$ sigardapanel ssl renew --site 1\nssl renew job queued (job_id=16)' },
            { cmd: 'ssl renew-all [--dry-run]', desc: 'Renew all expiring certs', example: '$ sigardapanel ssl renew-all --dry-run\nSSL certificates expiring within 30 days (2):\n\nID  DOMAIN              EXPIRES\n1   blog.example.com    2026-07-25 (13 days left)\n2   api.example.com     2026-07-20 (8 days left)\n\nDry run — no jobs queued.' },
          ] as item}
            <div class="bg-[#0d1526] rounded-lg border border-white/[0.08] p-3">
              <code class="text-sm font-mono text-sky-300">sigardapanel {item.cmd}</code>
              <p class="text-xs text-slate-400 mt-1">{item.desc}</p>
              <pre class="text-xs font-mono text-emerald-400/70 mt-2 bg-black/20 rounded p-2 overflow-x-auto"><code>{item.example}</code></pre>
            </div>
          {/each}
        </div>

        <!-- Database -->
        <h2 class="text-xl font-semibold text-white mb-4">Database Management</h2>
        <div class="space-y-2 mb-8">
          {#each [
            { cmd: 'db create --site ID --name X [--type mysql|postgres]', desc: 'Create database', example: '$ sigardapanel db create --site 1 --name mydb --type mysql\ndatabase queued (id=1, job_id=20)' },
            { cmd: 'db list --site ID', desc: 'List databases', example: '$ sigardapanel db list --site 1\nID  NAME    TYPE    CHARSET  SIZE\n1   mydb    mysql   utf8mb4  0' },
            { cmd: 'db update --id ID [--name N] [--charset C]', desc: 'Update database', example: '$ sigardapanel db update --id 1 --charset utf8mb4\ndatabase updated (id=1)' },
            { cmd: 'db delete <id>', desc: 'Delete database', example: '$ sigardapanel db delete 1\ndatabase 1 deleted (cleanup job queued)' },
            { cmd: 'db user create --database ID --username U [--password P]', desc: 'Create DB user', example: '$ sigardapanel db user create --database 1 --username app_user\ndb user queued (user_id=1, username=app_user, job_id=25)' },
            { cmd: 'db user rotate-password --database DB_ID --user USER_ID', desc: 'Rotate password', example: '$ sigardapanel db user rotate-password --database 1 --user 2\npassword rotated for user 2 on database 1' },
          ] as item}
            <div class="bg-[#0d1526] rounded-lg border border-white/[0.08] p-3">
              <code class="text-sm font-mono text-sky-300">sigardapanel {item.cmd}</code>
              <p class="text-xs text-slate-400 mt-1">{item.desc}</p>
              <pre class="text-xs font-mono text-emerald-400/70 mt-2 bg-black/20 rounded p-2 overflow-x-auto"><code>{item.example}</code></pre>
            </div>
          {/each}
        </div>

        <!-- Backup -->
        <h2 class="text-xl font-semibold text-white mb-4">Backup Management</h2>
        <div class="space-y-2 mb-8">
          {#each [
            { cmd: 'backup create --site ID', desc: 'Create a new backup', example: '$ sigardapanel backup create --site 1\nbackup queued (backup_id=5, job_id=30)' },
            { cmd: 'backup list [--site ID]', desc: 'List backups', example: '$ sigardapanel backup list --site 1\nID  SITE_ID  STATUS    CREATED\n5   1        success   2026-07-12T10:00:00Z' },
            { cmd: 'backup delete <id>', desc: 'Delete a backup', example: '$ sigardapanel backup delete 5\nbackup 5 deleted' },
            { cmd: 'backup restore <id>', desc: 'Restore from backup', example: '$ sigardapanel backup restore 5\nrestore job queued (backup_id=5, job_id=35)' },
          ] as item}
            <div class="bg-[#0d1526] rounded-lg border border-white/[0.08] p-3">
              <code class="text-sm font-mono text-sky-300">sigardapanel {item.cmd}</code>
              <p class="text-xs text-slate-400 mt-1">{item.desc}</p>
              <pre class="text-xs font-mono text-emerald-400/70 mt-2 bg-black/20 rounded p-2 overflow-x-auto"><code>{item.example}</code></pre>
            </div>
          {/each}
        </div>

        <!-- Docker -->
        <h2 class="text-xl font-semibold text-white mb-4">Docker Management</h2>
        <div class="space-y-2 mb-8">
          {#each [
            { cmd: 'docker ps --server ID', desc: 'List containers', example: '$ sigardapanel docker ps --server 1\nID            NAME          IMAGE          STATE      PORTS\n2b7b7b89b277  test-nginx    nginx:alpine   running    0.0.0.0:8888->80/tcp' },
            { cmd: 'docker start --server ID --id CID', desc: 'Start container', example: '$ sigardapanel docker start --server 1 --id abc123\ncontainer abc123 started' },
            { cmd: 'docker stop --server ID --id CID', desc: 'Stop container', example: '$ sigardapanel docker stop --server 1 --id abc123\ncontainer abc123 stopped' },
            { cmd: 'docker logs --server ID --id CID [--lines N]', desc: 'View logs', example: '$ sigardapanel docker logs --server 1 --id abc123 --lines 10\n[nginx] 172.17.0.1 - - "GET / HTTP/1.1" 200 612 "-" "curl/7.88.1"' },
            { cmd: 'docker images --server ID', desc: 'List images', example: '$ sigardapanel docker images --server 1\nREPOSITORY   TAG     SIZE      CREATED\nnginx        alpine  93.3MB    2026-06-22' },
            { cmd: 'docker compose up --server ID [--path PATH]', desc: 'Start compose', example: '$ sigardapanel docker compose up --server 1 --path /app/docker-compose.yml\ncompose up completed' },
          ] as item}
            <div class="bg-[#0d1526] rounded-lg border border-white/[0.08] p-3">
              <code class="text-sm font-mono text-sky-300">sigardapanel {item.cmd}</code>
              <p class="text-xs text-slate-400 mt-1">{item.desc}</p>
              <pre class="text-xs font-mono text-emerald-400/70 mt-2 bg-black/20 rounded p-2 overflow-x-auto"><code>{item.example}</code></pre>
            </div>
          {/each}
        </div>

        <!-- Firewall -->
        <h2 class="text-xl font-semibold text-white mb-4">Firewall Management</h2>
        <div class="space-y-2 mb-8">
          {#each [
            { cmd: 'firewall status --server ID', desc: 'Show firewall status', example: '$ sigardapanel firewall status --server 1\n🟢 Firewall: ACTIVE\n\nStatus: active\n\nTo                         Action      From\n--                         ------      ----\n22/tcp                     ALLOW       Anywhere\n80/tcp                     ALLOW       Anywhere\n443/tcp                    ALLOW       Anywhere' },
            { cmd: 'firewall allow --server ID --port PORT [--protocol PROTO]', desc: 'Allow port', example: '$ sigardapanel firewall allow --server 1 --port 443 --protocol tcp\nport 443 allowed' },
            { cmd: 'firewall deny --server ID --port PORT [--from IP]', desc: 'Deny port', example: '$ sigardapanel firewall deny --server 1 --port 3306 --from 10.0.0.0/8\nport 3306 denied' },
            { cmd: 'firewall rules --server ID', desc: 'List rules', example: '$ sigardapanel firewall rules --server 1\n#  ACTION  FROM        TO\n1  ALLOW   Anywhere    22/tcp\n2  ALLOW   Anywhere    80/tcp\n3  ALLOW   Anywhere    443/tcp' },
            { cmd: 'firewall enable --server ID', desc: 'Enable firewall', example: '$ sigardapanel firewall enable --server 1\nfirewall enabled' },
            { cmd: 'firewall disable --server ID', desc: 'Disable firewall', example: '$ sigardapanel firewall disable --server 1\nfirewall disabled' },
          ] as item}
            <div class="bg-[#0d1526] rounded-lg border border-white/[0.08] p-3">
              <code class="text-sm font-mono text-sky-300">sigardapanel {item.cmd}</code>
              <p class="text-xs text-slate-400 mt-1">{item.desc}</p>
              <pre class="text-xs font-mono text-emerald-400/70 mt-2 bg-black/20 rounded p-2 overflow-x-auto"><code>{item.example}</code></pre>
            </div>
          {/each}
        </div>

        <!-- Redis -->
        <h2 class="text-xl font-semibold text-white mb-4">Redis Management</h2>
        <div class="space-y-2 mb-8">
          {#each [
            { cmd: 'redis stats --server ID', desc: 'Show stats', example: '$ sigardapanel redis stats --server 1\nRedis Statistics:\n─────────────────────────────────────\n  connected_clients              1\n  used_memory                    856.2K\n  uptime_in_seconds              86400' },
            { cmd: 'redis info --server ID', desc: 'Show detailed info', example: '$ sigardapanel redis info --server 1\n# Server\nredis_version:7.0.12\nuptime_in_seconds:86400\n# Memory\nused_memory_human:856.2K' },
            { cmd: 'redis flush --server ID', desc: 'Flush ALL data', example: '$ sigardapanel redis flush --server 1\nFlush ALL Redis data? [y/N]: y\nRedis data flushed' },
            { cmd: 'redis flushdb --server ID --db N', desc: 'Flush specific db', example: '$ sigardapanel redis flushdb --server 1 --db 0\nRedis database 0 flushed' },
          ] as item}
            <div class="bg-[#0d1526] rounded-lg border border-white/[0.08] p-3">
              <code class="text-sm font-mono text-sky-300">sigardapanel {item.cmd}</code>
              <p class="text-xs text-slate-400 mt-1">{item.desc}</p>
              <pre class="text-xs font-mono text-emerald-400/70 mt-2 bg-black/20 rounded p-2 overflow-x-auto"><code>{item.example}</code></pre>
            </div>
          {/each}
        </div>

        <!-- System -->
        <h2 class="text-xl font-semibold text-white mb-4">System & User Management</h2>
        <div class="space-y-2 mb-8">
          {#each [
            { cmd: 'system disk-usage --server ID', desc: 'Check disk usage', example: '$ sigardapanel system disk-usage --server 1\nServer Disk Usage: ✅\n\n  Disk:  45.2 GB / 100.0 GB (45.2% used, 54.8 GB free)\n  Mem:   3.2 GB / 8.0 GB\n  CPU:   12.5%' },
            { cmd: 'system updates --server ID', desc: 'Check updates', example: '$ sigardapanel system updates --server 1\nAvailable updates:\n\nnginx/stable 1.24.0 amd64\npostgresql-16/stable 16.3 amd64' },
            { cmd: 'user create [--username U] [--email E] [--password P] [--role R]', desc: 'Create user', example: '$ sigardapanel user create --username john --email john@example.com --role admin\nuser created (id=4)\nusername: john\nemail: john@example.com\nrole: admin' },
            { cmd: 'user list', desc: 'List users', example: '$ sigardapanel user list\nID  USERNAME    EMAIL                     ROLE         CREATED\n1   admin       admin@localhost           admin        2026-07-10\n2   hanyawahyu  hanyawahyusaja@gmail.com  super_admin  2026-07-12\ntotal: 2' },
            { cmd: 'user reset-password <id>', desc: 'Reset password', example: '$ sigardapanel user reset-password 1 --password newpass\npassword updated for user 1' },
          ] as item}
            <div class="bg-[#0d1526] rounded-lg border border-white/[0.08] p-3">
              <code class="text-sm font-mono text-sky-300">sigardapanel {item.cmd}</code>
              <p class="text-xs text-slate-400 mt-1">{item.desc}</p>
              <pre class="text-xs font-mono text-emerald-400/70 mt-2 bg-black/20 rounded p-2 overflow-x-auto"><code>{item.example}</code></pre>
            </div>
          {/each}
        </div>

        <!-- Env Vars -->
        <h2 class="text-xl font-semibold text-white mb-4">Environment Variables</h2>
        <div class="bg-[#0d1526] rounded-xl border border-white/[0.08] p-4 overflow-x-auto">
          <table class="w-full text-sm">
            <thead>
              <tr class="text-left text-slate-500 border-b border-white/[0.06]">
                <th class="pb-2 pr-4">Variable</th>
                <th class="pb-2 pr-4">Default</th>
                <th class="pb-2">Description</th>
              </tr>
            </thead>
            <tbody class="text-slate-300">
              {#each [
                { var: 'SIGARDAPANEL_API_ADDR', def: ':7700', desc: 'API listen address' },
                { var: 'SIGARDAPANEL_AGENT_ADDR', def: ':7710', desc: 'Agent listen address' },
                { var: 'SIGARDAPANEL_DB_PATH', def: 'sigardapanel.db', desc: 'Database path' },
                { var: 'SIGARDAPANEL_API_URL', def: '-', desc: 'API base URL for CLI' },
                { var: 'SIGARDAPANEL_TOKEN', def: '-', desc: 'API token from login' },
                { var: 'SIGARDAPANEL_OUTPUT', def: 'table', desc: 'Output format (table|json|yaml)' },
              ] as item}
                <tr class="border-b border-white/[0.04]">
                  <td class="py-2 pr-4 font-mono text-sky-300">{item.var}</td>
                  <td class="py-2 pr-4 font-mono text-slate-500">{item.def}</td>
                  <td class="py-2 text-slate-400">{item.desc}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      </section>

      <!-- API Reference -->
      <section id="api" class="mb-16">
        <h1 class="text-3xl font-bold text-white mb-2">API Reference</h1>
        <p class="text-slate-400 mb-8">REST API for integrations and automation.</p>

        <h2 class="text-xl font-semibold text-white mb-4">Authentication</h2>
        <div class="bg-[#0d1526] rounded-xl border border-white/[0.08] p-4 mb-6">
          <pre class="text-sm font-mono text-slate-300"><code>Authorization: Bearer YOUR_TOKEN_HERE</code></pre>
        </div>

        <h2 class="text-xl font-semibold text-white mb-4">Response Format</h2>
        <div class="bg-[#0d1526] rounded-xl border border-white/[0.08] p-4 mb-6">
          <pre class="text-sm font-mono text-slate-300"><code>&#123;'&#123;'&#125;
  "data": &#123;'&#123;'&#125; ... &#123;'&#125;'&#125;,
  "request_id": "req_xxx"
{'}'}</code></pre>
        </div>

        <h2 class="text-xl font-semibold text-white mb-4">Example: Login</h2>
        <div class="bg-[#0d1526] rounded-xl border border-white/[0.08] p-4 mb-6">
          <pre class="text-sm font-mono text-slate-300 overflow-x-auto"><code>curl -X POST http://localhost:7700/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '&#123;"username":"admin","password":"mypassword"&#125;'

# Response:
&#123;'&#123;'&#125;
  "data": &#123;'&#123;'&#125;
    "token": "abc123...",
    "user": &#123;'&#123;'&#125;
      "id": 1,
      "username": "admin",
      "role": "admin"
    &#123;'&#125;'&#125;
  &#123;'&#125;'&#125;,
  "request_id": "req_xxx"
{'}'}</code></pre>
        </div>

        <h2 class="text-xl font-semibold text-white mb-4">Endpoints</h2>
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
          ] as ep}
            <div class="flex items-center gap-3 p-3 rounded-lg bg-white/[0.03] border border-white/[0.06]">
              <span class="px-2 py-0.5 rounded text-xs font-mono font-bold
                {ep.method === 'GET' ? 'bg-emerald-500/10 text-emerald-400' :
                 ep.method === 'POST' ? 'bg-sky-500/10 text-sky-400' :
                 ep.method === 'PATCH' ? 'bg-amber-500/10 text-amber-400' :
                 'bg-red-500/10 text-red-400'}">{ep.method}</span>
              <code class="text-sm font-mono text-slate-300">{ep.path}</code>
              <span class="text-xs text-slate-500 ml-auto hidden sm:block">{ep.desc}</span>
            </div>
          {/each}
        </div>
      </section>

      <!-- Troubleshooting -->
      <section id="troubleshooting" class="mb-16">
        <h1 class="text-3xl font-bold text-white mb-2">Troubleshooting</h1>
        <p class="text-slate-400 mb-8">Common issues and solutions.</p>

        <div class="space-y-4">
          <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-5">
            <h3 class="text-sm font-semibold text-white mb-2">Agent shows "Offline"</h3>
            <ul class="space-y-1 text-sm text-slate-300">
              <li>1. Check agent is running: <code class="bg-white/[0.06] px-1 rounded text-sky-300">systemctl status sigardapanel-agent</code></li>
              <li>2. Check port 7710 is open: <code class="bg-white/[0.06] px-1 rounded text-sky-300">ss -tlnp | grep 7710</code></li>
              <li>3. Check agent token matches panel config</li>
              <li>4. Check network connectivity from panel to agent</li>
            </ul>
          </div>

          <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-5">
            <h3 class="text-sm font-semibold text-white mb-2">SSL certificate issue fails</h3>
            <ul class="space-y-1 text-sm text-slate-300">
              <li>1. Ensure domain points to this server's IP</li>
              <li>2. Check port 80 is accessible (Let's Encrypt HTTP challenge)</li>
              <li>3. Verify nginx is running: <code class="bg-white/[0.06] px-1 rounded text-sky-300">systemctl status nginx</code></li>
              <li>4. Check certbot is installed: <code class="bg-white/[0.06] px-1 rounded text-sky-300">certbot --version</code></li>
            </ul>
          </div>

          <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-5">
            <h3 class="text-sm font-semibold text-white mb-2">Docker commands fail</h3>
            <ul class="space-y-1 text-sm text-slate-300">
              <li>1. Check Docker is installed: <code class="bg-white/[0.06] px-1 rounded text-sky-300">docker --version</code></li>
              <li>2. Check Docker is running: <code class="bg-white/[0.06] px-1 rounded text-sky-300">systemctl status docker</code></li>
              <li>3. Check user permissions: <code class="bg-white/[0.06] px-1 rounded text-sky-300">usermod -aG docker $USER</code></li>
            </ul>
          </div>

          <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-5">
            <h3 class="text-sm font-semibold text-white mb-2">Firewall blocks access</h3>
            <ul class="space-y-1 text-sm text-slate-300">
              <li>1. Check firewall status: <code class="bg-white/[0.06] px-1 rounded text-sky-300">sigardapanel firewall status --server 1</code></li>
              <li>2. Allow SSH: <code class="bg-white/[0.06] px-1 rounded text-sky-300">sigardapanel firewall allow --server 1 --port 22 --protocol tcp</code></li>
              <li>3. Allow HTTP/HTTPS: <code class="bg-white/[0.06] px-1 rounded text-sky-300">sigardapanel firewall allow --server 1 --port 80,443 --protocol tcp</code></li>
            </ul>
          </div>
        </div>
      </section>

      <!-- Footer -->
      <footer class="border-t border-white/[0.06] pt-8 text-center text-sm text-slate-500">
        <p>SigardaPanel Documentation v0.5.3</p>
        <p class="mt-1">Built with ❤️ by <a href="https://github.com/bayurstarcool" class="text-sky-400 hover:text-sky-300">bayurstarcool</a> Built with ❤️ by <a href="https://github.com/bayurstarcool" class="text-sky-400 hover:text-sky-300">bayurstarcool</a></p>
      </footer>
    </main>
  </div>
</div>
