# PWA downloads: source & subtitle selection, offline subtitle auto-enable, offline boot redirect

**Date:** 2026-07-04 · **Status:** approved by owner (chat, all three recommended options)
**Metrics:** UXΔ = +3 (Better) · CDI = 0.04 * 21 · MVQ = Griffin 85%/80%

## Owner asks

1. When downloading (single episode AND whole season), let the user pick **which provider/source** to download from and **which subtitles** to include.
2. The chosen subtitles must **actually display during offline playback** (owner confirmed: auto-enable them — an explicit download-time choice overrides the global "subs default OFF, never auto-enable" rule for that download only).
3. Opening the app **without internet lands directly on /downloads**.
4. Downloads are **Wi-Fi-only by default**: on mobile data, pause + an extra explicit confirmation «качать с мобильных данных» (added at spec review, 2026-07-04).

## Current state (verified in code)

- `DownloadDialog.vue` picks only quality (480/720/1080) + scope (episode/season). No combo, no subtitle UI.
- Combo is frozen without user input: in-player → current `state.combo` snapshot (`AePlayer.vue` `onConfirmDownload`); card flow → `pickDefaultCombo()` (`src/offline/seasonDownloadFlow.ts:63`).
- The engine caches only provider-bundled subs that arrive in `StreamResult.subtitles` (`downloadEngine.ts` `cacheSubtitles`). Aggregated tracks (Jimaku/OpenSubtitles via `GET /api/anime/{id}/subtitles/all?episode=N`) are NOT downloaded — offline that API fails and those tracks vanish.
- No offline redirect: the SW serves the shell, the user must navigate to /downloads manually.

## Design

### 1. DownloadDialog: "Source" section (full combo picker)

`DownloadDialog.vue` stays presentational; it gains a compact combo picker mirroring the player's Source panel logic:

- **Audio**: RAW/DUB toggle (`AudioKind` 'sub'/'dub').
- **Language**: RU/EN — shown only under DUB (RAW drops the lang filter, per the player model).
- **Provider**: select built from `rowsFromReport(report, { audio, lang, content })` (`useProviderFeed.ts`); content tries `'common'` then `'hentai'` exactly like `pickDefaultCombo`. Under RAW the served lang derives from `GROUP_PRIMARY_LANG[row.group]`.
- **Team**: select, options loaded via a `loadTeams(provider, audio)` function prop (host wraps `resolver.listTeams`); default «Авто» = `null`. Load failure → just «Авто».
- `server` stays `''` — adapters pick a compatible server themselves (already proven by the card season flow).

Defaults: in-player → the currently playing combo; card flow → `pickDefaultCombo()` result. When audio/lang change removes the selected provider from the row list, re-default via `pickSmartDefault(rows) ?? pickSelectableFallback(rows)`.

New props: `report: CapabilityReport | null`, `initialCombo: Combo`, `loadTeams`, `subOptions: SubOption[]`. New emit shape: `confirm(quality, scope, combo, subPref)`.

Player exemption note: DownloadDialog lives under `components/player/` where native controls are allowed (reka portals break in fullscreen) — keep native-styled buttons/selects consistent with the existing dialog.

### 2. DownloadDialog: "Subtitles" section

Single-select (one active subtitle track matches the player UX). Options are `SubOption = { key: string; label: string; pref: SubPref }` built by the host:

- «Не включать» — default.
- **Bundled tracks**: in-player, concrete entries from the current stream's `providerBundledTracks` (kind `bundled`, per lang). Card flow (no resolved stream) offers one generic «Встроенные в поток» entry (kind `bundled`, lang `'auto'`).
- **External tracks**: aggregated tracks from `/subtitles/all` — in-player from `useSubtitleTracks().tracks` (aggregated part; `ensureLoaded()` fires when the dialog opens); card flow fetches `/subtitles/all` for the first target episode. Fetch failure → external group simply absent.

Provider-bundled subs continue to be cached unconditionally (unchanged, they ride along with the stream). The selection controls (a) which **external** track is additionally downloaded per episode and (b) which track **auto-enables** at offline playback.

### 3. Data model: preference descriptor, not URL

Aggregated track URLs are **per-episode** (and signed URLs expire in queue), so a season download cannot freeze a URL at dialog time. Chosen approach (A): store a descriptor and resolve per episode.

```ts
// src/offline/types.ts
export type SubPref =
  | { kind: 'bundled'; lang: string }            // lang 'auto' = first bundled track
  | { kind: 'external'; provider: string; lang: string; label?: string }

interface OfflineDownload {
  // ... existing fields unchanged ...
  subPref?: SubPref       // what the user asked for (audit/re-resolve)
  autoSubUrl?: string     // local /__offline/{id}/sub/{k} path of the matched track
}
```

Backward compatible: old records without these fields behave exactly as today (no auto-enable).

Rejected alternatives: (B) freezing concrete URLs at dialog time — breaks for season scope + expiring signatures; (C) always downloading ALL aggregated tracks — Jimaku ships many files per episode; wasteful quota/API load and still needs a separate auto-enable choice.

### 4. Download engine

`DownloadRequest` gains `subPref?: SubPref` and `resolveSubs?: () => Promise<SubtitleTrack[]>` (caller-supplied per-episode closure, same pattern as `resolve()`).

In `runDownload`, after stream resolve:

```ts
const external = req.resolveSubs ? await req.resolveSubs().catch(() => []) : []
const localSubs = await cacheSubtitles(id, [...(stream.subtitles ?? []), ...external])
```

Then compute `autoSubUrl` from `localSubs` by matching `subPref`:
- `external` → first cached track with matching `provider` + `lang` (prefer exact `label` match). External providers (jimaku/opensubtitles/…) never collide with scraper provider ids, so matching by provider is unambiguous.
- `bundled` with concrete lang → first cached track whose `provider === combo.provider` and lang matches.
- `bundled`/`'auto'` → first cached track with `provider === combo.provider`.
- No match (track missing for this episode, fetch failed) → `autoSubUrl` stays unset; the download itself is NOT failed (same non-fatal stance as today's `cacheSubtitles`).

Persist `subPref` + `autoSubUrl` in the record `update()`. The single-flight re-resolve path (`ensureFreshUrls`) is untouched — subs are cached before segment fetching starts.

**Shared helper** `src/offline/externalSubs.ts`: `makeExternalSubResolver(animeId, pref)` → `(ep: EpisodeOption) => () => Promise<SubtitleTrack[]>` — calls `subtitlesApi.all(animeId, ep.number)`, flattens the `languages` map, filters to the matching track. The flatten logic is extracted from `useSubtitleTracks.ts` into a shared util (single source; the composable reuses it) rather than duplicated.

`SeasonContext` (`seasonDownload.ts`) gains `subPref?` and `resolveSubsFor?: (ep) => () => Promise<SubtitleTrack[]>`; `enqueueSeason` threads both into each `enqueueDownload`.

### 5. Entry-point wiring

**In-player** (`AePlayer.vue`): pass the live capability report, `initialCombo = state.combo.value`, `loadTeams` wrapping `resolver.listTeams`, and `subOptions` computed from `providerBundledTracks` + aggregated tracks (call `ensureSubsLoaded()` on dialog open). `onConfirmDownload(quality, scope, combo, subPref)` uses the **dialog's** combo for `resolve`/`resolveFor` closures (not the player's current combo — the user may have changed it in the dialog).

**Card/season flow** (`seasonDownloadFlow.ts` + `SeasonDownloadHost.vue`): flow state additionally holds `report` and `subOptions` (external options fetched for the first target episode; tolerant of failure). `confirmSeasonDownload(quality, scope, combo, subPref)`: if `combo.provider` differs from the default it resolved with, **re-list episodes via the new provider and recompute `seasonTargets`** (against fresh `listDownloads()` states) before enqueueing — episode lists are provider-specific. Combo (incl. team) is frozen once per batch, as today.

### 6. Offline playback: auto-enable + visibility

- **Auto-enable** (`AePlayer.vue`, offline mode only): when a stream loads for episode N, find the matching `done` record in `props.offline.downloads`; if `record.autoSubUrl` is set, find `stream.subtitles` track with that `url` and select it (the `onSelectSubTrack` path: sets `chosenSub` + `state.subLang`). `subLang` is a session ref (not persisted), so the user's global "subs off by default" preference is untouched. Respect an explicit in-session opt-out: once the user hits «Выкл» during this playback session, later episode loads do not re-enable (a local `userDisabledSubs` flag set in `onSubtitlesOff`).
- **Visibility** (`DownloadsPage.vue`): each episode card gains a meta line — provider · quality · chosen subtitle (label/lang of the `autoSubUrl` track, when set). This makes "what exactly did I download" visible before playback.

### 7. Offline boot → /downloads

`router/index.ts`, a `beforeEach` that acts only on the **initial** navigation (`from === START_LOCATION`):

```ts
if (from === START_LOCATION && !navigator.onLine && offlineDownloadsEnabled && to.path !== '/downloads')
  return { path: '/downloads' }
```

- First navigation only — in-app navigation while offline is never hijacked.
- `navigator.onLine === false` is the trigger; captive-portal/"online but dead" detection is out of scope.
- No downloads / no SW → /downloads renders its own empty/explanatory state, still the most useful offline landing.

### 8. Wi-Fi-only downloads by default

**Detection** — new `src/offline/network.ts`:
- `isCellular(): boolean` — `navigator.connection?.type === 'cellular'`. Unknown/absent type (desktop browsers, Safari) → `false`: never nag users the API can't classify.
- `onConnectionChange(cb)` — subscribes to `connection` `change`; returns a no-op unsubscribe when the API is absent.
- `allowCellularThisSession` flag with getter/setter — session-scoped, NOT persisted: the Wi-Fi-only default returns on every app launch.

**Confirm-time gate** (`DownloadDialog.vue`): when `isCellular()` and the session override is off, pressing «Начать» does not confirm — the dialog swaps its footer into an explicit second step: warning «Вы на мобильных данных. По умолчанию скачивание только по Wi-Fi.» with buttons «Качать по мобильным данным» (sets the session override, then emits confirm) and «Отмена» (back to the normal footer).

**Engine gate + mid-run pause** (`downloadEngine.ts`):
- `OfflineDownload.pausedBy?: 'user' | 'network'` — distinguishes a manual pause (never auto-resumed) from a network pause.
- At the top of `runDownload` (next to the quota re-check, before `resolve()` so no scraper resolution is burned): `isCellular() && !allowCellular` → record goes `paused` with `pausedBy: 'network'`.
- The engine subscribes to connection changes: switching **to** cellular without the override pauses the active download and every queued item (`pausedBy: 'network'`) and surfaces a notice; switching **back to Wi-Fi** auto-resumes exactly the `pausedBy === 'network'` records (user-paused ones stay paused), clearing `pausedBy`.

**Surface** (`DownloadsPage.vue`): when network-paused records exist and the device is on cellular, show a banner «Скачивание приостановлено — ждём Wi-Fi» with «Качать по мобильным данным» (sets the override + resumes network-paused records). A toast fires when the mid-run auto-pause triggers.

## Error handling summary

- Capability fetch failure in card flow → existing `failed` notice (unchanged).
- `/subtitles/all` failure at dialog time → external options absent; at download time → `resolveSubs` catch → episode downloads without external subs, `autoSubUrl` unset.
- `listTeams` failure → team select shows only «Авто».
- Provider switched in dialog + re-list failure at confirm → season flow `failed` notice.

## Testing

- `DownloadDialog.spec.ts`: source rows render from a fixture report; RAW/DUB + lang filtering; provider re-default when filtered out; emits `(quality, scope, combo, subPref)`; subtitle options render bundled + external groups.
- `downloadEngine.spec.ts`: external track cached and `autoSubUrl` set (external pref); external fetch failure → download succeeds, `autoSubUrl` unset; bundled pref matching (concrete lang + `'auto'`).
- `externalSubs.spec.ts`: flatten + provider/lang/label matching.
- `seasonDownloadFlow.spec.ts`: provider changed at confirm → episodes re-listed, targets recomputed; subPref threaded to `enqueueSeason`.
- Router guard: unit test for the extracted guard predicate (offline + initial nav + non-/downloads target → redirect).
- `network.spec.ts`: unknown connection type → not cellular; change subscription no-ops without the API.
- Engine cellular gate: on cellular without override → record `paused`/`pausedBy:'network'` and `resolve()` NOT called; Wi-Fi return auto-resumes only network-paused records; dialog two-step confirm sets the session override.
- Auto-enable: unit-test a `pickAutoSub(record, streamSubs)` helper; AePlayer wiring covered by existing offline-mode component tests where practical.
- i18n en/ru/ja parity for all new keys (`player.aePlayer.offline.*`); DS-lint hook; `/frontend-verify` before finishing.

## Out of scope

- Retroactive subtitle download for existing records (re-download the episode to change subs).
- Multiple simultaneous external tracks per download (single-select matches the player's one-active-track model).
- Auto-enabling subs in ONLINE playback (global "subs default OFF" rule stands everywhere else).
- Captive-portal detection; Background Fetch (still deferred from the base feature).
- A persistent "always allow mobile data" setting — the override is deliberately per-session so the safe default always returns.
