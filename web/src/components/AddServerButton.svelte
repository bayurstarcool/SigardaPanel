<script lang="ts">
  import { auth, hasPermission } from '$lib/auth.svelte';
  import { apiFetch } from '$lib/api';
  import CreateServerModal from '$components/CreateServerModal.svelte';
  import Modal from '$components/Modal.svelte';
  import { Plus, ArrowUpRight } from 'lucide-svelte';

  let { oncreated, size = 'sm' }: { oncreated?: () => void; size?: 'sm' | 'md' } = $props();

  let showModal = $state(false);
  let showLimit = $state(false);
  let limit = $state<{ current: number; max: number; tier: string; upgrade: string } | null>(null);
  let canManage = $derived(hasPermission('server.manage'));

  // The license sets how many servers the panel manages (Free: this one).
  async function open() {
    try {
      const d = await apiFetch(auth.token, 'GET', 'license');
      const s = d?.limits?.servers;
      if (s && s.max >= 0 && s.current >= s.max) {
        limit = { current: s.current, max: s.max, tier: d.tier_name || d.tier, upgrade: d.upgrade_url };
        showLimit = true;
        return;
      }
    } catch {
      /* the API checks again when the server is added */
    }
    showModal = true;
  }
</script>

{#if canManage}
  <button onclick={open} class="btn btn-primary {size === 'sm' ? 'btn-sm' : ''}">
    <Plus size={14} /> Add server
  </button>
  <CreateServerModal bind:open={showModal} {oncreated} />
  <Modal bind:open={showLimit} title="Server limit reached" maxWidth="sm">
    {#if limit}
      <p class="text-sm leading-relaxed text-muted">
        Your {limit.tier} license covers {limit.max} server{limit.max === 1 ? '' : 's'} and {limit.current} {limit.current === 1 ? 'is' : 'are'} in use.
        Every feature is available on the servers you have; upgrade the plan to manage more servers from this panel.
      </p>
    {/if}
    {#snippet footer()}
      <button class="btn btn-secondary" onclick={() => (showLimit = false)}>Close</button>
      {#if limit?.upgrade}
        <a class="btn btn-primary" href={limit.upgrade} target="_blank" rel="noopener noreferrer"><ArrowUpRight size={14} /> Upgrade plan</a>
      {/if}
    {/snippet}
  </Modal>
{/if}
