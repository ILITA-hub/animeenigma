# Requirements: AnimeEnigma `hero-spotlight` workstream — v1.0

**Milestone:** v1.0 HeroSpotlightBlock
**Defined:** 2026-05-21
**Core value:** Top-of-home rotating hero block surfacing 9 dynamic content types in a single auto-cycling carousel. Empty cards never appear (eligibility-filtered). Existing `trendingRecs` row replaced by the `personal_pick` card.
**Source design doc:** `/data/animeenigma/docs/superpowers/specs/2026-05-21-hero-spotlight-block-design.md`

Requirement IDs use prefix `HSB-` (Hero Spotlight Block) followed by a
category (`BE` backend, `FE` frontend, `MIG` migration, `NF` non-functional).

---

## v1.0 Requirements

### Backend — Aggregator endpoint (Phase 1)

- [ ] **HSB-BE-01**: New endpoint `GET /api/home/spotlight` in `services/catalog`,
  routed through gateway. Returns JSON `{cards: SpotlightCard[], generated_at: string}`.
  Optional `Authorization: Bearer <jwt>` — when present, login-only cards
  become eligible.
- [ ] **HSB-BE-02**: New package `services/catalog/internal/service/spotlight/`
  with `aggregator.go` and per-card resolvers under `cards/`. Each resolver
  implements `Resolve(ctx, userID *string) (Card, error)`.
- [ ] **HSB-BE-03**: Each card resolver runs with a per-card context deadline
  of **800ms**. If it times out or errors, the card is dropped (eligible=false)
  and a structured log entry `spotlight.card_failed{type, error}` is emitted.
- [ ] **HSB-BE-04**: Overall request budget **2 seconds**. On overall timeout,
  return the last-known-good cached snapshot (best-effort) via Redis key
  `spotlight:snapshot:<anon|user_id>:YYYY-MM-DD`.
- [ ] **HSB-BE-05**: Server-side eligibility filter — cards with `eligible=false`
  are excluded from the response payload entirely. Frontend never sees them.
- [ ] **HSB-BE-06**: Gateway routes `GET /api/home/spotlight → catalog:8081`
  (public route, optional auth via existing auth middleware).
- [ ] **HSB-BE-07**: Feature flag env `SPOTLIGHT_ENABLED` (default `true`) on
  the catalog service. When `false`, the endpoint returns `404 Not Found` and
  the frontend self-hides.

### Backend — Static cards (Phase 1)

- [ ] **HSB-BE-10**: `anime_of_day` resolver. Queries `catalog.GetTopRated(score_min=8.0, limit=200)`,
  picks `items[seed % len]` where `seed = YYYY*100*32 + MM*32 + DD`. Redis
  key `spotlight:anime_of_day:<YYYY-MM-DD>`, TTL **24h**.
- [ ] **HSB-BE-11**: `random_tail` resolver. Selects from animes ranked 101..2000
  by score (excluding top-100 to favor discovery). Same date-seeded pick.
  Redis key `spotlight:random_tail:<YYYY-MM-DD>`, TTL **24h**.
- [ ] **HSB-BE-12**: `latest_news` resolver. Fetches changelog via new
  `services/catalog/internal/service/spotlight/client/web_client.go` →
  `http://web:80/changelog.json` inside docker network. Returns first 3 entries
  (newest). Redis key `spotlight:changelog:<YYYY-MM-DD>`, TTL **24h**.
- [ ] **HSB-BE-13**: `platform_stats` resolver. Computes up to 3 metrics:
  `anime_added_7d`, `episodes_added_7d`, `active_rooms_7d`. Eligible if ≥1
  metric is non-null. Redis key `spotlight:stats:<YYYY-MM-DD>`, TTL **24h**.
  Cross-service queries use existing shared GORM connection.

### Backend — Personal & live cards (Phase 3)

- [ ] **HSB-BE-20**: `personal_pick` resolver. Anon: calls `/api/anime/trending?limit=10`.
  Login: calls existing recs API. Random 3 from top-10 (or fewer per adaptive
  rule). Redis keys `spotlight:trending:<YYYY-MM-DD>` (anon) and
  `spotlight:personal:<user_id>:<YYYY-MM-DD>` (login), TTL **24h**.
- [ ] **HSB-BE-21**: `telegram_news` resolver. Reuses existing `parser/telegram`
  + cache `news:telegram` (TTL 30 min, untouched). Returns first 3 posts.
- [ ] **HSB-BE-22**: `now_watching` resolver. SQL:
  ```sql
  SELECT DISTINCT ON (wp.user_id)
         u.username, u.public_id, a.id, a.name, a.name_ru, a.poster_url,
         wp.episode_number, wp.updated_at
  FROM watch_progress wp
  JOIN users u  ON u.id = wp.user_id
  JOIN animes a ON a.id = wp.anime_id
  WHERE wp.updated_at > NOW() - INTERVAL '5 minutes'
  ORDER BY wp.user_id, wp.updated_at DESC
  LIMIT 5;
  ```
  Returns 1..3 sessions (adaptive rule). Redis key `spotlight:now_watching`,
  TTL **10s** (rate-limits DB pressure).
- [ ] **HSB-BE-23**: New `services/catalog/internal/service/spotlight/client/player_client.go` —
  thin HTTP client to `http://player:8083` for cards 8 and 9.
- [ ] **HSB-BE-24**: `not_time_yet` resolver (login only). Player API:
  `GET /internal/users/{user_id}/list?status=planned,postponed`. Filters to
  anime where `airing_started=true`, random pick of 1. Redis key
  `spotlight:not_time_yet:<user_id>`, TTL **30s**.
- [ ] **HSB-BE-25**: `continue_watching_new` resolver (login only). Player API:
  `GET /internal/users/{user_id}/list?status=watching`. Filters where
  `anime.episodes_aired > last_watch_progress.episode_number + 1` (NEW
  episode aired since last viewing). Most-recently-aired pick. Redis key
  `spotlight:continue_new:<user_id>`, TTL **30s**.
- [ ] **HSB-BE-26**: Player exposes the two internal endpoints (or extends an
  existing one) used by HSB-BE-24 and HSB-BE-25 — internal-only (NOT
  gateway-routed), JWT not required (called from inside the docker network
  with a service-to-service contract).

### Backend — Adaptive layout (Phase 3)

- [ ] **HSB-BE-30**: For multi-item resolvers (`personal_pick`, `latest_news`,
  `now_watching`, `telegram_news`): if `len(items) == 2`, the resolver
  randomly picks ONE of the two for the response. If `len(items) == 1`,
  returns the single item. If `len(items) >= 3`, returns the top 3
  (most-recent / highest-ranked).

### Frontend — Block & carousel (Phase 2)

- [ ] **HSB-FE-01**: New component `frontend/web/src/components/home/spotlight/HeroSpotlightBlock.vue`.
  Mounts on `Home.vue` immediately after `<SystemStatusBanner />`, above the
  3-column Ongoing/Top/Announced grid. Removes the existing `trendingRecs`
  block from `Home.vue` (lines ~39-132) in Phase 3.
- [ ] **HSB-FE-02**: Fetches `GET /api/home/spotlight` once on mount via the
  existing API client. On error or empty response → block hides itself
  (`v-if="cards.length > 0"`).
- [ ] **HSB-FE-03**: Auto-cycle interval **7s**. Pauses on `mouseenter` /
  `focusin` on the wrapper, resumes on `mouseleave` / `focusout`.
- [ ] **HSB-FE-04**: Manual navigation — left/right chevrons + dot indicators
  (one per eligible card). Keyboard: `ArrowLeft` / `ArrowRight` seek when the
  block has focus; `Tab` cycles through dots.
- [ ] **HSB-FE-05**: Initial slide chosen randomly (`randomInt(0, cards.length-1)`)
  on every page load.
- [ ] **HSB-FE-06**: Respects `prefers-reduced-motion: reduce` — disables
  auto-cycle (manual nav still works), removes slide-transition animation.
- [ ] **HSB-FE-07**: A11y — wrapper has `role="region"` + `aria-roledescription="carousel"`.
  Each slide has `aria-roledescription="slide"` and `aria-label` of form
  `"Slide N of M: <card title>"`. Slide container has `aria-live="polite"`.
- [ ] **HSB-FE-08**: Loading state — animated skeleton placeholder matching
  the block's final height (avoid layout shift).
- [ ] **HSB-FE-09**: Feature flag env `VITE_HERO_SPOTLIGHT_ENABLED` (default
  `true`). When `false`, block does not mount; legacy `trendingRecs` row
  stays visible during Phase 2 transitional release.

### Frontend — Card components (Phase 2 for 4 static, Phase 3 for 5 dynamic)

- [ ] **HSB-FE-20**: `AnimeOfDayCard.vue` — poster left + meta right
  (title, score, episodes, genres) + CTA buttons "Watch" / "Add to list".
  Mobile: poster top, meta below.
- [ ] **HSB-FE-21**: `LatestNewsCard.vue` — 3 changelog entries in row
  (desktop) or vertical stack (mobile). Links to full LastUpdates view.
- [ ] **HSB-FE-22**: `PlatformStatsCard.vue` — up to 3 metric chips with
  delta indicators. Layout: 3-in-row desktop, stack mobile.
- [ ] **HSB-FE-23**: `RandomTailCard.vue` — single poster + meta with
  "Random pick — discover something new" header.
- [ ] **HSB-FE-24**: `PersonalPickCard.vue` (Phase 3) — 1..3 posters
  with per-item reason chip. Desktop: 3 in row. Mobile single + "+ ещё 2 →"
  link to `/browse?sort=trending` (anon) or `/recs` (login).
- [ ] **HSB-FE-25**: `NowWatchingCard.vue` (Phase 3) — 1..3 user-session
  rows: `@username → anime title · ep N`. Green dot for "live".
  Avatar/poster optional. Layout: 3 stack desktop & mobile (text rows
  stack compactly).
- [ ] **HSB-FE-26**: `TelegramNewsCard.vue` (Phase 3) — 3 telegram post
  excerpts in row (desktop) or stack (mobile). Each links to source post
  on `t.me/<channel>/<msg_id>`.
- [ ] **HSB-FE-27**: `NotTimeYetCard.vue` (Phase 3) — single poster + meta
  with header "Не пришло ли время?" + sub-line referencing the user's
  list ("Из «Запланировано»").
- [ ] **HSB-FE-28**: `ContinueWatchingNewCard.vue` (Phase 3) — single poster
  + meta with header "Продолжить просмотр" + "Новая серия ep N!" badge.

### Frontend — i18n (Phase 2)

- [ ] **HSB-FE-40**: All card-specific strings live under `spotlight.*` in
  `frontend/web/src/locales/en.json` and `frontend/web/src/locales/ru.json`.
  Each card has at minimum: `.title`, `.cta` (where applicable), and any
  per-card sub-strings.

### Migration (Phase 3)

- [ ] **HSB-MIG-01**: Phase 3 removes the `trendingRecs` markup + script state
  from `Home.vue` (lines ~39-132 in the current file, plus
  `trendingRecs` / `trendingProgress` / `trendingLoading` / `rowLabelKey` /
  `reasonI18nKey` / `onRecClick`). The `/api/anime/recommended` backend
  endpoint stays — it now powers `personal_pick` inside the aggregator.
- [ ] **HSB-MIG-02**: After Phase 3 ships, drop the dual-flag setup. The
  feature flags `SPOTLIGHT_ENABLED` and `VITE_HERO_SPOTLIGHT_ENABLED` remain
  available as kill switches for one release, then are removed in Phase 4
  (if scheduled) or in a follow-up cleanup commit.

### Non-functional

- [ ] **HSB-NF-01**: Endpoint p95 latency **≤ 400ms** for cached responses
  and **≤ 1500ms** for cold (cache-miss) responses. Measured via existing
  `http_request_duration_seconds` histogram with `path="/api/home/spotlight"`.
- [ ] **HSB-NF-02**: `now_watching` SQL query uses an index on
  `watch_progress(updated_at)` — verify the index exists; if not, Phase 3
  adds it via GORM `AutoMigrate`'s `index:idx_watch_progress_updated_at` tag.
- [ ] **HSB-NF-03**: All new Redis keys use the `spotlight:` prefix for
  observability and bulk-flush.
- [ ] **HSB-NF-04**: Privacy — `now_watching` exposes only `username` and
  `public_id` (already publicly visible on user profile pages). No private
  data leaks. If user demand emerges for an opt-out, Phase 4+ adds
  `users.show_in_now_watching boolean default true` and respects it in
  the WHERE clause.
- [ ] **HSB-NF-05**: Documentation — `CLAUDE.md` gets a new section under
  "Common Tasks" explaining how to add a 10th card type (new resolver +
  new Vue component + new i18n key set). One-screen reference.
