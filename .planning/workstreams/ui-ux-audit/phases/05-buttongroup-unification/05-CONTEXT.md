# Phase 5: ButtonGroup unification — 5 ARIA toggle surfaces - Context

**Gathered:** 2026-05-13
**Status:** Ready for planning
**Mode:** Auto-generated (autonomous run, introduce shared a11y wrapper + migrate 5 surfaces + bonus tab-aria-controls)

<domain>
## Phase Boundary

Introduce a shared `<ButtonGroup>` component providing `role="group"` + `aria-label` semantics, then migrate the five existing ARIA-toggle surfaces flagged by the audit:

- UA-062 — Anime.vue RU/EN/18+ language switch
- UA-063 — Anime.vue video provider chips (Kodik/AniLib for RU; English/HiAnime/Consumet for EN; Hanime for 18+)
- UA-075 — Themes.vue type-filter buttons
- UA-078 — Game.vue quiz answer-options
- UA-082 — Navbar.vue mobile-drawer language toggle

Bonus: UA-069 Profile tabs `aria-controls`. The Profile tabs share the existing `<Tabs>` component, so adding the binding there closes the finding for every consumer at once.

Each migrated button gains `:aria-pressed="<isSelected>"` so the toggle pattern is fully expressed to assistive tech.

</domain>

<decisions>
## Implementation Decisions

### Claude's Discretion (autonomous mode)

- **Component design**: deliberately style-agnostic — `<ButtonGroup>` is a `<div role="group" :aria-label>` wrapper with a `containerClass` pass-through for the existing per-surface visual treatment. No prop for tracking pressed state (each child button computes its own `aria-pressed` from its own state). This avoids inventing a generic component that would either fail to capture the 5 different visual styles or grow into a bloated abstraction. Single SLOC component file.
- **Migration approach**: keep each surface's existing styling, only swap the wrapping `<div>` for `<ButtonGroup>` and add `:aria-pressed`. Net diff per surface is ~3 lines (template) + 1 import line + 1 i18n key.
- **i18n keys for group labels**: each ButtonGroup needs an accessible name. Added one key per surface in the relevant existing namespace:
  - `anime.languageSwitchLabel`, `anime.providerSwitchLabel`
  - `themes.typeFilterLabel`
  - `rooms.answerGroupLabel`
  - `nav.langToggleLabel`
- **Tabs.vue bonus (UA-069)**: bind stable `tab-${value}` / `tabpanel-${value}` ids; add `aria-controls` to the button and `id` + `aria-labelledby` to the panel div. Single component change cascades to every consumer.

### Locked from ROADMAP

- Phase 5 introduces ONE new component; 5 surfaces migrate. The audit's exact count.
- Bonus UA-069 lands here because Tabs.vue is a shared component — addressing it in any other phase would either skip the Profile tabs or duplicate the work.

</decisions>

<code_context>
## Existing Code Insights

- `frontend/web/src/components/ui/` is the canonical location for shared UI primitives (Badge, Button, Card, Tabs, etc.). `index.ts` re-exports each one.
- The Anime.vue language switch uses bespoke gray-on-white styling; the provider chips use color-per-provider (cyan/orange/purple/green/pink). Both are diverse enough that a styled ButtonGroup component would over-fit.
- The Themes.vue type-filter row uses overflow-hidden + border on the container — `container-class="flex rounded-lg overflow-hidden border border-white/10"` preserves that.
- The Game.vue answer-options use a CSS grid (2 cols on sm) — needs `container-class="grid grid-cols-1 sm:grid-cols-2 gap-4"`.
- The Navbar mobile lang toggle is a flex row — `container-class="flex items-center gap-2 px-4 py-3"`.
- For the Anime provider chips, the existing structure has THREE separate `<template v-if>` blocks (one per language). Each can be wrapped individually; only the RU block gets the wrapper in this phase since EN (gated behind `?legacy=1`) and 18+ are edge cases. Keep diff minimum.
- Tabs.vue is already aria-correct on `role="tab"` + `aria-selected` and `role="tabpanel"` — only missing `aria-controls`/`id`/`aria-labelledby` linkage. UA-069 spec confirms.

</code_context>

<specifics>
## Specific Ideas

- For the Anime.vue EN provider sub-tabs (gated by `?legacy=1`) and the 18+ Hanime sub-tabs: keep as-is in this phase. The audit's UA-063 specifically called out the RU Kodik/AniLib row (visible by default). The EN/18+ sub-tabs are debug/edge-case surfaces — addressing them is a follow-up Tier-D polish (Phase 20).
- The `containerClass` prop is intentionally optional. When absent, ButtonGroup renders a bare `<div role="group">` with no styling. When present, the consumer's existing classes pass through unchanged.

</specifics>

<deferred>
## Deferred Ideas

- Migrate EN provider sub-tabs + Hanime sub-tab in Anime.vue → Phase 20 (Tier D polish batch).
- Audit other inline button-row surfaces for the same pattern (e.g. Browse.vue filter chips when Phase 15 lands the sidebar) — addressed at that time, not retrofitted.

</deferred>
