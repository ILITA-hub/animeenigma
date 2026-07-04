// src/offline/cellularGuard.spec.ts
import 'fake-indexeddb/auto'
import { describe, it, expect, vi, beforeEach } from 'vitest'

const enqueueDownload = vi.fn(async (_req?: unknown) => 'id')
// vi.mock intercepts DYNAMIC imports too — these cover the guard's lazy loads.
// externalSubs must be mocked as well: its static @/api/client chain pulls
// router+i18n into the suite otherwise.
vi.mock('./downloadEngine', () => ({
  enqueueDownload, isEngineWorking: () => false, pauseAllForCellular: vi.fn(),
}))
vi.mock('./externalSubs', () => ({ makeExternalSubResolver: () => undefined }))
vi.mock('@/composables/aePlayer/useProviderResolver', () => ({
  useProviderResolver: () => ({ resolveStream: vi.fn(async () => ({ url: 'u', type: 'hls' })) }),
}))
import { resumeNetworkPaused } from './cellularGuard'
import { _resetDbForTests, putDownload } from './registry'
import type { OfflineDownload } from './types'

const rec = (over: Partial<OfflineDownload>): OfflineDownload => ({
  id: over.id ?? 'a:1', animeId: 'a', animeTitle: 'T',
  episode: { key: 1, label: 1, number: 1 }, quality: '720', streamType: 'hls',
  combo: { audio: 'sub', lang: 'en', provider: 'gogo', server: '', team: null },
  state: 'paused', bytes: 0, resourcesDone: 0, resourcesTotal: 0, createdAt: 1,
  playlistLocalPath: '/__offline/a%3A1/master.m3u8', subtitles: [], ...over,
})

describe('resumeNetworkPaused', () => {
  beforeEach(async () => { await _resetDbForTests(); enqueueDownload.mockClear() })

  it('re-enqueues only pausedBy:network records, rebuilding closures', async () => {
    await putDownload(rec({ id: 'net', pausedBy: 'network' }))
    await putDownload(rec({ id: 'manual' })) // user-paused: stays parked
    await putDownload(rec({ id: 'done', state: 'done', pausedBy: 'network' })) // stale flag on a finished record
    const n = await resumeNetworkPaused()
    expect(n).toBe(1)
    expect(enqueueDownload).toHaveBeenCalledTimes(1)
    expect(enqueueDownload.mock.calls[0][0]).toMatchObject({ animeId: 'a', subPref: undefined })
  })
})
