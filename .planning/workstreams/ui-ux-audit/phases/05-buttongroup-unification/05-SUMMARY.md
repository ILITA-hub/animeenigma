# Phase 5 Summary: ButtonGroup unification

**Completed:** 2026-05-13
**Plan:** 05-PLAN.md
**Outcome:** New `<ButtonGroup>` component shipped; 5 audit-cited toggle surfaces migrated; bonus UA-069 closed via Tabs.vue.

## Changes shipped

### New component

`frontend/web/src/components/ui/ButtonGroup.vue` — semantic wrapper that renders `<div role="group" :aria-label="label" :class="containerClass">` and exposes a slot for the children. Props: `label: string` (required, accessible name), `containerClass?: string` (optional, pass-through styling). Exported from `components/ui/index.ts`.

### Migrations (5 surfaces, each gains role=group + aria-label + per-button aria-pressed)

- **Anime.vue RU/EN/18+ language switch** (UA-062) — wraps the existing bg-white/5 rounded toggle.
- **Anime.vue RU provider chips** (UA-063) — wraps the Kodik/AniLib chip row via `container-class="contents"` to preserve the parent flex layout. EN provider sub-tabs (gated by `?legacy=1`) and 18+ Hanime sub-tab intentionally deferred to Phase 20.
- **Themes.vue type-filter row** (UA-075) — wraps the rounded-border filter strip.
- **Game.vue quiz answer-options** (UA-078) — wraps the responsive grid.
- **Navbar.vue mobile-drawer language toggle** (UA-082) — wraps the RU/EN/JA buttons.

### Bonus — UA-069 Profile tab aria-controls

`Tabs.vue` (shared component) — each tab button now sets `:id="tab-${value}"` + `:aria-controls="tabpanel-${value}"`. The panel div sets `:id="tabpanel-${modelValue}"` + `:aria-labelledby="tab-${modelValue}"`. Single shared-component change cascades to every consumer (Profile tabs + anywhere else `<Tabs>` is used).

### i18n

5 new label keys per locale (`en`/`ru`/`ja`):
- `anime.languageSwitchLabel` / `anime.providerSwitchLabel`
- `themes.typeFilterLabel`
- `rooms.answerGroupLabel`
- `nav.langToggleLabel`

## Verification

See `05-VERIFICATION.md` for the success-criteria scorecard.

## Files touched

```
frontend/web/src/components/ui/ButtonGroup.vue           (new)
frontend/web/src/components/ui/index.ts                  (+1 export)
frontend/web/src/components/ui/Tabs.vue                  (UA-069: id/aria-controls/aria-labelledby)
frontend/web/src/views/Anime.vue                          (UA-062 + UA-063 RU)
frontend/web/src/views/Themes.vue                         (UA-075 + import)
frontend/web/src/views/Game.vue                           (UA-078 + import)
frontend/web/src/components/layout/Navbar.vue             (UA-082 + import)
frontend/web/src/locales/en.json                          (+5 keys)
frontend/web/src/locales/ru.json                          (+5 keys)
frontend/web/src/locales/ja.json                          (+5 keys)
.planning/workstreams/ui-ux-audit/phases/05-buttongroup-unification/
  05-CONTEXT.md      (new)
  05-PLAN.md         (new)
  05-SUMMARY.md      (this file)
  05-VERIFICATION.md (new)
```

## Notes for downstream phases

- The ButtonGroup component is reusable for any future toggle row. Phase 15 (multi-axis catalog filter sidebar) and Phase 12 (AdminRecs SPA quality) should use it for their toggle surfaces rather than re-implementing the role=group pattern.
- The Tabs.vue aria-controls binding is now the reference pattern for any tab-shaped UI in the project.
- Anime.vue EN provider sub-tabs + Hanime sub-tab remain unmigrated — they're rarely-rendered edge cases that should land in Phase 20 (Tier D polish batch) alongside other low-priority cleanups.
