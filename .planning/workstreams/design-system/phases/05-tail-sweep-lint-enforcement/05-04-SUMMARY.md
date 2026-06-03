---
phase: 05-tail-sweep-lint-enforcement
plan: 04
subsystem: frontend-design-system
tags: [design-system, tokens, accent, shadcn, main-css, DS-MIGRATE-05]
requires:
  - "05-03 Task 3 (ActivityFeed repoint via stash-isolation — zero brand var(--accent) survivors)"
provides:
  - "--accent resolves to the shadcn hover surface (var(--elevated)); temporary brand-cyan alias deleted"
  - "Wave-3 lint-gate prerequisite: any var(--accent) brand usage is now a true violation"
affects:
  - "frontend/web/src/styles/main.css"
tech-stack:
  added: []
  patterns:
    - "Atomic single-file token redefinition behind a hard precondition grep + auto-approved human-verify smoke"
key-files:
  created:
    - ".planning/workstreams/design-system/phases/05-tail-sweep-lint-enforcement/05-04-HUMAN-UAT.md"
    - ".planning/workstreams/design-system/phases/05-tail-sweep-lint-enforcement/05-04-SUMMARY.md"
  modified:
    - "frontend/web/src/styles/main.css"
decisions:
  - "Chose var(--elevated) (#1c1c2c) as the shadcn hover surface — the neutral elevated token already consumed by --popover / --secondary; consistent with shadcn-vue's bg-accent hover semantics."
metrics:
  duration: "~6 min"
  completed: "2026-06-03"
  tasks: 2
  files: 1
---

# Phase 5 Plan 04: `--accent` Flip to shadcn Hover Surface Summary

Flipped `main.css :root --accent` from its temporary brand-cyan alias to the neutral shadcn
hover surface (`var(--elevated)`) and deleted the temp-alias scaffolding — the design-system
milestone's ONE intentional rendered change — in a single atomic commit gated behind a hard
zero-survivors precondition.

## What Was Done

**Task 1 — flip + temp-alias deletion (commit `7127275f`):**
- HARD PRECONDITION asserted first: `grep -rnE 'var\(--accent\b' src --include='*.vue'`
  (minus literal `--accent-soft|-line|-glow`) returned **ZERO** hits across all of `src/`.
  ActivityFeed.vue and every other brand `var(--accent)` usage were repointed in Wave-1
  (incl. 05-03 Task 3's stash-isolated commit), so there were no survivors to block on.
- Redefined `--accent: var(--brand-cyan)` → `--accent: var(--elevated)` (the neutral
  `#1c1c2c` elevated surface — matching how shadcn-vue components read `bg-accent` as a
  hover background, consistent with `--popover` / `--secondary`).
- Updated the inline comment to reflect the new shadcn-hover-surface meaning.
- Removed the "NOTE: shadcn --accent (hover surface) is deferred to P2 … stays brand-cyan
  for back-compat" comment block.
- Left `--brand-cyan`, `--accent-soft`, `--accent-line`, `--accent-glow` untouched. No
  rule reordered/relayered (Tailwind v4 cascade footgun).
- Staged ONLY `frontend/web/src/styles/main.css` by explicit path; pre-existing unrelated
  uncommitted changes (ActivityFeed analytics, scraper, changelog, root STATE.md) were NOT
  swept in. Diff: 1 insertion, 3 deletions.

**Task 2 — human-verify checkpoint (auto-approved):**
- `auto_advance: true` → auto-approved. Automated gate is green (precondition grep 0 hits,
  vue-tsc exit 0, vite build exit 0). The standing 5-surface in-browser visual smoke is
  deferred and persisted as `05-04-HUMAN-UAT.md` for live confirmation — jsdom/the build
  cannot catch a Tailwind-v4 cascade regression (DS-NF-06), so the visual surface is the
  only thing not machine-verifiable here.

## The one intentional rendered change

This is, by design, **the design-system milestone's only intentional rendered change** — a
deliberate, documented hover-surface correction. Everywhere that read `bg-accent` /
`var(--accent)` as a hover surface now shows the intended neutral elevated hover instead of
the old brand cyan. No brand-cyan element regresses, because all brand usages were repointed
to `--brand-cyan` directly in Wave-1. The live 5-surface visual confirmation persists as an
open HUMAN-UAT item (auto-approved in autonomous mode; see `05-04-HUMAN-UAT.md`).

## Verification

| Check | Result |
|---|---|
| Hard precondition — brand `var(--accent)` in `src/**/*.vue` (minus literal aliases) | 0 hits |
| `--accent` no longer `var(--brand-cyan)` | confirmed |
| `--brand-cyan` + literal `--accent-soft/-line/-glow` intact | confirmed |
| `bunx vue-tsc --noEmit` | exit 0 |
| `bunx vite build` | exit 0 |
| Commit stages ONLY main.css | confirmed (1 file, 1 ins / 3 del) |

## Deviations from Plan

None — plan executed exactly as written. The chosen shadcn-hover-surface value
(`var(--elevated)`) was selected per the plan's `<current_state>` guidance (a neutral
elevated/surface-2 token) and DESIGN-SYSTEM.md Tier-2 notes, and documented in the commit.

## Known Stubs

None.

## Effort / Vibe

- UXΔ = +1 (Better) — corrects `--accent` to its intended shadcn hover-surface meaning.
- CDI = 0.01 * 5 — one token redefinition in one file; minimal spread, single intentional shift.
- MVQ = Sprite 86%/84% — tiny surgical flip behind a hard precondition + in-browser smoke gate.

## Self-Check: PASSED

- FOUND: `frontend/web/src/styles/main.css` (`--accent: var(--elevated)`, temp alias gone)
- FOUND: commit `7127275f`
- FOUND: `05-04-HUMAN-UAT.md`
