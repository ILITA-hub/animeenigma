---
phase: 04-high-traffic-surface-migration
plan: 01
subsystem: frontend-design-system
tags: [design-system, tokens, migration, vue, tailwind, catalog-surfaces]
requires:
  - "Phase 1 canonical tokens (--success/--warning/--info/--destructive/--brand-violet, --muted-foreground, --card, --border, --foreground, --brand-cyan)"
  - "Phase 1 deprecated alias map (--ink-3→--muted-foreground, --accent→--brand-cyan)"
provides:
  - "Token-clean Home.vue + Browse.vue catalog surfaces (zero off-palette, zero repointable alias)"
  - "Token-clean anime-card family (AnimeCardNew, AnimeCard, EpisodeCard, AnimeContextMenu); status pills from semantic tokens"
affects:
  - "frontend/web/src/views/Home.vue"
  - "frontend/web/src/components/anime/*"
tech-stack:
  added: []
  patterns:
    - "Off-palette Tailwind utility → semantic token utility (DS-MIGRATE-02), opacity modifier kept on base token when no -soft matches the alpha"
    - "Hardcoded #hex in <style scoped> → var(--token) (DS-MIGRATE-03)"
    - "Deprecated var(--alias) usage repoint to canonical token name WITHOUT flipping the alias definition (DS-MIGRATE-04 partial)"
key-files:
  created:
    - ".planning/workstreams/design-system/phases/04-high-traffic-surface-migration/04-01-SUMMARY.md"
    - ".planning/workstreams/design-system/phases/04-high-traffic-surface-migration/deferred-items.md"
  modified:
    - "frontend/web/src/views/Home.vue"
    - "frontend/web/src/components/anime/AnimeCardNew.vue"
    - "frontend/web/src/components/anime/AnimeCard.vue"
    - "frontend/web/src/components/anime/EpisodeCard.vue"
    - "frontend/web/src/components/anime/AnimeContextMenu.vue"
decisions:
  - "#ffd700 star-gold snapped to var(--warning) (#ffd600) — 1-unit hue delta, within tolerance per research A3"
  - "Status-pill opacity modifiers (/80, /90) kept on the base semantic token; no -soft token matches those alphas"
  - "Badge primitive swap on status pills deferred (DS-MIGRATE-06 partial) — a structural swap would shift pixels, violating the zero-rendered-change premise"
  - "Browse.vue required no edit — verified grep-clean of off-palette/alias (research inventory confirmed)"
  - "bg-cyan-500/80 in AnimeCardNew left as-is — cyan is the brand-primary color, not in the off-palette acceptance regex, not in the locked mapping"
  - "--accent-line / --accent-soft in Home.vue left untouched — literal aliases with no canonical equivalent (DS-MIGRATE-04 literal list)"
metrics:
  duration: "~14 min"
  completed: "2026-06-02"
  tasks: 3
  files_modified: 5
---

# Phase 04 Plan 01: High-Traffic Catalog Surface Migration Summary

Migrated Home, Browse, and the anime-card family off off-palette Tailwind color classes and hardcoded hex onto the canonical semantic-token system, and repointed deprecated `--ink-3`/`--accent` alias usages in Home.vue to canonical names — a value-preserving (zero-rendered-change) color/token-only migration.

## What Was Built

Three tasks executed against the catalog surfaces (smoke surfaces 1 Home + 2 Browse):

**Task 1 — Home.vue + Browse.vue** (commit `99c89e8a`):
- 3× `text-gray-400` → `text-muted-foreground` on empty-rail messages (ongoing/top/announced).
- `<style scoped>`: 2× `var(--ink-3)` → `var(--muted-foreground)` (search shell + ⌘K kbd hint); `var(--accent)` → `var(--brand-cyan)` (`.btn-ghost-accent` text).
- Left `--accent-line` / `--accent-soft` untouched (literal aliases, no canonical equivalent).
- Browse.vue: verified grep-clean — no edit needed.
- No `.cta-*` / `.spotlight-frame` / `.glass-card` / `.shuffle-deck` rule moved between layered and unlayered (cascade footgun DS-NF-02 respected).

**Task 2 — anime-card family** (commit `7b11666a`):
- AnimeCardNew.vue status pills (opacity modifier preserved on base token): `bg-amber-500/90`→`bg-warning/90`, `bg-purple-500/80`→`bg-brand-violet/80`, `bg-emerald-500/80`→`bg-success/80`, `bg-amber-500/80`→`bg-warning/80`, `bg-red-500/80`→`bg-destructive/80`.
- EpisodeCard.vue: `bg-emerald-500`→`bg-success` (watched indicator).
- AnimeContextMenu.vue: `text-amber-400`→`text-warning` (rating star).
- AnimeCard.vue hex→token: `#1a1a1a`/`#111`→`var(--card)`, `#ff6b6b`→`var(--destructive)` (×2), `#ffd700`→`var(--warning)`, `#fff`→`var(--foreground)`, `#999`/`#aaa`→`var(--muted-foreground)`, `#333`→`var(--border)`.
- GenreChip / AnimeKebab / AnimeQuickNav: verified grep-clean — no edit needed.

**Task 3 — verification gate** (checkpoint:human-verify, auto-approved):
- Full vitest: 830 passed / 1 pre-existing failure (documented below).
- vue-tsc `--noEmit`: clean across all tracked/in-scope files.
- vite production build: clean bundle (Home, Anime, Browse chunks built).

## Acceptance Evidence

- Off-palette grep across all 9 plan files: **zero hits**.
- Hex grep on AnimeCard.vue: **zero hits**.
- Repointable `var(--ink|--accent|--pink)` grep on Home.vue/Browse.vue: **zero hits** (only literal `--accent-line`/`--accent-soft` remain).
- Status colors on cards now resolve from `--success`/`--warning`/`--brand-violet`/`--destructive`.

## Deviations from Plan

None — plan executed exactly as written. All color mappings followed the locked DS-MIGRATE-02 table and the research per-file line map. The `#ffd700` star-gold judgment call (snap to `var(--warning)`) was pre-authorized by research assumption A3 and documented in the Task 2 commit message.

## Deferred Issues (pre-existing, out of scope — see deferred-items.md)

1. **`frontend/web/src/analytics/__tests__/index.spec.ts`** — `TS2307: Cannot find module '../index'` (×4). Untracked file from the in-progress analytics workstream; `src/analytics/` has no `index.ts` barrel. Causes `bun run build` to fail at its `vue-tsc` step (so the vite bundle was validated via `bunx vite build` directly + a standalone vue-tsc pass that is clean for all in-scope files). NOT caused by this plan.
2. **`AnimeContextMenu.spec.ts:227`** — `reference`-prop assertion receives `undefined` (1/9). Verified PRE-EXISTING by stashing the color-only edit and reproducing the identical 1-fail/8-pass result on the pristine file. Concerns Reka DropdownMenu anchored-mode plumbing (Phase 3 detail), not the migrated color class. The 8 structural assertions covering the migrated component pass.

## Known Stubs

None — pure presentational color/token migration; no data sources, props, or UI rendering paths changed.

## Self-Check: PASSED

- FOUND: frontend/web/src/views/Home.vue (commit 99c89e8a)
- FOUND: frontend/web/src/components/anime/AnimeCardNew.vue (commit 7b11666a)
- FOUND: frontend/web/src/components/anime/AnimeCard.vue (commit 7b11666a)
- FOUND: frontend/web/src/components/anime/EpisodeCard.vue (commit 7b11666a)
- FOUND: frontend/web/src/components/anime/AnimeContextMenu.vue (commit 7b11666a)
- FOUND commit 99c89e8a (Task 1)
- FOUND commit 7b11666a (Task 2)
