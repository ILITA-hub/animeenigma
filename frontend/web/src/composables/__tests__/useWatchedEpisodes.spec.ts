import { describe, it, expect, vi, beforeEach } from 'vitest'

const getWatchlistEntry = vi.fn()
vi.mock('@/api/client', () => ({
  userApi: { getWatchlistEntry: (...a: unknown[]) => getWatchlistEntry(...a) },
}))

let isAuthenticated = true
vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({ get isAuthenticated() { return isAuthenticated } }),
}))

import { useWatchedEpisodes } from '../useWatchedEpisodes'

beforeEach(() => { getWatchlistEntry.mockReset(); isAuthenticated = true })

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
})
