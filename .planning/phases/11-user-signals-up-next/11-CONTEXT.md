# Phase 11: User Signals & "Up Next for you" Row - Context

**Gathered:** 2026-05-06
**Status:** Ready for planning
**Mode:** Auto-generated with locked decisions from design spec §13 + Phase 9/10 patterns (autonomous mode)

<domain>
## Phase Boundary

Land S1 (score-cluster k-NN) + S2 (item-item metadata overlap), the 6-hour user-signal cron with debounced on-write trigger, and the logged-in "Up Next for you" home row. After this phase ships, logged-in users see a personalized row on the home page that reacts (within ~5 min) to new `watch_history` rows.

In scope:
- S1 (score-cluster k-NN): predicts user's score for each unwatched candidate via k-NN over `anime_list.score` history. Returns 0 (cleanly, no NaN) when user has < 3 scored anime.
- S2 (item-item metadata overlap): ranks candidates by metadata similarity (Jaccard) to user's top-scored anime over the **available** attribute dimensions on `animes` today (genres are confirmed; tags / studios may need plan-phase inventory — see §Open verification). Returns 0 for users with no scored anime.
- 6-hour user-signal cron (`UserOrchestrator.Start` — in-process ticker, mirrors `PopulationOrchestrator` from Phase 10) running per-user `Precompute` for every user with at least one `watch_history` row. Stale cache for that user gets invalidated when the precompute completes.
- Debounced on-write trigger: a watch_history insert (from `ListService.MarkEpisodeWatched` in `services/player/internal/service/list.go:262`) fires `userOrchestrator.TriggerForUser(ctx, userID)`. The trigger checks Redis `recs:debounce:{user_id}` (TTL 300s); if present, skip; if absent, set it and spawn the precompute goroutine. This caps re-runs at one per 5 minutes per user across all replicas.
- Extended S11 user-specific filter (`S11Filter.CandidatePoolForUser(ctx, userID)`) — extends Phase 10's anonymous `CandidatePool` with `LEFT JOIN anime_list AND anime_list.status NOT IN ('completed', 'dropped')` per REC-UX-04.
- Personalized handler branch in `services/player/internal/handler/recs.go` — when a JWT is present (`authz.ClaimsFromContext`), serve from `recs:user:{user_id}:topN` (Redis 6h TTL) using full ensemble `0.30·S1 + 0.20·S2 + 0.20·S3 + 0.10·S4` (S5 still 0 in this phase). When no JWT, keep the Phase 10 anonymous flow unchanged.
- Frontend: "Up Next for you" row on `Home.vue` — same `Carousel` + `AnimeCard` shape as the trending row; distinct row label `recs.upNext` (already reserved in i18n by Phase 10). Logged-in users see "Up Next for you" replacing the trending label; the row is conditional on `auth.isAuthenticated`.
- Auth-change cache bust: when `useAuth` transitions logged-out → logged-in (or back), `useRecs` re-fetches without using stale anonymous data. (Phase 7 already established the 24h client cache bust; this phase reuses that pattern for the recs row.)
- VAL-02 informational verification (success criterion #5): a unit test seeds a JP-sub-only user, asserts no RU-dub anime appears in the row. Recommendations are read-only — they do NOT write the user's preference state. The test is informational confirmation, not a hard runtime gate.

Out of scope (later phases):
- S5 TF-IDF attribute affinity → Phase 12 (will also inventory + backfill missing `animes` columns: tags, source, demographic, type, studios, producers if not already present)
- S6 combo-watched-after pin → Phase 13 (synchronous seed update inside `MarkEpisodeWatched`)
- Admin debug page + force-recompute endpoint → Phase 14
- Per-signal CTR Prometheus metrics + `rec_click` / `rec_watched` events → Phase 14
- Anonymous personalization (X-Anon-ID) → v2.1 backlog

</domain>

<decisions>
## Implementation Decisions

### S1 — Score-Cluster k-NN

- **Method:** Pearson correlation similarity over user-anime score vectors. Pearson handles per-user score-bias robustly (one user's "7" is another user's "5") and is the standard k-NN choice for collaborative filtering on rating data.
- **k (neighbor count):** k=10. Standard default; tunable later if Phase 14 admin breakdown shows weak signal.
- **Minimum overlap to compute similarity:** users must share ≥ 2 rated anime to be neighbors (avoids "one shared anime, perfect correlation" noise).
- **Cold-start gate:** target user must have ≥ 3 scored anime in `anime_list` before S1 emits non-zero scores. Below threshold, `Score` returns an empty map (normalizer treats missing entries as zero — REC-SIG-03).
- **Output:** for each unwatched candidate `i`, `S1_raw(u, i) = weighted_avg(neighbor_scores)` where weights are Pearson similarity. Min-max normalized over the candidate pool.
- **Precompute artifact:** writes `rec_user_signals.s1_vector` as JSONB `{anime_id: predicted_score}` for the user's full candidate pool. Empty `{}` when below cold-start gate.

### S2 — Item-Item Metadata Overlap

- **Method:** Jaccard similarity over the union of attribute sets (cheaper than cosine; no vector storage; reads cleanly out of Postgres). Operates over whatever attribute dimensions are available on `animes` today.
- **Attribute selection (plan-phase inventory):**
  - Genres are confirmed present (m2m via `anime_genres` from `services/catalog/internal/domain/anime.go`).
  - Tags / studios are NOT confirmed on `animes` schema today. Plan-phase MUST inventory the schema before locking the S2 attribute set. Two acceptable outcomes:
    - (a) Genres-only S2 in Phase 11; defer richer attributes to Phase 12 alongside S5 backfill.
    - (b) Phase 11 also adds the tags / studios m2m tables + ingestion in plan-phase if the lift is small (< 1 day).
  - Either path satisfies REC-SIG-04: "ranks candidates by similarity to the user's top-scored anime over tags + genres + studios" — the requirement is an aspirational scope; ship what's available without faking absent attributes.
- **"User's top-scored anime" definition:** anime with `anime_list.score >= 7` (mirrors S6's qualifying-completion threshold and §13 locked decision); fall back to score >= 5 if pool is empty; if still empty, S2 emits zero (cold-start).
- **Per-candidate computation:** `S2_raw(u, i) = max_{seed ∈ user.top_scored} jaccard(seed.attrs, i.attrs)`. Using `max` (not average) keeps the "one strong match" signal — the most common item-item shape in real recs systems.
- **Precompute:** S2 raw scores are computed on-demand inside the user precompute step, written into a separate JSONB if storage cost matters, OR (preferred) recomputed at request-time from the candidate pool. Plan-phase decides — if `rec_user_signals.s2_vector` adds < 5 KB per user (likely true for ~3500 anime universe), persist it. Otherwise compute at request-time.

### S11 Extension — User-Specific Filter

- **New method:** `S11Filter.CandidatePoolForUser(ctx, userID string) ([]recs.AnimeID, error)`.
- **Implementation:** SQL — `SELECT a.id FROM animes a LEFT JOIN anime_list al ON al.anime_id = a.id AND al.user_id = ? WHERE a.hidden = false AND a.deleted_at IS NULL AND (al.status IS NULL OR al.status NOT IN ('completed', 'dropped'))`.
- **Anonymous (existing) `CandidatePool` stays untouched** — Phase 10 contract for anonymous flow remains stable.
- **Test:** seed `ui_audit_bot` with one `completed` and one `dropped` row; `CandidatePoolForUser` must omit both.

### User-Signal Cron (REC-INFRA-02)

- **Type:** New `UserOrchestrator` struct in `services/player/internal/service/recs/user_orchestrator.go`. Mirrors `PopulationOrchestrator` from Phase 10 in shape (Start, RunOnce per-user, in-process ticker). Cadence: 6 hours.
- **Per-tick scope:** every user with `EXISTS (SELECT 1 FROM watch_history WHERE user_id = u.id)`. The cron iterates this set; per-user precompute calls `s1.Precompute(ctx, userID)` and `s2.Precompute(ctx, userID)` (or recompute-at-request if S2 stays request-time per the decision above).
- **Boot tick:** like `PopulationOrchestrator`, fires once on boot so signals are warm within seconds of redeploy.
- **Cache invalidation on completion:** after a successful per-user precompute, `cache.Del(ctx, "recs:user:{user_id}:topN")` to force the next request to re-merge with the fresh signals.
- **Failure handling:** per-user errors are logged but do NOT halt the tick — the tick continues with the next user. Identical contract to `PopulationOrchestrator.RunOnce`.

### Debounced On-Write Trigger

- **Trigger site:** add a single line in `services/player/internal/service/list.go` immediately after the `s.prefRepo.CreateWatchHistory(ctx, history)` call (line 262). Pattern: `s.userOrchestrator.TriggerForUser(ctx, userID)` in a fire-and-forget goroutine so `MarkEpisodeWatched` latency is unaffected.
- **`TriggerForUser` implementation:**
  1. Redis `SET NX EX 300` on key `recs:debounce:{user_id}` — atomic, distributed-safe, 5-minute TTL.
  2. If SETNX returns false (key existed), debounce hit → log debug, skip.
  3. If SETNX returns true → spawn goroutine that calls `RunForUser(ctx, userID)` and `cache.Del(ctx, "recs:user:{user_id}:topN")` on success. Errors logged but not propagated to the trigger site.
- **Why Redis SETNX over in-memory map:** survives restarts, works across multi-replica deployments, simpler than coordinated in-memory state.
- **Why fire-and-forget over a queue:** queues add infra (Redis Streams / NATS / RabbitMQ) for a workload of ~tens of inserts per minute at peak. Stdlib goroutine + Redis lock is sufficient at current scale; can extract to a queue in v3.0 if instrumentation shows queue pressure.

### API Contract — Logged-In Branch

- `GET /api/users/recs` — same endpoint, same envelope shape, but auth-aware:
  - **JWT present** (`claims := authz.ClaimsFromContext(ctx); claims != nil`) → branch to `computeFreshForUser(ctx, claims.UserID)`:
    - Try `cache.Get(ctx, "recs:user:{user_id}:topN", &cached)` — hit returns `cache_hit: true`.
    - Miss: build candidate pool via `s11.CandidatePoolForUser(ctx, userID)`. Construct full ensemble `[{S1, 0.30}, {S2, 0.20}, {S3, 0.20}, {S4, 0.10}]` (S5 stays 0 — registered with weight 0 OR omitted entirely; plan-phase decides). `ensemble.Rank(ctx, userID, pool)` produces normalized scores. Slice top-50 server-side (frontend slices to 20 per §13 locked decision); cache 6h.
  - **No JWT** → existing Phase 10 anonymous flow, unchanged.
- **Response envelope additions:**
  - `row_label_key`: `"recs.upNext"` for logged-in flow, `"recs.trending"` for anonymous (already reserved by Phase 10).
- **Total cards returned:** server returns up to 50 for logged-in (per §13 locked); the Phase-10 anonymous path keeps returning 20. Plan-phase encodes the size as a per-flow constant.

### Frontend

- **Existing components:** `frontend/web/src/composables/useRecs.ts`, `frontend/web/src/views/Home.vue` already reference `row_label_key`. The composable just needs to honor the auth state (JWT presence drives the call; backend handles the actual personalization branch).
- **Auth-change cache bust:** in `useRecs.ts`, watch the auth store's user / token field; on transition (logged-in ↔ logged-out), call the existing `refresh()` action so the UI doesn't render anonymous data for a logged-in user (or vice versa).
- **i18n:** `recs.upNext` already added by Phase 10 (EN: "Up Next for you" / RU: "Подобрано для вас" / JA: defaults to EN). Confirm during code review that all three locale files have the key — if not, add. Also add `recs.empty` if not already present.
- **Empty-state:** if logged-in user's pool is empty (e.g., user has marked everything completed/dropped), hide the row — same pattern as Phase 10 trending row.
- **No new frontend layout changes:** the row slot in Home.vue is reusable; only the i18n key swap is visible to logged-in users.

### Locked from spec §13 (do not relitigate)

- Ensemble weights: `0.30·S1 + 0.20·S2 + 0.20·S3 + 0.10·S4 + 0.20·S5` (S5 emits 0 in Phase 11)
- Top-N returned to frontend: 50 for logged-in (frontend slices to 20)
- User-signal cron cadence: 6 hours
- Redis cache TTL: 6 hours
- Backend: extend `services/player/`
- New table additions: none — Phase 11 reuses `rec_user_signals` from Phase 9
- Cold-start: always show row; silently upgrade as data accumulates

### Claude's Discretion

- Exact normalization shape for S1 weighted-average — predicted score in `[score_min, score_max]` range pre-normalize, then min-max into `[0, 1]` over candidate pool (standard k-NN-then-normalize)
- Whether S2 persists into `rec_user_signals.s2_vector` (new JSONB column via GORM AutoMigrate) or is computed at request-time — plan-phase chooses based on per-user storage cost and cache hit rate
- Whether to register S5 with weight 0 or omit it from the Phase 11 ensemble registry — both work; omit is simpler, register-with-zero is closer to the §13 final formula
- File layout under `services/player/internal/service/recs/signals/`: `s1_score_cluster.go`, `s2_metadata.go` per spec §6; tests alongside
- File layout for the user orchestrator: `services/player/internal/service/recs/user_orchestrator.go` (parallel to `population.go`)
- The cron's "users with ≥ 1 watch_history row" set may be cached for the duration of a tick (avoids re-running the same SELECT N times); plan-phase chooses
- VAL-02 test fixture details (which JP-sub anime, which RU-dub anime to seed) — plan-phase chooses; pick from existing `ui_audit_bot` seed data if possible

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- `services/player/internal/service/recs/{ensemble,normalize,signal,types,precompute}.go` — Phase 9 foundation. `Ensemble.Rank` already supports any number of weighted signals via `[]WeightedSignal`. No changes needed.
- `services/player/internal/service/recs/population.go` — Phase 10 `PopulationOrchestrator` is the architectural mirror for the new `UserOrchestrator`. Same Start/RunOnce/in-process-ticker shape, just per-user instead of population-wide.
- `services/player/internal/service/recs/signals/s11_filter.go` — Phase 10 candidate pool. Add `CandidatePoolForUser` method here; keep the existing `CandidatePool` for the anonymous flow.
- `services/player/internal/handler/recs.go` — Phase 10 handler with TODO at line 122-124 ("Phase-11: branch on claims"). Add the personalized branch; reuse `hydrateAnime` and the cache-error helpers verbatim.
- `services/player/internal/repo/recs.go` — Phase 9 `RecsRepository` already has `GetUserSignals` / `UpsertUserSignals`. No new repo methods needed unless plan-phase persists S2.
- `services/player/internal/repo/preference.go:88` — `CreateWatchHistory` is the existing insert site. The on-write trigger fires from the SERVICE layer (`list.go:262`), not the repo, to avoid binding the repo to orchestrator types.
- `libs/cache` — Redis client with `Get`, `Set`, `Del`, `Exists` and the SETNX semantics (already used in Phase 10). Plan-phase confirms the SETNX method name; if absent, add a thin helper to libs/cache.
- `libs/authz` — `ClaimsFromContext` extracts JWT user claims; already used by Phase 10 handler at line 121.
- `frontend/web/src/composables/useRecs.ts` — fetches `/api/users/recs`; already exposes `recs / isLoading / error / generatedAt / rowLabelKey`. Auth-change watcher is the only addition.
- `frontend/web/src/views/Home.vue` — already has the row slot wired with `row_label_key` indirection. No layout change needed.

### Established Patterns
- **In-process ticker for cron** (Phase 10): `time.NewTicker(interval)` + boot-tick + per-iteration error logging without service crash. `UserOrchestrator` mirrors this verbatim.
- **Cache miss helper** (Phase 10 `recs.go:isCacheMiss`): keep using the same `errors.Is(..., ErrNotFound) || err.Error() == "cache: key not found"` shape.
- **Per-pool min-max normalization** (Phase 9 `normalize.go`): every signal goes through this; plan-phase confirms S1/S2 raw outputs feed `Ensemble.Rank` which already normalizes per-pool.
- **JSONB persistence on `rec_user_signals`**: Phase 9 already declares `S1Vector` and `S5Affinity` JSONB columns (`services/player/internal/domain/recs.go:22-23`). S1 writes via `UpsertUserSignals`. If S2 persists, plan-phase adds an `S2Vector` column via GORM AutoMigrate.
- **HTTP envelope** (`libs/httputil`): `httputil.OK(w, data)` wraps in `{success, data, error, meta}`. Phase 10 handler already follows this.
- **`SetIfNotExists` / `SETNX`** semantics in `libs/cache`: plan-phase confirms availability; if not, add a thin wrapper. Used for the debounce lock.

### Integration Points
- **Wire UserOrchestrator in `services/player/cmd/player-api/main.go`**: register S1, S2 modules, construct `NewUserOrchestrator(modules, db, cache, log)`, call `userOrchestrator.Start(ctx, 6 * time.Hour)` after the existing `populationOrchestrator.Start` line from Phase 10.
- **Wire trigger in `services/player/internal/service/list.go`**: add `userOrchestrator *recs.UserOrchestrator` field to `ListService`, inject via constructor, call `s.userOrchestrator.TriggerForUser(ctx, userID)` in a goroutine right after `CreateWatchHistory` succeeds (~line 263).
- **Update `services/player/internal/handler/recs.go`**: add personalized branch in `GetRecs` based on `claims != nil`. Keep all existing anonymous code paths.
- **No gateway changes** — `/api/users/recs` already routes to player:8083 per Phase 10.
- **No new env vars.**
- **No new dependencies** — stdlib `time.Ticker` + existing GORM/Redis/Pearson math (implement Pearson inline; ~20 lines of Go).

</code_context>

<specifics>
## Specific Ideas

- **Spec ref:** Design spec `docs/superpowers/specs/2026-05-03-rec-engine-design.md` §3.3 cold-start matrix is the authoritative behavior table for sparse-data users. "Logged in, sparse data (1–3 ratings, 0 watch_history)" → S1/S2 weak, S3/S4 active — the ensemble naturally degrades to trending-leaning. Success criterion #3 is satisfied by emitting zero (not NaN) from S1/S2 and letting Phase 9's normalizer handle the all-zero pool case via `MinMaxNormalize`'s degenerate-pool guard.
- **Pearson over cosine:** spec §3 names "k-NN" without locking the metric. Pearson is the canonical choice for rating data (handles user score-bias); cosine is fine for TF-IDF-style sparse vectors but worse for fixed-scale 1-10 ratings. Codifying Pearson here so plan-phase doesn't relitigate.
- **Existing v1.0 telemetry:** ~10 active users, ~10 watch_history rows/week. The 6-hour cron + 5-minute debounce trigger covers this volume comfortably; per-user precompute latency is essentially `O(candidate_pool × neighbor_count)` which is ~3500 × 10 = 35k operations per user — millisecond-range in Go.
- **VAL-02 informational test:** the success criterion #5 phrase "informational only; recs do not write preference state" is the correct framing. Recommendations are purely read-only — they never mutate `user_preference`, never call `SetForce` or `SetPreference`. The test confirms the row's contents respect language boundaries by virtue of the candidate pool not containing cross-language anime; if the test fails, it's a candidate-pool bug (not a signal bug).
- **Phase 12 dependency hand-off:** the schema gap on `animes` (tags / studios / etc.) becomes a hard requirement for Phase 12 (S5 needs all seven attribute dimensions). Phase 11's S2 inventory work is the natural place to surface this. If plan-phase 11 chooses path (a) "genres-only", the S5 inventory + backfill in Phase 12 must explicitly add tags + studios first; if it chooses path (b) "extend in this phase", Phase 12 only needs to add the remaining dimensions (demographic, source, type, producers).
- **Per CLAUDE.md:** all new copy goes to BOTH EN and RU; JA can mirror EN. The `recs.upNext` key is already present per Phase 10 prep — confirm in plan-phase that it actually landed in all three locale files.

</specifics>

<deferred>
## Deferred Ideas

- S5 TF-IDF affinity → Phase 12
- S6 combo-watched-after pin → Phase 13
- Admin debug page (per-signal contribution table, S1 nearest-neighbor expand, force-recompute) → Phase 14
- Per-signal CTR Prometheus metrics + `rec_click` / `rec_watched` events → Phase 14
- Anonymous personalization via X-Anon-ID → v2.1 (`REC-V21-01` backlog)
- Diversification re-rank (S12) → v2.1
- Adult-content filter as part of S11 → v2.1+ (per spec §14.6 open question; out of scope until a per-user setting exists)

</deferred>
