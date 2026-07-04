import { describe, it, expect } from 'vitest'
import { makeOfflineResolver, offlineCapabilityReport, pickOfflineAutoSub, type OfflinePlayback } from './offlineAdapter'
import type { OfflineDownload } from './types'

function dl(n: number, over: Partial<OfflineDownload> = {}): OfflineDownload {
  return {
    id: `a1:${n}:gogoanime:sub:en::720`, animeId: 'a1', animeTitle: 'T', quality: '720',
    episode: { key: n, label: n, number: n }, streamType: 'hls', state: 'done',
    combo: { audio: 'sub', lang: 'en', provider: 'gogoanime', server: 's', team: null },
    bytes: 1, resourcesDone: 2, resourcesTotal: 2, createdAt: n,
    playlistLocalPath: `/__offline/a1:${n}/master.m3u8`,
    subtitles: [{ url: `/__offline/a1:${n}/sub/0`, provider: 'jimaku', lang: 'ja', label: 'JA', format: 'ass' }],
    ...over,
  }
}

const p: OfflinePlayback = { animeId: 'a1', title: 'T', downloads: [dl(2), dl(1)] }

describe('makeOfflineResolver', () => {
  const r = makeOfflineResolver(p)
  it('lists downloaded episodes sorted by number', async () => {
    expect((await r.listEpisodes('offline', 'a1')).map((e) => e.number)).toEqual([1, 2])
  })
  it('resolves a local StreamResult with local subtitles', async () => {
    const eps = await r.listEpisodes('offline', 'a1')
    const s = await r.resolveStream('offline', 'a1', eps[0], dl(1).combo)
    expect(s.url).toBe('/__offline/a1:1/master.m3u8')
    expect(s.type).toBe('hls')
    expect(s.subtitles?.[0].url).toBe('/__offline/a1:1/sub/0')
  })
  it('throws for an episode that is not downloaded', async () => {
    await expect(r.resolveStream('offline', 'a1', { key: 9, label: 9, number: 9 }, dl(1).combo))
      .rejects.toThrow()
  })
  it('ignores non-done downloads', async () => {
    const r2 = makeOfflineResolver({ ...p, downloads: [dl(1, { state: 'error' })] })
    expect(await r2.listEpisodes('offline', 'a1')).toEqual([])
  })
})

describe('offlineCapabilityReport', () => {
  it('exposes exactly one active selectable provider named offline', () => {
    const rep = offlineCapabilityReport(p)
    expect(rep.anime_id).toBe('a1')
    expect(rep.families).toHaveLength(1)
    expect(rep.families[0].providers).toHaveLength(1)
    expect(rep.families[0].providers[0]).toMatchObject({ provider: 'offline', state: 'active', selectable: true })
  })
})

describe('pickOfflineAutoSub', () => {
  const sub = { url: '/__offline/a%3A1/sub/0', provider: 'jimaku', lang: 'ja', label: 'J', format: 'ass' }
  const play = (over: Partial<OfflineDownload>) => ({ animeId: 'a', title: 'T', downloads: [dl(1, over)] })

  it('returns the stream track matching the record autoSubUrl', () => {
    expect(pickOfflineAutoSub(play({ state: 'done', autoSubUrl: sub.url }), 1, [sub])).toEqual(sub)
  })
  it('null when no autoSubUrl / episode not done / track missing from stream', () => {
    expect(pickOfflineAutoSub(play({ state: 'done' }), 1, [sub])).toBeNull()
    expect(pickOfflineAutoSub(play({ state: 'paused', autoSubUrl: sub.url }), 1, [sub])).toBeNull()
    expect(pickOfflineAutoSub(play({ state: 'done', autoSubUrl: sub.url }), 1, [])).toBeNull()
  })
})
