# SigardaPanel Web (SvelteKit)

Frontend dashboard untuk SigardaPanel, dibangun dengan SvelteKit + Svelte 5 + Tailwind CSS.

## Development

```bash
cd web
npm install
npm run dev
```

Default: `http://localhost:7720` → proxy ke API `http://localhost:8090`.

## Build Production

```bash
npx vite build    # output di build/
node build        # jalankan server
```

## Validasi

```bash
npx svelte-check --tsconfig ./tsconfig.json
```

## Struktur

- `src/lib/` — auth store, api helper, format utils, types
- `src/components/` — Sidebar, DashboardLayout, StatusPill, dll
- `src/routes/` — halaman (file-based routing)
- `src/routes/api/proxy/[...path]/` — proxy ke Go backend

## Catatan

- Frontend React/Next.js versi lama ada di `../web-react/` untuk referensi
- Backend Go tidak diubah — API compatible 100%
- Auth: Bearer token via `Authorization` header
