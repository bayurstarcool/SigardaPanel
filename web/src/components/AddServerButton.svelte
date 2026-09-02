<script lang="ts">
  import { auth, hasRole } from '$lib/auth.svelte';
  import CreateServerModal from '$components/CreateServerModal.svelte';

  let { oncreated }: { oncreated?: () => void } = $props();

  let showModal = $state(false);
  let canManage = $derived(hasRole(auth.user?.role, 'admin'));
</script>

{#if canManage}
  <button
    onclick={() => { showModal = true; }}
    class="self-start inline-flex items-center justify-center gap-1.5 px-5 py-2.5 rounded-xl text-sm font-semibold bg-gradient-to-r from-sky-400 to-indigo-400 text-[#0a1628] shadow-[0_4px_15px_rgba(56,189,248,0.2)] hover:-translate-y-0.5 hover:shadow-[0_6px_20px_rgba(56,189,248,0.3)] transition-all duration-150 cursor-pointer border-0 whitespace-nowrap"
  >
    + Add Server
  </button>
  <CreateServerModal bind:open={showModal} {oncreated} />
{/if}
