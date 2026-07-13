import { adminApi } from '@/api/client'
import type { ScraperProviderWire, ScraperProvidersResponse } from '@/api/client'

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

export type { ScraperProviderWire, ScraperProvidersResponse }

function unwrap<T>(data: unknown): T {
  const d = data as { data?: T }
  return d && typeof d === 'object' && 'data' in d ? (d.data as T) : (data as T)
}

export function useAdminProviders() {
  async function list(): Promise<ScraperProviderWire[]> {
    const res = await adminApi.listScraperProviders()
    return unwrap<ScraperProvidersResponse>(res.data).providers
  }

  // As of 2026-07-13 the admin sends only the probe status: 'auto' (re-enable +
  // hand back to the machine) or 'disabled' (hard lock). The auto↔manual
  // failover axis is machine-set from health server-side, so the returned wire
  // may read policy 'manual' even when 'auto' was sent (a still-down provider
  // re-enables as parked). 'manual' is never a valid admin input.
  async function setPolicy(name: string, policy: 'auto' | 'disabled'): Promise<ScraperProviderWire> {
    const res = await adminApi.setScraperProviderPolicy(name, policy)
    return unwrap<ScraperProviderWire>(res.data)
  }

  return { list, setPolicy }
}
