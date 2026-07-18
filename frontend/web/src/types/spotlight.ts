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
 *  4. `PlatformStatsData` was refactored in Phase 27 (platform-stats-jc)
 *     from a flat `metrics: PlatformMetric[]` array to a structured
 *     `{ hero: StatsHero, tiles: StatsTile[] }` shape. The old
 *     `PlatformMetric` interface has been removed. `PlatformStatsCard.vue`
 *     was updated to match in the same phase.
 *  5. Per RESEARCH.md Pitfall 8, fields stay snake_case all the way from
 *     backend through composable to card component — no transform step.
 */

/* ──────────────────────────────────────────────────────────────────────── */
/*  Shared anime sub-shape used by featured + random_tail card variants.    */
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
  // genre chips (UI-SPEC §FeaturedCard) tolerate undefined gracefully.
  genres?: { id: string; name?: string; russian?: string }[]
}

/* ──────────────────────────────────────────────────────────────────────── */
/*  Per-card data payloads.                                                 */
/* ──────────────────────────────────────────────────────────────────────── */

export interface FeaturedData {
  anime: SpotlightAnime
  // Phase 1 reserves an optional reason key (e.g. "featured.seasonal").
  // Currently absent from the runtime payload but defined here so a future
  // backend bump does not require a coordinated frontend type change.
  reason_i18n_key?: string
}

export interface CuratedData {
  anime: SpotlightAnime
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

export interface StatsHero {
  working_ok: boolean
  // Real 7-day uptime %, from Prometheus. Omitted/null when Prometheus is
  // unreachable — the card then shows the quip without a number.
  uptime_percent?: number | null
  uptime_quip: string
  service: string
  ux_delta: string
  cdi: string
  mvq: string
  tagline: string
}

export interface StatsTile {
  label: string
  value: number
  window: 'day' | 'week' | 'all'
  format: 'int' | 'bytes' | 'seconds'
}

export interface PlatformStatsData {
  hero: StatsHero
  tiles: StatsTile[]
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
 * the human-facing `title?` / `excerpt` / external `link?` / ISO `date?` /
 * optional `image_url?`.
 * The card renders excerpts as line-clamp-3 with an "Open post →" anchor
 * carrying `rel="noopener noreferrer"` (T-03-18 in the threat register).
 *
 * `image_url` added in v1.1-polish Phase 06 (HSB-V11-TG-01). The Telegram
 * channel scraper already extracts `background-image:url(...)` from
 * `.tgme_widget_message_photo_wrap`; the spotlight backend now surfaces
 * the field. Roughly 30% of @anime_enigma posts carry a CDN URL
 * (cdn4.telesco.pe); text-only posts emit the field as omitted/undefined.
 */
export interface TelegramPost {
  title?: string
  excerpt: string
  link?: string
  date?: string
  image_url?: string
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
  /**
   * ISO-8601 timestamp of when the user added the anime to their list
   * (anime_list.updated_at). snake_case to match the Go `added_at` JSON
   * tag — emitted with `omitempty`, so it may be absent. The card renders
   * it as a relative "Added X ago" line via formatAgo() when present.
   */
  added_at?: string | null
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

/**
 * DailyFanficCard — a single AI-authored/user-submitted fanfic excerpt for
 * the daily "Fanfic Spotlight". Mirrors the fanfic-engine wire DTO
 * (services/fanfic, port 8097) — snake_case end-to-end per Pitfall 8.
 */
export interface DailyFanficData {
  id: string
  fanfic_title: string
  anime_title: string
  anime_japanese: string
  anime_poster: string
  excerpt: string
  rating: string
  language: string
  explicit: boolean
  author_username: string
  credited: boolean
  ai_generated: boolean
  part_count: number
  created_at: string
}

/**
 * UpcomingForYouCard — login-only announcement matches (spec 2026-07-17,
 * relevance-hardened 2026-07-18). `reason.kind`:
 *   - `'franchise'`   — matched a title in a franchise the viewer rated;
 *                       carries the seed anime's names + the viewer's score.
 *   - `'attribute'`   — matched on a shared attribute; `attribute` is the
 *                       dimension (`'studio' | 'source'`) and `attribute_name`
 *                       its value (studio display name, or a source code that
 *                       maps through `anime.sources.*`).
 *   - `'anticipated'` — pool-relative MAL popularity is high.
 *   - `'taste'`       — generic attribute-affinity fallback.
 */
export interface UpcomingReasonFE {
  kind: 'franchise' | 'attribute' | 'anticipated' | 'taste'
  seed_anime_id?: string
  seed_anime_name?: string
  seed_anime_name_ru?: string
  user_score?: number
  attribute?: string
  attribute_name?: string
}

export interface UpcomingForYouItem {
  anime: SpotlightAnime
  match_score: number
  reason: UpcomingReasonFE
}

/** Login-only announcement matches — `upcoming_for_you` card. */
export interface UpcomingForYouData {
  items: UpcomingForYouItem[]
}

/* ──────────────────────────────────────────────────────────────────────── */
/*  Discriminated union — narrows correctly in the v-if/v-else-if dispatch  */
/*  chain inside HeroSpotlightBlock.vue.                                    */
/* ──────────────────────────────────────────────────────────────────────── */

export type SpotlightCard = (
  | { type: 'featured'; data: FeaturedData }
  | { type: 'random_tail'; data: RandomTailData }
  | { type: 'latest_news'; data: LatestNewsData }
  | { type: 'platform_stats'; data: PlatformStatsData }
  | { type: 'personal_pick'; data: PersonalPickData }
  | { type: 'telegram_news'; data: TelegramNewsData }
  | { type: 'now_watching'; data: NowWatchingData }
  | { type: 'not_time_yet'; data: NotTimeYetData }
  | { type: 'continue_watching_new'; data: ContinueWatchingNewData }
  | { type: 'curated'; data: CuratedData }
  | { type: 'daily_fanfic'; data: DailyFanficData }
  | { type: 'upcoming_for_you'; data: UpcomingForYouData }
) & { priority?: number }

/* ──────────────────────────────────────────────────────────────────────── */
/*  Top-level fetch envelope returned by `GET /api/home/spotlight`.         */
/*  Catalog endpoints sometimes wrap responses in `{success, data:{...}}` — */
/*  useSpotlight() unwraps that defensively (precedent useContinueWatching).*/
/* ──────────────────────────────────────────────────────────────────────── */

export interface SpotlightResponse {
  cards: SpotlightCard[]
  generated_at: string
}
