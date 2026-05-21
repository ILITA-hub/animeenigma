/**
 * Workstream hero-spotlight — Phase 2 (frontend-carousel) Plan 02-01.
 *
 * Discriminated-union types for the `GET /api/home/spotlight` envelope.
 *
 * Field shapes were verified against the LIVE Phase 1 endpoint on 2026-05-21
 * via:
 *   curl -s http://localhost:8000/api/home/spotlight | jq '.'
 *
 * Key observations from the live payload that differ from the Phase 2 design
 * doc (docs/superpowers/specs/2026-05-21-hero-spotlight-block-design.md §4.1)
 * and from RESEARCH.md Pattern 5:
 *
 *  1. Cards have NO envelope-level `id` field. The discriminator is `type`
 *     only. Vue keying must fall back to `${type}:${index}` (see Pitfall 10
 *     in 02-RESEARCH.md). Including `id?` here would be misleading.
 *  2. `LatestNewsCard` entries are `{date, type, message}` — NOT
 *     `{id, date, title, summary}` as the design doc claimed. The backend's
 *     Phase 1 resolver reads `frontend/web/public/changelog.json` whose
 *     records use `date`/`type`/`message`. We match the runtime shape.
 *  3. Anime objects are MUCH richer than the design doc's minimal
 *     `SpotlightAnime` — backend ships the full catalog row (description,
 *     year, season, status, kind, rating, has_* flags, etc.). We declare
 *     them all here so card components have full type-safety, but every
 *     extra field is optional.
 *  4. `PlatformMetric.delta` is not currently emitted by the Phase 1
 *     resolver (snapshot showed only `{key, value}`). We keep it as
 *     `delta?: number | null` so the card can render delta when the
 *     backend adds it without a frontend type bump (HSB-FE-22 forward-
 *     compatible).
 *  5. Per RESEARCH.md Pitfall 8, fields stay snake_case all the way from
 *     backend through composable to card component — no transform step.
 *
 * Phase 3 will add additional card variants (personal_pick, now_watching,
 * telegram_news, not_time_yet, continue_watching_new). Those are NOT
 * forward-declared here — adding them now without backend support would
 * make `cardFor()` switch coverage misleadingly "exhaustive". Phase 3's
 * plan owns extending this union.
 */

/* ──────────────────────────────────────────────────────────────────────── */
/*  Shared anime sub-shape used by anime_of_day + random_tail card variants. */
/* ──────────────────────────────────────────────────────────────────────── */

export interface SpotlightAnime {
  id: string
  name?: string
  name_en?: string
  name_ru?: string
  name_jp?: string
  description?: string
  year?: number
  season?: string
  status?: string
  kind?: string
  rating?: string
  material_source?: string
  episodes_count?: number
  episodes_aired?: number
  episode_duration?: number
  score?: number
  poster_url?: string
  shikimori_id?: string
  mal_id?: string
  has_video?: boolean
  has_dub?: boolean
  has_kodik?: boolean
  has_animelib?: boolean
  has_raw?: boolean
  has_english?: boolean
  hidden?: boolean
  aired_on?: string
  created_at?: string
  updated_at?: string
  // Optional — only present on some catalog rows. Card UIs that show
  // genre chips (UI-SPEC §AnimeOfDayCard) tolerate undefined gracefully.
  genres?: { id: string; name?: string; russian?: string }[]
}

/* ──────────────────────────────────────────────────────────────────────── */
/*  Per-card data payloads.                                                 */
/* ──────────────────────────────────────────────────────────────────────── */

export interface AnimeOfDayData {
  anime: SpotlightAnime
  // Phase 1 reserves an optional reason key (e.g. "anime_of_day.seasonal").
  // Currently absent from the runtime payload but defined here so a future
  // backend bump does not require a coordinated frontend type change.
  reason_i18n_key?: string
}

export interface RandomTailData {
  anime: SpotlightAnime
}

/**
 * One entry inside the latest_news card. Matches the runtime shape served
 * by the Phase 1 resolver, which mirrors `frontend/web/public/changelog.json`:
 *   { date: 'YYYY-MM-DD', type: 'feature' | 'fix' | ..., message: string }
 *
 * Note: the design doc and RESEARCH.md Pattern 5 specified
 * `{id, date, title, summary}` — that schema is NOT what the backend ships.
 * Card components must consume `message` for the body and may use the
 * leading sentence of `message` as a title fallback. See LatestNewsCard
 * implementation in Plan 02-05.
 *
 * TODO(spotlight): if Phase 1 backend is later updated to emit a structured
 * `{id, title, summary}` shape, extend this interface (optional fields, do
 * not break old clients) and update LatestNewsCard accordingly.
 */
export interface ChangelogEntry {
  date: string // ISO 8601 date (e.g. "2026-05-21") — calendar date, not datetime
  type?: string // 'feature' | 'fix' | 'improvement' | … (free-form per changelog.json)
  message: string
}

export interface LatestNewsData {
  entries: ChangelogEntry[]
}

export interface PlatformMetric {
  // Open union of known metric keys; backend may add more without a
  // frontend bump (the card renders unknown keys with their localized
  // label falling back to the raw key).
  key: 'anime_added_7d' | 'episodes_added_7d' | 'active_rooms_7d' | string
  value: number
  delta?: number | null
}

export interface PlatformStatsData {
  metrics: PlatformMetric[]
}

/* ──────────────────────────────────────────────────────────────────────── */
/*  Discriminated union — narrows correctly in `cardFor(card.type)`         */
/*  switch / map in HeroSpotlightBlock.vue.                                 */
/* ──────────────────────────────────────────────────────────────────────── */

export type SpotlightCard =
  | { type: 'anime_of_day'; data: AnimeOfDayData }
  | { type: 'random_tail'; data: RandomTailData }
  | { type: 'latest_news'; data: LatestNewsData }
  | { type: 'platform_stats'; data: PlatformStatsData }

/* ──────────────────────────────────────────────────────────────────────── */
/*  Top-level fetch envelope returned by `GET /api/home/spotlight`.         */
/*  Catalog endpoints sometimes wrap responses in `{success, data:{...}}` — */
/*  useSpotlight() unwraps that defensively (precedent useContinueWatching).*/
/* ──────────────────────────────────────────────────────────────────────── */

export interface SpotlightResponse {
  cards: SpotlightCard[]
  generated_at: string
}
