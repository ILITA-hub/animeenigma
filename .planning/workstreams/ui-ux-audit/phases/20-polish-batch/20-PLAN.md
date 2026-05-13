# Phase 20 Plan: Tier D — polish batch

**Status:** Active
**Plan #:** 1
**Created:** 2026-05-13

Final phase. 5 small polish items. Scope is intentionally tight. Closes UX-36 + UA-085.

## Tasks

### Drawer Schedule entry (UA-085)

- [ ] In `frontend/web/src/components/layout/Navbar.vue` mobile drawer, add a `<router-link to="/schedule">{{ $t('nav.schedule') }}</router-link>` between the Profile/Login section and the Language toggle.
- [ ] Use the existing nav-link styling pattern (same `class` as the v-for nav-link block at line ~188).

### Kebab focus polish

- [ ] `frontend/web/src/components/anime/AnimeKebab.vue` — add `focus-within:opacity-100` (or move opacity-0 to outer wrapper + add `group-focus-within:opacity-100`) so keyboard navigation reveals the kebab.

### Skip-Intro CTA auto-dismiss (lightweight)

- [ ] Add a localStorage-backed reactive `skipIntroDismissSec` value (default 8) — create `frontend/web/src/composables/useSkipIntroSettings.ts` or inline in Player components.
- [ ] In HiAnimePlayer.vue + ConsumetPlayer.vue: when `showSkipIntro` becomes true, start a `setTimeout(N seconds)` that flips a local `dismissed` ref to hide the CTA. Reset on next OP window (i.e. next episode).
- [ ] Profile Settings tab: add input "Skip-Intro CTA visible for (seconds)" with min=2/max=60. Persists to localStorage. (LOOK at existing Profile.vue settings tab structure first.)
- [ ] i18n keys: `profile.settings.skipIntroDismissSec.label` (3 locales). Defer the units suffix into the i18n string itself.

### FAQ transition polish

- [ ] In About.vue or a global stylesheet (e.g. `frontend/web/src/assets/main.css`), add CSS:
  ```css
  details > div, details > p {
    overflow: hidden;
    transition: max-height 200ms ease-out;
    max-height: 0;
  }
  details[open] > div, details[open] > p {
    max-height: 1000px;
  }
  ```
  Or restructure About.vue to wrap each FAQ's answer paragraph in a `<div>` for smoother animation.

### AnimeQuickNav drift check

- [ ] Verify Anime.vue has `id="section-overview"`, `id="section-episodes"`, `id="section-similar"`, `id="section-comments"`. If any missing, add. If renamed, update AnimeQuickNav.vue references to match.
- [ ] grep `id="section-` Anime.vue: confirms 4 matches.

### Verification

- [ ] `cd frontend/web && bunx vue-tsc --noEmit` clean.
- [ ] `bash scripts/i18n-lint.sh` clean (1 new key × 3 locales).
- [ ] `make redeploy-web` succeeds.
- [ ] grep `to="/schedule"` in Navbar.vue (drawer link present).
- [ ] grep `focus-within\|focus-visible` in AnimeKebab.vue.
- [ ] Manual: open drawer, click Schedule — routes to /schedule. Tab through cards — kebab visible on focus.

## Files touched

```
frontend/web/src/components/layout/Navbar.vue          (UA-085 drawer Schedule entry)
frontend/web/src/components/anime/AnimeKebab.vue       (focus reveal)
frontend/web/src/composables/useSkipIntroSettings.ts   (new — small composable)
frontend/web/src/components/player/HiAnimePlayer.vue   (auto-dismiss timer)
frontend/web/src/components/player/ConsumetPlayer.vue  (auto-dismiss timer)
frontend/web/src/views/Profile.vue                     (settings input)
frontend/web/src/views/About.vue                       (FAQ transition CSS)
frontend/web/src/locales/en.json                       (+1 key)
frontend/web/src/locales/ru.json                       (+1 key)
frontend/web/src/locales/ja.json                       (+1 key)
.planning/workstreams/ui-ux-audit/phases/20-polish-batch/
  20-CONTEXT.md
  20-PLAN.md
  20-SUMMARY.md       (written at execute end)
  20-VERIFICATION.md  (written at execute end)
```

## Closes

| Item | Surface | Mechanism |
|---|---|---|
| UA-085 | Navbar drawer | New `/schedule` router-link in mobile drawer |
| (Polish) | Cards | Kebab focus-visible reveal |
| (Polish) | Players | Skip-Intro CTA auto-dismiss after configurable timeout |
| (Polish) | About FAQ | CSS transitions on `<details>` open/close |
| (Polish) | Anime detail | Verify QuickNav section IDs match |
