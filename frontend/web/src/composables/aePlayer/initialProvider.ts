import type { ProviderRow } from '@/types/aePlayer'

/**
 * Decide whether a notification deep-link's `?provider=` value should pin the
 * aePlayer source. Honored ONLY when it names a real provider row that is
 * currently `active` — coarse/legacy values (e.g. 'english') and
 * unavailable/inactive providers return null so the smart default picks.
 *
 * Pure + sync so it is unit-testable without mounting UnifiedPlayer.vue.
 */
export function pickInitialProvider(
  initialProvider: string | undefined | null,
  rows: ProviderRow[],
): string | null {
  if (!initialProvider) return null
  return rows.some((r) => r.def.id === initialProvider && r.state === 'active')
    ? initialProvider
    : null
}
