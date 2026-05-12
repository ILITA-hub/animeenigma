export interface WatchCombo {
  // Phase 16: 'english' is the new unified English-source player that replaces
  // the visible 'hianime' + 'consumet' tabs at the UI; the legacy values stay
  // in the union for ?legacy=1 debug paths and existing watch_history rows.
  player: 'kodik' | 'animelib' | 'hianime' | 'consumet' | 'hanime' | 'english'
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
