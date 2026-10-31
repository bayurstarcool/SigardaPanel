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
    <p class="text-sm leading-relaxed text-muted">{message}</p>
  {/if}

  {#snippet footer()}
    <button onclick={cancel} class="btn btn-secondary btn-sm">{cancelLabel}</button>
    <button onclick={confirm} class="btn btn-sm {destructive ? 'btn-danger' : 'btn-primary'}">{confirmLabel}</button>
  {/snippet}
</Modal>
