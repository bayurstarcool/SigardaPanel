<script lang="ts">
  // Import a database that already exists on a server (created outside the
  // panel) as an "external" database: the panel lists and uses it (viewer,
  // manager, backups, phpMyAdmin) but never drops, alters or re-grants it.
  // Super admin only (the API refuses everyone else).
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import { toasts } from '$lib/toast.svelte';
  import { serverCtx } from '$lib/serverContext.svelte';
  import Modal from '$components/Modal.svelte';
  import Combobox from './Combobox.svelte';
  import { fmtSize, siteLabel } from './pma';
  import { LoaderCircle, RefreshCw, Info, AlertTriangle } from 'lucide-svelte';

  let { open = $bindable(false), onimported }: { open?: boolean; onimported: () => void } = $props();

  let serverId = $state(0);
  let allSites = $state<any[]>([]);
  let discovered = $state<any[]>([]);
  let engineErrors = $state<Record<string, string>>({});
  let scanning = $state(false);
  let scanError = $state('');
  let selectedKey = $state('');
  let siteId = $state(0);
  let siteQuery = $state('');
  let username = $state('');
  let password = $state('');
  let busy = $state(false);
  let importError = $state('');

  const servers = $derived(serverCtx.servers);
  const selected = $derived(discovered.find((d) => key(d) === selectedKey));
  const serverSites = $derived(allSites.filter((s) => s.server_id === serverId));
  const siteOptions = $derived(
    siteQuery
      ? serverSites.filter((s) => siteLabel(s).toLowerCase().includes(siteQuery.toLowerCase())).slice(0, 12)
      : serverSites.slice(0, 12),
  );
  const credsHalf = $derived((username === '') !== (password === ''));

  function key(d: any) { return `${d.engine}/${d.name}`; }
  function engineLabel(e: string) { return e === 'postgres' ? 'PostgreSQL' : 'MySQL/MariaDB'; }

  function defaultServer(): number {
    if (serverCtx.selectedId) return serverCtx.selectedId;
    const local = servers.find((s: any) => s.is_local);
    return local?.id || servers[0]?.id || 0;
  }

  function reset() {
    selectedKey = '';
    siteId = 0;
    siteQuery = '';
    username = '';
    password = '';
    importError = '';
  }

  // Opening the dialog: default server, the sites list, then a scan.
  let wasOpen = false;
  $effect(() => {
    if (open && !wasOpen) {
      wasOpen = true;
      void start();
    } else if (!open) {
      wasOpen = false;
    }
  });

  async function start() {
    reset();
    await serverCtx.ensure();
    if (!serverId || !servers.some((s: any) => s.id === serverId)) serverId = defaultServer();
    try {
      const data = await apiFetch(auth.token, 'GET', 'sites');
      allSites = data?.sites || [];
    } catch { allSites = []; }
    await scan();
  }

  async function scan() {
    discovered = [];
    engineErrors = {};
    scanError = '';
    selectedKey = '';
    if (!serverId) return;
    scanning = true;
    try {
      const data = await apiFetch(auth.token, 'GET', `servers/${serverId}/databases/discover`);
      discovered = data?.databases || [];
      engineErrors = data?.errors || {};
    } catch (e: any) {
      scanError = e.message || 'Could not list the databases on this server';
    } finally { scanning = false; }
  }

  function changeServer() {
    siteId = 0;
    siteQuery = '';
    void scan();
  }

  function chooseSite(site: any) {
    siteId = site.id;
    siteQuery = siteLabel(site);
  }
  function selectSiteByQuery() {
    siteId = serverSites.find((s) => siteLabel(s) === siteQuery)?.id || 0;
  }

  async function doImport() {
    if (!selected || !siteId || credsHalf) return;
    busy = true;
    importError = '';
    try {
      const body: any = { site_id: siteId, type: selected.engine, name: selected.name };
      if (username) { body.username = username; body.password = password; }
      await apiFetch(auth.token, 'POST', 'databases/import', body);
      toasts.success(`Database ${selected.name} imported`);
      open = false;
      onimported();
    } catch (e: any) {
      importError = e.message || 'Import failed';
    } finally { busy = false; }
  }
</script>

<Modal bind:open title="Import existing database" maxWidth="2xl">
  <form id="import-db-form" class="space-y-4" onsubmit={(e) => { e.preventDefault(); doImport(); }}>
    <div class="flex items-start gap-2 rounded-lg border border-line bg-surface-2/40 px-3 py-2.5 text-[13px] text-muted">
      <Info size={14} class="mt-0.5 shrink-0 text-subtle" />
      <p>
        Attach a database that was created outside the panel to a site. It is marked <span class="badge">External</span>:
        the panel shows it and uses it (manager, backups, phpMyAdmin), but never drops, alters or re-grants it, and never changes its accounts or passwords.
      </p>
    </div>

    <div class="flex items-end gap-2">
      <div class="flex-1">
        <label for="import-db-server" class="label">Server</label>
        <select id="import-db-server" bind:value={serverId} onchange={changeServer} class="input">
          {#each servers as s (s.id)}
            <option value={s.id}>{s.name}{s.is_local ? ' (this panel)' : ''}</option>
          {/each}
        </select>
      </div>
      <button type="button" class="btn btn-secondary" onclick={scan} disabled={scanning || !serverId} title="Scan again">
        <RefreshCw size={14} class={scanning ? 'animate-spin' : ''} /> Scan
      </button>
    </div>

    <div>
      <p class="label">Databases on the server</p>
      {#if scanning}
        <div class="space-y-2">
          <div class="skeleton h-8"></div>
          <div class="skeleton h-8 w-3/4"></div>
        </div>
      {:else if scanError}
        <p class="text-[13px] text-danger" role="alert">{scanError}</p>
      {:else if discovered.length === 0}
        <p class="text-[13px] text-subtle">No databases found (MySQL/MariaDB and PostgreSQL, without the server's own).</p>
      {:else}
        <div class="table-wrap max-h-64 overflow-y-auto rounded-lg border border-line">
          <table class="table">
            <thead>
              <tr>
                <th class="w-8"><span class="sr-only">Select</span></th>
                <th>Database</th>
                <th class="max-sm:hidden">Engine</th>
                <th class="max-sm:hidden">Owner</th>
                <th class="max-sm:hidden">Size</th>
                <th>Status</th>
              </tr>
            </thead>
            <tbody>
              {#each discovered as d (key(d))}
                {@const disabled = d.registered || !d.importable}
                <tr class={disabled ? 'opacity-60' : 'cursor-pointer'} onclick={() => { if (!disabled) selectedKey = key(d); }}>
                  <td class="w-8">
                    <input type="radio" name="import-db-choice" value={key(d)} bind:group={selectedKey} {disabled} aria-label="Select {d.name}" />
                  </td>
                  <td class="font-mono text-[13px] text-fg">{d.name}</td>
                  <td class="max-sm:hidden"><span class="badge {d.engine === 'mysql' ? 'badge-success' : 'badge-info'}"><span class="dot"></span>{d.engine}</span></td>
                  <td class="font-mono text-[13px] text-muted max-sm:hidden">{d.owner || '—'}</td>
                  <td class="text-muted tabular-nums max-sm:hidden">{d.size_bytes ? fmtSize(d.size_bytes) : '—'}</td>
                  <td class="text-[13px]">
                    {#if d.registered}
                      <span class="text-subtle">Already in the panel</span>
                    {:else if !d.importable}
                      <span class="text-subtle" title={d.reason}>Not supported</span>
                    {:else}
                      <span class="text-muted">Available</span>
                    {/if}
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
      {#each Object.entries(engineErrors) as [engine, msg] (engine)}
        <p class="mt-2 flex items-start gap-1.5 text-xs text-warning"><AlertTriangle size={12} class="mt-0.5 shrink-0" />{engineLabel(engine)}: {msg}</p>
      {/each}
    </div>

    {#if selected}
      <Combobox
        id="import-db-site"
        label="Attach to site"
        bind:value={siteQuery}
        options={siteOptions}
        optionLabel={siteLabel}
        optionKey={(s) => s.id}
        placeholder="Search a site on this server…"
        emptyText="No site on this server"
        oninput={selectSiteByQuery}
        onselect={chooseSite} />

      <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
        <div>
          <label for="import-db-user" class="label">Database user <span class="text-subtle">(optional)</span></label>
          <input id="import-db-user" bind:value={username} class="input font-mono" autocomplete="off" placeholder={selected.owner?.split(',')[0] || ''} />
        </div>
        <div>
          <label for="import-db-pass" class="label">Password <span class="text-subtle">(optional)</span></label>
          <input id="import-db-pass" type="password" bind:value={password} class="input font-mono" autocomplete="new-password" />
        </div>
      </div>
      <p class="text-xs text-subtle">
        The application's own account. The panel signs in with it to check it before importing, then uses it for the manager, backups and phpMyAdmin.
        Without it the database is only listed.
      </p>
      {#if credsHalf}<p class="text-xs text-warning">Give both a user and a password, or neither.</p>{/if}
    {/if}

    {#if importError}
      <p class="rounded-lg border border-danger/30 px-3 py-2 text-[13px] text-danger" role="alert">{importError}</p>
    {/if}
  </form>
  {#snippet footer()}
    <button class="btn btn-secondary btn-sm" onclick={() => (open = false)}>Cancel</button>
    <button type="submit" form="import-db-form" class="btn btn-primary btn-sm" disabled={busy || !selected || !siteId || credsHalf}>
      {#if busy}<LoaderCircle size={14} class="animate-spin" /> Importing…{:else}Import{/if}
    </button>
  {/snippet}
</Modal>
