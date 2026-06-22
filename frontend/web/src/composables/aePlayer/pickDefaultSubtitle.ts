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
