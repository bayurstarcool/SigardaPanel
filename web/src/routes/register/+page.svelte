<script lang="ts">
  import { Eye, EyeOff, Loader2, Check, ArrowRight, AlertCircle } from 'lucide-svelte';
  import Logo from '$components/Logo.svelte';

  const plans = [
    { id: 'community', label: 'Community', price: 'Free' },
    { id: 'premium', label: 'Premium', price: '$29/mo' },
    { id: 'enterprise', label: 'Enterprise', price: '$99/mo' },
  ];

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

<div class="flex min-h-dvh flex-col items-center justify-center bg-bg px-4 py-10">
  <div class="w-full max-w-md animate-fade-in">
    <div class="mb-8 flex flex-col items-center text-center">
      <a href="/" aria-label="SigardaPanel home"><Logo size={36} /></a>
      <h1 class="mt-5 text-xl font-semibold tracking-tight text-fg">Create account</h1>
      <p class="mt-1 text-sm text-muted">Get your API token and start managing servers</p>
    </div>

    <!-- Steps -->
    <ol class="mb-6 flex items-center justify-center gap-2">
      {#each ['Account', 'Details', 'Complete'] as s, i (s)}
        <li class="flex items-center gap-2">
          <span
            class="flex size-6 items-center justify-center rounded-full text-xs font-semibold tabular-nums
              {step > i + 1 ? 'bg-success/10 text-success' : step === i + 1 ? 'bg-accent text-accent-fg' : 'bg-surface-2 text-subtle'}"
          >
            {#if step > i + 1}<Check size={12} />{:else}{i + 1}{/if}
          </span>
          <span class="text-xs max-sm:hidden {step === i + 1 ? 'text-fg' : 'text-subtle'}">{s}</span>
          {#if i < 2}<span class="mx-1 h-px w-8 bg-line"></span>{/if}
        </li>
      {/each}
    </ol>

    <div class="card p-6 shadow-sm">
      {#if step === 3}
        <div class="py-6 text-center">
          <div class="mx-auto mb-4 flex size-11 items-center justify-center rounded-xl bg-success/10 text-success">
            <Check size={20} />
          </div>
          <h2 class="text-base font-semibold text-fg">Welcome aboard!</h2>
          <p class="mt-1 text-[13px] text-muted">Your account has been created. Check your email for the API token.</p>
          <a href="/login" class="btn btn-primary mt-6">Sign In <ArrowRight size={16} /></a>
        </div>
      {:else}
        <form onsubmit={submit}>
          {#if err}
            <div class="mb-5 flex items-start gap-2 rounded-lg bg-danger/10 px-3 py-2.5 text-[13px] text-danger ring-1 ring-danger/20 ring-inset" role="alert">
              <AlertCircle size={16} class="mt-px shrink-0" />
              <span>{err}</span>
            </div>
          {/if}

          {#if step === 1}
            <div class="space-y-4">
              <div>
                <label for="reg-name" class="label">Full name</label>
                <input id="reg-name" bind:value={name} class="input" placeholder="John Doe" autocomplete="name" required />
              </div>
              <div>
                <label for="reg-email" class="label">Email</label>
                <input id="reg-email" type="email" bind:value={email} class="input" placeholder="john@example.com" autocomplete="email" required />
              </div>
              <div>
                <label for="reg-pass" class="label">Password</label>
                <div class="relative">
                  <input id="reg-pass" type={showPass ? 'text' : 'password'} bind:value={password} class="input pr-10" placeholder="Min 8 characters" autocomplete="new-password" required />
                  <button
                    type="button"
                    onclick={() => (showPass = !showPass)}
                    class="btn btn-ghost btn-xs btn-icon absolute top-1/2 right-1 -translate-y-1/2"
                    aria-label={showPass ? 'Hide password' : 'Show password'}
                    tabindex="-1"
                  >
                    {#if showPass}<EyeOff size={14} />{:else}<Eye size={14} />{/if}
                  </button>
                </div>
              </div>
              <div>
                <label for="reg-pass2" class="label">Confirm password</label>
                <input id="reg-pass2" type="password" bind:value={confirmPassword} class="input" placeholder="Repeat password" autocomplete="new-password" required />
              </div>
            </div>
          {:else}
            <div class="space-y-4">
              <div>
                <label for="reg-company" class="label">Company <span class="text-subtle">(optional)</span></label>
                <input id="reg-company" bind:value={company} class="input" placeholder="Your company name" autocomplete="organization" />
              </div>
              <div>
                <span class="label">Plan</span>
                <div class="grid grid-cols-3 gap-2" role="radiogroup" aria-label="Plan">
                  {#each plans as p (p.id)}
                    <button
                      type="button"
                      role="radio"
                      aria-checked={plan === p.id}
                      onclick={() => (plan = p.id)}
                      class="rounded-lg border p-3 text-center transition-colors duration-150
                        {plan === p.id ? 'border-accent bg-accent/10' : 'border-line hover:border-line-strong hover:bg-surface-2'}"
                    >
                      <div class="text-xs font-semibold {plan === p.id ? 'text-accent' : 'text-fg'}">{p.label}</div>
                      <div class="mt-0.5 text-[11px] text-subtle">{p.price}</div>
                    </button>
                  {/each}
                </div>
              </div>
            </div>
          {/if}

          <button type="submit" disabled={busy} class="btn btn-primary mt-6 w-full">
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

    <p class="mt-8 text-center text-xs text-subtle">
      Already have an account? <a href="/login" class="link">Sign in</a>
    </p>
  </div>
</div>
