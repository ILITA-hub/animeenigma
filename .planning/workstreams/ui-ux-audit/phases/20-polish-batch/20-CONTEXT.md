# Phase 20: Tier D — polish batch - Context

**Gathered:** 2026-05-13
**Status:** Ready for planning
**Mode:** Auto-generated (autonomous run, final polish batch — small surgical fixes only)

<domain>
## Phase Boundary

Last phase in the v0.1 milestone. Mop up small polish items discovered during prior phases. Closes UX-36.

**Scope is INTENTIONALLY SMALL.** Most catastrophic / major / minor severity audit findings were closed in Phases 1-19. This phase addresses:

1. **UA-085 — Drawer Schedule entry.** The mobile drawer in Navbar.vue is missing the `/schedule` link entry. Phase 16 (broadcast schedule) added the Schedule view; the drawer link was deferred to Phase 20. Add it now.
2. **AnimeCardNew kebab focus polish.** The AnimeKebab affordance reveals on hover; add `focus-visible` state so keyboard navigation reveals it too.
3. **Skip-Intro settings.** Add a Profile setting `skip_intro_auto_dismiss_seconds` (default 8) to control how long the skip-intro CTA stays visible. Adds polish granularity. (LIGHTWEIGHT — settings UI only, no backend persistence beyond localStorage.)
4. **FAQ accordion transition.** The `<details>` accordion on About.vue is functional but jumpy. Add CSS transitions for `content-visibility` + `interpolate-size` (modern Chrome) or fallback for cross-browser smoothness.
5. **AnimeQuickNav (Phase 11) section IDs.** Verify the section IDs on Anime.vue match exactly what AnimeQuickNav.vue references. Common bug-class — verify and fix any drift.

Items NOT in this phase (deferred to next milestone):
- Deep visual redesign
- New features
- Comprehensive a11y re-audit
- Performance optimization sweep
- Any UA-NNN findings beyond what's listed above

</domain>

<decisions>
## Implementation Decisions

### Claude's Discretion (autonomous mode)

**UA-085 — Drawer Schedule entry:**
- In `frontend/web/src/components/layout/Navbar.vue` mobile drawer (lines ~185-256), add a router-link to `/schedule` matching the existing nav-link pattern.
- Position: between the Profile/Login section and the Language toggle.
- i18n key: already `nav.schedule` exists from prior phases.

**Kebab focus polish:**
- `frontend/web/src/components/anime/AnimeKebab.vue` — add `focus-within:opacity-100` to the parent container OR `focus-visible:opacity-100` on the kebab itself so keyboard users can reveal it.

**Skip-Intro auto-dismiss settings (lightweight):**
- Add `skipIntroAutoDismissSeconds` to a local-only Pinia store (or just localStorage-backed composable). Default 8.
- In HiAnimePlayer + ConsumetPlayer, use `setTimeout` to hide the CTA after the configured seconds.
- Profile Settings tab: new input "Skip-Intro CTA visible for (seconds)" with min=2, max=60. Persists to localStorage on change.
- Defer: a fancier UI for this could be a Phase 20+ polish item.

**FAQ transition:**
- About.vue `<details>` elements: add CSS rule to animate `max-height` on `:open`. Modern Chrome supports `interpolate-size: allow-keywords`; Firefox doesn't. Use `transition: max-height 200ms ease-out` with a `max-height` set on `:not([open])` (collapsed) and `[open]` (expanded). Approximate, but smoother than the current jump.

**AnimeQuickNav drift check:**
- Verify Anime.vue still has all 4 section IDs: `section-overview`, `section-episodes`, `section-similar`, `section-comments`. Re-check post-Phase 11. If any are missing or renamed, fix the QuickNav references to match.

### Locked from ROADMAP

- Last phase. No more dependencies on future phases.
- Deferred items get carried to next milestone's REQUIREMENTS.md.

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets

- `frontend/web/src/components/layout/Navbar.vue` — mobile drawer is where UA-085 lives.
- `frontend/web/src/views/About.vue` — Phase 14's FAQ accordion target.
- `frontend/web/src/components/player/HiAnimePlayer.vue` + `ConsumetPlayer.vue` — Phase 18's skip-intro CTA.
- `frontend/web/src/views/Anime.vue` — Phase 11's section IDs target.

### Established Patterns

- Navbar drawer router-link pattern: see lines 188-197.
- Settings persistence via localStorage: already used in Theater Mode (Phase 11).

### Integration Points

- No backend changes. Pure frontend polish.

</code_context>

<specifics>
## Specific Ideas

- Phase 20 is the audit's "design-review checkpoint" gate. Don't sneak features in. Stick to the 5 items.
- Each item is small (10-30 LOC). Total diff should be ~200 LOC.

</specifics>

<deferred>
## Deferred Ideas (to next milestone)

- Theater mode keyboard shortcut (T key) — was scoped to Phase 20 but defer; non-trivial.
- Drag-and-drop reorder of collection items — Phase 17 deferred this.
- Backfill of provider booleans (HasKodik etc.) — Phase 15 deferred; future scheduler job.
- True "Because you watched X" rec chip with seed anime name — backend rework required, out of v0.1.
- Comprehensive a11y re-audit post-v0.1 — should run as a follow-up audit, separate workstream.

</deferred>
