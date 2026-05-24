# Phase 04 — Deferred items

Issues discovered during Phase 04 execution that are **out of scope** per
the Phase 04 SCOPE BOUNDARY rule (only auto-fix issues DIRECTLY caused by
the current task's changes). Both reproduce on the prior commit's
PersonalPickCard.vue — they pre-date Phase 04 and should be addressed by
a dedicated a11y plan / phase.

## A11Y-DEFER-01 — `axe-core` heading-order violation in `spotlight.spec.ts:202`

- **Symptom:** `e2e/spotlight.spec.ts:202` ("axe-core reports zero a11y
  violations on the block") and `e2e/spotlight-full.spec.ts:207` both fail
  with axe rule `heading-order — Heading order invalid`.
- **Reproduced on:** prior commit (`87c75f8`, original PersonalPickCard)
  — confirmed via temporary revert + re-run during Phase 04. **Fails
  identically.**
- **Root cause:** Home.vue page goes `h1` → spotlight `h3` → Ongoing rail
  `h2`. The spotlight cards use `<header><h3>…</h3></header>` (e.g.
  LatestNewsCard, PlatformStatsCard, AnimeOfDayCard, RandomTailCard),
  which axe flags as a "skip" (h1 → h3). Pre-dates Phase 04.
- **Suggested fix:** Either demote all card-internal `<h3>` to `<h4>` so
  the page stays `h1 → h2 → h3 → h4`, OR add an `<h2 sr-only>` to the
  spotlight `<section>` so the chain has the missing intermediate.
  Cross-card audit + design call required; leave for a follow-up phase.

## A11Y-DEFER-02 — `spotlight.spec.ts:38` "mounts above the legacy trending row"

- **Symptom:** Test fails because `searchBar` is not found at expected
  coords / `blockBox.y` assertion failure.
- **Reproduced on:** prior commit (`87c75f8`) — also fails. Pre-dates
  Phase 04.
- **Likely cause:** the home page DOM layout has shifted since the test
  was written; the assertion that the spotlight block's Y < searchBar's Y
  no longer holds with the current Home.vue ordering. Out of scope for
  Phase 04.

## Flakes (passing on retry, not real failures)

- `spotlight-full.spec.ts:241` — arrow-key navigation cycles all 9 slides
  (transition timing flake; passes on retry).
- `spotlight.spec.ts:214` — dot indicators reflect active state
  (transition timing flake; passes on retry).

Both also flake on the prior commit. Not Phase 04 regressions.
