export interface WatchCombo {
  player: 'kodik' | 'animelib' | 'hianime' | 'consumet' | 'hanime'
  language: 'ru' | 'en' | '18+'
  watch_type: 'dub' | 'sub'
  translation_id: string
  translation_title: string
}

export interface ResolvedCombo extends WatchCombo {
  tier: string
  tier_number: number
}

export interface ResolveResponse {
  resolved: ResolvedCombo | null
}
