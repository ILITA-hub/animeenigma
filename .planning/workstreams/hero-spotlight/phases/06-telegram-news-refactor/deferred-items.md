# Deferred Items — Phase 06

Out-of-scope failures discovered during Plan 06 verification. Per the
SCOPE BOUNDARY rule in the executor, these are NOT caused by Plan 06's
changes — they reproduce on the parent commit `87c75f8`.

## Pre-existing Playwright failures (reproduced on baseline)

1. **`e2e/spotlight.spec.ts > mounts above the legacy trending row`** —
   Asserts `input[placeholder]` is below the spotlight block, but Home.vue's
   search affordance is a `<button>` that opens an overlay search; there
   is no `input[placeholder]` on the home page at load time. Fix would
   require changing the locator (out of scope for Plan 06).

2. **`e2e/spotlight.spec.ts > axe-core reports zero a11y violations`** —
   Axe reports `heading-order` violation on the home page. Root cause is
   the home page's `<h1>AnimeEnigma</h1>` → `<h3>` jump (no `<h2>`).
   Pre-dates the v1.1-polish workstream.

3. **`e2e/spotlight-full.spec.ts > axe-core reports 0 violations on the
   9-card spotlight`** — Same `heading-order` violation, same root cause.
   The spotlight cards themselves render valid headings; the issue is the
   page-level scaffolding around the carousel.

## Why deferred

All three failures reproduce on commit `87c75f8` (immediately before
Plan 06's first commit), confirming Plan 06 is not the cause. Per the
executor SCOPE BOUNDARY rule:

> Only auto-fix issues DIRECTLY caused by the current task's changes.
> Pre-existing warnings, linting errors, or failures in unrelated files
> are out of scope.

These should be addressed in a dedicated UI audit / a11y polish phase,
not retro-fitted into a card refactor.
