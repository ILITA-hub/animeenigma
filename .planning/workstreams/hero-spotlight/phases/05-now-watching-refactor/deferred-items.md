# Phase 05 — Deferred items

Issues observed during Phase 05 execution that are **out of scope** per
the SCOPE BOUNDARY rule (only auto-fix issues DIRECTLY caused by the
current task's changes). All items below also reproduce on the
pre-Phase-05 NowWatchingCard.vue (`5eb612f`), confirmed via temporary
revert + re-run during Phase 05 verification.

## A11Y-DEFER-01 — `axe-core` heading-order violation in `spotlight-full.spec.ts:207`

- **Symptom:** `e2e/spotlight-full.spec.ts:207` ("axe-core reports zero
  a11y violations on the 9-card spotlight") fails with axe rule
  `heading-order — Heading order invalid`. Failing element is
  `<h3 class="text-lg md:text-xl font-semibold text-white">From our
  Telegram</h3>` (TelegramNewsCard), **not** NowWatchingCard.
- **Reproduced on:** pre-Phase-05 commit (`5eb612f`, original
  NowWatchingCard) — confirmed via temporary revert + re-run during
  Phase 05 verification. **Fails identically.**
- **Inherited from:** Phase 04 deferred-items A11Y-DEFER-01 (same
  root cause: `h1 → spotlight h3 → Ongoing h2` on Home.vue). Phase 04
  documented it; Phase 05 inherits it unchanged.
- **Suggested fix:** see Phase 04 deferred-items A11Y-DEFER-01 — either
  demote card `<h3>` to `<h4>`, or add `<h2 sr-only>` to the spotlight
  `<section>`. Cross-card audit + design call required.

## Flakes (passing on retry, not real failures)

- `spotlight-full.spec.ts:241` — arrow-key navigation cycles all 9
  slides (transition timing flake; passed on retry during Phase 05 run).
  Also flakes on pre-Phase-05 commit. Not a Phase 05 regression.
