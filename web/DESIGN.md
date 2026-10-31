# SigardaPanel UI — design rules

Premium, simple, clean, fast. Flat surfaces, 1px borders, one accent color,
no decorative effects. Everything is Tailwind v4 utilities plus a small set of
component classes defined in `src/app.css`. The reference implementation is
`src/routes/ssl/+page.svelte` — copy its structure.

## Theme tokens (the only colors you use)

Defined as CSS variables in `app.css`, switched by `html.dark` / `html.light`,
exposed to Tailwind via `@theme inline`. They work in both themes automatically,
so **do not use `dark:` / `light:` variants** for colors.

| Purpose | Utility |
|---|---|
| Page background | `bg-bg` |
| Card / panel | `bg-surface` |
| Hover, inset, table header, chips | `bg-surface-2` |
| Modal / popover / dropdown | `bg-elevated` |
| Borders | `border-line`, stronger: `border-line-strong` |
| Text: primary / secondary / meta | `text-fg` / `text-muted` / `text-subtle` |
| Accent (primary action, active, links) | `text-accent`, `bg-accent`, `bg-accent/10`, `ring-accent/20` |
| Status | `text-success` `text-warning` `text-danger` `text-info`, soft backgrounds via `/10` (e.g. `bg-danger/10`) |
| Solid destructive button | `bg-danger-solid` (use `.btn-danger`) |

Allowed raw colors: `text-white` on solid accent/danger fills, `bg-black/60` for overlays (use `.overlay`).

**Never use:** `slate-*`, `gray-*`, `zinc-*`, `sky-*`, `indigo-*`, `white/[…]`, `var(--bg-primary)`-style
legacy vars, `bg-gradient-*` / `bg-linear-*`, gradient text (`bg-clip-text`), `backdrop-blur-*`, `blur-*`,
glow shadows (`shadow-[0_0_…]`, colored shadows), `transition-all`, `hover:-translate-y-*`,
`hover:scale-*`, infinite decorative animations, `<style>` blocks with hard-coded colors,
`:global(html.light …)` overrides, emoji as UI icons.

## Component classes (app.css)

- Surfaces: `card`, `card-header`, `card-title`, `card-body`
- Buttons: `btn` + one of `btn-primary` `btn-secondary` `btn-ghost` `btn-danger` `btn-danger-ghost`; sizes `btn-sm` `btn-xs`; icon-only `btn-icon` (add `aria-label`)
- Forms: `label`, `input` (also on `<select>` and `<textarea>`), `hint`, `checkbox`
- Badges: `badge` + `badge-success|warning|danger|info|accent`; status dot `<span class="dot"></span>`
- Tables: wrap in `<div class="table-wrap">`, then `<table class="table">` (th/td styled automatically)
- Tabs: `tabs` container, `tab` + `tab-active`; segmented filter: `segmented`, `segment` + `segment-active`
- Misc: `skeleton`, `kbd`, `link`, `code`, `stat-label`, `stat-value`, `progress` + `progress-bar`, `overlay`, `popover`
- Motion helpers: `animate-fade-in`, `animate-pop-in` (opacity/transform only)

## Svelte components (`$components/…`)

- `DashboardLayout` — every authenticated page. Prop `active` is optional (nav highlights from the URL); `wide` for full-width tools.
- `PageHeader` — `title`, `description`, `back` (href), snippets `meta` (badges next to title) and `actions` (buttons).
- `StatCard` — `label`, `value`, `hint`, `icon`, `tone`, `href`.
- `EmptyState` — `icon`, `title`, `description`, `actionLabel` + `onaction`, `compact`.
- `Modal` — `bind:open`, `title`, `maxWidth` (`sm|md|lg|xl`), `onclose`, children + `footer` snippet.
- `ConfirmDialog` — `bind:open`, `title`, `message`, `confirmLabel`, `destructive`, `onconfirm`.
- `StatusPill` — `status` (+ optional `label`); maps common statuses to badge tones.
- `Pagination`, `LoadingSpinner`, `SkeletonLoader`, `ProBadge`, `Logo`.

## Layout & type

- Page = `PageHeader`, then sections separated by `mb-6` / `space-y-6`. Grids `gap-3`/`gap-4`.
- Stats row: `grid grid-cols-2 gap-3 lg:grid-cols-4` of `StatCard`.
- Lists: a `card` with an optional toolbar row (`border-b border-line px-4 py-3`: segmented filter + search input `input h-8 pl-8`), then `table-wrap` > `table`.
- Text: body `text-sm`; secondary 13px `text-[13px] text-muted`; meta `text-xs text-subtle`; section titles `text-sm font-semibold text-fg`; numbers `tabular-nums`; paths/commands `font-mono text-[13px]`.
- Icons: `lucide-svelte`, 14–16px in buttons/rows, 16px in cards.
- Loading: `skeleton` rows shaped like the content, not spinners filling the page. Spinners only inside buttons (`animate-spin` on a `RefreshCw`/`LoaderCircle` icon).
- Mobile (390px) must work: tables inside `table-wrap`, secondary columns `max-sm:hidden`, header actions wrap, modals become bottom sheets automatically.

## Behavior & performance

- Keep every feature, API call and permission check. Restyling must not change behavior.
- Navigation: `<a href>` (client-side), or `goto()` from `$app/navigation` — not `window.location.href = …` (that reloads the whole app). Logout/login redirects may stay as they are.
- Heavy widgets (CodeMirror, xterm, Chart.js) load lazily: `{#await import('…') then m}<m.default … />{/await}` or a dynamic import on first open.
- Polling: skip when `document.hidden`, always clear intervals on destroy.
- `{#each}` blocks over data get a key: `{#each items as it (it.id)}`.
- Inputs get `id` + `<label for>`; icon-only buttons get `aria-label`.
