# Roadmap: AnimeEnigma

## Milestones

- ✅ **v1.0 Smart Watch Picker Overhaul** — Phases 1-8 (shipped 2026-05-03)
- 🚧 **v2.0 Recommendations Engine** — Phases 9-14 (in progress, started 2026-05-04)

## Phases

**Phase Numbering:**
- Integer phases (1-14): Planned milestone work
- Decimal phases (e.g., 10.1): Reserved for urgent insertions if scoping reveals one
- Phase numbering is continuous across milestones (v1.0 ended at Phase 8; v2.0 starts at Phase 9)

### v1.0 Smart Watch Picker Overhaul (Shipped 2026-05-03)

- [x] **Phase 1: Instrumentation Baseline** ✓ 2026-04-27
- [x] **Phase 2: Analytics Audit** ✓ 2026-04-28
- [x] **Phase 3: Single Source of Truth for "Watched"** ✓ 2026-04-28
- [x] **Phase 4: Resume State Machine in All Four Players** ✓ 2026-05-03
- [x] **Phase 5: Analytics Gap-Fill** ✓ 2026-05-03
- [x] **Phase 6: Tier 2 Inference Rewrite** ✓ 2026-05-03
- [x] **Phase 7: Advanced Settings, Anonymous UX, Cross-Device Freshness** ✓ 2026-05-03
- [x] **Phase 8: Recommendations Readiness Documentation** ✓ 2026-05-03

### v2.0 Recommendations Engine (In Progress)

- [x] **Phase 9: Recs Foundation — Interface, Ensemble, Normalizer, Schema** — Land the pluggable `SignalModule` interface, weighted-ensemble aggregator, per-pool min-max normalizer, and three new tables. No signals, no user-facing surface — silent infrastructure other phases bolt onto. ✅ shipped 2026-05-06
- [x] **Phase 10: Population Signals, Filter, Trending Row** — Land S3 (trending), S4 (recency), S11 (filter), 60-minute population cron, Redis 6h top-N cache, and the anonymous "Trending now" home row. ✅ shipped 2026-05-06
- [x] **Phase 11: User Signals & "Up Next for you" Row** — Land S1 (score-cluster) + S2 (item-item metadata), 6-hour user cron + debounced on-write trigger, and the logged-in "Up Next for you" home row. ✅ shipped 2026-05-06
- [ ] **Phase 12: TF-IDF Attribute Affinity (S5)** — Land S5 with seven weighted attribute dimensions and the Kodik episode-count fallback. Personalization quality jumps; weights are tuned via admin breakdown view.
- [ ] **Phase 13: Combo-Watched-After Pin (S6)** — Land S6 cascade (local co-occurrence → Shikimori `/similar`), synchronous seed update on `MarkEpisodeWatched`, and the "Because you finished X" pinned tile.
- [ ] **Phase 14: Admin Debug Page & Eval Pipeline** — Land the full `/admin/recs/:user_id` page (per-signal contribution, S5 term expand, S11 audit), force-recompute endpoint, frontend `rec_click` / `rec_watched` events, and Prometheus `rec_signal_ctr` metric.

## Phase Details

<details>
<summary>✅ v1.0 Smart Watch Picker Overhaul (Phases 1-8) — SHIPPED 2026-05-03</summary>

### Phase 1: Instrumentation Baseline ✓ 2026-04-27
**Goal**: Make the project's success metric (auto-pick override rate) observable in Grafana before any behavior changes ship.
**Requirements**: M-01, M-02
**Plans**: 7 plans (all complete)

### Phase 2: Analytics Audit ✓ 2026-04-28
**Goal**: Read-only inventory of `watch_history` / `watch_progress` / `anime_list` and prioritized gap analysis.
**Requirements**: C-01, C-02
**Plans**: 1 plan (complete)
**Deliverable**: `docs/analytics-audit-2026-04-28.md`

### Phase 3: Single Source of Truth for "Watched" ✓ 2026-04-28
**Goal**: `watch_progress.completed` becomes the single source of truth.
**Requirements**: A-01, A-02, D-02
**Plans**: 5 tasks (single plan, complete)

### Phase 4: Resume State Machine in All Four Players ✓ 2026-05-03
**Goal**: Pre-player episode selection follows the watching/finished/not-yet-aired state machine across all four players.
**Requirements**: A-03, A-04
**Plans**: 1 plan (complete)
**UI hint**: yes

### Phase 5: Analytics Gap-Fill ✓ 2026-05-03
**Goal**: Add highest-value low-risk columns/events from the audit; distinguish session-start vs session-resume.
**Requirements**: C-03
**Plans**: 1 plan (complete)

### Phase 6: Tier 2 Inference Rewrite ✓ 2026-05-03
**Goal**: Replace `COUNT(*) GROUP BY` with weighted, time-decayed, two-signal inference + min-confidence floor.
**Requirements**: B-01, B-02, B-03, B-04
**Plans**: 1 plan (complete)

### Phase 7: Advanced Settings, Anonymous UX, Cross-Device Freshness ✓ 2026-05-03
**Goal**: Surface new resolver behavior to power users; anonymous localStorage parity; bust 24h client cache on auth change.
**Requirements**: B-05, D-01, D-03
**Plans**: 1 plan (complete)
**UI hint**: yes

### Phase 8: Recommendations Readiness Documentation ✓ 2026-05-03
**Goal**: Document additional capture needed for a future recs engine; no engine built here.
**Requirements**: C-04
**Plans**: 1 plan (complete)
**Deliverable**: `docs/recommendations-readiness-2026-05-03.md`

</details>

### v2.0 Recommendations Engine (Phases 9-14)

**Milestone Goal:** Ship a personalized "Up Next for you" home row plus an admin debug surface, built on a pluggable foundation that accepts future signals (S7 vectors, S9-implicit OP/ED, S10 staff) without rewrites. Anonymous users get a "Trending now" row. Each phase is independently shippable and adds a usable subset of the rec engine.

#### Phase 9: Recs Foundation — Interface, Ensemble, Normalizer, Schema
**Goal**: Land the architectural seam (`SignalModule` interface), weighted-ensemble aggregator, shared per-pool min-max normalizer, and the three persistence tables. Signals slot into this in subsequent phases without modifying the ensemble, normalizer, or API handler. Ships as silent infrastructure — no user-facing surface yet.
**Depends on**: Nothing (first phase of v2.0; v1.0 milestone is closed)
**Requirements**: REC-FOUND-01, REC-FOUND-02, REC-FOUND-03, REC-FOUND-04
**Success Criteria** (what must be TRUE):
  1. After `make redeploy-player`, the player service starts cleanly and `\dt rec_*` in psql shows three tables: `rec_user_signals`, `rec_population_signals`, `rec_completion_co_occurrence` (with their FKs to `users`/`animes` and indexes from spec §4.1)
  2. A throwaway test signal that returns `{anime_id: 1.0}` for two candidates can be registered with weight 1.0; the ensemble returns those two anime sorted by final score with no NaN, no Inf, and per-pool-normalized values in `[0, 1]`
  3. The normalizer's property tests (empty pool / single element / all-equal / normal pool) pass under `go test ./services/player/internal/service/recs/...`
  4. Adding a second test signal does not require diff in `ensemble.go`, `normalize.go`, or any API handler beyond a one-line registry entry — verified by inspecting the diff during code review
**Plans:** 1 plan
- [x] 09-01-PLAN.md — Land SignalModule interface, weighted-ensemble aggregator, per-pool min-max normalizer, three Postgres tables, and precompute orchestrator stub (silent infrastructure for v2.0 signals) ✅ shipped 2026-05-06

#### Phase 10: Population Signals, Filter, Trending Row
**Goal**: Land the three stateless / population-wide signals (S3 trending, S4 recency, S11 filter), the 60-minute precompute cron, the Redis 6h top-N cache, and the anonymous "Trending now" home row. After this phase ships, anonymous users on `/` see a working trending row backed by real population data.
**Depends on**: Phase 9 (uses `SignalModule` interface, `rec_population_signals` table, normalizer)
**Requirements**: REC-SIG-01, REC-SIG-02, REC-SIG-07, REC-INFRA-01, REC-INFRA-04, REC-UX-02
**Success Criteria** (what must be TRUE):
  1. An anonymous (no-JWT) request to `GET /api/users/recs` returns at least 20 anime ranked by `0.20 × S3 + 0.10 × S4` (after per-pool normalization), with no completed/dropped/hidden anime in the list (S11 still applies for the `hidden=true` case at population scope)
  2. The home page (`Home.vue`) shows a "Trending now" row (label visible in EN and RU) for logged-out users, populated from the same endpoint, sliced to 20 cards
  3. The 60-minute population cron runs in the player service, writes fresh `s3_trending_score` and `s4_recency_score` rows for every anime in `animes` on each tick, and a Grafana log query confirms two consecutive successful runs in production
  4. A second identical request to `/api/users/recs` within 6 hours hits the Redis cache (verified via `recs:popsignal:lastcomputed` key existing and per-anon `recs:user:anon:{anon_id}:topN` returning `cache_hit: true` in the response envelope)
  5. Cron failure (e.g. forced DB error in test) is logged but does not crash the service; stale signals continue serving until the next successful run
**Plans:** 1 plan
- [x] 10-01-PLAN.md — Land S3 (trending), S4 (recency), S11 (hidden filter), 60-min population cron, Redis 6h top-N cache, GET /api/users/recs handler, useRecs composable, and Trending now row on Home.vue (EN + RU + JA i18n) ✅ shipped 2026-05-06
**UI hint**: yes

#### Phase 11: User Signals & "Up Next for you" Row
**Goal**: Land S1 (score-cluster k-NN) + S2 (item-item tags/genres/studios overlap), the 6-hour user-signal cron with debounced on-write trigger, and the logged-in "Up Next for you" home row. After this phase ships, logged-in users see a personalized row on the home page that reacts (within ~5 min) to new watch_history rows.
**Depends on**: Phase 10 (uses ensemble + cache infra; row layout already wired)
**Requirements**: REC-SIG-03, REC-SIG-04, REC-INFRA-02, REC-UX-01, REC-UX-04
**Success Criteria** (what must be TRUE):
  1. A logged-in `ui_audit_bot` request to `GET /api/users/recs` returns up to 20 anime ordered by the full `0.30·S1 + 0.20·S2 + 0.20·S3 + 0.10·S4` ensemble (S5 still 0 in this phase), with anime where `anime_list.status ∈ {completed, dropped}` excluded and admin-hidden anime excluded
  2. The home page shows an "Up Next for you" row (EN + RU copy) for logged-in users, distinct in label from the anonymous "Trending now" row, populated from the same endpoint
  3. A user with fewer than 3 scored anime gets S1 = 0 and S2 = 0 cleanly (no NaN, no errors); the ensemble degrades to trending-leaning and the row still renders 20 cards
  4. The 6-hour user-signal cron writes fresh `rec_user_signals.s1_vector` and updates `last_computed` for every user with at least one `watch_history` row; the on-write trigger after a `watch_history` insert re-runs the per-user precompute within 5 minutes (with a 5-minute-per-user debounce) and busts the Redis cache for that user
  5. The strict no-cross-language / no-cross-dub-sub boundary from VAL-02 is not violated by any rec — verified by a test that seeds a JP-sub-only user and asserts no RU-dub anime appears in the row (informational only; recs do not write preference state)
**Plans:** 1 plan
- [x] 11-01-PLAN.md — Land S1 (score-cluster k-NN), S2 (item-item genres metadata, request-time), S11.CandidatePoolForUser, libs/cache.SetNX, UserOrchestrator with 6h cron + debounced TriggerForUser, personalized GET /api/users/recs branch, and auth-aware refresh in useRecs (genres-only S2 per plan-phase schema inventory; tags/studios deferred to Phase 12) ✅ shipped 2026-05-06
**UI hint**: yes

#### Phase 12: TF-IDF Attribute Affinity (S5)
**Goal**: Land the heaviest single signal — S5 TF-IDF time-weighted attribute affinity across seven attribute dimensions (tags 0.30, studios 0.20, genres 0.15, demographic 0.10, source 0.10, type 0.10, producers 0.05) with the Kodik episode-count fallback. Inventory and (if needed) backfill missing `animes` columns identified in design §14.1. Quality of the "Up Next for you" row jumps measurably.
**Depends on**: Phase 11 (S5 plugs into the same ensemble + user cron; needs `rec_user_signals.s5_affinity`)
**Requirements**: REC-SIG-05
**Success Criteria** (what must be TRUE):
  1. A logged-in user with at least 5 watch_history rows sees S5 contribute non-zero, normalized scores for at least 80% of their candidate pool — verified by hitting the Phase-14 admin debug breakdown (the prerequisite read-only breakdown view ships incrementally during this phase, even though the polished page is a Phase 14 deliverable)
  2. For Kodik watch_history rows (`player='kodik'`), S5 uses integer episode count instead of `duration_watched`; for HiAnime/Consumet/AnimeLib rows it uses `max(duration_watched/60, 1)` minutes — verified by unit test on a mixed-player history fixture
  3. All seven attribute dimensions (tags, studios, genres, demographic, source, type, producers) contribute to S5 with their locked weights; missing attribute columns on `animes` are either backfilled from Shikimori during this phase OR explicitly skipped with a logged warning (decision documented in the plan)
  4. The full ensemble `0.30·S1 + 0.20·S2 + 0.20·S3 + 0.10·S4 + 0.20·S5` runs end-to-end with no NaN / Inf / negative values across the candidate pool (property test on production-like fixture)
  5. After redeploy, the existing logged-in row (Phase 11) still renders 20 cards but now with visibly different ordering for the seeded `ui_audit_bot` — the top-3 anime change relative to the Phase-11 baseline (regression sanity check that S5 is actually contributing)
**Plans:** 3 plans
- [x] 12-01-PLAN.md — Land Phase-12 schema additions on the catalog service: Kind / Rating / MaterialSource columns + Studios m2m + Tags m2m on Anime, extend Shikimori parser GraphQL queries on all 6 fetch paths, add new AniList GraphQL client with FetchTags + slugifyTagName, register new types with AutoMigrate. Producers absorbed into studios per Decision §A2. (Wave 1) ✅ shipped 2026-05-06 — 8 commits, schema verified live, Shikimori parser hydrates kind/rating/material_source/studios on every fetch (Frieren refresh: tv/pg_13/manga/Madhouse). 3857 rows queued for Wave 2 backfill.
- [ ] 12-02-PLAN.md — Land the one-shot backfill tool in the maintenance service: BackfillRunner with Shikimori half (kind/rating/material_source/studios) + AniList half (tags via ARM->AniList mapping), Makefile target make backfill-attributes, dry-run + canary + full-run gating. Idempotent re-runnable. (Wave 2 — depends on 12-01)
- [ ] 12-03-PLAN.md — Implement S5 TF-IDF SignalModule with Kodik fallback, register in handler ensemble at weight 0.20 (full ensemble: 0.30·S1 + 0.20·S2 + 0.20·S3 + 0.10·S4 + 0.20·S5), add to UserOrchestrator module list, ship Phase-12 changelog entry. Top-3 ordering shift verified vs Phase-11 baseline. (Wave 3 — depends on 12-02)


#### Phase 13: Combo-Watched-After Pin (S6)
**Goal**: Land S6 with local co-occurrence + Shikimori `/similar` cascade, the synchronous seed-update path inside `MarkEpisodeWatched`, and the "Because you finished X" pinned tile in the frontend row. After this phase ships, finishing an anime with score ≥ 7 surfaces an instant pin on the home page.
**Depends on**: Phase 12 (full ensemble exists; pin sits on top of it). Also leans on `MarkEpisodeWatched` / `anime_list.completed_at` / `anime_list.score` from v1.0.
**Requirements**: REC-SIG-06, REC-INFRA-03, REC-UX-03
**Success Criteria** (what must be TRUE):
  1. A logged-in user marks an anime complete with score ≥ 7 via the player's mark-watched flow; within seconds, refreshing the home page shows that anime's S6 pin at index 0 of the "Up Next for you" row, labeled "Because you finished {seed_name}" (EN + RU)
  2. The pin disappears 7 days after the qualifying completion, OR is replaced earlier by a newer qualifying completion (verified by manipulating `s6_seed_completed_at` on `ui_audit_bot` and asserting the pin updates)
  3. Local co-occurrence cascade: when ≥ 5 candidates exist in `rec_completion_co_occurrence` for the seed (other users completed both with score ≥ 7), they are used; when fewer than 5 exist, Shikimori `/api/animes/:id/similar` is hit and its results are filtered through S11 — verified by toggling the local pool size on `ui_audit_bot` fixtures
  4. If the score-7 cascade pool is empty for both local and Shikimori, the threshold drops to score ≥ 5 (never to score > 0); if still empty, the pin is silently omitted and the row falls back to the Phase-12 ensemble — verified by unit test
  5. The synchronous S6 seed update inside `MarkEpisodeWatched` writes `s6_seed_anime_id` / `s6_seed_completed_at` / `s6_seed_score` and invalidates `recs:user:{user_id}:topN` in Redis; the request returns within 5 ms additional overhead measured against the Phase-12 baseline
  6. The pinned tile is visually distinct from the rest of the row (border, label, or badge — designer's call) so users perceive it as a recommendation tied to a specific completion, not a generic rec
**Plans**: TBD
**UI hint**: yes

#### Phase 14: Admin Debug Page & Eval Pipeline
**Goal**: Land the full `/admin/recs/:user_id` debug page (per-signal contribution table, S5 TF-IDF term breakdown on row expand, S11 filter audit), the force-recompute endpoint, the frontend `rec_click` / `rec_watched` events tagged with the top contributor signal, and the Prometheus `rec_signal_ctr` per-signal CTR metric. After this phase ships, every ranking decision is auditable and v2.1 weight tuning has data.
**Depends on**: Phase 13 (all six signals must return data so the breakdown table has all columns populated)
**Requirements**: REC-ADMIN-01, REC-ADMIN-02, REC-EVAL-01, REC-EVAL-02
**Success Criteria** (what must be TRUE):
  1. An admin loading `/admin/recs/{ui_audit_bot_id}` sees a top-50 table with columns: rank, anime, poster, final, S1, S2, S3, S4, S5, S6, top contributor; expanding a row reveals the S5 TF-IDF term breakdown (e.g. studio "Madhouse", tf 0.41, idf 2.30) and (when applicable) S6 cascade source (local vs Shikimori); a separate "Filter audit" panel lists anime that S11 removed with reason (`status=completed`, `status=dropped`, `hidden=true`)
  2. Clicking "Force recompute" calls `POST /api/admin/recs/{user_id}/recompute`, which invalidates the user's Redis cache, triggers a synchronous precompute, and returns the new top-N + computation latency; the page re-renders within 2 seconds with fresh data and an updated `computed_at` timestamp
  3. A logged-in user clicking a card in the "Up Next for you" row emits a `rec_click` event tagged with the top contributor signal ID at click time; opening the player and crossing the 20-minute auto-mark threshold emits a `rec_watched` event with the same tag — verified by tailing the events table on production for the `ui_audit_bot` user
  4. `curl http://localhost:8083/metrics | grep rec_signal_ctr` returns a per-signal-id labeled gauge or summary, computed as `rec_watched_total / rec_click_total` over the last hour, scraped by Prometheus and visible in Grafana via a new "Rec engine" dashboard row
  5. EN + RU locale parity for any new admin-page copy that surfaces to non-admin contexts (rec row labels are already covered in earlier phases; this phase adds nothing user-facing other than telemetry)
**Plans**: TBD
**UI hint**: yes

## Progress

**Execution Order:**
v1.0 phases (1-8) executed in numeric order and shipped 2026-05-03. v2.0 phases execute in numeric order: 9 → 10 → 11 → 12 → 13 → 14. Each v2.0 phase is independently shippable: deploying after Phase 10 gives anonymous users a working trending row; deploying after Phase 11 gives logged-in users a personalized row; deploying after Phase 13 gives them the pin; Phase 14 closes the loop with auditability and eval data.

**Critical dependency notes:**
- Phase 9 is silent infrastructure; nothing renders for users until Phase 10.
- Phases 10 and 11 are independently deployable but Phase 11 strictly depends on Phase 10's row layout, cache, and ensemble runner.
- Phase 12 (S5) is the heaviest implementation lift but does not change the row's appearance — only its ranking.
- Phase 13 (S6) requires the synchronous-seed-update edit to `MarkEpisodeWatched`, a hot path from v1.0 — code review must verify the < 5 ms overhead constraint.
- Phase 14 is the only phase whose success criteria require all prior signals to be live (the breakdown table has six columns to populate).

| Phase | Milestone | Plans Complete | Status | Completed |
|-------|-----------|----------------|--------|-----------|
| 1. Instrumentation Baseline | v1.0 | 7/7 | Complete | 2026-04-27 |
| 2. Analytics Audit | v1.0 | 1/1 | Complete | 2026-04-28 |
| 3. Single Source of Truth for "Watched" | v1.0 | 5/5 | Complete | 2026-04-28 |
| 4. Resume State Machine in All Four Players | v1.0 | 1/1 | Complete | 2026-05-03 |
| 5. Analytics Gap-Fill | v1.0 | 1/1 | Complete | 2026-05-03 |
| 6. Tier 2 Inference Rewrite | v1.0 | 1/1 | Complete | 2026-05-03 |
| 7. Advanced Settings, Anonymous UX, Cross-Device Freshness | v1.0 | 1/1 | Complete | 2026-05-03 |
| 8. Recommendations Readiness Documentation | v1.0 | 1/1 | Complete | 2026-05-03 |
| 9. Recs Foundation | v2.0 | 0/1 | Planned | - |
| 10. Population Signals, Filter, Trending Row | v2.0 | 0/1 | Planned | - |
| 11. User Signals & "Up Next for you" Row | v2.0 | 0/1 | Planned | - |
| 12. TF-IDF Attribute Affinity (S5) | v2.0 | 0/TBD | Not started | - |
| 13. Combo-Watched-After Pin (S6) | v2.0 | 0/TBD | Not started | - |
| 14. Admin Debug Page & Eval Pipeline | v2.0 | 0/TBD | Not started | - |
