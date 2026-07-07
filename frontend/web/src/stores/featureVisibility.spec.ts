import { beforeEach, describe, expect, it, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useFeatureVisibilityStore } from './featureVisibility'
import type { FeaturesMineResponse } from '@/api/client'

// ── Mock the api client ──────────────────────────────────────────────────────
// vi.mock is hoisted, so the factory can't reference module-level vars that
// haven't been initialised yet — mirrors stores/gacha.spec.ts.
vi.mock('@/api/client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api/client')>()
  return {
    ...actual,
    featuresApi: {
      getFeaturesMine: vi.fn(),
    },
  }
})

function mineResponse(override?: Partial<FeaturesMineResponse>): FeaturesMineResponse {
  return {
    rouletteEnabled: true,
    visible: ['anidle', 'themes'],
    roulette: ['anidle', 'themes'],
    ...override,
  }
}

describe('featureVisibility store', () => {
  beforeEach(async () => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    const { featuresApi } = await import('@/api/client')
    vi.mocked(featuresApi.getFeaturesMine).mockResolvedValue(mineResponse())
  })

  it('load() GETs /policy/features/mine and populates visible/roulette/rouletteEnabled, sets loaded=true', async () => {
    const { featuresApi } = await import('@/api/client')
    const store = useFeatureVisibilityStore()

    expect(store.loaded).toBe(false)
    expect(store.visible.size).toBe(0)

    await store.load()

    expect(featuresApi.getFeaturesMine).toHaveBeenCalledTimes(1)
    expect(store.loaded).toBe(true)
    expect(store.visible).toEqual(new Set(['anidle', 'themes']))
    expect(store.roulette).toEqual(['anidle', 'themes'])
    expect(store.rouletteEnabled).toBe(true)
  })

  it('resolves ready after a successful load()', async () => {
    const store = useFeatureVisibilityStore()
    void store.load()
    await expect(store.ready).resolves.toBeUndefined()
    expect(store.loaded).toBe(true)
  })

  it('a rejected request leaves loaded=false but STILL resolves ready', async () => {
    const { featuresApi } = await import('@/api/client')
    vi.mocked(featuresApi.getFeaturesMine).mockRejectedValue(new Error('network down'))
    const store = useFeatureVisibilityStore()

    await store.load()

    expect(store.loaded).toBe(false)
    expect(store.visible.size).toBe(0)
    await expect(store.ready).resolves.toBeUndefined()
  })

  it('double load() fetches only once (concurrent calls share the in-flight fetch)', async () => {
    const { featuresApi } = await import('@/api/client')
    const store = useFeatureVisibilityStore()

    const p1 = store.load()
    const p2 = store.load()
    await Promise.all([p1, p2])

    // Pinia wraps action return values (its own .then() chain for $onAction
    // hooks), so the two promises aren't reference-equal even though they
    // share one in-flight fetch — assert on the thing that actually
    // matters: the API was hit exactly once.
    expect(featuresApi.getFeaturesMine).toHaveBeenCalledTimes(1)
  })

  it('double load() after settling still fetches only once', async () => {
    const { featuresApi } = await import('@/api/client')
    const store = useFeatureVisibilityStore()

    await store.load()
    await store.load()

    expect(featuresApi.getFeaturesMine).toHaveBeenCalledTimes(1)
  })
})
