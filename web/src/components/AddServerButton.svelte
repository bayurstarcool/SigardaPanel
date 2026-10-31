<script lang="ts">
  import { auth, hasRole } from '$lib/auth.svelte';
  import CreateServerModal from '$components/CreateServerModal.svelte';
  import { Plus } from 'lucide-svelte';

  let { oncreated, size = 'sm' }: { oncreated?: () => void; size?: 'sm' | 'md' } = $props();

  let showModal = $state(false);
  let canManage = $derived(hasRole(auth.user?.role, 'admin'));
</script>

{#if canManage}
  <button onclick={() => (showModal = true)} class="btn btn-primary {size === 'sm' ? 'btn-sm' : ''}">
    <Plus size={14} /> Add server
  </button>
  <CreateServerModal bind:open={showModal} {oncreated} />
{/if}
