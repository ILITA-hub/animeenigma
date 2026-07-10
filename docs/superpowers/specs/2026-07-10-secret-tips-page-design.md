# Secret tips & hotkeys page — design

**Source:** feedback `2026-07-08T15-21-31_tNeymik_manual` (tNeymik): «Сделать секретную страницу с подсказками и хоткеями».

**Metrics:** UXΔ = +2 (Better) · CDI = 0.03 * 8 · MVQ = Sprite 90%/85%

## Problem

The app has a rich hotkey surface (13 player shortcuts, scrub-bar keys, contextual
Escape/Enter affordances) and a pile of deliberately-hidden features (footer
roulette, RAW/DUB semantics, subtitle auto-sync, scrub storyboards, poster
context menu, theater mode, PWA downloads, Watch Together). None of it is
documented anywhere in-product. The owner wants a *secret page* that collects
tips and hotkeys — hidden knowledge for users who go looking.

## Approaches considered

- **A (chosen): new public page `/tips` in the secret-features roulette pool, plus a global `?` hotkey.**
  Fits the existing secret-surface architecture exactly (registry + server-resolved
  pool); the `?` hotkey is a second, thematically apt discovery path and itself
  becomes the first "global hotkey" the page documents.
- **B: `?`-triggered cheatsheet modal only.** Rejected — the request explicitly
  says «страницу» (a page), and a modal can't comfortably hold the tips section.
- **C: footer-linked docs page.** Rejected — not secret; the roulette exists
  precisely to surface hidden pages.

## Design

### 1. `views/TipsPage.vue` — route `/tips` (lazy, `meta.titleKey: 'tips.title'`)

Static i18n'd content page in the style of `About.vue`/`StatusPage.vue`. Sections:

1. **Header** — title + subtitle (playful "you found the secret page" tone).
2. **Player hotkeys** — grouped rows rendered from a typed const array, keys as
   styled `<kbd>` (scoped `.kbd` class bound to semantic tokens: `bg-muted`,
   `border-border`, `--font-mono`). Groups: playback (Space/K, J/L, ←/→, 0–9
   deciles), volume (↑/↓, M), view (F, P, Esc-closes-panels), subtitles (C, Z/X
   ∓0.1s). Content mirrors `composables/aePlayer/playerHotkeys.ts` — the
   code-verified contract.
3. **Everywhere else** — `?` opens this page; Esc closes drawers/lightbox/theater
   mode; scrub bar focused: ←/→ ±1% + Home/End; chat: Enter / Shift+Enter;
   spotlight focused: ←/→.
4. **Tips grid** — `Card` primitives + lucide icons, ~9 code-verified tips:
   footer roulette · RAW=original-audio slider semantics · subtitle auto-sync
   (VAD) switch in the subs menu · timeline hover storyboards · poster
   right-click context menu (mark-watched / download season) · theater mode ·
   install as PWA → offline downloads · Watch Together button in the player ·
   Report button on broken streams.

### 2. Global `?` hotkey

- `utils/globalHotkeys.ts`: pure `isHelpHotkey(e)` — `key === '?'`, no
  ctrl/meta/alt, target not INPUT/TEXTAREA/SELECT/contenteditable. Unit-spec'd
  (mirrors the `mapKeyToAction` pure-function pattern).
- `App.vue` wiring: window keydown → if `isHelpHotkey(e)` AND no `<video>` in
  the DOM (never yank a user off an active watch surface) AND not already on
  `/tips` → `router.push('/tips')`.

### 3. Secret-features registration

- `utils/secretFeatures.ts`: add `'tips'` to the key union + registry entry
  (`labelKey: 'admin.secretFeatures.feature.tips'`).
- Policy service `SeedFlags()`: `everyone("tips", "Tips & hotkeys")` —
  insert-if-absent seed, roulette-eligible for everyone, admin-manageable on
  `/admin/policy` like every other flag. No gateway change.

### 4. i18n

New top-level `tips.*` section + `admin.secretFeatures.feature.tips` in
**en/ru/ja** (full parity, no ICU placeholders).

## Error handling

- Policy feed outage: existing fail-open semantics — the route stays directly
  reachable; only the roulette pool membership degrades (same as every entry).
- `?` on unusual layouts: `e.key` is layout-resolved (RU Shift+7 → `?`), works.

## Testing

- `utils/globalHotkeys.spec.ts` — predicate unit tests.
- Existing `secretFeatures.spec.ts` passes unchanged (no roster-size assertions).
- `/frontend-verify` (DS-lint, i18n 3-locale parity, real build).
- `go test ./...` in `services/policy`; redeploy `policy` + `web`.
