# Phase 1: Token Foundation + Reference — SUMMARY

**Status:** ✅ Complete (2026-06-02)
**Milestone:** v1.0 Design System Consolidation
**Requirements covered:** DS-FOUND-01..07, DS-NF-01, DS-NF-02

> Phase 1 was implemented and shipped on `main` *before* this workstream existed, so its working artifacts live in the repo (not in this `phases/` dir). This summary records it for workstream continuity.

## What shipped

Consolidated the three overlapping token vocabularies in `frontend/web/src/styles/main.css` into one layered, shadcn-vue-anchored source of truth — **additively, with zero rendered change**:

- **Tier 2** shadcn-vue semantic slots + **Tier 3** brand-extension tokens added to `:root`, each referencing a Tier-1 primitive.
- `@theme inline` wiring so utilities (`bg-primary`, `bg-destructive`, `border-border`, `bg-success-soft`, …) generate.
- Legacy duplicates (`--ink`, `--ink-3`, `--accent`, `--pink`, `--pink-soft`) converted to value-preserving aliases; `--accent-soft`/`--ink-2`/`--ink-4` left literal.
- `.btn-primary`/`.btn-secondary`/`.btn:focus-visible` re-pointed to canonical tokens.
- Canonical `frontend/web/src/styles/DESIGN-SYSTEM.md` reference.
- Vitest guards: built-CSS utility emission + token/alias/`.btn-*` assertions.

## Commits (on `main`, not yet pushed)

`ba8e4e83` doc · `2e028562` tokens · `76d12a62` @theme inline + guard · `a9a6a76a` aliases · `9d2bd6fe` .btn-* re-point · `d2baa16d` review polish

## Verification

- `tsc --noEmit` clean; **686 vitest pass**; production build clean.
- Deployed via `make redeploy-web`; served bundle confirmed to carry canonical tokens + new utilities.
- In-browser smoke on 5 surfaces (home stats card, RandomTail purple `cta-hero`, catalog, anime detail cyan `.btn-primary`, 404) — **zero rendered diff**.
- Opus final code review: *approve-with-minors*; both minors fixed (`.btn-*` test coverage + doc grouping).

## Carry-forward

- The `--accent` semantic flip is deferred to **Phase 5** (DS-MIGRATE-05) — must not flip until all `var(--accent)` brand usages are repointed.
- Phase 1 commits await explicit ship / `/animeenigma-after-update` before push.

## References

- Spec: `docs/superpowers/specs/2026-06-02-design-system-consolidation-design.md`
- Plan: `docs/superpowers/plans/2026-06-02-design-system-consolidation-p1.md`
