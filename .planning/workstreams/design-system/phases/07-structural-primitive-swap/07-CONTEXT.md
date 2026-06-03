# Phase 7: Structural Primitive Swap - Context

**Gathered:** 2026-06-03
**Status:** Ready for planning
**Mode:** Post-audit addition — user chose "add a structural-swap phase" at the v1.0 milestone audit (2026-06-03) to close DS-MIGRATE-06 + DS-MIGRATE-01's primitive half before completing the milestone.

<domain>
## Phase Boundary

Replace the remaining hand-rolled `<button>` elements with the `@/components/ui` `Button` primitive
WHERE the primitive's variant/size API genuinely fits, completing the structural half of the
migration. Unlike Phases 4-5 (color/token-only, zero-diff), this CHANGES markup/props/events, so:
- Every swap is **per-element adjudicated** — swap when `<Button variant size>` reproduces the
  element's appearance + behavior (click handler, disabled, aria, keyboard) with no visual/behavioral
  diff; otherwise **leave it bespoke and document the reason** (the requirement is "where they
  exist/fit", not "swap unconditionally").
- Result is build + type + test verified; affected-surface in-browser smoke deferred → HUMAN-UAT
  (standing rule DS-NF-06).

Scope = the 10 remaining raw `<button>` sites (audit-enumerated):
`components/player/SubtitleSettingsMenu.vue` (×5), `views/Themes.vue`, `views/Browse.vue`,
`components/home/spotlight/SpotlightIcon.vue`, `components/home/spotlight/CarouselDots.vue`,
`components/anime/AnimeCard.vue`. Plus any hand-rolled badge that has a `Badge` analog and fits it.

OUT OF SCOPE: `<Card>` — currently used nowhere by design (bespoke card layouts that don't fit the
Card API stay as-is; the requirement's "where they exist" covers this). No color/token changes (the
lint gate must still pass — swaps must not reintroduce off-palette/hex/alias). No new dependencies.

</domain>

<decisions>
## Implementation Decisions

### Adjudication rule (the core of this phase) — Claude's discretion within these bounds
- **Swap to `<Button>`** when ALL hold: the element is a genuine button (not a tiny visual control),
  a `Button` variant+size reproduces its look (use `ghost`/`outline`/`default`/`brand`/`destructive`
  + `sm`/`md`/`lg`/`icon`), and ALL behavior is preserved — `@click`, `:disabled`, `type`, `aria-*`,
  keyboard focus, and any `class` passthrough (Button merges `props.class` via `cn`). Visual result
  must be identical (or trivially closer to the design system).
- **Keep bespoke + document** when the element is a specialized control the Button API doesn't model
  cleanly — e.g. `CarouselDots` (a row of tiny dot toggles), `SpotlightIcon` (decorative icon
  trigger), or a button whose layout/state can't be expressed as a Button variant without markup
  contortion. Record the one-line reason in the SUMMARY (and, if useful, an inline comment).
- **Likely swaps:** the Themes/Browse/AnimeCard action buttons + most of the SubtitleSettingsMenu
  menu items (assess each — player chrome sometimes needs bespoke sizing). **Likely keep:**
  CarouselDots, SpotlightIcon (adjudicate, don't force).

### Verification (this phase is behavioral — stricter than 4-5)
- Per swapped file: `bunx vue-tsc --noEmit` clean, the file's existing spec (if any) green or
  realigned (Rule-1 deviation, documented), and a manual read confirming events/a11y preserved.
- Phase close: full `bunx vitest run` (no NEW failures beyond pre-existing AnimeContextMenu.spec.ts:227),
  `bunx vue-tsc --noEmit` exit 0, `bunx vite build` clean, AND `make lint-design` exit 0 (no
  off-palette/hex/alias reintroduced). In-browser smoke of the affected surfaces → HUMAN-UAT.

### Staging discipline (unchanged)
Stage only the phase's files by explicit path; never `git add -A`. ActivityFeed.vue + other
pre-existing uncommitted changes stay untouched.

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- `frontend/web/src/components/ui/Button.vue` + `button-variants.ts` — the `cva` variant API to swap onto
  (`variant`: default/brand/ghost/outline/destructive; `size`: sm/md/lg/icon; merges `props.class`,
  adds `touch-target`). 21 existing `<Button>` consumers across 11 files are the reference pattern.
- `frontend/web/src/components/ui/Badge.vue` — for any hand-rolled badge that fits.
- The design-system lint gate (`make lint-design`) — must still pass after swaps.

### Established Patterns
- Button is imported from `@/components/ui`. Existing consumers (e.g. the 11 files already using it)
  show the idiomatic `<Button variant="ghost" size="icon" @click="...">` usage.

### Integration Points
- 6 files (the raw-`<button>` sites). Each is independent → can be batched by area
  (player / views / home-spotlight / anime-card) with disjoint files for parallel-safe execution.

</code_context>

<specifics>
## Specific Ideas

- Read each `<button>`'s full markup + the nearest existing `<Button>` consumer before swapping, so
  variant/size/class choices match the established idiom rather than guessing.
- If a swap would change a computed visual value, prefer keeping it bespoke and documenting — this
  phase must not introduce a visible diff beyond "now uses the primitive".

</specifics>

<deferred>
## Deferred Ideas

- A lint rule that flags raw `<button>` in favor of `<Button>` — explicitly NOT added; primitive
  reuse stays governance-only (Phase 6 decision). This phase closes the existing debt, it does not
  add structural enforcement.

</deferred>
