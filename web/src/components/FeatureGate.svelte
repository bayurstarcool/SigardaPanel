<script lang="ts">
  import { onMount } from 'svelte';
  import ProBadge from './ProBadge.svelte';

  let {
    feature = '',
    tier = 'pro',
    compact = false
  }: {
    feature?: string;
    tier?: string;
    compact?: boolean;
  } = $props();

  let features: Record<string, boolean> = $state({});
  let tierLevel: string = $state('free');

  onMount(async () => {
    try {
      const token = localStorage.getItem('sigarda_token');
      const res = await fetch('/api/proxy/license/features', {
        headers: { Authorization: `Bearer ${token}` }
      });
      if (res.ok) {
        const data = await res.json();
        features = data.features || {};
        tierLevel = data.tier || 'free';
      }
    } catch (e) {
      // Default to free tier
    }
  });

  const hasAccess = $derived(features[feature] === true);
</script>

{#if hasAccess}
  <slot />
{:else}
  {#if compact}
    <div class="locked-inline">
      <slot name="locked" />
      <div class="lock-overlay">
        <ProBadge {tier} small />
      </div>
    </div>
  {:else}
    <div class="locked-card">
      <div class="lock-icon">🔒</div>
      <div class="lock-title">
        <slot name="locked" />
      </div>
      <ProBadge {tier} />
      <p class="lock-desc">Upgrade to access this feature</p>
      <a href="/settings/billing" class="lock-btn">Upgrade Now</a>
    </div>
  {/if}
{/if}

<style>
  .locked-card {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    padding: 2rem;
    border: 2px dashed #374151;
    border-radius: 12px;
    background: rgba(0, 0, 0, 0.3);
    text-align: center;
    min-height: 200px;
  }
  .lock-icon {
    font-size: 2rem;
    margin-bottom: 0.5rem;
    opacity: 0.6;
  }
  .lock-title {
    font-size: 1rem;
    font-weight: 600;
    color: #9ca3af;
    margin-bottom: 0.5rem;
  }
  .lock-desc {
    font-size: 0.85rem;
    color: #6b7280;
    margin: 0.5rem 0;
  }
  .lock-btn {
    display: inline-block;
    padding: 8px 20px;
    background: linear-gradient(135deg, #f59e0b, #f97316);
    color: #000;
    border-radius: 8px;
    font-weight: 600;
    font-size: 0.85rem;
    text-decoration: none;
    margin-top: 0.5rem;
    transition: opacity 0.2s;
  }
  .lock-btn:hover { opacity: 0.85; }
  .locked-inline {
    position: relative;
    display: inline-block;
  }
  .lock-overlay {
    position: absolute;
    top: -4px;
    right: -4px;
  }
</style>
