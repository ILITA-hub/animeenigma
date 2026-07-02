import 'fake-indexeddb/auto'
import { describe, it, expect, beforeEach } from 'vitest'
import { putDownload, getDownload, listDownloads, deleteDownloadRecord, enqueuePending, drainPending, _resetDbForTests } from './registry'
import { downloadId, offlinePath, type OfflineDownload } from './types'

function sample(id: string): OfflineDownload {
  return {
    id, animeId: 'a1', animeTitle: 'Test', quality: '720', streamType: 'hls',
    episode: { key: 1, label: 1, number: 1 },
    combo: { audio: 'sub', lang: 'en', provider: 'gogoanime', server: 's1', team: null },
    state: 'queued', bytes: 0, resourcesDone: 0, resourcesTotal: 0, createdAt: 1,
    playlistLocalPath: offlinePath(id, 'master.m3u8'), subtitles: [],
  }
}

beforeEach(() => _resetDbForTests())

describe('registry CRUD', () => {
  it('put/get/list/delete round-trips', async () => {
    const id = downloadId('a1', 1, sample('x').combo, '720')
    await putDownload(sample(id))
    expect((await getDownload(id))?.animeTitle).toBe('Test')
    expect((await listDownloads()).map((d) => d.id)).toEqual([id])
    await deleteDownloadRecord(id)
    expect(await getDownload(id)).toBeUndefined()
  })
  it('put overwrites by id (state transition)', async () => {
    await putDownload(sample('k'))
    await putDownload({ ...sample('k'), state: 'done' })
    expect((await getDownload('k'))?.state).toBe('done')
    expect((await listDownloads()).length).toBe(1)
  })
})

describe('pending_progress queue', () => {
  it('drains FIFO and deletes handled entries; stops on handler failure', async () => {
    await enqueuePending({ n: 1 })
    await enqueuePending({ n: 2 })
    await enqueuePending({ n: 3 })
    const handled: number[] = []
    const ok = await drainPending(async (p) => {
      const n = (p as { n: number }).n
      if (n === 3) return false // simulate network failure
      handled.push(n)
      return true
    })
    expect(handled).toEqual([1, 2])
    expect(ok).toBe(false)
    // entry 3 survives for the next drain
    const rest: number[] = []
    await drainPending(async (p) => { rest.push((p as { n: number }).n); return true })
    expect(rest).toEqual([3])
  })
})
