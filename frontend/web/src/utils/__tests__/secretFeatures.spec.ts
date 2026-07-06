import { describe, it, expect, vi, beforeEach } from 'vitest'

// Controllable gate state — the registry reads these lazily at pick time.
const h = vi.hoisted(() => ({
  standalone: { value: false },
  isAuthenticated: false,
  wallVisible: false,
  gachaVisible: false,
  fanficVisible: false,
}))

vi.mock('@/offline/flag', () => ({ offlineDownloadsEnabled: true }))
vi.mock('@/pwa/standalone', () => ({ useStandaloneDisplay: () => h.standalone }))
vi.mock('@/utils/profileWallGate', () => ({
  useProfileWallVisible: () => ({
    get value() {
      return h.wallVisible
    },
  }),
}))
vi.mock('@/utils/gachaGate', () => ({
  useGachaVisible: () => ({
    get value() {
      return h.gachaVisible
    },
  }),
}))
vi.mock('@/utils/fanficGate', () => ({
  useFanficVisible: () => ({
    get value() {
      return h.fanficVisible
    },
  }),
}))
vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    get isAuthenticated() {
      return h.isAuthenticated
    },
  }),
}))

import {
  pickSecretFeature,
  applySecretFeatureAdminState,
  isRouletteEnabled,
  _resetSecretFeatureForTests,
} from '../secretFeatures'

function rollKeys(n: number, currentPath = '/'): string[] {
  return Array.from({ length: n }, () => pickSecretFeature(currentPath))
    .filter((f): f is NonNullable<typeof f> => f != null)
    .map((f) => f.key)
}

beforeEach(() => {
  _resetSecretFeatureForTests()
  h.standalone.value = false
  h.isAuthenticated = false
  h.wallVisible = false
  h.gachaVisible = false
  h.fanficVisible = false
})

describe('pickSecretFeature', () => {
  it('anonymous browser view → pool is anidle+status+themes+game+downloads', () => {
    const keys = new Set(rollKeys(200))
    expect(keys).toEqual(new Set(['anidle', 'status', 'themes', 'game', 'downloads']))
  })

  it('installed PWA → downloads leaves the pool (it kept its nav link)', () => {
    h.standalone.value = true
    const keys = new Set(rollKeys(200))
    expect(keys).toEqual(new Set(['anidle', 'status', 'themes', 'game']))
  })

  it('authed + wall gate open → showcase editor joins with the deep-link target', () => {
    h.isAuthenticated = true
    h.wallVisible = true
    const picks = Array.from({ length: 300 }, () => pickSecretFeature('/'))
    const editor = picks.find((p) => p?.key === 'showcase-editor')
    expect(editor).toBeDefined()
    expect(editor!.to).toEqual({ path: '/profile', query: { showcase: 'edit' } })
  })

  it('authed but wall gate closed → no showcase editor', () => {
    h.isAuthenticated = true
    const keys = new Set(rollKeys(200))
    expect(keys.has('showcase-editor')).toBe(false)
  })

  it('authed → my-feedback joins the pool (login-only)', () => {
    h.isAuthenticated = true
    const keys = new Set(rollKeys(200))
    expect(keys.has('my-feedback')).toBe(true)
  })

  it('anonymous → my-feedback stays out of the pool', () => {
    const keys = new Set(rollKeys(200))
    expect(keys.has('my-feedback')).toBe(false)
  })

  it('never repeats the previous pick while alternatives exist', () => {
    const keys = rollKeys(100)
    for (let i = 1; i < keys.length; i++) {
      expect(keys[i]).not.toBe(keys[i - 1])
    }
  })

  it('never rolls the page the user is already on', () => {
    const keys = rollKeys(200, '/anidle')
    expect(keys.includes('anidle')).toBe(false)
  })

  it('excludes the current page from the enlarged always-on pool', () => {
    // Standalone (downloads out) on /status: pool {anidle,status,themes,game}
    // minus the current page = {anidle,themes,game}; status never rolls. With
    // 4 unconditional entries the exclusion filters can no longer empty the
    // alternatives, so the relax-on-empty guard in pickSecretFeature is now
    // defensive-only (unreachable via gate state).
    h.standalone.value = true
    const keys = new Set(rollKeys(200, '/status'))
    expect(keys).toEqual(new Set(['anidle', 'themes', 'game']))
  })
})

describe('admin override state', () => {
  it('defaults fail-open: roulette enabled, nothing filtered', () => {
    expect(isRouletteEnabled()).toBe(true)
    const keys = new Set(rollKeys(200))
    expect(keys).toEqual(new Set(['anidle', 'status', 'themes', 'game', 'downloads']))
  })

  it('admin-disabled keys are excluded from the pool', () => {
    applySecretFeatureAdminState({ rouletteEnabled: true, disabledKeys: ['themes', 'game'] })
    const keys = new Set(rollKeys(200))
    expect(keys.has('themes')).toBe(false)
    expect(keys.has('game')).toBe(false)
    expect(keys.has('anidle')).toBe(true)
  })

  it('isRouletteEnabled reflects the master switch', () => {
    applySecretFeatureAdminState({ rouletteEnabled: false, disabledKeys: [] })
    expect(isRouletteEnabled()).toBe(false)
  })

  it('returns null when every eligible feature is admin-disabled', () => {
    applySecretFeatureAdminState({
      rouletteEnabled: true,
      disabledKeys: ['anidle', 'status', 'themes', 'game', 'downloads', 'showcase-editor'],
    })
    expect(pickSecretFeature('/')).toBeNull()
  })

  it('null state resets to the fail-open default', () => {
    applySecretFeatureAdminState({ rouletteEnabled: false, disabledKeys: ['anidle'] })
    applySecretFeatureAdminState(null)
    expect(isRouletteEnabled()).toBe(true)
    expect(new Set(rollKeys(200)).has('anidle')).toBe(true)
  })
})

describe('gacha entry (dark-shipped, admin-only client gate)', () => {
  it('is absent from the pool when gacha is not visible', () => {
    h.gachaVisible = false
    expect(new Set(rollKeys(200)).has('gacha')).toBe(false)
  })

  it('joins the pool once gacha is visible (client eligibility)', () => {
    h.gachaVisible = true
    expect(new Set(rollKeys(200)).has('gacha')).toBe(true)
  })

  it('stays out when admin-disabled even if gacha is visible (its shipped default)', () => {
    h.gachaVisible = true
    applySecretFeatureAdminState({ rouletteEnabled: true, disabledKeys: ['gacha'] })
    expect(new Set(rollKeys(200)).has('gacha')).toBe(false)
  })
})

describe('fanfic entry (dark-shipped, admin-only client gate)', () => {
  it('is absent from the pool when fanfic is not visible', () => {
    h.fanficVisible = false
    expect(new Set(rollKeys(200)).has('fanfic')).toBe(false)
  })

  it('joins the pool once fanfic is visible (client eligibility → /fanfics)', () => {
    h.fanficVisible = true
    const fanfic = Array.from({ length: 200 }, () => pickSecretFeature('/')).find(
      (p) => p?.key === 'fanfic',
    )
    expect(fanfic).toBeDefined()
    expect(fanfic!.to).toBe('/fanfics')
  })

  it('stays out when admin-disabled even if fanfic is visible', () => {
    h.fanficVisible = true
    applySecretFeatureAdminState({ rouletteEnabled: true, disabledKeys: ['fanfic'] })
    expect(new Set(rollKeys(200)).has('fanfic')).toBe(false)
  })
})
