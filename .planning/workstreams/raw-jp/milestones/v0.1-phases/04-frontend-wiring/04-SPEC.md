---
id: RAW-frontend-wiring
title: Anime.vue provider chip integration, feature flag, Playwright e2e, changelog
workstream: raw-jp
milestone: v0.1
phase: 04
created_at: 2026-05-18
status: SPEC-ready
ambiguity_score: 0.15
mode: --auto
---

# Phase 04 (workstream `raw-jp`, milestone v0.1): Frontend Wiring + Changelog — Specification

**Workstream:** `raw-jp`
**Milestone:** v0.1 Raw Provider MVP
**Source design doc:** `/data/animeenigma/docs/superpowers/specs/2026-05-18-raw-jp-provider-design.md`
**Requirements:** RAW-07, RAW-08, RAW-NF-02
**Depends on:** Phase 3 components
**Mode:** `--auto`

## Goal

Integrate the new `'raw'` provider into `Anime.vue`'s language-grouped provider chip switcher as a third "RAW JP" group. Add Playwright e2e against `ui_audit_bot` covering the happy path. Write the user-facing changelog entry. Gate visibility behind `VITE_RAW_PROVIDER_ENABLED` feature flag.

## Background

**Today, two things are true and need to change:**

1. **Provider chip switcher has two language groups: RU and EN.** `frontend/web/src/views/Anime.vue:1944-1968` switches the active video provider via a `preferred_video_provider` localStorage value with options `'kodik' | 'animelib' | 'hianime' | 'consumet' | 'hanime' | 'english'`. The language-group localStorage keys are `preferred_ru_provider` (kodik, animelib) and `preferred_en_provider` (english, hianime, consumet). No "raw" group exists.

2. **No e2e test covers raw playback.** Existing Playwright tests under `frontend/web/e2e/` cover the EN HLS path (`hianime-integration.spec.ts`) and the dubbed RU path. A new raw-player.spec.ts is needed to lock in the v0.1 happy path against `ui_audit_bot`.

**The implementation:**
- Extend the `preferred_video_provider` type union with `'raw'`.
- Add a third `preferred_raw_provider` localStorage key (single-option for v0.1 — `'allanime'` — but the key exists for v0.2's hybrid resolver to opt into `'minio'`).
- Add a "RAW JP" chip group in the switcher with one chip ("AllAnime").
- Gate the entire chip on `VITE_RAW_PROVIDER_ENABLED` so we can ship dark and flip it on after live testing.
- Add Playwright e2e: login as `ui_audit_bot`, navigate to a known AllAnime-covered anime, switch to RAW JP, assert RawPlayer loads, assert subtitle picker populated, open "Other subs" panel, switch language track, assert overlay updates.
- Write the changelog entry in all three locales.

## Requirements

### RAW-07: Provider chip integration in Anime.vue

- **Current:** Two language-group chips (RU, EN). `preferred_video_provider` union excludes `'raw'`. No `preferred_raw_provider` localStorage.
- **Target:**
  - Add `'raw'` to the `preferred_video_provider` union in `frontend/web/src/types/player.ts` (create the file if it doesn't exist; otherwise extend the existing definition — discover during plan-phase).
  - Add a `'RAW JP'` language group below the existing RU and EN groups in the provider chip switcher.
  - For v0.1 the group contains a single chip: "AllAnime" (provider key `'raw'`, since the resolver is the only raw source).
  - Add `preferred_raw_provider` localStorage key (default `'raw'`, single-option for v0.1).
  - Provider preference persists across reload — same pattern as `preferred_ru_provider` / `preferred_en_provider`.
  - When `'raw'` is the selected provider, `Anime.vue` lazy-loads and mounts `RawPlayer.vue` (mirror the `EnglishPlayer`/`HiAnimePlayer` `defineAsyncComponent` pattern at `Anime.vue:1042-1048`).
  - **Feature flag:** Wrap the RAW JP chip group in `v-if="rawProviderEnabled"`. Define `const rawProviderEnabled = import.meta.env.VITE_RAW_PROVIDER_ENABLED === 'true'`.
  - Locale strings for the language-group label: `provider.languageGroup.rawJp` in en/ru/ja.
- **Acceptance:** With `VITE_RAW_PROVIDER_ENABLED=true`, opening any anime shows three language-group chips. Selecting RAW JP loads RawPlayer. Refresh preserves selection. With the flag false (default), the chip is hidden and the page renders identically to today.

### RAW-08: Playwright e2e against ui_audit_bot

- **Current:** No e2e covers the raw player path. Existing tests under `frontend/web/e2e/` cover HiAnime, list management, and search flows.
- **Target:** New file `frontend/web/e2e/raw-player.spec.ts`. Test scenario:
  1. Login as `ui_audit_bot` via `/api/auth/login` (use the password documented in CLAUDE.md UI Audit Test User section — `audit_bot_test_password_2026`).
  2. Set `VITE_RAW_PROVIDER_ENABLED` for the test run (via `.env.test` or test runner config).
  3. Navigate to `/anime/52082` (Bocchi the Rock — known AllAnime coverage).
  4. Assert: three language-group chips visible.
  5. Click "RAW JP" → click "AllAnime" chip.
  6. Assert: RawPlayer mounts within 5s. Episode list visible.
  7. Click episode 1.
  8. Assert: video element receives a `src` attribute pointing to an HLS playlist (or HLS.js is initialized).
  9. Assert: subtitle picker dropdown has at least one option.
  10. Click "Other subs" button.
  11. Assert: OtherSubsPanel modal opens, shows ≥1 language group.
  12. Click a different-language track.
  13. Assert: panel closes, SubtitleOverlay updates (overlay element receives new content within 1s).
- **Acceptance:** `bunx playwright test raw-player` passes locally and in CI.

### RAW-NF-02: Changelog entry

- **Current:** `frontend/web/public/changelog.json` has entries for prior features.
- **Target:** New entry under the next version section (decide during plan-phase by inspecting the existing changelog). Three locales (en/ru/ja) following the established tone (informative + enthusiastic, emoji-light, single-paragraph summary with bullet list of new capabilities). Mention:
  - New "RAW JP" video provider option for original Japanese audio.
  - New "Other subs" button surfaces all available subtitle tracks across providers, grouped by language.
  - Multi-language subtitle support powered by Jimaku + OpenSubtitles aggregation.
- **Acceptance:** After `make redeploy-web`, the LastUpdates.vue Changelog tab shows the new entry in all three locales.

## Acceptance Criteria

1. `frontend/web/src/views/Anime.vue` has a third "RAW JP" language-group chip wired to lazy-load RawPlayer.
2. `preferred_raw_provider` localStorage key set on selection, restored on reload.
3. `VITE_RAW_PROVIDER_ENABLED=false` (default) hides the chip and renders the page identically to pre-RAW state.
4. `VITE_RAW_PROVIDER_ENABLED=true` shows the chip and successfully loads RawPlayer for Bocchi the Rock.
5. `frontend/web/e2e/raw-player.spec.ts` exists and passes locally (`bunx playwright test raw-player`).
6. Changelog entry visible in en/ru/ja in the LastUpdates.vue Changelog tab after redeploy.
7. `bun run build` passes with zero TypeScript errors.
8. `bunx eslint src/` passes with zero new warnings.

## Auto-selected implementation decisions

- **Type definitions location:** Inspect `frontend/web/src/types/` during plan-phase. If a `player.ts` exists, extend it. If not, search `Anime.vue` and provider components for the `PreferredVideoProvider` type definition and extend at its source.
- **Feature flag default:** `false` in `.env.example`; explicitly set `true` in `frontend/web/.env.development` and `.env.test`; production default `false` until live testing on `ui_audit_bot` validates.
- **Chip styling:** Mirror the existing RU/EN language-group chip pattern at `Anime.vue:390-410` exactly. Same spacing, same border, same hover.
- **Playwright test data:** Use Shikimori ID 52082 (Bocchi the Rock — known AllAnime coverage, popular enough to have ≥1 JP and ≥1 EN sub track).
- **Playwright auth approach:** Use existing helper if one exists under `frontend/web/e2e/` (search during plan-phase); otherwise inline the `fetch('/api/auth/login', ...)` pattern documented in CLAUDE.md.
- **Changelog version key:** Inspect `changelog.json` during plan-phase and use the next version slot (typically the next minor bump from the current latest).

## Touches

- **Extend:** `frontend/web/src/views/Anime.vue` — provider chip group, RawPlayer import, feature flag
- **Extend:** `frontend/web/src/types/player.ts` (or wherever `PreferredVideoProvider` lives)
- **Extend:** `frontend/web/.env.example`, `.env.development`, `.env.test`, `vite.config.ts` if needed
- **Extend:** `frontend/web/src/locales/{en,ru,ja}.json` — `provider.languageGroup.rawJp`
- **Extend:** `frontend/web/public/changelog.json` — new entry × 3 locales
- **New:** `frontend/web/e2e/raw-player.spec.ts`

## Out of Scope (for this phase)

- Backend changes — all done in Phases 1 + 2.
- Frontend components — all done in Phase 3.
- Live ui_audit_bot validation (smoke test) — happens during after-update / verification step post-execute.
- Self-hosted MinIO library — v0.2.

## Citations to design doc

- Architecture → "frontend/web/src/views/Anime.vue (EXTENDED) — Add 'raw' to preferred_video_provider type union; Add raw chip in provider switcher, third language group 'RAW JP'"
- Rollout → "v0.1 (streaming-only) — additive, no migration. Behind feature flag RAW_PROVIDER_ENABLED until live testing on ui_audit_bot passes"
- Testing → "RawPlayer.vue — Playwright e2e: open known AllAnime-backed anime as ui_audit_bot, assert raw video loads, subtitle picker populates, 'Other subs' panel opens"
