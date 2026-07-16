/**
 * Vitest spec for useContentVerify() — content-verify probing Task 13.
 * Mocks `@/api/client`'s contentVerifyApi and drives the poll lifecycle with
 * fake timers: immediate fetch on activation, re-poll every pollMs while
 * `active` stays true, and a hard stop (keeping the last report) once
 * `active` flips false.
 */
import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { nextTick, ref } from 'vue'

const getMock = vi.fn()
vi.mock('@/api/client', async (importOriginal) => {
  const orig = await importOriginal<typeof import('@/api/client')>()
  return { ...orig, contentVerifyApi: { get: (id: string) => getMock(id) } }
})

import { normalizeVerify, useContentVerify } from './useContentVerify'

describe('normalizeVerify', () => {
  it('maps the wire array to a provider record', () => {
    const raw = {
      anime_id: 'a1',
      providers: [
        { provider: 'gogoanime', summary: { status: 'partial', raw: true, dub_langs: ['en'], hardsub_langs: [] }, units: [] },
      ],
    }
    const rep = normalizeVerify(raw)!
    expect(rep.animeId).toBe('a1')
    expect(rep.providers.gogoanime.dub_langs).toEqual(['en'])
    expect(rep.providers.gogoanime.status).toBe('partial')
    expect(rep.providers.gogoanime.raw).toBe(true)
    expect(rep.providers.gogoanime.units).toEqual([])
  })

  it('defaults units to [] when the wire entry omits them', () => {
    const raw = {
      anime_id: 'a1',
      providers: [
        { provider: 'kodik', summary: { status: 'unverified', raw: false, dub_langs: [], hardsub_langs: [] } },
      ],
    }
    const rep = normalizeVerify(raw)!
    expect(rep.providers.kodik.units).toEqual([])
  })

  it('skips malformed provider entries (missing provider id or summary)', () => {
    const raw = {
      anime_id: 'a1',
      providers: [
        { summary: { status: 'unverified', raw: false, dub_langs: [], hardsub_langs: [] } },
        { provider: 'gogoanime' },
        { provider: 'ok', summary: { status: 'unverified', raw: false, dub_langs: [], hardsub_langs: [] } },
      ],
    }
    const rep = normalizeVerify(raw)!
    expect(Object.keys(rep.providers)).toEqual(['ok'])
  })

  it('returns null on garbage', () => {
    expect(normalizeVerify(null)).toBeNull()
    expect(normalizeVerify({})).toBeNull()
    expect(normalizeVerify({ anime_id: 'a1' })).toBeNull()
    expect(normalizeVerify({ providers: [] })).toBeNull()
  })
})

describe('useContentVerify', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    getMock.mockReset().mockResolvedValue({ data: { data: { anime_id: 'a1', providers: [] } } })
  })
  afterEach(() => {
    vi.useRealTimers()
  })

  it('fetches immediately and re-polls every pollMs while active', async () => {
    const active = ref(true)
    useContentVerify(ref('a1'), active) // component-less setup: composable guards lifecycle hooks
    await vi.advanceTimersByTimeAsync(0)
    expect(getMock).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(45000)
    expect(getMock).toHaveBeenCalledTimes(2)
    active.value = false
    await nextTick()
    await vi.advanceTimersByTimeAsync(90000)
    expect(getMock).toHaveBeenCalledTimes(2) // stopped
  })

  it('keeps the last report after active flips false', async () => {
    getMock.mockResolvedValue({
      data: { data: { anime_id: 'a1', providers: [{ provider: 'gogoanime', summary: { status: 'verified', raw: true, dub_langs: ['en'], hardsub_langs: [] }, units: [] }] } },
    })
    const active = ref(true)
    const { report } = useContentVerify(ref('a1'), active)
    await vi.advanceTimersByTimeAsync(0)
    expect(report.value?.providers.gogoanime.status).toBe('verified')
    active.value = false
    await nextTick()
    expect(report.value?.providers.gogoanime.status).toBe('verified')
  })

  it('does not poll while the page is hidden', async () => {
    Object.defineProperty(document, 'hidden', { configurable: true, get: () => true })
    const active = ref(true)
    useContentVerify(ref('a1'), active)
    await vi.advanceTimersByTimeAsync(0)
    expect(getMock).not.toHaveBeenCalled()
    Object.defineProperty(document, 'hidden', { configurable: true, get: () => false })
  })

  it('refresh() fetches on demand', async () => {
    const active = ref(false)
    const { refresh } = useContentVerify(ref('a1'), active)
    await vi.advanceTimersByTimeAsync(0)
    expect(getMock).not.toHaveBeenCalled()
    await refresh()
    expect(getMock).toHaveBeenCalledTimes(1)
  })
})
