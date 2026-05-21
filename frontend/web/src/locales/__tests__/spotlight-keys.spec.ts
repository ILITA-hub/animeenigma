import { describe, it, expect } from 'vitest'
import en from '../en.json'
import ru from '../ru.json'

// Recursively collect every leaf key path in an object, e.g.
// { a: { b: 'x', c: 'y' } } -> ['a.b', 'a.c']
function leafPaths(obj: unknown, prefix = ''): string[] {
  if (obj === null || typeof obj !== 'object') return [prefix]
  return Object.entries(obj as Record<string, unknown>).flatMap(([k, v]) => {
    const next = prefix ? `${prefix}.${k}` : k
    return leafPaths(v, next)
  })
}

// Walk to a value via dot-path. Returns undefined if any segment missing.
function get(obj: unknown, path: string): unknown {
  return path.split('.').reduce<unknown>((acc, seg) => {
    if (acc == null || typeof acc !== 'object') return undefined
    return (acc as Record<string, unknown>)[seg]
  }, obj)
}

describe('spotlight i18n parity', () => {
  const enSpotlight = (en as Record<string, unknown>).spotlight
  const ruSpotlight = (ru as Record<string, unknown>).spotlight

  it('en.json has a top-level spotlight object', () => {
    expect(enSpotlight).toBeTypeOf('object')
    expect(enSpotlight).not.toBeNull()
  })

  it('ru.json has a top-level spotlight object', () => {
    expect(ruSpotlight).toBeTypeOf('object')
    expect(ruSpotlight).not.toBeNull()
  })

  it('en and ru spotlight key sets are identical', () => {
    const enKeys = leafPaths(enSpotlight).sort()
    const ruKeys = leafPaths(ruSpotlight).sort()
    expect(ruKeys).toEqual(enKeys)
  })

  it('every spotlight.* leaf value is a non-empty string in en.json', () => {
    const paths = leafPaths(enSpotlight)
    for (const p of paths) {
      const v = get(enSpotlight, p)
      expect(typeof v, `en spotlight.${p}`).toBe('string')
      expect((v as string).trim().length, `en spotlight.${p} non-empty`).toBeGreaterThan(0)
    }
  })

  it('every spotlight.* leaf value is a non-empty string in ru.json', () => {
    const paths = leafPaths(ruSpotlight)
    for (const p of paths) {
      const v = get(ruSpotlight, p)
      expect(typeof v, `ru spotlight.${p}`).toBe('string')
      expect((v as string).trim().length, `ru spotlight.${p} non-empty`).toBeGreaterThan(0)
    }
  })

  // Specific expected keys — explicit assertions so a deletion is surfaced.
  const expectedTopLevel = [
    'regionLabel',
    'slideLabel',
    'slideLabelWithTitle',
    'prevSlide',
    'nextSlide',
    'goToSlide',
    'pauseAutoplay',
  ] as const

  it.each(expectedTopLevel)('en.json has spotlight.%s', (key) => {
    expect(typeof (enSpotlight as Record<string, unknown>)[key]).toBe('string')
  })

  it.each(expectedTopLevel)('ru.json has spotlight.%s', (key) => {
    expect(typeof (ruSpotlight as Record<string, unknown>)[key]).toBe('string')
  })

  const expectedSubNamespaces = [
    'animeOfDay',
    'randomTail',
    'latestNews',
    'platformStats',
    'personalPick',
    'telegramNews',
    'nowWatching',
    'notTimeYet',
    'continueWatchingNew',
  ] as const

  it.each(expectedSubNamespaces)('en.json has spotlight.%s sub-namespace', (ns) => {
    expect((enSpotlight as Record<string, unknown>)[ns]).toBeTypeOf('object')
  })

  it.each(expectedSubNamespaces)('ru.json has spotlight.%s sub-namespace', (ns) => {
    expect((ruSpotlight as Record<string, unknown>)[ns]).toBeTypeOf('object')
  })

  const animeOfDayKeys = ['title', 'watchCta', 'addCta', 'scoreLabel', 'episodesLabel'] as const

  it.each(animeOfDayKeys)('spotlight.animeOfDay.%s present in both locales', (k) => {
    expect(typeof (enSpotlight as Record<string, Record<string, unknown>>).animeOfDay?.[k]).toBe('string')
    expect(typeof (ruSpotlight as Record<string, Record<string, unknown>>).animeOfDay?.[k]).toBe('string')
  })

  const randomTailKeys = ['title', 'subtitle', 'discoverCta'] as const
  it.each(randomTailKeys)('spotlight.randomTail.%s present in both locales', (k) => {
    expect(typeof (enSpotlight as Record<string, Record<string, unknown>>).randomTail?.[k]).toBe('string')
    expect(typeof (ruSpotlight as Record<string, Record<string, unknown>>).randomTail?.[k]).toBe('string')
  })

  const latestNewsKeys = ['title', 'readMore', 'entryDate'] as const
  it.each(latestNewsKeys)('spotlight.latestNews.%s present in both locales', (k) => {
    expect(typeof (enSpotlight as Record<string, Record<string, unknown>>).latestNews?.[k]).toBe('string')
    expect(typeof (ruSpotlight as Record<string, Record<string, unknown>>).latestNews?.[k]).toBe('string')
  })

  // platformStats sub-keys — camelCase matches the camelize() helper that
  // PlatformStatsCard applies to the backend's snake_case `m.key` values
  // (Plan 02-03 SUMMARY decision #1).
  const platformStatsKeys = [
    'title',
    'animeAdded7d',
    'episodesAdded7d',
    'activeRooms7d',
    'deltaPositive',
    'noChange',
  ] as const
  it.each(platformStatsKeys)('spotlight.platformStats.%s present in both locales', (k) => {
    expect(typeof (enSpotlight as Record<string, Record<string, unknown>>).platformStats?.[k]).toBe('string')
    expect(typeof (ruSpotlight as Record<string, Record<string, unknown>>).platformStats?.[k]).toBe('string')
  })

  // ── Phase 3 (Plan 03-05) ─ five new sub-namespaces ──────────────────────
  const personalPickKeys = ['title', 'titleAnon', 'moreLink'] as const
  it.each(personalPickKeys)('spotlight.personalPick.%s present in both locales', (k) => {
    expect(typeof (enSpotlight as Record<string, Record<string, unknown>>).personalPick?.[k]).toBe('string')
    expect(typeof (ruSpotlight as Record<string, Record<string, unknown>>).personalPick?.[k]).toBe('string')
  })

  it('spotlight.personalPick.moreLink preserves {n} interpolation', () => {
    expect((enSpotlight as Record<string, Record<string, string>>).personalPick.moreLink).toContain('{n}')
    expect((ruSpotlight as Record<string, Record<string, string>>).personalPick.moreLink).toContain('{n}')
  })

  const telegramNewsKeys = ['title', 'openCta'] as const
  it.each(telegramNewsKeys)('spotlight.telegramNews.%s present in both locales', (k) => {
    expect(typeof (enSpotlight as Record<string, Record<string, unknown>>).telegramNews?.[k]).toBe('string')
    expect(typeof (ruSpotlight as Record<string, Record<string, unknown>>).telegramNews?.[k]).toBe('string')
  })

  const nowWatchingKeys = ['title', 'sessionLabel', 'liveBadge'] as const
  it.each(nowWatchingKeys)('spotlight.nowWatching.%s present in both locales', (k) => {
    expect(typeof (enSpotlight as Record<string, Record<string, unknown>>).nowWatching?.[k]).toBe('string')
    expect(typeof (ruSpotlight as Record<string, Record<string, unknown>>).nowWatching?.[k]).toBe('string')
  })

  it('spotlight.nowWatching.sessionLabel preserves {username},{anime},{n} interpolation', () => {
    const en = (enSpotlight as Record<string, Record<string, string>>).nowWatching.sessionLabel
    const ru = (ruSpotlight as Record<string, Record<string, string>>).nowWatching.sessionLabel
    for (const tpl of [en, ru]) {
      expect(tpl).toContain('{username}')
      expect(tpl).toContain('{anime}')
      expect(tpl).toContain('{n}')
    }
  })

  const notTimeYetKeys = ['title', 'subtitlePlanned', 'subtitlePostponed', 'watchCta'] as const
  it.each(notTimeYetKeys)('spotlight.notTimeYet.%s present in both locales', (k) => {
    expect(typeof (enSpotlight as Record<string, Record<string, unknown>>).notTimeYet?.[k]).toBe('string')
    expect(typeof (ruSpotlight as Record<string, Record<string, unknown>>).notTimeYet?.[k]).toBe('string')
  })

  const continueWatchingNewKeys = ['title', 'newEpisodeBadge', 'resumeCta'] as const
  it.each(continueWatchingNewKeys)('spotlight.continueWatchingNew.%s present in both locales', (k) => {
    expect(typeof (enSpotlight as Record<string, Record<string, unknown>>).continueWatchingNew?.[k]).toBe('string')
    expect(typeof (ruSpotlight as Record<string, Record<string, unknown>>).continueWatchingNew?.[k]).toBe('string')
  })

  it('spotlight.continueWatchingNew.newEpisodeBadge preserves {n} interpolation', () => {
    expect((enSpotlight as Record<string, Record<string, string>>).continueWatchingNew.newEpisodeBadge).toContain('{n}')
    expect((ruSpotlight as Record<string, Record<string, string>>).continueWatchingNew.newEpisodeBadge).toContain('{n}')
  })
})
