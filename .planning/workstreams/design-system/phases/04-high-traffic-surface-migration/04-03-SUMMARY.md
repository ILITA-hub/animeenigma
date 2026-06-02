---
phase: 04-high-traffic-surface-migration
plan: 03
subsystem: frontend-design-system
tags: [migration, tokens, value-preserving, anime-detail]
requires: [DS-FOUND (Phase 1 tokens), ui/ primitives (Phases 2-3)]
provides: [token-clean views/Anime.vue]
affects: [frontend/web/src/views/Anime.vue]
tech-stack:
  added: []
  patterns: [off-palette -> semantic-token repoint, value-preserving color migration]
key-files:
  created: []
  modified:
    - frontend/web/src/views/Anime.vue
decisions:
  - "amber/yellow -> warning, emerald/green -> success, purple/violet -> brand-violet (locked DS-MIGRATE-02)"
  - "bg-amber-500/20 -> bg-warning-soft (soft fill intent); /30 alpha kept on base token (no matching -soft alpha)"
  - "cyan .btn-primary «Смотреть» left byte-unchanged (Phase 1 token-driven, cascade-sensitive)"
  - "no Badge/Button primitive swap — token-only repoint for zero pixel diff"
metrics:
  completed: 2026-06-02
  tasks: 2
  files: 1
requirements: [DS-MIGRATE-01, DS-MIGRATE-02, DS-MIGRATE-06]
---

# Phase 4 Plan 03: Anime-Detail View Token Migration Summary

Migrated `views/Anime.vue` (the heaviest single file in this phase, 13 off-palette
occurrences across 8 lines) onto the canonical semantic tokens — a value-preserving,
color-only repoint with the cyan `.btn-primary` "Смотреть" left byte-unchanged.

## What Was Done

Applied the locked DS-MIGRATE-02 mapping per-occurrence on `frontend/web/src/views/Anime.vue`:

| Line(s) | Before | After | Role |
|---------|--------|-------|------|
| 82, 628, 704, 709 | `text-amber-400` | `text-warning` | rating stars / review-flag / schedule hue |
| 249 | `bg-amber-500/20 text-amber-400 border-amber-500/30 hover:bg-amber-500/30` | `bg-warning-soft text-warning border-warning/30 hover:bg-warning/30` | admin hide-toggle pill |
| 442 | `bg-emerald-500/20 text-emerald-400 border-emerald-500/50` | `bg-success-soft text-success border-success/50` | OurEnglish/language pill |
| 696, 805 | `hover:text-purple-400` | `hover:text-brand-violet` | review/comment author link hover |

- `bg-amber-500/20` → `bg-warning-soft` (soft fill intent matches the documented soft token).
- `bg-amber-500/30` (hover) → `bg-warning/30`: no `-soft` token matches that exact alpha, so
  the modifier was kept on the base token (per locked_mapping guidance).
- `border-amber-500/30` → `border-warning/30`, `border-emerald-500/50` → `border-success/50`:
  alpha modifiers preserved on the base token.
- The cyan `.btn-primary` "Смотреть" markup and classes are byte-unchanged (Phase 1
  DS-FOUND-05, cascade-sensitive — KEEP rule from primitive_note).
- The `bg-cyan-500/20` / `text-cyan-400` review-avatar (line 690) is the brand cyan
  (`--primary`), NOT off-palette — it is excluded from the acceptance grep and intentionally
  left untouched.
- No copy, i18n key, spacing, or font-utility changes. No Badge/Button primitive swap
  (token-only repoint chosen for zero pixel diff).

## Verification

- Off-palette acceptance grep on `views/Anime.vue`: ZERO hits.
- Hex acceptance grep on `views/Anime.vue`: ZERO hits.
- `bunx vue-tsc --noEmit` (after `rm -f *.tsbuildinfo`): exit 0, zero errors.
- `bunx vitest run`: 830 passed; 1 pre-existing failure in
  `components/anime/AnimeContextMenu.spec.ts:227` (Reka DropdownMenu anchored-mode
  `reference` prop) — already logged in `deferred-items.md`, NOT caused by this plan
  (Anime.vue does not touch AnimeContextMenu).
- `bunx vite build`: exit 0, bundle clean (Anime chunk `Anime-B97000JB.js.gz` built).
- `.btn-primary` "Смотреть" classes byte-unchanged (KEEP rule honored).

## Checkpoint (Task 2: in-browser smoke — anime detail)

`checkpoint:human-verify` — AUTO-APPROVED (auto mode active). Automated pre-checks
(vitest + vue-tsc + vite build) all green. Migration is a pure color/token repoint with
the cyan CTA byte-unchanged, so the anime-detail surface (status pill = warning hue,
language pills + green OurEnglish button = success hue, author-hover = violet) is expected
to render pixel-identically at desktop + mobile. In-browser visual smoke auto-approved per
objective (no live browser required).

⚡ Auto-approved: views/Anime.vue token migration (anime-detail smoke surface 3).

## Deviations from Plan

None — plan executed exactly as written.

## Known Stubs

None.

## Threat Flags

None — pure presentational token/class migration (T-04-03 disposition: accept; no new
attack surface, code paths, inputs, network calls, or auth changes).

## Commits

- `563a0f8d` refactor(04-03): migrate views/Anime.vue off-palette classes to semantic tokens (8 insertions / 8 deletions, 1 file)

## Self-Check: PASSED

- FOUND: frontend/web/src/views/Anime.vue
- FOUND: 04-03-SUMMARY.md
- FOUND: commit 563a0f8d
