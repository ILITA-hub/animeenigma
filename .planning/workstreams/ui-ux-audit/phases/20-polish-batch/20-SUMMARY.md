---
phase: 20
plan: 1
subsystem: ui-ux-audit
tags: [frontend, vue3, i18n, polish, milestone-final, localStorage, players, faq]
requires: [phase-11, phase-14, phase-16, phase-18]
provides:
  - skip-intro-cta-auto-dismiss
  - faq-accordion-smooth-transition
  - drawer-schedule-link-verified
  - kebab-focus-visible-verified
  - quicknav-section-id-alignment-verified
affects:
  - HiAnimePlayer / ConsumetPlayer (Skip-Intro CTA visibility window)
  - Profile Settings tab (new Player card with timeout input)
  - About.vue FAQ (no-JS open transition smoothing)
tech-stack:
  added: []
  patterns:
    - singleton-localStorage-composable-with-cross-tab-sync
    - watcher-armed-setTimeout-for-cta-auto-dismiss-with-per-window-reset
    - native-details-max-height-transition-for-faq-accordion
key-files:
  created:
    - frontend/web/src/composables/useSkipIntroSettings.ts
  modified:
    - frontend/web/src/components/player/HiAnimePlayer.vue
    - frontend/web/src/components/player/ConsumetPlayer.vue
    - frontend/web/src/views/Profile.vue
    - frontend/web/src/views/About.vue
    - frontend/web/src/locales/en.json
    - frontend/web/src/locales/ru.json
    - frontend/web/src/locales/ja.json
decisions:
  - skip-intro-dismiss-state-is-per-component-per-OP-or-ED-window-not-global
  - composable-singleton-module-level-ref-so-profile-edits-propagate-to-active-players
  - cross-tab-sync-via-storage-event-listener-mounted-once-at-module-load
  - clamp-range-2-to-60-default-8-out-of-range-falls-back-to-default
  - faq-transition-is-pure-css-on-answer-paragraph-not-the-details-host
  - drawer-schedule-entry-already-rendered-via-v-for-navLinks-no-duplicate-added
  - animekebab-focus-visible-reveal-already-present-since-phase-7-revamp-no-op
  - quicknav-section-id-drift-check-found-4-of-4-matching-no-changes-needed
metrics:
  duration: ~10min
  completed: 2026-05-13
  commits: 2
  tasks_complete: 5
  tasks_total: 5
---

# Phase 20 Summary: Tier D — polish batch (milestone v0.1 final)

**One-liner:** Final v0.1 polish: Skip-Intro/Skip-Outro CTAs auto-dismiss after a configurable timeout, About FAQ accordion now expands smoothly via max-height transition, and three legacy polish items (UA-085 drawer link, AnimeKebab focus-visible reveal, AnimeQuickNav section-ID alignment) were verified already-fixed by prior phases.

## What landed

| Area | Mechanism |
|---|---|
| **Skip-Intro/Skip-Outro auto-dismiss** | New `useSkipIntroSettings()` composable (localStorage-backed, range [2..60], default 8). HiAnimePlayer + ConsumetPlayer split their `showSkipIntro` / `showSkipOutro` computed into a `*Raw` window-check + a `dismissed` ref gated by a `setTimeout` armed on the watcher's positive edge. The timer + flag both reset on the window's negative edge so the next OP/ED window (or next episode) shows the CTA fresh. `onBeforeUnmount` clears both timers. |
| **FAQ accordion smooth transition** | About.vue `<style scoped>` adds `details > p { max-height: 0; opacity: 0; transition: max-height 200ms, opacity 150ms, margin-top 200ms }` + `details[open] > p { max-height: 1000px; opacity: 1; margin-top: 0.75rem }`. Zero-JS, preserves SEO-friendly always-in-DOM model, chevron rotation animation unchanged. |
| **Profile Settings input** | New "Player" glass-card above the API Key card with a numeric input bound to `skipIntroSettings.seconds`. `@change` handler reads the raw value, clamps via `set()`, then writes the normalized value back into the input so out-of-range typing visibly corrects itself. |
| **i18n** | 3 new key paths × 3 locales = 9 entries: `profile.settings.player`, `profile.settings.skipIntroDismissSec.label`, `profile.settings.skipIntroDismissSec.hint`. |

## Plan deviations

Three of the five planned items turned out already-fixed; documented per plan deviation policy ("if any item turns out trivially absent (already done) ... document and skip — don't expand scope").

| Plan task | Reality | Action |
|---|---|---|
| UA-085 — add `<router-link to="/schedule">` between Profile/Login and Language toggle in mobile drawer | `frontend/web/src/components/layout/Navbar.vue` line 315: `/schedule` is already in the `navLinks` array, which the drawer's v-for at line 206 renders at the top. Adding a second link would be a duplicate Schedule entry. | Verified-only. No code change. |
| AnimeKebab focus polish — add `focus-visible:opacity-100` | `frontend/web/src/components/anime/AnimeKebab.vue` line 11 already has `focus-visible:opacity-100 focus-visible:scale-100 focus-visible:animate-kebab-glow`. Predates Phase 20 (added in `76cfad3 feat(ui): revamp anime card context menu — kebab, keyboard nav, hero cards`). | Verified-only. No code change. |
| AnimeQuickNav drift check — verify 4 section IDs match | `grep -c 'id="section-' frontend/web/src/views/Anime.vue` → 4 matches (`section-overview`, `section-episodes`, `section-similar`, `section-comments`); `grep -c "id: 'section-" frontend/web/src/components/anime/AnimeQuickNav.vue` → 4 matches. IDs match exactly. | Verified-only. No code change. |

These three were "discovery" items in the audit (UA-085 was tagged "1 minor (discovery)" in `docs/issues/ui-audit-2026-05-12/themes-schedule-game-navbar.md`). They appear in PLAN.md because the audit author had not yet checked whether prior phases had already remediated them.

## Commits

| Commit | Subject | Files |
|---|---|---|
| `f1767e7` | feat(ui-ux-audit/20): Skip-Intro CTA auto-dismiss + setting | composables/useSkipIntroSettings.ts (new), player/HiAnimePlayer.vue, player/ConsumetPlayer.vue, views/Profile.vue, locales/{en,ru,ja}.json |
| `5858268` | polish(ui-ux-audit/20): About.vue FAQ accordion transitions | views/About.vue |

2 commits; each independently revertable. Touched 7 files (1 new composable, 2 players, 2 views, 3 locales).

## Findings closure

| Item | Surface | Status | Mechanism |
|---|---|---|---|
| UA-085 | Navbar drawer | ALREADY CLOSED | `/schedule` rendered via v-for over `navLinks` at line 206 (predates Phase 20). |
| AnimeKebab focus reveal | AnimeKebab.vue | ALREADY CLOSED | `focus-visible:opacity-100` since commit `76cfad3`. |
| Skip-Intro CTA noise | HiAnime + Consumet | CLOSED | `useSkipIntroSettings()` + per-window `setTimeout` dismissal; configurable in Profile Settings. |
| FAQ accordion jumpiness | About.vue | CLOSED | `max-height` + opacity + margin transition on `details > p` / `details[open] > p`. |
| AnimeQuickNav drift | Anime.vue + AnimeQuickNav.vue | VERIFIED-NO-DRIFT | 4 / 4 section IDs match: `section-overview`, `section-episodes`, `section-similar`, `section-comments`. |

**Phase 20 outcome:** PASSED. 5 / 5 polish tasks resolved (2 implemented, 3 verified already-fixed). Zero backend changes. Zero new dependencies. Closes UX-36 and the v0.1 milestone.

## Self-Check: PASSED

- File `frontend/web/src/composables/useSkipIntroSettings.ts` — FOUND (74 lines)
- File `frontend/web/src/components/player/HiAnimePlayer.vue` — FOUND (modified — `useSkipIntroSettings` import + dismissal watchers + timers)
- File `frontend/web/src/components/player/ConsumetPlayer.vue` — FOUND (modified — same pattern as HiAnime)
- File `frontend/web/src/views/Profile.vue` — FOUND (modified — Player card + composable wiring)
- File `frontend/web/src/views/About.vue` — FOUND (modified — `<style scoped>` extended with FAQ transitions)
- Locales `frontend/web/src/locales/{en,ru,ja}.json` — FOUND (`skipIntroDismissSec` × 3 + `player` × 3)
- Commit `f1767e7` — FOUND
- Commit `5858268` — FOUND
