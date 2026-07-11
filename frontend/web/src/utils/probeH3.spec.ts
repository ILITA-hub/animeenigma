import { describe, it, expect, vi, afterEach } from 'vitest'
import { probeH3, type ProbeLadder } from './probeH3'

function makeLadder(overrides: Partial<ProbeLadder> = {}): ProbeLadder {
  return {
    tierBase: (id) => (id === 'h3' ? 'https://stream3.example' : null),
    currentEwmaMbps: () => 5, // baseline h2 EWMA -> accept threshold = 5.5 Mbps
    hasProbedH3: () => false,
    recordProbe: vi.fn(),
    switchTo: vi.fn(),
    ...overrides,
  }
}

function fakeResponse(bytes: number, url = ''): Response {
  return {
    url,
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

  it('accepts the upshift and calls switchTo when measured throughput clears 1.1x the current EWMA', async () => {
    const ladder = makeLadder({ currentEwmaMbps: () => 5 }) // accept threshold = 5.5 Mbps
    vi.spyOn(performance, 'now').mockReturnValueOnce(0).mockReturnValueOnce(2000) // 2s elapsed
    vi.spyOn(performance, 'getEntriesByName').mockReturnValue([{ nextHopProtocol: 'h3' }] as unknown as PerformanceEntryList)
    // 2,000,000 bytes * 8 / 2s / 1e6 = 8 Mbps >= 5.5 Mbps
    const fetchMock = vi.fn(async () => fakeResponse(2_000_000))

    await probeH3(ladder, 'seg.ts', 'master.m3u8', fetchMock as unknown as typeof fetch)

    expect(ladder.recordProbe).toHaveBeenCalledWith(8, true, expect.any(String))
    expect(ladder.switchTo).toHaveBeenCalledWith('h3', expect.stringContaining('probe'))
  })

  it('rejects (no switch) when measured throughput is below 1.1x the current EWMA', async () => {
    const ladder = makeLadder({ currentEwmaMbps: () => 5 }) // accept threshold = 5.5 Mbps
    vi.spyOn(performance, 'now').mockReturnValueOnce(0).mockReturnValueOnce(2000) // 2s elapsed
    vi.spyOn(performance, 'getEntriesByName').mockReturnValue([{ nextHopProtocol: 'h3' }] as unknown as PerformanceEntryList)
    // 1,000,000 bytes * 8 / 2s / 1e6 = 4 Mbps < 5.5 Mbps
    const fetchMock = vi.fn(async () => fakeResponse(1_000_000))

    await probeH3(ladder, 'seg.ts', 'master.m3u8', fetchMock as unknown as typeof fetch)

    expect(ladder.recordProbe).toHaveBeenCalledWith(4, false, expect.any(String))
    expect(ladder.switchTo).not.toHaveBeenCalled()
  })

  it('records rejected with note h3-unavailable when nextHopProtocol is not h3, and does not switch', async () => {
    const ladder = makeLadder()
    vi.spyOn(performance, 'now').mockReturnValueOnce(0).mockReturnValueOnce(1000)
    vi.spyOn(performance, 'getEntriesByName').mockReturnValue([{ nextHopProtocol: 'h2' }] as unknown as PerformanceEntryList)
    const fetchMock = vi.fn(async () => fakeResponse(2_000_000))

    await probeH3(ladder, 'seg.ts', 'master.m3u8', fetchMock as unknown as typeof fetch)

    expect(ladder.recordProbe).toHaveBeenCalledWith(expect.any(Number), false, 'h3-unavailable')
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

    expect(ladder.recordProbe).toHaveBeenCalledWith(expect.any(Number), false, expect.any(String))
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
    expect(ladder.recordProbe).toHaveBeenCalledWith(expect.any(Number), false, expect.any(String))
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
