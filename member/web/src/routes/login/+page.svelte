<script>
  import { Shield, Eye, EyeOff, Loader2 } from 'lucide-svelte';
  import { goto } from '$app/navigation';

  let email = $state('');
  let password = $state('');
  let err = $state('');
  let busy = $state(false);
  let showPass = $state(false);

  async function submit(e) {
    e.preventDefault();
    err = '';
    busy = true;
    try {
      const res = await fetch('/api/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email, password })
      });
      const data = await res.json();
      if (!res.ok) throw new Error(data.detail || 'Login failed');
      localStorage.setItem('member_token', data.data.token);
      localStorage.setItem('member', JSON.stringify(data.data.member));
      goto('/dashboard');
    } catch (e) {
      err = e.message;
    } finally {
      busy = false;
    }
  }
</script>

<div class="min-h-screen bg-[#060d1f] text-white flex items-center justify-center p-4">
  <div class="max-w-md w-full">
    <div class="text-center mb-8">
      <a href="/" class="inline-flex items-center gap-2">
        <div class="w-10 h-10 rounded-xl bg-gradient-to-br from-sky-400 to-indigo-500 flex items-center justify-center">
          <Shield size={20} class="text-white" />
        </div>
        <span class="font-bold">SigardaPanel</span>
      </a>
      <h1 class="text-2xl font-bold mt-4">Welcome back</h1>
      <p class="text-sm text-slate-500">Sign in to your member account</p>
    </div>

    <div class="bg-[#0d1526] border border-white/[0.06] rounded-2xl p-8">
      <form onsubmit={submit}>
        {#if err}
          <div class="mb-4 text-sm text-red-400 bg-red-500/10 border border-red-500/20 rounded-xl p-3">{err}</div>
        {/if}
        <div class="space-y-4">
          <div>
            <label class="block text-xs text-slate-500 mb-1.5 uppercase tracking-wider">Email</label>
            <input type="email" bind:value={email} class="w-full rounded-xl px-4 py-3 text-sm bg-white/[0.03] border border-white/[0.08] text-white placeholder-slate-500 outline-none focus:border-sky-400/40 transition-all" placeholder="john@example.com" required />
          </div>
          <div>
            <label class="block text-xs text-slate-500 mb-1.5 uppercase tracking-wider">Password</label>
            <div class="relative">
              <input type={showPass ? 'text' : 'password'} bind:value={password} class="w-full rounded-xl px-4 py-3 pr-12 text-sm bg-white/[0.03] border border-white/[0.08] text-white placeholder-slate-500 outline-none focus:border-sky-400/40 transition-all" placeholder="••••••••" required />
              <button type="button" onclick={() => showPass = !showPass} class="absolute right-3 top-1/2 -translate-y-1/2 text-slate-500 hover:text-white" tabindex="-1">
                {#if showPass}<EyeOff size={16} />{:else}<Eye size={16} />{/if}
              </button>
            </div>
          </div>
        </div>
        <button type="submit" disabled={busy}
          class="mt-6 w-full py-3 rounded-xl text-sm font-semibold bg-gradient-to-r from-sky-400 to-indigo-400 text-[#040a14] hover:shadow-[0_4px_24px_rgba(56,189,248,0.3)] transition-all disabled:opacity-50 flex items-center justify-center gap-2">
          {#if busy}<Loader2 size={16} class="animate-spin" />Signing in...{:else}Sign In{/if}
        </button>
      </form>
    </div>

    <p class="text-center text-xs text-slate-500 mt-6">
      Don't have an account? <a href="/register" class="text-sky-400/60 hover:text-sky-400">Create one</a>
    </p>
  </div>
</div>
