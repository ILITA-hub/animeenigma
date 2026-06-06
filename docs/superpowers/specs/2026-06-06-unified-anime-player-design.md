# Unified Anime Player ("Neon Tokyo") — Design Spec

**Date:** 2026-06-06
**Status:** Approved for planning
**Source design:** Claude Design handoff bundle `animeenigma-design-system` →
`ui_kits/web/player.html` (+ `Player.jsx`, `WatchPage.jsx`, `player.css`, and the
9-round design chat `chats/chat1.md`). The prototype is a cosmetic React/Babel
recreation; this spec adapts it to the production Vue 3 codebase.

---

## 1. Goal & Motivation

Today the watch experience leans on **third-party and default skins**: the HTML5
players (`OurEnglishPlayer`, `AnimeLibPlayer`, `HanimePlayer`, `RawPlayer`,
`Anime18Player`) render native `<video controls>`, and Kodik is a third-party
iframe. The result feels generic and inconsistent across sources.

We are building **one unified, fully-branded "Neon Tokyo" player** — a single Vue
component that owns its own `<video>` + `hls.js` core and a custom control bar,
designed so every source eventually looks and behaves identically.

### Architecture target vs. stage-1 rollout

- **Target architecture (eventual):** ONE unified shell where **providers are
  swappable stream-backend adapters**. The player's own internal Source &
  translation panel (Audio × Language → Provider → Server → Team) replaces the
  external `RU EN 18+ RAW JP` language/provider tabs entirely.
- **Stage 1 (this spec):** Add the unified player **non-destructively** as a
  **new pill in the top-level `RU EN 18+ RAW JP` language selector** in
  `Anime.vue`, alongside the existing players. Nothing old is removed or
  repointed. We test it live, then a later stage removes the old tabs and
  repoints all combo logic into the player.

**Out of scope for stage 1** (explicitly deferred):
- Removing the old language/provider tabs or repointing combo logic.
- Live Watch Together wiring (the `party` prop).
- The real AnimeEnigma/SVO self-hosted backend.
- A real intro/outro skip-timings backend (UI ships; data is a later TODO).

---

## 2. Build Philosophy

**Design-system-first**, architected for **clear separation of concerns,
predictable state management, reusable components, and performance by design.**

`UnifiedPlayer.vue` is a **thin orchestrator**, never a monolith. The video
engine, the source backends, the player state, and the UI are **four
independently-replaceable pieces** — so "later, repoint all combo logic to it"
becomes wiring adapters + widening state scope, not a rewrite.

---

## 3. Architecture

### 3.A State & logic — composables (reactive, testable, no DOM)

| Composable | Responsibility | Notes |
|---|---|---|
| `usePlayerState()` | Single reactive source of truth: `playing, progress, volume, muted, quality, speed, combo {audio, lang, provider, server, team}, subtitle prefs {lang, size, bg, offset, chosenTrack}`. All mutations via named actions. | Stage-1 local to the instance. Isolated from the view so a later stage can swap it for a shared store when Watch Together needs cross-member state. |
| `useVideoEngine(videoEl)` | Wraps `<video>` + **lazy-imported `hls.js`**: attach / load source / destroy, level list, buffered ranges, fatal-error → recovery. | Knows nothing about providers or UI. `hls.js` pinned `~1.5.20` (NOT caret — see hls.js 1.6.x codec regression). Native HLS (Safari) path respected. |
| `useProviderResolver()` | The **"providers = stream backends" seam**. A registry of adapters; each implements `resolve(animeId, episode, combo) → StreamResult { url, headers, type: 'hls'\|'mp4', qualities, audios, servers }`. | Stage-1 adapters listed in §4. Adapters reuse the *existing* per-player resolution endpoints — no new stream-resolution backend. |
| `useProviderHealth(animeCtx)` | Polls the merged registry+health endpoint; returns provider rows with computed **chip-state** (`active \| disabled \| down \| irrelevant \| wip`) + reason text. Polled on an interval with caching so it "updates automatically." | Merges `scraper-providers.yaml` metadata (`Enabled/Reason/Description`) with the live `/scraper/health` snapshot (stage `up`). |

### 3.B Presentational components (reusable, props-in / events-out)

New, under `components/player/unified/`:
- `UnifiedPlayer.vue` — orchestrator: owns the `<video>`, wires composables, lays out overlays + control bar.
- `PlayerControlBar.vue` — play/pause, ±10s, hover-volume, Source pill, Subtitles, Settings, PiP, theater, fullscreen.
- `PlayerScrubBar.vue` — track with buffered + hover-preview thumb + intro/outro chapter markers.
- `SourcePanel.vue` — Audio + Language sliders → Team → Provider list → Server list.
- `ProviderChip.vue` — one provider chip rendering its health state + hover tooltip (reason/description).
- `PlaybackSettingsMenu.vue` — gear: Quality, Speed, Autoplay/Auto-skip toggles.
- `SubtitlesMenu.vue` — subtitle language list + entry to settings + Browse-all.
- `BrowseSubsModal.vue` — search bar + provider/language filter chips, tracks grouped by language.
- Overlays: `SkipIntroChip.vue`, `NextEpisodeCard.vue`, `BigPlayButton.vue`.
- `WatchTogetherButton.vue` — **WIP stub** (visible icon, disabled, "WIP" label + tooltip). No panel, no `party` prop in stage 1.

**Reuse existing (do not rebuild):** `SubtitleOverlay.vue`,
`SubtitleSettingsMenu.vue`, `OtherSubsPanel.vue` (powers Browse-all data path),
`ResumePill.vue`, `EpisodeSelector.vue`, and `ui/` primitives (Button, Switch,
Modal, Tooltip, Popover, Tabs).

### 3.C Styling — design-system-first

Semantic tokens only; bind, never hardcode. Provider identity hues use the
**exempt brand hues** (cyan = Kodik/AnimeEnigma, orange = AniLib, pink = Hanime,
rose = Raw/18anime). Must pass `frontend/web/scripts/design-system-lint.sh`.
In-browser smoke at desktop + mobile per DS-NF-06 (jsdom can't catch Tailwind-v4
cascade bugs). Read `frontend/web/src/styles/DESIGN-SYSTEM.md` before styling.

### 3.D Performance by design

- Lazy dynamic `import('hls.js')` — keep it off the initial bundle.
- `requestAnimationFrame`-throttled time/progress updates — **no full-tree
  re-render per tick** (the prototype's restart-animation/`opacity:0` overlay bug
  is designed out: overlays render at base opacity, no per-tick remount).
- Health polled on an interval with caching; pause polling when tab hidden.
- Overlays mounted only when active (`v-if`, not `v-show` + opacity).
- Scrub hover-preview is a single positioned node updated on `mousemove`.

---

## 4. Provider Model & Health

### 4.A Source panel = flat real-provider list

The Source & translation panel lists the **real backend providers** from the
health registry, **filtered by Audio (Sub/Dub) × Language × content-type**:

| Group | Providers | Stage-1 status |
|---|---|---|
| EN (scraper chain) | allanime, animefever, gogoanime, miruro, nineanime | live-eligible (subject to health) |
| EN (scraper, disabled) | animepahe | **disabled** — Cloudflare challenge (ISS-023) |
| RU | kodik (via `kodikextract` ad-free HLS) | live-eligible |
| RU | animelib | **disabled** — not working |
| 18+ | 18anime | live-eligible **only on hentai titles**; else irrelevant |
| 18+ | hanime | **disabled** — not working |
| JP | raw (AllAnime) | live-eligible |
| First-party | **AnimeEnigma / SVO** | **Inactive (WIP)** — hover: *"We are working on our own hosting"* |

`Server` = mirror within a provider (each backend's own servers). `Team` =
fandub/fansub group, shown only where a backend exposes groups (e.g. Kodik).

### 4.B Chip states (driven live)

Each `ProviderChip` computes one state from registry metadata + live health:

| State | Trigger | Render |
|---|---|---|
| `active` | enabled + relevant + healthy | selectable |
| `disabled` | registry `enabled:false` (animepahe, animelib, hanime) | tinted, not selectable, hover = `Reason`/`Description` |
| `down` | live health stage `up:false` | tinted, not selectable, hover = "Temporarily unreachable" |
| `irrelevant` | wrong audio/language/content (e.g. 18anime on a non-hentai title) | tinted, not selectable, hover explains the mismatch |
| `wip` | AnimeEnigma | tinted, hover = "We are working on our own hosting" |

### 4.C Health data source

- **Scraper providers** (allanime/animefever/gogoanime/miruro/nineanime/animepahe):
  live signal from `GET /api/anime/_/scraper/health` (orchestrator snapshot,
  `providers` key with per-stage `up` + `last_updated`) merged with
  `scraper-providers.yaml` metadata (`ProviderMeta{Name, Enabled, Reason,
  Description, Group}`).
- **Non-scraper backends** (kodik, animelib, hanime, raw, 18anime): no
  orchestrator stage signal. Treated as **"enabled unless flagged"** via registry
  metadata; lighter liveness probe where one exists, else registry state only.
- **Proposed backend addition:** a small gateway-exposed endpoint returning the
  **merged provider rows** (enabled + reason + description + up, per `ProviderRow`)
  in one call, so the frontend doesn't reconstruct the merge. Polled by
  `useProviderHealth`.

---

## 5. Control Surfaces (from the locked prototype)

- **Control bar:** play/pause, ±10s, hover-volume, scrub (buffered + hover-preview
  thumb + intro/outro chapter markers), Source pill, Subtitles, Settings, PiP,
  theater, fullscreen. On phones, secondary controls trim so the bar never
  overflows (theater + menus cover them).
- **Source & translation panel** (opened by the control-bar pill
  "`<provider> · <audio> ▾`"): Audio slider + Language slider (filters) → Team
  chips → Provider list (health-stated) → Server list.
- **Gear (playback only):** Quality, Speed, **Autoplay next** + **Auto-skip
  intro** toggles — **both OFF by default**.
- **Subtitles menu:** subtitle language list → **Subtitle settings** (text size,
  background opacity, **precise typeable timing offset** with ± step + Reset) →
  **Browse all subtitles** modal (search by release/group + provider/language
  filter chips, tracks grouped by language, provider badge + format + Select).
  Browse-all reuses the existing `OtherSubsPanel` data path (Jimaku /
  OpenSubtitles). Subtitle rendering reuses `SubtitleOverlay.vue`.
- **Overlays:** resume pill (reuse existing watch-progress + `ResumePill.vue`),
  Skip Intro chip, next-episode autoplay card, big-play button, inline ⇄ theater
  toggle, episode side panel + drawer (reuse `EpisodeSelector.vue`).
- **Watch Together:** **dropped for stage 1.** The WT icon stays in the top bar
  with a **"WIP"** label (disabled + tooltip). No `party` prop, no panel yet.

### Skip Intro / chapter markers
Shown **only when real skip timings exist** for the title; hidden otherwise (no
fake markers). Stage 1 ships the UI + the hidden/empty state.
**TODO (later stage):** add a markers-fetch backend (aniskip-style) and feed real
intro/outro times.

---

## 6. Stage-1 Wiring into `Anime.vue`

1. New pill in the `RU EN 18+ RAW JP` `ButtonGroup` (e.g. labeled with a brand
   mark + a small "beta" affordance) that selects the unified player.
2. New value in the `videoLanguage`/player-selection state that mounts
   `<UnifiedPlayer>` (placed **after** the existing `v-if/v-else-if` player chain
   per the Vue template rule — never between chain links).
3. The unified player resolves streams via `useProviderResolver` (reusing the
   existing per-provider endpoints); its internal Source panel does provider/
   server switching independently of the external tabs.
4. Gated behind a Vite flag (e.g. `VITE_UNIFIED_PLAYER_ENABLED`, default on for
   the trial) so it can be dark-shipped.

---

## 7. i18n

All new strings added to **`en.json`, `ru.json`, `ja.json`** (parity-locked,
1149 lines each). Namespaced under `player.unified.*`. A missing key in any one
file fails the locale parity test.

---

## 8. Testing & Verification

- **Composables:** unit tests for `usePlayerState` (action→state), `useProviderHealth`
  (registry+health merge → chip-state matrix), `useProviderResolver` (adapter
  dispatch). Handwritten fakes; no live API calls.
- **Components:** `.spec.ts` per presentational component (≥5 assertions),
  especially `ProviderChip` state matrix and `SourcePanel` filtering.
- **DS lint:** `design-system-lint.sh` passes (prove fail-path with `--selftest`).
- **Type-check:** `bunx tsc --noEmit`.
- **In-browser smoke:** desktop + mobile (DS-NF-06) on a real title — verify all
  five players' streams resolve through the unified shell, menus/panels open,
  scrub + subtitle overlay work, provider health tinting renders with tooltips.
- Verify against the **user's actual titles**, not just one known-good case.

---

## 9. Open / Deferred Items (tracked, not blocking stage 1)

- [ ] Intro/outro skip-timings backend (aniskip-style) — UI ready, data TODO.
- [ ] AnimeEnigma/SVO real self-hosted (MinIO/library) backend.
- [ ] Live Watch Together wiring via `party` prop.
- [ ] Light liveness probes for non-scraper backends (kodik/raw/18anime).
- [ ] Later stage: remove old `RU EN 18+ RAW JP` tabs + repoint combo logic.

---

## 10. Resolved Decisions (decision log)

1. **Architecture:** one unified shell; providers = stream-backend adapters.
2. **Rollout:** stage 1 adds a new pill in the language selector,
   non-destructive; repoint later.
3. **Stage-1 backends:** all HTML5 backends as adapters **+ Kodik** (kodikextract).
4. **Provider taxonomy:** flat real-provider list from the health registry,
   filtered by audio × language × content.
5. **AnimeEnigma/SVO:** a real future first-party backend, but **Inactive (WIP)**
   for now ("We are working on our own hosting").
6. **Disabled by default:** animepahe (Cloudflare), animelib, hanime.
7. **Tinted/disabled chips** for disabled + irrelevant providers, with hover
   descriptions; health auto-updated from live source.
8. **Watch Together:** dropped for stage 1; WT icon kept with "WIP" label.
9. **Skip markers:** UI only when real timings exist; backend fetch is a later TODO.
10. **i18n:** en + ru + ja parity.
11. **Build:** design-system-first; separation of concerns, predictable state,
    reusable components, performance by design.
