import type { SubtitleTrack as SubTrack } from '@/types/aePlayer'

function providerRank(provider: string): number {
  if (provider === 'jimaku') return 0
  if (provider === 'opensubtitles') return 2
  return 1 // provider-own (gogoanime, etc.)
}

function best(tracks: SubTrack[]): SubTrack | null {
  if (tracks.length === 0) return null
  return [...tracks].sort((a, b) => providerRank(a.provider) - providerRank(b.provider))[0]
}

export function pickDefaultSubtitle(tracks: SubTrack[], opts: { lang: string }): SubTrack | null {
  if (tracks.length === 0) return null
  const matches = tracks.filter((t) => t.lang === opts.lang)
  return best(matches) ?? best(tracks)
}

// Best track for EXACTLY this language (no cross-language fallback). Used by the
// quick chooser's RU/EN/JP fast buttons.
export function pickBestForLang(tracks: SubTrack[], lang: string): SubTrack | null {
  return best(tracks.filter((t) => t.lang === lang))
}

/**
 * Decide which subtitle (if any) to AUTO-enable when a SUB cut resolves.
 *
 * Rules (in order):
 *  1. A soft track the PROVIDER shipped with the stream (`bundled`) is a real
 *     selectable subtitle the provider intends — always honor the first one.
 *  2. No provider track, raw original-Japanese cut (`lang === 'ja'`): nothing is
 *     burned into the video, so auto-enable the best aggregated track
 *     (Jimaku/OpenSubtitles) for that language.
 *  3. No provider track, EN/RU cut: the provider HARDSUBBED the subtitles into
 *     the video. Auto-enabling an aggregated overlay on top would just double
 *     the subtitles, so return null — the user can still pick one manually.
 *
 * `aggregated` is the merged track list (provider-bundled + Jimaku/OpenSubtitles);
 * it is only consulted in rule 2, where `bundled` is empty so it carries no
 * provider tracks.
 */
export function pickAutoSubtitle(opts: {
  lang: string
  bundled: SubTrack[]
  aggregated: SubTrack[]
}): SubTrack | null {
  if (opts.bundled.length > 0) return opts.bundled[0]
  if (opts.lang !== 'ja') return null // burned into the video by the provider
  return pickDefaultSubtitle(opts.aggregated, { lang: opts.lang })
}
