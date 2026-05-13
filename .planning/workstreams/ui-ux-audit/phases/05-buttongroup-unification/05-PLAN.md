# Phase 5 Plan: ButtonGroup unification

**Status:** Active
**Plan #:** 1
**Created:** 2026-05-13

## Tasks

### Component

- [ ] Create `frontend/web/src/components/ui/ButtonGroup.vue` — slot wrapper with `role="group"`, `:aria-label` prop, optional `containerClass` pass-through.
- [ ] Export from `frontend/web/src/components/ui/index.ts`.

### Migrations

- [ ] Anime.vue language switch → ButtonGroup + per-button `aria-pressed`. Closes UA-062.
- [ ] Anime.vue RU provider chips → ButtonGroup + `aria-pressed`. Closes UA-063 (RU branch; EN/18+ deferred to Phase 20).
- [ ] Themes.vue typeFilter → ButtonGroup + `aria-pressed`. Closes UA-075.
- [ ] Game.vue answer-options → ButtonGroup + `aria-pressed`. Closes UA-078.
- [ ] Navbar.vue mobile-lang toggle → ButtonGroup + `aria-pressed`. Closes UA-082.

### Bonus: UA-069 Profile tab aria-controls

- [ ] `Tabs.vue`: bind `:id="tab-${tab.value}"` + `:aria-controls="tabpanel-${tab.value}"` on each tab button; bind matching `:id="tabpanel-${modelValue}"` + `:aria-labelledby="tab-${modelValue}"` on the panel div. Shared component change cascades to all consumers including Profile tabs.

### i18n

- [ ] Add to `en.json` / `ru.json` / `ja.json`:
  - `anime.languageSwitchLabel`
  - `anime.providerSwitchLabel`
  - `themes.typeFilterLabel`
  - `rooms.answerGroupLabel`
  - `nav.langToggleLabel`

### Verification

- [ ] `bunx vue-tsc --noEmit` — passes.
- [ ] `make redeploy-web` — frontend rebuilt and shipped.
- [ ] `grep -n 'aria-pressed' frontend/web/src/views/{Anime,Themes,Game}.vue frontend/web/src/components/layout/Navbar.vue` — confirms bindings on 5 surfaces.
- [ ] `grep -n 'aria-controls\|aria-labelledby' frontend/web/src/components/ui/Tabs.vue` — confirms tab/panel linkage.

## Files touched

```
frontend/web/src/components/ui/ButtonGroup.vue    (new)
frontend/web/src/components/ui/index.ts           (export)
frontend/web/src/components/ui/Tabs.vue           (UA-069 bonus)
frontend/web/src/views/Anime.vue                  (UA-062 + UA-063)
frontend/web/src/views/Themes.vue                 (UA-075)
frontend/web/src/views/Game.vue                   (UA-078)
frontend/web/src/components/layout/Navbar.vue     (UA-082)
frontend/web/src/locales/en.json                  (+5 keys)
frontend/web/src/locales/ru.json                  (+5 keys)
frontend/web/src/locales/ja.json                  (+5 keys)
.planning/workstreams/ui-ux-audit/phases/05-buttongroup-unification/
  05-CONTEXT.md
  05-PLAN.md
  05-SUMMARY.md
  05-VERIFICATION.md
```
