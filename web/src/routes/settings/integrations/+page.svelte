<script lang="ts">
  // Git provider connections (GitHub, GitLab, Gitea) of the signed-in user:
  // an access token per account, used to pick repositories on a site's Git
  // tab, add its deploy key and webhook, and report deploys on commits.
  import { auth, hasRole } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { toasts } from '$lib/toast.svelte';
  import { fmtRelative } from '$lib/format';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import PageHeader from '$components/PageHeader.svelte';
  import ConfirmDialog from '$components/ConfirmDialog.svelte';
  import { GitBranch, Plus, Trash2, LoaderCircle, ExternalLink, ShieldCheck, ChevronLeft } from 'lucide-svelte';
  import { onMount } from 'svelte';

  type Conn = { id: number; provider: string; base_url: string; username: string; sites: number; created_at: string };
  let conns = $state<Conn[]>([]);
  let loading = $state(true);
  let showAdd = $state(false);
  let provider = $state<'github' | 'gitlab' | 'gitea'>('github');
  let baseURL = $state('');
  let token = $state('');
  let allowPrivate = $state(false);
  let saving = $state(false);
  let removeTarget = $state<Conn | null>(null);
  let removeOpen = $state(false);
  const isSuper = $derived(hasRole(auth.user?.role, 'super_admin'));

  const providers = {
    github: {
      name: 'GitHub',
      placeholder: 'https://github.com (or your GitHub Enterprise URL)',
      tokenURL: (b: string) => `${b || 'https://github.com'}/settings/personal-access-tokens/new`,
      scopes: 'Fine-grained token, repository access: the repositories you deploy. Permissions: Contents (read), Administration (read and write, for deploy keys), Webhooks (read and write), Deployments (read and write), Metadata (read). A classic token needs the repo and admin:repo_hook scopes.',
    },
    gitlab: {
      name: 'GitLab',
      placeholder: 'https://gitlab.com (or your GitLab server)',
      tokenURL: (b: string) => `${b || 'https://gitlab.com'}/-/user_settings/personal_access_tokens`,
      scopes: 'Personal (or group/project) access token with the api scope. Your role on the project must be Maintainer to add deploy keys and webhooks.',
    },
    gitea: {
      name: 'Gitea',
      placeholder: 'https://git.example.com (required)',
      tokenURL: (b: string) => (b ? `${b}/user/settings/applications` : ''),
      scopes: 'Access token with read:user and write:repository permissions. You need admin rights on the repository for deploy keys and webhooks.',
    },
  } as const;
  const p = $derived(providers[provider]);

  async function load() {
    try {
      const d = await apiFetch(auth.token, 'GET', 'git/connections');
      conns = d?.connections || [];
    } catch (e: any) {
      if (e?.message !== 'session expired') toasts.error(e.message);
    } finally {
      loading = false;
    }
  }

  async function add() {
    saving = true;
    try {
      const c = await apiFetch(auth.token, 'POST', 'git/connections', { provider, base_url: baseURL.trim(), token: token.trim(), allow_private: allowPrivate });
      toasts.success(`Connected ${providers[provider].name} as ${c.username}`);
      token = '';
      baseURL = '';
      allowPrivate = false;
      showAdd = false;
      await load();
    } catch (e: any) {
      toasts.error(e.message);
    } finally {
      saving = false;
    }
  }

  async function remove() {
    if (!removeTarget) return;
    try {
      await apiFetch(auth.token, 'DELETE', `git/connections/${removeTarget.id}`);
      toasts.success('Connection removed');
      await load();
    } catch (e: any) {
      toasts.error(e.message);
    }
  }

  const label = (c: Conn) => (providers as any)[c.provider]?.name || c.provider;
  onMount(() => {
    if (!auth.user) { window.location.href = '/login'; return; }
    void load();
  });
</script>

<DashboardLayout active="Settings">
  <a href="/settings" class="mb-3 inline-flex items-center gap-1 text-[13px] text-subtle hover:text-fg"><ChevronLeft size={14} /> Settings</a>
  <PageHeader title="Integrations" description="Connect GitHub, GitLab or Gitea to deploy sites from your repositories.">
    {#snippet actions()}
      {#if !showAdd}<button onclick={() => (showAdd = true)} class="btn btn-primary btn-sm"><Plus size={14} /> Connect account</button>{/if}
    {/snippet}
  </PageHeader>

  {#if showAdd}
    <section class="card mb-6 animate-fade-in">
      <div class="card-header"><h2 class="card-title">Connect a git account</h2></div>
      <div class="card-body space-y-4">
        <div class="segmented" role="group" aria-label="Provider">
          {#each Object.entries(providers) as [k, v] (k)}
            <button onclick={() => (provider = k as any)} class="segment {provider === k ? 'segment-active' : ''}">{v.name}</button>
          {/each}
        </div>
        <div class="grid gap-4 md:grid-cols-2">
          <div>
            <label for="git-base" class="label">Server URL {#if provider !== 'gitea'}<span class="text-subtle">(optional)</span>{/if}</label>
            <input id="git-base" bind:value={baseURL} class="input font-mono text-[13px]" placeholder={p.placeholder} />
          </div>
          <div>
            <label for="git-token" class="label">Access token</label>
            <input id="git-token" bind:value={token} type="password" autocomplete="off" class="input font-mono text-[13px]" placeholder="Paste the token" />
          </div>
        </div>
        <div class="rounded-lg border border-line bg-surface-2/60 p-3 text-[12.5px] text-muted">
          <div class="mb-1 flex items-center gap-1.5 font-medium text-fg"><ShieldCheck size={14} class="text-accent" /> Token permissions</div>
          {p.scopes}
          {#if p.tokenURL(baseURL.trim())}
            <a href={p.tokenURL(baseURL.trim().replace(/\/$/, ''))} target="_blank" rel="noopener" class="link ml-1 inline-flex items-center gap-0.5">Create a token <ExternalLink size={11} /></a>
          {/if}
          <p class="mt-1.5 text-subtle">The token is stored encrypted and used only for your sites. Each connected site gets its own read-only deploy key; the token itself never reaches the servers.</p>
        </div>
        {#if isSuper && provider !== 'github'}
          <label class="flex cursor-pointer items-center gap-2 text-[13px] text-muted">
            <input type="checkbox" bind:checked={allowPrivate} class="checkbox" /> The server is on a private network (allow private addresses and plain http)
          </label>
        {/if}
        <div class="flex gap-2">
          <button onclick={add} disabled={saving || !token.trim() || (provider === 'gitea' && !baseURL.trim())} class="btn btn-primary btn-sm">
            {#if saving}<LoaderCircle size={14} class="animate-spin" /> Verifying…{:else}Connect{/if}
          </button>
          <button onclick={() => (showAdd = false)} class="btn btn-ghost btn-sm">Cancel</button>
        </div>
      </div>
    </section>
  {/if}

  <section class="card">
    <div class="card-header"><h2 class="card-title">Connected accounts</h2></div>
    {#if loading}
      <div class="card-body space-y-2">{#each Array(2) as _, i (i)}<div class="skeleton h-12 rounded-lg"></div>{/each}</div>
    {:else if conns.length === 0}
      <div class="card-body py-10 text-center">
        <span class="mx-auto mb-3 flex size-10 items-center justify-center rounded-xl bg-accent/10 text-accent"><GitBranch size={18} /></span>
        <p class="text-sm font-medium text-fg">No git account connected</p>
        <p class="mt-1 text-[13px] text-muted">Connect GitHub, GitLab or Gitea, then open a site's Git tab to deploy from a repository.</p>
      </div>
    {:else}
      <ul class="divide-y divide-line">
        {#each conns as c (c.id)}
          <li class="flex items-center gap-3 px-5 py-3.5">
            <span class="flex size-9 shrink-0 items-center justify-center rounded-lg border border-line bg-surface-2 text-muted"><GitBranch size={16} /></span>
            <div class="min-w-0 flex-1">
              <div class="flex flex-wrap items-center gap-2">
                <span class="text-sm font-medium text-fg">{label(c)}</span>
                <span class="text-[13px] text-muted">@{c.username}</span>
                {#if c.sites > 0}<span class="badge">{c.sites} site{c.sites === 1 ? '' : 's'}</span>{/if}
              </div>
              <div class="truncate font-mono text-[11.5px] text-subtle">{c.base_url} · connected {fmtRelative(c.created_at)}</div>
            </div>
            <button onclick={() => { removeTarget = c; removeOpen = true; }} class="btn btn-ghost btn-icon btn-sm" aria-label="Remove connection" title={c.sites > 0 ? 'Disconnect its sites first' : 'Remove'}><Trash2 size={15} /></button>
          </li>
        {/each}
      </ul>
    {/if}
  </section>

  <ConfirmDialog
    bind:open={removeOpen}
    title="Remove this connection?"
    message={removeTarget ? `${label(removeTarget)} @${removeTarget.username} is removed from the panel. Revoke the token on ${removeTarget.base_url} too if you no longer use it.` : ''}
    confirmLabel="Remove"
    destructive
    onconfirm={remove}
  />
</DashboardLayout>
