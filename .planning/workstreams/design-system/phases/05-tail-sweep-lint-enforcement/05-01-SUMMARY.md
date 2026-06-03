---
phase: 05-tail-sweep-lint-enforcement
plan: 01
subsystem: frontend-design-system
tags: [design-system, tokens, migration, vue, tailwind, spotlight, home-rails, tail-sweep]
requires:
  - "Phase 1 canonical tokens (--success/--warning/--info/--destructive/--brand-violet, --muted-foreground, --foreground, --brand-cyan)"
  - "Phase 1 deprecated alias map (--ink→--foreground, --ink-3→--muted-foreground, --accent→--brand-cyan)"
provides:
  - "Token-clean home spotlight carousel (11 SFCs) — cyan/pink/violet glow + status hues preserved"
  - "Token-clean home rails (5 SFCs: Continue Watching, Collections, columns, status banner)"
  - "Novel-hex inventory seeded for the Wave-3 design-system-allowlist.txt"
affects:
  - "frontend/web/src/components/home/spotlight/**"
  - "frontend/web/src/components/home/{CollectionsRow,ColumnItem,ContinueWatchingRow,HomeColumn,SystemStatusBanner}.vue"
tech-stack:
  added: []
  patterns:
    - "Off-palette Tailwind utility → semantic token utility (DS-MIGRATE-02); opacity modifier kept on base token when no -soft matches the alpha"
    - "Deprecated var(--ink|--ink-3|--accent) repoint to canonical name WITHOUT flipping the alias definition (DS-MIGRATE-04 partial)"
    - "var(--token, #fallback) — repoint the token NAME; the #fallback after the comma is canonical-fine, left verbatim"
    - "Novel near-base / cover-gradient hex with no token-within-tolerance kept verbatim + recorded for the Wave-3 allowlist"
key-files:
  created:
    - ".planning/workstreams/design-system/phases/05-tail-sweep-lint-enforcement/05-01-SUMMARY.md"
  modified:
    - "frontend/web/src/components/home/spotlight/CarouselControls.vue"
    - "frontend/web/src/components/home/spotlight/CarouselDots.vue"
    - "frontend/web/src/components/home/spotlight/DeltaChip.vue"
    - "frontend/web/src/components/home/spotlight/cards/ContinueWatchingNewCard.vue"
    - "frontend/web/src/components/home/spotlight/cards/FeaturedCard.vue"
    - "frontend/web/src/components/home/spotlight/cards/LatestNewsCard.vue"
    - "frontend/web/src/components/home/spotlight/cards/NotTimeYetCard.vue"
    - "frontend/web/src/components/home/spotlight/cards/NowWatchingCard.vue"
    - "frontend/web/src/components/home/spotlight/cards/PlatformStatsCard.vue"
    - "frontend/web/src/components/home/spotlight/cards/RandomTailCard.vue"
    - "frontend/web/src/components/home/spotlight/cards/TelegramNewsCard.vue"
    - "frontend/web/src/components/home/CollectionsRow.vue"
    - "frontend/web/src/components/home/ColumnItem.vue"
    - "frontend/web/src/components/home/ContinueWatchingRow.vue"
    - "frontend/web/src/components/home/HomeColumn.vue"
    - "frontend/web/src/components/home/SystemStatusBanner.vue"
    - "frontend/web/src/components/home/spotlight/DeltaChip.spec.ts (test realignment)"
    - "frontend/web/src/components/home/spotlight/cards/ContinueWatchingNewCard.spec.ts (test realignment)"
    - "frontend/web/src/components/home/spotlight/cards/LatestNewsCard.spec.ts (test realignment)"
    - "frontend/web/src/components/home/spotlight/cards/NotTimeYetCard.spec.ts (test realignment)"
    - "frontend/web/src/components/home/spotlight/cards/NowWatchingCard.spec.ts (test realignment)"
    - "frontend/web/src/components/home/spotlight/cards/RandomTailCard.spec.ts (test realignment)"
decisions:
  - "NowWatching avatar palette (red/amber/emerald/sky/violet → destructive/warning/success/info/brand-violet) — semantic tokens are hue-equivalent to the off-palette base colors, so the wide-hue avatar spread is preserved; cyan/pink/orange palette slots left as brand/provider identity"
  - "fuchsia-500 (ContinueWatchingNew ribbon gradient) NOT in the off-palette acceptance regex — left verbatim as brand-gradient identity"
  - "var(--token, #fallback) fallbacks (e.g. var(--brand-cyan,#00d4ff), var(--color-surface,#11111c)) repointed by name only; the #fallback stays as canonical-fine"
  - "Literal aliases with no canonical equivalent left untouched: --ink-2, --ink-4, --accent-line, --accent-glow, --violet (foundation var)"
  - "Cascade footgun respected: no .cta-* / .spotlight-frame / .shuffle-deck / .glass-card rule moved between layered and unlayered; color/class-only swaps in place"
  - "6 co-located spotlight specs realigned to the migrated token class strings (Rule 1 — the value-preserving migration broke stale literal-class assertions); committed with Task 2"
metrics:
  duration: "~12 min"
  completed: "2026-06-03"
  tasks: 2
  files_modified: 22
---

# Phase 05 Plan 01: Spotlight + Home-Rail Tail-Sweep Migration Summary

Migrated the home spotlight carousel (11 SFCs) and the home rails (5 SFCs) off off-palette Tailwind color classes and deprecated brand-alias `var()` usages onto canonical semantic tokens — a value-preserving (zero-rendered-change) color/token-only migration. Cyan/pink/violet glow and status hues are preserved; novel near-base and cover-gradient hex are documented for the Wave-3 allowlist seed.

## What Was Built

**Task 1 — 11 spotlight SFCs** (commit `87a4afec`):
- `DeltaChip.vue`: `bg-gray-500/20 text-gray-300` → `bg-muted/20 text-muted-foreground`; `bg-green-500/20 text-green-200` → `bg-success/20 text-success`; `bg-red-500/20 text-red-200` → `bg-destructive/20 text-destructive`.
- `NotTimeYetCard.vue`: `from-amber-500/30` → `from-warning/30`; `shadow-amber-500/20` → `shadow-warning/20`; `text-amber-300`/`text-amber-200`/`text-amber-300/70` → `text-warning(/70)`; planned/postponed pill `bg-yellow-500/20 text-yellow-200`→`bg-warning/20 text-warning`, `bg-slate-500/20 text-slate-300`→`bg-muted/20 text-muted-foreground`; `.nty-muted` `var(--ink-3)`→`var(--muted-foreground)`.
- `TelegramNewsCard.vue`: `text-sky-300`/`text-sky-200`/`text-sky-300/70` → `text-info(/70)`.
- `ContinueWatchingNewCard.vue`: `from-purple-500/30`→`from-brand-violet/30`; `shadow-purple-500/30`→`shadow-brand-violet/30`; ribbon `from-purple-600`→`from-brand-violet` (`to-fuchsia-500` kept); `text-purple-300`/`text-purple-200` → `text-brand-violet`; `.cwn-muted` `var(--ink-3)`→`var(--muted-foreground)`.
- `RandomTailCard.vue`: `from-purple-500/30`→`from-brand-violet/30`; `shadow-purple-500/20`+`group-hover:shadow-purple-500/40`→`shadow-brand-violet/20`+`group-hover:shadow-brand-violet/40`; `text-purple-300`/`text-purple-200` (both kicker headers + comment)→`text-brand-violet`; score chip `bg-yellow-500/20 text-yellow-200`→`bg-warning/20 text-warning`; `.rt-muted` `var(--ink-3)`→`var(--muted-foreground)`. `.cta-*` cascade rules untouched.
- `NowWatchingCard.vue`: header `text-green-300`→`text-success`; live dot `bg-green-400`→`bg-success`; avatar `PALETTE` red/amber/emerald/sky/violet → destructive/warning/success/info/brand-violet (cyan/pink/orange kept).
- `LatestNewsCard.vue`: `text-amber-300`→`text-warning`; fallback `text-gray-300`→`text-muted-foreground`; `.news-date` `var(--ink-3)`→`var(--muted-foreground)`.
- `CarouselControls.vue`: `.arrow-*:hover` `var(--ink)`→`var(--foreground)` (+ comment).
- `CarouselDots.vue`: `.dot-active` glow `var(--accent)`→`var(--brand-cyan)` (+ comment).
- `FeaturedCard.vue`: `.featured-eyebrow` + `.pulse` + `.btn-primary-hero` `var(--accent)`→`var(--brand-cyan)`; `.featured-title .jp` + `.featured-meta` `var(--ink-3)`→`var(--muted-foreground)`; `.btn-secondary-hero` `var(--ink)`→`var(--foreground)`. `var(--ink-2)`, `var(--accent-glow)`, `#001218` left verbatim.
- `PlatformStatsCard.vue`: `var(--accent)`→`var(--brand-cyan)` (×3), `var(--ink)`→`var(--foreground)` (×2), `var(--ink-3)`→`var(--muted-foreground)`. `var(--ink-2)`, `var(--ink-4)`, `#0d2030`/`#050a12` gradient left verbatim.

**Task 2 — 5 home-rail SFCs** (commit `058eb4a6`):
- `SystemStatusBanner.vue`: `bg-red-500/90`→`bg-destructive/90`.
- `HomeColumn.vue`: `.icon.blue` + `.all:hover` `var(--accent)`→`var(--brand-cyan)`; `.all` `var(--ink-3)`→`var(--muted-foreground)`. `var(--ink-4)`, `var(--accent-line)` kept.
- `ColumnItem.vue`: `.title:hover` + `.chip.announced` + `.chip.site-score` + `.next-ep` `var(--accent)`→`var(--brand-cyan)`; `.meta` `var(--ink-3)`→`var(--muted-foreground)`. `var(--violet)` foundation var kept.
- `CollectionsRow.vue`: `.collections-heading` + `.collection-card-title` `var(--ink)`→`var(--foreground)`. Cover-gradient `#0e7490`/`#6b21a8`, `var(--color-surface,#11111c)`, `var(--accent-line)`/`var(--accent-glow)` kept.
- `ContinueWatchingRow.vue`: `var(--ink, #fff)`→`var(--foreground, #fff)` (×2); `var(--ink-3, ...)`→`var(--muted-foreground, ...)`; `var(--accent, #00d4ff)`→`var(--brand-cyan, #00d4ff)` (×6). `var(--ink-4)`, `var(--accent-line)`, `var(--accent-glow)`, `var(--color-surface,#11111c)`, `#001218` kept.
- Realigned 6 co-located spotlight specs (DeltaChip, ContinueWatchingNew, LatestNews, NotTimeYet, NowWatching, RandomTail) to the migrated token class strings.

## Acceptance Evidence

- Off-palette grep across all 16 plan SFCs: **0 hits**.
- Repointable `var(--ink|--accent|--pink)` grep (minus literal `--ink-2/-4`, `--accent-soft/-line/-glow`): **0 hits**.
- `bunx vue-tsc --noEmit`: **exit 0**.
- `bunx vite build`: **clean** (Home chunk built, full bundle emitted).
- `bunx vitest run src/components/home/`: **223 passed / 0 failed** (17 files).
- `git status`: only this plan's files committed; no analytics/scraper files swept in.

## Novel hex for Wave-3 allowlist

| path | hex | reason |
|------|-----|--------|
| `frontend/web/src/components/home/spotlight/cards/FeaturedCard.vue` | `#001218` | `.btn-primary-hero` text-on-cyan near-base ink; no token within tolerance |
| `frontend/web/src/components/home/spotlight/cards/PlatformStatsCard.vue` | `#0d2030` | ambient card-glow linear-gradient start; novel near-base, no token |
| `frontend/web/src/components/home/spotlight/cards/PlatformStatsCard.vue` | `#050a12` | ambient card-glow linear-gradient end; novel near-base, no token |
| `frontend/web/src/components/home/spotlight/cards/NowWatchingCard.vue` | `#0a0e1a` | live-dot `ring-[#0a0e1a]` avatar-ring near-base; novel, no token |
| `frontend/web/src/components/home/CollectionsRow.vue` | `#0e7490` | collection-cover fallback gradient start (cyan-tinted); novel cover hue |
| `frontend/web/src/components/home/CollectionsRow.vue` | `#6b21a8` | collection-cover fallback gradient end (violet-tinted); novel cover hue |
| `frontend/web/src/components/home/ContinueWatchingRow.vue` | `#001218` | `.cw-play` icon-on-cyan near-base ink; no token within tolerance |

> Canonical-fine `var(--token, #fallback)` fallbacks (`#00d4ff` for `var(--brand-cyan)`, `#fff` for `var(--foreground)`, `#11111c` for `var(--color-surface)`) are NOT novel hex — they are token fallbacks and need no allowlist entry, but are noted here for the Wave-3 lint-script author.

## Deviations from Plan

**1. [Rule 1 - Bug] Realigned 6 spotlight specs to migrated token class strings**
- **Found during:** Task 2 verification (`bunx vitest run src/components/home/` after Task 1's component edits).
- **Issue:** 15 assertions across 6 co-located spotlight specs hardcoded the PRE-migration off-palette class strings (e.g. `expect(html).toContain('from-purple-500/30')`, `text-green-200`, `bg-yellow-500/20`, the avatar `PALETTE` array, `span.bg-green-400.animate-pulse`). The value-preserving migration changed those literal class strings, so the assertions failed against the (correctly) migrated render.
- **Fix:** Updated each assertion to the migrated token class (`from-brand-violet/30`, `text-success`, `bg-warning/20`, the new semantic `PALETTE`, `span.bg-success.animate-pulse`, etc.). No behavioral test logic changed — only the asserted class literals.
- **Files modified:** the 6 `.spec.ts` files listed under key-files.
- **Commit:** `058eb4a6` (committed with Task 2; the specs assert Task-1 SFCs but Task 1 was already committed, so the realignment lands with Task 2 within the same plan).

The spec files are not in the plan's `files_modified` frontmatter, but they are co-located tests broken by the in-scope SFC edits — fixing them is required to leave the tree green (success criterion: relevant vitest green).

## Known Stubs

None — pure presentational color/token migration; no data sources, props, template structure, or UI rendering paths changed.

## Self-Check: PASSED

- FOUND: frontend/web/src/components/home/spotlight/DeltaChip.vue (commit 87a4afec)
- FOUND: frontend/web/src/components/home/spotlight/cards/RandomTailCard.vue (commit 87a4afec)
- FOUND: frontend/web/src/components/home/ContinueWatchingRow.vue (commit 058eb4a6)
- FOUND: frontend/web/src/components/home/SystemStatusBanner.vue (commit 058eb4a6)
- FOUND commit 87a4afec (Task 1)
- FOUND commit 058eb4a6 (Task 2)
