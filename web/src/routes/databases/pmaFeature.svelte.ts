import { apiFetch } from '$lib/api';
import { auth } from '$lib/auth.svelte';

/** Whether phpMyAdmin is installed on this panel (installer: `install.sh phpmyadmin`). */
export const pmaFeature = $state({ available: false });

let loading: Promise<void> | null = null;

export function loadPmaFeature(): Promise<void> {
  loading ??= apiFetch(auth.token, 'GET', 'databases/features')
    .then((d) => { pmaFeature.available = !!d?.phpmyadmin; })
    .catch(() => { loading = null; });
  return loading;
}
