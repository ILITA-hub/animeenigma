---
phase: 05-tail-sweep-lint-enforcement
plan: 02
subsystem: frontend-design-system
tags: [design-system, tokens, migration, vue, tailwind, admin-views, profile, tail-sweep]
requires:
  - "Phase 1 canonical tokens (--success/--warning/--info/--destructive/--brand-violet, --muted-foreground, --foreground, --brand-cyan)"
  - "DS-MIGRATE-02 LOCKED off-palette → semantic-token mapping (red→destructive, amber/yellow→warning, emerald/green→success, blue/sky→info, purple/violet→brand-violet)"
provides:
  - "Token-clean 4 admin views (AdminCollectionEdit, AdminCollections, AdminRecs, RawLibrary)"
  - "Token-clean 4 standalone views (Game, Profile, StatusPage, Themes) — highest-hit tail files cleared"
  - "Zero novel hex introduced (no Wave-3 allowlist additions from this plan)"
affects:
  - "frontend/web/src/views/admin/{AdminCollectionEdit,AdminCollections,AdminRecs,RawLibrary}.vue"
  - "frontend/web/src/views/{Game,Profile,StatusPage,Themes}.vue"
tech-stack:
  added: []
  patterns:
    - "Off-palette Tailwind utility → semantic token utility (DS-MIGRATE-02); /NN opacity kept on base token when no -soft matches the alpha"
    - "Categorical-palette slots (AdminRecs topContributorClass s1-s5) mapped hue-preservingly to semantic tokens; non-regex hues (orange s4, cyan s6_pin) left as palette/brand"
    - "Status-meaning judgment: green/emerald=success, red=destructive, amber/yellow=warning, blue=info, purple=brand-violet"
    - "Brand cyan border/ring + pink-400 (not in off-palette regex) left verbatim as brand identity"
key-files:
  created:
    - ".planning/workstreams/design-system/phases/05-tail-sweep-lint-enforcement/05-02-SUMMARY.md"
  modified:
    - "frontend/web/src/views/admin/AdminCollectionEdit.vue"
    - "frontend/web/src/views/admin/AdminCollections.vue"
    - "frontend/web/src/views/admin/AdminRecs.vue"
    - "frontend/web/src/views/admin/RawLibrary.vue"
    - "frontend/web/src/views/Game.vue"
    - "frontend/web/src/views/Profile.vue"
    - "frontend/web/src/views/StatusPage.vue"
    - "frontend/web/src/views/Themes.vue"
decisions:
  - "AdminRecs topContributorClass (s1-s6) is a CATEGORICAL signal-source palette, not status colors. s1 blue→info, s2 emerald→success, s3 yellow→warning, s5 purple→brand-violet (in off-palette regex, hue-preserving). s4 orange-500 + s6_pin cyan-500 left verbatim — orange is NOT in the off-palette acceptance regex and cyan is brand-primary; documented in commit message"
  - "AdminRecs reasonBadgeClass IS status-meaning: completed→success, dropped→destructive, hidden→warning"
  - "Profile statusColors list-status pill map: watching green→success, completed blue→info, plan_to_watch purple→brand-violet, on_hold yellow→warning (kept text-black for contrast), dropped red→destructive; /80 opacity kept on base token (no -soft matches alpha)"
  - "Profile text-pink-400 publicIdError left verbatim — pink is NOT in the off-palette regex and is brand; not a repointable alias (it's a Tailwind utility, not var(--pink))"
  - "Profile/Themes brand cyan border/ring (border-cyan-500/50, focus:ring-cyan-500, bg-cyan-500/20 text-cyan-400) left as brand identity"
  - "StatusPage service up/down + overall status: emerald→success, red→destructive, amber→warning (semantic up/down/degraded mapping)"
  - "No co-located specs exist for the 8 views — no test realignment needed (unlike 05-01)"
metrics:
  duration: "~10 min"
  completed: "2026-06-03"
  tasks: 2
  files_modified: 8
---

# Phase 05 Plan 02: Admin + Standalone View Tail-Sweep Migration Summary

Migrated the 4 admin views (AdminCollectionEdit, AdminCollections, AdminRecs, RawLibrary) and the 4 standalone user-facing views (Game, Profile, StatusPage, Themes) off off-palette Tailwind color classes onto canonical semantic tokens — a value-preserving (zero-rendered-change) color/token-only migration. These were the highest-hit-count tail files (Profile ≈22, StatusPage ≈13, AdminRecs ≈12, RawLibrary ≈10). Status colors now resolve from `--success / --warning / --info / --destructive / --brand-violet`. No novel hex introduced.

## What Was Built

**Task 1 — 4 admin views** (commit `e9080e23`):
- `AdminCollectionEdit.vue`: error banner `border-red-500/40 text-red-300` → `border-destructive/40 text-destructive`; saved indicator `text-emerald-400` → `text-success`; remove pill `bg-red-500/30 hover:bg-red-500/50 text-red-200` → `bg-destructive/30 hover:bg-destructive/50 text-destructive`.
- `AdminCollections.vue`: both error banners red→destructive; draft pill `bg-amber-500/20 text-amber-300` → `bg-warning/20 text-warning`; delete pill red→destructive.
- `AdminRecs.vue`: both error banners red→destructive; S5 contributor header `text-purple-300` → `text-brand-violet`; `topContributorClass` categorical palette (s1 blue→info, s2 emerald→success, s3 yellow→warning, s5 purple→brand-violet; **s4 orange-500 + s6_pin cyan-500 kept** — orange not in regex, cyan brand); `reasonBadgeClass` status (completed→success, dropped→destructive, hidden→warning).
- `RawLibrary.vue`: error banner red→destructive; cancel-job hover `hover:bg-red-500/40` → `hover:bg-destructive/40`; failed section heading/border/error-text red→destructive; retry pill `bg-amber-500/30 hover:bg-amber-500/60 text-amber-100` → `bg-warning/30 hover:bg-warning/60 text-warning`; pending-link heading/border + search focus ring amber→warning.

**Task 2 — 4 standalone views** (commit `2a9cdec9`):
- `Game.vue`: leaderboard score numbers `text-amber-400` → `text-warning` (×3).
- `Profile.vue`: stat numbers (avgScore yellow→warning, totalEpisodes green→success, completed blue→info); 8× sync/apiKey success `text-green-400` → `text-success`; remove-pill `hover:bg-red-500/20 hover:text-red-400` → `hover:bg-destructive/20 hover:text-destructive`; score badge + edit input `text-yellow-400` → `text-warning` (cyan border/ring kept); apiKeyGenerated notice `text-yellow-400` → `text-warning`; tier lock box `bg-emerald-500/10 border-emerald-500/30` / `bg-amber-500/10 border-amber-500/30` → success/warning; reset msg `text-emerald-400` → `text-success`; `statusColors` pill map (green→success, blue→info, purple→brand-violet, yellow→warning [text-black kept], red→destructive).
- `StatusPage.vue`: service-status dots `bg-emerald-500 / bg-red-500` → `bg-success / bg-destructive` (×2 blocks); error text `text-red-400` → `text-destructive` (×2 + down state); `overallDotClass` + `overallBorderClass` (operational emerald→success, degraded amber→warning, default/null red→destructive).
- `Themes.vue`: admin sync syncing pill `bg-yellow-500/20 text-yellow-400` → `bg-warning/20 text-warning` (cyan idle state kept).

## Acceptance Evidence

- Off-palette grep across all 8 plan SFCs: **0 hits**.
- Repointable `var(--ink|--accent|--pink)` grep (minus literal `--ink-2/-4`, `--accent-soft/-line/-glow`): **0 hits**.
- Raw-hex (`#xxxxxx`) scan across all 8 files: **0 hits** (no novel hex introduced).
- `bunx vue-tsc --noEmit`: **exit 0**.
- `bunx vite build`: **clean** (Profile + Themes chunks emitted, full bundle built).
- `bunx vitest run src/views/`: **26 passed / 0 failed** (1 file).
- `git status`: only this plan's 8 files committed across 2 atomic commits; no analytics/scraper/changelog files swept in.

## Novel hex for Wave-3 allowlist

None — this plan introduced **zero** new hardcoded hex. All 8 files are raw-hex-clean. No additions to the Wave-3 `design-system-allowlist.txt` seed from this plan.

## Deviations from Plan

None — plan executed exactly as written. No co-located specs exist for any of the 8 views (verified: no `src/views/{Game,Profile,StatusPage,Themes}.spec.ts` and no `src/views/admin/*.spec.ts`), so unlike plan 05-01 no test-realignment Rule-1 deviation was needed. The two unrelated specs matching pre-migration class strings (Badge.spec.ts, NowWatchingCard/LatestNewsCard.spec.ts) were confirmed not to reference any of these 8 view files.

## Semantic-judgment calls (documented per plan instruction)

- **AdminRecs `topContributorClass` orange `s4` / cyan `s6_pin`**: this function maps signal-source IDs (s1–s6) to a *categorical distinguishing palette*, not status meaning. The blue/emerald/yellow/purple slots ARE in the off-palette acceptance regex so they migrated hue-preservingly (info/success/warning/brand-violet). Orange-500 (`s4`) is NOT in the regex and cyan-500 (`s6_pin`) is brand-primary — both left verbatim. This is the plan's flagged judgment call: treated as palette-distinction, not warning/brand status colors.
- **AdminRecs `reasonBadgeClass`** by contrast IS genuine status meaning (completed/dropped/hidden) → success/destructive/warning.

## Known Stubs

None — pure presentational color/token migration; no data sources, props, template structure, or rendering paths changed.

## Self-Check: PASSED

- FOUND: frontend/web/src/views/admin/AdminCollectionEdit.vue (commit e9080e23)
- FOUND: frontend/web/src/views/admin/AdminCollections.vue (commit e9080e23)
- FOUND: frontend/web/src/views/admin/AdminRecs.vue (commit e9080e23)
- FOUND: frontend/web/src/views/admin/RawLibrary.vue (commit e9080e23)
- FOUND: frontend/web/src/views/Game.vue (commit 2a9cdec9)
- FOUND: frontend/web/src/views/Profile.vue (commit 2a9cdec9)
- FOUND: frontend/web/src/views/StatusPage.vue (commit 2a9cdec9)
- FOUND: frontend/web/src/views/Themes.vue (commit 2a9cdec9)
- FOUND commit e9080e23 (Task 1)
- FOUND commit 2a9cdec9 (Task 2)
