import type { Combo, SubtitleTrack } from '@/types/aePlayer'
import type { EpisodeOption } from '@/components/player/EpisodeSelector.types'

export type DownloadState = 'queued' | 'downloading' | 'paused' | 'done' | 'error'
export type DownloadError = 'network' | 'quota' | 'evicted' | 'resolve' | 'mismatch'

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
  /** Local resume position, written by offline playback. */
  lastPositionSec?: number
  /** Entry URL for the player: /__offline/{id}/master.m3u8 or /__offline/{id}/media.mp4 */
  playlistLocalPath: string
  /** Subtitle tracks rewritten to /__offline/{id}/sub/{k} local URLs. */
  subtitles: SubtitleTrack[]
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
