---
id: RAW-player-frontend
title: RawPlayer.vue + OtherSubsPanel.vue
workstream: raw-jp
milestone: v0.1
phase: 03
created_at: 2026-05-18
status: SPEC-ready
ambiguity_score: 0.15
mode: --auto
---

# Phase 03 (workstream `raw-jp`, milestone v0.1): RawPlayer.vue + Other Subs Panel — Specification

**Workstream:** `raw-jp`
**Milestone:** v0.1 Raw Provider MVP
**Source design doc:** `/data/animeenigma/docs/superpowers/specs/2026-05-18-raw-jp-provider-design.md`
**Requirements:** RAW-05, RAW-06
**Depends on:** Phase 1 + Phase 2 endpoints
**Mode:** `--auto`

## Goal

Build two new Vue components: `RawPlayer.vue` (HLS video + primary subtitle picker) and `OtherSubsPanel.vue` (modal listing every available subtitle track grouped by language with provider attribution).

## Background

**Today, two things are true and need to change:**

1. **No player exposes a JP audio + multi-language sub experience.** The four current players (`KodikPlayer`, `AnimeLibPlayer`, `HiAnimePlayer`, `ConsumetPlayer`) all serve dubbed audio. JP subtitles via Jimaku are surfaced inline in HiAnime/Consumet's flyout settings panel, but the UX treats subtitles as a power-user concern. For a raw JP audio player, subtitles are the *primary* control surface — the whole point of using raw is that you depend on the subs.

2. **Subtitle picker is buried.** `HiAnimePlayer.vue` and `ConsumetPlayer.vue` expose subtitle selection inside the Video.js settings menu (gear icon → Captions → list). Users browsing for a different RU fansub track have to dig three clicks deep. A dedicated "Other subs" surface that shows everything available, grouped by language with provider attribution, eliminates this friction.

**The implementation:**
- `RawPlayer.vue` mirrors the HLS+SubtitleOverlay structure of `HiAnimePlayer.vue` but with the subtitle picker promoted to a primary toolbar control. Reuses `SubtitleOverlay.vue` and `subtitle-parser.ts` unchanged.
- `OtherSubsPanel.vue` is a modal/sheet UI listing every track from `/api/anime/{id}/subtitles/all`, grouped by ISO language, with a provider attribution chip per row (Jimaku / OpenSubtitles).

## Requirements

### RAW-05: RawPlayer.vue

- **Current:** No `frontend/web/src/components/player/RawPlayer.vue` file.
- **Target:** New component with the following structure:
  - Template:
    - Episode list sidebar (mirror `HiAnimePlayer.vue` pattern).
    - Video element + Video.js/HLS.js switchable wrapper (mirror existing players).
    - **Primary toolbar** below the video with three controls:
      1. Subtitle language picker (dropdown of available languages from `/subtitles?lang=ru,en,ja` response — user's `preferred_subtitle_language` localStorage default).
      2. "Other subs" button (opens `OtherSubsPanel`).
      3. Quality picker (existing pattern from HiAnimePlayer).
    - `SubtitleOverlay.vue` mounted with current track, teleports to fullscreen target.
    - Three-phase loader (mirror EnglishPlayer.vue: looking up sources / connecting / verifying playback).
  - Script setup:
    - Fetches `/api/anime/{id}/raw/episodes` on mount.
    - On episode click → fetches `/raw/stream?episode={n}` + `/subtitles?lang=ru,en,ja&episode={n}` in parallel.
    - Default subtitle pick: user's preferred language → first available track. Loads track URL into `SubtitleOverlay` via `subtitle-parser.ts`.
    - Watch on `selectedTrack` ref → re-parse and re-mount overlay.
- **Acceptance:** Standalone render in a Storybook-style test page (or in `Anime.vue` via dev flag) shows a working HLS video, a subtitle picker populated with at least one language, and a working "Other subs" button.

### RAW-06: OtherSubsPanel.vue

- **Current:** No such component. Subtitle selection lives inside Video.js settings menu (HiAnime/Consumet).
- **Target:** New component:
  - Props: `shikimoriId: string`, `episode: number`, `currentTrackUrl: string | null`.
  - On mount, fetches `/api/anime/{shikimoriId}/subtitles/all?episode={episode}`.
  - Template:
    - Modal/sheet UI (use existing `Modal.vue` or `Sheet.vue` from `frontend/web/src/components/ui/` — mirror the codebase's existing modal pattern).
    - Group results by ISO language code; render a section per language with a localized header (e.g. "日本語 (3)", "English (5)", "Русский (2)").
    - Each row shows: provider chip ("Jimaku", "OpenSubtitles", future "Kage"), release name, file size if known, "Select" button.
    - Highlight the currently-selected track.
  - Emits `select-track(track: SubtitleTrack)` up to `RawPlayer`.
  - Locale strings live in `frontend/web/src/locales/{en,ru,ja}.json` under `player.otherSubs.*`.
- **Acceptance:** Opening the panel for Bocchi the Rock (Shikimori 52082) shows ≥2 language groups, each with ≥1 row. Selecting a row closes the panel and the SubtitleOverlay re-renders with the new track within 1 second.

## Acceptance Criteria

1. `frontend/web/src/components/player/RawPlayer.vue` exists and renders without errors.
2. `frontend/web/src/components/player/OtherSubsPanel.vue` exists and renders without errors.
3. `frontend/web/src/api/client.ts` extended with:
   - `rawApi.getEpisodes(shikimoriId): Promise<Episode[]>`
   - `rawApi.getStream(shikimoriId, episode, quality?): Promise<Stream>`
   - `subtitlesApi.byLang(shikimoriId, episode, langs[]): Promise<GroupedSubs>`
   - `subtitlesApi.all(shikimoriId, episode): Promise<GroupedSubs>`
4. Type definitions in `frontend/web/src/types/` for `Episode`, `Stream`, `SubtitleTrack`, `GroupedSubs`.
5. RawPlayer auto-picks the user's `preferred_subtitle_language` (default `ja` if unset) on first load.
6. "Other subs" button visibly distinct from the standard captions picker. Click opens OtherSubsPanel.
7. Selecting a track in OtherSubsPanel updates the SubtitleOverlay within 1s without page reload.
8. Locale strings in en/ru/ja for: `player.otherSubs.title`, `player.otherSubs.empty`, `player.otherSubs.providerChip.jimaku`, `player.otherSubs.providerChip.opensubtitles`, `player.subtitlePicker.label`, `player.subtitlePicker.none`.
9. No `[intlify]` warnings on page load.
10. Vue + TypeScript build (`bun run build`) passes with zero errors.

## Auto-selected implementation decisions

- **Player tech:** Video.js + HLS.js, switchable via `playerType` ref (mirror `HiAnimePlayer.vue:53-60`). Native HLS path is the fallback for Safari.
- **Modal vs sheet:** Modal centered on desktop, bottom sheet on mobile (mirror existing `Modal.vue` responsive behavior).
- **Provider chip styling:** Reuse the existing `Chip.vue` or `Badge.vue` component if present; fall back to a new inline class otherwise. Decide during plan-phase by inspecting `frontend/web/src/components/ui/`.
- **Empty-state copy:** "Subtitles are not yet available for this episode. Try checking back later or selecting a different episode." Localized.
- **Default language pick precedence:** `preferred_subtitle_language` localStorage → user's UI locale (`i18n.locale`) → first available track.
- **Hot-swap behavior:** When user selects a new track, the existing track is unmounted, parsed, re-mounted via the SubtitleOverlay's existing API (no SubtitleOverlay changes needed). RawPlayer should preserve current playback time across the swap.

## Touches

- **New:** `frontend/web/src/components/player/RawPlayer.vue`
- **New:** `frontend/web/src/components/player/OtherSubsPanel.vue`
- **Extend:** `frontend/web/src/api/client.ts` — new `rawApi` and `subtitlesApi` modules
- **Extend:** `frontend/web/src/types/` — new types (file location decided during plan-phase based on existing convention)
- **Extend:** `frontend/web/src/locales/{en,ru,ja}.json` — new `player.otherSubs.*` keys
- **Reused unchanged:** `frontend/web/src/components/player/SubtitleOverlay.vue`, `frontend/web/src/utils/subtitle-parser.ts`

## Out of Scope (for this phase)

- Integrating RawPlayer into `Anime.vue` provider switcher — Phase 4.
- Feature flag plumbing — Phase 4.
- Playwright e2e — Phase 4.
- Changelog entry — Phase 4.

## Citations to design doc

- Architecture → "frontend/web/src/components/player/RawPlayer.vue (NEW)"
- Architecture → "frontend/web/src/components/player/OtherSubsPanel.vue (NEW)"
- Data flow → "RawPlayer.vue receives episodes + stream URLs … 'Other subs' button opens OtherSubsPanel"
- Tech-choices table → "Other-subs UI: Modal panel triggered from player toolbar"
