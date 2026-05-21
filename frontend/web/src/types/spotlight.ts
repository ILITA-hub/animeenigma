/**
 * Workstream hero-spotlight — Phase 2 (frontend-carousel) Plan 02-01,
 * extended by Phase 3 (dynamic-cards-migration) Plan 03-05 with 5 new variants.
 *
 * Discriminated-union types for the `GET /api/home/spotlight` envelope. All
 * 9 SpotlightCard variants are declared here; HeroSpotlightBlock.vue dispatches
 * via an explicit v-if/v-else-if chain (NOT `<component :is>`) so vue-tsc
 * narrows `card.data` for each branch.
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
/*  Phase 3 (Plan 03-05) additions — 5 new card variants                    */
/* ──────────────────────────────────────────────────────────────────────── */

/**
 * PersonalPickCard — 1..3 picks. When the viewer is anonymous, the backend
 * downgrades to `source: 'trending'`; logged-in users get a personalized list
 * (`source: 'personal'`). The UI uses `source` to swap the title key and the
 * mobile-footer "+ N more →" router target (/browse?sort=trending vs /recs).
 */
export interface PersonalPickItem {
  anime: SpotlightAnime
  // Optional i18n key (e.g. "spotlight.personalPick.reason.becauseYouWatched")
  // — the card renders `t(item.reason_i18n_key)` when present.
  reason_i18n_key?: string
}
export interface PersonalPickData {
  items: PersonalPickItem[]
  source: 'trending' | 'personal'
}

/**
 * TelegramNewsCard — 1..3 telegram-channel post excerpts. Backend supplies
 * the human-facing `title?` / `excerpt` / external `link?` / ISO `date?`.
 * The card renders excerpts as line-clamp-2 with an "Open post →" anchor
 * carrying `rel="noopener noreferrer"` (T-03-18 in the threat register).
 */
export interface TelegramPost {
  title?: string
  excerpt: string
  link?: string
  date?: string
}
export interface TelegramNewsData {
  posts: TelegramPost[]
}

/**
 * NowWatchingCard — 1..3 live-watching sessions. Each row renders a green
 * "LIVE" dot, the username (linking to the public profile), the anime title
 * (linking to detail), and the current episode number. Backend enforces
 * HSB-NF-04 (no PII beyond username/public_id/anime-fields).
 */
export interface NowWatchingSession {
  username: string
  public_id: string
  anime_id: string
  anime_name?: string
  anime_name_ru?: string
  poster_url?: string
  episode_number: number
  updated_at: string
}
export interface NowWatchingData {
  sessions: NowWatchingSession[]
}

/**
 * NotTimeYetCard — single anime from the viewer's Planned or Postponed list
 * that the backend deemed "ready to start". The UI swaps the subtitle key
 * based on `status` and links to the anime detail page for an opt-in start.
 */
export interface NotTimeYetData {
  anime: SpotlightAnime
  status: 'planned' | 'postponed'
}

/**
 * ContinueWatchingNewCard — single anime where the user finished
 * `last_watched_episode` and a `new_episode_number` aired since. The card
 * renders a purple "New ep N!" badge and a "Resume" CTA.
 */
export interface ContinueWatchingNewData {
  anime: SpotlightAnime
  last_watched_episode: number
  new_episode_number: number
}

/* ──────────────────────────────────────────────────────────────────────── */
/*  Discriminated union — narrows correctly in the v-if/v-else-if dispatch  */
/*  chain inside HeroSpotlightBlock.vue.                                    */
/* ──────────────────────────────────────────────────────────────────────── */

export type SpotlightCard =
  | { type: 'anime_of_day'; data: AnimeOfDayData }
  | { type: 'random_tail'; data: RandomTailData }
  | { type: 'latest_news'; data: LatestNewsData }
  | { type: 'platform_stats'; data: PlatformStatsData }
  | { type: 'personal_pick'; data: PersonalPickData }
  | { type: 'telegram_news'; data: TelegramNewsData }
  | { type: 'now_watching'; data: NowWatchingData }
  | { type: 'not_time_yet'; data: NotTimeYetData }
  | { type: 'continue_watching_new'; data: ContinueWatchingNewData }

/* ──────────────────────────────────────────────────────────────────────── */
/*  Top-level fetch envelope returned by `GET /api/home/spotlight`.         */
/*  Catalog endpoints sometimes wrap responses in `{success, data:{...}}` — */
/*  useSpotlight() unwraps that defensively (precedent useContinueWatching).*/
/* ──────────────────────────────────────────────────────────────────────── */

export interface SpotlightResponse {
  cards: SpotlightCard[]
  generated_at: string
}
