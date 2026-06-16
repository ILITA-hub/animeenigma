---
id: PLAYER-18plus-consolidation
title: Consolidate the two 18+ players (HanimePlayer + Anime18Player) behind one surface
captured_at: 2026-06-16
captured_during: frontend dead-code sweep (2026-06-16) — flagged while removing orphaned components
deferred_from: dead-code cleanup pass (not mechanically dead — a product/architecture call)
status: backlog
---

# Consolidate the two 18+ players

## Context

The frontend ships **two separate adult-content players**, both live and wired into
the same surfaces:

- `frontend/web/src/components/player/HanimePlayer.vue` (~510 lines) — Hanime source,
  HTML5 `<video>` + hls.js.
- `frontend/web/src/components/player/Anime18Player.vue` (~481 lines) — `18anime` source.

Both are dispatched side-by-side in `views/Anime.vue` (lines ~619/631) and selected by
`composables/unifiedPlayer/useProviderResolver.ts`; `HanimePlayer` is additionally used
in `views/WatchTogetherView.vue`. They are NOT dead code — this is deliberate cleanup
scope carved out of the 2026-06-16 dead-code sweep because merging them is a
product/architecture decision, not a mechanical deletion.

## Why consolidate

~991 lines across two SFCs that very likely share most of their HTML5/hls.js playback,
quality-switching, tracking, and ReportButton wiring (same pattern as the other unified
players). Two near-parallel implementations drift independently and double the
maintenance + test surface for an edge feature.

## What to investigate / do

1. **Diff the two SFCs** — quantify genuinely shared vs. provider-specific logic
   (source resolution, CDN/referer handling, quality model). Hanime has its own CDN
   families (`hanime.tv`/`htv-*`/`hydaelyn-*`/`zodiark-*`); 18anime has its own.
2. **Decide the shape** — either (a) one `Adult`/`NSFW` player component that takes a
   provider prop + per-provider source adapter, mirroring the `UnifiedPlayer` pattern,
   or (b) extract the shared playback core into a composable both keep using.
3. **Unify dispatch** — collapse the two `v-if` branches in `Anime.vue` (and the
   `WatchTogetherView.vue` path) into one, keeping the resolver as the source selector.
4. **Tests** — fold/rewrite `Anime18Player.spec.ts` + any HanimePlayer spec against the
   consolidated component.

## Out of scope / caveats

- This is the 18+ surface — keep it behind the same gating it has today; don't change
  visibility/availability as a side effect of the refactor.
- Per-provider accent identity (Hanime pink) and CDN allowlist/signing behavior must be
  preserved.

## Acceptance

- One adult-player surface (component or shared core) with provider-specific adapters.
- `Anime.vue` + `WatchTogetherView.vue` dispatch a single component.
- No regression in Hanime or 18anime playback, quality switching, or reporting.
