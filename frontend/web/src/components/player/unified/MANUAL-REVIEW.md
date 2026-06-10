# Unified Player — Manual Browser-Review Spec

A driveable checklist for in-browser review of the **AnimeEnigma (Beta)** unified
player against the locked prototype design. Each item lists the **design-expected**
state, **how to verify** it in the browser, and a **status** box to fill during a
pass. The "actual / notes" column is where divergences get recorded.

**Sources of truth**
- Prototype component: `docs/superpowers/specs/assets/unified-player-prototype/Player.jsx`
- Prototype styles: `docs/superpowers/specs/assets/unified-player-prototype/player.css`
- Prototype icons: `docs/superpowers/specs/assets/unified-player-prototype/Icons.jsx`
- Design spec: `docs/superpowers/specs/2026-06-06-unified-anime-player-design.md`

**Status legend:** ✅ matches design · ⚠️ minor divergence · ❌ broken / wrong · — not yet checked

---

## 0. Setup

1. Browser: connected local Chrome (ask which, then `select_browser`).
2. Title (healthy, common): **Frieren** `https://animeenigma.ru/anime/f0b40660-6627-4a59-8dcf-7ec8596b3623`.
3. Also test a **hentai** title (for 18anime relevance) and a **Russian** combo (Kodik).
4. Activate the player: click the **`AnimeEnigma · Бета`** pill in the `RU EN RAW JP …` selector.
5. Review at **desktop (≥1280)** AND **mobile (≤680)** — DS-NF-06: jsdom can't catch Tailwind-v4 cascade bugs.
6. Helpful JS probe for `<video>` state:
   ```js
   (() => { const v = document.querySelector('.pl video');
     return JSON.stringify({ rs: v?.readyState, ct: +v?.currentTime?.toFixed(1), dur: +v?.duration?.toFixed(1),
       paused: v?.paused, vol: v?.volume, muted: v?.muted, vw: v?.videoWidth }); })()
   ```

---

## A. Control-bar layout & structure (the big one)

The prototype `.pl-controls` is **two stacked rows**:

```
.pl-controls
 ├─ .pl-scrub-row :  [current time]  [ track (flex:1) ]  [total time]      ← times FLANK the track
 └─ .pl-btns      :  ▶ | ⟲5 | ⟳5 | 🔊vol | ‹spacer› | [src pill] | CC | ⚙ | ⧉pip | ⤢fs
```

| # | Design-expected | How to verify | Status | Actual / notes |
|---|---|---|---|---|
| A1 | Scrub track sits on its **own row above** the buttons, **inside** `.pl-controls` (not a separate floating overlay) | Inspect: `.pl-controls > .pl-scrub-row > .pl-track`; times are siblings of the track | — | |
| A2 | **Current time on the left of the track, total time on the right**, both `mono` font, 42px wide | Look at the scrub row ends | — | |
| A3 | Time is **NOT** shown in the button row | The button row has no `m:ss / m:ss` cluster | — | |
| A4 | Button order L→R: play, −5s, +5s, volume, **spacer**, source pill, CC, gear, PiP, fullscreen | Visual scan | — | |
| A5 | **No theater button** (removed by request) | Confirm absent between PiP and fullscreen | — | |
| A6 | Controls sit over a bottom gradient `linear-gradient(transparent, rgba(0,0,0,.82))` | Visual | — | |
| A7 | No element's hover/focus outline overlaps the progress track | Tab through / hover each button | — | |

---

## B. Individual controls

| # | Control | Design-expected | Status | Actual / notes |
|---|---|---|---|---|
| B1 | Play/Pause | Toggles; icon swaps play↔pause; `Icon size 22` | — | |
| B2 | −5s / +5s | Circular `forward10` arrow; back is mirrored (`scaleX(-1)`); seeks ∓5s; digit "5" inside (per user request) | — | |
| B3 | Volume button | Mute toggle; icon reflects muted / <0.5 / high | — | |
| B4 | Volume slider | Hidden until hover the `.pl-vol` cluster; expands to 72px; `min=0 max=100`; drags smoothly; `accent-color: brand-cyan` | — | |
| B5 | Source pill | `[hue dot] {Provider} · {Sub/Dub} ▾`; **gets `is-open` highlight when its panel is open** | — | |
| B6 | CC (subtitles) | Opens subtitles menu; **button shows `is-open` when menu open** | — | |
| B7 | Gear (settings) | Opens playback menu; **`is-open` when open** | — | |
| B8 | PiP | Toggles picture-in-picture | — | |
| B9 | Fullscreen | Toggles fullscreen on the player shell | — | |

---

## C. Source & translation panel (opened by the source pill)

Design: `.pl-srcpanel` floats `top:64px right:14px`, width 300, card with `pl-pop` anim.

| # | Design-expected | Status | Actual / notes |
|---|---|---|---|
| C1 | Header "Sources" + "N available" count | — | |
| C2 | **AUDIO** slider: `Sub / Dub` two-segment slider with sliding cyan→pink thumb | — | |
| C3 | **LANGUAGE** slider: `English / Русский / 日本語` (+ count) with sliding thumb | — | |
| C4 | **PROVIDER** list = real backends filtered by audio×lang×content, each a `ProviderChip` with identity-hue dot | — | |
| C5 | Chip states render: `active` selectable; `disabled/down/irrelevant/wip` tinted + not selectable + hover reason | — | |
| C6 | AnimeEnigma chip = **WIP**, hover "We are working on our own hosting" | — | |
| C7 | Selecting a provider re-resolves + closes/keeps panel per design; active row highlighted | — | |
| C8 | Server list shows when a backend exposes mirrors | — | |

---

## D. Settings (gear) menu

Design `.pl-settings` min-width 250, root → quality/speed sub-views.

| # | Design-expected | Status | Actual / notes |
|---|---|---|---|
| D1 | Header "Playback" | — | |
| D2 | **Quality** row → sub-view, options `Auto / 1080p / 720p / 480p`, `1080p` gets HD badge | — | |
| D3 | **Speed** row → sub-view, options `0.75 / 1 / 1.25 / 1.5 / 2` (1 = "Normal") | — | |
| D4 | **Autoplay next** toggle — OFF by default | — | |
| D5 | **Auto-skip intro** toggle — OFF by default | — | |
| D6 | Sub-views have a back chevron returning to root | — | |

---

## E. Subtitles menu

Design: root = language list + "Subtitle settings" entry → `subsettings` sub-view (text size, background, timing stepper, Browse all).

| # | Design-expected | Status | Actual / notes |
|---|---|---|---|
| E1 | Language list = **`Off / English / Русский / 日本語`** (exactly four, no duplicate "Off", no EN/RU/JA abbreviations) | — | |
| E2 | Active language shows a check | — | |
| E3 | "Subtitle settings" opens a sub-view (not all inline) | — | |
| E4 | Text size + Background sliders | — | |
| E5 | Timing offset: typeable number + `−`/`+` 0.1s stepper + "In sync" hint + Reset | — | |
| E6 | "Browse all subtitles" opens `BrowseSubsModal` | — | |

---

## F. Scrub bar (PlayerScrubBar)

| # | Design-expected | Status | Actual / notes |
|---|---|---|---|
| F1 | Track base `rgba(255,255,255,.25)`, 4px, rounded | — | |
| F2 | Buffered range `rgba(255,255,255,.4)` | — | |
| F3 | Fill = `brand-cyan` with glow | — | |
| F4 | Thumb (13px white, cyan halo) appears on hover; track grows 4→6px on hover | — | |
| F5 | Click / drag seeks; reflects while paused | — | |
| F6 | Intro/outro chapter markers ONLY when real timings exist (else none — no fake markers) | — | |
| F7 | Hover preview thumbnail + time (if implemented) | — | |

---

## G. Overlays

| # | Design-expected | Status | Actual / notes |
|---|---|---|---|
| G1 | Big play button: 88px cyan-glow circle, centered, hover scale 1.08; only when paused | — | |
| G2 | Resume pill (bottom-left) when watch progress exists | — | |
| G3 | Skip-intro chip only with real timings (hidden in stage 1) | — | |
| G4 | Next-episode autoplay card near end (when Autoplay next ON) | — | |
| G5 | Top bar: back chevron, EP eyebrow `EP n · [dot]Provider · Sub`, title, WT (WIP) + episode-list buttons | — | |

---

## H. Keyboard & accessibility

| # | Design-expected | Status | Actual / notes |
|---|---|---|---|
| H1 | Space / k → play-pause | — | |
| H2 | ← → / j l → seek ∓5s | — | |
| H3 | ↑ ↓ → volume ±5 | — | |
| H4 | m mute · f fullscreen · c subs · p pip | — | |
| H5 | 0–9 → seek to decile | — | |
| H6 | Esc closes any open menu | — | |
| H7 | Shortcuts only fire when pointer over player or focus within (don't hijack page typing) | — | |
| H8 | Root is focusable (`tabindex=0`), `role=region`, aria hint, focus-visible ring | — | |

---

## I. Playback (now working — verify per source)

| # | Source / combo | Expected | Status | Actual / notes |
|---|---|---|---|---|
| I1 | EN auto (scraper) | Resolves + plays; `readyState ≥ 3`, currentTime advances | — | |
| I2 | EN pinned gogoanime / miruro (HLS) | hls.js attaches (`blob:` src), plays | — | |
| I3 | RU Kodik (ad-free HLS) | Plays | — | |
| I4 | JP Raw (AllAnime) | Plays | — | |
| I5 | 18anime (on hentai title only) | Selectable + plays; irrelevant/tinted on non-hentai | — | |
| I6 | Provider switch mid-play | Re-resolves cleanly, no double-load / stale stream | — | |
| I7 | Sub/Dub + Language switch | Re-resolves to a valid provider; no cross-language leak | — | |

---

## J. Responsive (≤680px)

| # | Design-expected | Status | Actual / notes |
|---|---|---|---|
| J1 | Skip ±5, PiP, fullscreen hidden; bar never overflows | — | |
| J2 | Icons shrink 40→36px, `.pl-btns` gap 0 | — | |
| J3 | Title 15px, eyebrow 11px | — | |
| J4 | Source panel width clamps to viewport | — | |
| J5 | Menus open within viewport bounds | — | |

---

## Divergence log

### Review pass — 2026-06-10 (desktop, Frieren)

| ID | Element | Design says | Observed | Severity | Resolution |
|---|---|---|---|---|---|
| D-01 | Control bar (A1-A3) | `.pl-controls` = `.pl-scrub-row` (time · track · time) **above** `.pl-btns` | Scrub was a separate floating overlay; current/total time jammed **inside** the button row | ❌ | **Fixed** — moved `PlayerScrubBar` into `PlayerControlBar` as `.pl-scrub-row` with flanking times; removed `.pl-scrub-overlay` and the in-button time |
| D-02 | Subtitle language list (E1) | `Off · English · Русский · 日本語` | Showed `Off · OFF · EN · RU · JA` (duplicate Off + abbreviations) | ❌ | **Fixed** — `subLangsAvailable` → `['en','ru','ja']`; menu maps codes → display names |
| D-03 | Source/CC/gear buttons (B5-B7) | `is-open` highlight when their menu is open | Source pill hardcoded `is-open:false`; CC/gear had no `is-open` binding → never highlighted | ❌ | **Fixed** — pass `openMenu` to the bar; bind `is-open` on all three |
| D-04 | Speed options (D3) | `0.75 / 1 / 1.25 / 1.5 / 2` | `0.25 / 0.5 / 0.75 / 1 / 1.25 / 1.5 / 1.75 / 2` | ⚠️ | **Fixed** — aligned to design list |
| D-05 | Quality options (D2) | `Auto / 1080p / 720p / 480p` | `Auto` only | ⚠️ | **Deferred** — scraper exposes no quality ladder; leaving `Auto` (data-driven) rather than faking levels. Track when a backend reports variants. |
| D-06 | Menu anchoring | `.pl-menu` anchored above its own trigger button via `.pl-menu-wrap` | Settings/subs float at a fixed `right:14px; bottom:76px` | ⚠️ | **Acceptable** — visually equivalent floating card; revisit only if pixel-anchor matters |

All ❌ resolved + verified in-browser; ⚠️ items either fixed or consciously deferred with reason.
