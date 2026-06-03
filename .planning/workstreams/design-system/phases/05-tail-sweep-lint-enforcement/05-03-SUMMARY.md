---
phase: 05-tail-sweep-lint-enforcement
plan: 03
subsystem: frontend-design-system
tags: [design-system, tokens, migration, vue, tailwind, ui-composites, themes, browse, profile, watch-together, activity-feed, stash-isolation, tail-sweep]
requires:
  - "Phase 1 canonical tokens (--popover/--success/--warning/--info/--destructive/--brand-violet/--brand-cyan/--foreground/--muted-foreground/--card)"
  - "Phase 1 deprecated alias map (--ink→--foreground, --ink-3→--muted-foreground, --accent→--brand-cyan)"
  - "05-01 + 05-02 tail-sweeps (home/spotlight/rails + admin/standalone) — this plan closes the remaining tail incl. ActivityFeed"
provides:
  - "Token-clean ui/ composites (GenreFilterPopup, Select, Toaster) — popovers on --popover surface"
  - "Token-clean App.vue + LastUpdates + themes (RatingStars/ThemeCard) + browse sidebar + profile sessions card + WT room sidebar"
  - "ActivityFeed.vue color lines repointed to canonical tokens via stash-isolation (committed color-only; analytics changes preserved uncommitted)"
  - "ZERO repo-wide off-palette + brand-alias survivors in src/**/*.vue — Wave-2 --accent flip-gate prerequisite satisfied"
affects:
  - "frontend/web/src/App.vue"
  - "frontend/web/src/components/ui/{GenreFilterPopup,Select,Toaster}.vue"
  - "frontend/web/src/components/{LastUpdates,ActivityFeed}.vue"
  - "frontend/web/src/components/themes/{RatingStars,ThemeCard}.vue"
  - "frontend/web/src/components/browse/BrowseSidebar.vue"
  - "frontend/web/src/components/profile/ActiveSessionsCard.vue"
  - "frontend/web/src/components/watch-together/RoomSidebar.vue"
tech-stack:
  added: []
  patterns:
    - "Off-palette Tailwind utility → semantic token utility (DS-MIGRATE-02); opacity modifier kept on base token when no -soft matches the alpha"
    - "Elevated popper surfaces (bg-slate-900/95) → bg-popover/95; non-elevated aside panel (bg-slate-900/40) → bg-card/40 (Phase-4 precedent)"
    - "Changelog category hex (#34d399/#fbbf24/#38bdf8/#9ca3af) → var(--color-success)/var(--color-warning)/var(--info)/var(--muted-foreground) (color: property)"
    - "Deprecated var(--ink|--ink-3|--accent) repoint to canonical NAME without flipping the alias definition (DS-MIGRATE-04)"
    - "Stash-isolation: git stash push -- <file> → edit color lines at HEAD → git add+commit color-only → git stash pop restores disjoint pending hunks"
    - "Per-provider identity hue (AnimeLib orange-500) left verbatim as decorative accent (not a status token), documented inline"
key-files:
  created:
    - ".planning/workstreams/design-system/phases/05-tail-sweep-lint-enforcement/05-03-SUMMARY.md"
  modified:
    - "frontend/web/src/App.vue"
    - "frontend/web/src/components/ui/GenreFilterPopup.vue"
    - "frontend/web/src/components/ui/Select.vue"
    - "frontend/web/src/components/ui/Toaster.vue"
    - "frontend/web/src/components/LastUpdates.vue"
    - "frontend/web/src/components/themes/RatingStars.vue"
    - "frontend/web/src/components/themes/ThemeCard.vue"
    - "frontend/web/src/components/browse/BrowseSidebar.vue"
    - "frontend/web/src/components/profile/ActiveSessionsCard.vue"
    - "frontend/web/src/components/watch-together/RoomSidebar.vue"
    - "frontend/web/src/components/ActivityFeed.vue (color lines only — stash-isolated commit; analytics changes NOT committed)"
decisions:
  - "App.vue PWA-update button bg-purple-500 hover:bg-purple-600 → bg-brand-violet hover:bg-brand-violet/90 (no darker brand-violet token; /90 preserves a slight hover-darken; precedent FeedbackButton hover:bg-brand-violet/30)"
  - "GenreFilterPopup + Select elevated poppers bg-slate-900/95 → bg-popover/95 (DS-MIGRATE-02 elevated-surface rule); cyan/pink brand classes left (outside off-palette regex)"
  - "Toaster bg-red-500/90→destructive/90, bg-emerald-500/90→success/90; bg-cyan-500/90 (info) left as brand-cyan"
  - "LastUpdates changelog color: hex mapped per LOCKED mapping_table — #34d399→var(--color-success), #fbbf24→var(--color-warning), #38bdf8→var(--info), #9ca3af→var(--muted-foreground); rgba(...) badge BACKGROUNDS left verbatim (not hex literals, not off-palette classes, and -soft alpha 0.14≠0.2 would shift the rendered tint — value-preserving)"
  - "LastUpdates thumb gradient #4c1d95 → var(--brand-violet) (within mapping tolerance); #0e7490 kept verbatim (novel cyan-tinted cover hue, Wave-3 allowlist — same as CollectionsRow in 05-01)"
  - "ThemeCard ED-type badge bg-purple-500/20 text-purple-400 → bg-brand-violet/20 text-brand-violet; rating-star text-yellow-400 → text-warning (yellow→warning per mapping)"
  - "BrowseSidebar non-elevated aside bg-slate-900/40 → bg-card/40 (resting surface, not popover); english provider text-emerald-500 focus:ring-emerald-500 → text-success focus:ring-success (green/success-family identity); AnimeLib text-orange-500 focus:ring-orange-500 KEPT (provider-identity decorative hue, mirrors --player-accent #f97316 allowlist seed — orange not in off-palette regex anyway); kodik text-cyan-500 brand, untouched"
  - "ActiveSessionsCard error text-red-400→text-destructive; this-device/online pill emerald-400 set → success (text-success bg-success/10 border-success/30, opacity kept on base token)"
  - "RoomSidebar reconnecting amber banner (text-amber-300 bg-amber-500/10 border-amber-500/20 + bg-amber-300 dot) → warning equivalents on BOTH desktop + mobile branches (amber→warning, reconnecting is a warning indicator)"
  - "ActivityFeed via stash-isolation: var(--accent)×2 → var(--brand-cyan) (.feed-who:hover, .feed-ttl); var(--ink)→var(--foreground), var(--ink-3)→var(--muted-foreground); literal var(--ink-2)/var(--ink-4) left; committed color-only (9 ins / 9 del), analytics changes (63 ins) preserved uncommitted in working tree, stash list returned to empty"
metrics:
  duration: "~14 min"
  completed: "2026-06-03"
  tasks: 3
  files_modified: 11
---

# Phase 05 Plan 03: Tail-Sweep — ui composites + App/LastUpdates + themes/browse/profile/WT + ActivityFeed (stash-isolated) Summary

Migrated the remaining tail SFCs — 3 `ui/` composites (GenreFilterPopup, Select, Toaster), App.vue, LastUpdates.vue, themes (RatingStars, ThemeCard), browse sidebar, profile active-sessions card, and the WT room sidebar — off off-palette Tailwind color classes and deprecated brand-alias `var()` usages onto canonical semantic tokens (value-preserving, zero-rendered-change). THEN repointed ActivityFeed.vue's color lines in **isolation** from its pending analytics-workstream changes via the stash-isolation recipe. Together with 05-01 + 05-02, the `src/**/*.vue` tree is now off-palette-clean and brand-alias-clean with **ZERO survivors**.

## What Was Built

**Task 1 — 3 ui/ composites + App.vue** (commit `9b820ab3`):
- `App.vue`: PWA-update button `bg-purple-500 hover:bg-purple-600` → `bg-brand-violet hover:bg-brand-violet/90`; admin-redirect banner `bg-red-500/90` → `bg-destructive/90`. `text-pink-400` (error glyph) left (brand class).
- `GenreFilterPopup.vue`: elevated dropdown popper `bg-slate-900/95` → `bg-popover/95`. cyan classes left.
- `Select.vue`: `dropdownClasses` popper `bg-slate-900/95` → `bg-popover/95`. cyan classes left.
- `Toaster.vue`: error `bg-red-500/90`→`bg-destructive/90`, success `bg-emerald-500/90`→`bg-success/90`; info `bg-cyan-500/90` left (brand-cyan).

**Task 2 — LastUpdates + themes + browse + profile + WT** (commit `a09c915b`):
- `LastUpdates.vue`: changelog category `color:` hex → tokens (`#34d399`→`var(--color-success)`, `#fbbf24`→`var(--color-warning)`, `#38bdf8`→`var(--info)`, `#9ca3af`→`var(--muted-foreground)`); `var(--ink)`→`var(--foreground)` (×4), `var(--ink-3)`→`var(--muted-foreground)` (×3); thumb gradient `#4c1d95`→`var(--brand-violet)` (`#0e7490` kept novel). `var(--ink-2)`/`var(--ink-4)` literal aliases + rgba badge backgrounds left.
- `RatingStars.vue`: remove-rating `hover:text-red-400`→`hover:text-destructive`. cyan rating-cell classes left.
- `ThemeCard.vue`: ED-type badge `bg-purple-500/20 text-purple-400`→`bg-brand-violet/20 text-brand-violet`; score star `text-yellow-400`→`text-warning`. cyan OP-badge/title classes left.
- `BrowseSidebar.vue`: aside `bg-slate-900/40`→`bg-card/40`; english provider `text-emerald-500 focus:ring-emerald-500`→`text-success focus:ring-success`; AnimeLib `text-orange-500 focus:ring-orange-500` kept (provider-identity, documented inline); kodik cyan + reset pink classes left.
- `ActiveSessionsCard.vue`: error `text-red-400`→`text-destructive`; this-device pill `text-emerald-400 bg-emerald-400/10 border-emerald-400/30`→`text-success bg-success/10 border-success/30`.
- `RoomSidebar.vue`: reconnecting amber banner (`text-amber-300 bg-amber-500/10 border-amber-500/20` + `bg-amber-300` pulse dot) → warning equivalents, on BOTH desktop (`py-2`) and mobile (`py-1`) branches.

**Task 3 — ActivityFeed.vue via stash-isolation** (commit `b61b0c4f`):
- `git stash push -- frontend/web/src/components/ActivityFeed.vue` (stashed ONLY the uncommitted analytics changes; file reverted to HEAD where the `var(--accent)`/`var(--ink)` lines are pre-existing code).
- Repointed: `var(--accent)`×2 → `var(--brand-cyan)` (`.feed-who:hover`, `.feed-ttl`); `var(--ink)`→`var(--foreground)` (×4), `var(--ink-3)`→`var(--muted-foreground)` (×3). Literal `var(--ink-2)`/`var(--ink-4)` left untouched. No logic changes.
- `git add` + commit color-only (`b61b0c4f`, **9 insertions / 9 deletions — all color lines, zero analytics hunks**).
- `git stash pop` — auto-merged cleanly (NO conflict; color lines disjoint from analytics hunks). Stash list returned to empty; ActivityFeed.vue shows ` M` again (analytics changes, 63 insertions, restored uncommitted).

## Acceptance Evidence

- Off-palette grep across all 4 Task-1 files: **0 hits**. Repointable-alias grep: **0 hits**.
- Off-palette grep across all 6 Task-2 files: **0 hits**. Repointable-alias grep: **0 hits**.
- ActivityFeed `var(--accent\b)` (excl `-soft/-line/-glow`): **0**. Repointable `var(--ink|--accent|--pink\b)` (excl literals): **0**.
- ActivityFeed committed diff = **color lines only**, no analytics hunks; `git stash list` back to empty (prior state); working tree shows ` M` ActivityFeed (analytics preserved).
- **Repo-wide off-palette** grep (`src/**/*.vue`, minus `.spec.`): **0 hits** (05-01 + 05-02 + 05-03 all landed).
- **Repo-wide repointable-alias** grep (minus `--ink-2/-4`, `--accent-soft/-line/-glow`, `--pink-soft`): **0 hits — NO survivors**. → Wave-2 `--accent` flip-gate prerequisite satisfied.
- `bunx vue-tsc --noEmit`: **exit 0** (incl. with ActivityFeed analytics changes in the working tree).
- `bunx vite build`: **clean** (full bundle emitted).
- `bunx vitest run` (ui / themes / watch-together / browse / profile): **194 passed / 0 failed** (98 ui + 96 area).
- `git status --porcelain`: only ActivityFeed.vue ` M` (preserved analytics); no analytics/scraper/STATE files swept into any of the 3 commits.

## Novel hex for Wave-3 allowlist

| path | hex | reason |
|------|-----|--------|
| `frontend/web/src/components/LastUpdates.vue` | `#0e7490` | `.update-thumb--empty` news-cover fallback gradient START (cyan-tinted); no token within tolerance (same novel hue 05-01 kept for CollectionsRow) |

> The other changelog hex (`#34d399`, `#fbbf24`, `#38bdf8`, `#9ca3af`, `#4c1d95`) were repointed to tokens — NOT novel, no allowlist entry. The `rgba(...)` badge BACKGROUNDS in LastUpdates (`rgba(16,185,129,0.2)` etc.) are literal rgba (not hex, not off-palette classes); left verbatim as value-preserving — if a future lint rule flags raw rgba in `<style>`, these four soft-tint backgrounds (feature/fix/perf/other) are candidates to repoint to `--*-soft` tokens, but the alpha differs (0.14 vs 0.2) so a swap would shift the rendered tint.

## Deviations from Plan

None — plan executed exactly as written. The stash-isolation recipe (Task 3) ran cleanly: `git stash push` reverted ActivityFeed to HEAD, the color repoint committed in isolation (color-only diff, no analytics hunks), and `git stash pop` re-applied the analytics changes via clean auto-merge (no conflict, as predicted — the edited color lines are disjoint from the analytics hunks).

> Note (not a deviation): the plan referenced the `var(--accent)` lines as "~343/352" — at HEAD (after stash, analytics reverted) they were at lines 290/299, because the stashed analytics changes had previously added lines above them. Edits were made by content, not line number, so the shift was immaterial.

## Known Stubs

None — pure presentational color/token migration; no data sources, props, template structure, or UI rendering paths changed.

## Threat Flags

None — no new network endpoints, auth paths, file access, or schema changes. Color/token-only edits.

## Self-Check: PASSED

- FOUND: frontend/web/src/App.vue (commit 9b820ab3)
- FOUND: frontend/web/src/components/ui/GenreFilterPopup.vue (commit 9b820ab3)
- FOUND: frontend/web/src/components/ui/Select.vue (commit 9b820ab3)
- FOUND: frontend/web/src/components/ui/Toaster.vue (commit 9b820ab3)
- FOUND: frontend/web/src/components/LastUpdates.vue (commit a09c915b)
- FOUND: frontend/web/src/components/themes/RatingStars.vue (commit a09c915b)
- FOUND: frontend/web/src/components/themes/ThemeCard.vue (commit a09c915b)
- FOUND: frontend/web/src/components/browse/BrowseSidebar.vue (commit a09c915b)
- FOUND: frontend/web/src/components/profile/ActiveSessionsCard.vue (commit a09c915b)
- FOUND: frontend/web/src/components/watch-together/RoomSidebar.vue (commit a09c915b)
- FOUND: frontend/web/src/components/ActivityFeed.vue color repoint (commit b61b0c4f, color-only; analytics preserved uncommitted)
- FOUND commit 9b820ab3 (Task 1)
- FOUND commit a09c915b (Task 2)
- FOUND commit b61b0c4f (Task 3)
</content>
</invoke>
