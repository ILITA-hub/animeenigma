# Deferred / Out-of-Scope Items — Phase 04 High-Traffic Surface Migration

Items discovered during execution that are NOT caused by this plan's changes.
Logged per the SCOPE BOUNDARY rule; NOT fixed by this plan.

## Pre-existing vue-tsc failure (unrelated analytics workstream)

- **Discovered during:** Plan 04-01, Task 1 (vue-tsc gate)
- **File:** `frontend/web/src/analytics/__tests__/index.spec.ts` (UNTRACKED — `git status` `??`)
- **Error:** `TS2307: Cannot find module '../index'` (×4) — the spec imports `../index`,
  but `src/analytics/` has no `index.ts` (only `autocapture.ts`, `identity.ts`,
  `session.ts`, `transport.ts`, `types.ts`).
- **Provenance:** belongs to the in-progress frontend analytics workstream
  (`feat(analytics-fe): ...` commits + uncommitted `ActivityFeed.vue`, clickstream snippet),
  NOT the design-system migration. Zero overlap with the color/token edits in this plan.
- **Disposition:** out of scope — do NOT fix here. The analytics workstream owner must add
  the missing `src/analytics/index.ts` barrel (or remove the orphan spec).

## Pre-existing AnimeContextMenu.spec.ts failure (1/9, unrelated to color edit)

- **Discovered during:** Plan 04-01, Task 2 (AnimeContextMenu spec gate)
- **File/test:** `frontend/web/src/components/anime/AnimeContextMenu.spec.ts:227` —
  `expect(...DropdownMenuStub.props('reference')).toStrictEqual(...)` receives `undefined`.
- **Provenance:** verified PRE-EXISTING — `git stash` of my color-only edit
  (`text-amber-400` → `text-warning`, line 28) and re-running the spec on the pristine file
  reproduces the SAME 1 failed / 8 passed result. The failing assertion concerns the Reka
  `DropdownMenu` anchored-mode `reference` prop plumbing (a Phase 3 DropdownMenu greenfield
  detail), NOT the migrated color class. A color class change cannot affect a prop assertion.
- **Disposition:** out of scope for this color migration — do NOT fix here. Belongs to the
  DropdownMenu/kebab anchored-mode work (Phase 3/Plan 04 kebab rebuild). The 8 structural
  assertions that exercise the migrated component still pass.
