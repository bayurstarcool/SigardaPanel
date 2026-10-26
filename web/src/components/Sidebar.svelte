<script lang="ts">
  import { theme } from '$lib/theme.svelte';
  import { auth, hasRole } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { fmtBytesFull } from '$lib/format';
  import { onMount } from 'svelte';
  import ProBadge from './ProBadge.svelte';
  import { licenseStore } from '$lib/license';
  import {
    LayoutDashboard, Server, Globe, ShieldCheck, Database, Package, Zap,
    Folder, BarChart3, Users, ClipboardCheck, LogOut, ChevronRight,
    Cloud, User, Bell, DollarSign, ShoppingCart, Layers, Box, Shield, HardDrive, Mail, Cpu, Bug, Sun, Moon, ShieldAlert,
  } from 'lucide-svelte';

  let { active, mobileOpen, onCloseMobile }: { active: string; mobileOpen: boolean; onCloseMobile: () => void } = $props();

  const iconMap: Record<string, any> = {
    Overview: LayoutDashboard, Servers: Server, Stack: Layers, Sites: Globe, SSL: ShieldCheck,
    Databases: Database, Backups: Package, Jobs: Zap, Files: Folder,
    'Database Users': Users, Cloudflare: Cloud, Storage: Database, Profile: User,
    Notifications: Bell, Plans: DollarSign, Orders: ShoppingCart,
    Monitoring: BarChart3, Users: Users, Audit: ClipboardCheck,
    Docker: Box, Firewall: Shield, Redis: Zap, System: HardDrive, 'Threat Monitor': ShieldAlert,
    Email: Mail, PM2: Cpu, 'Bot Blocker': Bug,
  };

  const navItems = [
    { label: 'Overview', href: '/dashboard' },
    { label: 'Servers', href: '/servers', pro: true },
    { label: 'Stack', href: '/stack' },
    { label: 'Sites', href: '/sites' },
    { label: 'SSL', href: '/ssl' },
    { label: 'Databases', href: '/databases' },
    { label: 'Backups', href: '/backups' },
    { label: 'Docker', href: '/docker' },
    { label: 'Firewall', href: '/firewall' },
    { label: 'Redis', href: '/redis' },
    { label: 'Email', href: '/email' },
    { label: 'PM2', href: '/pm2' },
    { label: 'Bot Blocker', href: '/bot-blocker' },
    { label: 'System', href: '/system' },
    { label: 'Profile', href: '/profile' },
    { label: 'Notifications', href: '/notifications' },
    { label: 'Jobs', href: '/jobs' },
    { label: 'Monitoring', href: '/monitoring' },
    { label: 'Cloudflare', href: '/cloudflare' },
    { label: 'Docs', href: '/docs' },
  ];

  const adminNav = $derived.by(() => {
    const items = [
      { label: 'Files', href: '/files' },
      { label: 'Users', href: '/users', pro: true },
      { label: 'Audit', href: '/audit' },
      { label: 'Threat Monitor', href: '/threats' },
      { label: 'Storage', href: '/backups' },
    ];
    if (hasRole(auth.user?.role, 'super_admin')) {
      items.push({ label: 'Plans', href: '/admin/plans' });
      items.push({ label: 'Orders', href: '/admin/orders' });
    }
    return items;
  });

  let usageData = $state<any>(null);

  onMount(() => {
    if (!auth.user?.id) return;
    const currentId = auth.isImpersonating && auth.impersonatingUser ? auth.impersonatingUser.id : auth.user.id;
    apiFetch(auth.token, 'GET', `users/${currentId}/usage`).then((d) => { usageData = d; }).catch(() => {});
  });

  const diskUnlimited = $derived(usageData?.disk_unlimited);
  const hasDiskLimit = $derived(!diskUnlimited && (usageData?.disk_limit_bytes ?? 0) > 0);
  const diskPct = $derived(hasDiskLimit ? Math.min(Math.round((usageData?.disk_used_bytes ?? 0) / (usageData?.disk_limit_bytes ?? 1) * 100), 100) : 0);
  const diskLabel = $derived(hasDiskLimit ? `${fmtBytesFull(usageData?.disk_used_bytes ?? 0)} / ${usageData?.disk_limit_gb} GB` : `${fmtBytesFull(usageData?.disk_used_bytes ?? 0)} / Unlimited`);
  const diskBarPct = $derived(diskUnlimited ? Math.min(100, Math.round((usageData?.disk_used_bytes ?? 0) / 1073741824 / 100 * 100)) : diskPct);

  const maxSites = $derived(usageData?.max_sites);
  const sitesUnlimited = $derived(maxSites === null || maxSites === 0);
  const sitesPct = $derived(!sitesUnlimited ? Math.min(Math.round((usageData?.site_count ?? 0) / maxSites * 100), 100) : 0);
  const sitesLabel = $derived(!sitesUnlimited ? `${usageData?.site_count} / ${maxSites}` : `${usageData?.site_count} / Unlimited`);

  function handleLogout() {
    if (auth.isImpersonating) {
      auth.stopImpersonation();
    } else {
      auth.logout();
      window.location.href = '/login';
    }
  }

  const displayName = $derived(auth.isImpersonating && auth.impersonatingUser ? auth.impersonatingUser : auth.user);
</script>

<svelte:head>
  <style>
    /* ===== SIDEBAR LIGHT MODE — Soft Premium ===== */

    html.light .sidebar-card {
      background: rgba(255, 255, 255, 0.96);
      border-right: 1px solid rgba(0, 0, 0, 0.04);
      box-shadow: 4px 0 24px -4px rgba(0, 0, 0, 0.03);
    }

    html.light .sidebar-header {
      background: rgba(255, 255, 255, 0.97);
      border-bottom: 1px solid rgba(0, 0, 0, 0.04);
    }

    html.light .sidebar-footer {
      background: rgba(255, 255, 255, 0.97);
      border-top: 1px solid rgba(0, 0, 0, 0.04);
    }

    /* Nav items — warm, soft feel */
    html.light .sidebar-nav-item {
      color: #6b7280;
      border: 1px solid transparent;
    }
    html.light .sidebar-nav-item:hover {
      background: rgba(0, 0, 0, 0.025);
      color: #374151;
    }
    html.light .sidebar-nav-item.active {
      background: linear-gradient(135deg, rgba(56, 189, 248, 0.07), rgba(99, 102, 241, 0.04), rgba(139, 92, 246, 0.02));
      border: 1px solid rgba(56, 189, 248, 0.12);
      color: #1a2332;
      box-shadow: 0 1px 4px rgba(56, 189, 248, 0.06);
    }

    /* Active icon accent — softer glow */
    html.light .sidebar-nav-item.active .sidebar-active-bar {
      box-shadow: 0 0 10px rgba(56, 189, 248, 0.25);
      background: linear-gradient(180deg, #38bdf8, #818cf8);
    }

    /* Progress bars — softer */
    html.light .sidebar-progress {
      background: rgba(0, 0, 0, 0.035);
    }

    /* Divider — barely there */
    html.light .sidebar-divider {
      border-color: rgba(0, 0, 0, 0.04);
    }

    /* Admin label — clean white pill */
    html.light .sidebar-admin-label {
      background: rgba(255, 255, 255, 0.97);
    }

    /* Sign out button — softer hover */
    html.light .sidebar-signout:hover {
      background: rgba(239, 68, 68, 0.04);
      border-color: rgba(239, 68, 68, 0.12);
      color: #dc2626;
    }

    /* User avatar — subtle shadow */
    html.light .sidebar-user-avatar {
      box-shadow: 0 2px 8px rgba(56, 189, 248, 0.15);
    }

    /* Scrollbar in sidebar */
    html.light .sidebar-card ::-webkit-scrollbar {
      width: 4px;
    }
    html.light .sidebar-card ::-webkit-scrollbar-track {
      background: transparent;
    }
    html.light .sidebar-card ::-webkit-scrollbar-thumb {
      background: rgba(0, 0, 0, 0.08);
      border-radius: 2px;
    }
  </style>
</svelte:head>

{#snippet navContent()}
  <nav class="flex-1 space-y-1 overflow-y-auto overflow-x-hidden py-3 px-3 scrollbar-thin">
    {#each navItems as item}
      {@const isActive = active === item.label}
      <a href={item.href} onclick={onCloseMobile}
        class="sidebar-nav-item group relative flex items-center gap-3 rounded-xl px-3.5 py-2.5 text-[0.82rem] font-medium transition-all duration-200 {isActive ? 'active text-[var(--text-primary)]' : 'text-[var(--text-muted)] hover:text-[var(--text-secondary)]'}">
        {#if isActive}
          <div class="sidebar-active-bar absolute left-0 top-1/2 -translate-y-1/2 w-[3px] h-5 rounded-full bg-gradient-to-b from-sky-400 to-indigo-400 shadow-[0_0_8px_rgba(56,189,248,0.5)]"></div>
        {/if}
        <span class="relative z-10 w-5 flex items-center justify-center transition-colors duration-200 {isActive ? 'text-sky-500 dark:text-sky-400' : 'text-[var(--text-muted)] group-hover:text-[var(--text-secondary)]'}">
          {#if iconMap[item.label]}
            {@const Icon = iconMap[item.label]}
            <Icon size={17} />
          {/if}
        </span>
        <span class="relative z-10">{item.label}</span>
        {#if item.pro}
          <ProBadge tier="pro" small />
        {/if}
        {#if isActive}
          <ChevronRight size={12} class="relative z-10 ml-auto text-sky-400 opacity-50" />
        {/if}
      </a>
    {/each}

    {#if hasRole(auth.user?.role, 'admin')}
      <div class="relative my-5 mx-3">
        <div class="absolute inset-0 flex items-center"><div class="w-full border-t sidebar-divider border-[var(--border-primary)]"></div></div>
        <div class="relative flex justify-center">
          <span class="sidebar-admin-label px-2.5 text-[0.6rem] uppercase tracking-[0.18em] text-[var(--text-muted)] opacity-80 font-semibold">Admin</span>
        </div>
      </div>
      {#each adminNav as item}
        {@const isActive = active === item.label}
        <a href={item.href} onclick={onCloseMobile}
          class="sidebar-nav-item group relative flex items-center gap-3 rounded-xl px-3.5 py-2.5 text-[0.82rem] font-medium transition-all duration-200 {isActive ? 'active text-[var(--text-primary)]' : 'text-[var(--text-muted)] hover:text-[var(--text-secondary)]'}">
          {#if isActive}
            <div class="sidebar-active-bar absolute left-0 top-1/2 -translate-y-1/2 w-[3px] h-5 rounded-full bg-gradient-to-b from-sky-400 to-indigo-400 shadow-[0_0_8px_rgba(56,189,248,0.5)]"></div>
          {/if}
          <span class="relative z-10 w-5 flex items-center justify-center transition-colors duration-200 {isActive ? 'text-sky-500 dark:text-sky-400' : 'text-[var(--text-muted)] group-hover:text-[var(--text-secondary)]'}">
            {#if iconMap[item.label]}
              {@const Icon = iconMap[item.label]}
              <Icon size={17} />
            {/if}
          </span>
          <span class="relative z-10">{item.label}</span>
          {#if item.pro}
            <ProBadge tier="pro" small />
          {/if}
        </a>
      {/each}
    {/if}
  </nav>
{/snippet}

{#snippet userCard()}
  {#if displayName}
    <div class="shrink-0 sidebar-footer p-3 transition-colors duration-300">
      <div class="flex items-center gap-2.5 px-1">
        <div class="relative">
          <div class="sidebar-user-avatar w-9 h-9 rounded-xl bg-gradient-to-br from-sky-400 to-indigo-500 flex items-center justify-center text-white text-sm font-bold shadow-[0_4px_12px_rgba(56,189,248,0.25)]">
            {displayName.username.charAt(0).toUpperCase()}
          </div>
          <div class="absolute -bottom-0.5 -right-0.5 w-3 h-3 rounded-full bg-emerald-400 border-2 border-[var(--bg-secondary)] shadow-[0_0_6px_rgba(74,222,128,0.5)] transition-colors duration-300"></div>
        </div>
        <div class="flex-1 min-w-0">
          <div class="text-[0.8rem] font-semibold text-[var(--text-primary)] truncate">{displayName.username}</div>
          <div class="text-[0.65rem] text-[var(--text-muted)] capitalize">{displayName.role}</div>
          {#if auth.isImpersonating && auth.impersonator}
            <div class="text-[0.6rem] text-amber-500 dark:text-amber-400/80 truncate leading-tight mt-0.5">by {auth.impersonator.username}</div>
          {/if}
        </div>
        {#if auth.isImpersonating}
          <div class="shrink-0 px-1.5 py-0.5 rounded text-[0.5rem] font-bold tracking-wider uppercase bg-amber-100 dark:bg-amber-400/15 text-amber-600 dark:text-amber-400 border border-amber-200 dark:border-amber-400/20">IMP</div>
        {/if}
      </div>
      {#if usageData}
        <div class="mt-3 space-y-2 sidebar-divider border-t border-[var(--border-primary)] pt-3 px-1 transition-colors duration-300">
          <div>
            <div class="flex items-center justify-between text-[0.65rem] text-[var(--text-muted)] mb-1">
              <span>Disk</span>
              <span class="tabular-nums">{diskLabel}</span>
            </div>
            <div class="h-1.5 sidebar-progress bg-[var(--surface-1)] rounded-full overflow-hidden">
              <div class="h-full rounded-full bg-gradient-to-r from-sky-400 to-indigo-400 transition-all duration-500" style="width: {diskBarPct}%"></div>
            </div>
          </div>
          <div>
            <div class="flex items-center justify-between text-[0.65rem] text-[var(--text-muted)] mb-1">
              <span>Sites</span>
              <span class="tabular-nums">{sitesLabel}</span>
            </div>
            <div class="h-1.5 sidebar-progress bg-[var(--surface-1)] rounded-full overflow-hidden">
              <div class="h-full rounded-full bg-gradient-to-r from-indigo-400 to-purple-400 transition-all duration-500" style="width: {sitesUnlimited ? 0 : sitesPct}%"></div>
            </div>
          </div>
        </div>
      {/if}
      <div class="flex gap-2 mt-3">
        <button onclick={() => theme.toggle()}
          class="sidebar-theme-toggle flex-1 flex items-center justify-center gap-2 rounded-xl px-3 py-2.5 text-xs font-medium transition-all duration-200 border border-[var(--border-primary)] text-[var(--text-muted)] hover:text-[var(--text-primary)] hover:bg-white/[0.05] dark:hover:bg-white/[0.05] hover:border-[var(--border-primary)]"
          title={theme.isDark ? 'Switch to light mode' : 'Switch to dark mode'}>
          {#if theme.isDark}
            <Sun size={14} />
            <span>Light</span>
          {:else}
            <Moon size={14} />
            <span>Dark</span>
          {/if}
        </button>
        <button onclick={handleLogout}
          class="sidebar-signout flex-1 flex items-center justify-center gap-2 rounded-xl px-3 py-2.5 text-xs font-medium transition-all duration-200 border {auth.isImpersonating ? 'text-amber-500 dark:text-amber-400/80 hover:text-amber-500 dark:hover:text-amber-300 hover:bg-amber-50 dark:hover:bg-amber-500/10 border-transparent hover:border-amber-200 dark:hover:border-amber-500/15' : 'text-[var(--text-muted)] hover:text-red-500 dark:hover:text-red-400 hover:bg-red-50 dark:hover:bg-red-500/5 border-transparent hover:border-red-200 dark:hover:border-red-500/10'}">
          <LogOut size={14} />
          <span>{auth.isImpersonating ? 'Kembali' : 'Sign out'}</span>
        </button>
      </div>
    </div>
  {/if}
{/snippet}

{#if mobileOpen}
  <div class="fixed inset-0 z-40 bg-black/50 lg:hidden transition-opacity" role="button" onclick={onCloseMobile} onkeydown={() => {}} tabindex="-1"></div>
{/if}

<!-- Mobile sidebar -->
<aside class="fixed top-0 left-0 z-50 flex flex-col h-full w-72 sidebar-card bg-[var(--bg-secondary)]/95 dark:bg-[var(--bg-secondary)]/95 border-r border-[var(--border-primary)] transition-all duration-300 ease-out lg:hidden {mobileOpen ? 'translate-x-0' : '-translate-x-full'}">
  <div class="sidebar-header shrink-0 px-5 pt-5 pb-4 transition-colors duration-300">
    <div class="flex items-center gap-3">
      <div class="w-9 h-9 rounded-xl bg-gradient-to-br from-sky-400 to-indigo-500 flex items-center justify-center shadow-[0_4px_12px_rgba(56,189,248,0.25)]">
        <ShieldCheck size={18} class="text-white" />
      </div>
      <div>
        <div class="text-[0.95rem] font-bold tracking-tight text-[var(--text-primary)]">SigardaPanel</div>
        <div class="flex items-center gap-2 mt-0.5">
          <span class="text-[0.6rem] text-[var(--text-muted)] tracking-wide">VPS control plane</span>
          {#if licenseStore.tier !== 'free'}
            <ProBadge tier={licenseStore.tier} small />
          {:else}
            <span class="text-[0.55rem] px-1.5 py-0.5 rounded-full bg-gray-100 dark:bg-gray-800 text-gray-500 dark:text-gray-400 font-medium">Free</span>
          {/if}
        </div>
      </div>
    </div>
  </div>
  {@render navContent()}
  {@render userCard()}
</aside>

<!-- Desktop sidebar -->
<aside class="hidden lg:flex lg:flex-col fixed left-0 top-0 h-full w-72 sidebar-card bg-[var(--bg-secondary)]/95 dark:bg-[var(--bg-secondary)]/95 border-r border-[var(--border-primary)] transition-colors duration-300">
  <div class="sidebar-header shrink-0 px-5 pt-5 pb-4 transition-colors duration-300">
    <div class="flex items-center gap-3">
      <div class="w-9 h-9 rounded-xl bg-gradient-to-br from-sky-400 to-indigo-500 flex items-center justify-center shadow-[0_4px_12px_rgba(56,189,248,0.25)]">
        <ShieldCheck size={18} class="text-white" />
      </div>
      <div>
        <div class="text-[0.95rem] font-bold tracking-tight text-[var(--text-primary)]">SigardaPanel</div>
        <div class="flex items-center gap-2 mt-0.5">
          <span class="text-[0.6rem] text-[var(--text-muted)] tracking-wide">VPS control plane</span>
          {#if licenseStore.tier !== 'free'}
            <ProBadge tier={licenseStore.tier} small />
          {:else}
            <span class="text-[0.55rem] px-1.5 py-0.5 rounded-full bg-gray-100 dark:bg-gray-800 text-gray-500 dark:text-gray-400 font-medium">Free</span>
          {/if}
        </div>
      </div>
    </div>
  </div>
  {@render navContent()}
  {@render userCard()}
</aside>
