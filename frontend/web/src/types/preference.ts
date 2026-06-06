export interface WatchCombo {
  player: 'kodik' | 'animelib' | 'hanime' | 'english'
  language: 'ru' | 'en' | '18+'
  watch_type: 'dub' | 'sub'
  translation_id: string
  translation_title: string
  /**
   * Episodes this team/translation actually has loaded into our sources. Used
   * by the resume state machine to override Shikimori's lagging `episodesAired`
   * so a freshly-uploaded episode isn't mislabeled "not loaded yet". Optional —
   * players that don't surface a per-translation count omit it. Not consumed by
   * the backend preference resolver (it ignores unknown fields).
   */
  episodes_count?: number
}

export interface ResolvedCombo extends WatchCombo {
  tier: string
  tier_number: number
}

export interface ResolveResponse {
  resolved: ResolvedCombo | null
}
