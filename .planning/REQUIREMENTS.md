# Requirements: v2.0 Recommendations Engine

**Defined:** 2026-05-04
**Core value:** A logged-in user opens the home page and sees a personalized "Up Next for you" row of anime they have not yet started — ranked by a transparent weighted-ensemble of signals. After completing an anime they enjoyed (score ≥ 7), a "Because you finished X" pin appears at the top of the row immediately. Anonymous users still see a useful "Trending now" row. Admins can audit every ranking decision per user.
**Design spec:** `docs/superpowers/specs/2026-05-03-rec-engine-design.md` — locked decisions in §13.

## v2 Requirements

### Foundation

- [ ] **REC-FOUND-01**: Pluggable `SignalModule` interface allows new signals to be added without modifying the ensemble, normalizer, or API handler — verified by adding the seven v2.0 signals (S1, S2, S3, S4, S5, S6, S11) without diff in `ensemble.go` or the API handler beyond a registry entry.
- [ ] **REC-FOUND-02**: Weighted-ensemble aggregator computes `final = Σ weight × per-pool-normalized(raw)` and returns sorted `[]Recommendation`. Empty pool returns nil. Signal errors surface (do not silently zero out).
- [ ] **REC-FOUND-03**: Per-pool min-max normalizer maps raw signal output to `[0, 1]` over the candidate pool with degenerate-pool guard (no NaN, no Inf, no negative). Property tests cover empty / single-element / all-equal / normal pools.
- [ ] **REC-FOUND-04**: Persistence tables `rec_user_signals`, `rec_population_signals`, `rec_completion_co_occurrence` are auto-migrated on player-service startup and survive container restarts.

### Personalized Surfaces

- [ ] **REC-UX-01**: Logged-in users see an "Up Next for you" row on the home page (`Home.vue`) with up to 20 personalized recommendations, fetched from `GET /api/users/recs`. EN + RU copy.
- [ ] **REC-UX-02**: Anonymous users see a "Trending now" row using population signals only (S3 + S4 ensemble). Same endpoint serves both — auth state determines which row label is shown.
- [x] **REC-UX-03**: After a user completes an anime with score ≥ 7, a "Because you finished X" pin appears at the top of their "Up Next for you" row within seconds (synchronous S6 seed update during `MarkEpisodeWatched`, Redis cache invalidated). Pin remains for 7 days or until a newer qualifying completion. ✅ shipped 2026-05-06 (Phase 13)
- [ ] **REC-UX-04**: Recommendations exclude anime where the user's `anime_list.status ∈ {completed, dropped}` or where `animes.hidden = true`.

### Signal Library

- [ ] **REC-SIG-01**: S3 (population trending) ranks anime by last-30-day `watch_history` start count. Stateless per-user; runs against the population pool. Carries cold-start.
- [ ] **REC-SIG-02**: S4 (recency) boosts anime that are currently airing (`status='ongoing'`) or aired in the last 90 days. Stateless; pure metadata function.
- [ ] **REC-SIG-03**: S1 (score-cluster) predicts a user's score for unwatched anime via k-NN over their `anime_list.score` history. Returns 0 when user has < 3 scored anime (cold-start safe).
- [ ] **REC-SIG-04**: S2 (item-item metadata) ranks candidates by similarity to the user's top-scored anime over tags + genres + studios. Returns 0 for users with no scored anime.
- [ ] **REC-SIG-05**: S5 (TF-IDF attribute affinity) ranks candidates by time-weighted attribute overlap. Per-attribute weights: tags 0.30, studios 0.20, genres 0.15, demographic 0.10, source 0.10, type 0.10, producers 0.05. Kodik rows fall back to integer episode count instead of `duration_watched`.
- [x] **REC-SIG-06**: S6 (combo-watched-after) cascades local co-occurrence (users who completed seed AND candidate with score ≥ 7) → Shikimori `/api/animes/:id/similar` when local pool has fewer than 5 candidates after S11. Local pool fallback threshold drops to ≥ 5 if score-7 pool is empty. ✅ shipped 2026-05-06 (Phase 13)
- [ ] **REC-SIG-07**: S11 (filter) excludes anime where the user's `anime_list.status ∈ {completed, dropped}` or `animes.hidden = true`. Applied after ensemble ranking, before pin.

### Refresh & Storage

- [ ] **REC-INFRA-01**: Population signals (S3, S4) are precomputed every 60 minutes via cron. Cron failure is logged but does not crash the service; stale signals serve until next successful run.
- [ ] **REC-INFRA-02**: User signals (S1, S5) are precomputed every 6 hours via cron. Optional debounced on-write trigger (5-min minimum interval per user) re-runs after `watch_history` insert.
- [x] **REC-INFRA-03**: S6 seed (`s6_seed_anime_id`, `s6_seed_completed_at`, `s6_seed_score`) updates synchronously inside `MarkEpisodeWatched` when a row qualifies (score ≥ 7, status = completed, completed_at recent). Synchronous write is < 5 ms. ✅ shipped 2026-05-06 (Phase 13) — production p95 = 48ms full-stack (5ms in-process bound verified by sqlite micro-benchmark)
- [ ] **REC-INFRA-04**: Top-N recommendations are cached in Redis with 6-hour TTL at key `recs:user:{user_id}:topN`. Cache is invalidated on user-signal recompute or S6 seed change.

### Admin & Eval

- [ ] **REC-ADMIN-01**: Admin debug page at `/admin/recs/:user_id` displays per-signal contribution table (final, weight × normalized for each signal, top contributor per row), TF-IDF term breakdown for S5 on row expand, and S11 filter-audit list (which anime were excluded and why).
- [ ] **REC-ADMIN-02**: Admin can force-recompute a user's recs via `POST /api/admin/recs/{user_id}/recompute`. Endpoint invalidates Redis cache, triggers immediate precompute, returns new top-N + computation latency.
- [ ] **REC-EVAL-01**: Frontend emits `rec_click` (impression-to-click) and `rec_watched` (click-to-actually-started) events tagged with the top contributor signal ID at click time.
- [ ] **REC-EVAL-02**: Prometheus exposes per-signal click-through-rate metric (`rec_signal_ctr` labeled by signal_id) for v2.1 weight tuning.

## Future Requirements (deferred — not in this milestone)

### v2.1 Candidates

- **REC-V21-01**: Anonymous user personalization using aggregated `X-Anon-ID` history (currently captured but not joined into signal pipeline).
- **REC-V21-02**: S6 Variant A (weight-shift) replacing the v2.0 pin tile, contingent on v2.0 pin CTR data.
- **REC-V21-03**: S9-implicit OP/ED affinity from skip-behavior — see `.planning/backlog/REC-S9-implicit-op-ed-affinity.md`. Requires Phase 5 instrumentation extensions (`intro_skip` / `outro_skip` events) that do not yet exist.
- **REC-V21-04**: Light diversification re-rank pass (penalize over-representation by studio/genre in top 20).

### v3.0 Candidates

- **REC-V3-01**: "Similar to this" sidebar on anime detail page (anime-conditioned, no user state).
- **REC-V3-02**: S7 content-vector similarity (synopsis embeddings via pgvector or sentence-transformers).
- **REC-V3-03**: S8 franchise / sequel proximity using Shikimori `related` data.
- **REC-V3-04**: S10 director / staff / VA affinity.
- **REC-V3-05**: Two-stage retrieval+ranker architecture migration (when user count approaches ~100k).

## Out of Scope

| Feature | Reason |
|---------|--------|
| "Similar to this" sidebar on anime detail page | Anime-conditioned recommendation; deferred to v3.0 — different UX surface, different math (no user state). |
| Anonymous user personalization (beyond trending) | X-Anon-ID exists but no aggregated history yet; deferred to v2.1. |
| Real-time streaming refresh (e.g., WebSocket-pushed rec updates) | Up to 6h staleness plus instant S6 pin is acceptable; streaming infrastructure cost is not justified. |
| S7 content-vector similarity (pgvector) | Plan-phase decision deferred until S5 ships — measure CTR first to decide if vectors add value. |
| S8 franchise/sequel proximity | Niche signal; bundled with S7 in v3.0. |
| S9-explicit OP/ED ratings | Replaced by S9-implicit (skip-behavior); see `.planning/backlog/REC-S9-implicit-op-ed-affinity.md`. |
| S9-implicit OP/ED affinity | Requires analytics instrumentation (`intro_skip` / `outro_skip` events) that does not yet exist; lands in v2.1. |
| S10 director / staff / VA affinity | Sparse data; v3.0. |
| S6 Variant A (weight-shift) | v2.0 ships Variant B (pinned tile) for transparency; weight-shift deferred to v2.1 once we measure pin CTR. |
| Diversification re-rank | Light pass deferred to v2.1; full diversification is a v3.0 problem. |
| Two-stage retrieval+ranker architecture | Ensemble math grows into it; not needed at <100k users. |

## Traceability

Phase mapping assigned by roadmapper 2026-05-06. Phase numbering continues from v1.0 (last shipped phase = 8); v2.0 spans Phases 9-14.

| Requirement | Phase | Status |
|-------------|-------|--------|
| REC-FOUND-01 | Phase 9 | Pending |
| REC-FOUND-02 | Phase 9 | Pending |
| REC-FOUND-03 | Phase 9 | Pending |
| REC-FOUND-04 | Phase 9 | Pending |
| REC-SIG-01 (S3 trending) | Phase 10 | Pending |
| REC-SIG-02 (S4 recency) | Phase 10 | Pending |
| REC-SIG-07 (S11 filter) | Phase 10 | Pending |
| REC-INFRA-01 (60-min population cron) | Phase 10 | Pending |
| REC-INFRA-04 (Redis 6h cache) | Phase 10 | Pending |
| REC-UX-02 (Trending now row) | Phase 10 | Pending |
| REC-SIG-03 (S1 score-cluster) | Phase 11 | Pending |
| REC-SIG-04 (S2 item-item metadata) | Phase 11 | Pending |
| REC-INFRA-02 (6h user cron + on-write) | Phase 11 | Pending |
| REC-UX-01 (Up Next for you row) | Phase 11 | Pending |
| REC-UX-04 (exclude completed/dropped/hidden) | Phase 11 | Pending |
| REC-SIG-05 (S5 TF-IDF affinity) | Phase 12 | Pending |
| REC-SIG-06 (S6 combo-watched-after) | Phase 13 | ✅ Complete (2026-05-06) |
| REC-INFRA-03 (synchronous S6 seed update) | Phase 13 | ✅ Complete (2026-05-06) |
| REC-UX-03 (Because you finished X pin) | Phase 13 | ✅ Complete (2026-05-06) |
| REC-ADMIN-01 (admin debug page) | Phase 14 | Pending |
| REC-ADMIN-02 (force-recompute endpoint) | Phase 14 | Pending |
| REC-EVAL-01 (rec_click / rec_watched events) | Phase 14 | Pending |
| REC-EVAL-02 (Prometheus rec_signal_ctr) | Phase 14 | Pending |

**Coverage:** 23/23 v2.0 requirements mapped. No orphans.
