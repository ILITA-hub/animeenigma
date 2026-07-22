import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'

// Prevent Axios from actually constructing during import of the
// featureVisibility store (pulls in @/api/client) — mirrors
// useFeatureVisible.spec.ts / stores/featureVisibility.spec.ts /
// utils/__tests__/secretFeatures.spec.ts.
vi.mock('@/api/client', () => ({
  apiClient: { defaults: { baseURL: '/api' } },
  featuresApi: { getFeaturesMine: vi.fn() },
}))

import { useFeatureVisible } from '@/composables/useFeatureVisible'
import { useFeatureVisibilityStore } from '@/stores/featureVisibility'
import { pickSecretFeature, roulettePoolAvailable, _resetSecretFeatureForTests } from '@/utils/secretFeatures'

/**
 * Day-one parity regression (RBAC-and-roulette P4 Task 4, spec D2).
 *
 * Seeds `stores/featureVisibility.ts` with the exact `mine` response a
 * freshly-seeded backend produces for three identities — matching the
 * CORRECTED seed in `services/policy/internal/domain/feature_flag.go`
 * `SeedFlags()` after the Task-3 parity fix (fanfic/showcase-editor roll
 * for admins; my-feedback is authed-only, NOT everyone; gacha is
 * roulette:false) — and asserts FE visibility + the footer roulette pool
 * reproduce today's pre-RBAC behavior exactly for each identity. This is
 * the regression lock for D2; a manual check on a freshly-seeded backend
 * happens at deploy.
 *
 * Note: `useFeatureVisible` only consults `authStore.isAdmin` when the feed
 * hasn't loaded (D1 fail-open fallback). Every seed below sets
 * `store.loaded = true`, so the assertions below exercise the "feed
 * loaded" path only — the resolved `store.visible` set alone drives the
 * outcome regardless of the current auth identity.
 */

const ALL_TWELVE_KEYS = [
  'fanfic',
  'profile-wall',
  'gacha',
  'anidle',
  'status',
  'themes',
  'game',
  'downloads',
  'showcase-editor',
  'my-feedback',
  'following',
  'recommendations',
]

function seed(
  store: ReturnType<typeof useFeatureVisibilityStore>,
  visible: string[],
  roulette: string[],
): void {
  store.loaded = true
  store.visible = new Set(visible)
  store.roulette = roulette
  store.rouletteEnabled = true
}

/** Roll the footer roulette N times and collect the distinct keys returned. */
function rolledKeys(n: number, currentPath = '/x'): Set<string> {
  return new Set(
    Array.from({ length: n }, () => pickSecretFeature(currentPath))
      .filter((f): f is NonNullable<typeof f> => f != null)
      .map((f) => f.key),
  )
}

describe('feature-visibility day-one parity (D2)', () => {
  let store: ReturnType<typeof useFeatureVisibilityStore>

  beforeEach(() => {
    setActivePinia(createPinia())
    _resetSecretFeatureForTests()
    store = useFeatureVisibilityStore()
  })

  it('admin — sees all 12 keys; roulette pool is exactly the 10 roulette-enabled keys (gacha + profile-wall excluded)', () => {
    const roulette = [
      'anidle',
      'status',
      'themes',
      'game',
      'downloads',
      'fanfic',
      'showcase-editor',
      'my-feedback',
      'following',
      'recommendations',
    ]
    seed(store, ALL_TWELVE_KEYS, roulette)

    expect(useFeatureVisible('fanfic').value).toBe(true)
    expect(useFeatureVisible('gacha').value).toBe(true)
    expect(useFeatureVisible('profile-wall').value).toBe(true)

    expect(roulettePoolAvailable()).toBe(true)
    const keys = rolledKeys(300)
    expect(keys.size).toBeGreaterThan(0)
    for (const k of keys) expect(roulette).toContain(k)
    // gacha (visible but roulette:false) and profile-wall (no route, admin
    // dark-ship, not in the footer registry) must never come up.
    expect(keys.has('gacha')).toBe(false)
    expect(keys.has('profile-wall')).toBe(false)
  })

  it('user — sees the 8 public/authenticated keys; fanfic/gacha/profile-wall stay hidden; pool = those 8 only', () => {
    const visible = ['anidle', 'status', 'themes', 'game', 'downloads', 'my-feedback', 'following', 'recommendations']
    seed(store, visible, visible)

    expect(useFeatureVisible('fanfic').value).toBe(false)
    expect(useFeatureVisible('gacha').value).toBe(false)
    expect(useFeatureVisible('profile-wall').value).toBe(false)

    expect(roulettePoolAvailable()).toBe(true)
    const keys = rolledKeys(300)
    expect(keys.size).toBeGreaterThan(0)
    for (const k of keys) expect(visible).toContain(k)
    expect(keys.has('fanfic')).toBe(false)
    expect(keys.has('showcase-editor')).toBe(false)
  })

  it('anon — sees only the 5 everyone keys; fanfic/gacha/profile-wall stay hidden; pool = those 5 only (no my-feedback, no showcase-editor — the parity fix)', () => {
    const visible = ['anidle', 'status', 'themes', 'game', 'downloads']
    seed(store, visible, visible)

    expect(useFeatureVisible('fanfic').value).toBe(false)
    expect(useFeatureVisible('gacha').value).toBe(false)
    expect(useFeatureVisible('profile-wall').value).toBe(false)

    expect(roulettePoolAvailable()).toBe(true)
    const keys = rolledKeys(300)
    expect(keys.size).toBeGreaterThan(0)
    for (const k of keys) expect(visible).toContain(k)
    expect(keys.has('my-feedback')).toBe(false)
    expect(keys.has('showcase-editor')).toBe(false)
    expect(keys.has('fanfic')).toBe(false)
  })
})
