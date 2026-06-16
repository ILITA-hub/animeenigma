/** One provider's learned-reliability record (mirrors catalog sourceranking.Record JSON). */
export interface SourceRecord {
  provider: string
  score: number
  reached_rate: number
  ok_rate: number
  p95_ms: number
  stall_rate: number
  samples: number
}

/** GET /api/anime/{id}/source-ranking payload (the {success,data} `data`). */
export interface SourceRanking {
  global: SourceRecord[]
  perAnime: SourceRecord[]
  /** Same-day override provider id (srcfix), or '' when none. */
  fix: string
}
