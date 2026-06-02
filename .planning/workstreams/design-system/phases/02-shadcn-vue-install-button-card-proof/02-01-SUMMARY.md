---
phase: 02-shadcn-vue-install-button-card-proof
plan: 01
subsystem: frontend/design-system
tags: [shadcn-vue, cva, tailwind-merge, reka-ui, button, card, toolchain]
requires: []
provides:
  - "cn() helper (clsx + tailwind-merge) at @/lib/utils"
  - "buttonVariants cva (default/brand/ghost/outline/destructive + legacy primary/secondary aliases; sm/md/lg/icon)"
  - "Button.vue (reka-ui Primitive + cva + cn, back-compat surface intact)"
  - "Card.vue (cn() rebuild, glass + prop API preserved) + CardHeader/CardTitle/CardContent/CardFooter subcomponents"
  - "components.json (config for future shadcn-vue add; never used by init)"
affects:
  - "frontend/web ui barrel (additive exports only)"
tech-stack:
  added:
    - "reka-ui@2.9.8"
    - "class-variance-authority@0.7.1"
    - "clsx@2.1.1"
    - "tailwind-merge@3.6.0"
  patterns:
    - "cva + cn() token-driven component variants (shadcn-vue manual install)"
    - "reka-ui Primitive for as/href polymorphism with attr/event fallthrough"
key-files:
  created:
    - frontend/web/components.json
    - frontend/web/src/lib/utils.ts
    - frontend/web/src/lib/utils.spec.ts
    - frontend/web/src/components/ui/button-variants.ts
    - frontend/web/src/components/ui/Button.spec.ts
    - frontend/web/src/components/ui/CardHeader.vue
    - frontend/web/src/components/ui/CardTitle.vue
    - frontend/web/src/components/ui/CardContent.vue
    - frontend/web/src/components/ui/CardFooter.vue
    - frontend/web/src/components/ui/Card.spec.ts
  modified:
    - frontend/web/package.json
    - frontend/web/bun.lock
    - frontend/web/src/components/ui/Button.vue
    - frontend/web/src/components/ui/Card.vue
    - frontend/web/src/components/ui/index.ts
decisions:
  - "A2 text-color: default/primary variant omits any text-* color utility — today's cyan button inherits white via text-base (font-size), so adding text-primary-foreground (#08080f) would be a visible dark regression. Kept current (decided per orchestrator guardrail; live in-browser confirm pending)."
  - "Plain twMerge (no extendTailwindMerge) — all token utilities are color-group utilities that conflict/merge by prefix (proven by cn() Test 4)."
  - "Back-compat (DS-NF-04): primary/secondary kept as cva variant aliases duplicating default/brand class strings → 9 Button consumers compile with zero edits."
metrics:
  tasks: 5 (3 auto code tasks complete; 2 in-browser checkpoints deferred to orchestrator)
  commits: 3
  files-created: 10
  files-modified: 5
  completed: 2026-06-02
---

# Phase 2 Plan 01: shadcn-vue Install + Button/Card Proof Summary

Added the shadcn-vue toolchain (Reka UI + cva + clsx + tailwind-merge + a hand-written `cn()`) and converted `Button` and `Card` to token-driven cva/`cn` components that render identically to today, with full back-compat so the 9 existing `<Button>` consumers needed zero edits.

## What Was Built

- **Task 1 — toolchain + cn():** `bun add`ed the 4 pinned deps into `dependencies`, hand-wrote `cn()` (`twMerge(clsx(...))`), and TDD'd it with 5 unit tests (concat, falsy-drop, conflict-merge, token-group conflict, array input). No `shadcn-vue init` / CLI used. No `package-lock.json` created.
- **Task 2 — Button:** `components.json` (config only, never run), `buttonVariants` cva with `default`(cyan)/`brand`(pink)/`ghost`/`outline`/`destructive` + legacy `primary`/`secondary` aliases and `sm/md/lg/icon` sizes; base binds `ring-ring` token. Rewrote `Button.vue` on reka-ui `Primitive`, preserving `href`→anchor, loading spinner, `full-width`, `#icon` slot. 10 behaviors covered (16 specs total with cn()).
- **Task 4 — Card:** Rebuilt `Card.vue` with `cn()` + Primitive, exact prop API and glass look preserved; added 4 token-driven subcomponents (Header/Title/Content/Footer, no consumers yet); barrel additively exports them. 11 specs.

## Verification Results (code gates — all green)

- `bunx vue-tsc --noEmit` → clean (exit 0). Proves DS-NF-04: 9 Button consumers typecheck with zero edits.
- `bunx vitest run` → **47 files, 714 tests passing** (686 prior + 28 new across utils/Button/Card).
- `bun run build` → clean production build (exit 0).
- `make redeploy-web` → succeeded; `animeenigma-web` container reports `healthy`.
- `main.css` UNCHANGED — `git diff` over the 3 commits shows 0 lines touched in `frontend/web/src/styles/main.css`.
- No `package-lock.json`; exactly the 4 named deps added (DS-NF-03 attested).

## TDD Gate Compliance

Each code task followed RED → implement → GREEN. (Per-task commits batch the test+impl into one `feat` commit rather than separate `test`/`feat` commits, since these are plan-level `tdd="true"` auto tasks, not a `type: tdd` plan.) RED was observed for every spec before the corresponding source existed.

## Deviations from Plan

None — plan executed exactly as written. The cva already encoded the A2 "keep current text color" decision; no edit to `button-variants.ts` was required.

## Deferred to Orchestrator (no browser available to executor)

- **Task 3 (checkpoint:human-verify):** Live in-browser confirmation that the cyan `default`/`primary` button text renders white (not dark). Code is correct per guardrail (no `text-*` color util on default/primary).
- **Task 5 (checkpoint:human-verify):** Standing 5-surface in-browser smoke (Home / Browse / Anime detail / a player / 404) + Profile.vue + ProfileSetup.vue, confirming zero rendered diff at desktop + mobile widths.

## Commits (all UNPUSHED)

- `7083bbca` feat(02-design-system): install shadcn-vue toolchain + cn() helper
- `d0a68c15` feat(02-design-system): Button -> cva + cn (shadcn-vue), back-compat aliases + components.json
- `bd975bfa` feat(02-design-system): Card -> cn() + glass preserved, add Card subcomponents

## Effort Metrics

- **UXΔ = 0 (Ambiguous)** — zero rendered change is the explicit acceptance bar.
- **CDI = 0.02 * 8** — tiny spread (2 components + cn + config), low shift (additive, back-compat aliases), Fibonacci effort 8.
- **MVQ = Sprite 88%/82%** — small, sharp, self-contained toolchain bridgehead.

## Self-Check: PASSED

- All 10 created files exist on disk.
- All 3 commit SHAs present in `git log`.
