import type { SubtitleTrack } from '@/types/aePlayer'
import type { EpisodeOption } from '@/components/player/EpisodeSelector.types'
import { subtitlesApi } from '@/api/client'
import { flattenAggregateSubs, type AggregateSubsResponse } from '@/composables/aePlayer/useSubtitleTracks'
import type { SubPref, SubOption } from './types'

function matchExternal(tracks: SubtitleTrack[], pref: Extract<SubPref, { kind: 'external' }>): SubtitleTrack | undefined {
  const same = tracks.filter((t) => t.provider === pref.provider && t.lang === pref.lang)
  return same.find((t) => t.label === pref.label) ?? same[0]
}

/** Local cached URL of the track the user asked to auto-enable, resolved
 *  against the download's cached track list. Missing match (track absent for
 *  this episode, fetch failed) → undefined; the download itself never fails. */
export function matchAutoSub(pref: SubPref | null | undefined, subs: SubtitleTrack[], streamProvider: string): string | undefined {
  if (!pref) return undefined
  if (pref.kind === 'external') return matchExternal(subs, pref)?.url
  const bundled = subs.filter((s) => s.provider === streamProvider)
  return (pref.lang === 'auto' ? bundled[0] : bundled.find((s) => s.lang === pref.lang))?.url
}

/** Per-episode fetch closure factory for external tracks — aggregated
 *  /subtitles/all URLs are episode-specific, so a season batch re-queries per
 *  episode. Returns undefined for non-external prefs (bundled tracks already
 *  ride the resolved stream). */
export function makeExternalSubResolver(
  animeId: string,
  pref: SubPref | null | undefined,
): ((ep: EpisodeOption) => () => Promise<SubtitleTrack[]>) | undefined {
  if (pref?.kind !== 'external') return undefined
  return (ep) => async () => {
    const resp = await subtitlesApi.all(animeId, ep.number)
    const data = (resp.data?.data ?? resp.data) as AggregateSubsResponse
    const hit = matchExternal(flattenAggregateSubs(data), pref)
    return hit ? [hit] : []
  }
}

/** Download-dialog options for external (aggregated) tracks — shared by the
 *  in-player and card-flow hosts so the option key format, dedup rule, and
 *  pref shape live in exactly one place. */
export function externalSubOptions(tracks: readonly SubtitleTrack[]): SubOption[] {
  const opts: SubOption[] = []
  const seen = new Set<string>()
  for (const tr of tracks) {
    const key = `e:${tr.provider}:${tr.lang}:${tr.label}`
    if (seen.has(key)) continue
    seen.add(key)
    opts.push({ key, label: `${tr.label} · ${tr.lang.toUpperCase()}`, pref: { kind: 'external', provider: tr.provider, lang: tr.lang, label: tr.label } })
  }
  return opts
}
