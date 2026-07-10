import { adminApi } from '@/api/client'
import type { ScraperProviderPolicy, ScraperProviderWire, ScraperProvidersResponse } from '@/api/client'

// RBAC-and-roulette P5 (Task 2) — admin composable for the Providers tab,
// mirroring useAdminPolicy.ts's shape.
//
// Backend contract (services/catalog, gateway-proxied at
// /api/admin/scraper-providers*, admin-JWT-gated — Task 1
// admin_scraper_providers.go):
//   GET /api/admin/scraper-providers            -> {success,data:{providers}}
//   PUT /api/admin/scraper-providers/{name}/policy -> {success,data:<wire>}
//
// Mirrors useAdminRecs.ts / useAdminFeedback.ts: responses are wrapped in
// {success,data} via httputil.OK, so we unwrap `res.data?.data ?? res.data`.

export type { ScraperProviderPolicy, ScraperProviderWire, ScraperProvidersResponse }

function unwrap<T>(data: unknown): T {
  const d = data as { data?: T }
  return d && typeof d === 'object' && 'data' in d ? (d.data as T) : (data as T)
}

export function useAdminProviders() {
  async function list(): Promise<ScraperProviderWire[]> {
    const res = await adminApi.listScraperProviders()
    return unwrap<ScraperProvidersResponse>(res.data).providers
  }

  // All three policy values are admin levers — nothing is machine-set
  // (probe auto demote/promote retired 2026-07-08). `manual` parks the
  // provider out of auto-failover while keeping it manually selectable.
  async function setPolicy(name: string, policy: ScraperProviderPolicy): Promise<ScraperProviderWire> {
    const res = await adminApi.setScraperProviderPolicy(name, policy)
    return unwrap<ScraperProviderWire>(res.data)
  }

  return { list, setPolicy }
}
