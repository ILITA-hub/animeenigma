import { describe, it, expect, vi, afterEach } from 'vitest'
import { probeH3, type ProbeLadder } from './probeH3'

function makeLadder(overrides: Partial<ProbeLadder> = {}): ProbeLadder {
  return {
    tierBase: (id) => (id === 'h3' ? 'https://stream3.example' : null),
    currentEwmaMbps: () => 5, // baseline h2 EWMA -> accept threshold = 5.5 Mbps
    hasProbedH3: () => false,
    currentTierId: () => 'h2',
    recordProbe: vi.fn(),
    switchTo: vi.fn(),
    ...overrides,
  }
}

function fakeResponse(bytes: number, url = '', ok = true, status = 200): Response {
  return {
    url,
    ok,
    status,
    arrayBuffer: async () => new ArrayBuffer(bytes),
  } as unknown as Response
}

afterEach(() => {
  vi.restoreAllMocks()
  vi.useRealTimers()
})

describe('probeH3()', () => {
  it('primes the h3-tier PLAYLIST URL first, then measures the h3-tier SEGMENT URL with cache:no-store', async () => {
    const ladder = makeLadder()
    vi.spyOn(performance, 'now').mockReturnValueOnce(0).mockReturnValueOnce(1000)
    vi.spyOn(performance, 'getEntriesByName').mockReturnValue([{ nextHopProtocol: 'h3' }] as unknown as PerformanceEntryList)

    const calls: Array<[string, RequestInit | undefined]> = []
    const fetchMock = vi.fn(async (url: RequestInfo | URL, init?: RequestInit) => {
      calls.push([String(url), init])
      return calls.length === 1 ? fakeResponse(0) : fakeResponse(1_000_000, String(url))
    })

    await probeH3(
      ladder,
      'https://stream2.example/hls/ep1/seg-005.ts?sig=abc', // sampleUrl (segment, current tier)
      '/hls/ep1/master.m3u8?sig=xyz', // playlistUrl (relative, current tier)
      fetchMock as unknown as typeof fetch,
    )

    expect(calls).toHaveLength(2)
    expect(calls[0][0]).toBe('https://stream3.example/hls/ep1/master.m3u8?sig=xyz') // prime: playlist, origin swapped
    expect(calls[1][0]).toBe('https://stream3.example/hls/ep1/seg-005.ts?sig=abc') // measure: segment, origin swapped
    expect(calls[1][1]).toMatchObject({ cache: 'no-store' })
  })

  it('cancels the prime response body instead of leaking it, and tolerates a null body (M8)', async () => {
    const ladder = makeLadder()
    vi.spyOn(performance, 'now').mockReturnValueOnce(0).mockReturnValueOnce(1000)
    vi.spyOn(performance, 'getEntriesByName').mockReturnValue([{ nextHopProtocol: 'h3' }] as unknown as PerformanceEntryList)

    const cancel = vi.fn(async () => {})
    let call = 0
    const fetchMock = vi.fn(async () => {
      call += 1
      if (call === 1) return { ...fakeResponse(0), body: { cancel } } as unknown as Response
      return fakeResponse(1_000_000) // measure response has no `body` field — must not throw
    })

    await expect(
      probeH3(ladder, 'seg.ts', 'master.m3u8', fetchMock as unknown as typeof fetch),
    ).resolves.toBeUndefined()

    expect(cancel).toHaveBeenCalledTimes(1)
  })

  it('accepts the upshift and calls switchTo when measured throughput clears 1.1x the current EWMA', async () => {
    const ladder = makeLadder({ currentEwmaMbps: () => 5 }) // accept threshold = 5.5 Mbps
    vi.spyOn(performance, 'now').mockReturnValueOnce(0).mockReturnValueOnce(2000) // 2s elapsed
    vi.spyOn(performance, 'getEntriesByName').mockReturnValue([{ nextHopProtocol: 'h3' }] as unknown as PerformanceEntryList)
    // 2,000,000 bytes * 8 / 2s / 1e6 = 8 Mbps >= 5.5 Mbps
    const fetchMock = vi.fn(async () => fakeResponse(2_000_000))

    await probeH3(ladder, 'seg.ts', 'master.m3u8', fetchMock as unknown as typeof fetch)

    expect(ladder.recordProbe).toHaveBeenCalledWith('h3', 8, true, expect.any(String))
    expect(ladder.switchTo).toHaveBeenCalledWith('h3', expect.stringContaining('probe'))
  })

  it('rejects (no switch) when measured throughput is below 1.1x the current EWMA', async () => {
    const ladder = makeLadder({ currentEwmaMbps: () => 5 }) // accept threshold = 5.5 Mbps
    vi.spyOn(performance, 'now').mockReturnValueOnce(0).mockReturnValueOnce(2000) // 2s elapsed
    vi.spyOn(performance, 'getEntriesByName').mockReturnValue([{ nextHopProtocol: 'h3' }] as unknown as PerformanceEntryList)
    // 1,000,000 bytes * 8 / 2s / 1e6 = 4 Mbps < 5.5 Mbps
    const fetchMock = vi.fn(async () => fakeResponse(1_000_000))

    await probeH3(ladder, 'seg.ts', 'master.m3u8', fetchMock as unknown as typeof fetch)

    expect(ladder.recordProbe).toHaveBeenCalledWith('h3', 4, false, expect.any(String))
    expect(ladder.switchTo).not.toHaveBeenCalled()
  })

  it('records rejected with note h3-unavailable when nextHopProtocol is not h3, and does not switch', async () => {
    const ladder = makeLadder()
    vi.spyOn(performance, 'now').mockReturnValueOnce(0).mockReturnValueOnce(1000)
    vi.spyOn(performance, 'getEntriesByName').mockReturnValue([{ nextHopProtocol: 'h2' }] as unknown as PerformanceEntryList)
    const fetchMock = vi.fn(async () => fakeResponse(2_000_000))

    await probeH3(ladder, 'seg.ts', 'master.m3u8', fetchMock as unknown as typeof fetch)

    expect(ladder.recordProbe).toHaveBeenCalledWith('h3', expect.any(Number), false, 'h3-unavailable')
    expect(ladder.switchTo).not.toHaveBeenCalled()
  })

  it('records rejected no-baseline and skips the network entirely when the EWMA is zero (C2)', async () => {
    // A zero baseline (fresh session, or an MP4/native-HLS source that never
    // feeds reportFragment) makes PROBE_ACCEPT_FACTOR × 0 = 0 — any measured
    // throughput would spuriously clear that threshold, so this must bail
    // before even fetching.
    const ladder = makeLadder({ currentEwmaMbps: () => 0 })
    const fetchMock = vi.fn()

    await probeH3(ladder, 'seg.ts', 'master.m3u8', fetchMock as unknown as typeof fetch)

    expect(fetchMock).not.toHaveBeenCalled()
    expect(ladder.recordProbe).toHaveBeenCalledWith('h3', 0, false, 'no-baseline')
    expect(ladder.switchTo).not.toHaveBeenCalled()
  })

  it('records rejected http-<status> and does not switch when the measure response is not ok (C2)', async () => {
    const ladder = makeLadder()
    let call = 0
    const fetchMock = vi.fn(async () => {
      call += 1
      return call === 1 ? fakeResponse(0) : fakeResponse(0, '', false, 503) // prime ok, measure 503
    })

    await probeH3(ladder, 'seg.ts', 'master.m3u8', fetchMock as unknown as typeof fetch)

    expect(fetchMock).toHaveBeenCalledTimes(2) // prime + measure, no retry
    expect(ladder.recordProbe).toHaveBeenCalledWith('h3', 0, false, 'http-503')
    expect(ladder.switchTo).not.toHaveBeenCalled()
  })

  it('never throws on a rejected fetch (network error) and records a rejected probe', async () => {
    const ladder = makeLadder()
    const fetchMock = vi.fn(async () => {
      throw new TypeError('network error')
    })

    await expect(
      probeH3(ladder, 'seg.ts', 'master.m3u8', fetchMock as unknown as typeof fetch),
    ).resolves.toBeUndefined()

    expect(ladder.recordProbe).toHaveBeenCalledWith('h3', expect.any(Number), false, expect.any(String))
    expect(ladder.switchTo).not.toHaveBeenCalled()
  })

  it('aborts and records a rejected probe after the 20s timeout, without throwing', async () => {
    vi.useFakeTimers()
    const ladder = makeLadder()
    const hangingFetch = vi.fn((_url: RequestInfo | URL, init?: RequestInit) => {
      return new Promise<Response>((_resolve, reject) => {
        init?.signal?.addEventListener('abort', () => {
          reject(new DOMException('Aborted', 'AbortError'))
        })
      })
    })

    const pending = probeH3(ladder, 'seg.ts', 'master.m3u8', hangingFetch as unknown as typeof fetch)
    await vi.advanceTimersByTimeAsync(20_000)
    await expect(pending).resolves.toBeUndefined()

    expect(hangingFetch).toHaveBeenCalledTimes(1) // only the prime fetch was reached
    expect(ladder.recordProbe).toHaveBeenCalledWith('h3', expect.any(Number), false, expect.any(String))
    expect(ladder.switchTo).not.toHaveBeenCalled()
  })

  it('resolves without fetching when no h3 tier is configured', async () => {
    const ladder = makeLadder({ tierBase: () => null })
    const fetchMock = vi.fn()

    await probeH3(ladder, 'seg.ts', 'master.m3u8', fetchMock as unknown as typeof fetch)

    expect(fetchMock).not.toHaveBeenCalled()
    expect(ladder.recordProbe).not.toHaveBeenCalled()
    expect(ladder.switchTo).not.toHaveBeenCalled()
  })

  it('resolves without fetching when this session has already probed h3', async () => {
    const ladder = makeLadder({ hasProbedH3: () => true })
    const fetchMock = vi.fn()

    await probeH3(ladder, 'seg.ts', 'master.m3u8', fetchMock as unknown as typeof fetch)

    expect(fetchMock).not.toHaveBeenCalled()
    expect(ladder.recordProbe).not.toHaveBeenCalled()
    expect(ladder.switchTo).not.toHaveBeenCalled()
  })
})
