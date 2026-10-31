<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import PageHeader from '$components/PageHeader.svelte';
  import { prefs, editorThemes, type EditorTheme } from '$lib/prefs';
  import { toasts } from '$lib/toast.svelte';
  import { User, Check, Palette } from 'lucide-svelte';

  let form = $state({ username: '', email: '', old_password: '', new_password: '' });
  let busy = $state(false);

  // Editor prefs
  let editorTheme = $state<EditorTheme>('github-dark');
  let editorFontSize = $state(13);

  $effect(() => {
    if (auth.user) {
      form.username = auth.user.username;
      form.email = auth.user.email || '';
    }
  });

  // Load prefs
  $effect(() => {
    const unsub = prefs.subscribe((p) => {
      editorTheme = p.editorTheme;
      editorFontSize = p.editorFontSize;
    });
    return unsub;
  });

  async function saveProfile() {
    busy = true;
    try {
      const body: any = {};
      if (form.email !== auth.user?.email) body.email = form.email;
      if (form.new_password) {
        if (!form.old_password) { toasts.error('Current password required'); busy = false; return; }
        body.old_password = form.old_password;
        body.new_password = form.new_password;
      }
      await apiFetch(auth.token, 'PUT', 'auth/profile', body);
      toasts.success('Profile updated');
      form.old_password = ''; form.new_password = '';
    } catch (e: any) { toasts.error(e.message); }
    finally { busy = false; }
  }

  function saveEditorPrefs() {
    prefs.set({ editorTheme, editorFontSize });
    toasts.success('Editor settings saved');
  }

  const fontSizes = [11, 12, 13, 14, 15, 16, 18];
  const LIGHT_THEMES = ['solarized-light', 'light'];
  const themeGroups = [
    { label: 'Dark', items: editorThemes.filter((t) => !LIGHT_THEMES.includes(t.id)) },
    { label: 'Light', items: editorThemes.filter((t) => LIGHT_THEMES.includes(t.id)) },
  ];
  // Preview colors come from the selected editor theme (data, not UI chrome).
  const pv = $derived(editorThemes.find((t) => t.id === editorTheme));
  const pvColor = (i: number, fallback: string) => pv?.preview[i] || fallback;
  const pvFg = $derived(LIGHT_THEMES.includes(editorTheme) ? '#24292f' : '#e6edf3');
</script>

<DashboardLayout active="Profile">
  <PageHeader title="Profile" description="Manage your account settings" />

  <div class="max-w-3xl space-y-6">
    <!-- Account -->
    <section class="card">
      <div class="card-header">
        <h2 class="card-title flex items-center gap-2"><User size={16} class="text-subtle" /> Account</h2>
      </div>
      <div class="card-body">
        <div class="mb-6 flex items-center gap-3">
          <div class="flex size-11 items-center justify-center rounded-full bg-accent/10 text-base font-semibold text-accent">
            {(auth.user?.username || 'U').charAt(0).toUpperCase()}
          </div>
          <div class="min-w-0">
            <div class="truncate text-sm font-semibold text-fg">{auth.user?.username}</div>
            <div class="text-xs text-subtle capitalize">{auth.user?.role}</div>
          </div>
        </div>

        <div class="grid gap-4 sm:grid-cols-2">
          <div>
            <label for="pf-user" class="label">Username</label>
            <input id="pf-user" value={form.username} readonly disabled class="input" />
          </div>
          <div>
            <label for="pf-email" class="label">Email</label>
            <input id="pf-email" type="email" bind:value={form.email} class="input" autocomplete="email" />
          </div>
          <div>
            <label for="pf-cur" class="label">Current password</label>
            <input id="pf-cur" type="password" bind:value={form.old_password} class="input" placeholder="••••••••" autocomplete="current-password" />
            <p class="hint">Required to change your password.</p>
          </div>
          <div>
            <label for="pf-new" class="label">New password</label>
            <input id="pf-new" type="password" bind:value={form.new_password} class="input" placeholder="••••••••" autocomplete="new-password" />
            <p class="hint">Leave blank to keep the current one.</p>
          </div>
        </div>

        <div class="mt-6 flex justify-end">
          <button onclick={saveProfile} disabled={busy} class="btn btn-primary btn-sm">{busy ? 'Saving...' : 'Save profile'}</button>
        </div>
      </div>
    </section>

    <!-- Editor -->
    <section class="card">
      <div class="card-header">
        <h2 class="card-title flex items-center gap-2"><Palette size={16} class="text-subtle" /> Editor settings</h2>
      </div>
      <div class="card-body space-y-6">
        <div>
          <span class="label">Code editor theme</span>
          {#each themeGroups as g (g.label)}
            <div class="mt-3 mb-2 text-xs font-medium text-subtle">{g.label}</div>
            <div class="grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-4">
              {#each g.items as theme (theme.id)}
                {@const active = editorTheme === theme.id}
                <button
                  onclick={() => (editorTheme = theme.id)}
                  aria-pressed={active}
                  class="relative flex items-center gap-2.5 rounded-lg border p-2 text-left transition-colors duration-150
                    {active ? 'border-accent bg-accent/10' : 'border-line hover:border-line-strong hover:bg-surface-2'}"
                >
                  <span class="flex size-8 shrink-0 items-center justify-center gap-px rounded-md ring-1 ring-line" style="background-color: {theme.bg};">
                    {#each theme.preview.slice(0, 3) as color, ci (ci)}
                      <span class="size-1 rounded-full" style="background-color: {color};"></span>
                    {/each}
                  </span>
                  <span class="min-w-0 flex-1">
                    <span class="block truncate text-xs font-medium text-fg">{theme.name}</span>
                    <span class="block truncate font-mono text-[11px] text-subtle">{theme.bg}</span>
                  </span>
                  {#if active}
                    <span class="absolute top-1.5 right-1.5 flex size-4 items-center justify-center rounded-full bg-accent text-accent-fg">
                      <Check size={10} />
                    </span>
                  {/if}
                </button>
              {/each}
            </div>
          {/each}
        </div>

        <div>
          <span class="label">Font size</span>
          <div class="segmented flex-wrap">
            {#each fontSizes as size (size)}
              <button onclick={() => (editorFontSize = size)} class="segment font-mono tabular-nums {editorFontSize === size ? 'segment-active' : ''}">
                {size}px
              </button>
            {/each}
          </div>
        </div>

        <div>
          <span class="label">Preview</span>
          <div class="overflow-x-auto rounded-lg border border-line">
            <div class="p-4 font-mono leading-relaxed whitespace-pre" style="background-color: {pv?.bg || '#0d1117'}; color: {pvFg}; font-size: {editorFontSize}px;">
              <div><span style="color: {pvColor(2, '#7ee787')};">const</span> <span style="color: {pvColor(0, '#ff7b72')};">app</span> = <span style="color: {pvColor(1, '#d2a8ff')};">require</span>(<span style="color: {pvColor(2, '#7ee787')};">'express'</span>);</div>
              <div><span style="color: {pvColor(3, '#ffa657')};">app</span>.<span style="color: {pvColor(1, '#d2a8ff')};">get</span>(<span style="color: {pvColor(2, '#7ee787')};">'/'</span>, (<span style="color: {pvColor(0, '#ff7b72')};">req</span>, <span style="color: {pvColor(0, '#ff7b72')};">res</span>) <span style="color: {pvColor(4, '#c678dd')};">{'=>'} {'{'}</span></div>
              <div class="pl-4"><span style="color: {pvColor(3, '#ffa657')};">res</span>.<span style="color: {pvColor(1, '#d2a8ff')};">send</span>(<span style="color: {pvColor(2, '#7ee787')};">'Hello World'</span>);</div>
              <div>{'}'});</div>
            </div>
          </div>
        </div>

        <div class="flex justify-end">
          <button onclick={saveEditorPrefs} class="btn btn-primary btn-sm">Save editor settings</button>
        </div>
      </div>
    </section>
  </div>
</DashboardLayout>
