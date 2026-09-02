<script lang="ts">
  let { status }: { status: string } = $props();

  const cfg: Record<string, { dark: string; light: string; dot: string; textDark: string; textLight: string }> = {
    active:    { dark: 'bg-emerald-500/10 border-emerald-500/20', light: 'bg-emerald-50 border-emerald-200', dot: 'bg-emerald-400 shadow-[0_0_8px_rgba(74,222,128,0.6)]', textDark: 'text-emerald-400', textLight: 'text-emerald-600' },
    online:    { dark: 'bg-emerald-500/10 border-emerald-500/20', light: 'bg-emerald-50 border-emerald-200', dot: 'bg-emerald-400 shadow-[0_0_8px_rgba(74,222,128,0.6)]', textDark: 'text-emerald-400', textLight: 'text-emerald-600' },
    succeeded: { dark: 'bg-emerald-500/10 border-emerald-500/20', light: 'bg-emerald-50 border-emerald-200', dot: 'bg-emerald-400 shadow-[0_0_8px_rgba(74,222,128,0.6)]', textDark: 'text-emerald-400', textLight: 'text-emerald-600' },
    imported:  { dark: 'bg-emerald-500/10 border-emerald-500/20', light: 'bg-emerald-50 border-emerald-200', dot: 'bg-emerald-400 shadow-[0_0_8px_rgba(74,222,128,0.6)]', textDark: 'text-emerald-400', textLight: 'text-emerald-600' },
    pending:   { dark: 'bg-amber-500/10 border-amber-500/20',     light: 'bg-amber-50 border-amber-200',     dot: 'bg-amber-400 shadow-[0_0_8px_rgba(251,191,36,0.6)]',   textDark: 'text-amber-400', textLight: 'text-amber-600' },
    warning:   { dark: 'bg-amber-500/10 border-amber-500/20',     light: 'bg-amber-50 border-amber-200',     dot: 'bg-amber-400 shadow-[0_0_8px_rgba(251,191,36,0.6)]',   textDark: 'text-amber-400', textLight: 'text-amber-600' },
    deploying: { dark: 'bg-sky-500/10 border-sky-500/20',         light: 'bg-sky-50 border-sky-200',         dot: 'bg-sky-400 shadow-[0_0_8px_rgba(56,189,248,0.6)]',     textDark: 'text-sky-400', textLight: 'text-sky-600' },
    running:   { dark: 'bg-sky-500/10 border-sky-500/20',         light: 'bg-sky-50 border-sky-200',         dot: 'bg-sky-400 shadow-[0_0_8px_rgba(56,189,248,0.6)]',     textDark: 'text-sky-400', textLight: 'text-sky-600' },
    queued:    { dark: 'bg-purple-500/10 border-purple-500/20',   light: 'bg-purple-50 border-purple-200',   dot: 'bg-purple-400 shadow-[0_0_8px_rgba(167,139,250,0.6)]', textDark: 'text-purple-400', textLight: 'text-purple-600' },
    disabled:  { dark: 'bg-red-500/10 border-red-500/20',         light: 'bg-red-50 border-red-200',         dot: 'bg-red-400 shadow-[0_0_8px_rgba(248,113,113,0.6)]',    textDark: 'text-red-400', textLight: 'text-red-600' },
    failed:    { dark: 'bg-red-500/10 border-red-500/20',         light: 'bg-red-50 border-red-200',         dot: 'bg-red-400 shadow-[0_0_8px_rgba(248,113,113,0.6)]',    textDark: 'text-red-400', textLight: 'text-red-600' },
    offline:   { dark: 'bg-red-500/10 border-red-500/20',         light: 'bg-red-50 border-red-200',         dot: 'bg-red-400 shadow-[0_0_8px_rgba(248,113,113,0.6)]',    textDark: 'text-red-400', textLight: 'text-red-600' },
    none:      { dark: 'bg-red-500/10 border-red-500/20',         light: 'bg-red-50 border-red-200',         dot: 'bg-red-400 shadow-[0_0_8px_rgba(248,113,113,0.6)]',    textDark: 'text-red-400', textLight: 'text-red-600' },
    valid:     { dark: 'bg-emerald-500/10 border-emerald-500/20', light: 'bg-emerald-50 border-emerald-200', dot: 'bg-emerald-400 shadow-[0_0_8px_rgba(74,222,128,0.6)]', textDark: 'text-emerald-400', textLight: 'text-emerald-600' },
    expiring_soon: { dark: 'bg-amber-500/10 border-amber-500/20', light: 'bg-amber-50 border-amber-200',     dot: 'bg-amber-400 shadow-[0_0_8px_rgba(251,191,36,0.6)]',   textDark: 'text-amber-400', textLight: 'text-amber-600' },
    expired:   { dark: 'bg-red-500/10 border-red-500/20',         light: 'bg-red-50 border-red-200',         dot: 'bg-red-400 shadow-[0_0_8px_rgba(248,113,113,0.6)]',    textDark: 'text-red-400', textLight: 'text-red-600' },
  };

  let c = $derived(cfg[status] || { dark: 'bg-gray-50 border-gray-200', light: 'bg-gray-50 border-gray-200', dot: 'bg-gray-400', textDark: 'text-gray-400', textLight: 'text-gray-600' });
</script>

<span class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-lg text-[0.7rem] font-semibold border dark:{c.dark} {c.light} dark:{c.textDark} {c.textLight}">
  <span class="w-1.5 h-1.5 rounded-full {c.dot}"></span>
  {status}
</span>
