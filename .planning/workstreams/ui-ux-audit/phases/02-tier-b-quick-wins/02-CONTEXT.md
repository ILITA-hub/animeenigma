# Phase 2: Tier B — Quick-wins batch - Context

**Gathered:** 2026-05-13
**Status:** Ready for planning
**Mode:** Auto-generated (autonomous run, batched fixes with concrete audit citations)

<domain>
## Phase Boundary

Close ~13 small Tier-B findings from the 2026-05-12 UX audit in one PR (~50 LOC across ~8 files). The 2026-04-20 expected delta that didn't ship as Batches G/H/I is mostly restored here. Bundled into four logical sub-batches per `docs/issues/ui-audit-2026-05-12/ranked-findings-with-metrics.md`:

- **B1 — i18n leaks** (UX-03): UA-043 Navbar "Open menu", UA-080 "Close menu" English literals; UA-050 `'Failed to fetch anime'` / `'Failed to fetch anime list'` literals in `useAnime.ts`. UA-073 (locale switcher "ru" with no aria-label) is technically Phase 6 navbar-drawer scope — handle there.
- **B2 — Dynamic titles** (UX-04): UA-051 Anime detail title; UA-068 Profile title.
- **B3 — Aria-label batch** (UX-05): UA-042 Home `/schedule` icon link; UA-070 Auth h1 sr-only; UA-071 QR canvas role+label; UA-081 Navbar search-close button; UA-099 AdminRecs recompute button.
- **B4 — Tier-A-adjacent quick wins** (UX-06): UA-055 drawer Schedule entry; UA-059 RecItem image `alt=""` (decorative; visible title adjacent); UA-067 import placeholders mention URL acceptance.

Out of scope for this phase: every other audit batch (C1 ButtonGroup, C2 contrast sweep, C3 drawer a11y, C5 dropdown a11y, etc.).

</domain>

<decisions>
## Implementation Decisions

### Claude's Discretion (autonomous mode)

- **i18n key namespace**: extend the existing `nav.*` family for menu/search-close keys; add an `errors.*` top-level for the composable-side fetch failures (TypeScript composables use `useI18n().t()` rather than `$t()`).
- **Dynamic title strategy**: simple `document.title = ...` in a `watchEffect` inside `Anime.vue` and `Profile.vue` setup blocks (no extra dependency on @unhead/vue or similar). Pattern: append " - AnimeEnigma" suffix to match the existing router-level title format. Fallback to the existing `titleKey` default while data is loading.
- **Auth h1**: render as `<h1 class="sr-only">` keyed off `auth.heading` — the visible h2 stays; we just promote a screen-reader heading above it. SEO note: this also gives the page a proper top-level heading.
- **QR canvas a11y**: `role="img"` + `aria-label` on the canvas; spinner overlay already has `aria-hidden`.
- **RecItem alt**: switch to literal `alt=""` (decorative). Justification: visible title text immediately below the image redundantly announces the same name to screen readers. This is the audit's recommended fix path.
- **Import placeholders**: append " or URL" hint in all three locales — short, no full sentence; existing placeholders are imperative.
- **AdminRecs**: the recompute button already shows visible text (`forceRecompute` / `recomputing`). Audit's UA-099 specifically calls for an explicit aria-label that doesn't change when the button is busy — add `:aria-label` separate from the button text so SR announcement stays consistent across the two states.

</decisions>

<code_context>
## Existing Code Insights

- `frontend/web/src/composables/useAnime.ts` — uses `useAnime()` factory pattern. Need to wire `useI18n()` inside. Two error sites at L116 + L133.
- `frontend/web/src/components/layout/Navbar.vue` — L172 hamburger; L105-112 search-close button; L268-273 `navLinks` array (add `/schedule` entry).
- `frontend/web/src/views/Auth.vue` — L21 h2 is current top heading; L43 `<canvas ref="qrCanvas">`.
- `frontend/web/src/views/Home.vue` — L14-22 mobile schedule icon link; L46-83 inline rec card with `:alt="getLocalizedTitle(...)"` at line ~65.
- `frontend/web/src/views/Anime.vue` — `anime` ref populated by `fetchAnime()`; setup script around L769.
- `frontend/web/src/views/Profile.vue` — public profile route alias; needs username-aware title.
- `frontend/web/src/views/admin/AdminRecs.vue` — L25 recompute `<button>` toggles between `recomputing` / `forceRecompute` text.
- `frontend/web/src/router/index.ts` — L131 sets `document.title` from `titleKey`. We override per-view via watchEffect.
- Three locale files at `frontend/web/src/locales/{en,ru,ja}.json`. Existing `nav.*` family + `auth.*` family + `profile.import.*` family already present.

</code_context>

<specifics>
## Specific Ideas

- The audit's "RecItem" refers to the rec-row card pattern inline in `Home.vue` (no standalone RecItem.vue exists in this codebase). The fix lands on the inline `<img>` inside the `v-for="item in trendingRecs"` block.
- `nav.schedule` already exists in the i18n catalog — reuse it for the drawer entry and the icon-link aria-label.
- `closeSearch` button — distinguish from `closeMenu` (drawer toggle). Use `nav.closeSearch` for the X-icon button next to the search field.

</specifics>

<deferred>
## Deferred Ideas

- UA-073 (locale switcher "ru" label) → Phase 6 (navbar drawer a11y).
- UA-091 / UA-092 / UA-096 / UA-097 / UA-098 / UA-101 (AdminRecs polish) → Phase 12.
- UA-058 (RecItem h3) → Phase 7.

</deferred>
