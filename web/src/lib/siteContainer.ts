// The Docker container of a site. Docker sites run as sp-site-<id>; older
// sites used site-<name>. Exact names only: a partial match (site name or
// domain contained in another container's name) could show, stop or remove
// another tenant's container.
export function siteContainerNames(site: { id: number; name?: string }): string[] {
  const names = [`sp-site-${site.id}`];
  if (site.name) names.push(`site-${String(site.name).replace(/ /g, '-')}`);
  return names;
}

export function findSiteContainer<T extends { name?: string }>(list: T[], site: { id: number; name?: string }): T | undefined {
  const names = siteContainerNames(site);
  for (const n of names) {
    const c = list.find((x) => (x.name || '').replace(/^\//, '') === n);
    if (c) return c;
  }
  return undefined;
}
