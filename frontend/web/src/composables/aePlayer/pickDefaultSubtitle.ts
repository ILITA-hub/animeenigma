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
