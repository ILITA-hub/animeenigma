# Plan 04: Frontend Wiring + Changelog

**Phase:** 4 — Frontend Wiring + Changelog (workstream `raw-jp`, milestone v0.1)
**Created:** 2026-05-18
**Status:** Ready for execution

## Tasks

1. **Anime.vue type union** — extend `videoLanguage` ('raw') and `videoProvider` ('raw') unions; extend `onUserPickedProvider` parameter type.
2. **Anime.vue switchLanguage('raw')** — load `preferred_raw_provider`, default `'raw'`; persist via existing watcher.
3. **Anime.vue lazy import + feature flag** — `defineAsyncComponent` for RawPlayer; `rawProviderEnabled` const reading `import.meta.env.VITE_RAW_PROVIDER_ENABLED`.
4. **Anime.vue template — RAW JP language pill** — render inside the existing ButtonGroup after 18+, gated by `v-if="rawProviderEnabled"`.
5. **Anime.vue template — RAW JP chip group** — `<template v-else-if="videoLanguage === 'raw' && rawProviderEnabled">` with one AllAnime button using rose-500 palette.
6. **Anime.vue template — RawPlayer mount** — `<RawPlayer v-else-if="videoProvider === 'raw' && rawProviderEnabled" :anime-id="anime.id" />`.
7. **Anime.vue tracker mapping** — map `'raw'` → `'kodik'` for player bucket + picker event (out-of-PlayerName-set handling, mirrors `'hanime'`).
8. **`.env.example`** — document `VITE_RAW_PROVIDER_ENABLED=false`.
9. **`.env`** (local dev) — `VITE_RAW_PROVIDER_ENABLED=true` so the chip surfaces in development + ui_audit_bot smoke.
10. **changelog.json** — prepend three feature entries to the 2026-05-18 group describing RAW JP source, Other Subs button, server-side aggregation.
11. **e2e/raw-player.spec.ts** — login → resolve UUID → navigate → switch to RAW JP → assert RawPlayer mounts → assert subtitle `<select>` → click Other Subs → assert modal opens.
12. **Build verification** — `bunx tsc --noEmit` clean, `bun run build` clean.

## Out of scope

- Live AllAnime smoke (requires real network response; deferred to after-update phase).
- Multi-locale changelog (single-locale today).
- Flipping production `VITE_RAW_PROVIDER_ENABLED=true` (manual operator step after smoke).
