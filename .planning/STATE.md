---
gsd_state_version: 1.0
milestone: v2.0
milestone_name: Recommendations Engine
status: executing
stopped_at: Phase 12 Wave 1 (catalog schema + Shikimori parser + AniList client) shipped — 8 commits including 2 Rule-1 deviation fixes, full production verification passed. Frieren refresh proves the Shikimori parser end-to-end (kind=tv, rating=pg_13, material_source=manga, studios=[Madhouse]). Wave 2 (services/maintenance/cmd/backfill-attributes — 3857 rows) is the next plan.
last_updated: "2026-05-06T11:55:13.863Z"
last_activity: 2026-05-06
progress:
  total_phases: 14
  completed_phases: 3
  total_plans: 6
  completed_plans: 5
  percent: 83
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-05-04 for v2.0)

**Core value:** A logged-in user opens the home page and sees a personalized "Up Next for you" row of anime they have not yet started — ranked by a transparent weighted-ensemble of signals. After completing an anime they enjoyed (score ≥ 7), a "Because you finished X" pin appears at the top of the row.
**Current focus:** Phase 12 — TF-IDF Attribute Affinity (S5)

## Current Position

Phase: 12 (TF-IDF Attribute Affinity (S5)) — EXECUTING
Plan: 3 of 3 (Wave 2 — backfill)
Status: Ready to execute
Last activity: 2026-05-06

## Performance Metrics

**Velocity:**

- Total plans completed: 0 (v2.0)
- Average duration: —
- Total execution time: —

*Updated after each plan completion*

## Accumulated Context

### Decisions

Decisions are logged in PROJECT.md Key Decisions table. Recent decisions affecting current work:

- 2026-05-06: Shikimori adaptation-source field is named `origin`, NOT `source` (CONTEXT.md §S5 was wrong). Live introspection confirmed; parser fixed in Phase 12 Wave 1.
- 2026-05-06: libs/database wrapper's AutoMigrate doesn't create m2m join tables for relations added to pre-existing structs — fall through to gorm's native AutoMigrate after the wrapper for new m2m. Caught at Phase 12 Wave 1 redeploy verification.
- 2026-05-06: v2.0 roadmap structured as 6 phases (9-14): Foundation → Population Signals + Trending → User Signals + Up Next Row → S5 TF-IDF → S6 Pin → Admin Debug + Eval. Each phase independently shippable. Phase numbering continues from v1.0 (last shipped phase = 8).
- 2026-05-04: v2.0 ensemble pattern locked over tiered fallback or two-stage retrieval+ranker — graceful cold-start, free admin breakdown, can grow into two-stage at scale without rewrite.
- 2026-05-04: Per-pool min-max normalization is the architectural fix that lets weights be coherent across signals at different raw scales.
- 2026-05-04: S6 score threshold ≥ 7 with fallback to ≥ 5 if pool too thin (more conservative than initial recommendation; cleaner signal).
- 2026-05-04: S6 Variant B (pinned tile) over Variant A (weight-shift) for v2.0 — more transparent, easier to debug; weight-shift deferred to v2.1 once pin CTR measured.
- 2026-05-04: Hybrid storage — Postgres precomputed signals + Redis 6h top-N cache. S6 seed update is synchronous on `MarkEpisodeWatched` so the pin appears immediately.
- 2026-05-04: Anonymous personalization deferred to v2.1; v2.0 anon users see population-only "Trending now" row.
- 2026-05-04: Pluggable `SignalModule` interface from day one — single seam for future signals (S7-S10).
- 2026-05-04: S5 TF-IDF time-weighting falls back to integer episode count for Kodik rows (84% of watch_history; duration_watched unreliable).

(Decisions carried from v1.0 are preserved in PROJECT.md Key Decisions table; this section tracks v2.0-fresh decisions only.)

### Pending Todos

- Plan-phase 9 (Recs Foundation): inventory `animes` table schema during plan-phase to confirm which of `tags`, `source`, `demographic`, `type`, `studios`, `producers` exist vs. need backfill (open question §14.1 of design spec) — informs Phase 12 scope.
- Plan-phase 10: confirm Redis key namespacing for anonymous trending vs. logged-in top-N to avoid cache collisions.
- Plan-phase 13: code review must verify the synchronous S6 seed update inside `MarkEpisodeWatched` adds < 5 ms p95 overhead — this is a hot path from v1.0.

### Blockers/Concerns

None yet.

## Deferred Items

| Category | Item | Status | Deferred At |
|----------|------|--------|-------------|
| Signal | S9-implicit OP/ED affinity (skip-behavior) | Backlog (`.planning/backlog/REC-S9-implicit-op-ed-affinity.md`) | 2026-04-28 (during v2.0 design brainstorm) |
| Architecture | S6 Variant A weight-shift | Deferred to v2.1 | 2026-05-04 |
| Signal | S7 content-vector similarity, S8 franchise, S10 staff | Deferred to v3.0 | 2026-05-04 |
| Surface | "Similar to this" sidebar on anime detail | Deferred to v3.0 | 2026-05-04 |
| Personalization | Anonymous user personalization (beyond trending) | Deferred to v2.1 | 2026-05-04 |

## v1.0 Carryover

- **Phase 7 follow-up (override-rate re-snapshot)** runs ≥ 7 d after Phase 6 deploy (≥ 2026-05-10) in parallel with v2.0 phases. Does NOT block v2.0.

## Session Continuity

Last session: 2026-05-06T10:15:00.000Z
Stopped at: Phase 12 Wave 1 (catalog schema + Shikimori parser + AniList client) shipped — 8 commits including 2 Rule-1 deviation fixes, full production verification passed. Frieren refresh proves the Shikimori parser end-to-end (kind=tv, rating=pg_13, material_source=manga, studios=[Madhouse]). Wave 2 (services/maintenance/cmd/backfill-attributes — 3857 rows) is the next plan.
Resume file: .planning/phases/12-tf-idf-attribute-affinity-s5/12-01-SUMMARY.md
