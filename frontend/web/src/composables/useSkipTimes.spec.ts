// Task 10 (opskip) — combo-aware skip times. Mocks `animeApi.getSkipTimes`
// and asserts the composable builds the anime/provider/team query params
// from the optional `combo` ref, and re-fetches (latest-wins) when the
// combo ref's value changes — e.g. a provider/team switch, since different
// encodes can have different OP/ED cut points for the same episode.
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { ref } from 'vue'
import { flushPromises } from '@vue/test-utils'

const getSkipTimes = vi.fn()
vi.mock('@/api/client', () => ({
  animeApi: {
    getSkipTimes: (malId: string, episode: number, opts?: unknown) => getSkipTimes(malId, episode, opts),
  },
}))

import { useSkipTimes, type SkipTimesComboContext } from './useSkipTimes'

beforeEach(() => {
  getSkipTimes.mockReset()
})

function okResult(startTime = 5, endTime = 20) {
  return {
    data: {
      found: true,
      results: [{ interval: { startTime, endTime }, skipType: 'op', skipId: '', episodeLength: 0 }],
    },
  }
}

describe('useSkipTimes combo-awareness', () => {
  it('passes anime/provider/team query params when combo is present', async () => {
    getSkipTimes.mockResolvedValue(okResult())
    const malId = ref<string | number | null>('123')
    const episode = ref<number | null>(1)
    const combo = ref<SkipTimesComboContext | null>({
      animeId: 'anime-uuid', provider: 'gogoanime', team: 'TeamA',
    })

    useSkipTimes(malId, episode, combo)
    await flushPromises()

    expect(getSkipTimes).toHaveBeenCalledWith('123', 1, {
      anime: 'anime-uuid', provider: 'gogoanime', team: 'TeamA',
    })
  })

  it('omits the team param when combo.team is null', async () => {
    getSkipTimes.mockResolvedValue(okResult())
    const malId = ref<string | number | null>('123')
    const episode = ref<number | null>(1)
    const combo = ref<SkipTimesComboContext | null>({
      animeId: 'anime-uuid', provider: 'gogoanime', team: null,
    })

    useSkipTimes(malId, episode, combo)
    await flushPromises()

    expect(getSkipTimes).toHaveBeenCalledWith('123', 1, { anime: 'anime-uuid', provider: 'gogoanime' })
  })

  it('passes no opts (undefined) when the combo ref is null', async () => {
    getSkipTimes.mockResolvedValue(okResult())
    const malId = ref<string | number | null>('123')
    const episode = ref<number | null>(1)
    const combo = ref<SkipTimesComboContext | null>(null)

    useSkipTimes(malId, episode, combo)
    await flushPromises()

    expect(getSkipTimes).toHaveBeenCalledWith('123', 1, undefined)
  })

  it('passes no opts when the combo argument is omitted entirely (backward compat)', async () => {
    getSkipTimes.mockResolvedValue(okResult())
    const malId = ref<string | number | null>('123')
    const episode = ref<number | null>(1)

    useSkipTimes(malId, episode)
    await flushPromises()

    expect(getSkipTimes).toHaveBeenCalledWith('123', 1, undefined)
  })

  it('does not send params when animeId is present but provider is missing', async () => {
    getSkipTimes.mockResolvedValue(okResult())
    const malId = ref<string | number | null>('123')
    const episode = ref<number | null>(1)
    const combo = ref<SkipTimesComboContext | null>({ animeId: 'anime-uuid' })

    useSkipTimes(malId, episode, combo)
    await flushPromises()

    expect(getSkipTimes).toHaveBeenCalledWith('123', 1, undefined)
  })

  it('refetches on a combo change (provider switch), and the stale in-flight response is dropped (latest-wins)', async () => {
    let resolveFirst!: (v: unknown) => void
    const firstPending = new Promise((res) => { resolveFirst = res })
    getSkipTimes.mockImplementationOnce(() => firstPending)
    getSkipTimes.mockImplementationOnce(async () => okResult(1, 2))

    const malId = ref<string | number | null>('123')
    const episode = ref<number | null>(1)
    const combo = ref<SkipTimesComboContext | null>({
      animeId: 'anime-uuid', provider: 'gogoanime', team: null,
    })

    const { opening } = useSkipTimes(malId, episode, combo)
    await flushPromises()
    expect(getSkipTimes).toHaveBeenCalledTimes(1)
    expect(opening.value).toBeNull() // first request still in flight

    // Provider switch — per-encode timings differ, so this must trigger a
    // fresh fetch even though malId/episode are unchanged.
    combo.value = { animeId: 'anime-uuid', provider: 'animepahe', team: null }
    await flushPromises()
    expect(getSkipTimes).toHaveBeenCalledTimes(2)
    expect(getSkipTimes).toHaveBeenLastCalledWith('123', 1, { anime: 'anime-uuid', provider: 'animepahe' })
    expect(opening.value).toEqual({ start: 1, end: 2 }) // resolved from the second (current) request

    // The stale first request finally resolves — its result must be dropped
    // (latest-wins token), not overwrite the second request's segment.
    resolveFirst(okResult(999, 999))
    await flushPromises()
    expect(getSkipTimes).toHaveBeenCalledTimes(2) // no extra fetch triggered by the late resolution
    expect(opening.value).toEqual({ start: 1, end: 2 }) // unchanged — stale response discarded
  })
})
