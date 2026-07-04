import type { Combo, StreamResult } from '@/types/aePlayer'
import type { EpisodeOption } from '@/components/player/EpisodeSelector.types'
import type { DownloadState } from './types'
import { enqueueDownload } from './downloadEngine'
import { getDownload } from './registry'

/** Episodes a season download should enqueue: everything not already stored,
 *  queued, or in flight. Paused and errored episodes re-enqueue (resume path). */
export function seasonTargets(
  episodes: EpisodeOption[],
  states: Record<number, DownloadState>,
): EpisodeOption[] {
  return episodes.filter((ep) => {
    const s = states[ep.number]
    return s !== 'done' && s !== 'queued' && s !== 'downloading'
  })
}

export interface SeasonContext {
  animeId: string
  animeTitle: string
  poster?: string
  combo: Combo
  quality: string
  /** Factory for the engine's fresh-resolve closure (re-called on signed-URL expiry). */
  resolveFor: (ep: EpisodeOption) => () => Promise<StreamResult>
}

/** Serially enqueue every target. The engine's per-download quota pre-check
 *  marks a record `error:'quota'` instead of queueing it — once that happens
 *  every later enqueue would fail the same check, so stop instead of spamming
 *  error records. Returns how many episodes were actually enqueued. */
export async function enqueueSeason(targets: EpisodeOption[], ctx: SeasonContext): Promise<number> {
  let enqueued = 0
  for (const ep of targets) {
    const id = await enqueueDownload({
      animeId: ctx.animeId,
      animeTitle: ctx.animeTitle,
      poster: ctx.poster,
      episode: ep,
      combo: ctx.combo,
      quality: ctx.quality,
      resolve: ctx.resolveFor(ep),
    })
    const rec = await getDownload(id)
    if (rec?.state === 'error' && rec.error === 'quota') break
    enqueued++
  }
  return enqueued
}
