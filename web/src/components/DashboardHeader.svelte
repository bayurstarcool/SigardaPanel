<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { theme } from '$lib/theme.svelte';
  import { onMount } from 'svelte';
  import { Bell, Search, Settings, LogOut, ChevronDown, Menu, X, Sun, Moon, User, Undo2 } from 'lucide-svelte';

  let { mobileOpen, onToggleMobile, onSearchOpen }: { mobileOpen: boolean; onToggleMobile: () => void; onSearchOpen?: () => void } = $props();

  let showUserMenu = $state(false);
  let menuEl = $state<HTMLDivElement>();
  let unreadCount = $state(0);

  function handleLogout() {
    auth.logout();
    window.location.href = '/login';
  }

  async function fetchUnreadCount() {
    // Skip polling while the tab is in the background.
    if (!auth.user || document.hidden) return;
    try {
      const data = await apiFetch(auth.token, 'GET', 'notifications/unread-count');
      unreadCount = data?.unread ?? 0;
    } catch {
      // silent — the header must never break on a failed poll
    }
  }

  onMount(() => {
    const onDown = (e: MouseEvent) => {
      if (menuEl && !menuEl.contains(e.target as Node)) showUserMenu = false;
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') showUserMenu = false;
    };
    const onVisible = () => {
      if (!document.hidden) fetchUnreadCount();
    };
    document.addEventListener('mousedown', onDown);
    document.addEventListener('keydown', onKey);
    document.addEventListener('visibilitychange', onVisible);
    fetchUnreadCount();
    const timer = setInterval(fetchUnreadCount, 30000);
    return () => {
      document.removeEventListener('mousedown', onDown);
      document.removeEventListener('keydown', onKey);
      document.removeEventListener('visibilitychange', onVisible);
      clearInterval(timer);
    };
  });

  const initial = $derived((auth.user?.username || 'U').charAt(0).toUpperCase());
</script>

<header class="sticky top-0 z-20 lg:pl-60">
  {#if auth.isImpersonating && auth.impersonatingUser}
    <div class="flex items-center justify-between gap-3 border-b border-warning/20 bg-warning/10 px-4 py-2 text-[13px] lg:px-8">
      <div class="flex min-w-0 items-center gap-2 text-warning">
        <span class="font-medium">Viewing as</span>
        <span class="truncate font-semibold text-fg">{auth.impersonatingUser.username}</span>
        <span class="badge badge-warning">{auth.impersonatingUser.role}</span>
        {#if auth.impersonator}
          <span class="hidden text-subtle sm:inline">via {auth.impersonator.username}</span>
        {/if}
      </div>
      <button onclick={() => auth.stopImpersonation()} class="btn btn-xs btn-secondary">
        <Undo2 size={13} /> Kembali
      </button>
    </div>
  {/if}

  <div class="flex h-14 items-center justify-between gap-3 border-b border-line bg-bg px-4 lg:px-8">
    <div class="flex min-w-0 items-center gap-2">
      <button onclick={onToggleMobile} class="btn btn-ghost btn-icon btn-sm lg:hidden" aria-label="Toggle navigation">
        {#if mobileOpen}<X size={18} />{:else}<Menu size={18} />{/if}
      </button>
      <button
        onclick={() => onSearchOpen?.()}
        class="flex h-8 w-56 cursor-pointer items-center gap-2 rounded-lg border border-line bg-surface px-2.5 text-left text-[13px] text-subtle transition-colors hover:border-line-strong max-sm:w-auto lg:w-72"
      >
        <Search size={14} />
        <span class="max-sm:hidden">Search…</span>
        <span class="kbd ml-auto max-sm:hidden">⌘K</span>
      </button>
    </div>

    <div class="flex items-center gap-1">
      <button
        onclick={() => theme.toggle()}
        class="btn btn-ghost btn-icon btn-sm"
        aria-label={theme.isDark ? 'Switch to light mode' : 'Switch to dark mode'}
        title={theme.isDark ? 'Light mode' : 'Dark mode'}
      >
        {#if theme.isDark}<Sun size={16} />{:else}<Moon size={16} />{/if}
      </button>
      <a href="/notifications" class="btn btn-ghost btn-icon btn-sm relative" aria-label="Notifications" title="Notifications">
        <Bell size={16} />
        {#if unreadCount > 0}
          <span class="absolute top-1 right-1 flex h-4 min-w-4 items-center justify-center rounded-full bg-accent px-1 text-[10px] font-semibold text-accent-fg tabular-nums">
            {unreadCount > 99 ? '99+' : unreadCount}
          </span>
        {/if}
      </a>
      <a href="/settings" class="btn btn-ghost btn-icon btn-sm" aria-label="Settings" title="Settings">
        <Settings size={16} />
      </a>

      <div class="relative ml-1" bind:this={menuEl}>
        <button
          onclick={() => (showUserMenu = !showUserMenu)}
          class="flex h-8 cursor-pointer items-center gap-2 rounded-lg pr-1.5 pl-1 transition-colors hover:bg-surface-2"
          aria-haspopup="menu"
          aria-expanded={showUserMenu}
        >
          <span class="flex size-6 items-center justify-center rounded-md bg-accent/15 text-xs font-semibold text-accent">{initial}</span>
          <span class="hidden text-[13px] font-medium text-fg md:inline">{auth.user?.username || 'User'}</span>
          <ChevronDown size={14} class="text-subtle" />
        </button>

        {#if showUserMenu}
          <div class="popover animate-pop-in absolute top-full right-0 z-30 mt-1.5 w-56 overflow-hidden" role="menu">
            <div class="border-b border-line px-3.5 py-3">
              <div class="truncate text-sm font-medium text-fg">{auth.user?.username}</div>
              <div class="mt-0.5 truncate text-xs text-subtle">{auth.user?.email}</div>
              <div class="mt-2"><span class="badge capitalize">{auth.user?.role?.replace('_', ' ')}</span></div>
            </div>
            <div class="p-1">
              <a href="/profile" onclick={() => (showUserMenu = false)} role="menuitem"
                class="flex h-8 items-center gap-2.5 rounded-md px-2.5 text-[13px] text-muted transition-colors hover:bg-surface-2 hover:text-fg">
                <User size={14} /> Profile
              </a>
              <a href="/settings" onclick={() => (showUserMenu = false)} role="menuitem"
                class="flex h-8 items-center gap-2.5 rounded-md px-2.5 text-[13px] text-muted transition-colors hover:bg-surface-2 hover:text-fg">
                <Settings size={14} /> Settings
              </a>
              <button onclick={handleLogout} role="menuitem"
                class="flex h-8 w-full cursor-pointer items-center gap-2.5 rounded-md px-2.5 text-[13px] text-danger transition-colors hover:bg-danger/10">
                <LogOut size={14} /> Sign out
              </button>
            </div>
          </div>
        {/if}
      </div>
    </div>
  </div>
</header>
