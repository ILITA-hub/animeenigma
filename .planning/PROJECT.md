# AnimeEnigma

## What This Is

A self-hosted anime streaming platform with Shikimori/MAL integration, four video players (Kodik, AnimeLib, HiAnime, Consumet), a smart auto-picker that selects what users watch, and a personalized recommendations engine. v1.0 (Smart Watch Picker Overhaul) shipped 2026-05-03 — every logged-in user lands on the right episode in the right combo without thinking, and we measure success via override-rate. v2.0 (Recommendations Engine) shipped 2026-05-07 — added a personalized "Up Next for you" home row plus the pluggable foundation for future signals.

## Core Value

A logged-in user opens the home page and sees a personalized "Up Next for you" row of anime they have not yet started — ranked by a transparent weighted-ensemble of signals (their score history, attribute affinity, population trending, recency, item-item metadata, and "users who finished X also watched Y"). After completing an anime they enjoyed (score ≥ 7), a "Because you finished X" pin appears at the top of the row immediately. Anonymous users still see a useful "Trending now" row. Admins can audit every ranking decision per user.

## Current State

- ✅ **v1.0 Smart Watch Picker Overhaul** — shipped 2026-05-03 (Phases 1-8) — see `.planning/milestones/v1.0-ROADMAP.md`
- ✅ **v2.0 Recommendations Engine** — shipped 2026-05-07 (Phases 9-14) — see `.planning/milestones/v2.0-ROADMAP.md`

## Next Milestone

To be defined via `/gsd-new-milestone`. **Working title: universal anime scraper** — the abandoned HiAnime (`aniwatch` upstream / `hianime.to`) and broken Consumet (`enc-dec.app` contract change) provider paths must be replaced with a self-hosted scraping service over alive English sources (candidates: AnimeKai, AnimePahe, Anitaku/Gogoanime). Kodik (RU iframe) and AnimeLib (RU MP4) remain as separate parsers and are not in scope.

## Requirements

### Validated

<!-- v1.0 milestone items shipped 2026-05-03; preserved as system invariants for v2.0. -->

- ✓ **VAL-01**: 5-tier preference resolution (per-anime → user-global → community → pinned → default) — `services/player/internal/service/resolver.go`
- ✓ **VAL-02**: Strict no-cross-language and no-cross-dub/sub boundary lock once any tier sets it — same file
- ✓ **VAL-03**: Per-anime preference upserted on every progress save — `services/player/internal/service/preference.go`
- ✓ **VAL-04**: 4 video players (Kodik, AnimeLib, HiAnime, Consumet) consume `preferred-combo` prop and self-select on load
- ✓ **VAL-05**: 24h `localStorage` cache of resolved combo per anime — `frontend/web/src/composables/useWatchPreferences.ts`
- ✓ **VAL-06**: Frontend players auto-call `MarkEpisodeWatched` after 20 minutes of playback — bumps `anime_list.episodes` AND sets `watch_progress.completed`
- ✓ **VAL-07**: Manual "mark watched" button in every player (accessibility)
- ✓ **VAL-08**: Admin pinned-translations system (Tier 4) — `services/catalog/internal/repo/anime.go`
- ✓ **VAL-09**: Schedule view exists with `next_episode_at` / `episodes_aired` data on `AnimeInfo`
- ✓ **VAL-10** (v1.0 A-01..A-04): Resume CTA correctness — pre-player state machine with 5 states across all 4 players (`useResumeStateMachine.ts`, `Anime.vue`); single source of truth via `watch_progress.completed`
- ✓ **VAL-11** (v1.0 B-01..B-04): Tier 2 inference rewrite — duration-weighted aggregation, exponential time decay (30d half-life), two-signal output (coarse lock + fine team), min-confidence threshold
- ✓ **VAL-12** (v1.0 B-05): Profile > Advanced Settings panel with tier debug, lock override, force combo, raw weights view, reset learned preferences
- ✓ **VAL-13** (v1.0 C-01..C-04): Analytics audit + Phase 5 gap-fill (`watch_count`, `session_id`, `dropped_off_at` + dropoff beacon endpoint)
- ✓ **VAL-14** (v1.0 D-01): Anonymous localStorage-backed preference + state-machine resume CTA from localStorage watch progress
- ✓ **VAL-15** (v1.0 D-03): Cross-device freshness — `prefs_version` cookie, cache invalidation on auth change and combo save
- ✓ **VAL-16** (v1.0 M-01..M-02): `combo_override` event + Grafana override-rate dashboard with tier/language/anon/player segmentation; baseline captured pre-Phase-6 (60% / 7d / n=10)

### Active

_None — v2.0 shipped. Next milestone's requirements will be defined via `/gsd-new-milestone`._

<details>
<summary>✅ v2.0 Recommendations Engine — 23/23 requirements shipped 2026-05-07 (see <code>.planning/milestones/v2.0-REQUIREMENTS.md</code>)</summary>

#### Foundation

- [x] **REC-FOUND-01**: Pluggable `SignalModule` interface allows new signals to be added without modifying the ensemble, normalizer, or API handler
- [x] **REC-FOUND-02**: Weighted-ensemble aggregator computes `final = Σ weight × per-pool-normalized(raw)` and returns sorted `Recommendation` list
- [x] **REC-FOUND-03**: Per-pool min-max normalizer maps raw signal output to `[0, 1]` over the candidate pool with degenerate-pool guard (no NaN, no Inf)
- [x] **REC-FOUND-04**: Persistence tables `rec_user_signals`, `rec_population_signals`, `rec_completion_co_occurrence` are auto-migrated on player-service startup

#### Personalized Surfaces

- [x] **REC-UX-01**: Logged-in users see an "Up Next for you" row on the home page with up to 20 personalized recommendations
- [x] **REC-UX-02**: Anonymous users see a "Trending now" row using population signals only (S3 + S4 ensemble)
- [x] **REC-UX-03**: After a user completes an anime with score ≥ 7, a "Because you finished X" pin appears at the top of their "Up Next for you" row within seconds
- [x] **REC-UX-04**: Recommendations exclude anime the user has already completed, dropped, or that are admin-hidden

#### Signal Library

- [x] **REC-SIG-01**: S3 (population trending) ranks anime by last-30-day watch_history start count
- [x] **REC-SIG-02**: S4 (recency) boosts anime that are currently airing or aired in the last 90 days
- [x] **REC-SIG-03**: S1 (score-cluster) predicts a user's score for unwatched anime via k-NN over their `anime_list.score` history
- [x] **REC-SIG-04**: S2 (item-item metadata) ranks candidates by similarity to the user's top-scored anime over tags, genres, and studios
- [x] **REC-SIG-05**: S5 (TF-IDF attribute affinity) ranks candidates by time-weighted attribute overlap (tags 0.30, studios 0.20, genres 0.15, demographic 0.10, source 0.10, type 0.10, producers 0.05) with episode-count fallback for Kodik rows
- [x] **REC-SIG-06**: S6 (combo-watched-after) cascades local co-occurrence (score ≥ 7 completions) → Shikimori `/api/animes/:id/similar` when local pool is too thin
- [x] **REC-SIG-07**: S11 (filter) excludes anime where the user's `anime_list.status ∈ {completed, dropped}` or the anime has `hidden = true`

#### Refresh & Storage

- [x] **REC-INFRA-01**: Population signals (S3, S4) are precomputed every 60 minutes via cron
- [x] **REC-INFRA-02**: User signals (S1, S5) are precomputed every 6 hours via cron, with optional debounced on-write trigger after `watch_history` insert
- [x] **REC-INFRA-03**: S6 seed (`s6_seed_anime_id`, `s6_seed_completed_at`, `s6_seed_score`) updates synchronously inside `MarkEpisodeWatched` when a row qualifies
- [x] **REC-INFRA-04**: Top-N recommendations are cached in Redis with 6-hour TTL, invalidated on user-signal recompute or S6 seed change

#### Admin & Eval

- [x] **REC-ADMIN-01**: Admin debug page at `/admin/recs/:user_id` displays per-signal contribution table, top contributor per row, S5 TF-IDF term breakdown on expand, and S11 filter-audit list
- [x] **REC-ADMIN-02**: Admin can force-recompute a user's recs via `POST /api/admin/recs/{user_id}/recompute`
- [x] **REC-EVAL-01**: Frontend emits `rec_click` and `rec_watched` events tagged with the top contributor signal ID
- [x] **REC-EVAL-02**: Prometheus exposes per-signal click-through-rate metric (`rec_signal_ctr`) for v2.1 weight tuning

</details>

### Out of Scope

| Feature | Reason |
|---------|--------|
| "Similar to this" sidebar on anime detail page | Anime-conditioned recommendation; deferred to v3.0 — different UX surface, different math (no user state). |
| Anonymous user *personalization* (beyond trending) | X-Anon-ID exists but no aggregated history yet; deferred to v2.1. |
| Real-time streaming refresh (e.g., WebSocket-pushed rec updates) | Up to 6h staleness plus instant S6 pin is acceptable; streaming infrastructure cost is not justified. |
| S7 content-vector similarity (synopsis embeddings, pgvector) | Plan-phase decision deferred until S5 ships — measure CTR first to decide if vectors add value. |
| S8 franchise/sequel proximity | Niche signal; bundled with S7 in v3.0. |
| S9-explicit OP/ED ratings | Replaced by S9-implicit (skip-behavior); see `.planning/backlog/REC-S9-implicit-op-ed-affinity.md`. |
| S9-implicit OP/ED affinity | Requires Phase 5+ analytics instrumentation (intro_skip / outro_skip events) that does not yet exist; lands in v2.1. |
| S10 director / staff / VA affinity | Sparse data; v3.0. |
| S6 Variant A (weight-shift) | v1 ships Variant B (pinned tile) for transparency; weight-shift deferred to v2.1 once we measure pin CTR. |
| Diversification re-rank | Light pass deferred to v2.1; full diversification is a v3.0 problem. |
| Two-stage retrieval+ranker architecture | Ensemble math grows into it; not needed at <100k users. |

## Context

**Codebase state (verified 2026-05-04):**
- Backend: Go 1.22, Chi router, GORM, Postgres, Redis. Player service at `services/player/` already owns `watch_history`, `watch_progress`, `anime_list` — natural home for the rec engine.
- Frontend: Vue 3 + TypeScript + Bun + Tailwind. Home page at `frontend/web/src/views/Home.vue`. Existing rows are good prior art for the "Up Next for you" row layout.
- Shared libs: `libs/metrics`, `libs/logger`, `libs/database`, `libs/cache`, `libs/idmapping` (ARM client for AniList ID resolution — useful if S2/S5 need cross-source attribute alignment).

**Data shape (verified by v1.0 Phase 2 audit, `docs/analytics-audit-2026-04-28.md`):**
- `watch_history` rows: ~84% are Kodik with unreliable `duration_watched` — S5 falls back to integer episode count for Kodik.
- `anime_list` table has clean `status`, `score`, `is_rewatching`, `completed_at` — direct input for S1, S6.
- `animes` table schema needs verification during plan-phase: which of `tags`, `source`, `demographic`, `type`, `studios`, `producers` are stored vs. need backfill from Shikimori (open question §14.1 of design spec).
- 11 active users, ~28 unique watch_history pairs today — too thin for collaborative filtering at scale; ensemble degrades gracefully via S3+S4 cold-start carriers.

**Prior commitments:**
- Strict fallback rules from `feedback_watch_preferences.md` apply to every layer that touches preference state — never cross language or sub/dub boundary. The rec row is informational (not a preference write), so this is non-binding for ranking but binding for any downstream "auto-play first rec" feature.
- Kodik dodge: any v2.0 signal that consumes `watch_history.duration_watched` MUST handle `player='kodik'` rows via the episode-count fallback documented in spec §3.1.

## Constraints

- **Tech stack**: Go 1.22 backend, Vue 3 / TypeScript / Bun frontend — no new languages or frameworks.
- **Performance**: `GET /api/users/recs` must serve in < 100 ms p95 from cache and < 500 ms p95 on cache-miss with 50 candidates.
- **Compatibility**: New endpoints only — no changes to existing `/api/users/preferences/resolve` or `/api/users/anime-list/*` contracts.
- **Multi-language UX**: All new copy ("Up Next for you", "Because you finished X", "Trending now") must be added to both EN and RU locales.
- **Deployment**: This server IS production. Each phase that touches prod-affecting code must be redeployed via `make redeploy-<service>` and verified before marking done.
- **Storage budget**: rec engine tables stay under 1 MB for current user count; revisit at ~100k users (architecture migrates to two-stage retrieval+ranker, not the v2.0 problem).

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| (v1.0) Single project covering A+B+C+D, recs deferred | All four threads share one root cause (we cannot tell what users want, so we guess wrong). Recs isolated as future milestone. | ✓ Validated 2026-05-03 — v1.0 shipped; recs is now v2.0 |
| (v1.0) Inferred preferences (no explicit dub/sub onboarding) + Advanced Settings escape hatch | Zero-friction default for normals; full control for power users. | ✓ Validated 2026-05-03 — Advanced Settings panel deployed |
| (v1.0) Override rate < 10% as success metric | Single observable behavior measuring the project's actual goal. | Pending re-snapshot post-Phase-6 (Phase 7 follow-up) |
| (v1.0) `watch_progress.completed` becomes single source of truth for "ep watched" | Avoids `anime_list.episodes` vs `watch_progress.completed` disagreement. | ✓ Validated 2026-04-28 |
| (v2.0) Weighted ensemble pattern over tiered fallback or two-stage retrieval+ranker | Dataset is uneven across users; ensemble degrades gracefully where tiered cliffs are jarring; admin debug page becomes per-signal contribution table for free; can grow into two-stage at ~100k users without rewrite. | ✓ Validated 2026-05-07 |
| (v2.0) Per-pool min-max normalization (architectural fix) | Without it, raw scales (S1 ~[0,10] vs S5 ~[0,0.05]) silently dominate or vanish regardless of weight. | ✓ Validated 2026-05-07 |
| (v2.0) S6 score threshold ≥ 7 (not ≥ 6) with fallback to ≥ 5 if pool too thin | More conservative than my recommendation; cleaner signal; avoids "more like the thing they hated". | ✓ Validated 2026-05-07 |
| (v2.0) S6 Variant B (pinned tile) over Variant A (weight-shift) for v2.0 | More transparent and easier to debug; weight-shift deferred to v2.1 once pin CTR measured. | ✓ Validated 2026-05-07 |
| (v2.0) Hybrid storage: Postgres precomputed signals + Redis top-N cache (6h TTL) | Postgres holds durable signal vectors; Redis serves fresh top-N cheaply; on completion S6 seed update is synchronous so the pin appears immediately. | ✓ Validated 2026-05-07 |
| (v2.0) Anonymous user personalization deferred to v2.1 | X-Anon-ID exists but no aggregated history; trending row is sufficient cold-start UX for v2.0. | ✓ Validated 2026-05-07 |
| (v2.0) S5 TF-IDF time-weighting with Kodik episode-count fallback | 84% of watch_history is Kodik with unreliable duration; falling back to integer episode count keeps S5 honest. | ✓ Validated 2026-05-07 |
| (v2.0) Pluggable `SignalModule` interface from day one | Architectural payoff: future signals (S7-S10) plug in without rewrites. Single seam = single review surface for new signals. | ✓ Validated 2026-05-07 |

### Loki retention constraint (carried from v1.0)

**Loki retention is 168h / 7 days**, NOT the 31d figure mentioned in early v1.0 Phase 1 notes. Verified at `docker/loki/loki-config.yml` line 27-28. v2.0 admin debug logging that needs > 7 d retention must use Postgres or a dedicated metric, not Loki. Prometheus retention (15d) is the correct sink for `rec_signal_ctr` and friends.

### v1.0 milestone closure

v1.0 (Smart Watch Picker Overhaul) shipped all 8 phases on 2026-05-03; baseline override-rate snapshot recorded for Phase 7 follow-up (60% / 7d / n=10). Phase 7 re-snapshot is a v1.0 follow-up that does NOT block v2.0 — it can run on its original schedule (≥ 7 d after Phase 6 deploy → ≥ 2026-05-10) in parallel with v2.0 phases.

## Evolution

This document evolves at phase transitions and milestone boundaries.

**After each phase transition** (via `/gsd-transition`):
1. Requirements invalidated? → Move to Out of Scope with reason
2. Requirements validated? → Move to Validated with phase reference
3. New requirements emerged? → Add to Active
4. Decisions to log? → Add to Key Decisions
5. "What This Is" still accurate? → Update if drifted

**After each milestone** (via `/gsd-complete-milestone`):
1. Full review of all sections
2. Core Value check — still the right priority?
3. Audit Out of Scope — reasons still valid?
4. Update Context with current state

---
*Last updated: 2026-05-09 — v2.0 Recommendations Engine milestone closed (shipped 2026-05-07, audit passed, requirements archived). Next milestone (universal anime scraper) pending definition via `/gsd-new-milestone`.*
