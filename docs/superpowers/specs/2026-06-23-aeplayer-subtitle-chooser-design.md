# aePlayer subtitle chooser rework — design

**Date:** 2026-06-23
**Status:** Approved (design); ready for implementation plan
**Scope:** Frontend-only (`frontend/web/`). No backend changes.

## Problem

The aePlayer (OurEnglish EN player) subtitle UX is broken and confusing:

1. **"Language off" is meaningless.** The quick menu (`SubtitlesMenu.vue`) renders
   `Language: [Off] [<one lang>]`. The inline `Language` label sitting next to the
   `Off` pill reads as "Language off".
2. **No fast language switching.** `subLangsAvailable` (`AePlayer.vue:2230`) only ever
   returns the *currently chosen* language, so the menu shows at most one language pill —
   you can't switch between RU/EN/JP.
3. **"Browse all subtitles" is not interactible.** Opening the browse modal sets
   `browseOpen = true` but leaves the floating subs menu open (`openMenu` stays `'subs'`).
   The `onClickOutside` handler (`AePlayer.vue:2100`) then fires on the **first click inside
   the modal** (the click is outside the small floating menu) and calls `closeMenus()`,
   which sets `browseOpen = false`. The modal disappears on first touch.
4. **Browse modal shows every language**, not the relevant RU/EN/JP set.

## Goal

A YouTube-like subtitle chooser:

- Quick row: `[ <provider> ] [ Off ] [ RU ] [ EN ] [ JP ]`
- Provider's bundled subs auto-selected on load; no "Auto" button.
- Browse modal scoped to RU/EN/JP by default, "More languages" reveals the rest.
- Everything actually wired to drive the player.

## Decisions (locked with owner)

- **No "Auto" button.** The player still auto-selects the provider's bundled subtitle on
  load when the resolved stream ships one. This is surfaced as a **provider chip** (the
  scraper provider name, e.g. `gogoanime`) that is **hidden when there are no bundled subs**.
- **Fast buttons are fixed:** `Off / RU / EN / JP`, always in that order. A language button
  is **enabled only if a track exists** for it; otherwise dimmed + unclickable with a
  "no subtitles" hint. (Layout never shifts.)
- **Browse modal:** keep the search box **and** the language filter chips, but constrain the
  chips to **RU / EN / JP + "More languages"**. Default body shows only RU/EN/JP groups;
  "More languages" reveals the rest (chips + groups). Keep provider badges per track.
- **Frontend-only.** Catalog already returns languages grouped (`/subtitles/all`) and
  provider subs ship with the resolved stream (`currentStream.subtitles`).

## Design

### 1. Quick chooser — `SubtitlesMenu.vue`

Replace the `Language: [Off] [<one lang>]` row with a fixed YouTube-style row under the
existing "Subtitles" header. Remove the inline `Language` label (the "Language off" source).

```
Subtitles
[ gogoanime ]  [ Off ]  [ RU ]  [ EN ]  [ JP ]
   provider chip          dim if no track
   (hidden if none)
```

Behaviour:

- **Provider chip** — shown only when `currentStream.subtitles` is non-empty. Label = the
  bundled track's provider name. Clicking it selects the bundled track. It is the auto-selected
  default on load.
- **Off** — turns subtitles off (`subLang = 'off'`, `chosenSub = null`), latches user choice.
- **RU / EN / JP** — always render in fixed order. Enabled iff a track exists for that lang in
  the merged track list. Clicking selects the **best track for that language by relevance**
  (existing `providerRank`: provider-own → Jimaku for JP → OpenSubtitles), latches the choice,
  turns the overlay on. Disabled buttons are dimmed, `aria-disabled`, with a title/hint
  "No <language> subtitles".
- **Active-highlight is mutually exclusive:** if the active `chosenSub` is the provider-bundled
  track → highlight the provider chip; else highlight the matching lang button.
- Settings (text size / background / timing offset) and "Browse all subtitles" are unchanged.

New/changed props: the menu receives `availableSubLangs: string[]` (real, merged) and a
`providerChip: { provider: string } | null` (null hides the chip). Emits `update:subLang` with
`'off' | 'ru' | 'en' | 'ja'` and a new `select-provider` event.

### 2. Browse modal — `BrowseSubsModal.vue`

- **Interactivity fix is in `AePlayer.vue`** (see §3): closing the floating menu when browse
  opens stops `onClickOutside` from killing the modal. No structural change needed in the modal
  for the bug itself, but verify backdrop `@click.self="emit('close')"` still closes it.
- **Language scope:** add a `showAllLangs` ref (default `false`).
  - Language chips render `RU / EN / JP` by default. When other languages exist, render a
    **"More languages (N)"** chip; clicking it sets `showAllLangs = true`, which appends the
    remaining distinct-language chips.
  - The grouped body renders only RU/EN/JP groups by default; remaining-language groups render
    only when `showAllLangs` is true. (When `showAllLangs` is false but other-language groups
    exist, show the "More languages (N)" affordance at the bottom of the list too.)
  - `PRIMARY_LANGS = ['ru', 'en', 'ja']`. "More languages (N)" counts distinct non-primary
    languages actually present.
- **Keep** the search box and the per-track provider badges. Search still narrows by
  label/provider across whatever is currently visible.

### 3. Wiring — `AePlayer.vue` + `useSubtitleTracks.ts`

- **Fix the broken availability source:** replace `subLangsAvailable` (chosen-lang-only) with
  `availableSubLangs` computed from the merged `subtitleTracks` (distinct `lang` values).
- **Provider chip source:** compute `providerChip` from `providerSubtitles`
  (`currentStream.subtitles`) — `null` when empty, else `{ provider }` from the first bundled
  track. Add `providerBundledUrls` (a Set) to detect when `chosenSub` is a bundled track for the
  mutually-exclusive highlight.
- **`pickBestForLang(lang)`** helper (reuse `providerRank` from `pickDefaultSubtitle.ts`):
  best track among those whose `lang === lang`. Wire RU/EN/JP clicks to it.
- **`autoSelectSubtitle`** prefers the bundled track: if `providerSubtitles` is non-empty,
  select the first bundled track; else fall back to existing `pickDefaultSubtitle`.
- **Interactivity fix:** the `open-browse` handler sets `openMenu.value = null` (in addition to
  `browseOpen = true; void ensureSubsLoaded()`). This removes the `activeMenuEl` target so
  `onClickOutside` no longer fires inside the modal.

### 4. i18n (en / ru / ja)

Add keys under `player.aePlayer.subs.*` (all three locales, parity-gated):

- `subs.noTrackHint` — "No {language} subtitles"
- `subs.moreLanguages` — "More languages ({count})"
- `subs.providerSource` (optional tooltip on the provider chip) — "Subtitles from {provider}"

Reuse existing `subs.off`, `subs.loading`, `subs.loadError`, `subs.retry`, `subs.empty`,
`subs.providersDown`. Fast-button labels `RU / EN / JP` are language codes (not translated);
provider chip shows the raw provider id.

## Files touched

| File | Change |
|------|--------|
| `frontend/web/src/components/player/aePlayer/SubtitlesMenu.vue` | New fixed Off/RU/EN/JP row, provider chip, disabled states; drop `Language` label |
| `frontend/web/src/components/player/aePlayer/BrowseSubsModal.vue` | RU/EN/JP default scope + "More languages" toggle; keep search; constrain lang chips |
| `frontend/web/src/components/player/aePlayer/AePlayer.vue` | `availableSubLangs`, `providerChip`, `pickBestForLang`, bundled-pref auto-select, `open-browse` closes floating menu |
| `frontend/web/src/composables/aePlayer/pickDefaultSubtitle.ts` | Export `pickBestForLang` / share `providerRank` |
| `frontend/web/src/locales/{en,ru,ja}.json` | New subs keys (parity) |
| `*.spec.ts` co-located | Update/extend specs for both components |

## Non-goals (YAGNI)

- No backend changes; no new subtitle provider; no release-name fuzzy "relevance" beyond the
  existing provider-rank ordering.
- No furigana / styling changes to `SubtitleOverlay.vue`.
- Fast buttons stay a fixed RU/EN/JP set — not a dynamic per-episode language list.

## Testing

- Co-located Vitest specs: `SubtitlesMenu.spec.ts` (fixed row renders, disabled state for
  missing langs, provider chip hidden when no bundled subs, emits correct lang), `BrowseSubsModal.spec.ts`
  (RU/EN/JP-only default, "More languages" reveal, search still filters).
- `/frontend-verify`: DS-lint, i18n en/ru/ja parity, real `bun run build`, vue-tsc.
- Opt-in Chrome smoke (owner-gated) to confirm the browse modal is now interactible and the
  language buttons drive the overlay end-to-end.

## Metrics (project convention)

- **UXΔ = +3 (Better)** — replaces a broken, confusing, single-language control with a clear
  YouTube-like chooser + a browse menu that actually works.
- **CDI = 0.02 * 8** — contained to the aePlayer subtitle slice (4 SFCs/composable + i18n),
  low spread, low shift, Effort_Fib 8.
- **MVQ = Sprite 88%/85%** — small, self-contained, high-clarity UI fix with strong
  slop-resistance (typed props, parity-gated i18n, co-located specs).
