# aePlayer Reference — the unified video player

> **Single source of truth for how the AnimeEnigma video player works.** If you
> are an AI agent (or a new dev) and you are about to touch anything under
> `frontend/web/src/components/player/` or reason about playback, **read this
> first**. The CLAUDE.md "5 video players" table is a historical sketch; this
> document describes the player as it is actually built today.
>
> Last verified against code: 2026-07-04 (mobile redesign + season downloads;
> scrub-preview storyboard sprites + SW segment cache, §5.5).

---

## 0. The one thing to unlearn

**There is no longer one component per source.** The old `OurEnglishPlayer.vue`,
`RawPlayer.vue`, `AnimeLibPlayer.vue`, and `HanimePlayer.vue` were **deleted**.
Every source now plays through **one** component:

```
frontend/web/src/components/player/aePlayer/AePlayer.vue   ← THE player (~2800 lines)
```

The only other surviving player surface is `KodikPlayer.vue` — the legacy Kodik
**iframe** mounted as a binary "Classic Kodik" fallback toggle on the anime page
(`views/Anime.vue`), and the surface used if `VITE_AE_PLAYER_ENABLED=false`. It
is NOT part of the aePlayer; it has no shared state with it.

What CLAUDE.md calls "5 players" (Kodik / AniLib / OurEnglish / Hanime / Raw) are
now **provider families inside one player**, selected via the in-player **Source**
panel. An agent that goes looking for `OurEnglishPlayer.vue` will not find it and
will conclude the player is broken. It isn't — it's unified.

---

## 1. Mental model in 60 seconds

The player has exactly two moving parts:

1. **The capability feed** — `GET /api/anime/{id}/capabilities` returns the list
   of providers the backend is *willing to serve for this title*, each with a
   `state`, `group`, `audios`, `order`, `selectable`, `reason`. **This is the
   single source of truth** for what shows up. Disabled providers are **omitted
   entirely** — if it's in the feed, it's real and backend-sanctioned. State is
   derived backend-side from `(policy, health, content)`, **not** from a stale
   `status` column.

2. **The combo** — the user's current selection, a 5-field reactive object:

   ```ts
   interface Combo { audio: AudioKind; lang: TrackLang; provider: string; server: string; team: string | null }
   //                 'sub' | 'dub'    'en'|'ru'|'ja'
   ```

   `audio` + `lang` drive a **relevance filter** over the feed → the Source panel
   renders the matching providers → the user (or the smart default) picks one →
   the matching **adapter** resolves episodes → a stream URL → `<video>`.

Everything else (subtitles, quality, watch tracking, URL sync, Watch Together) is
layered on top of those two.

---

## 2. RAW vs DUB — the single most misread part

The top slider in the Source panel has two positions: **RAW** and **DUB**. They
map onto `combo.audio` with a naming twist that trips up every agent:

| Slider | `combo.audio` | Meaning | Language slider |
|--------|---------------|---------|-----------------|
| **RAW** | `'sub'` | **Original (Japanese) audio.** Includes EN-subbed, RU-subbed, and pure-JP sources. | **Hidden** — lang is *derived* from the chosen provider. |
| **DUB** | `'dub'` | **Localized (dubbed) audio.** | **Shown** — `EN` / `RU` only (there is no Japanese dub). |

> ⚠️ **"RAW" does not mean "unprocessed video".** It means *original voice track*.
> And `combo.audio: 'sub'` **is** the RAW position — the internal `AudioKind` is
> historical (`'sub'` = "watch with original audio", `'dub'` = "watch dubbed").
> Do not assume `audio:'sub'` means "subtitles are on" — subtitles are a
> completely separate axis (see §7) and default OFF.

Consequences baked into the code:

- **RAW drops the language filter.** Under RAW, `relevant()` matches any provider
  with `sub` *or* `raw` caps regardless of `lang`, so EN-sub / RU-sub / pure-JP
  sources all surface together (`useProviderFeed.ts:relevant()`). This is why a
  JP-only title is no longer empty under RAW.
- **Under RAW, `combo.lang` is derived, not chosen.** When the user picks a RAW
  provider, `langForProviderUnderRaw(group, currentLang)` sets the served lang to
  the provider's group language (`providerGroups.ts`). It's applied via
  `setServedLang()`, which—unlike `setLang()`—**does not reset `team`**
  (`usePlayerState.ts`, watcher in `AePlayer.vue` ~L913).
- **There is no Japanese dub.** `clampLangForAudio('dub','ja') → 'en'`
  (`comboMapping.ts`). Applied on every audio/lang entry point (slider, saved
  combo, URL facet) so switching DUB while carrying a `ja` lang clamps to EN.

---

## 3. Provider groups (`providerGroups.ts`)

Each provider has a backend-assigned `group`. The group → served-facets mapping is
the **only** provider knowledge the FE still hardcodes (the rest comes from the
feed). One table, three consumers (relevance filter, deep-link clamp, served-lang
derivation):

```ts
GROUP_LANGS    = { en:['en'], ru:['ru'], adult:['en','ru'], jp:['ja'], firstparty:['en','ru','ja'] }
GROUP_CONTENT  = { en:['common'], ru:['common'], adult:['hentai'], jp:['common'], firstparty:['common'] }
GROUP_PRIMARY_LANG = { en:'en', ru:'ru', adult:'en', jp:'ja', firstparty:'ja' }   // RAW fallback lang
```

| group | who | serves langs | content |
|-------|-----|--------------|---------|
| `en` | EN scraper chain (gogoanime, allanime, miruro, …) | EN | common |
| `ru` | Kodik | RU | common |
| `adult` | 18anime / Hanime | EN, RU | hentai |
| `jp` | Raw (AllAnime raw) | JA | common |
| `firstparty` | `ae` (self-hosted) | EN, RU, JA | common |

To **add a provider to a group**, you change the backend `group` it emits — and if
it's a brand-new group, you add one row to each of the three maps above. Adding it
to the wrong group silently breaks language filtering.

---

## 4. The capability feed (backend single source of truth)

`GET /api/anime/{uuid}/capabilities` → `CapabilityReport` (`types/capabilities.ts`,
mirrors Go `domain.CapabilityReport`). Consumed by `useCapabilities.ts`.

```ts
ProviderCap { provider, display_name, state, selectable, hacker_only, order, group, audios, reason, variants, ... }
```

**`ChipState`** (`'active' | 'recovering' | 'degraded' | 'no_content'`) — the real
set. (Older notes saying `disabled/down/irrelevant/wip` are wrong.)

| state | meaning | selectable? |
|-------|---------|-------------|
| `active` | in the auto-failover chain; auto-eligible, ranked by `order` | yes |
| `recovering` | auto policy, health healing; surfaces in **hacker mode only** (normal list renders `active` only) | hacker-only |
| `degraded` | pinned out of the auto chain (policy=manual); `hacker_only` | hacker-only |
| `no_content` | no episodes for this title (e.g. `ae` before encoding) | never |

Key invariants:
- **Disabled providers are omitted from the feed.** Don't look for a "disabled"
  state — it isn't sent.
- State is **derived live** from `(policy, health, content)`. See
  [`docs/scraper-health-reference.md`](scraper-health-reference.md) and the
  policy/health self-healing memory. The legacy `status` column is NOT authority.
- `audios` is `('sub'|'dub'|'raw')[]`. The FE collapses `'raw' → 'sub'` when
  building rows (`useProviderFeed.ts:toRow()`), so a raw-only provider is a
  selectable RAW row.

---

## 5. Component & composable map

### Components (`components/player/aePlayer/`)
| File | Role |
|------|------|
| `AePlayer.vue` | Orchestrator. Owns `<video>`, wires every composable, runs the default-selection + URL-sync logic, renders overlays/menus. |
| `PlayerControlBar.vue` | Play/pause, ±5s, volume, Source pill, CC, gear, PiP, fullscreen. |
| `PlayerScrubBar.vue` / `ScrubPreview.vue` | Progress track + hover preview (§5.5). |
| `SourcePanel.vue` | RAW/DUB slider, language slider (DUB), team chips, **provider list (top-3 + hacker)**, server list. The combo-selection UI. |
| `ProviderChip.vue` | One provider row: state tint, hue dot, reason tooltip, BEST badge. |
| `PlaybackSettingsMenu.vue` | Quality (Auto only), speed, autoplay-next (off), auto-skip-intro (off). |
| `SubtitlesMenu.vue` / `BrowseSubsModal.vue` | CC menu + "browse all subtitles" (Jimaku/OpenSubtitles). |
| `EpisodesPanel.vue` | Episode list, resume pill, airing schedule. |
| `overlays/` | `BigPlayButton`, `SkipIntroChip`, `NextEpisodeCard`, `WatchTogetherButton`, `BufferingOverlay`, `DebugHud`. |

### Composables (`composables/aePlayer/`)
| File | Owns |
|------|------|
| `usePlayerState.ts` | Central reactive state: `playing/progress/volume/muted/quality/speed/autoNext/autoSkip`, **`combo`**, `subLang/subSize/subBg/subOffset`, `hackerMode/hudPinned`. **All mutations go through named setters.** Default combo `{audio:'sub', lang:'en', provider:'', server:'', team:null}`. |
| `useCapabilities.ts` | Fetches/polls the capability feed → `report`, `capMap`. |
| `useProviderFeed.ts` | `rowsFromReport()` + `relevant()` — flatten feed → relevance-filtered, order-sorted `ProviderRow[]`. |
| `providerGroups.ts` | The group → facet tables (§3). |
| `smartDefault.ts` | `pickSmartDefault` / `pickRawBiased` / `pickSelectableFallback`. |
| `deepLinkProvider.ts` | `resolveDeepLinkProvider()` — clamp a `?provider=` to a pinnable row. |
| `comboMapping.ts` | `Combo` ↔ legacy `WatchCombo` (persistence) ↔ WT token; `clampLangForAudio`, `providerToLegacyPlayer`. |
| `useProviderResolver.ts` | The adapter seam — per-provider episode/server/stream resolution (scraper EN, raw JP, kodik, anime18, ae, hanime). Normalizes to `EpisodeOption`/`StreamResult`. |
| `useVideoEngine.ts` | Wraps `<video>` + lazy `hls.js` (pinned `~1.5.20`). Knows nothing about providers. |
| `useSubtitleTracks.ts` / `pickDefaultSubtitle.ts` | Aggregate + pick subtitle tracks (default OFF). |
| `useWatchTracking.ts` | Emits `watch_history` / `watch_progress`, room sync. |
| `episodeSelection.ts` / `episodeProgress.ts` | Watching/finished/not-yet-aired episode state. |
| `playerHotkeys.ts` | Key → action map. |

---

## 5.5 Scrub-bar hover preview (`ScrubPreview.vue`)

Hovering the scrub track renders a thumbnail bubble above it. There are
**three preview sources**, tried in this priority order:

1. **Storyboard sprite mode** — library ("ae" first-party) episodes ship a
   pre-baked WebVTT thumbnail track. When `StreamResult.storyboardUrl` is set
   and the VTT parses, the component draws sprite crops out of a handful of
   JPEG sheets and **never boots the shadow `hls.js` engine** — zero per-hover
   proxy egress.
2. **Shadow engine + SW segment cache** — no storyboard, but the PWA service
   worker is registered: the shadow engine's own HLS segment fetches are
   tagged `aescrub=1` and served cache-first from `ae-seg-v1` once warm.
3. **Plain shadow engine** — no storyboard, no SW (killed, unsupported, or not
   yet registered): the pre-existing live-seek-and-capture behavior, unchanged.

Every fallback is silent: a broken storyboard VTT, a missing SW, or a cache
miss never surfaces an error — the component just drops to the next source.

**The "shadow engine"** (both sources #2 and #3) is a hidden, muted
`<video>` (`shadowRef`, `ScrubPreview.vue`) that seeks independently of the
main player and captures every decoded frame into a canvas cache keyed by a
5s time bucket (LRU, `CACHE_MAX = 150` entries). A hover renders the nearest
cached thumbnail **instantly** (no network); the shadow video only issues a
real seek once the pointer *settles* (`SETTLE_MS = 180`), and a background
pump prefetches a handful of evenly-spaced timeline points so the whole bar
has distinct frames within seconds of the first hover. This is unrelated to —
and does not reuse — `useVideoEngine.ts` (that composable is the main
player's own `<video>`/hls.js wrapper); the shadow engine constructs and owns
a second, independent `hls.js` instance pinned to `currentLevel = 0`
(lowest quality — a 192×108 bubble needs no ABR). `scrubPreviewDebug.ts`
mirrors engine/cache/queue health into `DebugHud.vue`'s **PREVIEW** row
(hacker mode) for exactly this reason: "stale bubble" has two unrelated
causes (frontend pump wedged vs. provider fragment latency) that look
identical on screen.

### Prop chain

`storyboardUrl` hops down four components before it reaches the renderer:

```
AePlayer.vue          :preview-storyboard-url="currentStream?.storyboardUrl ?? null"
  → PlayerControlBar.vue   previewStoryboardUrl prop → re-emitted :preview-storyboard-url
    → PlayerScrubBar.vue     previewStoryboardUrl prop → passed as :storyboard-url
      → ScrubPreview.vue       storyboardUrl prop
```

`currentStream` is the resolved `StreamResult` (`types/aePlayer.ts`). Only the
**ae adapter** (`makeAeAdapter`, `useProviderResolver.ts`) ever populates
`storyboardUrl` — every other adapter (scraper/EN, kodik, anime18/hanime,
animejoy) leaves it `undefined`, which correctly routes those sources to
preview source #2/#3 above. "Library content only" in practice means
first-party `ae` episodes; Raw (JP) plays through the scraper chain
(allanime/okru) today, not the library, so it never carries a storyboard
either (see the `raw` provider removal in memory/CLAUDE.md history).

### 1. Storyboard sprite mode — geometry, generation, signing

**Geometry is LOCKED** (`services/library/internal/ffmpeg/storyboard.go`):
5s cadence (`StoryboardCadenceSec`), 10×10 sheets of 160×90 JPEG tiles
(`StoryboardCols`/`Rows`/`TileW`/`TileH`, `-q:v 8` — low quality is
deliberate, this is a preview-only asset), file names `storyboard_NNN.jpg`
(1-based, `sort.Strings` order) + `storyboard.vtt`
(`StoryboardVTTName`), written under the episode's **existing** MinIO prefix
alongside the HLS output (`{prefix}storyboard_NNN.jpg`, `{prefix}storyboard.vtt`
— `minio/writer.go:UploadStoryboard`). Changing any of these constants means
regenerating every stored storyboard.

**Generation has two paths:**
- `encoder_worker.go`'s step 8b runs the storyboard ffmpeg pass **after** the
  HLS upload succeeds (a storyboard failure never blocks playable output) and
  **before** the `library_episodes` insert (so `HasStoryboard` is known at
  insert time, avoiding a second UPDATE). Skipped when `job.ShikimoriID == ""`
  — pending/anonymous uploads have no episode row to flag. Any ffmpeg or
  upload failure is logged and swallowed; the episode ships without a
  preview, never blocked on it.
- `storyboard_backfill.go` catches up episodes ingested before the pass
  existed: pulls a batch of `backfillBatchSize = 50` storyboard-less rows
  (oldest first) per cycle, processes the first one not in failure-backoff, a
  per-episode failure enters a `backfillCooldown = 6h` in-memory cooldown so
  one persistently-broken row can't starve the rest of the batch (the
  starvation guard). Gated on the same disk-guard threshold the
  download/encode admit path uses; `STORYBOARD_BACKFILL_ENABLED` (default
  `true`) is the off switch, `STORYBOARD_BACKFILL_PAUSE_SEC` (default `60`,
  `NewStoryboardBackfill`) the per-episode pause. Deliberately the
  lowest-priority workload on the host — a 500-episode library backfills over
  days, not minutes.
- `has_storyboard BOOLEAN NOT NULL DEFAULT FALSE` (migration
  `015_storyboard.sql`) is what both paths flip; it drives `HasStoryboard` in
  the episode list/get handlers (`handler/episodes.go`) and is what makes
  `GetAeStream` attach a `storyboard_url`/`RawStream.Storyboard` at all.

**Signing chain:** catalog's `RawResolver.GetLibraryStream`
(`raw_resolver.go`, backing `GET /api/anime/{id}/ae/stream`) signs the
storyboard URL exactly like it signs the playlist URL (`streamsign.Sign`) and
attaches it as `RawStream.Storyboard {url, exp, sig}` (`newLibraryStream`).
The signed URL then passes through the HLS proxy like any other library
asset: `libs/videoutils/proxy.go` detects the WebVTT response
(Content-Type contains `vtt`, or a `.vtt` path suffix) and runs
`rewriteVTTURLs` instead of the plain passthrough — it rewrites each
`storyboard_NNN.jpg#xywh=x,y,w,h` cue payload into a signed
`/api/streaming/hls-proxy` URL, the same treatment `rewriteM3U8URLs` gives
playlist children (one correlation token minted per manifest, AR-EGRESS-04).
Only image-cue lines match (the `vttImageCue` regex); `WEBVTT`/`NOTE`/timing
lines and non-image payloads (real subtitle text, on a different proxy path)
are left untouched.

**Parser contract (`storyboardVtt.ts`):** `parseStoryboardVtt` accepts only
absolute or root-relative cue URLs (`^(?:https?:)?\/\/` or a leading `/`) — a
bare-relative cue (the raw `storyboard_003.jpg#xywh=…` the ffmpeg pass writes,
before the proxy rewrite runs) means the rewrite didn't happen, and the cue is
silently **skipped** rather than resolved client-side, which could never
produce a fetchable *signed* URL anyway. Malformed cues degrade the same way
— no throw, just fewer cues; an empty cue list falls straight through to the
shadow engine (`storyCues = cues.length > 0 ? cues : null` in
`ScrubPreview.vue`'s `loadStoryboard`). A `storyGen` generation token (mirrors
the shadow engine's own `initToken`) guards a slow in-flight VTT fetch from
clobbering a newer stream's cues if the episode changes mid-load.

### 2. The SW segment cache (`src/pwa/segmentCache.ts`)

Cache name `ae-seg-v1`, registered as a `workbox-routing` route in `src/sw.ts`
alongside the offline-download and RU-edge-fallback routes. Reliability
contract (owner directive 2026-07-04):

| Property | Value |
|---|---|
| Cache key | the upstream `url=` query param only (`segmentCacheKey`) — `exp`/`sig`/`sess` rotate on every playlist rewrite and must NOT fragment the cache |
| Eligible requests | `/api/streaming/hls-proxy` responses whose upstream path ends `.ts` or `.m4s` (`SEG_EXT`); `type=mp4` and any request carrying a `Range` header are excluded outright |
| Serve policy | cache-first **only** for `aescrub=1`-marked requests (`markScrubUrl`, set by the shadow engine's `xhrSetup`); unmarked requests — i.e. the **main player** — always hit the network |
| Write policy | every 200 response, marked or not, is teed into the cache in the background — the main player's own segment fetches feed the cache too, so a re-hover of a spot the main player already buffered can still hit |
| Concurrency | `MAX_INFLIGHT_TEES = 4`; past the cap, the tee is skipped and the response is still returned normally |
| Bounds | `SEG_MAX_ENTRIES = 150` FIFO (Cache API preserves insertion order, so the front of `cache.keys()` is oldest), `SEG_TTL_MS` = 3h (checked on read, expired entries deleted lazily) |
| Storage guard | writes are dropped outright when `navigator.storage.estimate()` is unavailable, or headroom (`quota - usage`) is below `MIN_HEADROOM_BYTES` = 1 GB |
| Failure mode | a write **never** blocks or fails the response — `event.waitUntil(writeSegment(...).catch(() => {}))` swallows every error |

**Kill switch:** `registerPwa.ts`'s `shouldPurgeCacheKey` matches
`ae-seg-*` (alongside the `workbox-*` app-shell precache) — when
`/sw-config.json` reports `kill: true`, `unregisterAll()` unregisters the SW
**and** purges the segment cache. `ae-offline-*` (user-downloaded episodes)
deliberately does NOT match: the kill-switch retires a broken SW, it must
never destroy user data.

### Coverage caveats (read before assuming "previews work everywhere")

- **Ranged segments bypass the cache entirely.** `handleSegmentRequest`
  treats any request carrying a `Range` header as uncacheable — a cached full
  200 response would corrupt a byte-range read. EXT-X-BYTERANGE streams
  (multiple segments sharing one URL across ranges) always fall through to
  plain network.
- **Extension-less path-style segment CDNs are never keyed.**
  `segmentCacheKey` requires the upstream path to end `.ts`/`.m4s`; CDNs that
  serve path-style segments with no extension (the same okcdn quirk the
  proxy already special-cases for M3U8 playlists without a `.m3u8` suffix —
  see the comment at `proxy.go:739-745`) never get a cache key and silently
  keep today's uncached behavior. Nothing breaks; those requests just never
  benefit from the tee.
- **MP4-progressive providers are excluded.** `segmentCacheKey` returns
  `null` outright when `type=mp4` — there's no HLS segment concept to key on,
  and a Range request would bypass it regardless.
- **Safari is tee-only.** The shadow engine's Safari fallback is
  `v.src = streamUrl` (native HLS — `Hls.isSupported()` is false) with no
  `xhrSetup` hook, so Safari's shadow-engine fetches are never marked
  `aescrub=1` and are never served from cache. They're still **teed** into
  `ae-seg-v1` if the URL shape matches (any client's segment fetches are), so
  Safari indirectly warms the cache for others — it just never reads from it
  itself.
- **Privacy.** The SW cache persists actual video segment bytes to disk —
  including from 18+ (Hanime) sources, since the cache keys on URL shape, not
  content rating — for up to 3h or 150 entries, where nothing persisted
  client-side before this feature. The FIFO/TTL bounds and the
  `sw-config.json` kill switch are the only mitigations; there is no
  per-title or per-rating opt-out.

---

## 6. Default selection & precedence

On open, the player resolves a combo in this order (highest wins):

```
URL facet/provider   >   saved watch preference   >   smart default
```

Flow (`AePlayer.vue`, watcher on `rows` ~L862):
1. `buildAvailable()` enumerates **every** real source's `(player, audio, lang)`
   facet across **all** families (not just the currently-filtered rows) so a saved
   `ru/dub/ja` combo can match. (Iterating only the filtered `rows` was the bug
   that collapsed everything to SUB-EN.)
2. `resolvePreference(available)` restores the saved audio/lang/team
   (`applyResolvedCombo`).
3. `applyUrlFacet()` overrides audio/lang from `?audio=`/`?lang=`.
4. `applyInitialProvider()` consumes `?provider=`/`?team=`, **clamping** audio/lang
   so the deep-linked row becomes relevant (`resolveDeepLinkProvider`).
5. `preferenceSettled` flips true → a second watcher picks the provider via
   `pickFacetDefault()`: `pickRawBiased` under RAW (don't cross language),
   `pickSmartDefault` under DUB, then `pickSelectableFallback` so a fully-degraded
   fleet still attempts playback instead of dead-ending.

**Top-3 padding (`SourcePanel.vue`).** The collapsed Source list shows up to
`TOP_N = 3` providers: the top `active` rows, **padded** with the next-best
non-active (degraded/recovering) rows if fewer than 3 are active, plus the selected
row always pinned in. Those padded rows are made **selectable without hacker mode**
(`forcedSelectableIds`) so the panel is never a dead end. This is **FE-side only**
— the backend feed is unchanged, so don't be alarmed that the feed lists 1 active
provider while the panel offers 3.

---

## 7. Subtitles — OFF by default, on purpose

`subLang` defaults to `'off'` and **the player NEVER auto-enables a subtitle
track.** This is intentional design, not a bug — do not "fix" it by auto-selecting.

- **Hard vs soft.** EN/RU SUB streams from providers usually have subtitles
  **burned into the video** (hardsub) — there is no selectable track, and the CC
  menu shows the `hardsubNote` ("subs are baked into this source"). A raw JP cut is
  NOT hardsubbed; its subs come from the optional Jimaku/OpenSubtitles overlay.
- **Soft tracks.** When a provider ships a soft track, or the user browses
  Jimaku/OpenSubtitles (`OtherSubsPanel.vue` / `BrowseSubsModal.vue`), the user
  picks one → `SubtitleOverlay.vue` renders it (custom ASS/SRT/VTT renderer,
  teleported into the fullscreen element, RAF-synced).
- **Persistence.** Once the user opts in, the *language* choice persists across
  episodes; the track URL is episode-specific so it's dropped on episode change and
  re-bound via `pickBestForLang(tracks, subLang)` for the new episode
  (`AePlayer.vue` ~L2240).
- `subLang` is a free `string` (not `TrackLang`) — browsed tracks can be any
  language. `subLang` is **independent of `combo.lang`** (EN dub + JP subs is
  valid).
- There is deliberately **no** `pickAutoSubtitle` helper. It was removed; only
  `pickBestForLang` (exact-language, no cross-lang fallback) remains.

---

## 8. URL / deep-link sync

Query params on the anime page (`views/Anime.vue`, read one-shot at mount into the
`:initial-*` props): `?provider=` `?team=` `?audio=` `?lang=` `?episode=`.

- **Read:** `resolveDeepLinkProvider` honors `?provider=` only when it names a
  provider present in the feed (disabled are omitted), content-compatible, serving
  ≥1 audio kind; otherwise it falls through to the smart default (bad params are
  silently ignored). `?audio=raw|sub` → `audio:'sub'`; `?audio=dub` → `'dub'`.
- **Write-back:** `AePlayer` emits `url-sync` → `onUrlSync` mirrors it to the URL,
  but **only for a user-pinned source** (`providerAutoSelected === false`). An
  auto/smart-default pick emits empty provider/team so a plain reload re-runs the
  deterministic BEST default (product rule: a previously-watched source must not
  silently override the best one). Gated on `preferenceSettled`. Suppressed inside
  a Watch-Together room.

---

## 9. Adapters & stream resolution (`useProviderResolver.ts`)

The resolver is the seam between the unified player and each source's API. Each
adapter implements get-episodes / get-servers / get-stream and returns normalized
`EpisodeOption` / `StreamResult`. Wired adapters: **scraper (EN chain), raw (JP),
kodik (RU), anime18 / hanime (18+), ae (first-party)**. `animelib` is NOT wired
(deprecated).

The EN scraper adapter talks to the backend route family:
```
GET /api/anime/{uuid}/scraper/episodes?prefer=<provider>&exclusive=<bool>
GET /api/anime/{uuid}/scraper/servers?episode=<id>&prefer=<provider>
GET /api/anime/{uuid}/scraper/stream?episode=<id>&server=<id>&category=sub|dub&prefer=<provider>
```
`prefer` soft-pins (move to front); `exclusive=true` restricts to that provider.
Backend failover order and provider mechanics:
[`docs/scraper-framework.md`](scraper-framework.md).

> **No automatic failover *inside* the player.** The backend scraper chain fails
> over between providers, but once a stream resolution fails for the *selected*
> provider the player surfaces an error / "switching to the next best…" toast and
> may try the next provider — it does NOT silently re-route forever. The user can
> switch providers manually in the Source panel.

Stream URLs are wrapped through the backend **HLS proxy** and **signed**
(`videoutils.SignStreamURL`); scraper CDN hosts are auto-trusted via signing and
do **not** need an allowlist entry (see CLAUDE.md proxy section).

**Known issue — HLS codec stall (D-07):** for some HLS sources hls.js loads the
master + level playlist but never requests `.ts` fragments (readyState stays 0).
Pre-existing; tracked in `aePlayer/MANUAL-REVIEW.md`. hls.js is pinned to
`~1.5.20` (NOT caret) because 1.6.x regressed codec handling.

---

## 10. Watch tracking & persistence (`useWatchTracking.ts`)

On play/pause/seek/episode-change the player writes `watch_history` +
`watch_progress`. The combo is persisted as a legacy `WatchCombo`
(`comboToWatchCombo`): `player` is the **coarse** family (`english`/`kodik`/`raw`/
`ae`/`hanime`) — all EN scraper providers collapse to `english` — plus
`language`, `watch_type` (sub/dub), `translation_title` (team). On the next open
this restores audio/lang/team (the exact provider id is re-chosen by the smart
default, since it's not derivable from the coarse player). See
[watch-preferences strict fallback] in memory for the restore rules.

---

## 11. Watch Together

Inside a WT room the room's combo is **authoritative** (`roomHasCombo` suppresses
the saved-combo restore and URL write-back). The 5 combo fields are carried
opaquely in the room's `translation_id` via `comboToToken` / `tokenToCombo` so
every member resolves the identical stream. Full WT architecture:
[`docs/watch-together-reference.md`](watch-together-reference.md).

---

## 12. Misc behaviors

- **Quality:** `PlaybackSettingsMenu` exposes **Auto only** today — the scraper
  doesn't surface a quality ladder and hls.js auto-selects by bandwidth. A
  per-URL `qualityLabel` (e.g. Kodik) is shown next to Auto when present. Per-URL
  ladders / manual quality UI are deferred.
- **Hacker mode:** localStorage `pl_hacker_mode='1'` → `DebugHud.vue` (per-fragment
  HLS stats, bandwidth, level, seek trace, source-decision/fallback log) and the
  full unfiltered provider list. **Dev/troubleshooting tool only** — not a feature
  flag, not a provider override. `pl_hud_pin='1'` keeps the HUD on screen.
- **Autoplay-next / auto-skip-intro:** both default OFF. Skip-intro chip only
  shows if real intro/outro timings exist (no backend yet → effectively hidden).

---

## 12.5 Mobile (≤680px / coarse pointer) — redesign 2026-07-04

`useMobilePlayer.ts` provides reactive `isMobile` (`max-width: 680px`, the
player CSS breakpoint) and `isCoarse` (`pointer: coarse`, gates gestures).

- **Template root is `.pl-wrap`** (carries `data-test="ae-player"`); the video
  box `.pl` is its first child and keeps `rootRef`, hotkeys, theater/pseudo-FS
  classes. Below it a mobile-only **action row** (`.pl-actions`): `[Эп N ▾]
  [Источник ▾] [⬇ Скачать]` — hidden per-button under `offline` /
  `!canDownload`.
- **Fullscreen is capability-based** (`onToggleFullscreen`): native element FS
  (+ best-effort `screen.orientation.lock('landscape')` on coarse) where
  available; on iPhone (no element-FS API) a **pseudo-FS** takeover
  (`pl--pseudo-fs`, fixed inset-0 z-100, `html.pl-noscroll`). Pseudo-FS pushes
  a MERGED history state (`{...history.state, plPseudoFs:true}`) so the back
  gesture exits it; exit correctness rides an instance-local
  `pseudoFsEntryPushed` flag because url-sync's `router.replace` clobbers
  `history.state`. Never use `video.webkitEnterFullscreen()` — it drops
  SubtitleOverlay/SourcePanel/WT.
- **Menus are bottom sheets on mobile**: the four floating menus +
  BrowseSubsModal + DownloadDialog wrap in `<Teleport to="body"
  :disabled="!sheetTeleport">` where `sheetTeleport = isMobile &&
  !nativeFsActive` (body children are invisible under a native-FS element).
  Sheet class `.pl-floating--mobile-sheet` z-110 over a z-105 scrim (both above
  pseudo-FS z-100); `--prov` is re-bound on each wrapper (CSS vars don't
  cascade to body).
- **Gestures (coarse only)**: single tap on video toggles chrome (never
  play/pause — the center 64px pause overlay is the play/pause affordance
  while playing); double-tap side thirds = ±10s with a flash pill; desktop
  click path is byte-identical.
- **Control bar mobile trim**: ±5s/PiP/episodes-pill/volume-slider hidden;
  fullscreen ALWAYS visible (hiding it was the original bug); source pill =
  dot+chevron; 44px hit targets on coarse.
- **Full-bleed**: `views/Anime.vue` adds scoped `.player-card` (class+attr
  specificity — the only reliable way to beat unlayered `.glass-card` under
  Tailwind v4) killing card gutters at ≤680px; `.pl` drops radius/side borders.
- **Season downloads**: DownloadDialog v2 has an episode/season scope selector
  (+ storage headroom warning); season path recomputes `seasonTargets()` at
  confirm time and `enqueueSeason()` serially feeds the engine (which re-checks
  quota headroom at the start of every `runDownload`). Season chip also lives
  in the EpisodesPanel header.

---

## 13. Gotchas checklist (read before editing the player)

1. **One player.** `AePlayer.vue` is it. `OurEnglishPlayer/RawPlayer/AnimeLibPlayer/
   HanimePlayer` are deleted. `KodikPlayer.vue` is only the separate iframe fallback.
2. **`audio:'sub'` = RAW = original audio**, not "subtitles on". `audio:'dub'` = DUB.
3. **Subtitles default OFF and never auto-enable** — intentional. No `pickAutoSubtitle`.
4. **The capability feed is the source of truth.** Disabled providers are omitted;
   state is `(policy,health,content)`-derived, not the `status` column.
5. **`ChipState` = `active|recovering|degraded|no_content`.** Nothing else.
6. **Top-3 padding is FE-side.** The panel can offer more selectable rows than the
   feed marks `active`.
7. **Under RAW the lang filter is dropped** and `combo.lang` is **derived** from the
   provider's group via `setServedLang` (preserves team).
8. **No Japanese dub** — `clampLangForAudio` forces `dub/ja → dub/en`.
9. **URL write-back only for user-pinned sources** — auto picks emit empty so reload
   re-runs BEST.
10. **`hls.js` pinned `~1.5.20`** — do not bump to 1.6.x.
11. **All combo mutations go through the `usePlayerState` setters** — never mutate
    `combo.value` fields directly.
12. **Don't add scraper CDNs to the proxy allowlist** — signing auto-trusts them.

---

## 14. Related references

- [`docs/scraper-framework.md`](scraper-framework.md) — backend EN scraper: failover chain, `prefer`/`exclusive`, provider registration.
- [`docs/scraper-health-reference.md`](scraper-health-reference.md) — provider policy/health authority that derives the capability feed state.
- [`docs/watch-together-reference.md`](watch-together-reference.md) — co-watch rooms.
- `docs/superpowers/specs/2026-06-06-unified-anime-player-design.md` — the original locked design contract (pre-RAW/DUB; superseded by this doc for current behavior).
- `frontend/web/src/components/player/aePlayer/MANUAL-REVIEW.md` — in-browser verification checklist + known-issue log.
- CLAUDE.md → *Video Player Architecture* — high-level summary (points here for detail).
