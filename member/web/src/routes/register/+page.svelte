<script>
  import { Shield, Eye, EyeOff, Loader2, ArrowRight, Check } from 'lucide-svelte';

  let name = $state('');
  let email = $state('');
  let password = $state('');
  let company = $state('');
  let plan = $state('community');
  let err = $state('');
  let busy = $state(false);
  let showPass = $state(false);
  let step = $state(1);
  let licenseKey = $state('');

  async function submit(e) {
    e.preventDefault();
    err = '';

    if (step === 1) {
      if (!name || !email || !password) { err = 'Fill all fields'; return; }
      if (password.length < 8) { err = 'Password min 8 chars'; return; }
      step = 2;
      return;
    }

    busy = true;
    try {
      const res = await fetch('/api/register', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name, email, password, company, plan })
      });
      const data = await res.json();
      if (!res.ok) throw new Error(data.detail || 'Registration failed');
      licenseKey = data.data?.license_key || '';
      step = 3;
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
      <h1 class="text-2xl font-bold mt-4">Create Account</h1>
      <p class="text-sm text-slate-500">Get your license key and API token</p>
    </div>

    <div class="bg-[#0d1526] border border-white/[0.06] rounded-2xl p-8">
      {#if step === 3}
        <div class="text-center py-6">
          <div class="w-14 h-14 rounded-2xl bg-emerald-500/10 flex items-center justify-center mx-auto mb-4">
            <Check size={28} class="text-emerald-400" />
          </div>
          <h2 class="text-lg font-bold mb-2">Welcome!</h2>
          <p class="text-sm text-slate-400 mb-4">Your account has been created.</p>
          {#if licenseKey}
            <div class="p-3 rounded-xl bg-black/20 border border-white/[0.06] mb-4">
              <p class="text-[0.65rem] text-slate-500 mb-1">Your License Key</p>
              <code class="text-sm font-mono text-sky-300">{licenseKey}</code>
            </div>
          {/if}
          <a href="/login" class="inline-flex items-center gap-2 px-6 py-2.5 rounded-xl text-sm font-semibold bg-gradient-to-r from-sky-400 to-indigo-400 text-[#040a14]">
            Sign In <ArrowRight size={14} />
          </a>
        </div>
      {:else}
        <form onsubmit={submit}>
          {#if err}
            <div class="mb-4 text-sm text-red-400 bg-red-500/10 border border-red-500/20 rounded-xl p-3">{err}</div>
          {/if}

          {#if step === 1}
            <div class="space-y-4">
              <div>
                <label class="block text-xs text-slate-500 mb-1.5 uppercase tracking-wider">Name</label>
                <input bind:value={name} class="w-full rounded-xl px-4 py-3 text-sm bg-white/[0.03] border border-white/[0.08] text-white placeholder-slate-500 outline-none focus:border-sky-400/40 transition-all" placeholder="John Doe" required />
              </div>
              <div>
                <label class="block text-xs text-slate-500 mb-1.5 uppercase tracking-wider">Email</label>
                <input type="email" bind:value={email} class="w-full rounded-xl px-4 py-3 text-sm bg-white/[0.03] border border-white/[0.08] text-white placeholder-slate-500 outline-none focus:border-sky-400/40 transition-all" placeholder="john@example.com" required />
              </div>
              <div>
                <label class="block text-xs text-slate-500 mb-1.5 uppercase tracking-wider">Password</label>
                <div class="relative">
                  <input type={showPass ? 'text' : 'password'} bind:value={password} class="w-full rounded-xl px-4 py-3 pr-12 text-sm bg-white/[0.03] border border-white/[0.08] text-white placeholder-slate-500 outline-none focus:border-sky-400/40 transition-all" placeholder="Min 8 characters" required />
                  <button type="button" onclick={() => showPass = !showPass} class="absolute right-3 top-1/2 -translate-y-1/2 text-slate-500 hover:text-white" tabindex="-1">
                    {#if showPass}<EyeOff size={16} />{:else}<Eye size={16} />{/if}
                  </button>
                </div>
              </div>
            </div>
          {:else}
            <div class="space-y-4">
              <div>
                <label class="block text-xs text-slate-500 mb-1.5 uppercase tracking-wider">Company (optional)</label>
                <input bind:value={company} class="w-full rounded-xl px-4 py-3 text-sm bg-white/[0.03] border border-white/[0.08] text-white placeholder-slate-500 outline-none focus:border-sky-400/40 transition-all" placeholder="Your company" />
              </div>
              <div>
                <label class="block text-xs text-slate-500 mb-1.5 uppercase tracking-wider">Plan</label>
                <div class="grid grid-cols-3 gap-2">
                  {#each [{ id: 'community', label: 'Community', price: 'Free' }, { id: 'premium', label: 'Premium', price: '$29/mo' }, { id: 'enterprise', label: 'Enterprise', price: '$99/mo' }] as p}
                    <button type="button" onclick={() => plan = p.id}
                      class="p-3 rounded-xl border text-center transition-all {plan === p.id ? 'border-sky-400/40 bg-sky-400/10' : 'border-white/[0.06] bg-white/[0.02] hover:border-white/[0.12]'}">
                      <div class="text-xs font-semibold {plan === p.id ? 'text-sky-400' : 'text-white'}">{p.label}</div>
                      <div class="text-[0.6rem] text-slate-500 mt-0.5">{p.price}</div>
                    </button>
                  {/each}
                </div>
              </div>
            </div>
          {/if}

          <button type="submit" disabled={busy}
            class="mt-6 w-full py-3 rounded-xl text-sm font-semibold bg-gradient-to-r from-sky-400 to-indigo-400 text-[#040a14] hover:shadow-[0_4px_24px_rgba(56,189,248,0.3)] transition-all disabled:opacity-50 flex items-center justify-center gap-2">
            {#if busy}<Loader2 size={16} class="animate-spin" />{:else if step === 1}Continue <ArrowRight size={14} />{:else}Create Account{/if}
          </button>
        </form>
      {/if}
    </div>

    <p class="text-center text-xs text-slate-500 mt-6">
      Already have an account? <a href="/login" class="text-sky-400/60 hover:text-sky-400">Sign in</a>
    </p>
  </div>
</div>
