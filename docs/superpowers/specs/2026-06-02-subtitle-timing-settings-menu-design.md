# Subtitle Timing Settings Menu — Design

**Date:** 2026-06-02
**Status:** Approved (design)
**Scope:** Frontend only (Raw + OurEnglish players)

## Problem

The RAW JP player (and the EN OurEnglish player) render external subtitles
through the shared `SubtitleOverlay.vue` component, but expose **no UI for
adjusting subtitle timing**. Soft-subbed releases frequently drift out of sync
with the stream; users have no way to nudge cues earlier/later. The user
reported: "For RAW JP player I can't see sub settings menu (for timing and
size)."

`SubtitleOverlay.vue` already supports a timing `offset` prop and applies it
(`SubtitleOverlay.vue:96-102`, `const t = currentTime.value - props.offset`),
but:
- `RawPlayer.vue` never passes `offset` (defaults to 0).
- No player renders any control to change it.

## Scope decisions (locked)

- **Players:** Raw + OurEnglish — the two players that actually consume
  external/soft subs via `SubtitleOverlay`. AniLib/Hanime excluded.
- **Controls:** Timing offset only. Font size is **out of scope** for this pass
  (auto-computed from video height stays as-is). The design keeps the menu
  trivially extensible so a font-size control can be added later as one more row.
- **Persistence:** A single global value in `localStorage`
  (`subtitle_timing_offset`), remembered across episodes and sessions.
  Per-release re-sync is a manual nudge; we do not reset per episode.

## Approach

Shared composable owns the state; a shared menu component renders the controls;
both players mount the menu in their existing toolbar and feed the offset into
their `SubtitleOverlay`. The overlay stays a pure renderer.

Rejected alternatives:
- **Per-player local state + inline gear markup** — duplicates logic across two
  players.
- **Gear baked into `SubtitleOverlay`** (fullscreen-capable) — teleport /
  z-index / pointer-events tangle with native video controls; deferred. Timing
  is a set-once-then-watch action, so a toolbar gear is sufficient for v1.

## Components

### 1. `frontend/web/src/composables/useSubtitleTimingOffset.ts`

Module-singleton reactive state so every consumer (the menu and each player's
overlay) shares one source of truth.

```ts
const STORAGE_KEY = 'subtitle_timing_offset'
const MIN = -30, MAX = 30

function load(): number {
  const raw = Number(localStorage.getItem(STORAGE_KEY))
  return Number.isFinite(raw) ? clamp(raw) : 0
}

const offset = ref(load())                 // module-level singleton
watch(offset, (v) => localStorage.setItem(STORAGE_KEY, String(v)))

export function useSubtitleTimingOffset() {
  function nudge(delta: number) {
    offset.value = clamp(Math.round((offset.value + delta) * 10) / 10)
  }
  function reset() { offset.value = 0 }
  return { offset, nudge, reset }
}
```

- `clamp` keeps the value in `[-30, 30]`.
- Rounding to 1 decimal avoids float drift from repeated `±0.1` nudges.
- Write-back happens via the module-level `watch`, registered once.

### 2. `frontend/web/src/components/player/SubtitleSettingsMenu.vue`

Gear button + popover. Styled to match existing toolbar controls
(`bg-white/10 hover:bg-white/15 … border border-white/10`).

Props:
- `hasActiveSub: boolean` — when false, the gear is `disabled` (no point
  adjusting timing with subs off).

Behavior:
- Gear toggles a small popover anchored to the button.
- Popover contents (single row): `−1s` · `−0.1s` · live readout (`+0.0s`) ·
  `+0.1s` · `+1s` · `Reset`.
- Readout formats with an explicit sign and one decimal, e.g. `+1.5s`, `0.0s`,
  `-0.5s`.
- Uses `useSubtitleTimingOffset()` directly (no v-model needed — singleton).
- Closes on outside-click and `Escape`.
- All user-facing strings via i18n under `player.subtitleSettings.*`.

### 3. Player wiring

**`RawPlayer.vue`:**
- `const { offset } = useSubtitleTimingOffset()`.
- Add `:offset="offset"` to its `<SubtitleOverlay>` (currently missing).
- Render `<SubtitleSettingsMenu :has-active-sub="!!activeSubUrl" />` in the
  primary toolbar (the flex row at `RawPlayer.vue:70`), next to the Other Subs
  button.

**`OurEnglishPlayer.vue`:**
- Same composable + menu placement.
- Confirm/ensure its `<SubtitleOverlay>` receives `:offset="offset"`.

### 4. i18n

Add a `player.subtitleSettings` sub-namespace to **both**
`frontend/web/src/locales/en.json` and `frontend/web/src/locales/ru.json`
(the locale-parity test `src/locales/__tests__` fails if a key exists in only
one file). Keys (illustrative):

- `player.subtitleSettings.label` — gear tooltip / aria-label ("Subtitle
  timing" / "Тайминг субтитров")
- `player.subtitleSettings.title` — popover heading
- `player.subtitleSettings.reset` — "Reset" / "Сброс"
- `player.subtitleSettings.offsetHint` — short explainer ("Shift subtitles
  earlier or later")

## Data flow

```
localStorage[subtitle_timing_offset]
        │ load() once
        ▼
   offset (singleton ref) ──watch──▶ localStorage (write-back)
        │                    │
        │ used by            │ used by
        ▼                    ▼
 SubtitleSettingsMenu   SubtitleOverlay (:offset)
 (nudge / reset)        (currentTime - offset → active cues)
```

## Error handling / edge cases

- Corrupt/missing localStorage value → `load()` falls back to `0`.
- Gear disabled when no active subtitle track.
- Offset clamped to `[-30, 30]`; rounding prevents float drift.
- No network, no backend — failure modes are local only.

## Testing

- **Unit (Vitest):** `SubtitleSettingsMenu.spec.ts`
  - nudges update the readout (coarse and fine, both directions)
  - reset zeroes the value
  - disabled state when `hasActiveSub=false`
  - persistence: a nudge writes the expected `localStorage` key
- **Locale parity:** existing `src/locales/__tests__` spec picks up the new keys.
- **Type-check:** `bunx tsc --noEmit`.
- **Smoke (post-deploy):** load Raw player, pick a JP sub, open gear, nudge,
  confirm cues shift and the value survives a reload. (i18n key paths are
  string-typed — smoke-verify per project rule.)

## Non-goals

- Font size override (deferred; design is extensible to add it).
- Vertical position control.
- Fullscreen-embedded controls (toolbar gear only for v1).
- Per-release / per-anime offset storage (single global value).
- Backend changes.

## Metrics (project convention)

- **UXΔ** = +2 (Better)
- **CDI** = 0.02 * 5
- **MVQ** = Sprite 88%/85%
