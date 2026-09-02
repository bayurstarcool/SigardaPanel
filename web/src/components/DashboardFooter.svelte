<script lang="ts">
  import { Heart, ExternalLink, ArrowUp } from 'lucide-svelte';
  const year = new Date().getFullYear();
</script>

<svelte:head>
  <style>
    html.light .footer-bar {
      background: rgba(255, 255, 255, 0.95);
      border-top: 1px solid rgba(0, 0, 0, 0.06);
      box-shadow: 0 -2px 12px rgba(0, 0, 0, 0.03);
    }
  </style>
</svelte:head>

<footer class="relative z-10 lg:ml-72">
  <div class="footer-bar relative bg-[var(--bg-secondary)]/95 dark:bg-[var(--bg-secondary)]/95 border-t border-[var(--border-primary)] transition-colors duration-300">
    <div class="absolute top-0 left-1/2 -translate-x-1/2 w-1/3 h-px bg-gradient-to-r from-transparent via-sky-400/15 to-transparent"></div>
    <div class="flex items-center justify-between px-4 lg:px-8 py-3">
      <div class="text-[0.65rem] text-[var(--text-muted)] flex items-center gap-1.5">
        <span>&copy; {year}</span>
        <a href="https://sigarda.com" target="_blank" rel="noopener noreferrer" class="text-sky-500 dark:text-sky-400 hover:text-sky-400 dark:hover:text-sky-300 transition-colors font-medium">sigarda.com</a>
        <span class="text-[var(--text-muted)] opacity-50 hidden sm:inline">&mdash; VPS control plane</span>
      </div>
      <div class="hidden md:flex items-center gap-1.5 text-[0.65rem] text-[var(--text-muted)] opacity-50">
        <span>Built with</span>
        <Heart size={10} class="text-red-400 opacity-70 fill-red-400" />
        <span>using</span>
        <a href="https://svelte.dev" target="_blank" rel="noopener noreferrer" class="text-[var(--text-secondary)] hover:text-[var(--text-primary)] transition-colors">SvelteKit</a>
      </div>
      <div class="flex items-center gap-3">
        <a href="#" onclick={(e) => { e.preventDefault(); window.scrollTo({ top: 0, behavior: 'smooth' }); }}
          class="flex items-center gap-1 text-[0.65rem] text-[var(--text-muted)] hover:text-sky-500 dark:hover:text-sky-400 transition-all duration-200 hover:-translate-y-0.5" title="Scroll to top">
          <ArrowUp size={12} />
          <span class="hidden sm:inline">Top</span>
        </a>
        <div class="w-px h-3 bg-[var(--border-primary)]"></div>
        <a href="https://github.com/sigardapanel" target="_blank" rel="noopener noreferrer" class="text-[var(--text-muted)] hover:text-[var(--text-secondary)] transition-colors flex items-center" title="GitHub">
          <ExternalLink size={10} />
        </a>
      </div>
    </div>
  </div>
</footer>
