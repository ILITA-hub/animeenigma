# Recommendations Engine — v1 Design Spec

**Date:** 2026-05-03
**Author:** Brainstormed via `superpowers:brainstorming` (Claude Opus 4.7) with project owner
**Status:** Design approved; awaits user review of this spec; implementation plan TBD
**Target:** First milestone *after* the current Smart Watch Picker Overhaul (v1.0) closes
**Doubles as:** Input for the **Phase 8 — Recommendations Readiness Documentation** deliverable in the current milestone (success criterion C-04)

---

## 1. Goals & non-goals

### 1.1 Goal

Surface a **personalized "Up Next for you" homepage row** to logged-in users on AnimeEnigma, plus an **admin-only debug page** that explains every ranking decision. Built as a **pluggable foundation** that accepts new signal modules (vector similarity, implicit OP/ED affinity, combo-watched-after, etc.) without rewrites.

### 1.2 What "Up Next" means

A ranked list of anime the user has **not yet watched on this site**, presented as candidates to start. Distinct from:

- **Continue Watching** (Phase 4 of current milestone) — handles in-progress anime
- **Trending now** (existing home row) — population-level, unpersonalized
- **Similar to this** (deferred to v2) — anime detail page sidebar, anime-conditioned

### 1.3 Non-goals (v1)

- "Similar to this" sidebar on anime detail page → v2
- Anonymous user personalization → v1.1 (anon users see Trending row only in v1)
- Real-time streaming refresh → v2 (refresh window is up to 6h plus instant S6 pin on completion)
- Content embeddings / vector similarity → v2 (S7)
- Staff / VA affinity → v3 (S10)

---

## 2. Architecture

### 2.1 Pattern: weighted ensemble

Each signal module emits `(user_id, anime_id) → raw_score`. A shared per-pool **min-max normalization** maps every signal to `[0, 1]` over the candidate pool, so weights are coherent across signals. Final rank = weighted sum, then **S11 filter** removes already-watched / dropped / hidden, then optional **S6 pin** prepends a "Because you finished X" tile.

Why ensemble over tiered fallback (the existing Smart Watch Picker pattern):

1. The dataset is uneven across users — ensemble degrades gracefully where tiered cliffs are jarring.
2. "Powerful engine on top" maps naturally to "add more summands".
3. Admin debug page becomes a per-signal contribution table for free.
4. Two-stage retrieval+ranker is the right v3 architecture; ensemble can grow into it without rewrite.

### 2.2 Final score formula

```
final(u, i) = 0.30 × S1(u, i)   # score-cluster (k-NN over scored anime_list)
            + 0.20 × S2(u, i)   # item-item metadata overlap
            + 0.20 × S3(u, i)   # population trending (cold-start carrier)
            + 0.10 × S4(u, i)   # recency / currently-airing boost
            + 0.20 × S5(u, i)   # TF-IDF attribute affinity (multi-attr)

         then S11 filter:        # exclude completed/dropped/hidden
         then S6 pin:             # prepend if completion ≥7 in last 7d
```

Weights sum to 1.0 by design. **Weights are NOT renormalized when individual signals emit zero** (e.g., cold-start users) — total scores just sit lower, which is honest and preserves cross-user comparison.

### 2.3 Per-pool min-max normalization (the architectural fix)

Without per-pool normalization, raw scores at different scales (S1 ~[0,10], S5 ~[0,0.05]) silently dominate or vanish regardless of weight. The fix:

```
S_k(u, i) = (S_k_raw(u, i) − min_pool) / (max_pool − min_pool + ε)     # ε = 1e-9
```

Applied identically to every signal. Degenerate pools (max−min < ε) emit all zeros — no NaN, no division-by-zero. Implemented as a shared helper, not duplicated per signal.

---

## 3. Signal modules (v1)

| ID  | Name                       | Computes                                                             | Source                                  | When                       |
|-----|----------------------------|----------------------------------------------------------------------|-----------------------------------------|----------------------------|
| S1  | Score-cluster              | k-NN over user's score profile, predicts score for unwatched anime   | `anime_list.score`                      | Precomputed every 6h       |
| S2  | Item-item by metadata      | Cosine similarity of user's top-scored anime to candidate by tags+genres+studios | anime metadata               | Precomputed every 6h       |
| S3  | Population trending        | Last-30d watch_history starts (cold-start carrier)                   | `watch_history.watched_at`              | Precomputed every 60 min   |
| S4  | Recency boost              | Boost for `status='ongoing'` or aired in last 90 days                | `animes.aired_at`, `animes.status`      | Stateless / request-time   |
| S5  | Attribute affinity         | TF-IDF time-weighted: tags / studios / genres / demographic / source / type / producers | `watch_history × anime attrs` | Precomputed every 6h       |
| S6  | Combo-watched-after        | Co-occurrence: users who completed X with score ≥ 7 also watched Y. Cascade local → Shikimori | `anime_list.completed_at`, `anime_list.score`, Shikimori `similar` API | Precomputed nightly + Shikimori fallback request-time |
| S11 | Watched/dropped/hidden filter | Exclude anime where the user's `anime_list.status ∈ {completed, dropped}` or anime has `hidden=true` | `anime_list`, `animes` | Request-time, applied after ensemble |

> **Superseded by implementation (2026-07-17):** the shipped `S11Filter.CandidatePoolForUser` (`services/recs/internal/service/recs/signals/s11_filter.go`) excludes any anime with **ANY** `anime_list` row for the user — `watching`, `planned`, `on_hold`, `completed`, or `dropped` — not just `completed`/`dropped` as scoped above. Rationale: recs are for "things not yet on your list" full stop; the ranking signals (S1/S2/S5) still read `anime_list` independently to compute affinity, so watch history continues to shape ordering without being recommended back. This note documents the delta rather than rewriting the original spec text above.

### 3.1 S5 — TF-IDF attribute affinity (math)

The "powerful engine" piece. Per attribute `a` (across genres, studios, producers, tags, demographic, source, type):

```
unit(u, anime) = max(duration_watched / 60, 1)  if reliable_player(player)  # minutes
               = episode_count(u, anime)        otherwise                   # Kodik fallback

tf(u, a)  = Σ unit(u, anime) for anime in u.history if a ∈ anime.attrs
            ─────────────────────────────────────────────────────────
                         total_units(u)

idf(a)    = log( total_users / (1 + users_with_any_history_in[a]) )

affinity(u, a) = tf(u, a) × idf(a)
```

**Kodik dodge:** the audit revealed 84% of `watch_history` is Kodik (no video events, unreliable `duration_watched`). Reliable players: HiAnime, Consumet, AnimeLib. Kodik rows fall back to integer episode count.

**Per-attribute-type weighting:**

```
S5_raw(u, i) = 0.30 × Σ affinity(u, a) for a in i.tags
             + 0.20 × Σ affinity(u, a) for a in i.studios
             + 0.15 × Σ affinity(u, a) for a in i.genres
             + 0.10 × Σ affinity(u, a) for a in i.demographic
             + 0.10 × Σ affinity(u, a) for a in i.source         # manga / LN / original / VN / game
             + 0.10 × Σ affinity(u, a) for a in i.type           # TV / Movie / OVA / ONA / Special
             + 0.05 × Σ affinity(u, a) for a in i.producers      # noisy, low weight
```

Then min-max normalize over the candidate pool to `[0, 1]`.

**Why TF-IDF:** popular attributes (e.g. comedy genre) get a low IDF and stop dominating; users who disproportionately watch a niche studio get a strong, non-noisy signal.

**Implementation note (verify in plan-phase):** Phase 2 audit covered `watch_history`, `watch_progress`, `anime_list`, `reviews`. Whether `tags`, `source`, `demographic`, `type` are stored on the `animes` table or need backfill from Shikimori is a known unknown — must inventory the `animes` schema before signal implementation.

### 3.2 S6 — Combo-watched-after with cascade fallback

**Trigger:** user has an `anime_list` row with `status = 'completed'`, `score >= 7`, `completed_at >= now() - 7 days`.

**Variant B (v1):** prepend a single "Because you finished X" tile to the row, anchored to the seed anime. Rest of the row uses the normal ensemble. This is more transparent and easier to debug than weight-shifting (Variant A, deferred to v2).

**Cascade:**

1. Local co-occurrence: query `anime_list` for `(seed_anime, candidate_anime)` pairs from other users who completed both with score ≥ 7. With only 1952 completions across 11 users today, expect this to be sparse.
2. Shikimori fallback: hit `https://shikimori.one/api/animes/:id/similar` — Shikimori publishes "viewers who liked X also liked Y" data community-wide. Free piggyback until our local signal grows.
3. Filter cascade output through S11 (don't recommend already-watched).

**Threshold tuning:** locked at `score >= 7`. If the resulting cascade pool is too thin (zero candidates after S11), fall back to `score >= 5` for that user. **Never** use `score > 0` (the unrestricted threshold) — could surface "more like the thing they hated".

**Pin presentation contract:** API returns top item with `pinned: true` and `pin_reason: "Because you finished {anime_name}"`. Frontend renders this as a labeled tile distinct from the rest of the row.

### 3.3 Cold-start safety per signal

| User state                                                | S1     | S2     | S3      | S4      | S5     | Effective row |
|-----------------------------------------------------------|--------|--------|---------|---------|--------|---------------|
| Anonymous                                                 | —      | —      | active  | active  | —      | "Trending now" — pure S3+S4 |
| Logged in, 0 ratings, 0 watch_history                     | 0      | 0      | active  | active  | 0      | Trending-only via S3+S4 |
| Logged in, sparse data (1–3 ratings, 0 watch_history)     | weak   | weak   | active  | active  | 0      | Trending-leaning |
| Logged in, dense data                                     | active | active | active  | active  | active | Full ensemble |
| + just completed ≥7 in last 7d                            | full   | full   | full    | full    | full   | + S6 pin |

S1, S2, S5 emit zero (not NaN, not undefined) for empty inputs — the ensemble math handles this without special-casing.

---

## 4. Storage layout (hybrid C)

### 4.1 Postgres tables (new)

```sql
-- Per-user precomputed signals
CREATE TABLE rec_user_signals (
    user_id              UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    s1_vector            JSONB NOT NULL DEFAULT '{}'::jsonb,    -- {anime_id: predicted_score}
    s5_affinity          JSONB NOT NULL DEFAULT '{}'::jsonb,    -- {attr_id: affinity_value}
    s6_seed_anime_id     UUID REFERENCES animes(id),            -- last completion ≥7 in last 7d, NULL if none
    s6_seed_completed_at TIMESTAMP,
    s6_seed_score        SMALLINT,
    last_computed        TIMESTAMP NOT NULL DEFAULT now(),
    INDEX idx_rec_user_signals_last_computed (last_computed)
);

-- Population-wide signals (shared across users)
CREATE TABLE rec_population_signals (
    anime_id             UUID PRIMARY KEY REFERENCES animes(id) ON DELETE CASCADE,
    s3_trending_score    REAL NOT NULL DEFAULT 0,    -- raw start count last 30d
    s4_recency_score     REAL NOT NULL DEFAULT 0,    -- f(aired_at, status)
    last_computed        TIMESTAMP NOT NULL DEFAULT now()
);

-- Optional: materialized co-occurrence matrix for S6 local lookups
CREATE TABLE rec_completion_co_occurrence (
    seed_anime_id        UUID NOT NULL REFERENCES animes(id) ON DELETE CASCADE,
    candidate_anime_id   UUID NOT NULL REFERENCES animes(id) ON DELETE CASCADE,
    co_count             INTEGER NOT NULL,    -- users who completed both with score ≥ 7
    last_computed        TIMESTAMP NOT NULL DEFAULT now(),
    PRIMARY KEY (seed_anime_id, candidate_anime_id),
    INDEX idx_rec_co_occurrence_seed (seed_anime_id, co_count DESC)
);
```

### 4.2 Redis keys

```
recs:user:{user_id}:topN          → JSON [{anime_id, final, breakdown, pinned}, ...]
                                     TTL 6h, invalidated on any user-level signal recompute
                                     OR on s6_seed change

recs:popsignal:lastcomputed       → unix timestamp; serves as cache-buster for population signals
```

### 4.3 Storage budget

At today's scale (11 users, ~3500 anime):

- `rec_user_signals`: ~11 rows × ~5 KB = 55 KB
- `rec_population_signals`: ~3500 rows × 64 B = 224 KB
- `rec_completion_co_occurrence`: bounded by completed pairs, ~thousands of rows tops
- Redis top-N: 11 × ~10 KB = 110 KB

Total well under 1 MB. Storage cost is a non-concern until ~100k users. At that point the architecture evolves to two-stage retrieval+ranker (see §10).

---

## 5. Refresh strategy

| Signal               | Frequency                             | Trigger                                                     |
|----------------------|---------------------------------------|-------------------------------------------------------------|
| S1, S5 (per-user)    | Every 6h, via cron                    | OR debounced job on `watch_history` row insert (5 min/user) |
| S3, S4 (population)  | Every 60 min, via cron                | Pure cron, no event triggers                                |
| S6 seed              | **Synchronous** during `MarkEpisodeWatched` handler | Triggered when (status, score, completed_at) meets threshold |
| Final top-N          | Lazy: computed on first request after invalidation | Cached in Redis 6h or until any signal recompute fires      |

The S6 seed update is **synchronous on the request path** because the "Because you finished X" pin is a flagship UX moment — the user expects it to appear *immediately* after marking complete, not in 6 hours. The synchronous update is cheap (one row write to `rec_user_signals`).

The user-signal cron uses a 6h cadence as a backstop in case the on-write trigger misses (e.g., backfill from MAL import). The on-write debouncer is the primary path.

---

## 6. Backend layout

Extend the existing **player** service (it already owns `watch_history`, `watch_progress`, `anime_list`):

```
services/player/internal/service/recs/
├── ensemble.go                  # final score aggregation + filter + pin
├── normalize.go                 # shared min-max helper
├── precompute.go                # cron job runner
├── api.go                       # HTTP handlers
└── signals/
    ├── s1_score_cluster.go
    ├── s2_metadata.go
    ├── s3_trending.go
    ├── s4_recency.go
    ├── s5_attribute.go
    ├── s6_completion_seed.go    # local lookup + Shikimori cascade
    └── s11_filter.go
```

### 6.1 Signal module interface (Go)

```go
type SignalModule interface {
    // Identifier for logging, debug page columns, weight registry.
    ID() string

    // Precompute step (runs in cron / on-write). May be a no-op for stateless signals.
    Precompute(ctx context.Context, userID string) error

    // Score a candidate pool against a user (request-time). Returns map of raw scores.
    // Signals that don't depend on user state (S3, S4) ignore userID.
    Score(ctx context.Context, userID string, candidates []AnimeID) (map[AnimeID]float64, error)
}
```

The ensemble holds a registry of `{module: weight}` and orchestrates Precompute / Score / Normalize / Sum.

### 6.2 Adding a future signal (S7, S8, S9-implicit, etc.)

1. Implement the `SignalModule` interface in `signals/sX_name.go`.
2. Register in the ensemble with a weight, normalizing all weights to sum to 1.0.
3. Add a column to the admin debug page. (Driven by `module.ID()`.)
4. Schedule the precompute step in the cron registry.
5. No changes required to `ensemble.go`, `normalize.go`, or the API handler.

This is the architectural payoff of choosing weighted ensemble over tiered fallback.

---

## 7. API contract

```
GET /api/users/recs                            # logged-in user
  Auth: required (JWT or API key)
  Response 200: {
    success: true,
    data: {
      recs: [
        { anime: {...}, final: 0.764, pinned: false, ... },
        { anime: {...}, final: 0.580, pinned: false, ... },
        ...
      ],
      generated_at: "2026-05-03T08:00:00Z",
      cache_hit: true,
      total: 50
    }
  }

  When S6 active:
    recs[0] = { anime: {...}, pinned: true, pin_reason: "Because you finished Frieren", final: null }
    recs[1..] = ensemble output

GET /api/admin/recs/{user_id}                  # admin debug
  Auth: admin role required
  Response 200: {
    success: true,
    data: {
      recs: [
        {
          anime: {...},
          final: 0.764,
          breakdown: { s1: 0.91, s2: 0.85, s3: 0.40, s4: 0.20, s5: 0.95 },
          weights: { s1: 0.30, s2: 0.20, s3: 0.20, s4: 0.10, s5: 0.20 },
          top_contributor: "s5",
          contributor_detail: { studio: "Madhouse", tf: 0.41, idf: 2.30 }
        },
        ...
      ],
      computed_at: "2026-05-03T06:00:00Z",
      signal_versions: { s1: "v1.0", ..., s6: "v1.0" },
      filtered_out: [{anime_id, reason}]    # what S11 removed and why
    }
  }

POST /api/admin/recs/{user_id}/recompute
  Auth: admin role required
  Action: invalidates Redis cache for user, triggers immediate precompute, returns new top-N.
  Response 200: { computed_at, top_n, latency_ms }
```

All envelopes follow `httputil.OK { success, data, error, meta }` per the project convention.

---

## 8. Cold-start behavior

| User state                                              | Surface label              | Engine        |
|---------------------------------------------------------|----------------------------|---------------|
| Anonymous (no JWT, no API key)                          | "Trending now"             | S3 only (degenerate ensemble — others emit 0) |
| Logged in, 0 ratings, 0 watch_history                   | "Up Next for you"          | S3+S4-dominant; S1/S2/S5 = 0 |
| Logged in, sparse data                                  | "Up Next for you"          | Trending-leaning ensemble |
| Logged in, dense data                                   | "Up Next for you"          | Full ensemble |
| + just completed ≥7 in last 7d                          | "Up Next for you" + pinned | Pin + ensemble |

**No threshold-based row hiding.** The row always renders; cold-start users get sensible content (trending), and the experience smoothly upgrades as data accumulates. No frontend conditional, no "rate 5 anime to unlock" friction.

---

## 9. Admin debug page

`/admin/recs/:user_id` — Vue page rendering:

1. **Top-N table** (default 50 rows): rank | anime | poster | final | S1 | S2 | S3 | S4 | S5 | top contributor
2. **Per-row expand**: TF-IDF terms breakdown for S5, score-cluster nearest-neighbors for S1, S6 cascade source (local vs Shikimori) when applicable
3. **Filter audit**: list of anime that S11 removed, with reason (`status=completed`, `status=dropped`, `hidden=true`)
4. **Force recompute** button (calls `POST /api/admin/recs/{user_id}/recompute`)
5. **S6 seed history**: past 30 days of qualifying completions for this user
6. **Signal weight panel** (v1.1+): editable weights with "preview" button to re-rank without persisting

The debug page is what makes the ensemble auditable — a rec engine that can't be explained is a rec engine that can't be improved.

---

## 10. Future signal modules (v2+ backlog)

| ID  | Name                                       | Status   | Notes                                                      |
|-----|--------------------------------------------|----------|------------------------------------------------------------|
| S7  | Content-vector similarity                  | v2       | Embed synopsis text; pgvector or sentence-transformers     |
| S8  | Franchise / sequel proximity               | v2       | Shikimori `related` data; high precision when applicable  |
| S9  | OP/ED affinity (explicit ratings)          | **DROPPED** | Replaced by S9-implicit                                |
| S9-implicit | OP/ED affinity from skip behavior   | **v2.1, depends on Phase 5 instrumentation** | See `.planning/backlog/REC-S9-implicit-op-ed-affinity.md` |
| S10 | Director / staff / VA affinity             | v3       | Sparse data, niche cohorts                                |
| S12 | Diversification re-rank                    | v1.1 light, v2 full | Post-ranking pass: penalize over-representation by studio/genre |

Variant A (S6 weight-shift instead of pin) deferred to v2 — measure pin click-through first.

---

## 11. Testing strategy

### 11.1 Unit tests per signal module

Each `signals/sX_*.go` ships with `signals/sX_*_test.go`. Pattern:

- **Fixture-driven:** seed a known DB state, call `Score(ctx, user, candidates)`, assert exact scores.
- **Edge cases:** empty user history (returns all zeros, no NaN), single-user population (S3 returns degenerate but valid), candidate pool of size 1.
- **Determinism:** same input → same output. No randomness in scoring.

### 11.2 Property tests

- `NormalizePool` always returns values in `[0, 1]` (or all zeros for degenerate input)
- No NaN, no Inf, no negative values from any signal
- Monotonicity: if `raw_score(a) > raw_score(b)` then `normalized(a) >= normalized(b)`

### 11.3 Integration tests

Seed `ui_audit_bot` with known watch_history + anime_list state. Call `GET /api/users/recs`. Assert:

- Top-N has expected size
- S11 filter removes seeded "completed" entries
- S6 pin appears when seeded completion ≥ 7 within last 7d
- `/api/admin/recs/{user_id}` returns signal breakdown matching expectations

### 11.4 Smoke test (Playwright)

`ui_audit_bot` logs in → home page → "Up Next for you" row renders → at least one card visible. Run against staging on every deploy.

### 11.5 Eval / quality metrics (v1.1+)

Borrow the Smart Watch Picker pattern: emit a Prometheus counter `rec_click_total` (impressions) and `rec_watched_total` (anime started after click) labeled by signal contributor. Compute **CTR per signal** in Grafana — that's the v2 weight-tuning input.

---

## 12. Roadmap fit

### 12.1 Current milestone (Smart Watch Picker Overhaul, v1.0)

This document **fulfills C-04** — the Phase 8 readiness deliverable. Specifically:

- ✓ "describes the additional capture (events, columns, derived signals) that would be needed to ship REC-01 / REC-02" → §10 backlog + REC-S9 deferred file
- ✓ "explicitly states that no recommendations engine is built in this phase" → see §1.1 *and* this §12.1 sentence: **No recommendations engine code ships in this milestone.**
- ✓ "each proposed addition is annotated with rough cost" → §10 + REC-S9 backlog entry
- (pending Phase 1 baseline) "the override-rate baseline + post-overhaul number from Phase 7 are recorded for posterity" → must be appended to PROJECT.md after Phase 7 completes

When Phase 8 actually runs, the orchestrator can reference this spec as the readiness doc input rather than rewriting it.

### 12.2 Next milestone (Recommendations Engine, v2.0)

Proposed phase breakdown:

1. **Phase 1:** Schema + signal scaffolding (`SignalModule` interface, normalization helper, `rec_user_signals`/`rec_population_signals` tables, ensemble runner harness). No surface yet.
2. **Phase 2:** Land S3 + S4 + S11. Hook up admin debug page (read-only). Trending row works for anonymous users.
3. **Phase 3:** Land S1 + S2. Personalization for logged-in users. Frontend "Up Next for you" row.
4. **Phase 4:** Land S5 (TF-IDF affinity). Verify weights via A/B in admin debug page. Tune.
5. **Phase 5:** Land S6 (combo-watched-after with Shikimori cascade). Variant B pin in frontend.
6. **Phase 6:** Polish + Prometheus metrics + eval pipeline.

### 12.3 Prerequisites from current milestone

- **Phase 5 of v1.0** (analytics gap-fill: G-01 drop-off / G-02 per-episode rewatch / G-04-lite session_id) **must land before v2.0 Phase 4** so S5 has clean unit data, especially the duration-vs-episode-count fallback for Kodik.
- **Phase 6 of v1.0** (Tier 2 inference rewrite) **informs S2 weights** — the rewrite tells us which metadata signals actually correlate with override-rate improvement. v2.0 Phase 4 weight tuning piggybacks on this.

### 12.4 REC-S9 implicit OP/ED — needs Phase 5 + Phase 5.5

The deferred backlog item (`.planning/backlog/REC-S9-implicit-op-ed-affinity.md`) requires:
- `intro_skip` / `outro_skip` events (need new instrumentation; rides Phase 5 G-04 session_id work)
- Per-anime OP/ED time-window catalog (provider-dependent; Crunchyroll/HiAnime APIs expose, Kodik does not)

Realistically lands in **v2.1**, after v2.0's first signal set is in production and CTR-tuned.

---

## 13. Locked decisions

These are answers from the brainstorm session — captured for the implementation plan-phase. **Do not relitigate without explicit reason.**

| Decision                                  | Value                                          |
|-------------------------------------------|------------------------------------------------|
| Surface (v1)                              | Homepage row + admin debug page (option E)     |
| Engine pattern                            | Weighted ensemble (option B)                   |
| Cold-start strategy                       | Always show; silently upgrade (option A)       |
| Refresh + storage                         | Hybrid C (per-signal cron + Redis top-N cache) |
| S6 score threshold                        | `score >= 7`                                   |
| S6 fallback if pool too thin              | Lower to `score >= 5`                          |
| S6 pin window                             | 7 days                                         |
| S6 variant in v1                          | B (pinned tile, not weight-shift)              |
| Top-N returned to frontend                | 50 (frontend slices to 20)                     |
| User-signal cron cadence                  | 6 hours                                        |
| Population-signal cron cadence            | 60 minutes                                     |
| Redis cache TTL                           | 6 hours                                        |
| S5 attribute weights                      | tags 0.30 · studios 0.20 · genres 0.15 · demographic 0.10 · source 0.10 · type 0.10 · producers 0.05 |
| S5 Kodik fallback                         | Episode count instead of duration_watched      |
| Ensemble weights (final)                  | S1 0.30 · S2 0.20 · S3 0.20 · S4 0.10 · S5 0.20 |
| Anonymous user behavior in v1             | Trending row only ("Trending now" label)       |
| Backend service                           | Extend existing `services/player/`             |
| New tables                                | `rec_user_signals`, `rec_population_signals`, `rec_completion_co_occurrence` |
| Excluded from v1                          | S7 (vectors), S8 (franchise), S9-implicit, S10 (staff), S6 Variant A |

---

## 14. Open questions for plan-phase

1. **Schema verification:** which of `tags`, `source`, `demographic`, `type`, `studios`, `producers` are stored on `animes` today vs. need backfill from Shikimori? Phase 2 audit covered the watch tables but not `animes`. Spawn a sub-task during plan-phase.
2. **Shikimori API rate-limiting:** S6 cascade hits `shikimori.one/api/animes/:id/similar` on miss. Confirm rate-limit budget and cache duration for similar-API responses (suggest 24h).
3. **S3 trending window:** default last-30d, but if active user count is low, last-90d may produce richer signal. Validate with a Prometheus query before locking.
4. **S2 cosine vs Jaccard:** cosine similarity proposed; Jaccard over multi-set tags is also viable and cheaper. Decide in plan-phase based on Postgres array-ops vs vector storage.
5. **`hidden` flag semantics:** S11 excludes `hidden=true`. Confirm this column exists on `animes` (mentioned in CLAUDE.md migration `000005_add_hidden_flag`).
6. **Adult content filtering:** is there a per-user setting to include/exclude ecchi/R+? If so, S11 must respect it. Out of scope for math, but contract-relevant.

---

## 15. References

- Phase 2 audit (analytics inventory): `.planning/phases/02-analytics-audit/02-DRAFT-AUDIT.md`
- Backlog item (deferred S9): `.planning/backlog/REC-S9-implicit-op-ed-affinity.md`
- Project root: `.planning/PROJECT.md` (C-04 success criterion is fulfilled by this doc)
- Roadmap Phase 8: `.planning/ROADMAP.md` (Recommendations Readiness Documentation)
- Smart Watch Picker tier pattern (existing prior art for tiered fallback): `services/player/internal/service/preference/`
- HTTP envelope convention: `libs/httputil/`
- Existing watch domain models: `services/player/internal/domain/watch.go`

---

**End of design spec.** Implementation plan to be created via `superpowers:writing-plans` after user review.
