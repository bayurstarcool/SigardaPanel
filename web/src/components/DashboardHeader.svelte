<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { theme } from '$lib/theme.svelte';
  import { onMount, onDestroy } from 'svelte';
  import { Bell, Search, Settings, LogOut, ChevronDown, Shield, Menu, X, StopCircle, Sun, Moon } from 'lucide-svelte';

  let { mobileOpen, onToggleMobile, onSearchOpen }: { mobileOpen: boolean; onToggleMobile: () => void; onSearchOpen?: () => void } = $props();

  let showUserMenu = $state(false);
  let searchFocused = $state(false);
  let menuEl = $state<HTMLDivElement>();
  let unreadCount = $state(0);
  let pollInterval = $state<ReturnType<typeof setInterval> | null>(null);

  function handleLogout() {
    auth.logout();
    window.location.href = '/login';
  }

  async function fetchUnreadCount() {
    try {
      if (!auth.user) return;
      const data = await apiFetch(auth.token, 'GET', 'notifications/unread-count');
      unreadCount = data?.unread ?? 0;
    } catch {
      // silent — don't break header on error
    }
  }

  onMount(() => {
    const handler = (e: MouseEvent) => {
      if (menuEl && !menuEl.contains(e.target as Node)) showUserMenu = false;
    };
    document.addEventListener('mousedown', handler);

    fetchUnreadCount();
    pollInterval = setInterval(fetchUnreadCount, 30000);

    return () => {
      document.removeEventListener('mousedown', handler);
      if (pollInterval) clearInterval(pollInterval);
    };
  });

  onDestroy(() => {
    if (pollInterval) clearInterval(pollInterval);
  });
</script>

<svelte:head>
  <style>
    html.light .header-bar {
      background: rgba(255, 255, 255, 0.95);
      border-bottom: 1px solid rgba(0, 0, 0, 0.06);
    }
    html.light .header-search {
      background: rgba(0, 0, 0, 0.02);
      border: 1px solid rgba(0, 0, 0, 0.06);
    }
    html.light .header-search:focus-within {
      background: rgba(0, 0, 0, 0.03);
      border-color: rgba(56, 189, 248, 0.3);
      box-shadow: 0 0 0 3px rgba(56, 189, 248, 0.06);
    }
    html.light .header-btn { background: transparent; }
    html.light .header-btn:hover { background: rgba(0, 0, 0, 0.04); }
    html.light .header-dropdown {
      background: rgba(255, 255, 255, 0.95);
      border: 1px solid rgba(0, 0, 0, 0.08);
      box-shadow: 0 8px 30px rgba(0, 0, 0, 0.08);
    }
    html.light .header-impersonate {
      background: linear-gradient(90deg, rgba(251, 191, 36, 0.06), rgba(245, 158, 11, 0.03), transparent);
      border-bottom: 1px solid rgba(251, 191, 36, 0.15);
    }
  </style>
</svelte:head>

<header class="sticky top-0 z-40 lg:pl-72">
  {#if auth.isImpersonating && auth.impersonatingUser}
    <div class="header-impersonate relative dark:bg-gradient-to-r dark:from-amber-500/10 dark:via-yellow-500/5 dark:to-orange-500/10 transition-colors duration-300">
      <div class="flex items-center justify-between px-4 lg:px-6 py-2.5">
        <div class="flex items-center gap-2.5 text-sm">
          <div class="w-6 h-6 rounded-lg bg-amber-100 dark:bg-amber-400/15 flex items-center justify-center">
            <Shield size={12} class="text-amber-600 dark:text-amber-400" />
          </div>
          <span class="text-amber-600 dark:text-amber-300 font-medium">Impersonating</span>
          <span class="text-[var(--text-primary)] font-semibold">{auth.impersonatingUser.username}</span>
          <span class="text-[0.65rem] text-amber-600 dark:text-amber-400/80 bg-amber-100 dark:bg-amber-400/10 px-2 py-0.5 rounded-md font-medium">{auth.impersonatingUser.role}</span>
          {#if auth.impersonator}
            <span class="text-[0.7rem] text-[var(--text-muted)]"><span class="text-[var(--text-muted)] opacity-60">via</span> <span class="text-amber-600 dark:text-amber-400/80">{auth.impersonator.username}</span></span>
          {/if}
        </div>
        <button onclick={() => auth.stopImpersonation()}
          class="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium bg-amber-100 dark:bg-amber-400/10 text-amber-600 dark:text-amber-400 hover:bg-amber-200 dark:hover:bg-amber-400/20 transition-all whitespace-nowrap border-0 cursor-pointer">
          <StopCircle size={13} /> Kembali
        </button>
      </div>
    </div>
  {/if}

  <div class="header-bar relative bg-[var(--bg-secondary)]/95 dark:bg-[var(--bg-secondary)]/95 transition-colors duration-300">
    <div class="absolute bottom-0 left-1/2 -translate-x-1/2 w-1/2 h-px bg-gradient-to-r from-transparent via-sky-400/10 to-transparent dark:via-sky-400/10 light:via-sky-400/5"></div>
    <div class="flex items-center justify-between h-16 px-4 lg:px-6">
      <div class="flex items-center gap-4">
        <button onclick={onToggleMobile} class="lg:hidden p-2 rounded-xl header-btn transition text-[var(--text-muted)] hover:text-[var(--text-primary)]" aria-label="Toggle sidebar">
          {#if mobileOpen}<X size={18} />{:else}<Menu size={18} />{/if}
        </button>
        <button onclick={() => onSearchOpen?.()} class="hidden sm:flex items-center gap-2.5 header-search px-3.5 py-2 rounded-xl transition-all duration-300 w-52 lg:w-72 cursor-pointer text-left">
          <Search size={14} class="text-[var(--text-muted)]" />
          <span class="text-[0.82rem] text-[var(--text-muted)]">Search...</span>
          <kbd class="ml-auto hidden lg:inline-flex items-center px-1.5 py-0.5 text-[0.55rem] font-mono text-slate-500 bg-white/[0.05] border border-white/[0.08] rounded">⌘K</kbd>
        </button>
      </div>

      <div class="flex items-center gap-1.5">
        <button onclick={() => theme.toggle()}
          class="header-btn p-2.5 rounded-xl transition-all duration-200 text-[var(--text-muted)] hover:text-[var(--text-primary)]"
          title={theme.isDark ? 'Switch to light mode' : 'Switch to dark mode'}>
          {#if theme.isDark}<Sun size={17} />{:else}<Moon size={17} />{/if}
        </button>

        <button onclick={() => window.location.href = '/notifications'}
          class="relative header-btn p-2.5 rounded-xl transition text-[var(--text-muted)] hover:text-[var(--text-primary)] cursor-pointer">
          <Bell size={17} />
          {#if unreadCount > 0}
            <span class="absolute -top-0.5 -right-0.5 min-w-[1.125rem] h-[1.125rem] flex items-center justify-center px-1 rounded-full bg-sky-400 text-[0.55rem] font-bold text-slate-900 shadow-[0_0_8px_rgba(56,189,248,0.5)]">
              {unreadCount > 99 ? '99+' : unreadCount}
            </span>
          {/if}
        </button>
        <button onclick={() => window.location.href = '/settings'}
          class="header-btn p-2.5 rounded-xl transition text-[var(--text-muted)] hover:text-[var(--text-primary)] cursor-pointer">
          <Settings size={17} />
        </button>

        <div class="relative ml-1" bind:this={menuEl}>
          <button onclick={() => showUserMenu = !showUserMenu} class="flex items-center gap-2.5 pl-2 pr-2.5 py-1.5 rounded-xl header-btn transition-all group">
            <div class="relative">
              <div class="w-8 h-8 rounded-xl bg-gradient-to-br from-sky-400 to-indigo-500 flex items-center justify-center text-white text-xs font-bold shadow-[0_2px_8px_rgba(56,189,248,0.2)]">
                {(auth.user?.username || 'U').charAt(0).toUpperCase()}
              </div>
              <div class="absolute -bottom-0.5 -right-0.5 w-2.5 h-2.5 rounded-full bg-emerald-400 border-2 border-[var(--bg-secondary)] shadow-[0_0_4px_rgba(74,222,128,0.5)] transition-colors duration-300"></div>
            </div>
            <div class="hidden lg:block text-left">
              <div class="text-[0.8rem] font-semibold text-[var(--text-primary)]">{auth.user?.username || 'User'}</div>
              <div class="text-[0.6rem] text-[var(--text-muted)] capitalize">{auth.user?.role}</div>
            </div>
            <ChevronDown size={12} class="text-[var(--text-muted)] group-hover:text-[var(--text-secondary)] transition-colors {showUserMenu ? 'rotate-180' : ''} duration-200" />
          </button>

          {#if showUserMenu}
            <div class="fixed inset-0 z-10" role="button" onclick={() => showUserMenu = false} onkeydown={() => {}} tabindex="-1"></div>
            <div class="header-dropdown absolute right-0 top-full mt-2 w-52 z-20 bg-[var(--bg-secondary)]/95 dark:bg-[var(--bg-secondary)]/95 border border-[var(--border-primary)] rounded-xl shadow-2xl overflow-hidden transition-colors duration-300">
              <div class="px-4 py-3 border-b border-[var(--border-primary)]">
                <div class="text-sm font-semibold text-[var(--text-primary)]">{auth.user?.username}</div>
                <div class="text-xs text-[var(--text-muted)] mt-0.5">{auth.user?.email}</div>
              </div>
              <div class="p-1.5">
                <button onclick={() => { showUserMenu = false; window.location.href = '/profile'; }}
                  class="w-full text-left flex items-center gap-2.5 px-3 py-2.5 rounded-lg text-sm text-[var(--text-muted)] hover:text-[var(--text-primary)] hover:bg-black/[0.03] dark:hover:bg-white/[0.05] transition-all">
                  <Settings size={14} /> Profile Settings
                </button>
                <button onclick={handleLogout}
                  class="w-full text-left flex items-center gap-2.5 px-3 py-2.5 rounded-lg text-sm text-red-500 dark:text-red-400/80 hover:text-red-500 dark:hover:text-red-400 hover:bg-red-50 dark:hover:bg-red-500/10 transition-all">
                  <LogOut size={14} /> Sign Out
                </button>
              </div>
            </div>
          {/if}
        </div>
      </div>
    </div>
  </div>
</header>
