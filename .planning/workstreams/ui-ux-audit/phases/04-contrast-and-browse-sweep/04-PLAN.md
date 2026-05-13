# Phase 4 Plan: Color-contrast + Browse heading sweep

**Status:** Active
**Plan #:** 1
**Created:** 2026-05-13

## Tasks

### UX-10 — `text-white/40` → `/60` sweep

- [ ] `Anime.vue` — replace_all (closes UA-052 + UA-121)
- [ ] `Themes.vue` — replace_all (closes UA-074)
- [ ] `Schedule.vue` — replace_all (closes UA-076)
- [ ] `Game.vue` — replace_all (closes UA-077)
- [ ] `Navbar.vue` — replace_all (closes UA-086 + improves search dropdown subtitle)
- [ ] `Auth.vue` — replace_all (closes UA-072)
- [ ] `Profile.vue` — replace_all (closes UA-066 + improves all subtitle text in Profile)

### UX-11 — Browse a11y

- [ ] `GenreFilterPopup.vue`:
  - Add `aria-haspopup="listbox"` + `:aria-expanded="isOpen"` to the trigger button (closes UA-047).
  - Change placeholder span class from `text-white/30` → `text-white/60` (closes UA-046).
- [ ] `Browse.vue`: insert sr-only `<h2>{{ $t('browse.resultsHeading') }}</h2>` immediately before the results grid (closes UA-048).
- [ ] `locales/{en,ru,ja}.json`: add `browse.resultsHeading` key — "Results" / "Результаты" / "結果".

### Verification

- [ ] `bunx vue-tsc --noEmit` — passes.
- [ ] `make redeploy-web` — frontend rebuilt and shipped.
- [ ] `grep -rn 'text-white/40' frontend/web/src/{views,components/layout}` excluding player components — zero hits.
- [ ] `grep -n 'aria-haspopup\|aria-expanded' frontend/web/src/components/ui/GenreFilterPopup.vue` — confirms bindings.
- [ ] `grep -n 'sr-only' frontend/web/src/views/Browse.vue` — confirms heading.

## Files touched

```
frontend/web/src/views/Anime.vue
frontend/web/src/views/Themes.vue
frontend/web/src/views/Schedule.vue
frontend/web/src/views/Game.vue
frontend/web/src/views/Auth.vue
frontend/web/src/views/Profile.vue
frontend/web/src/views/Browse.vue
frontend/web/src/components/layout/Navbar.vue
frontend/web/src/components/ui/GenreFilterPopup.vue
frontend/web/src/locales/en.json
frontend/web/src/locales/ru.json
frontend/web/src/locales/ja.json
.planning/workstreams/ui-ux-audit/phases/04-contrast-and-browse-sweep/
  04-CONTEXT.md
  04-PLAN.md
  04-SUMMARY.md
  04-VERIFICATION.md
```
