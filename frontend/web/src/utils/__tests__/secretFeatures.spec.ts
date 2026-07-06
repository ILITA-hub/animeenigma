import { describe, it, expect, vi, beforeEach } from 'vitest'

// Controllable gate state — the registry reads these lazily at pick time.
const h = vi.hoisted(() => ({
  standalone: { value: false },
  isAuthenticated: false,
  wallVisible: false,
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
vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    get isAuthenticated() {
      return h.isAuthenticated
    },
  }),
}))

import { pickSecretFeature, _resetSecretFeatureForTests } from '../secretFeatures'

function rollKeys(n: number, currentPath = '/'): string[] {
  return Array.from({ length: n }, () => pickSecretFeature(currentPath).key)
}

beforeEach(() => {
  _resetSecretFeatureForTests()
  h.standalone.value = false
  h.isAuthenticated = false
  h.wallVisible = false
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
    const editor = picks.find((p) => p.key === 'showcase-editor')
    expect(editor).toBeDefined()
    expect(editor!.to).toEqual({ path: '/profile', query: { showcase: 'edit' } })
  })

  it('authed but wall gate closed → no showcase editor', () => {
    h.isAuthenticated = true
    const keys = new Set(rollKeys(200))
    expect(keys.has('showcase-editor')).toBe(false)
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
