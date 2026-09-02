<script lang="ts">
  let { children, fallback } = $props();
  let error = $state<Error | null>(null);

  function reset() {
    error = null;
  }

  function captureError(e: Error) {
    error = e;
  }
</script>

{#if error}
  <div class="error-boundary">
    <div class="error-content">
      <div class="error-icon">⚠️</div>
      <h2>Something went wrong</h2>
      <p class="error-message">{error.message}</p>
      <div class="error-actions">
        <button class="btn btn-primary" onclick={reset}>
          Try again
        </button>
        <button class="btn btn-secondary" onclick={() => window.location.reload()}>
          Reload page
        </button>
      </div>
    </div>
  </div>
{:else}
  {@render children()}
{/if}

<style>
  .error-boundary {
    display: flex;
    align-items: center;
    justify-content: center;
    min-height: 200px;
    padding: 2rem;
  }
  .error-content {
    text-align: center;
    max-width: 400px;
  }
  .error-icon {
    font-size: 3rem;
    margin-bottom: 1rem;
  }
  .error-content h2 {
    font-size: 1.25rem;
    margin-bottom: 0.5rem;
    color: #1f2937;
  }
  .error-message {
    color: #6b7280;
    margin-bottom: 1.5rem;
    font-size: 0.875rem;
  }
  .error-actions {
    display: flex;
    gap: 0.75rem;
    justify-content: center;
  }
  .btn {
    padding: 0.5rem 1rem;
    border-radius: 6px;
    font-size: 0.875rem;
    cursor: pointer;
    transition: all 0.2s;
  }
  .btn-primary {
    background: #3b82f6;
    color: white;
    border: none;
  }
  .btn-primary:hover {
    background: #2563eb;
  }
  .btn-secondary {
    background: white;
    color: #374151;
    border: 1px solid #d1d5db;
  }
  .btn-secondary:hover {
    background: #f3f4f6;
  }
</style>
