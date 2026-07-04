# Mobile player redesign + season downloads — design

**Date:** 2026-07-04 · **Owner request:** «Обнови дизайн плеера под мобилки — он слишком мелкий и невозможно скачать, добавь опцию "скачать целый сезон"» (screenshot: iPhone, anime page, clipped control bar, episode sheet unusable).

**Metrics:** UXΔ = +4 (Better) · CDI = 0.04 * 21 · MVQ = Griffin 85%/80%

## Problem inventory (code-verified)

1. **No fullscreen on mobile.** `PlayerControlBar.vue` hides `.pl-fs-btn`, `.pl-pip-btn`, `.pl-skip-*` at ≤680px. The only watch surface is the inline card.
2. **Video ~326px of a 390px screen.** `AePlayer` (`.pl`, 16:9) sits inside `glass-card p-4 md:p-6` inside the page gutter (`views/Anime.vue:404`). ~35% of screen area lost to chrome.
3. **Control bar overflows.** `@media (pointer: coarse)` force-expands the volume slider (72px) — dead weight on iOS where JS volume control is ignored — pushing right-side buttons past the edge (the clipped button in the owner's screenshot).
4. **In-player menus don't fit.** Source panel / episodes sheet are absolutely positioned inside `.pl` with `max-height: calc(100% - 130..150px)`; at a ~185px-tall mobile player that's a ≤55px window. Functionally unusable.
5. **Download unreachable.** The only entry is a 14px icon inside a 158px strip card inside that unusable sheet. No season option exists.

## Direction (chosen from 3 options; owner AFK — recommended option taken)

**"Full mobile pack + under-player action row", straight to code within the existing DS** (same tokens/components; layout and sizing change — no new visual language, so the design-prototyping sandbox loop is skipped; owner reviews live on prod from the phone).

Scope: **mobile only** (≤680px CSS breakpoint — matches existing player breakpoint; gestures gated on `(pointer: coarse)`). Desktop layout unchanged. Season download works on all viewports.

## Design

### D1. Full-bleed player on mobile
- `views/Anime.vue`: the player's `glass-card` drops padding on mobile (`p-0 sm:p-4 md:p-6`) and escapes the page gutter (`-mx-4 sm:mx-0`, matching the page container's mobile padding). Same treatment for the premiere-notice placeholder and the Classic-Kodik card.
- `.pl` at ≤680px: `border-radius: 0; border-left: 0; border-right: 0`.
- Result: video width = 100vw (+43% area at 390px).

### D2. Fullscreen restored (native + iOS pseudo-FS)
- Un-hide the fullscreen button at ≤680px (keep PiP and ±5s hidden — declutter; seek moves to gestures, D4).
- `onToggleFullscreen()` becomes capability-based:
  - `rootRef.requestFullscreen` exists (Android/desktop) → native element fullscreen as today, **plus** on coarse pointers a best-effort `screen.orientation.lock('landscape').catch(() => {})` on enter and `unlock()` on exit.
  - Otherwise (iPhone Safari — no element fullscreen API) → **pseudo-fullscreen**: reactive `pseudoFs`, class `pl--pseudo-fs` (`position: fixed; inset: 0; z-index: 100; aspect-ratio: auto; border-radius: 0; background: #000`), body scroll lock while active. Exit via the same button (Minimize icon) **and** a pushed history state so the phone back-gesture exits pseudo-FS instead of leaving the page.
  - `video.webkitEnterFullscreen()` (native iOS player) rejected: loses SubtitleOverlay, source panel, WT button.
- SubtitleOverlay already anchors to `rootRef` (`:fullscreen-container`), so it survives both modes unchanged.
- iOS 26 statusbar gotcha (memory `project_ios26_safari_statusbar_viewport_cover`): a fixed element at the top edge renders an opaque band — acceptable here (black over black video).

### D3. Control bar declutter + touch targets
Mobile bar (≤680px): `[▶] [mute] [0:12 / 23:40] ——— [•src] [CC] [⚙] [⛶]`
- Volume **slider** hidden on coarse pointers (hardware buttons rule; iOS ignores JS volume anyway). Mute icon stays.
- Episodes pill removed from the bar on mobile (two better entries remain: top-left EP trigger + action row, D6).
- Source pill on mobile trims to hue-dot + chevron (name lives in the top eyebrow and the sheet header).
- Time pill drops its pill chrome on mobile (plain text) to save width.
- `PlayerIconButton` and pills grow to ≥44px hit targets on coarse pointers; scrub bar gets a taller invisible touch zone and a larger thumb on touch.

### D4. Touch gestures
On coarse pointers only (desktop click behavior unchanged):
- **Single tap** on video = toggle chrome visibility (never play/pause). While chrome is visible and playback started, a **center play/pause overlay button** (semi-transparent, 56px) is shown — the mobile play/pause affordance.
- **Double-tap** left/right thirds = −/+10s with a brief ripple/label indicator.
- Implementation: tap/double-tap detector (~250ms window) in the video click/touch path; menus-open backdrop-dismiss behavior kept.

### D5. Menus become bottom sheets on mobile
- The four floating menus (source / episodes / settings / subs) + `BrowseSubsModal` + `DownloadDialog` render as **viewport bottom sheets** at ≤680px: `position: fixed; left/right/bottom: 0; max-height: 72dvh; border-radius 16px 16px 0 0`, scrim backdrop (tap = close), `env(safe-area-inset-bottom)` padding. **z-index above pseudo-FS** (player takeover z-100 → sheets z-110) so sheets stay usable in iOS pseudo-fullscreen.
- **Teleport**: each menu wrapper gets `<Teleport :to="sheetHost" :disabled="...">` — teleported to `body` on mobile when not in native fullscreen; kept in-place inside `rootRef` in native fullscreen (body-teleported nodes are invisible under a fullscreen element) and on desktop. Scrim handles dismissal on mobile (click-outside stays for desktop).
- EpisodesPanel per-card download icon gets a ≥44px padded hit area.

### D6. Under-player action row (mobile-only, lives in AePlayer)
- AePlayer template restructure: new outer wrapper root (`.pl-wrap`, carries `data-test="ae-player"`), containing the existing `.pl` box (keeps `rootRef`, tabindex/hotkeys, theater class) + a mobile-only action row below:
  `[Эп. N ▾] [Источник: <name> ▾] [⬇ Скачать]` — 44px buttons, DS `Button` variants, full-width row.
- Wiring: `toggleMenu('episodes')`, `toggleMenu('source')`, and open DownloadDialog for the current episode.
- Offline mode (`/downloads` page): Источник hidden (synthetic provider — nothing to switch), ⬇ hidden (`!offline && canDownload`, same gate as the panel). Эп stays.
- Living inside AePlayer (not Anime.vue) keeps it working on every mount point with zero cross-component wiring.

### D7. Season download
- **`DownloadDialog` v2** (bottom sheet on mobile, floating card on desktop): quality 480/720/1080 (persisted, unchanged) + **scope selector**: «Эта серия (N)» / «Весь сезон — K серий, ~X ГБ». Season scope counts only episodes not already `done`/`downloading`/`queued`. Projected size = K × per-episode hint. If projected > `storageEstimate()` free space → non-blocking warning line («может не хватить места, свободно ~Y») — engine's per-download quota pre-check remains the hard stop.
- **`enqueueSeason` helper** in `src/offline/` (pure target-selection + loop over `enqueueDownload`, per-episode `resolve` closures, one frozen combo snapshot). Engine queue is already serial with paced workers — a 24-episode season just queues. Platform-neutral, engine-API-level (respects the `OfflineMediaStore` portability seam — no new byte I/O).
- **Entry points:** action-row ⬇ and per-card icons open the dialog (default scope: that episode); EpisodesPanel header gains a «⬇ Сезон» chip (all viewports — desktop gets season downloads too) opening the dialog pre-set to season scope.
- Queued episodes surface immediately as spinners in the panel and as rows on `/downloads` (existing store groups by anime).

### D8. Top-bar mobile trim
≤680px: title 15px, hide the episode-title text in the eyebrow (`.pl-ep-title`), tighter padding, EP trigger ≥44px hit area, resume chip repositioned to clear the bar.

## Not doing (explicit)
- No Background Fetch (deferred from PWA spec; downloads still require the tab alive).
- No PiP on mobile, no per-stream quality ladder, no watch-takeover route.
- No desktop layout changes beyond the shared DownloadDialog v2 + season chip.

## Testing
- Unit: season target-selection helper (skips downloaded/queued, aired-only list as input); DownloadDialog scope emit + estimate; control-bar mobile trim (class presence); EpisodesPanel season-chip emit; action-row render gates (offline / canDownload).
- Gates: `/frontend-verify` (DS-lint, i18n en/ru/ja parity for all new keys, eslint, real build, touched-component vitest).
- Manual (owner, on phone): full-bleed + fullscreen both platforms, sheets, double-tap seek, season download on a real title, offline playback of a season batch.

## Risks
- **AePlayer root restructure** (D6) — check `__tests__`/e2e for selectors assuming root = `.pl`; keep `data-test="ae-player"` on the new root.
- **Teleport + fullscreen interplay** (D5) — dynamic teleport target must flip on `fullscreenchange`.
- **Tailwind v4 cascade** — player styles are scoped CSS, low risk; flag for opt-in Chrome smoke.
