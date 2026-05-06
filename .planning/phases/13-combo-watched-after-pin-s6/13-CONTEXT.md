# Phase 13: Combo-Watched-After Pin (S6) - Context

**Gathered:** 2026-05-06
**Status:** Ready for planning
**Mode:** Auto-generated with locked decisions from design spec §3.2 + Phase 11/12 hand-off (autonomous mode)

<domain>
## Phase Boundary

Land S6 with local co-occurrence + Shikimori `/similar` cascade, the synchronous seed-update path inside `MarkEpisodeWatched`, and the "Because you finished X" pinned tile in the frontend row. After this phase ships, finishing an anime with score ≥ 7 surfaces an instant pin on the home page.

In scope:
- **S6 seed update (synchronous, REC-INFRA-03):** when `MarkEpisodeWatched` flips `anime_list.status = 'completed'` AND the user just bumped `anime_list.score >= 7` (or it was already ≥ 7), update `rec_user_signals.{s6_seed_anime_id, s6_seed_completed_at, s6_seed_score}` in the same transaction. Latency budget: < 5 ms additional p95 overhead. Per spec §5: "synchronous on the request path because the pin is a flagship UX moment".
- **Co-occurrence materialization:** populate `rec_completion_co_occurrence` (Phase 9 schema) — nightly cron via the existing `PopulationOrchestrator` pattern. The query: for each `(seed, candidate)` pair where two users completed both with score ≥ 7, count distinct users. **Refresh cadence:** nightly (~24h) — co-occurrence changes slowly and the query is heavy.
- **S6 cascade resolver** (`services/player/internal/service/recs/signals/s6_combo_pin.go`): given a user's `s6_seed_anime_id`, return the top pin candidate.
  1. **Local co-occurrence:** query `rec_completion_co_occurrence` for the seed; sort by `co_count DESC`; filter through `S11.CandidatePoolForUser` (excludes user's completed/dropped/hidden); take top 1.
  2. **Shikimori fallback:** if local pool returns < 5 candidates after S11 filter, hit Shikimori `/api/animes/:shikimori_id/similar` REST endpoint. Parse the response, map Shikimori IDs → local anime IDs (via `animes.shikimori_id` lookup), filter through S11, take top 1.
  3. **Score-7 fallback:** if both local AND Shikimori cascades return zero candidates, retry the local query with `co_count` from score ≥ 5 completions (not score ≥ 7). NEVER fall to score > 0 (spec §3.2: "could surface 'more like the thing they hated'").
  4. **Pin window:** S6 only fires when `s6_seed_completed_at >= now() - INTERVAL '7 days'`. Older seeds → no pin (the row degrades to plain ensemble ranking).
- **Handler integration:** in `services/player/internal/handler/recs.go::GetRecs` logged-in branch, AFTER computing the ensemble top-50, check the user's `rec_user_signals.s6_seed_anime_id`. If non-NULL and within 7-day window, call S6 cascade. If S6 returns a pin candidate, prepend it as `recs[0]` with `pinned: true` and `pin_reason: "Because you finished {seed_name}"` (the seed anime's display name, EN + RU based on user locale or response default).
- **Cache invalidation:** the synchronous seed update must call `cache.Del(ctx, "recs:user:{user_id}:topN")` so the next `GET /api/users/recs` rebuilds with the fresh pin.
- **Frontend pinned tile:** `frontend/web/src/views/Home.vue` (or the `useRecs` composable / a new `RecCard.vue` wrapper) checks `rec.pinned === true` and renders a visually distinct tile — design choice between a colored border, a "Recommended for you" badge, or a special label. Implementer picks one consistent treatment (per CONTEXT.md decision §B6 below).
- **i18n:** `recs.pinReason` template `"Because you finished {name}"` (EN) / `"Потому что вы посмотрели {name}"` (RU). JA mirrors EN per project convention. Plus `recs.pinBadge` for the visual badge label if used (e.g., "PINNED" or "RECOMMENDED").
- **Spec ref:** `docs/superpowers/specs/2026-05-03-rec-engine-design.md` §3.2 is the binding contract. §13 locked decisions: `score >= 7` threshold, fallback `score >= 5`, pin window 7 days, Variant B (pinned tile, not weight-shift).

Out of scope (later phases):
- Admin debug page surfacing `rec_completion_co_occurrence` rows + cascade source (local vs Shikimori) → Phase 14
- `rec_click` / `rec_watched` event tagging → Phase 14
- Variant A (weight-shift) → v2.1 (deferred per spec §13)
- Co-occurrence weighting by user-similarity → v3.0 (current implementation treats all users equally)
- Pin-CTR tracking → Phase 14 telemetry

</domain>

<decisions>
## Implementation Decisions

### Synchronous Seed Update (Decision §B1)

- **Trigger:** inside `services/player/internal/service/list.go::MarkEpisodeWatched`, AFTER `s.listRepo.UpdateStatus` (or wherever the status flip to `completed` happens) AND after `s.prefRepo.CreateWatchHistory` succeeds. This places the seed update inside the same DB connection that's already open for MarkEpisodeWatched.
- **Qualifying condition:** `anime_list.status == 'completed'` AND `anime_list.score >= 7`. Both conditions checked from the freshly-updated row state.
- **Update fields:** `s6_seed_anime_id = animeID`, `s6_seed_completed_at = anime_list.completed_at`, `s6_seed_score = anime_list.score`. Plus `last_computed = now()` so the rec_user_signals row stays warm.
- **Repo method:** add `RecsRepository.UpdateS6Seed(ctx, userID, animeID, completedAt, score)` — a narrow UPDATE that touches only the four S6 columns + `last_computed`. Avoids the full-row UPSERT path used by `UpsertUserSignals` (which would clobber `s1_vector`/`s5_affinity` on race).
- **Latency budget:** < 5 ms p95 added to `MarkEpisodeWatched`. UPDATE on a single PK row with 4 columns → expected ~1-2 ms. Verified by Task 5 production timing comparison.
- **Cache bust:** AFTER the UPDATE succeeds, `cache.Del(ctx, "recs:user:{user_id}:topN")` (fire-and-forget goroutine — Phase 11 pattern). Failure to bust cache is logged but non-fatal (the cache TTL of 6h limits staleness).
- **Idempotency:** if the user re-watches an already-completed anime with score ≥ 7, the seed updates to the new `completed_at` (refreshes the 7-day window). If they downgrade their score below 7, the seed stays — the pin will simply expire after 7 days.

### S6 Module Layout (Decision §B2)

- **NOT a SignalModule** — S6 doesn't fit the `Score(ctx, userID, candidates)` shape. It's a separate "pin resolver" that runs after the ensemble ranks the row.
- **File:** `services/player/internal/service/recs/signals/s6_combo_pin.go` + `s6_combo_pin_test.go`. Lives in the `signals/` package for proximity to S1-S5 and S11 even though it isn't a SignalModule.
- **Public surface:**
  ```go
  type S6ComboPin struct {
      db        *gorm.DB
      shikimori shikimoriSimilarClient  // narrow interface for the /similar endpoint
      log       *logger.Logger
  }
  func NewS6ComboPin(db *gorm.DB, shikimori shikimoriSimilarClient, log *logger.Logger) *S6ComboPin
  func (s *S6ComboPin) Resolve(ctx context.Context, userID string, candidatePool []recs.AnimeID) (*PinCandidate, error)
  
  type PinCandidate struct {
      AnimeID    recs.AnimeID
      SeedName   string  // for "Because you finished {SeedName}"
      Source     string  // "local" | "shikimori_similar" — used by Phase 14 admin debug
  }
  ```
- **Resolve returns nil PinCandidate (not error) when:** no qualifying seed, seed older than 7 days, both cascades empty after both score thresholds. Handler treats nil as "no pin, just serve the ensemble row".
- **shikimoriSimilarClient interface:** narrow interface that the handler injects with the catalog service's Shikimori client (or a thin proxy that calls catalog HTTP). Tests inject a fake that returns canned similar lists.

### Shikimori `/similar` Client (Decision §B3)

- **New method:** `services/catalog/internal/parser/shikimori/client.go::GetSimilarAnime(ctx, shikimoriID) ([]SimilarAnime, error)`. Mirrors the existing `GetRelatedAnime` shape (REST GET, rate-limited, structured logging).
- **Endpoint:** `GET https://shikimori.io/api/animes/:id/similar` (REST, NOT GraphQL — Shikimori only exposes /similar via REST).
- **Response shape:** array of full anime objects with `id`, `name`, `russian`, `score`, `episodes`, `image.original`. Same shape as `/related` so we can reuse a similar parsing path.
- **Cross-service call:** the player service calls the catalog service via HTTP for `/similar` (NOT direct Shikimori — Shikimori rate budget is owned by catalog). New endpoint: `GET /api/anime/:id/similar` exposed on catalog. Internal call from player → gateway → catalog path.
- **Caching:** catalog caches `/similar` responses for 24h (`cache.KeySimilarAnime(shikimoriID)`). The cascade is bounded by Shikimori's rate limit on cache miss, but cache hits are sub-ms.

### Co-occurrence Cron (Decision §B4)

- **Source query:** `INSERT INTO rec_completion_co_occurrence (seed_anime_id, candidate_anime_id, co_count, last_computed) SELECT a.anime_id, b.anime_id, COUNT(DISTINCT a.user_id), now() FROM anime_list a JOIN anime_list b ON a.user_id = b.user_id AND a.anime_id != b.anime_id WHERE a.status = 'completed' AND a.score >= 7 AND b.status = 'completed' AND b.score >= 7 GROUP BY a.anime_id, b.anime_id HAVING COUNT(DISTINCT a.user_id) >= 1 ON CONFLICT (seed_anime_id, candidate_anime_id) DO UPDATE SET co_count = EXCLUDED.co_count, last_computed = EXCLUDED.last_computed;`
- **Cadence:** nightly (24h ticker) — at current scale (~10 users, ~1952 completions) this is millisecond-scale; at 100k users it'd be minute-scale. Co-occurrence changes slowly; nightly is fine.
- **Where:** new `services/player/internal/service/recs/co_occurrence.go::CoOccurrenceOrchestrator` mirroring `PopulationOrchestrator`. Wired in `cmd/player-api/main.go` after the existing user/population orchestrators with `orchestrator.Start(ctx, 24 * time.Hour)`.
- **Failure handling:** identical contract to PopulationOrchestrator — log error, continue ticking, stale data continues serving.

### Cascade Threshold (Decision §B5)

- **Initial query:** seed pool from `co_count >= 1` (any pair of users completing both with score ≥ 7). Sort by `co_count DESC`, filter through `S11.CandidatePoolForUser`, take top 1. Spec §3.2 doesn't lock a `co_count >= 5` threshold — that's the "before Shikimori fallback fires" trigger, not the pool-membership floor.
- **Cascade trigger to Shikimori:** if local pool yields < 5 candidates AFTER the S11 filter (not raw count). This matches spec §3.2 wording: "When ≥ 5 candidates exist in `rec_completion_co_occurrence` for the seed (other users completed both with score ≥ 7), they are used; when fewer than 5 exist, Shikimori `/api/animes/:id/similar` is hit and its results are filtered through S11".
- **Score-7 → score-5 fallback:** if BOTH local AND Shikimori cascades return zero candidates after S11 filter, RE-RUN the local query with `JOIN ON ... a.score >= 5 AND b.score >= 5` instead of `>= 7`. **Never** fall to `score > 0` (spec §3.2: "could surface 'more like the thing they hated'").
- **Final no-match behavior:** if score-5 fallback also returns zero, S6 returns nil PinCandidate. Handler serves the row without a pin.

### Frontend Pin Treatment (Decision §B6)

- **Visual differentiation:** a colored left-border (existing `RowCard` component pattern from CLAUDE.md) PLUS a small badge in the top-right corner labeled `recs.pinBadge` (default "PINNED" / "ВЫБРАНО" — implementer's call, can be tuned later).
- **Pin reason copy:** rendered above the card as a single line in the row header, OR as the card's hover tooltip (NOT as an overlay on the poster — that hides the art).
- **Why both border + badge:** the pin is a flagship UX moment per design spec §3.2; one visual treatment is too subtle. Two cheap treatments together make the pin instantly recognizable without redesigning the card.
- **No animation, no glow, no special card size** — the pin is meant to feel earned/contextual, not flashy.
- **i18n keys (EN / RU / JA mirror EN):**
  - `recs.pinReason`: "Because you finished {name}" / "Потому что вы посмотрели {name}"
  - `recs.pinBadge`: "PINNED" / "ВЫБРАНО"

### API Contract Extension (Decision §B7)

- `recs[0]` in the response when S6 fires gets two new fields:
  ```json
  {
    "anime": {...},
    "final": null,           // S6 pin doesn't have an ensemble final score; null is the explicit signal
    "pinned": true,
    "pin_reason": "Because you finished Frieren",
    "pin_seed_anime_id": "...",     // the seed that triggered the pin (admin debug uses this)
    "pin_source": "local",          // or "shikimori_similar"
    "rank": 1
  }
  ```
- All other rec items keep the existing shape (Phase 11). The frontend tolerates `final === null` for the pinned row.

### Locked from spec §13 (do not relitigate)

- S6 score threshold: ≥ 7 with fallback to ≥ 5
- S6 pin window: 7 days
- S6 Variant in v1: B (pinned tile, NOT weight-shift)
- Synchronous seed update inside `MarkEpisodeWatched`
- Cascade order: local co-occurrence → Shikimori `/similar`

### Claude's Discretion

- Whether the co-occurrence query uses a single nightly cron or fires incrementally on every qualifying completion. Plan-phase decides — incremental is more accurate but adds work to MarkEpisodeWatched (concern for the < 5 ms budget); nightly is simpler.
- Whether `RecsRepository.UpdateS6Seed` returns the freshly-updated row or just an error. Mirroring `UpdateStatus` and other simple GORM update wrappers in the player repo: error-only is the cleaner default.
- Whether the catalog `/api/anime/:id/similar` endpoint uses a public auth (anonymous OK) or requires the logged-in user's JWT. Default: anonymous OK (it's data, not user-specific).
- Whether the Vue pin-card treatment is a new `PinnedCard.vue` component or an inline conditional in `AnimeCard.vue` / `AnimeCardNew.vue`. Plan-phase chooses based on inspection of the existing card variants.

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- `services/player/internal/domain/recs.go:24-26` — `S6SeedAnimeID *string`, `S6SeedCompletedAt *time.Time`, `S6SeedScore *int` already defined on `RecUserSignals` (Phase 9). No schema work needed.
- `services/player/internal/domain/recs.go:50` — `RecCompletionCoOccurrence` struct + table already defined (Phase 9). No schema work needed.
- `services/player/internal/repo/recs.go::UpsertUserSignals` already includes `s6_seed_*` in `DoUpdates`. Phase 13 just needs a narrower `UpdateS6Seed` method to avoid the full-row UPSERT.
- `services/player/internal/service/list.go::MarkEpisodeWatched` (line 186) — the synchronous seed update lands here, after the existing `CreateWatchHistory` call (line 262 from Phase 11 trigger).
- `services/player/internal/service/recs/population.go` — `PopulationOrchestrator` is the architectural mirror for the new `CoOccurrenceOrchestrator`.
- `services/player/internal/handler/recs.go::GetRecs` logged-in branch (post-Phase-12 commit `cb3249b`) — Phase 13 prepends pin to `recs[0]`.
- `services/catalog/internal/parser/shikimori/client.go::GetRelatedAnime` (line 647) — closest analog for the new `GetSimilarAnime`. Same REST shape, same rate-limiter pattern.
- `services/catalog/internal/service/catalog.go::GetRelatedAnime` (line 293) — service-layer pattern with cache + Shikimori fallback. Plan can mirror this.
- `frontend/web/src/views/Home.vue` — already renders the "Up Next for you" row; adding pin treatment is a card-level conditional, not a row-level rewrite.
- `libs/cache.KeyRelatedAnime` — existing key generator for catalog cache; Phase 13 adds `KeySimilarAnime`.

### Established Patterns
- **Synchronous DB updates inside hot paths** — Phase 11's `MarkEpisodeWatched` adds `s.userOrchestrator.TriggerForUser(ctx, userID)` as fire-and-forget. Phase 13's seed update is DIFFERENT — it's a synchronous DB write because the user expects the pin to show up immediately on next refresh.
- **Cron orchestrators** — `PopulationOrchestrator` (Phase 10) and `UserOrchestrator` (Phase 11) are the patterns. New `CoOccurrenceOrchestrator` mirrors them.
- **Cross-service HTTP calls** — player → gateway → catalog for `/similar` is the existing path for any catalog-owned data. Phase 11 doesn't have prior art for this (player's recs only read its own DB tables and Redis); Phase 13 introduces the first player→catalog HTTP call. Keep the catalog handler thin (delegate to existing service layer).
- **Catalog service handlers** — pattern for `/api/anime/:id/X` endpoints in `services/catalog/internal/handler/anime.go` (or wherever the existing `/related` endpoint lives — verify in plan-phase).

### Integration Points
- **MarkEpisodeWatched edit:** add ~10 lines after the existing CreateWatchHistory + TriggerForUser block (Phase 11) for the seed-update + cache-bust path.
- **NewListService constructor:** add `recsRepo *repo.RecsRepository` field. Wire in `cmd/player-api/main.go`. Phase 11 already injected `userOrchestrator`; this is the same pattern.
- **NewRecsHandler constructor:** add `s6 *signals.S6ComboPin` field. Wire similarly.
- **CoOccurrenceOrchestrator:** wire `Start(ctx, 24 * time.Hour)` in `cmd/player-api/main.go` after the existing two orchestrator Start calls.
- **Catalog new endpoint:** `GET /api/anime/:id/similar` in `services/catalog/internal/transport/router.go`. Gateway route `/api/anime/*` already proxies to catalog (per CLAUDE.md gateway routing).
- **No new env vars.**
- **No new dependencies.**

</code_context>

<specifics>
## Specific Ideas

- **Spec ref:** Design spec §3.2 is the binding cascade contract. §5 mandates the synchronous seed update inside `MarkEpisodeWatched`. §13 locks score thresholds, pin window, and Variant B.
- **Latency monitoring:** Phase 14 adds Prometheus `rec_signal_ctr` — Phase 13 does NOT add per-route latency histograms (those are Phase 14). For the Phase 13 verification, do a one-off latency check via `time curl -X POST .../api/users/watchlist/{anime_id}/episode` before/after the seed update lands — must be within 5 ms variance.
- **Cascade test fixtures:** seed `ui_audit_bot` with two completed-with-score-≥7 anime, plus a third user with overlapping completions, to exercise the local co-occurrence path. Then null out the local pool to force the Shikimori fallback. Both paths should produce a non-nil PinCandidate in tests.
- **Pin reason locale:** the seed name is rendered from `animes.name_ru` or `animes.name` based on the user's preferred locale (TBD — Phase 11 didn't surface user locale in the recs API; if it's still not available, default to `name` (EN/JP) for both EN and RU UIs and let the user understand from context).
- **Production verification at Task 5:**
  1. Seed `ui_audit_bot` with a fresh score-≥7 completion via `make ui_audit_bot:add-completion` (or direct DB UPDATE for the test).
  2. Hit `GET /api/users/recs` with the API key — first item should have `pinned: true` and `pin_reason: "Because you finished {seed name}"`.
  3. Verify `rec_user_signals.s6_seed_*` columns are populated for ui_audit_bot.
  4. Verify the pin disappears 7 days later (manual: SET s6_seed_completed_at to now() - 8 days, refresh, pin should be gone).
- **Per CLAUDE.md:** all new copy goes to BOTH EN and RU; JA mirrors EN. Two new keys: `recs.pinReason` (template with `{name}` placeholder) and `recs.pinBadge`.
- **AniList tag backfill is still running in background** as of Phase 13 start — irrelevant to S6 (S6 doesn't use tags), but worth noting that the recs row's S5 tags dimension will improve over time.

</specifics>

<deferred>
## Deferred Ideas

- Admin debug page surfacing rec_completion_co_occurrence rows + cascade source → Phase 14
- Pin CTR tracking via `rec_click` / `rec_watched` events → Phase 14
- Variant A (weight-shift) replacing pin tile → v2.1 (deferred per spec §13 pending pin CTR data)
- Co-occurrence weighting by user-similarity (not just count) → v3.0
- Co-occurrence storage budget concerns → none until ~100k users (per spec §4.3)
- Pin history (show user "previously pinned" anime) → v2.1 polish
- Cross-language pin prevention (don't pin RU-dub for a JP-sub user) → covered by S11.CandidatePoolForUser already (excludes user's preference-mismatched candidates via the existing v1.0 preference resolver, indirectly)
- Pinning multiple seeds (user has 3 score-7 completions in last 7 days, each pinned) → v2.1; v1 picks the most-recent qualifying completion only

</deferred>
