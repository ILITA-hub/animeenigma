import type { Combo, SubtitleTrack } from '@/types/aePlayer'
import type { EpisodeOption } from '@/components/player/EpisodeSelector.types'

export type DownloadState = 'queued' | 'downloading' | 'paused' | 'done' | 'error'
export type DownloadError = 'network' | 'quota' | 'evicted' | 'resolve' | 'mismatch'

/** Download-time subtitle choice. A DESCRIPTOR, never a URL — aggregated
 *  track URLs are per-episode and signed URLs expire in queue; the engine
 *  re-resolves the concrete track for each episode. */
export type SubPref =
  | { kind: 'bundled'; lang: string } // lang 'auto' = first provider-bundled track
  | { kind: 'external'; provider: string; lang: string; label?: string }

/** One entry of the download dialog's subtitle picker (built by the hosts —
 *  labels are i18n'd there; this stays a plain data shape). */
export interface SubOption { key: string; label: string; pref: SubPref }

export interface OfflineDownload {
  /** Canonical key — NEVER a raw URL (signed proxy URLs expire hourly). */
  id: string
  animeId: string
  animeTitle: string
  episode: EpisodeOption
  combo: Combo
  quality: string // target: '480' | '720' | '1080'
  streamType: 'hls' | 'mp4'
  state: DownloadState
  error?: DownloadError
  bytes: number
  resourcesDone: number
  resourcesTotal: number
  createdAt: number
  /** Duration-scaled size estimate (projectedBytesFor at enqueue) — the
   *  "~450 MB" the UI shows next to live bytes. Absent on legacy records. */
  projectedBytes?: number
  /** Local resume position, written by offline playback. */
  lastPositionSec?: number
  /** Entry URL for the player: /__offline/{id}/master.m3u8 or /__offline/{id}/media.mp4 */
  playlistLocalPath: string
  /** Subtitle tracks rewritten to /__offline/{id}/sub/{k} local URLs. */
  subtitles: SubtitleTrack[]
  /** Download-time subtitle choice (descriptor; see SubPref). */
  subPref?: SubPref
  /** Local /__offline/{id}/sub/{k} URL of the track matching subPref —
   *  offline playback auto-enables exactly this track. */
  autoSubUrl?: string
  /** 'network' when the cellular guard parked it (auto-resumed on Wi-Fi).
   *  Manual pauses leave it unset and are never auto-resumed. */
  pausedBy?: 'network'
  /** /__offline/{id}/poster when the poster fetch succeeded. */
  posterPath?: string
}

export function downloadId(animeId: string, epNumber: number, combo: Combo, quality: string): string {
  return [animeId, epNumber, combo.provider, combo.audio, combo.lang, combo.team ?? '', quality].join(':')
}

export function offlineCacheName(id: string): string {
  return `ae-offline-${id}`
}

export function offlinePath(id: string, rest: string): string {
  return `/__offline/${encodeURIComponent(id)}/${rest}`
}
