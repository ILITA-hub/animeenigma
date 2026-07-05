# Fix notification episode + add Share button

**Date:** 2026-07-05
**Status:** Approved (owner "go ahead")

## Problem

1. **Notification links land on the wrong episode.** `services/notifications/internal/service/payload_builder.go` `BuildWatchURL` bakes `episode=<maxWatched+1>` (computed at notification-creation time) into the link. In `Anime.vue`, `?episode` is a **hard override** in `resumeStartEpisode` that beats the live `watchState` auto-selection — so a stale `maxWatched` sends the user to the wrong episode.
2. **No way to share an exact moment.** The passive address-bar URL sync only writes `provider/team/episode` for *pinned* sources; there is no explicit share affordance and no playback-timestamp capture.

## Part A — Notification links (backend)

- `BuildWatchURL(animeID)` → returns a clean `/anime/{id}` (drop `provider`, `team`, `episode`).
- Update the single call site (`payload_builder.go` `BuildNewEpisodePayload`) and tests.
- **Result:** notification link → clean page → `watchState.resolveStartEpisode` auto-selects the correct episode (a caught-up viewer lands on the newest episode). No FE change; `?episode`/`?provider` reading stays intact for `ContinueWatchingRow` + shared links.
- **Activation unchanged** (no new auto-open machinery, per scope decision). The resume CTA already shows the correct episode.

## Part B — Share button (frontend)

### B1 — Timestamp read + seek (`t` query param, net-new)
- Add `initialTimestamp?: number` prop to `AePlayer.vue`; `Anime.vue` reads `route.query.t` (integer seconds, `> 0`) → passes it.
- On the **first** stream-ready for the initial episode, seek `<video>.currentTime` to `t` **once** (consume-guard flag), reusing the resume-seek path. Seek only — no forced autoplay.
- When `t` is present, suppress the passive resume chip for that load (explicit shared position wins).

### B2 — Share URL builder (pure)
- New helper `buildShareUrl(origin, animeId, combo, episode, timeSec)` → full URL:
  `/anime/{id}?audio=&lang=&provider=&team=&episode=&t=`
- Built from the **live resolved combo** (`state.combo`), `selectedEpisode.number`, `Math.floor(currentTime)` — **unconditionally**, so it captures the actual current source even when auto-selected (unlike the passive sync, which only writes pinned sources).
- `server` excluded (rotates; not read-side supported). Empty combo facets are omitted.

### B3 — UI + copy
- A "Copy link to this moment" row at the bottom of `PlaybackSettingsMenu.vue` root view (icon `Link`), emits `share`.
- `AePlayer` handles `share`: `navigator.clipboard.writeText(url)` → `useToast` success; manual-copy fallback on failure (mirrors the WT-invite flow).
- i18n keys added to `en.json` / `ru.json` / `ja.json`.

## Non-goals (YAGNI)
- No native `navigator.share` sheet (clipboard + toast only).
- No `server` in the share link.
- No change to the passive address-bar url-sync gating (`urlSyncState` / `watchUrlSync.ts`).
- No new notification activation path.

## Metrics
- **UXΔ = +2 (Better)** — correct notification episode + precise moment-sharing.
- **CDI = 0.015 × 8** — small spread (1 BE func + ~3 FE files), low shift, Effort_Fib 8.
- **MVQ = Sprite 88%/85%** — small, self-contained, well-bounded.

## Test plan
- BE: `BuildWatchURL` returns `/anime/{id}` with no query; payload test updated.
- FE unit: `buildShareUrl` (combo → URL, omits empty facets, floors time); `t`-param parse/seek guard; `PlaybackSettingsMenu` emits `share`.
- Gates: `bunx vue-tsc --noEmit`, design-system lint, i18n parity, real `bun run build`, `go test ./...` (notifications).
