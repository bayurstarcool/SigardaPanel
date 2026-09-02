<script lang="ts">
  import { page } from '$app/stores';
  import { auth } from '$lib/auth.svelte';
  import { Shield, LayoutDashboard, CreditCard, Key, ArrowLeft, LogOut, User, Settings } from 'lucide-svelte';

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

<div class="min-h-screen bg-[#060d1f] text-white">
  <!-- Fixed Navbar -->
  <nav class="fixed top-0 left-0 right-0 z-50 bg-[#060d1f]/80 backdrop-blur-xl border-b border-white/[0.06]">
    <div class="flex items-center justify-between px-6 lg:px-12 py-4 max-w-7xl mx-auto">
      <div class="flex items-center gap-4">
        <a href="/" class="flex items-center gap-3">
          <div class="w-8 h-8 rounded-lg bg-gradient-to-br from-sky-400 to-indigo-500 flex items-center justify-center">
            <Shield size={16} class="text-white" />
          </div>
          <span class="text-sm font-bold tracking-tight hidden sm:block">SigardaPanel</span>
        </a>
        <div class="w-px h-5 bg-white/[0.08]"></div>
        <span class="text-xs text-slate-500">Member Area</span>
      </div>
      <div class="flex items-center gap-3">
        <a href="/dashboard" class="px-3 py-1.5 rounded-lg text-xs text-slate-400 hover:text-white hover:bg-white/[0.05] transition-all hidden sm:block">
          Panel Dashboard
        </a>
        <div class="flex items-center gap-2 px-3 py-1.5 rounded-lg bg-white/[0.04]">
          <div class="w-6 h-6 rounded-full bg-gradient-to-br from-sky-400 to-indigo-400 flex items-center justify-center text-[0.6rem] font-bold">
            {auth.user?.username?.[0]?.toUpperCase() || 'U'}
          </div>
          <span class="text-xs text-slate-300 hidden sm:block">{auth.user?.username}</span>
        </div>
      </div>
    </div>
  </nav>

  <div class="h-16"></div>

  <div class="max-w-7xl mx-auto flex">
    <!-- Sidebar -->
    <aside class="w-56 shrink-0 border-r border-white/[0.06] min-h-[calc(100vh-64px)] sticky top-16 hidden lg:block">
      <nav class="p-4 space-y-1">
        {#each navItems as item}
          <a href={item.href}
            class="flex items-center gap-3 px-3 py-2.5 rounded-xl text-sm transition-all duration-200 {isActive(item.href) ? 'bg-white/[0.08] text-white shadow-sm' : 'text-slate-400 hover:text-white hover:bg-white/[0.04]'}">
            <item.icon size={16} class="{isActive(item.href) ? 'text-sky-400' : ''}" />
            <span>{item.label}</span>
          </a>
        {/each}
      </nav>
    </aside>

    <!-- Mobile nav -->
    <div class="fixed bottom-0 left-0 right-0 z-40 bg-[#0a1122]/95 backdrop-blur-xl border-t border-white/[0.06] lg:hidden">
      <nav class="flex items-center justify-around py-2">
        {#each navItems as item}
          <a href={item.href} class="flex flex-col items-center gap-1 px-3 py-2 rounded-lg {isActive(item.href) ? 'text-sky-400' : 'text-slate-500'}">
            <item.icon size={18} />
            <span class="text-[0.6rem]">{item.label}</span>
          </a>
        {/each}
      </nav>
    </div>

    <!-- Main content -->
    <main class="flex-1 min-w-0 p-6 lg:p-10 pb-24 lg:pb-10">
      {@render children()}
    </main>
  </div>
</div>
