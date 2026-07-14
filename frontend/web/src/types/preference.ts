/** Known legacy player keys. The union documents the historical set; the
 *  `(string & Record<never, never>)` arm (the eslint-safe spelling of the
 *  `string & {}` autocomplete trick) keeps any roster-supplied player_key
 *  assignable while preserving autocomplete (AUTO-608 — the roster, not this
 *  type, is the authority for which players exist). */
export type LegacyPlayerKey = 'kodik' | 'animelib' | 'hanime' | 'english' | 'ae'

export interface WatchCombo {
  player: LegacyPlayerKey | (string & Record<never, never>)
  language: 'ru' | 'en' | '18+' | 'ja'
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
