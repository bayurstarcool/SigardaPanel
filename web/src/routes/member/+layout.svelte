<script lang="ts">
  import { page } from '$app/stores';
  import { auth } from '$lib/auth.svelte';
  import { LayoutDashboard, CreditCard, Key } from 'lucide-svelte';
  import Logo from '$components/Logo.svelte';

  let { children } = $props();

  const navItems = [
    { label: 'Dashboard', href: '/member/dashboard', icon: LayoutDashboard },
    { label: 'Billing', href: '/member/billing', icon: CreditCard },
    { label: 'API Keys', href: '/member/api-keys', icon: Key },
  ];

  function isActive(href: string) {
    return $page.url.pathname === href;
  }
</script>

<div class="min-h-dvh bg-bg text-fg">
  <header class="sticky top-0 z-40 border-b border-line bg-bg">
    <div class="mx-auto flex h-14 max-w-7xl items-center justify-between gap-3 px-4 sm:px-6">
      <div class="flex min-w-0 items-center gap-3">
        <a href="/" aria-label="SigardaPanel home"><Logo size={26} withText /></a>
        <span class="h-5 w-px bg-line max-sm:hidden"></span>
        <span class="text-xs text-subtle max-sm:hidden">Member area</span>
      </div>
      <div class="flex items-center gap-2">
        <a href="/dashboard" class="btn btn-ghost btn-sm max-sm:hidden">Panel dashboard</a>
        <div class="flex items-center gap-2 rounded-lg border border-line bg-surface px-2 py-1">
          <span class="flex size-6 items-center justify-center rounded-full bg-accent/10 text-[11px] font-semibold text-accent">
            {auth.user?.username?.[0]?.toUpperCase() || 'U'}
          </span>
          <span class="text-xs text-muted max-sm:hidden">{auth.user?.username}</span>
        </div>
      </div>
    </div>
  </header>

  <div class="mx-auto flex max-w-7xl">
    <aside class="sticky top-14 hidden h-[calc(100dvh-3.5rem)] w-56 shrink-0 border-r border-line lg:block">
      <nav class="space-y-0.5 p-3">
        {#each navItems as item (item.href)}
          {@const active = isActive(item.href)}
          <a
            href={item.href}
            aria-current={active ? 'page' : undefined}
            class="flex items-center gap-2.5 rounded-lg px-3 py-2 text-sm transition-colors duration-150 {active ? 'bg-surface-2 font-medium text-fg' : 'text-muted hover:bg-surface-2 hover:text-fg'}"
          >
            <item.icon size={16} class={active ? 'text-accent' : ''} />
            <span>{item.label}</span>
          </a>
        {/each}
      </nav>
    </aside>

    <!-- Mobile nav -->
    <nav class="fixed inset-x-0 bottom-0 z-40 flex items-center justify-around border-t border-line bg-surface py-1.5 lg:hidden">
      {#each navItems as item (item.href)}
        {@const active = isActive(item.href)}
        <a href={item.href} aria-current={active ? 'page' : undefined} class="flex flex-col items-center gap-0.5 rounded-lg px-3 py-1.5 {active ? 'text-accent' : 'text-subtle'}">
          <item.icon size={18} />
          <span class="text-[11px]">{item.label}</span>
        </a>
      {/each}
    </nav>

    <main class="min-w-0 flex-1 p-4 pb-24 sm:p-6 lg:p-10 lg:pb-10">
      {@render children()}
    </main>
  </div>
</div>
