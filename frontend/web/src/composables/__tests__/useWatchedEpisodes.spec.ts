import { describe, it, expect, vi, beforeEach } from 'vitest'

const getWatchlistEntry = vi.fn()
vi.mock('@/api/client', () => ({
  userApi: { getWatchlistEntry: (...a: unknown[]) => getWatchlistEntry(...a) },
}))

let isAuthenticated = true
vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({ get isAuthenticated() { return isAuthenticated } }),
}))

// Viewer-context aggregate (page-fetch optimization 2026-06-11): the first
// refresh per anime consumes this when present instead of fetching. The
// composable awaits whenLoaded (an in-flight-aware Promise wrapper).
let viewerCtx: { watchlist_entry: { episodes?: number } | null } | null = null
vi.mock('@/stores/viewerContext', () => ({
  useViewerContextStore: () => ({
    // forAnime deliberately null: proves the composable relies on whenLoaded
    // (in-flight aware), not on synchronous availability.
    forAnime: () => null,
    whenLoaded: () => Promise.resolve(viewerCtx),
  }),
}))

import { useWatchedEpisodes } from '../useWatchedEpisodes'

beforeEach(() => { getWatchlistEntry.mockReset(); isAuthenticated = true; viewerCtx = null })

describe('useWatchedEpisodes', () => {
  it('reads entry.episodes (wrapped data.data) when authenticated', async () => {
    getWatchlistEntry.mockResolvedValue({ data: { data: { episodes: 7 } } })
    const { watchedUpTo, refresh } = useWatchedEpisodes('a1')
    await refresh()
    expect(watchedUpTo.value).toBe(7)
    expect(getWatchlistEntry).toHaveBeenCalledWith('a1')
  })

  it('handles the unwrapped data shape (data.episodes)', async () => {
    getWatchlistEntry.mockResolvedValue({ data: { episodes: 3 } })
    const { watchedUpTo, refresh } = useWatchedEpisodes('a1')
    await refresh()
    expect(watchedUpTo.value).toBe(3)
  })

  it('stays 0 and never calls the API when unauthenticated', async () => {
    isAuthenticated = false
    const { watchedUpTo, refresh } = useWatchedEpisodes('a1')
    await refresh()
    expect(watchedUpTo.value).toBe(0)
    expect(getWatchlistEntry).not.toHaveBeenCalled()
  })

  it('falls back to 0 on API error', async () => {
    getWatchlistEntry.mockRejectedValue(new Error('404'))
    const { watchedUpTo, refresh } = useWatchedEpisodes('a1')
    await refresh()
    expect(watchedUpTo.value).toBe(0)
  })

  it('consumes the viewer-context aggregate on first refresh (no API call)', async () => {
    viewerCtx = { watchlist_entry: { episodes: 5 } }
    const { watchedUpTo, refresh } = useWatchedEpisodes('a1')
    await refresh()
    expect(watchedUpTo.value).toBe(5)
    expect(getWatchlistEntry).not.toHaveBeenCalled()
  })

  it('awaits an in-flight viewer-context load instead of fetching (deep-link race)', async () => {
    // whenLoaded resolves with data even though it wasn't synchronously
    // available — the composable must use it and skip the network.
    viewerCtx = { watchlist_entry: { episodes: 9 } }
    const { watchedUpTo, refresh } = useWatchedEpisodes('a1')
    await refresh()
    expect(watchedUpTo.value).toBe(9)
    expect(getWatchlistEntry).not.toHaveBeenCalled()
  })

  it('goes to the network on the SECOND refresh (post-mutation freshness)', async () => {
    viewerCtx = { watchlist_entry: { episodes: 5 } }
    getWatchlistEntry.mockResolvedValue({ data: { data: { episodes: 6 } } })
    const { watchedUpTo, refresh } = useWatchedEpisodes('a1')
    await refresh()
    expect(getWatchlistEntry).not.toHaveBeenCalled()
    await refresh()
    expect(getWatchlistEntry).toHaveBeenCalledWith('a1')
    expect(watchedUpTo.value).toBe(6)
  })
})
