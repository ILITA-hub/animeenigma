---
id: A11Y-tab-index-optimization
title: Tab-index optimization — refine keyboard tab ORDER, not just reachability
captured_at: 2026-06-15
captured_during: feedback triage of 2026-06-12T01-34-43_tNeymik_telegram
deferred_from: keyboard-tab-accessibility milestone (2026-06-12) — reachability shipped, ordering refinement carved out
status: backlog
---

# Tab-index optimization (separate from the shipped a11y reachability work)

## Context

The 2026-06-12 keyboard-tab-accessibility push (spec
`docs/superpowers/specs/2026-06-12-keyboard-tab-accessibility-design.md`, plan
`docs/superpowers/plans/2026-06-12-keyboard-tab-accessibility.md`) made interactive
elements **keyboard-reachable**: skip-to-content link (`1552927d`), schedule day cells
(`dfc518bc`), schedule filter chips (`c68bf34b`), gacha pull-summary cards (`78730ed4`),
theme card accordion (`59ad4917`), admin-feedback rows/kanban + raw-library results
(`f39aa8e7`), browse-subs modal Escape (`83898291`), scrub bar ARIA slider (`bc4a5b68`).

That closed the original ticket item «Сделать индекс табуляции (аксессабилити)» —
things are now *reachable*. This backlog item is the **optimization** of the tab
*order itself*, which is deliberately out of scope of the reachability pass and tracked
separately per owner request (2026-06-15).

## What "optimization" means here (the gap to fill)

Reachability ≠ a good tab order. Remaining work:

1. **Logical DOM order audit** — walk every primary surface (home, /anime, player,
   My List, profile, admin) with Tab only and confirm focus moves in reading order.
   Flag places where DOM order diverges from visual order (Tailwind `order-*`,
   flex/grid reordering, teleported overlays).
2. **Redundant / dead tab stops** — strip `tabindex="0"` from non-interactive wrappers,
   ensure decorative icons/links aren't focusable, collapse double stops where a row and
   its inner control both grab focus.
3. **Roving tabindex for composite widgets** — carousels (HeroSpotlight), tab strips,
   menus, the WatchlistRow grid: should be a single tab stop with arrow-key traversal
   inside (roving `tabindex`), not N separate stops.
4. **Focus management on overlays** — modals/menus/teleported panels (SubtitleOverlay,
   OtherSubsPanel, browse-subs, settings menus): trap focus while open, restore focus to
   the trigger on close, and don't leave a tab path into hidden-behind content.
5. **Skip-link coverage** — verify the skip-to-content target lands past the navbar on
   every route, and consider a second skip target for the player page.

## Why deferred / kept separate

- The reachability pass was the user-blocking part (you couldn't reach controls at all);
  the ordering refinement is polish that benefits from a dedicated keyboard-only audit
  pass rather than being bolted onto each individual fix.
- It is cross-cutting (touches many SFCs) and best done as one focused workstream with a
  before/after Tab-order recording, not piecemeal.

## Suggested approach

- Keyboard-only walkthrough per surface, recording the actual Tab sequence.
- jsdom/vitest can assert `tabindex` presence/absence and roving behavior, but the
  **visual order vs DOM order** check needs a real browser (Tailwind reorder utilities
  are invisible to jsdom — see `reference_tailwind_v4_css_cascade`).

## Cross-references

- Spec: `docs/superpowers/specs/2026-06-12-keyboard-tab-accessibility-design.md`
- Plan: `docs/superpowers/plans/2026-06-12-keyboard-tab-accessibility.md`
- Source ticket: feedback `2026-06-12T01-34-43_tNeymik_telegram`
