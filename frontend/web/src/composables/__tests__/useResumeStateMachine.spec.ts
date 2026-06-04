import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { ref, effectScope, nextTick, type EffectScope } from 'vue'
import { useResumeStateMachine, type ResumeStateInputs } from '../useResumeStateMachine'

const getProgress = vi.fn()
vi.mock('@/api/client', () => ({
  userApi: { getProgress: (...args: unknown[]) => getProgress(...args) },
}))

const HOUR = 60 * 60 * 1000

function makeInputs(over: Partial<Record<keyof ResumeStateInputs, unknown>> = {}): ResumeStateInputs {
  return {
    animeId: ref('a1'),
    totalEpisodes: ref(12),
    episodesAired: ref(3),
    nextEpisodeAt: ref<string | undefined>(undefined),
    status: ref('ongoing'),
    isAuthenticated: ref(true),
    ...(over as object),
  } as ResumeStateInputs
}

describe('useResumeStateMachine — airing state', () => {
  let scope: EffectScope

  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-06-04T12:00:00Z'))
    // User has watched up to ep 3 (== episodesAired by default → caught up).
    getProgress.mockResolvedValue({ data: { data: [{ episode_number: 3, completed: true }] } })
  })

  afterEach(() => {
    scope?.stop()
    vi.useRealTimers()
    getProgress.mockReset()
  })

  async function setup(inputs: ResumeStateInputs) {
    let sm!: ReturnType<typeof useResumeStateMachine>
    scope = effectScope()
    scope.run(() => {
      sm = useResumeStateMachine(inputs)
    })
    await sm.init()
    return sm
  }

  it('is episode-not-loaded-yet when caught up (last == aired) and air time just passed', async () => {
    // Default: episodesAired=3, last=3 (caught up). Next ep aired 1h ago, not loaded.
    const nextAt = new Date(Date.now() - 1 * HOUR).toISOString()
    const sm = await setup(makeInputs({ nextEpisodeAt: ref(nextAt) }))
    expect(sm.kind.value).toBe('episode-not-loaded-yet')
    expect(sm.episodeAiredAgoMs.value).toBeGreaterThanOrEqual(HOUR - 1000)
  })

  it('trusts the user when they have watched PAST the stale catalog aired count', async () => {
    // The reported Re:Zero S4 bug: user completed ep 9, but catalog
    // episodes_aired is stuck at 8. We must NOT say "ep 9 not loaded" — the
    // user watched it. last (9) > aired (8) → watching.
    getProgress.mockResolvedValue({ data: { data: [{ episode_number: 9, completed: true }] } })
    const nextAt = new Date(Date.now() - 27 * HOUR).toISOString()
    const sm = await setup(makeInputs({ episodesAired: ref(8), nextEpisodeAt: ref(nextAt) }))
    expect(sm.kind.value).toBe('watching')
    expect(sm.finishedEpisode.value).toBe(9)
    expect(sm.startEpisode.value).toBe(10)
  })

  it('self-heals not-yet-aired → episode-not-loaded-yet as the clock advances', async () => {
    const nextAt = new Date(Date.now() + 2 * 60_000).toISOString() // 2 min out
    const sm = await setup(makeInputs({ nextEpisodeAt: ref(nextAt) }))
    expect(sm.kind.value).toBe('not-yet-aired')

    // Advance past the air time; the internal 60s tick refreshes reactive `now`.
    await vi.advanceTimersByTimeAsync(3 * 60_000)
    await nextTick()
    expect(sm.kind.value).toBe('episode-not-loaded-yet')
  })

  it('stays not-yet-aired for a far-future air time', async () => {
    const nextAt = new Date(Date.now() + 48 * HOUR).toISOString()
    const sm = await setup(makeInputs({ nextEpisodeAt: ref(nextAt) }))
    expect(sm.kind.value).toBe('not-yet-aired')
    expect(sm.episodeAiredAgoMs.value).toBe(0)
  })
})
