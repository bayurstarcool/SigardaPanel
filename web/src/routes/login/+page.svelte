<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { theme } from '$lib/theme.svelte';
  import { onMount } from 'svelte';
  import { Check, Eye, EyeOff, Loader2, Shield } from 'lucide-svelte';

  let u = $state('');
  let p = $state('');
  let rm = $state(false);
  let err = $state('');
  let busy = $state(false);
  let showPass = $state(false);
  let shake = $state(false);
  let mounted = $state(false);

  onMount(() => {
    if (auth.token) window.location.href = '/dashboard';
    mounted = true;
  });

  async function submit(e: Event) {
    e.preventDefault();
    err = '';
    busy = true;
    try {
      await auth.login(u, p, rm);
      window.location.href = '/dashboard';
    } catch (e: any) {
      err = e.message;
      shake = true;
      setTimeout(() => shake = false, 500);
    } finally {
      busy = false;
    }
  }
</script>

<svelte:head>
  <style>
    @keyframes float { 0%, 100% { transform: translateY(0) scale(1); } 50% { transform: translateY(-20px) scale(1.02); } }
    @keyframes float-reverse { 0%, 100% { transform: translateY(0) scale(1); } 50% { transform: translateY(20px) scale(1.02); } }
    @keyframes pulse-glow { 0%, 100% { opacity: 0.4; } 50% { opacity: 0.8; } }
    @keyframes fade-in { from { opacity: 0; transform: translateY(20px); } to { opacity: 1; transform: translateY(0); } }
    @keyframes shake { 0%, 100% { transform: translateX(0); } 10%, 30%, 50%, 70%, 90% { transform: translateX(-4px); } 20%, 40%, 60%, 80% { transform: translateX(4px); } }
    @keyframes gradient-shift { 0%, 100% { background-position: 0% 50%; } 50% { background-position: 100% 50%; } }
    @keyframes particle-float { 0% { transform: translateY(100vh) rotate(0deg); opacity: 0; } 10% { opacity: 1; } 90% { opacity: 1; } 100% { transform: translateY(-100vh) rotate(720deg); opacity: 0; } }
    @keyframes orbit { 0% { transform: rotate(0deg) translateX(120px) rotate(0deg); } 100% { transform: rotate(360deg) translateX(120px) rotate(-360deg); } }
    @keyframes glow-pulse { 0%, 100% { box-shadow: 0 0 20px rgba(56,189,248,0.3), 0 0 60px rgba(56,189,248,0.1); } 50% { box-shadow: 0 0 30px rgba(56,189,248,0.5), 0 0 80px rgba(56,189,248,0.2); } }

    .login-shake { animation: shake 0.4s ease-in-out; }
    .login-fade { animation: fade-in 0.8s cubic-bezier(0.16, 1, 0.3, 1) both; }
    .login-fade-delay { animation: fade-in 0.8s cubic-bezier(0.16, 1, 0.3, 1) 0.2s both; }
    .login-fade-delay2 { animation: fade-in 0.8s cubic-bezier(0.16, 1, 0.3, 1) 0.4s both; }
    .glow-orb { animation: float 12s ease-in-out infinite; will-change: transform; }
    .particle { animation: particle-float linear infinite; will-change: transform, opacity; }
    .orbit-ring { animation: orbit 20s linear infinite; }
    .logo-glow { animation: glow-pulse 3s ease-in-out infinite; }

    html.light .login-card {
      background: rgba(255, 255, 255, 0.96);
      border: 1px solid rgba(0, 0, 0, 0.06);
      box-shadow: 0 4px 24px rgba(0, 0, 0, 0.04), 0 1px 3px rgba(0, 0, 0, 0.02);
    }
    html.light .login-input {
      background: rgba(0, 0, 0, 0.02);
      border: 1px solid rgba(0, 0, 0, 0.08);
    }
    html.light .login-input:focus {
      background: rgba(0, 0, 0, 0.03);
      border-color: rgba(56, 189, 248, 0.4);
      box-shadow: 0 0 0 3px rgba(56, 189, 248, 0.08);
    }
    html.light .login-orb-1 { background: radial-gradient(circle, rgba(125, 211, 252, 0.12), rgba(165, 180, 252, 0.06), transparent 70%); }
    html.light .login-grid { opacity: 0.04; }
  </style>
</svelte:head>

<div class="relative min-h-screen bg-[var(--bg-primary)] flex items-center justify-center p-4 overflow-hidden transition-colors duration-300">
  <!-- Animated Background -->
  <div class="absolute inset-0 overflow-hidden pointer-events-none">
    <!-- Floating orbs -->
    <div class="glow-orb absolute top-1/3 left-1/4 w-[400px] h-[400px] rounded-full login-orb-1 dark:bg-sky-500/[0.06]"></div>

    <!-- Particles -->
    {#if mounted}
      {#each Array(5) as _, i}
        <div class="particle absolute w-1 h-1 rounded-full dark:bg-white/20"
          style="left: {Math.random() * 100}%; animation-duration: {15 + Math.random() * 20}s; animation-delay: {Math.random() * 10}s;"></div>
      {/each}
    {/if}

    <!-- Grid pattern -->
    <div class="absolute inset-0 login-grid dark:opacity-[0.03]" style="background-image: linear-gradient(rgba(128,128,128,0.15) 1px, transparent 1px), linear-gradient(90deg, rgba(128,128,128,0.15) 1px, transparent 1px); background-size: 60px 60px;"></div>

    <!-- Orbit rings -->
    <div class="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 w-[300px] h-[300px] rounded-full border dark:border-white/[0.03] opacity-50"></div>
    <div class="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 w-[500px] h-[500px] rounded-full border dark:border-white/[0.02] opacity-30"></div>
  </div>

  <div class="relative z-10 w-full max-w-md">
    <!-- Logo -->
    <div class="text-center mb-10 login-fade">
      <div class="relative inline-flex items-center justify-center mb-6">
        <div class="absolute inset-0 rounded-2xl bg-gradient-to-r from-sky-400/20 to-indigo-400/20 blur-xl pulse-glow"></div>
        <div class="relative w-16 h-16 rounded-2xl bg-gradient-to-br from-sky-400 to-indigo-500 flex items-center justify-center logo-glow">
          <Shield size={32} class="text-white" />
        </div>
        <!-- Orbiting dot -->
        <div class="absolute w-2 h-2 rounded-full bg-sky-400 orbit-ring" style="animation-duration: 8s;"></div>
      </div>
      <h1 class="text-4xl font-bold tracking-tight bg-gradient-to-r from-white via-sky-100 to-indigo-200 bg-clip-text text-transparent" style="background-size: 200% 200%; animation: gradient-shift 4s ease infinite;">
        SigardaPanel
      </h1>
      <p class="text-sm text-[var(--text-muted)] mt-2 tracking-wide">VPS control plane</p>
    </div>

    <!-- Login Card -->
    <div class="login-fade-delay">
      <div class="relative group">
        <div class="absolute -inset-[1px] rounded-2xl bg-gradient-to-r from-sky-500/20 via-indigo-500/10 to-purple-500/20 opacity-0 group-hover:opacity-100 transition-opacity duration-500 blur-sm"></div>
        <div class="relative login-card dark:bg-[var(--bg-secondary)]/95 dark:border-[var(--border-primary)] rounded-2xl p-8 transition-colors duration-300">
          <form onsubmit={submit} class:login-shake={shake}>
            <h2 class="text-xl font-semibold text-[var(--text-primary)] mb-1">Welcome back</h2>
            <p class="text-sm text-[var(--text-muted)] mb-8">Sign in to your control panel</p>

            {#if err}
              <div class="mb-6 text-sm text-red-500 bg-red-50 dark:bg-red-500/10 border border-red-200 dark:border-red-500/20 rounded-xl p-3.5 flex items-center gap-2.5 transition-colors">
                <div class="w-5 h-5 rounded-full bg-red-100 dark:bg-red-500/20 flex items-center justify-center shrink-0">
                  <span class="text-red-500 text-xs font-bold">!</span>
                </div>
                {err}
              </div>
            {/if}

            <div class="space-y-5">
              <div>
                <label for="login-user" class="block text-xs font-medium text-[var(--text-muted)] mb-2 uppercase tracking-wider">Username</label>
                <input id="login-user" bind:value={u} 
                  class="login-input dark:bg-white/[0.03] dark:border-[var(--border-primary)] w-full rounded-xl px-4 py-3 text-sm text-[var(--text-primary)] placeholder-[var(--text-muted)] outline-none transition-all duration-200 focus:border-sky-400/40 focus:shadow-[0_0_0_3px_rgba(56,189,248,0.08)]" 
                  placeholder="admin" required />
              </div>

              <div>
                <label for="login-pass" class="block text-xs font-medium text-[var(--text-muted)] mb-2 uppercase tracking-wider">Password</label>
                <div class="relative">
                  <input id="login-pass" 
                    type={showPass ? 'text' : 'password'} 
                    bind:value={p} 
                    class="login-input dark:bg-white/[0.03] dark:border-[var(--border-primary)] w-full rounded-xl px-4 py-3 pr-12 text-sm text-[var(--text-primary)] placeholder-[var(--text-muted)] outline-none transition-all duration-200 focus:border-sky-400/40 focus:shadow-[0_0_0_3px_rgba(56,189,248,0.08)]" 
                    placeholder="••••••••" required />
                  <button type="button" onclick={() => showPass = !showPass}
                    class="absolute right-3 top-1/2 -translate-y-1/2 p-1.5 rounded-lg text-[var(--text-muted)] hover:text-[var(--text-secondary)] hover:bg-black/[0.05] dark:hover:bg-white/[0.05] transition-all duration-150"
                    tabindex="-1">
                    {#if showPass}<EyeOff size={16} />{:else}<Eye size={16} />{/if}
                  </button>
                </div>
              </div>
            </div>

            <label for="remember-me" class="flex items-center gap-3 mt-6 cursor-pointer group/select">
              <div class="relative flex items-center justify-center">
                <input type="checkbox" bind:checked={rm} class="peer sr-only" id="remember-me" />
                <div class="w-[18px] h-[18px] rounded-md border-2 flex items-center justify-center transition-all duration-200 peer-checked:bg-sky-400 peer-checked:border-sky-400 peer-checked:shadow-[0_0_12px_rgba(56,189,248,0.4)] border-[var(--border-primary)] bg-white dark:bg-white/[0.03] group-hover/select:border-[var(--border-hover)] peer-checked:group-hover/select:border-sky-400">
                  {#if rm}<Check size={11} class="text-[#040a14]" stroke-width={3} />{/if}
                </div>
              </div>
              <span class="text-sm text-[var(--text-muted)] group-hover/select:text-[var(--text-secondary)] transition-colors">Remember me</span>
            </label>

            <button type="submit" disabled={busy}
              class="mt-8 w-full relative overflow-hidden group/btn inline-flex items-center justify-center gap-2 px-5 py-3.5 rounded-xl text-sm font-semibold text-[#040a14] transition-all duration-300 cursor-pointer border-0 disabled:opacity-50 disabled:cursor-not-allowed hover:scale-[1.02] hover:shadow-[0_8px_30px_rgba(56,189,248,0.4)]">
              <div class="absolute inset-0 bg-gradient-to-r from-sky-400 to-indigo-400 group-hover/btn:from-sky-300 group-hover/btn:to-indigo-300 transition-all duration-300"></div>
              <div class="absolute inset-0 opacity-0 group-hover/btn:opacity-100 transition-opacity duration-300 bg-[radial-gradient(circle_at_50%_50%,rgba(255,255,255,0.3),transparent_70%)]"></div>
              <span class="relative z-10 flex items-center gap-2">
                {#if busy}<Loader2 size={16} class="animate-spin" />Signing in...{:else}Sign in{/if}
              </span>
            </button>
          </form>
        </div>
      </div>
    </div>

    <!-- Footer -->
    <div class="login-fade-delay2 mt-8 text-center">
      <p class="text-xs text-[var(--text-muted)]">
        &&copy; {new Date().getFullYear()} <a href="https://sigarda.com" target="_blank" class="text-sky-500 dark:text-sky-400/60 hover:text-sky-400 transition-colors">sigarda.com</a> &middot; <a href="https://github.com/bayurstarcool/SigardaPanel/issues" target="_blank" class="text-sky-500 dark:text-sky-400/60 hover:text-sky-400 transition-colors">Support</a>&copy; {new Date().getFullYear()} <a href="https://sigarda.com" target="_blank" class="text-sky-500 dark:text-sky-400/60 hover:text-sky-400 transition-colors">sigarda.com</a> &middot; <a href="https://github.com/bayurstarcool/SigardaPanel/issues" target="_blank" class="text-sky-500 dark:text-sky-400/60 hover:text-sky-400 transition-colors">Support</a>
        <span class="mx-2 text-[var(--text-muted)]/40">&middot;</span>
        VPS control plane
      </p>
    </div>
  </div>
</div>
