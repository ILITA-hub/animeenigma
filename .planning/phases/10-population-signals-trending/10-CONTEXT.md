# Phase 10: Population Signals, Filter, Trending Row - Context

**Gathered:** 2026-05-06
**Status:** Ready for planning
**Mode:** Auto-generated with locked decisions from design spec §13 (autonomous mode)

<domain>
## Phase Boundary

Land the three stateless / population-wide signals (S3 trending, S4 recency, S11 filter), the 60-minute precompute cron, the Redis 6h top-N cache, and the anonymous "Trending now" home row. After this phase ships, anonymous users on `/` see a working trending row backed by real population data.

In scope:
- S3 (population trending): rank by last-30-day `watch_history` start count
- S4 (recency boost): `status='ongoing'` OR aired in last 90 days
- S11 (filter): Phase 10 scope is `animes.hidden = true` exclusion only (the `anime_list.status ∈ {completed, dropped}` filter requires `user_id` and lands in Phase 11)
- 60-minute population precompute cron (in-process goroutine ticker — player service runs persistently)
- Redis cache key: `recs:public:trending:topN` (anonymous serves a single shared top-N — no per-anon-id key needed; that's deferred to v2.1 anonymous personalization)
- HTTP endpoint: `GET /api/users/recs` — returns 20 anime; auth state determines whether logged-in personalization is active (anonymous in this phase, full ensemble in Phase 11)
- Frontend: "Trending now" row on `Home.vue` — reuses existing `Carousel` + `AnimeCard` pattern from `home.ongoing` / `home.topAnime` rows
- EN + RU locale strings for "Trending now" / "Тренды сейчас"

Out of scope (later phases):
- User-specific filtering (REC-UX-04 dropped/completed filter) — Phase 11
- Logged-in "Up Next for you" personalization (REC-UX-01, REC-UX-03) — Phase 11
- S1, S2, S5, S6 — Phases 11-13
- Admin debug page — Phase 14
- Per-signal CTR Prometheus metrics — Phase 14
- S6 pin tile — Phase 13

</domain>

<decisions>
## Implementation Decisions

### Backend Architecture

- **Package layout:** Concrete signals live under `services/player/internal/service/recs/signals/` (one file per signal: `s3_trending.go`, `s4_recency.go`, `s11_filter.go`). They implement the `SignalModule` interface from Phase 9. Tests alongside.
- **Population precompute:** New `services/player/internal/service/recs/population.go` with `PopulationOrchestrator` running on an in-process `time.Ticker` (60-minute cadence, started in `cmd/player-api/main.go`). On boot the ticker fires once immediately so cold starts have data within seconds. Cron failure is logged via `libs/logger`; service does NOT crash; stale signals continue serving until next successful run.
- **HTTP handler:** New `services/player/internal/handler/recs.go` exposing `GET /api/users/recs`. Reads from Redis cache; on miss, computes from `rec_population_signals` + ensemble + S11 filter, writes back to Redis with 6h TTL. JWT-optional (no JWT = anonymous flow).
- **Routing:** Add `GET /api/users/recs` to `services/player/internal/transport/router.go`. Gateway routing already includes `/api/users/*` → `player:8083` per CLAUDE.md, so no gateway change needed.
- **Redis key:** `recs:public:trending:topN` — single shared anonymous top-N. TTL 6h. Cache-buster timestamp at `recs:popsignal:lastcomputed` for cross-process cache coherence.

### Signal Implementation Details

- **S3 raw score:** `COUNT(DISTINCT user_id) FROM watch_history WHERE anime_id = ? AND watched_at >= NOW() - INTERVAL '30 days'` per anime. Population scope means we run a single GROUP BY query producing all rows in one pass. Stored as `s3_trending_score` in `rec_population_signals`.
- **S4 raw score:** Pure metadata function on `animes` table:
  - `status = 'ongoing'` → 1.0
  - `aired_at >= NOW() - INTERVAL '90 days'` → 0.7
  - else → 0.0
- **S11 (Phase 10 scope):** Only filters `animes.hidden = true`. Implementation: SQL `WHERE hidden = false` clause when fetching the candidate pool. Phase 11 adds the user-specific layer.
- **Ensemble weights for anonymous:** Only S3 + S4 are non-zero. Per spec §2.2 final formula: `0.20 × S3 + 0.10 × S4`. The other ensemble weights (S1, S2, S5) emit zero for population scope per the cold-start matrix in spec §3.3.

### API Contract

- `GET /api/users/recs`
  - Auth: optional. JWT present → Phase 11 logic (deferred); JWT absent → anonymous trending flow (this phase).
  - Response 200:
    ```json
    {
      "success": true,
      "data": {
        "recs": [
          { "anime": {...}, "final": 0.764, "pinned": false, "rank": 1 },
          ...
        ],
        "generated_at": "2026-05-06T12:00:00Z",
        "cache_hit": true,
        "total": 20,
        "row_label_key": "recs.trending"
      }
    }
    ```
  - `row_label_key` is the i18n key the frontend uses to look up the row title; "recs.trending" for anonymous, "recs.upNext" reserved for Phase 11.
  - `total` is the actual returned count (anonymous serves 20 always; Phase 11 may serve less if pool is thin).
  - `cache_hit` is informational; not part of the contract semantics.

### Frontend

- **Component:** Add a new section to `Home.vue` between existing "Ongoing" and "Top Anime" rows OR as a new prominent first row — implementer's call based on visual flow. The trending row uses the existing `Carousel` component + `AnimeCard`/`AnimeCardNew` per project pattern.
- **Composable:** Add `frontend/web/src/composables/useRecs.ts` — fetches from `/api/users/recs`, returns `{ recs, isLoading, error, generatedAt }`. Cache-friendly (server already caches 6h via Redis; client doesn't double-cache).
- **i18n:** Add `recs.trending` ("Trending now" EN / "Тренды сейчас" RU) and `recs.upNext` ("Up Next for you" EN / "Подобрано для вас" RU — reserved label for Phase 11) to all three locale files (en.json, ru.json, ja.json — JA can default to EN labels for now per existing project pattern). Also `recs.empty` for empty-state.
- **Loading state:** Reuse `AnimeCardSkeleton` for the row.
- **Empty state:** Hide the row if zero anime returned (no awkward "no recommendations yet" message — let the page flow continue).

### Locked from spec §13 (do not relitigate)

- 60-minute population cron cadence
- 6-hour Redis TTL
- 20-card frontend slice (server returns up to 50 in v2.0 design but anonymous trending only needs 20; Phase 11 will serve 50 to support future filtering on the client)
- No anonymous personalization in v2.0 — single shared anonymous top-N
- Backend extends existing `services/player/`

### Claude's Discretion

- Exact placement of trending row in `Home.vue` (above "Ongoing" vs between "Ongoing" and "Top Anime") — pick whichever reads better with the existing visual hierarchy
- Exact AnimeCard variant (`AnimeCard.vue` vs `AnimeCardNew.vue`) — match the existing card variant used by neighboring rows on Home.vue for consistency
- In-process ticker vs separate scheduler service — go with in-process (simpler, sufficient at current scale; v3.0 may extract to dedicated scheduler service)
- S4 weight breakpoints (`0.7` for last-90-day) — sensible default; may be tuned via Prometheus CTR data in v2.1

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- `services/player/internal/service/recs/{ensemble,normalize,signal,types,precompute}.go` — Phase 9 foundation. SignalModule interface, Ensemble, MinMaxNormalize, Orchestrator all ready.
- `services/player/internal/repo/recs.go` — Phase 9 RecsRepository with `ListPopulationSignals` and `UpsertPopulationSignal` already.
- `libs/cache` — Redis client wrapper (used elsewhere in player service)
- `libs/logger` — structured logging
- `frontend/web/src/components/carousel/Carousel.vue` — horizontal scrolling row pattern
- `frontend/web/src/components/anime/AnimeCard.vue` and `AnimeCardNew.vue` — card variants
- `frontend/web/src/components/anime/AnimeCardSkeleton.vue` — loading skeleton

### Established Patterns
- Player handlers register with the Chi router in `services/player/internal/transport/router.go` (existing handler patterns in `handler/list.go`, `handler/progress.go` etc.)
- Vue composables follow `useXxx` naming, return reactive state plus actions (see `useWatchPreferences.ts`)
- i18n keys use dot-paths under categories (`home.ongoing`, `nav.home`, etc.)
- HTTP envelope follows `httputil.OK { success, data, error, meta }`

### Integration Points
- Add cron starter call in `cmd/player-api/main.go` after the FK-constraints block from Phase 9
- Add route registration in `transport/router.go`
- No gateway changes needed (existing `/api/users/*` route covers it)
- No new env vars required
- No new dependencies (stdlib `time.Ticker` + existing GORM/Redis libs)

</code_context>

<specifics>
## Specific Ideas

- Design spec `docs/superpowers/specs/2026-05-03-rec-engine-design.md` §3.3 cold-start matrix is the authoritative table for which signals contribute in which user state. Anonymous = S3+S4 only, all others emit zero.
- The "in-process ticker fires once on boot" pattern means population signals are computed within ~1s of redeploy. No bootstrap dance needed.
- Per CLAUDE.md, all new copy goes to BOTH EN and RU locales. JA can mirror EN.
- Existing v1.0 telemetry shows ~10 active anime in watch_history per week (small dataset). The trending row will be thin until population grows. Implementer should ensure the row gracefully handles "fewer than 20 anime have any 30-day history" — supplement from `rec_population_signals.s4_recency_score` ranking when S3 pool is empty.

</specifics>

<deferred>
## Deferred Ideas

- Anonymous personalization via X-Anon-ID — v2.1 (`REC-V21-01`)
- S3 trending window tuning (30d default; 90d if too thin) — Phase 12 verification can revisit
- S4 boost weight tuning — v2.1 once Prometheus CTR data exists
- Gateway-level cache layer (currently relies on Redis only) — only if Phase 14 metrics show meaningful cache miss latency

</deferred>
