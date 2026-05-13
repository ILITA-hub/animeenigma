# Phase 2 Plan: Tier B — Quick-wins batch

**Status:** Active
**Plan #:** 1 (single batched plan, four sub-batches)
**Created:** 2026-05-13

## Sub-batches

### B1 — i18n leaks (UX-03)

- [ ] Add `nav.openMenu`, `nav.closeMenu`, `nav.closeSearch`, `nav.scheduleLink` keys in en/ru/ja.
- [ ] Add `errors.fetchAnime`, `errors.fetchAnimeList` keys in en/ru/ja.
- [ ] `Navbar.vue` L172: replace `'Close menu' : 'Open menu'` literals with `$t('nav.closeMenu') : $t('nav.openMenu')`.
- [ ] `useAnime.ts`: wire `useI18n` into the composable; replace `'Failed to fetch anime'` and `'Failed to fetch anime list'` with `t('errors.fetchAnime')` / `t('errors.fetchAnimeList')`.

### B2 — Dynamic titles (UX-04)

- [ ] `Anime.vue`: add `watch(() => anime.value?.title, ...)` that sets `document.title = '${title} — AnimeEnigma'`.
- [ ] `Profile.vue`: add `watch(() => profileUser.value?.username, ...)` that sets `document.title = '${username} — AnimeEnigma'`.

### B3 — Aria-label batch (UX-05)

- [ ] `Home.vue` L14: add `:aria-label="$t('nav.scheduleLink')"` to the Schedule icon link (mobile icon-only).
- [ ] `Auth.vue` L20: insert a `<h1 class="sr-only">{{ $t('auth.heading') }}</h1>` above the existing h2.
- [ ] `Auth.vue` L43: add `role="img"` + `:aria-label="$t('auth.qrAlt')"` to the QR canvas.
- [ ] `Navbar.vue` L105-112: add `:aria-label="$t('nav.closeSearch')"` to the search-close icon button.
- [ ] `AdminRecs.vue` L21-28: add a non-changing `:aria-label="$t('admin.recs.forceRecompute')"` so the SR announcement stays consistent across busy/idle states.

### B4 — Tier-A-adjacent quick wins (UX-06)

- [ ] `Navbar.vue` `navLinks`: insert `{ to: '/schedule', label: 'nav.schedule' }` so the mobile drawer surfaces /schedule.
- [ ] `Home.vue` rec-card `<img>`: switch from `:alt="getLocalizedTitle(...)"` to `alt=""` (decorative — the visible card title below already announces the name).
- [ ] Update `profile.import.malPlaceholder` + `profile.import.shikimoriPlaceholder` to mention URL acceptance in all three locales. Also update `malDescription` + `shikimoriDescription` to remove the now-incorrect "Enter your username, not a URL" warning.

## Verification

- [ ] `bunx vue-tsc --noEmit` — type-check passes.
- [ ] `make redeploy-web` — frontend rebuilt and shipped.
- [ ] `grep -rn '"Open menu"|"Close menu"|"Failed to fetch anime"' frontend/web/src/ --include="*.vue" --include="*.ts" --include="*.js"` — zero hits (excluding the locales/ directory, which contains the i18n VALUE strings).
- [ ] Hit deployed bundle and confirm the new keys (`closeMenu`, `closeSearch`, `qrAlt`, `fetchAnime`, etc.) are present.
- [ ] Source-check aria-label additions on the 5 surfaces in B3.

## Files touched

```
frontend/web/src/locales/en.json
frontend/web/src/locales/ru.json
frontend/web/src/locales/ja.json
frontend/web/src/composables/useAnime.ts
frontend/web/src/components/layout/Navbar.vue
frontend/web/src/views/Auth.vue
frontend/web/src/views/Home.vue
frontend/web/src/views/Anime.vue
frontend/web/src/views/Profile.vue
frontend/web/src/views/admin/AdminRecs.vue
.planning/workstreams/ui-ux-audit/phases/02-tier-b-quick-wins/
  02-CONTEXT.md
  02-PLAN.md
  02-SUMMARY.md
  02-VERIFICATION.md
```
