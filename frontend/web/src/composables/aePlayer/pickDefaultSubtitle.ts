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
//
// NOTE: aePlayer subtitles default OFF with no exceptions (incl. raw-JP). There
// is deliberately NO auto-select helper — the player never enables an overlay on
// its own; the user opts in via the Subtitles menu and that choice persists
// across episodes (AePlayer re-binds the chosen language on each episode).
export function pickBestForLang(tracks: SubTrack[], lang: string): SubTrack | null {
  return best(tracks.filter((t) => t.lang === lang))
}

/**
 * A "hardsubbed" cut has its subtitles burned into the video pixels rather than
 * shipped as a selectable soft track. Rendering a soft overlay on top of one
 * DOUBLES the subtitles — the "Субтитры накладываются" (subtitles overlap)
 * report. True only for an EN/RU SUB stream that ships no provider-bundled soft
 * track and is playing on a real provider. A raw JP cut (`lang === 'ja'`) is
 * never hardsubbed — its subs come from the optional Jimaku/OpenSubtitles
 * overlay — and a provider that bundles a real soft track isn't hardsubbed
 * either. AePlayer uses this to suppress the soft-overlay UI (quick rows, the
 * persisted-language re-bind, and any track carried over from a prior source) so
 * the burned-in subs are never doubled.
 */
export function isHardsubbedCut(opts: {
  audio: 'sub' | 'dub'
  lang: string
  hasBundledSoftTracks: boolean
  hasActiveProvider: boolean
}): boolean {
  return (
    opts.audio === 'sub' &&
    opts.lang !== 'ja' &&
    !opts.hasBundledSoftTracks &&
    opts.hasActiveProvider
  )
}
