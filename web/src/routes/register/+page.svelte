<script lang="ts">
  import { onMount } from 'svelte';
  import { Shield, Eye, EyeOff, Loader2, Check, ArrowRight } from 'lucide-svelte';

  let name = $state('');
  let email = $state('');
  let password = $state('');
  let confirmPassword = $state('');
  let company = $state('');
  let plan = $state('community');
  let err = $state('');
  let busy = $state(false);
  let showPass = $state(false);
  let step = $state(1);

  async function submit(e: Event) {
    e.preventDefault();
    err = '';

    if (step === 1) {
      if (!name || !email || !password) {
        err = 'Please fill all fields';
        return;
      }
      if (password.length < 8) {
        err = 'Password must be at least 8 characters';
        return;
      }
      if (password !== confirmPassword) {
        err = 'Passwords do not match';
        return;
      }
      step = 2;
      return;
    }

    busy = true;
    try {
      const res = await fetch('/api/member/register', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name, email, password, company, plan })
      });
      const data = await res.json();
      if (!res.ok) throw new Error(data.error?.message || 'Registration failed');
      step = 3;
    } catch (e: any) {
      err = e.message;
    } finally {
      busy = false;
    }
  }
</script>

<svelte:head>
  <title>Register — SigardaPanel</title>
</svelte:head>

<div class="min-h-screen bg-[#060d1f] text-white flex items-center justify-center p-4">
  <!-- Background -->
  <div class="fixed inset-0 pointer-events-none overflow-hidden">
    <div class="absolute top-1/4 left-1/4 w-[400px] h-[400px] rounded-full bg-sky-500/[0.04] animate-float-slow"></div>
    <div class="absolute inset-0 opacity-[0.03]" style="background-image: radial-gradient(rgba(255,255,255,0.03) 1px, transparent 1px); background-size: 32px 32px;"></div>
  </div>

  <div class="relative z-10 w-full max-w-md">
    <!-- Logo -->
    <div class="text-center mb-8">
      <a href="/" class="inline-flex items-center gap-3">
        <div class="w-12 h-12 rounded-2xl bg-gradient-to-br from-sky-400 to-indigo-500 flex items-center justify-center shadow-[0_8px_32px_rgba(56,189,248,0.3)]">
          <Shield size={24} class="text-white" />
        </div>
      </a>
      <h1 class="text-2xl font-bold text-white mt-4">Create Account</h1>
      <p class="text-sm text-slate-500 mt-1">Get your API token and start managing servers</p>
    </div>

    <!-- Steps -->
    <div class="flex items-center justify-center gap-2 mb-8">
      {#each ['Account', 'Details', 'Complete'] as s, i}
        <div class="flex items-center gap-2">
          <div class="w-7 h-7 rounded-full flex items-center justify-center text-xs font-bold {step > i + 1 ? 'bg-emerald-500 text-white' : step === i + 1 ? 'bg-sky-400 text-[#040a14]' : 'bg-white/[0.06] text-slate-500'}">
            {#if step > i + 1}
              <Check size={14} />
            {:else}
              {i + 1}
            {/if}
          </div>
          <span class="text-xs {step === i + 1 ? 'text-white' : 'text-slate-500'} hidden sm:block">{s}</span>
          {#if i < 2}
            <div class="w-8 h-px bg-white/[0.08] mx-1"></div>
          {/if}
        </div>
      {/each}
    </div>

    <!-- Form Card -->
    <div class="relative group">
      <div class="absolute -inset-[1px] rounded-2xl bg-gradient-to-r from-sky-500/20 via-indigo-500/10 to-purple-500/20 opacity-0 group-hover:opacity-100 transition-opacity duration-500 blur-sm"></div>
      <div class="relative bg-[#0d1526]/95 border border-white/[0.06] rounded-2xl p-8">
        {#if step === 3}
          <!-- Success -->
          <div class="text-center py-8">
            <div class="w-16 h-16 rounded-2xl bg-emerald-500/10 flex items-center justify-center mx-auto mb-4">
              <Check size={32} class="text-emerald-400" />
            </div>
            <h2 class="text-xl font-bold text-white mb-2">Welcome aboard!</h2>
            <p class="text-sm text-slate-400 mb-6">Your account has been created. Check your email for the API token.</p>
            <a href="/login" class="inline-flex items-center gap-2 px-6 py-3 rounded-xl text-sm font-semibold bg-gradient-to-r from-sky-400 to-indigo-400 text-[#040a14] hover:shadow-[0_4px_24px_rgba(56,189,248,0.3)] transition-all">
              Sign In <ArrowRight size={16} />
            </a>
          </div>
        {:else}
          <form onsubmit={submit}>
            {#if err}
              <div class="mb-6 text-sm text-red-400 bg-red-500/10 border border-red-500/20 rounded-xl p-3">
                {err}
              </div>
            {/if}

            {#if step === 1}
              <div class="space-y-4">
                <div>
                  <label class="block text-xs font-medium text-slate-500 mb-2 uppercase tracking-wider">Full Name</label>
                  <input bind:value={name} class="w-full rounded-xl px-4 py-3 text-sm bg-white/[0.03] border border-white/[0.08] text-white placeholder-slate-500 outline-none focus:border-sky-400/40 focus:shadow-[0_0_0_3px_rgba(56,189,248,0.08)] transition-all" placeholder="John Doe" required />
                </div>
                <div>
                  <label class="block text-xs font-medium text-slate-500 mb-2 uppercase tracking-wider">Email</label>
                  <input type="email" bind:value={email} class="w-full rounded-xl px-4 py-3 text-sm bg-white/[0.03] border border-white/[0.08] text-white placeholder-slate-500 outline-none focus:border-sky-400/40 focus:shadow-[0_0_0_3px_rgba(56,189,248,0.08)] transition-all" placeholder="john@example.com" required />
                </div>
                <div>
                  <label class="block text-xs font-medium text-slate-500 mb-2 uppercase tracking-wider">Password</label>
                  <div class="relative">
                    <input type={showPass ? 'text' : 'password'} bind:value={password} class="w-full rounded-xl px-4 py-3 pr-12 text-sm bg-white/[0.03] border border-white/[0.08] text-white placeholder-slate-500 outline-none focus:border-sky-400/40 focus:shadow-[0_0_0_3px_rgba(56,189,248,0.08)] transition-all" placeholder="Min 8 characters" required />
                    <button type="button" onclick={() => showPass = !showPass} class="absolute right-3 top-1/2 -translate-y-1/2 p-1.5 rounded-lg text-slate-500 hover:text-white hover:bg-white/[0.05] transition-all" tabindex="-1">
                      {#if showPass}<EyeOff size={16} />{:else}<Eye size={16} />{/if}
                    </button>
                  </div>
                </div>
                <div>
                  <label class="block text-xs font-medium text-slate-500 mb-2 uppercase tracking-wider">Confirm Password</label>
                  <input type="password" bind:value={confirmPassword} class="w-full rounded-xl px-4 py-3 text-sm bg-white/[0.03] border border-white/[0.08] text-white placeholder-slate-500 outline-none focus:border-sky-400/40 focus:shadow-[0_0_0_3px_rgba(56,189,248,0.08)] transition-all" placeholder="Repeat password" required />
                </div>
              </div>
            {:else}
              <div class="space-y-4">
                <div>
                  <label class="block text-xs font-medium text-slate-500 mb-2 uppercase tracking-wider">Company (optional)</label>
                  <input bind:value={company} class="w-full rounded-xl px-4 py-3 text-sm bg-white/[0.03] border border-white/[0.08] text-white placeholder-slate-500 outline-none focus:border-sky-400/40 focus:shadow-[0_0_0_3px_rgba(56,189,248,0.08)] transition-all" placeholder="Your company name" />
                </div>
                <div>
                  <label class="block text-xs font-medium text-slate-500 mb-2 uppercase tracking-wider">Plan</label>
                  <div class="grid grid-cols-3 gap-2">
                    {#each [{ id: 'community', label: 'Community', price: 'Free' }, { id: 'premium', label: 'Premium', price: '$29/mo' }, { id: 'enterprise', label: 'Enterprise', price: '$99/mo' }] as p}
                      <button type="button" onclick={() => plan = p.id}
                        class="p-3 rounded-xl border text-center transition-all {plan === p.id ? 'border-sky-400/40 bg-sky-400/10' : 'border-white/[0.06] bg-white/[0.02] hover:border-white/[0.12]'}">
                        <div class="text-xs font-semibold {plan === p.id ? 'text-sky-400' : 'text-white'}">{p.label}</div>
                        <div class="text-[0.65rem] text-slate-500 mt-0.5">{p.price}</div>
                      </button>
                    {/each}
                  </div>
                </div>
              </div>
            {/if}

            <button type="submit" disabled={busy}
              class="mt-6 w-full py-3.5 rounded-xl text-sm font-semibold bg-gradient-to-r from-sky-400 to-indigo-400 text-[#040a14] hover:shadow-[0_4px_24px_rgba(56,189,248,0.3)] transition-all duration-300 disabled:opacity-50 disabled:cursor-not-allowed flex items-center justify-center gap-2">
              {#if busy}
                <Loader2 size={16} class="animate-spin" /> Creating account...
              {:else if step === 1}
                Continue <ArrowRight size={16} />
              {:else}
                Create Account
              {/if}
            </button>
          </form>
        {/if}
      </div>
    </div>

    <!-- Footer -->
    <div class="mt-8 text-center">
      <p class="text-xs text-slate-500">
        Already have an account? <a href="/login" class="text-sky-400/60 hover:text-sky-400 transition-colors">Sign in</a>
      </p>
    </div>
  </div>
</div>

<style>
  @keyframes float-slow { 0%, 100% { transform: translateY(0); } 50% { transform: translateY(-20px); } }
  .animate-float-slow { animation: float-slow 12s ease-in-out infinite; will-change: transform; }
</style>
