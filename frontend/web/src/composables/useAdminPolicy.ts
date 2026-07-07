import { adminApi } from '@/api/client'
import type { FeatureFlag, FailSafe, FeatureFlagPayload, PolicyFlagsResponse } from '@/api/client'

// RBAC-and-roulette P1 — policy admin composable (Task 6) consumed by
// AdminPolicy.vue (Task 7).
//
// Backend contract (services/policy, proxied via gateway /api/admin/policy/*,
// admin-JWT-gated):
//   GET /api/admin/policy/flags       -> {success,data:{flags,rouletteEnabled}}
//   PUT /api/admin/policy/flags/{key} -> {success,data:{key}}
//   PUT /api/admin/policy/roulette    -> {success,data:{enabled}}
//
// Mirrors useAdminRecs.ts / useAdminFeedback.ts: responses are wrapped in
// {success,data} via httputil.OK, so we unwrap `res.data?.data ?? res.data`.

export type { FeatureFlag, FailSafe, FeatureFlagPayload, PolicyFlagsResponse }

function unwrap<T>(data: unknown): T {
  const d = data as { data?: T }
  return d && typeof d === 'object' && 'data' in d ? (d.data as T) : (data as T)
}

export function useAdminPolicy() {
  async function list(): Promise<PolicyFlagsResponse> {
    const res = await adminApi.getPolicyFlags()
    return unwrap<PolicyFlagsResponse>(res.data)
  }

  async function setFlag(key: string, payload: FeatureFlagPayload): Promise<void> {
    await adminApi.setPolicyFlag(key, payload)
  }

  async function setRoulette(enabled: boolean): Promise<void> {
    await adminApi.setPolicyRoulette(enabled)
  }

  return { list, setFlag, setRoulette }
}
