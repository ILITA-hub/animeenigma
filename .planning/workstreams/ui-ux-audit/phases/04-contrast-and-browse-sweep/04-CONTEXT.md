# Phase 4: Color-contrast + Browse heading sweep - Context

**Gathered:** 2026-05-13
**Status:** Ready for planning
**Mode:** Auto-generated (autonomous run, mechanical CSS class swap + 3 Browse-specific a11y fixes)

<domain>
## Phase Boundary

Two related sub-batches:
- **UX-10 — `text-white/40` → `/60` global sweep** across 8 audit-cited views (UA-052/121 Anime detail, UA-066 Profile import-help and other Profile subtitle text, UA-072 Auth Telegram-Web summary, UA-074 Themes empty-state, UA-076 Schedule hint, UA-077 Game leaderboard rank, UA-086 Navbar search subtitle). Replace per-file via `replace_all` since the audit's mantra is a global sweep.
- **UX-11 — Browse a11y**: UA-046 GenreFilterPopup placeholder contrast (`text-white/30` → `/60`), UA-047 trigger button `aria-haspopup="listbox"` + `aria-expanded`, UA-048 sr-only `<h2>` between Browse `<h1>` and grid card `<h3>` headings.

Out of scope: player components (KodikPlayer, AnimeLib, etc.) and admin/setup views — audit didn't cite contrast violations there and a blind sweep would mass-change fullscreen player overlay surfaces that may rely on dim contrast intentionally.

</domain>

<decisions>
## Implementation Decisions

### Claude's Discretion (autonomous mode)

- **Strategy**: per-file `replace_all` of `text-white/40` → `text-white/60` in the 7 audited views. Over-touches some lines (e.g. small subtitle text the audit didn't explicitly name) but consistent with the audit's "global sweep" wording. Net effect is "all text-white/40 in user-visible views moves to /60" — no regressions, only contrast improvements.
- **GenreFilterPopup**: tightest fix is the placeholder span + aria attrs on the trigger. The trigger button uses `<button>` semantically, so `role` is implicit; only `aria-haspopup="listbox"` + `aria-expanded` need to land. The dropdown itself already has the listbox semantics inside `<Transition>`.
- **Browse heading**: insert a `<h2 class="sr-only">` keyed to `browse.resultsHeading` immediately before the grid div. Visually invisible, but axe-core's heading-order rule will see h1 → h2 → h3 and pass.

### Locked from ROADMAP

- Phase 4 depends only on Phase 1; all changes are CSS/aria only (no logic).
- Player components excluded from the sweep — out of scope per audit + risk of mass changes on intentional fullscreen UI.

</decisions>

<code_context>
## Existing Code Insights

- 79 total `text-white/40` occurrences across `frontend/web/src/`, 35 in the audit-cited views.
- `GenreFilterPopup.vue` placeholder is already at `text-white/30` (worse than /40); audit's fix is /60.
- `Browse.vue` has `<h1>` at line 6 and an "Recent Searches" `<h2>` at line 79 — but that h2 only renders when `!searchQuery`. The card grid is always rendered when there are results, so the heading-order violation occurs whenever a user searches. The sr-only h2 must live BEFORE the grid unconditionally.
- All three locales (`en.json`, `ru.json`, `ja.json`) need a new `browse.resultsHeading` key.

</code_context>

<specifics>
## Specific Ideas

- Keep `text-white/30` on lines that are intentionally subtle and small (cosmetic — e.g. `themes.syncHint` in Themes.vue line 92 which is `text-white/30 text-sm`). Those weren't flagged by the audit and changing them en masse drifts further from minimum diff.
- Tailwind class `/60` produces white text at 60% opacity ≈ #999 on a dark slate background — comfortably above WCAG AA 4.5:1.

</specifics>

<deferred>
## Deferred Ideas

- Player-component contrast review — audit didn't probe player fullscreen UI for contrast; could be a separate phase if anyone complains.
- `text-white/30` cleanup — audit only flagged one instance (UA-046 GenreFilterPopup placeholder). Other `/30` occurrences are usually intentional decorative subtitles.

</deferred>
