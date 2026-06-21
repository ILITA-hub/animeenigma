import { describe, it, expect, vi, beforeEach } from 'vitest'
import { ref, nextTick } from 'vue'
import { useProviderAvailability, overlayAvailability } from './useProviderAvailability'

const getEpisodes = vi.fn()
vi.mock('@/api/client', () => ({
  scraperApi: { getEpisodes: (...a: unknown[]) => getEpisodes(...a) },
}))

describe('useProviderAvailability', () => {
  beforeEach(() => getEpisodes.mockReset())

  it('checkExists records not_found on a 404', async () => {
    getEpisodes.mockRejectedValueOnce({ response: { status: 404 } })
    const a = useProviderAvailability(ref('anime1'))
    await a.checkExists('gogoanime')
    expect(a.get('gogoanime')).toEqual({ available: false, reason: 'not_found' })
    // exclusive=true was requested
    expect(getEpisodes).toHaveBeenCalledWith('anime1', 'gogoanime', true)
  })

  it('checkExists records available on 200 with episodes', async () => {
    getEpisodes.mockResolvedValueOnce({ data: { data: { episodes: [{ id: 'e1', number: 1 }] } } })
    const a = useProviderAvailability(ref('anime1'))
    await a.checkExists('gogoanime')
    expect(a.get('gogoanime')).toEqual({ available: true })
  })

  it('markCdnUnreachable overrides to cdn_unreachable', () => {
    const a = useProviderAvailability(ref('anime1'))
    a.markCdnUnreachable('gogoanime')
    expect(a.get('gogoanime')).toEqual({ available: false, reason: 'cdn_unreachable' })
  })

  it('caches checkExists (one request per provider)', async () => {
    getEpisodes.mockResolvedValue({ data: { data: { episodes: [{ id: 'e1', number: 1 }] } } })
    const a = useProviderAvailability(ref('anime1'))
    await a.checkExists('gogoanime')
    await a.checkExists('gogoanime')
    expect(getEpisodes).toHaveBeenCalledTimes(1)
  })

  it('resets cache when anime changes', async () => {
    getEpisodes.mockResolvedValue({ data: { data: { episodes: [{ id: 'e1', number: 1 }] } } })
    const id = ref('anime1')
    const a = useProviderAvailability(id)
    await a.checkExists('gogoanime')
    id.value = 'anime2'
    await nextTick()
    expect(a.get('gogoanime')).toBeUndefined()
  })
})

describe('overlayAvailability', () => {
  const t = (k: string) => k
  const row = { def: { id: 'gogoanime', name: 'GogoAnime', scraper: true }, state: 'active' } as unknown as import('@/types/aePlayer').ProviderRow

  it('passes through when available or unknown', () => {
    expect(overlayAvailability(row, undefined, t)).toBe(row)
    expect(overlayAvailability(row, { available: true }, t)).toBe(row)
  })
  it('not_found → irrelevant + lacks-anime reason', () => {
    const r = overlayAvailability(row, { available: false, reason: 'not_found' }, t)
    expect(r.state).toBe('irrelevant')
    expect(r.reason).toBe('player.sources.providerLacksAnime')
  })
  it('cdn_unreachable → down + cdn reason', () => {
    const r = overlayAvailability(row, { available: false, reason: 'cdn_unreachable' }, t)
    expect(r.state).toBe('down')
    expect(r.reason).toBe('player.sources.providerCdnUnreachable')
  })
})
