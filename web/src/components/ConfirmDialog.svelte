<script lang="ts">
  import Modal from './Modal.svelte';

  let {
    open = $bindable(false),
    title = 'Are you sure?',
    message = '',
    confirmLabel = 'Confirm',
    cancelLabel = 'Cancel',
    destructive = false,
    onconfirm,
    oncancel,
  }: {
    open: boolean;
    title?: string;
    message?: string;
    confirmLabel?: string;
    cancelLabel?: string;
    destructive?: boolean;
    onconfirm?: () => void;
    oncancel?: () => void;
  } = $props();

  function cancel() {
    open = false;
    oncancel?.();
  }

  function confirm() {
    open = false;
    onconfirm?.();
  }
</script>

<Modal bind:open {title} maxWidth="sm" onclose={cancel}>
  {#if message}
    <p class="text-sm text-gray-300 leading-relaxed">{message}</p>
  {/if}

  {#snippet footer()}
    <button
      onclick={cancel}
      class="px-4 py-2 rounded-xl text-sm font-medium text-gray-300 bg-gray-700/50 hover:bg-gray-700 transition cursor-pointer border-0"
    >
      {cancelLabel}
    </button>
    <button
      onclick={confirm}
      class="px-4 py-2 rounded-xl text-sm font-medium text-white transition cursor-pointer border-0
        {destructive ? 'bg-red-600 hover:bg-red-500' : 'bg-indigo-600 hover:bg-indigo-500'}"
    >
      {confirmLabel}
    </button>
  {/snippet}
</Modal>
