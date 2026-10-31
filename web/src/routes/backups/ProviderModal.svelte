<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { toasts } from '$lib/toast.svelte';
  import Modal from '$components/Modal.svelte';
  import { Globe, LoaderCircle, CheckCircle2, XCircle } from 'lucide-svelte';

  let {
    open = $bindable(false),
    provider = $bindable(),
    onsaved,
  }: {
    open: boolean;
    provider: any;
    onsaved: () => Promise<void> | void;
  } = $props();

  let saving = $state(false);
  let testingProvider = $state(false);
  let testResult = $state<any>(null);

  const endpointPlaceholder: Record<string, string> = {
    r2: '<account-id>.r2.cloudflarestorage.com',
    s3: 's3.amazonaws.com',
    b2: 's3.us-east-005.backblazeb2.com',
    wasabi: 's3.us-east-1.wasabisys.com',
    digitalocean: 'sgp1.digitaloceanspaces.com',
  };

  function close() {
    open = false;
    provider = null;
    testResult = null;
  }

  function onTypeChange() {
    if (provider.provider_type === 'r2') {
      provider.region = 'auto';
      provider.endpoint = '';
    } else if (provider.provider_type === 's3') {
      provider.region = 'us-east-1';
    } else {
      provider.region = '';
    }
    provider.endpoint = provider.provider_type === 'r2' ? '' : provider.endpoint;
    testResult = null;
  }

  async function saveProvider() {
    saving = true;
    try {
      const body = { ...provider };
      if (provider.id) {
        await apiFetch(auth.token, 'PATCH', `backup-providers/${provider.id}`, body);
        toasts.success('Provider updated');
      } else {
        await apiFetch(auth.token, 'POST', 'backup-providers', body);
        toasts.success('Provider created');
      }
      close();
      await onsaved();
    } catch (e: any) { toasts.error(e.message); }
    finally { saving = false; }
  }

  async function testProviderConnection(id: number) {
    testingProvider = true;
    testResult = null;
    try {
      testResult = await apiFetch(auth.token, 'POST', `backup-providers/${id}/test`);
    } catch (e: any) { toasts.error(e.message); }
    finally { testingProvider = false; }
  }
</script>

<Modal bind:open title={provider?.id ? 'Edit provider' : 'Add provider'} maxWidth="lg" onclose={close}>
  {#if provider}
    <form id="provider-form" class="space-y-4" onsubmit={(e) => { e.preventDefault(); saveProvider(); }}>
      <p class="text-[13px] text-muted">{provider.id ? 'Update storage provider settings.' : 'Configure a new backup storage provider.'}</p>
      <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
        <div>
          <label for="prov-name" class="label">Provider name</label>
          <input id="prov-name" type="text" bind:value={provider.name} placeholder="My provider" class="input" />
        </div>
        <div>
          <label for="prov-type" class="label">Type</label>
          <select id="prov-type" bind:value={provider.provider_type} onchange={onTypeChange} class="input">
            <option value="s3">Amazon S3</option>
            <option value="r2">Cloudflare R2</option>
            <option value="b2">Backblaze B2</option>
            <option value="wasabi">Wasabi</option>
            <option value="digitalocean">DigitalOcean Spaces</option>
            <option value="generic">S3-compatible (generic)</option>
          </select>
        </div>
      </div>
      <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
        <div>
          <label for="prov-access" class="label">Access key</label>
          <input id="prov-access" type="text" bind:value={provider.access_key} placeholder="AKID…" autocomplete="off" class="input font-mono" />
        </div>
        <div>
          <label for="prov-secret" class="label">Secret key</label>
          <input id="prov-secret" type="password" bind:value={provider.secret_key} placeholder="••••••••" autocomplete="new-password" class="input font-mono" />
        </div>
      </div>
      <div>
        <label for="prov-bucket" class="label">Bucket</label>
        <input id="prov-bucket" type="text" bind:value={provider.bucket} placeholder="my-backups" class="input font-mono" />
      </div>
      <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
        <div>
          <label for="prov-endpoint" class="label">Endpoint</label>
          <input
            id="prov-endpoint"
            type="text"
            bind:value={provider.endpoint}
            placeholder={endpointPlaceholder[provider.provider_type] ?? 'your-storage-endpoint.com'}
            class="input font-mono" />
          {#if provider.provider_type === 'r2'}<p class="hint">Find it in R2 dashboard → Settings → S3 API.</p>{/if}
        </div>
        <div>
          <label for="prov-region" class="label">Region</label>
          {#if provider.provider_type === 'r2'}
            <input id="prov-region" type="text" bind:value={provider.region} placeholder="auto" disabled class="input font-mono" />
            <p class="hint">Auto (R2 is global).</p>
          {:else}
            <input
              id="prov-region"
              type="text"
              bind:value={provider.region}
              placeholder={provider.provider_type === 'digitalocean' ? 'sgp1' : provider.provider_type === 's3' ? 'us-east-1' : 'us-east-005'}
              class="input font-mono" />
          {/if}
        </div>
      </div>
      <div>
        <label for="prov-retention" class="label">Retention (days, 0 = forever)</label>
        <input id="prov-retention" type="number" bind:value={provider.retention_days} placeholder="30" min="0" class="input tabular-nums" />
      </div>
      <label class="flex cursor-pointer items-center gap-2 text-sm text-fg">
        <input type="checkbox" bind:checked={provider.is_default} class="checkbox" />
        Set as default provider
      </label>
      {#if provider.id}
        <div class="space-y-2 border-t border-line pt-4">
          <button type="button" onclick={() => testProviderConnection(provider.id)} disabled={testingProvider} class="btn btn-secondary btn-sm w-full">
            {#if testingProvider}<LoaderCircle size={14} class="animate-spin" /> Testing…{:else}<Globe size={14} /> Test connection{/if}
          </button>
          {#if testResult}
            <div class="flex items-start gap-2 rounded-lg px-3 py-2 text-[13px] {testResult.connected ? 'bg-success/10 text-success' : 'bg-danger/10 text-danger'}" role="status">
              {#if testResult.connected}<CheckCircle2 size={16} class="mt-px shrink-0" />{:else}<XCircle size={16} class="mt-px shrink-0" />{/if}
              <span class="min-w-0 break-words">{testResult.message || testResult.error || 'Unknown result'}</span>
            </div>
          {/if}
        </div>
      {/if}
    </form>
  {/if}
  {#snippet footer()}
    <button class="btn btn-secondary btn-sm" onclick={close}>Cancel</button>
    <button type="submit" form="provider-form" class="btn btn-primary btn-sm" disabled={saving}>
      {#if saving}<LoaderCircle size={14} class="animate-spin" /> Saving…{:else}{provider?.id ? 'Update' : 'Create'}{/if}
    </button>
  {/snippet}
</Modal>
