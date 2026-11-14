<script lang="ts">
  import { page } from '$app/state';
  import { auth, hasRole, hasPermission } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { fmtBytesFull } from '$lib/format';
  import { onMount } from 'svelte';
  import ProBadge from './ProBadge.svelte';
  import Logo from './Logo.svelte';
  import { licenseStore } from '$lib/license';
  import {
    LayoutDashboard, Server, Globe, ShieldCheck, Database, Package, Zap,
    Folder, BarChart3, Users, ClipboardCheck, Cloud, Bell, DollarSign, ShoppingCart,
    Layers, Box, Shield, HardDrive, Mail, Cpu, Bug, ShieldAlert, BookOpen, Activity, SquareTerminal, ShieldBan,
  } from 'lucide-svelte';

  // `active` is kept for API compatibility with existing pages; the current
  // section is derived from the URL so it can never drift from the route.
  let { active = '', mobileOpen, onCloseMobile }: { active?: string; mobileOpen: boolean; onCloseMobile: () => void } = $props();

  type Item = { label: string; href: string; icon: any; pro?: boolean };

  // The API does not limit servers or users by license tier (the free tier's
  // max_servers is informational), so the badge must not imply a block.
  const PRO_NOT_ENFORCED =
    'Listed in the Pro plan. Not restricted on this panel: every license tier can use it.';
  type Group = { title: string; items: Item[] };

  const groups = $derived.by((): Group[] => {
    const can = (p: string) => hasPermission(p);
    const g: Group[] = [
      { title: '', items: [{ label: 'Overview', href: '/dashboard', icon: LayoutDashboard }] },
      {
        title: 'Hosting',
        items: [
          { label: 'Sites', href: '/sites', icon: Globe },
          { label: 'SSL', href: '/ssl', icon: ShieldCheck },
          { label: 'Databases', href: '/databases', icon: Database },
          { label: 'Files', href: '/files', icon: Folder },
          { label: 'Backups', href: '/backups', icon: Package },
          { label: 'Cloudflare', href: '/cloudflare', icon: Cloud },
        ],
      },
      {
        title: 'Infrastructure',
        items: [
          { label: 'Servers', href: '/servers', icon: Server, pro: true },
          { label: 'Stack', href: '/stack', icon: Layers },
          // Server-level tools: their API routes are admin-only, so other
          // roles would only get pages that cannot load.
          ...(can('docker.manage') ? [{ label: 'Docker', href: '/docker', icon: Box }] : []),
          ...(can('server.services')
            ? [
                { label: 'PM2', href: '/pm2', icon: Cpu },
                { label: 'Redis', href: '/redis', icon: Zap },
                { label: 'Email', href: '/email', icon: Mail },
              ]
            : []),
          ...(can('server.terminal') ? [{ label: 'Terminal', href: '/terminal', icon: SquareTerminal }] : []),
          { label: 'Monitoring', href: '/monitoring', icon: BarChart3 },
          { label: 'System', href: '/system', icon: HardDrive },
        ],
      },
      {
        title: 'Security',
        items: [
          ...(can('server.firewall')
            ? [
                { label: 'Firewall', href: '/firewall', icon: Shield },
                { label: 'Fail2ban', href: '/fail2ban', icon: ShieldBan },
                { label: 'Bot Blocker', href: '/bot-blocker', icon: Bug },
                { label: 'Threat Monitor', href: '/threats', icon: ShieldAlert },
              ]
            : []),
        ],
      },
      {
        title: 'Activity',
        items: [
          { label: 'Jobs', href: '/jobs', icon: Activity },
          { label: 'Notifications', href: '/notifications', icon: Bell },
        ],
      },
    ];
    const admin: Item[] = [];
    if (can('user.manage')) admin.push({ label: 'Users', href: '/users', icon: Users, pro: true });
    if (can('audit.view')) admin.push({ label: 'Audit', href: '/audit', icon: ClipboardCheck });
    if (can('plan.manage')) admin.push({ label: 'Plans', href: '/admin/plans', icon: DollarSign });
    if (can('order.manage')) admin.push({ label: 'Orders', href: '/admin/orders', icon: ShoppingCart });
    if (admin.length) g.push({ title: 'Admin', items: admin });
    g.push({ title: '', items: [{ label: 'Docs', href: '/docs', icon: BookOpen }] });
    return g;
  });

  // Longest href that prefixes the current path wins (/server/1/… → Servers).
  const activeHref = $derived.by(() => {
    const path = page.url.pathname;
    const aliases: Record<string, string> = { '/server': '/servers', '/database-users': '/databases' };
    let best = '';
    for (const grp of groups) {
      for (const it of grp.items) {
        if ((path === it.href || path.startsWith(it.href + '/')) && it.href.length > best.length) best = it.href;
      }
    }
    if (!best) {
      for (const [prefix, href] of Object.entries(aliases)) {
        if (path === prefix || path.startsWith(prefix + '/')) best = href;
      }
    }
    if (!best && active) {
      for (const grp of groups) for (const it of grp.items) if (it.label === active) best = it.href;
    }
    return best;
  });

  let usageData = $state<any>(null);

  // Usage is re-read every minute (the sidebar stays mounted across page
  // changes), and sooner while sites are still being measured: the first
  // measurement of a large site can take longer than one request waits.
  onMount(() => {
    if (!auth.user?.id) return;
    const currentId = auth.isImpersonating && auth.impersonatingUser ? auth.impersonatingUser.id : auth.user.id;
    let timer: ReturnType<typeof setTimeout> | undefined;
    let stopped = false;
    const load = () => {
      apiFetch(auth.token, 'GET', `users/${currentId}/usage`)
        .then((d) => { usageData = d; })
        .catch(() => {})
        .finally(() => {
          if (stopped) return;
          timer = setTimeout(load, (usageData?.disk_unmeasured_sites ?? 0) > 0 ? 15000 : 60000);
        });
    };
    load();
    return () => { stopped = true; clearTimeout(timer); };
  });

  // Denominator: the user's disk limit, else the disk capacity of the
  // servers in scope (all servers for an admin), else none.
  const diskLimit = $derived((usageData?.disk_limit_bytes ?? 0) as number);
  const diskCapacity = $derived((usageData?.disk_capacity_bytes ?? 0) as number);
  const hasDiskLimit = $derived(diskLimit > 0);
  const diskDenominator = $derived(hasDiskLimit ? diskLimit : diskCapacity);
  const diskUsed = $derived((usageData?.disk_used_bytes ?? 0) as number);
  const diskPct = $derived(diskDenominator > 0 ? Math.min(Math.round((diskUsed / diskDenominator) * 100), 100) : 0);
  // Sites whose server could not be measured are not in disk_used_bytes: then
  // the figure is a lower bound.
  const diskUnmeasured = $derived(usageData?.disk_unmeasured_sites ?? 0);
  const diskUsedText = $derived(`${diskUnmeasured > 0 ? '≥ ' : ''}${fmtBytesFull(diskUsed)}`);
  const diskLabel = $derived(diskDenominator > 0 ? `${diskUsedText} / ${fmtBytesFull(diskDenominator)}` : diskUsedText);
  const diskTitle = $derived(
    [
      hasDiskLimit
        ? 'Sites you own, against your disk limit'
        : usageData?.disk_scope === 'all'
          ? 'All sites on the panel, against the disk capacity of all servers'
          : 'Sites you own, against the disk capacity of their servers (no limit set)',
      diskUnmeasured > 0 ? `${diskUnmeasured} site(s) not measured yet (server offline or still measuring)` : '',
    ].filter(Boolean).join('. '),
  );

  const maxSites = $derived(usageData?.max_sites);
  const sitesUnlimited = $derived(maxSites === null || maxSites === undefined || maxSites === 0);
  const sitesPct = $derived(!sitesUnlimited ? Math.min(Math.round((usageData?.site_count ?? 0) / maxSites * 100), 100) : 0);
  const sitesLabel = $derived(!sitesUnlimited ? `${usageData?.site_count ?? 0} / ${maxSites}` : `${usageData?.site_count ?? 0}`);
</script>

{#snippet nav()}
  <nav class="flex-1 overflow-y-auto px-3 py-2">
    {#each groups as grp, i (i)}
      {#if grp.items.length}
        <div class={i > 0 ? 'mt-5' : ''}>
          {#if grp.title}
            <div class="mb-1.5 px-2.5 text-[11px] font-medium tracking-wide text-subtle uppercase">{grp.title}</div>
          {/if}
          <ul class="space-y-0.5">
            {#each grp.items as item (item.href)}
              {@const isActive = activeHref === item.href}
              <li>
                <a
                  href={item.href}
                  onclick={onCloseMobile}
                  aria-current={isActive ? 'page' : undefined}
                  class="group flex h-8 items-center gap-2.5 rounded-md px-2.5 text-[13px] font-medium transition-colors duration-100
                    {isActive ? 'bg-surface-2 text-fg' : 'text-muted hover:bg-surface-2/60 hover:text-fg'}"
                >
                  <item.icon size={16} strokeWidth={1.75} class={isActive ? 'text-accent' : 'text-subtle group-hover:text-muted'} />
                  <span class="truncate">{item.label}</span>
                  {#if item.pro}<span class="ml-auto"><ProBadge tier="pro" small title={PRO_NOT_ENFORCED} /></span>{/if}
                </a>
              </li>
            {/each}
          </ul>
        </div>
      {/if}
    {/each}
  </nav>
{/snippet}

{#snippet panel()}
  <div class="flex h-14 shrink-0 items-center justify-between gap-2 px-5">
    <a href="/dashboard" class="flex items-center" onclick={onCloseMobile}><Logo size={26} withText /></a>
    {#if licenseStore.tier !== 'free'}
      <ProBadge tier={licenseStore.tier} small />
    {/if}
  </div>
  {@render nav()}
  {#if usageData}
    <div class="shrink-0 space-y-3 border-t border-line px-5 py-4">
      <div>
        <div class="mb-1.5 flex justify-between text-[11px] text-subtle">
          <span>Disk</span><span class="tabular-nums" title={diskTitle}>{diskLabel}</span>
        </div>
        <div class="progress h-1"><div class="progress-bar {hasDiskLimit && diskPct >= 100 ? 'bg-danger' : hasDiskLimit && diskPct >= 80 ? 'bg-warning' : ''}" style="width: {diskPct}%"></div></div>
        {#if hasDiskLimit && diskPct >= 100}
          <div class="mt-1 text-[11px] text-danger">Disk limit reached: uploads, edits and deploys are blocked</div>
        {/if}
      </div>
      <div>
        <div class="mb-1.5 flex justify-between text-[11px] text-subtle">
          <span>Sites</span><span class="tabular-nums">{sitesLabel}</span>
        </div>
        <div class="progress h-1"><div class="progress-bar" style="width: {sitesPct}%"></div></div>
      </div>
    </div>
  {/if}
{/snippet}

<!-- Mobile: off-canvas (transform only) -->
{#if mobileOpen}
  <button class="overlay animate-fade-in lg:hidden" aria-label="Close menu" onclick={onCloseMobile}></button>
{/if}
<aside
  class="fixed inset-y-0 left-0 z-50 flex w-64 flex-col border-r border-line bg-surface transition-transform duration-200 ease-out lg:hidden
    {mobileOpen ? 'translate-x-0' : '-translate-x-full'}"
  aria-hidden={!mobileOpen}
  inert={!mobileOpen}
>
  {@render panel()}
</aside>

<!-- Desktop -->
<aside class="fixed inset-y-0 left-0 z-30 hidden w-60 flex-col border-r border-line bg-surface lg:flex">
  {@render panel()}
</aside>
