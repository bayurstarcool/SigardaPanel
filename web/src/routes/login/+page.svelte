<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { onMount } from 'svelte';
  import { Eye, EyeOff, Loader2, AlertCircle } from 'lucide-svelte';
  import Logo from '$components/Logo.svelte';

  let u = $state('');
  let p = $state('');
  let rm = $state(false);
  let err = $state('');
  let busy = $state(false);
  let showPass = $state(false);
  let shake = $state(false);
  // Second step, shown when the account has 2FA enabled
  let needs2fa = $state(false);
  let useRecovery = $state(false);
  let code = $state('');

  onMount(() => {
    if (auth.user) window.location.href = "/dashboard";
  });

  async function submit(e: Event) {
    e.preventDefault();
    err = '';
    busy = true;
    try {
      const factor = !needs2fa
        ? undefined
        : useRecovery
          ? { recovery_code: code.trim() }
          : { totp_code: code.trim() };
      await auth.login(u, p, rm, factor);
      window.location.href = '/dashboard';
    } catch (e: any) {
      if (e.code === '2fa_required') {
        needs2fa = true;
        err = '';
        busy = false;
        return;
      }
      if (e.code === 'invalid_2fa_code') code = '';
      err = e.message;
      shake = true;
      setTimeout(() => shake = false, 500);
    } finally {
      busy = false;
    }
  }
</script>

<svelte:head>
  <title>Sign in · SigardaPanel</title>
</svelte:head>

<div class="flex min-h-dvh flex-col items-center justify-center bg-bg px-4 py-10">
  <div class="w-full max-w-sm animate-fade-in">
    <div class="mb-8 flex flex-col items-center text-center">
      <a href="/" aria-label="SigardaPanel home"><Logo size={36} /></a>
      <h1 class="mt-5 text-xl font-semibold tracking-tight text-fg">Welcome back</h1>
      <p class="mt-1 text-sm text-muted">Sign in to your control panel</p>
    </div>

    <div class="card p-6 shadow-sm">
      <form onsubmit={submit} class:login-shake={shake}>
        {#if err}
          <div class="mb-5 flex items-start gap-2 rounded-lg bg-danger/10 px-3 py-2.5 text-[13px] text-danger ring-1 ring-danger/20 ring-inset" role="alert">
            <AlertCircle size={16} class="mt-px shrink-0" />
            <span>{err}</span>
          </div>
        {/if}

        <div class="space-y-4">
          <div>
            <label for="login-user" class="label">Username</label>
            <input id="login-user" bind:value={u} class="input" placeholder="admin" autocomplete="username" required />
          </div>

          <div>
            <label for="login-pass" class="label">Password</label>
            <div class="relative">
              <input
                id="login-pass"
                type={showPass ? 'text' : 'password'}
                bind:value={p}
                class="input pr-10"
                placeholder="••••••••"
                autocomplete="current-password"
                required
              />
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

          {#if needs2fa}
            <div class="animate-fade-in">
              <label for="login-2fa" class="label">{useRecovery ? 'Recovery code' : 'Authentication code'}</label>
              <input
                id="login-2fa"
                bind:value={code}
                inputmode={useRecovery ? 'text' : 'numeric'}
                autocomplete="one-time-code"
                maxlength={useRecovery ? 24 : 6}
                class="input font-mono tracking-widest"
                placeholder={useRecovery ? 'XXXX-XXXX-XXXX-XXXX' : '123456'}
                required
              />
              <button type="button" onclick={() => { useRecovery = !useRecovery; code = ''; }} class="link mt-2 text-xs">
                {useRecovery ? 'Use authenticator app instead' : 'Lost your device? Use a recovery code'}
              </button>
            </div>
          {/if}
        </div>

        <label for="remember-me" class="mt-5 flex cursor-pointer items-center gap-2.5 text-sm text-muted">
          <input type="checkbox" bind:checked={rm} class="checkbox" id="remember-me" />
          Remember me
        </label>

        <button type="submit" disabled={busy} class="btn btn-primary mt-6 w-full">
          {#if busy}<Loader2 size={16} class="animate-spin" />Signing in...{:else}Sign in{/if}
        </button>
      </form>
    </div>

    <p class="mt-8 text-center text-xs text-subtle">
      &copy; {new Date().getFullYear()} <a href="https://sigarda.com" target="_blank" rel="noopener" class="hover:text-fg">sigarda.com</a>
      <span class="mx-1.5">·</span>
      <a href="https://github.com/bayurstarcool/SigardaPanel/issues" target="_blank" rel="noopener" class="hover:text-fg">Support</a>
      <span class="mx-1.5">·</span>
      VPS control plane
    </p>
  </div>
</div>

<style>
  @keyframes shake {
    0%, 100% { transform: translateX(0); }
    20%, 60% { transform: translateX(-4px); }
    40%, 80% { transform: translateX(4px); }
  }
  .login-shake { animation: shake 0.35s ease-in-out; }
</style>
