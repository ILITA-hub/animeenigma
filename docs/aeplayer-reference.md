# aePlayer Reference — the unified video player

> **Single source of truth for how the AnimeEnigma video player works.** If you
> are an AI agent (or a new dev) and you are about to touch anything under
> `frontend/web/src/components/player/` or reason about playback, **read this
> first**. The CLAUDE.md "5 video players" table is a historical sketch; this
> document describes the player as it is actually built today.
>
> Last verified against code: 2026-06-29 (`git` HEAD `e69e3e77`).

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
| `PlayerScrubBar.vue` / `ScrubPreview.vue` | Progress track + hover preview. |
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
