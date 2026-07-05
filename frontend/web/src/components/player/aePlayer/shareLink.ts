import type { Combo } from '@/types/aePlayer'

/** The combo facets that survive into a shareable link. `server` is excluded
 *  deliberately — server ids rotate and the read side (Anime.vue) has no
 *  `?server=` support, so it would be dead weight. */
export type ShareCombo = Pick<Combo, 'audio' | 'lang' | 'provider' | 'team'>

export interface ShareLinkInput {
  /** window.location.origin, e.g. "https://animeenigma.org". */
  origin: string
  animeId: string
  combo: ShareCombo
  /** 1-based episode number; ≤ 0 omits the param. */
  episode: number
  /** current playback position in seconds; floored, ≤ 0 omits the param. */
  timeSec: number
}

/**
 * Build the "share this exact moment" link consumed on load by Anime.vue's
 * query readers (`?provider/?team/?audio/?lang/?episode/?t`). Unlike the passive
 * address-bar url-sync — which only writes a *pinned* source — this captures the
 * live resolved combo unconditionally, so the recipient lands on the exact same
 * source, episode, and timestamp the sharer was watching.
 *
 * Empty facets are omitted; `team` rides along only when there's a `provider`
 * (it is meaningless without one). The timestamp is floored to whole seconds.
 */
export function buildShareUrl(input: ShareLinkInput): string {
  const { origin, animeId, combo, episode, timeSec } = input
  const params = new URLSearchParams()
  if (combo.provider) {
    params.set('provider', combo.provider)
    if (combo.team) params.set('team', combo.team)
  }
  if (combo.audio) params.set('audio', combo.audio)
  if (combo.lang) params.set('lang', combo.lang)
  if (episode > 0) params.set('episode', String(episode))
  const t = Math.floor(timeSec)
  if (t > 0) params.set('t', String(t))
  const qs = params.toString()
  return `${origin}/anime/${animeId}${qs ? `?${qs}` : ''}`
}
