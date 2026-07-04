import { describe, it, expect, vi, beforeEach } from 'vitest'
import type { EpisodeOption } from '@/components/player/EpisodeSelector.types'
import type { Combo } from '@/types/aePlayer'

const enqueueDownload = vi.fn()
const getDownload = vi.fn()
vi.mock('./downloadEngine', () => ({ enqueueDownload: (r: unknown) => enqueueDownload(r) }))
vi.mock('./registry', () => ({ getDownload: (id: string) => getDownload(id) }))

import { seasonTargets, enqueueSeason, type SeasonContext } from './seasonDownload'

const ep = (n: number): EpisodeOption => ({ id: `e${n}`, number: n, label: String(n) }) as EpisodeOption
const combo: Combo = { audio: 'sub', lang: 'en', provider: 'p', server: 's', team: null }

function ctx(): SeasonContext {
  return {
    animeId: 'a1',
    animeTitle: 'T',
    combo,
    quality: '720',
    resolveFor: vi.fn((e: EpisodeOption) => async () => ({ url: `u${e.number}` }) as never),
  }
}

describe('seasonTargets', () => {
  it('skips done/queued/downloading, keeps paused/error/untouched', () => {
    const eps = [ep(1), ep(2), ep(3), ep(4), ep(5)]
    const out = seasonTargets(eps, { 1: 'done', 2: 'queued', 3: 'downloading', 4: 'paused' })
    expect(out.map((e) => e.number)).toEqual([4, 5])
  })

  it('returns [] for a fully downloaded season', () => {
    expect(seasonTargets([ep(1)], { 1: 'done' })).toEqual([])
  })
})

describe('enqueueSeason', () => {
  beforeEach(() => {
    enqueueDownload.mockReset().mockImplementation(async (r: { episode: EpisodeOption }) => `id-${r.episode.number}`)
    getDownload.mockReset().mockResolvedValue({ state: 'queued' })
  })

  it('enqueues every target in order with per-episode resolve closures', async () => {
    const c = ctx()
    const n = await enqueueSeason([ep(1), ep(2)], c)
    expect(n).toBe(2)
    expect(enqueueDownload).toHaveBeenCalledTimes(2)
    expect(enqueueDownload.mock.calls[0][0]).toMatchObject({ animeId: 'a1', quality: '720', episode: { number: 1 } })
    expect(c.resolveFor).toHaveBeenCalledTimes(2)
  })

  it('stops early when the engine reports a quota error', async () => {
    getDownload
      .mockResolvedValueOnce({ state: 'queued' })
      .mockResolvedValueOnce({ state: 'error', error: 'quota' })
    const n = await enqueueSeason([ep(1), ep(2), ep(3)], ctx())
    expect(n).toBe(1)
    expect(enqueueDownload).toHaveBeenCalledTimes(2) // third never attempted
  })
})
