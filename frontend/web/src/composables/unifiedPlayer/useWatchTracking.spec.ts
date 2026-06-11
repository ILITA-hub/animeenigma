import { describe, it, expect, vi, beforeEach } from 'vitest'

const updateProgress = vi.fn().mockResolvedValue({})
const markEpisodeWatched = vi.fn().mockResolvedValue({})
vi.mock('@/api/client', () => ({
  userApi: {
    updateProgress: (...a: unknown[]) => updateProgress(...a),
    markEpisodeWatched: (...a: unknown[]) => markEpisodeWatched(...a),
  },
}))

let isAuthenticated = true
vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    get isAuthenticated() {
      return isAuthenticated
    },
  }),
}))

import { useWatchTracking } from './useWatchTracking'

function makeTracking(ep: number | null = 3, hooks = {}) {
  return useWatchTracking(() => 'anime-1', () => ep, hooks)
}

beforeEach(() => {
  updateProgress.mockClear()
  markEpisodeWatched.mockClear()
  isAuthenticated = true
  localStorage.clear()
})

describe('useWatchTracking', () => {
  it('heartbeat-saves to server + localStorage every 30s of media time', () => {
    const t = makeTracking()
    t.onTick(10, 1440)
    expect(updateProgress).not.toHaveBeenCalled()

    t.onTick(31, 1440)
    expect(updateProgress).toHaveBeenCalledTimes(1)
    expect(updateProgress).toHaveBeenCalledWith(
      expect.objectContaining({ anime_id: 'anime-1', episode_number: 3, progress: 31 }),
    )
    const local = JSON.parse(localStorage.getItem('watch_progress:anime-1') || '{}')
    expect(local['3'].time).toBe(31)

    // No re-save until another 30s of playback passes
    t.onTick(45, 1440)
    expect(updateProgress).toHaveBeenCalledTimes(1)
  })

  it('tracks maxTime monotonically (scrubbing back does not lower it)', () => {
    const t = makeTracking()
    t.onTick(300, 1440)
    t.onTick(50, 1440)
    expect(t.maxTime.value).toBe(300)
  })

  it('auto-marks completed at >=90% of the real duration, exactly once', async () => {
    const t = makeTracking()
    t.onTick(95, 100)
    await vi.waitFor(() => expect(markEpisodeWatched).toHaveBeenCalledTimes(1))
    expect(markEpisodeWatched).toHaveBeenCalledWith('anime-1', 3, undefined, expect.any(String))

    t.onTick(96, 100)
    t.onTick(97, 100)
    await Promise.resolve()
    expect(markEpisodeWatched).toHaveBeenCalledTimes(1)
    expect(t.episodeMarked.value).toBe(true)
  })

  it('falls back to the 20-minute rule when duration is unknown', async () => {
    const t = makeTracking()
    t.onTick(1199, 0)
    expect(markEpisodeWatched).not.toHaveBeenCalled()
    t.onTick(1201, 0)
    await vi.waitFor(() => expect(markEpisodeWatched).toHaveBeenCalledTimes(1))
  })

  it('does not auto-mark an episode the user already has watched (resetEpisode(true))', () => {
    const t = makeTracking()
    t.resetEpisode(true)
    t.onTick(95, 100)
    expect(markEpisodeWatched).not.toHaveBeenCalled()
  })

  it('saveNow persists the last known position immediately', () => {
    const t = makeTracking()
    t.onTick(12, 1440) // below heartbeat threshold — nothing saved yet
    expect(updateProgress).not.toHaveBeenCalled()
    t.saveNow()
    expect(updateProgress).toHaveBeenCalledWith(expect.objectContaining({ progress: 12 }))
  })

  it('anonymous users save to localStorage only — no server calls, no marks', () => {
    isAuthenticated = false
    const t = makeTracking()
    t.onTick(31, 100)
    t.onTick(95, 100)
    t.saveNow()
    expect(updateProgress).not.toHaveBeenCalled()
    expect(markEpisodeWatched).not.toHaveBeenCalled()
    const local = JSON.parse(localStorage.getItem('watch_progress:anime-1') || '{}')
    expect(local['3']).toBeTruthy()
  })

  it('beaconSave ships the position via navigator.sendBeacon', () => {
    const sendBeacon = vi.fn().mockReturnValue(true)
    Object.defineProperty(navigator, 'sendBeacon', { value: sendBeacon, configurable: true })
    const t = makeTracking()
    t.onTick(42, 1440)
    t.beaconSave()
    expect(sendBeacon).toHaveBeenCalledTimes(1)
    expect(sendBeacon.mock.calls[0][0]).toBe('/api/users/progress')
  })

  it('invokes the onMarked hook after a successful mark', async () => {
    const onMarked = vi.fn()
    const t = makeTracking(5, { onMarked })
    await t.markWatched()
    expect(onMarked).toHaveBeenCalledWith(5)
  })

  it('is inert without an episode number', () => {
    const t = makeTracking(null)
    t.onTick(31, 1440)
    t.saveNow()
    expect(updateProgress).not.toHaveBeenCalled()
    expect(localStorage.getItem('watch_progress:anime-1')).toBeNull()
  })
})
