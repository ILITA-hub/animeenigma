// Offline playback masquerades as one more provider through the SAME seams the
// live player uses: a ProviderResolver + a capability report. AePlayer's
// default-selection machinery then needs zero special-casing — the synthetic
// feed has exactly one active provider, so it wins every pick.
import type { ProviderResolver } from '@/composables/aePlayer/useProviderResolver'
import type { EpisodeOption } from '@/components/player/EpisodeSelector.types'
import type { StreamResult } from '@/types/aePlayer'
import type { CapabilityReport } from '@/types/capabilities'
import type { OfflineDownload } from './types'

export interface OfflinePlayback {
  animeId: string
  title: string
  downloads: OfflineDownload[]
}

export const OFFLINE_PROVIDER_ID = 'offline'

function ready(p: OfflinePlayback): OfflineDownload[] {
  return p.downloads
    .filter((d) => d.state === 'done')
    .sort((a, b) => a.episode.number - b.episode.number)
}

export function makeOfflineResolver(p: OfflinePlayback): ProviderResolver {
  return {
    async listEpisodes(): Promise<EpisodeOption[]> {
      return ready(p).map((d) => d.episode)
    },
    async resolveStream(_provider, _animeId, ep): Promise<StreamResult> {
      const d = ready(p).find((x) => x.episode.number === ep.number)
      if (!d) throw new Error(`episode ${ep.number} is not downloaded`)
      return { url: d.playlistLocalPath, type: d.streamType, subtitles: d.subtitles }
    },
    async listTeams(): Promise<string[]> {
      return []
    },
  }
}

/** Synthetic one-provider feed. The REAL CapabilityReport shape is
 *  `{ anime_id, families: SourceFamily[] }` (types/capabilities.ts) — NOT a
 *  flat providers array; rowsFromReport() hard-requires Array.isArray(families)
 *  and returns [] otherwise, which would leave the offline player sourceless.
 *  Group 'firstparty' serves every lang, so the saved-combo restore can never
 *  filter the offline row out. */
export function offlineCapabilityReport(p: OfflinePlayback): CapabilityReport {
  const first = ready(p)[0]
  const audio = first?.combo.audio ?? 'sub'
  return {
    anime_id: p.animeId,
    families: [
      {
        family: 'offline',
        providers: [
          {
            provider: OFFLINE_PROVIDER_ID,
            display_name: 'Offline',
            state: 'active',
            selectable: true,
            hacker_only: false,
            order: 1,
            group: 'firstparty',
            audios: audio === 'dub' ? ['dub', 'sub'] : ['sub', 'dub'],
            reason: '',
            variants: [],
          },
        ],
      },
    ],
  }
}
