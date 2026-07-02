// Pure helpers for src/sw.ts — kept out of the SW bundle entry so vitest can
// exercise them under jsdom without a ServiceWorkerGlobalScope.

/** Navigations that must NEVER get the SPA shell. /admin/feedback etc. are SPA
 *  routes; only the four proxied infra dashboards are denied. */
export const NAV_DENYLIST: RegExp[] = [
  /^\/api\//,
  /^\/og\//,
  /^\/socket\.io/,
  /^\/__offline\//,
  /^\/admin\/(grafana|prometheus|pgadmin|k8s)(\/|$)/,
  /^\/health$/,
  /^\/sw\.js$/,
  /^\/sw-config\.json$/,
]

/** RU static-edge (Maskanya) chunk request → equivalent origin URL, so the SW
 *  can serve the precached origin copy when the edge is unreachable offline.
 *  Null ⇒ not an edge asset request (same-origin or unrelated host/path). */
export function edgeAssetToOriginPath(requestUrl: string, origin: string): string | null {
  try {
    const u = new URL(requestUrl)
    if (u.origin === origin) return null
    if (!u.pathname.startsWith('/assets/')) return null
    return origin + u.pathname
  } catch {
    return null
  }
}

export function isOfflinePath(pathname: string): boolean {
  return pathname.startsWith('/__offline/')
}
