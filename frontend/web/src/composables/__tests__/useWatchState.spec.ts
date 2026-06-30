import { describe, it, expect, vi, beforeEach } from 'vitest'
import { ref } from 'vue'
import { useWatchState } from '../useWatchState'

vi.mock('@/api/client', () => ({
  userApi: { getProgress: vi.fn() },
}))
import { userApi } from '@/api/client'

function opts(over: Record<string, unknown> = {}) {
  return {
    animeId: ref('anime-1'),
    totalEpisodes: ref(12),
    episodesAired: ref(12),
    status: ref('released'),
    nextEpisodeAt: ref<string | undefined>(undefined),
    loadedEpisodes: ref(0),
    listStatus: ref<string | null>(null),
    isAuthenticated: ref(true),
    formatEta: (iso: string) => `eta:${iso}`,
    ...over,
  }
}

beforeEach(() => {
  localStorage.clear()
  vi.mocked(userApi.getProgress).mockReset()
})

describe('useWatchState — authenticated', () => {
  it('derives lastWatched (max completed) from prefetched rows → startEpisode = last+1', async () => {
    const ws = useWatchState(opts())
    await ws.init([
      { episode_number: 3, completed: true },
      { episode_number: 5, completed: true },
      { episode_number: 6, completed: false },
    ])
    expect(ws.lastWatched.value).toBe(5)
    expect(ws.startEpisode.value).toBe(6)
    expect(ws.banner.value).toEqual({ kind: 'just-finished', episode: 5 })
    expect(ws.cta.value.action).toBe('continue')
  })

  it('first-time authed → startEpisode 1, banner none', async () => {
    const ws = useWatchState(opts())
    await ws.init([])
    expect(ws.startEpisode.value).toBe(1)
    expect(ws.banner.value).toEqual({ kind: 'none' })
  })
})

describe('useWatchState — anonymous (D-1 adapter)', () => {
  it('opens the in-progress episode from localStorage and keeps CTA consistent', async () => {
    // last-touched episode 6 (most recent updatedAt)
    localStorage.setItem(
      'watch_progress:anime-1',
      JSON.stringify({ '5': { updatedAt: 10 }, '6': { updatedAt: 20 } }),
    )
    const ws = useWatchState(opts({ isAuthenticated: ref(false) }))
    await ws.init()
    // lastWatched (completed) = parsedEp - 1 = 5  →  startEpisode = 6 (same ep as before)
    expect(ws.lastWatched.value).toBe(5)
    expect(ws.startEpisode.value).toBe(6)
    expect(ws.cta.value).toMatchObject({ action: 'continue', startEpisode: 6 })
  })

  it('anon with no localStorage → first-time, ep 1 (fixes the ep-12 default bug)', async () => {
    const ws = useWatchState(opts({ isAuthenticated: ref(false) }))
    await ws.init()
    expect(ws.lastWatched.value).toBe(0)
    expect(ws.startEpisode.value).toBe(1)
  })
})

describe('useWatchState — server fetch fallback', () => {
  it('fetches progress when no prefetch is supplied', async () => {
    vi.mocked(userApi.getProgress).mockResolvedValue({
      data: { data: [{ episode_number: 2, completed: true }] },
    } as never)
    const ws = useWatchState(opts())
    await ws.init()
    expect(userApi.getProgress).toHaveBeenCalledWith('anime-1')
    expect(ws.lastWatched.value).toBe(2)
  })
})
