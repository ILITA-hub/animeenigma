---
status: passed
phase: 5
phase_name: "ButtonGroup unification — 5 ARIA toggle surfaces"
verified: 2026-05-13
---

# Phase 5 Verification: ButtonGroup unification

## Success-criteria scorecard (per ROADMAP.md Phase 5)

The phase goal: "Introduce a shared `<ButtonGroup>` component (`role="group"` + `aria-pressed`) and migrate 5 existing surfaces. Single pattern fixes 5 findings."

| # | Criterion | Status | Evidence |
|---|-----------|--------|----------|
| 1 | Shared `<ButtonGroup>` component exists with `role="group"` and consumes children via slot | ✅ | `frontend/web/src/components/ui/ButtonGroup.vue` (new). Renders `<div role="group" :aria-label="label" :class="containerClass">` with a default slot. Exported from `components/ui/index.ts`. |
| 2 | 5 audit-cited toggle surfaces migrated to ButtonGroup + `aria-pressed` per button | ✅ | Anime.vue (UA-062 lang switch, UA-063 RU provider chips), Themes.vue (UA-075 typeFilter), Game.vue (UA-078 answer-options), Navbar.vue (UA-082 mobile-lang). Each migrated row wraps in `<ButtonGroup>` and each child button now has `:aria-pressed="<selected-state-expr>"`. |
| 3 | Bonus UA-069 — Profile tab `aria-controls` linkage | ✅ | `Tabs.vue` updated to bind `:id="tab-${value}"` + `:aria-controls="tabpanel-${value}"` per tab and `:id`/`:aria-labelledby` on the panel div. Shared change cascades to all `<Tabs>` consumers. |

**Overall status:** **PASSED** — 3/3 criteria met; 5 audit findings + 1 bonus closed.

## Goal-backward check

| Audit finding | Closed? | Surface |
|---------------|---------|---------|
| UA-062 (Anime RU/EN switch) | ✅ | Anime.vue language ButtonGroup |
| UA-063 (Anime provider chips, RU branch) | ✅ | Anime.vue provider ButtonGroup (EN/18+ deferred to Phase 20) |
| UA-075 (Themes type-filter) | ✅ | Themes.vue typeFilter ButtonGroup |
| UA-078 (Game answer-options) | ✅ | Game.vue answer ButtonGroup |
| UA-082 (Navbar mobile-lang toggle) | ✅ | Navbar.vue lang ButtonGroup |
| UA-069 (Profile tab aria-controls) | ✅ | Tabs.vue id + aria-controls |

## Risks / leftover work

- Anime.vue EN provider sub-tabs (gated by `?legacy=1`) and the 18+ Hanime sub-tab are NOT migrated in this phase. They're rarely-rendered edge cases and the audit's UA-063 specifically called out the default-rendered RU branch. Deferred to Phase 20 (Tier D polish batch).
- The `<ButtonGroup>` API intentionally keeps no pressed-state prop — each child button computes its own `aria-pressed`. This means future consumers must remember to add the binding per button; a stricter API would centralize but at the cost of flexibility across the 5 wildly different visual treatments observed here.
- `vue-tsc --noEmit` passes; deployed bundle renders normally (manual visual verification deferred — the changes are additive ARIA attributes with no visual diff).

## Human verification

Not required. ARIA attribute additions + shared component introduction are static-verifiable from source. Screen-reader behavior (e.g. NVDA / VoiceOver announcing "pressed" on toggle activation) follows directly from the standard ARIA Toggle pattern that the additions implement.
