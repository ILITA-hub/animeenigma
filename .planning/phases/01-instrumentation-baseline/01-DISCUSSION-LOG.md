# Phase 1: Instrumentation Baseline - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-04-27
**Phase:** 1-instrumentation-baseline
**Areas discussed:** Detection placement, Storage shape, What counts as an override, Anonymous user identity

---

## Detection Placement

User asked Claude to suggest a better idea before choosing.

**Better idea proposed:** Vue composable + thin POST endpoint — a single `useOverrideTracker.ts` imported by all 4 players observes prop-level combo changes and POSTs to `/api/users/preferences/override`. Solves the trilemma of (a) no per-player duplication, (b) Kodik coverage (changes happen at the Vue parent, not inside the iframe), and (c) full anon visibility (no auth required to emit).

| Option | Description | Selected |
|--------|-------------|----------|
| Accept (composable + thin endpoint) | Single composable in all 4 players → POST to player service. Covers anon + Kodik via prop-level observation. | ✓ |
| Backend-only | Detect by comparing resolved combo with the next progress save. Loses anon + late-progress users. | |
| Per-player code | Duplicate detection logic into each of the 4 players. 4× maintenance. | |

**User's choice:** Accept
**Notes:** None — accepted as proposed.

---

## Storage Shape

User asked Claude to suggest a better idea before choosing.

**Better idea proposed:** Prometheus counter + structured Loki log line — both write paths already exist in the stack. Counter powers the tile, Loki gives per-event detail for retroactive analysis. No new GORM table, no migration. Caveat: Loki retention ~31d (sufficient for baseline + Phase 7; if Phase 6 needs more, that's a Phase 5 schema add).

| Option | Description | Selected |
|--------|-------------|----------|
| Accept (Prometheus + Loki) | Counter for the tile, structured log for per-event detail. Reuses existing infra. | ✓ |
| Add DB table now | Persist to a new `combo_override_events` GORM table for unbounded retention. Costs migration + write per event. | |
| Prometheus only | Just the counter. No per-event detail; Phase 6 sees aggregates only. | |

**User's choice:** Accept
**Notes:** None — accepted as proposed.

---

## What Counts as an Override

User asked Claude to suggest a better idea before choosing.

**Better idea proposed:** First user-initiated change per `(load_session_id, dimension)` within 30s of player mount. Per-dimension labels (language/player/team/episode) tell you WHICH choice the auto-pick got wrong. Excludes auto-advance, scrubbing, pause/resume, quality switches.

| Option | Description | Selected |
|--------|-------------|----------|
| Accept (per-dimension first-change-in-30s) | UUID per mount, one event per dimension per session. Excludes non-combo changes. Per-dimension labels. | ✓ |
| Tighter window (e.g. 10s) | Only "rejected the auto-pick on sight" counts. Misses 25s rethinks. | |
| Wider window (any time during session) | Catches late rethinks but inflates the rate; harder to compare across sessions. | |

**User's choice:** Accept
**Notes:** None — accepted as proposed.

---

## Anonymous User Identity

User asked Claude to suggest a better idea before choosing.

**Better idea proposed:** UUIDv4 in localStorage as `aenig_anon_id`, sent via `X-Anon-ID` header. Real per-anon-user rate (not just a ratio). Phase 7's D-01 (anon localStorage preferences) needs exactly this key — pulling it forward to Phase 1 means D-01 inherits the infra for free.

| Option | Description | Selected |
|--------|-------------|----------|
| Accept (anon_id in localStorage) | UUIDv4 + X-Anon-ID header. Per-anon-user rate. Reused by Phase 7. | ✓ |
| Bucket only | anon=true label on the counter. Simpler now, but not a per-user rate. | |
| Defer to Phase 7 | Ship bucket-only now, add anon_id in Phase 7. Less precise baseline for anon. | |

**User's choice:** Accept
**Notes:** None — accepted as proposed.

---

## Claude's Discretion

User did not say "you decide" on any specific area, but accepted Claude-proposed defaults that left these implementation details to the planner:
- Exact Vue composable API shape (return values, lifecycle hooks)
- Whether to also emit `combo_resolve_total` on the cache-hit resolve path (recommendation: yes — every player load should be in the denominator)
- Endpoint name (`POST /api/users/preferences/override` is the working name; planner may rename if a closer convention exists)
- Debounce on the override POST (recommendation: 250ms, ignore if dimension already emitted in this session)

## Deferred Ideas

- Per-event DB table for unbounded retention — deferred to Phase 5 (Analytics Gap-Fill) if needed
- Override reason capture (WHY they overrode, not just WHICH dimension) — revisit if Phase 7's Advanced Settings has a natural place for it
- A/B segmentation of resolver changes — out of scope for Phase 1; revisit at start of Phase 6 if it becomes load-bearing
- Cross-device join of anon_id → user_id when an anon user signs up — privacy-sensitive, deferred
