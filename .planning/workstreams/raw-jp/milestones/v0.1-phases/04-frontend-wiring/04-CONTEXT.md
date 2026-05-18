# Phase 4: Frontend Wiring + Changelog — Context

**Gathered:** 2026-05-18
**Status:** Ready for planning
**Mode:** `--auto` (decisions auto-derived from SPEC.md; ambiguity_score: 0.15)

<spec_lock>
## Locked Requirements (from SPEC.md)

`milestones/v0.1-phases/04-frontend-wiring/04-SPEC.md` locks: RAW-07 (provider chip integration in Anime.vue), RAW-08 (Playwright e2e against ui_audit_bot), RAW-NF-02 (changelog entry).

Key acceptance points: `VITE_RAW_PROVIDER_ENABLED=true` shows a third "RAW JP" language group with a single "AllAnime" chip; selecting it lazy-loads RawPlayer; flag false hides everything. Playwright e2e covers login → switch RAW JP → RawPlayer mounts → Other Subs opens.
</spec_lock>

<domain>
## Phase Boundary

Wire the existing RawPlayer.vue + OtherSubsPanel.vue components (built in Phase 3) into `Anime.vue` as a third "RAW JP" language group alongside RU and EN. Gate visibility behind a Vite build flag. Add a Playwright e2e covering the happy path. Append a user-facing changelog entry describing v0.1's three key shipped capabilities.

No new backend code. No new components. Only wiring, env, e2e, and changelog.

</domain>

<decisions>
## Implementation Decisions

### Type union extension
- Extend `videoLanguage` ref's inline type union with `'raw'`: `'ru' | 'en' | '18+' | 'raw'`.
- Extend `videoProvider` ref's inline type union with `'raw'`: existing six + `'raw'`.
- Extend `onUserPickedProvider` parameter type with `'raw'`.
- Map `'raw'` to `'kodik'` for the playerSwitchTracker bucket label and recordPickerEvent (it's not in the tracked PlayerName set, same handling as `'hanime'`).

### switchLanguage('raw') branch
- Loads `preferred_raw_provider` localStorage; defaults to `'raw'` (single-option for v0.1; the key exists for v0.2's hybrid resolver where `'minio'` joins).
- Persists `preferred_raw_provider` in the existing videoProvider watcher.

### Provider chip
- `VITE_RAW_PROVIDER_ENABLED` env flag — `import.meta.env.VITE_RAW_PROVIDER_ENABLED === 'true'` resolves at build time.
- `rawProviderEnabled` const declared at the top of the script setup near the lazy import of RawPlayer.
- RAW JP language pill rendered inside the existing ButtonGroup, after the 18+ button, with `v-if="rawProviderEnabled"`.
- Chip block (`<template v-else-if="videoLanguage === 'raw' && rawProviderEnabled">`) renders one button: "AllAnime", `videoProvider === 'raw'` styling uses rose-500 to differentiate from existing cyan/orange/purple/green/pink palette.
- RawPlayer rendered inside the player area with `<RawPlayer v-else-if="videoProvider === 'raw' && rawProviderEnabled" :anime-id="anime.id" />`.

### Lazy import
- `const RawPlayer = defineAsyncComponent(() => import('@/components/player/RawPlayer.vue'))` — mirrors the EnglishPlayer pattern; no bundle weight when the flag is off.

### Env config
- `frontend/web/.env.example` documents `VITE_RAW_PROVIDER_ENABLED=false` (production default).
- `frontend/web/.env` (gitignored, dev) sets `VITE_RAW_PROVIDER_ENABLED=true` for local + ui_audit_bot live smoke.

### Locale strings
- Reuse `player.raw.tab` already added in Phase 03 for the language pill (no new top-level `provider` namespace needed — keeps i18n scoped).
- All Other Subs / subtitle picker strings already shipped in Phase 03 i18n.

### Changelog
- Single Russian-locale flat array per the existing changelog.json shape (LastUpdates.vue consumer reads `[{date, entries: [{type, message}]}]`).
- Prepend three feature entries to the most-recent dated group (2026-05-18) describing: RAW JP source, Other Subs button, server-side subtitle aggregation. Tone: informative + enthusiastic, single-paragraph each, emoji-light. Mention behind-the-flag rollout.

### Playwright e2e
- `frontend/web/e2e/raw-player.spec.ts`.
- Auth: reuse the `fetch('/api/auth/login', ...)` + localStorage token/user pattern already proven by `english-player.spec.ts`.
- Anime resolution: `GET /api/anime/shikimori/52082` (Bocchi the Rock) → use returned UUID. Skip the test if Bocchi isn't seeded (avoids silent false-pass on seed drift).
- Test.skip when the RAW JP pill isn't visible (handles the case where VITE_RAW_PROVIDER_ENABLED was false at build time).
- Asserts: pill exists → click → chip exists → click → `.raw-player` mounts → `<select>` picker visible → `Other subs` button visible → click → `[role="dialog"]` matching the modal title appears.

### Claude's Discretion
- Test framework: stick with the existing `@playwright/test` configuration; no new config knobs.
- Whether to assert HLS playback actually starts — out of scope for an e2e (would require AllAnime to be live + responding; flakiness risk too high). The smoke verifies wiring, not media playback.
- Whether the changelog message is one line or multiple — mirror the existing 2026-05-18 entries (single-paragraph with the emoji at the start).

</decisions>

<code_context>
## Existing Code Insights

### Reusable assets
- `frontend/web/src/components/player/RawPlayer.vue` — built in Phase 03.
- `frontend/web/src/components/player/OtherSubsPanel.vue` — built in Phase 03.
- `frontend/web/src/api/client.ts` rawApi + subtitlesApi — built in Phase 03.
- `frontend/web/src/types/raw.ts` — built in Phase 03.
- `frontend/web/e2e/english-player.spec.ts` — provides the auth + anime-resolution patterns to mirror.

### Established patterns
- Async component lazy load via `defineAsyncComponent` (Anime.vue:1042+).
- `videoLanguage` / `videoProvider` reactive pair with paired localStorage writes (Anime.vue:1165+).
- Provider chip grouping pattern with per-group color (cyan for kodik, orange for animelib, purple for hianime, green for consumet, pink for hanime — use rose for raw).
- e2e pattern: login → resolve anime UUID via API → navigate → assert DOM.

### Integration points
- One file modified: `Anime.vue` (script + template + watcher).
- One env example modified: `.env.example`.
- One changelog modified: `public/changelog.json`.
- One e2e added: `e2e/raw-player.spec.ts`.

</code_context>

<specifics>
## Specific Ideas

- Bocchi the Rock (Shikimori 52082) is the canonical test anime per SPEC.
- The `?legacy=1` query-string convention isn't reused — RAW JP is a first-class group, not a debug surface.
- Production flip from `VITE_RAW_PROVIDER_ENABLED=false` to `true` happens after manual ui_audit_bot smoke validates the live AllAnime fetch path.

</specifics>

<deferred>
## Deferred Ideas

- v0.2 hybrid resolver: `preferred_raw_provider` accepts `'minio'` for the self-hosted library fallback.
- Multi-locale changelog: changelog.json is single-locale (RU) today; adding en/ja is a larger refactor of LastUpdates.vue's consumer, deferred.
- e2e assertion of HLS playback actually starting — too flaky for a CI smoke; skipped.

</deferred>

<canonical_refs>
## Canonical References

- `.planning/workstreams/raw-jp/milestones/v0.1-phases/04-frontend-wiring/04-SPEC.md` — Locked requirements; MUST READ.
- `docs/superpowers/specs/2026-05-18-raw-jp-provider-design.md` — Design doc.
- `frontend/web/src/views/Anime.vue` — File being extended.
- `frontend/web/e2e/english-player.spec.ts` — Auth + UUID-resolution template.
- `frontend/web/src/components/LastUpdates.vue` — Changelog consumer (read to confirm shape).
- `frontend/web/public/changelog.json` — File being extended.

</canonical_refs>
