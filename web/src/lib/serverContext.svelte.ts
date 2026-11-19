// The server the user is working on, shared by every page.
//
// selectedId 0 means "All servers". Pages that act on one server (Docker,
// Firewall, Redis, …) use `pageServerId`: the selected server, or the only
// one when there is just one; 0 means they must ask the user to pick one
// (never silently the first server). The choice is remembered in
// localStorage and can come from the URL (?server=<id>) for shared links.
// This is a view filter only: the API decides what the user may do.
import { browser } from '$app/env';
import { apiFetch } from './api';
import { auth } from './auth.svelte';

const STORAGE_KEY = 'sigarda_server';

function createServerContext() {
  let servers = $state<any[]>([]);
  let selectedId = $state(0);
  let loaded = $state(false);
  let loading: Promise<void> | null = null;

  function readInitial(): number {
    if (!browser) return 0;
    const fromURL = Number(new URL(window.location.href).searchParams.get('server') || 0);
    if (fromURL > 0) return fromURL;
    try { return Number(localStorage.getItem(STORAGE_KEY) || 0) || 0; } catch { return 0; }
  }

  function persist(id: number) {
    if (!browser) return;
    try {
      if (id > 0) localStorage.setItem(STORAGE_KEY, String(id));
      else localStorage.removeItem(STORAGE_KEY);
    } catch { /* storage unavailable */ }
    // Keep a ?server= the page was opened with in step with the choice.
    const url = new URL(window.location.href);
    if (url.searchParams.has('server')) {
      if (id > 0) url.searchParams.set('server', String(id));
      else url.searchParams.delete('server');
      history.replaceState(history.state, '', url);
    }
  }

  async function fetchServers() {
    try {
      const d: any = await apiFetch(auth.token, 'GET', 'servers');
      servers = d?.servers ?? [];
    } catch {
      servers = [];
    }
    // A page opened at /server/<id>/… may have chosen already (adopt).
    if (!loaded && !selectedId) selectedId = readInitial();
    // A remembered server that no longer exists (or is not visible) is dropped.
    if (selectedId && !servers.some((s) => s.id === selectedId)) selectedId = 0;
    loaded = true;
  }

  return {
    get servers() { return servers; },
    get loaded() { return loaded; },
    get selectedId() { return selectedId; },
    get selected() { return servers.find((s) => s.id === selectedId) ?? null; },
    /** The server a single-server page works on (0 = ask the user). */
    get pageServerId() {
      if (selectedId) return selectedId;
      return servers.length === 1 ? servers[0].id : 0;
    },
    get pageServer() {
      const id = selectedId || (servers.length === 1 ? servers[0].id : 0);
      return servers.find((s) => s.id === id) ?? null;
    },
    nameOf(id: number) { return servers.find((s) => s.id === id)?.name ?? `server #${id}`; },
    /** Loads the server list once (shared by every page). */
    ensure(): Promise<void> {
      if (loaded) return Promise.resolve();
      if (!loading) loading = fetchServers().finally(() => { loading = null; });
      return loading;
    },
    refresh(): Promise<void> { return fetchServers(); },
    select(id: number) {
      if (id && !servers.some((s) => s.id === id)) return;
      selectedId = id;
      persist(id);
    },
    /** A page reached through /server/<id>/…: that server becomes the choice. */
    adopt(id: number) {
      if (id > 0 && id !== selectedId && (!loaded || servers.some((s) => s.id === id))) {
        selectedId = id;
        persist(id);
      }
    },
  };
}

export const serverCtx = createServerContext();
