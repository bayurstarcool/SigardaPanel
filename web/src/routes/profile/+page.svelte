<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import DashboardLayout from '$components/DashboardLayout.svelte';
  import { prefs, editorThemes, type EditorTheme } from '$lib/prefs';
  import { toasts } from '$lib/toast.svelte';
  import { User, X, Check, Monitor, Palette, Type } from 'lucide-svelte';

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
</script>

<DashboardLayout active="Profile">
  <div class="mb-8">
    <h1 class="text-2xl font-bold bg-gradient-to-r from-white to-sky-100 bg-clip-text text-transparent">Profile</h1>
    <p class="text-sm text-slate-500 mt-1">Manage your account settings</p>
  </div>



  <div class="grid gap-6 max-w-3xl">
    <!-- Account Section -->
    <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-6">
      <div class="flex items-center gap-3 mb-6 pb-4 border-b border-white/[0.05]">
        <User size={18} class="text-sky-400" />
        <h2 class="text-sm font-semibold text-slate-200">Account</h2>
      </div>

      <div class="flex items-center gap-4 mb-6">
        <div class="w-16 h-16 rounded-2xl bg-gradient-to-br from-sky-400 to-indigo-500 flex items-center justify-center text-white text-2xl font-bold shadow-[0_4px_15px_rgba(56,189,248,0.2)]">
          {(auth.user?.username || 'U').charAt(0).toUpperCase()}
        </div>
        <div>
          <div class="text-lg font-semibold text-slate-200">{auth.user?.username}</div>
          <div class="text-sm text-slate-500 capitalize">{auth.user?.role}</div>
        </div>
      </div>

      <div class="space-y-4">
        <div>
          <label for="pf-user" class="block text-xs text-slate-500 mb-1.5">Username</label>
          <input id="pf-user" value={form.username} readonly class="w-full bg-white/[0.04] border border-white/[0.07] rounded-xl px-4 py-2.5 text-sm text-slate-200/60 outline-none cursor-not-allowed" />
        </div>
        <div>
          <label for="pf-email" class="block text-xs text-slate-500 mb-1.5">Email</label>
          <input id="pf-email" bind:value={form.email} class="w-full bg-white/[0.04] border border-white/[0.07] rounded-xl px-4 py-2.5 text-sm text-slate-200 outline-none focus:border-sky-400/40 transition-colors" />
        </div>
        <div>
          <label for="pf-cur" class="block text-xs text-slate-500 mb-1.5">Current Password (required to change)</label>
          <input id="pf-cur" type="password" bind:value={form.old_password} class="w-full bg-white/[0.04] border border-white/[0.07] rounded-xl px-4 py-2.5 text-sm text-slate-200 outline-none focus:border-sky-400/40 transition-colors" placeholder="••••••••" />
        </div>
        <div>
          <label for="pf-new" class="block text-xs text-slate-500 mb-1.5">New Password (leave blank to keep)</label>
          <input id="pf-new" type="password" bind:value={form.new_password} class="w-full bg-white/[0.04] border border-white/[0.07] rounded-xl px-4 py-2.5 text-sm text-slate-200 outline-none focus:border-sky-400/40 transition-colors" placeholder="••••••••" />
        </div>
      </div>

      <div class="mt-6">
        <button onclick={saveProfile} disabled={busy} class="inline-flex items-center gap-1.5 px-4 py-2 rounded-xl text-sm font-medium bg-sky-500 text-white hover:bg-sky-400 transition disabled:opacity-40 cursor-pointer border-0">{busy ? 'Saving...' : 'Save Profile'}</button>
      </div>
    </div>

    <!-- Editor Settings Section -->
    <div class="bg-white/[0.03] border border-white/[0.07] rounded-xl p-6">
      <div class="flex items-center gap-3 mb-6 pb-4 border-b border-white/[0.05]">
        <Palette size={18} class="text-sky-400" />
        <h2 class="text-sm font-semibold text-slate-200">Editor Settings</h2>
      </div>

      <!-- Theme Selection -->
      <div class="mb-6">
        <label class="block text-xs text-slate-500 mb-3">Code Editor Theme</label>

        <!-- Dark themes -->
        <div class="mb-3">
          <span class="text-[0.6rem] uppercase tracking-wider text-slate-600 font-semibold">Dark</span>
        </div>
        <div class="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 gap-2 mb-5">
          {#each editorThemes.filter(t => !['solarized-light','light'].includes(t.id)) as theme}
            <button
              onclick={() => editorTheme = theme.id}
              class="relative flex items-center gap-2.5 p-2.5 rounded-xl border transition-all duration-200 cursor-pointer text-left {editorTheme === theme.id ? 'border-sky-400/60 bg-sky-400/10 shadow-[0_0_15px_rgba(56,189,248,0.1)]' : 'border-white/[0.07] bg-white/[0.02] hover:border-white/[0.15] hover:bg-white/[0.04]'}"
            >
              <div class="flex-shrink-0 w-8 h-8 rounded-lg overflow-hidden flex items-center justify-center" style="background-color: {theme.bg};">
                <div class="flex gap-px">
                  {#each theme.preview.slice(0, 3) as color}
                    <div class="w-1 h-1 rounded-full" style="background-color: {color};"></div>
                  {/each}
                </div>
              </div>
              <div class="min-w-0 flex-1">
                <div class="text-xs font-medium text-slate-200 truncate">{theme.name}</div>
                <div class="text-[0.6rem] font-mono truncate" style="color: {theme.preview[0]};">{theme.bg}</div>
              </div>
              {#if editorTheme === theme.id}
                <div class="absolute top-1.5 right-1.5 w-4 h-4 rounded-full bg-sky-400 flex items-center justify-center">
                  <Check size={10} class="text-[#0a1628]" />
                </div>
              {/if}
            </button>
          {/each}
        </div>

        <!-- Light themes -->
        <div class="mb-3">
          <span class="text-[0.6rem] uppercase tracking-wider text-slate-600 font-semibold">Light</span>
        </div>
        <div class="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 gap-2">
          {#each editorThemes.filter(t => ['solarized-light','light'].includes(t.id)) as theme}
            <button
              onclick={() => editorTheme = theme.id}
              class="relative flex items-center gap-2.5 p-2.5 rounded-xl border transition-all duration-200 cursor-pointer text-left {editorTheme === theme.id ? 'border-sky-400/60 bg-sky-400/10 shadow-[0_0_15px_rgba(56,189,248,0.1)]' : 'border-white/[0.07] bg-white/[0.02] hover:border-white/[0.15] hover:bg-white/[0.04]'}"
            >
              <div class="flex-shrink-0 w-8 h-8 rounded-lg overflow-hidden flex items-center justify-center border border-black/10" style="background-color: {theme.bg};">
                <div class="flex gap-px">
                  {#each theme.preview.slice(0, 3) as color}
                    <div class="w-1 h-1 rounded-full" style="background-color: {color};"></div>
                  {/each}
                </div>
              </div>
              <div class="min-w-0 flex-1">
                <div class="text-xs font-medium text-slate-200 truncate">{theme.name}</div>
                <div class="text-[0.6rem] font-mono truncate text-slate-500">{theme.bg}</div>
              </div>
              {#if editorTheme === theme.id}
                <div class="absolute top-1.5 right-1.5 w-4 h-4 rounded-full bg-sky-400 flex items-center justify-center">
                  <Check size={10} class="text-[#0a1628]" />
                </div>
              {/if}
            </button>
          {/each}
        </div>
      </div>

      <!-- Font Size -->
      <div class="mb-6">
        <label class="block text-xs text-slate-500 mb-3">Font Size</label>
        <div class="flex items-center gap-2 flex-wrap">
          {#each fontSizes as size}
            <button
              onclick={() => editorFontSize = size}
              class="px-3 py-1.5 rounded-lg text-xs font-mono transition-all duration-200 cursor-pointer {editorFontSize === size ? 'bg-sky-400/20 text-sky-300 border border-sky-400/40' : 'bg-white/[0.04] text-slate-400 border border-white/[0.07] hover:bg-white/[0.08] hover:text-slate-200'}"
            >
              {size}px
            </button>
          {/each}
        </div>
      </div>

      <!-- Preview -->
      <div class="mb-4">
        <label class="block text-xs text-slate-500 mb-2">Preview</label>
        <div class="rounded-xl overflow-hidden border border-white/[0.07]">
          <div class="p-4 font-mono text-sm leading-relaxed" style="background-color: {editorThemes.find(t => t.id === editorTheme)?.bg || '#0d1117'}; font-size: {editorFontSize}px;">
            <div><span style="color: {editorThemes.find(t => t.id === editorTheme)?.preview[2] || '#7ee787'};">const</span> <span style="color: {editorThemes.find(t => t.id === editorTheme)?.preview[0] || '#ff7b72'};">app</span> = <span style="color: {editorThemes.find(t => t.id === editorTheme)?.preview[1] || '#d2a8ff'};">require</span>(<span style="color: {editorThemes.find(t => t.id === editorTheme)?.preview[2] || '#7ee787'};">'express'</span>);</div>
            <div><span style="color: {editorThemes.find(t => t.id === editorTheme)?.preview[3] || '#ffa657'};">app</span>.<span style="color: {editorThemes.find(t => t.id === editorTheme)?.preview[1] || '#d2a8ff'};">get</span>(<span style="color: {editorThemes.find(t => t.id === editorTheme)?.preview[2] || '#7ee787'};">'/'</span>, (<span style="color: {editorThemes.find(t => t.id === editorTheme)?.preview[0] || '#ff7b72'};">req</span>, <span style="color: {editorThemes.find(t => t.id === editorTheme)?.preview[0] || '#ff7b72'};">res</span>) <span style="color: {editorThemes.find(t => t.id === editorTheme)?.preview[4] || '#c678dd'};">{'=>'} {'{'}</div>
            <div class="pl-4"><span style="color: {editorThemes.find(t => t.id === editorTheme)?.preview[3] || '#ffa657'};">res</span>.<span style="color: {editorThemes.find(t => t.id === editorTheme)?.preview[1] || '#d2a8ff'};">send</span>(<span style="color: {editorThemes.find(t => t.id === editorTheme)?.preview[2] || '#7ee787'};">'Hello World'</span>);</div>
            <div>{'}'});</div>
          </div>
        </div>
      </div>

      <div class="mt-4">
        <button onclick={saveEditorPrefs} class="inline-flex items-center gap-1.5 px-4 py-2 rounded-xl text-sm font-medium bg-sky-500 text-white hover:bg-sky-400 transition cursor-pointer border-0">Save Editor Settings</button>
      </div>
    </div>
  </div>
</DashboardLayout>
