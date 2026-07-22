import { describe, it, expect, vi, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'

// Prevent Axios from actually constructing during import of the
// featureVisibility store (pulls in @/api/client) — mirrors
// composables/useFeatureVisible.spec.ts / stores/featureVisibility.spec.ts.
vi.mock('@/api/client', () => ({
  apiClient: { defaults: { baseURL: '/api' } },
  featuresApi: { getFeaturesMine: vi.fn() },
}))

import {
  pickSecretFeature,
  roulettePoolAvailable,
  wantsNewTab,
  _resetSecretFeatureForTests,
} from '../secretFeatures'
import { useFeatureVisibilityStore } from '@/stores/featureVisibility'

function rollKeys(n: number, currentPath = '/'): string[] {
  return Array.from({ length: n }, () => pickSecretFeature(currentPath))
    .filter((f): f is NonNullable<typeof f> => f != null)
    .map((f) => f.key)
}

let store: ReturnType<typeof useFeatureVisibilityStore>

beforeEach(() => {
  setActivePinia(createPinia())
  _resetSecretFeatureForTests()
  store = useFeatureVisibilityStore()
  store.rouletteEnabled = true
  store.roulette = []
})

describe('pickSecretFeature — pool is store.roulette (server-resolved), not client eligibility', () => {
  it('only ever returns entries whose key is in store.roulette', () => {
    store.roulette = ['anidle', 'themes']
    const keys = new Set(rollKeys(200, '/x'))
    expect(keys).toEqual(new Set(['anidle', 'themes']))
  })

  it('a wider server pool (e.g. dark-shipped fanfic/gacha included for this user) rolls those too', () => {
    store.roulette = ['anidle', 'gacha', 'fanfic']
    const keys = new Set(rollKeys(300, '/x'))
    expect(keys).toEqual(new Set(['anidle', 'gacha', 'fanfic']))
  })

  it('ignores a roulette key the frontend registry does not know about', () => {
    store.roulette = ['anidle', 'some-future-key']
    const keys = new Set(rollKeys(200, '/x'))
    expect(keys).toEqual(new Set(['anidle']))
  })

  it('empty pool → returns null', () => {
    store.roulette = []
    expect(pickSecretFeature('/x')).toBeNull()
  })

  it('a pool with only server-disabled keys (none in roulette) → returns null', () => {
    store.roulette = []
    expect(pickSecretFeature('/anidle')).toBeNull()
  })

  it('never rolls the page the user is already on', () => {
    store.roulette = ['anidle', 'themes']
    const keys = rollKeys(200, '/anidle')
    expect(keys.includes('anidle')).toBe(false)
    expect(new Set(keys)).toEqual(new Set(['themes']))
  })

  it('never repeats the previous pick while alternatives exist', () => {
    store.roulette = ['anidle', 'themes', 'game']
    const keys = rollKeys(100, '/x')
    for (let i = 1; i < keys.length; i++) {
      expect(keys[i]).not.toBe(keys[i - 1])
    }
  })

  it('the showcase-editor entry keeps its deep-link target when the server includes it', () => {
    store.roulette = ['showcase-editor']
    const pick = pickSecretFeature('/')
    expect(pick).not.toBeNull()
    expect(pick!.to).toEqual({ path: '/profile', query: { showcase: 'edit' } })
  })

  it('routes the following secret feature to its hidden page', () => {
    store.roulette = ['following']
    expect(pickSecretFeature('/')?.to).toBe('/following')
  })

  it('routes personal recommendations to the hidden authenticated page', () => {
    store.roulette = ['recommendations']
    expect(pickSecretFeature('/')?.to).toBe('/recs')
  })

  it('routes the Zundamon voice lab to its hidden browser-only page', () => {
    store.roulette = ['zundamon-tts']
    expect(pickSecretFeature('/')?.to).toBe('/zundamon')
  })
})

describe('roulettePoolAvailable — footer button dead-affordance guard (P4 Task 3 review fix)', () => {
  it('false when store.roulette is empty (e.g. total policy-feed outage) even though the master switch fails open', () => {
    store.rouletteEnabled = true
    store.roulette = []
    expect(roulettePoolAvailable()).toBe(false)
  })

  it('true when store.roulette has at least one key the client registry knows', () => {
    store.rouletteEnabled = true
    store.roulette = ['anidle']
    expect(roulettePoolAvailable()).toBe(true)
  })

  it('false when store.roulette only has keys the client registry does not know', () => {
    store.roulette = ['some-future-key']
    expect(roulettePoolAvailable()).toBe(false)
  })

  it('the button-visibility gate is rouletteEnabled AND roulettePoolAvailable — either false hides it', () => {
    store.rouletteEnabled = false
    store.roulette = ['anidle']
    expect(store.rouletteEnabled && roulettePoolAvailable()).toBe(false)

    store.rouletteEnabled = true
    store.roulette = []
    expect(store.rouletteEnabled && roulettePoolAvailable()).toBe(false)

    store.rouletteEnabled = true
    store.roulette = ['anidle']
    expect(store.rouletteEnabled && roulettePoolAvailable()).toBe(true)
  })
})

describe('wantsNewTab — the roulette button honours open-in-new-tab gestures', () => {
  const ev = (init: Partial<MouseEvent>): MouseEvent =>
    ({ button: 0, metaKey: false, ctrlKey: false, ...init }) as MouseEvent

  it('plain left click → false (navigate in place)', () => {
    expect(wantsNewTab(ev({ button: 0 }))).toBe(false)
  })

  it('Cmd (meta) + left click → true (macOS new-tab gesture)', () => {
    expect(wantsNewTab(ev({ button: 0, metaKey: true }))).toBe(true)
  })

  it('Ctrl + left click → true (Windows/Linux new-tab gesture)', () => {
    expect(wantsNewTab(ev({ button: 0, ctrlKey: true }))).toBe(true)
  })

  it('middle click (button 1) → true', () => {
    expect(wantsNewTab(ev({ button: 1 }))).toBe(true)
  })

  it('right click (button 2) → false (no new tab, not even with a modifier)', () => {
    expect(wantsNewTab(ev({ button: 2, ctrlKey: true }))).toBe(false)
  })

  it('keyboard activation (button 0, no modifiers) → false', () => {
    expect(wantsNewTab(ev({ button: 0 }))).toBe(false)
  })
})
