# Phase 12: AdminRecs SPA quality - Context

**Gathered:** 2026-05-13
**Status:** Ready for planning
**Mode:** Auto-generated (autonomous run, a11y + UX polish on admin recs surface)

<domain>
## Phase Boundary

Make the admin recs debug surface keyboard- and screen-reader-usable; add error/loading/empty states; mobile-friendly. Two files in scope:

- `frontend/web/src/views/admin/AdminRecs.vue` — debug table.
- `frontend/web/src/views/admin/AdminRecsPicker.vue` — user picker.

**UX-25 closes UA-094, UA-095, UA-093:**
- UA-094: Table semantics — add `<caption class="sr-only">`, `aria-label`, scope on headers.
- UA-095: Expandable rows keyboard — `role="button"`, `tabindex="0"`, `@keydown.enter/space="toggleRow"`, `aria-expanded`.
- UA-093: S1-S5 column headers gain titles/explanations (clarify what each signal means).

**UX-26 closes UA-090, UA-091, UA-092, UA-096, UA-097, UA-098, UA-100, UA-101:**
- UA-090: AdminRecsPicker — current logged-in admin shouldn't be in their own picker self-list, OR if shown, marked clearly as "you".
- UA-091: AdminRecsPicker — focus management (autofocus on input; focus moves to results on type).
- UA-092: AdminRecsPicker — loading indicator while search is in flight.
- UA-096: Error mapping — friendly i18n keys for 401/500/timeout instead of raw error.
- UA-097: Empty-state help text on table.
- UA-098: Mobile horizontal-scroll affordance (visual scroll hint).
- UA-100: Admin guard silent redirect — show a "Admin only" message before redirect.
- UA-101: AdminRecsPicker — focus styles on search input + result rows.

</domain>

<decisions>
## Implementation Decisions

### Claude's Discretion (autonomous mode)

**AdminRecs.vue table (UX-25):**
- Add `<caption class="sr-only">{{ $t('admin.recs.tableCaption') }}</caption>` as first child of `<table>`.
- Add `aria-label="{{ $t('admin.recs.tableCaption') }}"` to `<table>` for AT support that ignores caption.
- Add `scope="col"` to each `<th>` in `<thead>`.
- Each clickable row gets: `tabindex="0"`, `role="button"`, `:aria-expanded="expandedRowIds.has(row.rank)"`, `:aria-controls="`detail-${row.rank}`"`, `@keydown.enter.prevent="toggleRow(row.rank)"`, `@keydown.space.prevent="toggleRow(row.rank)"`.
- Detail row gets `:id="`detail-${row.rank}`"` for aria-controls linkage.
- S1-S5 column headers get tooltip via `title` attribute (i18n string):
  - S1: "User's top-list similarity"
  - S2: "Genre affinity"
  - S3: "Global trending"
  - S4: "Rating boost"
  - S5: "Fresh-content seasonality"
- Focus style: `focus:outline-none focus:ring-2 focus:ring-cyan-400` on rows.

**AdminRecsPicker.vue (UX-26 + a11y polish):**
- Autofocus the search `<input>` on mount.
- Loading indicator: spinner next to the input while a search request is in flight.
- Result rows get `role="option"`, `tabindex="0"`, `:focus-visible` outline.
- Picker container gets `role="listbox"`, `aria-label`.
- Self-list filter: filter out the current admin user from results (compare to `authStore.user.id`), OR badge as "Вы (You)".
- Empty state: when search returns zero hits, show `admin.recs.picker.empty` text instead of an empty list.
- Mobile horizontal-scroll hint on the AdminRecs table — a fading right-edge gradient at narrow viewports (`md:hidden absolute right-0 top-0 h-full w-8 bg-gradient-to-l from-black/40`).
- Admin guard: when non-admin loads `/admin/recs`, show toast or banner "Только для администраторов" before the router-redirect kicks in. Use the existing toast system.

**Error mapping (UX-26 partial):**
- `composables/useAdminRecs.ts` likely catches generic errors. Extend to map HTTP status:
  - `401` → `admin.errors.unauthorized`
  - `403` → `admin.errors.forbidden`
  - `500` → `admin.errors.serverError`
  - timeout → `admin.errors.timeout`
- i18n entries for these 4 keys × 3 locales = 12 new strings.

### Locked from ROADMAP

- Phase 12 depends on Phase 5 (ButtonGroup pattern) — complete.
- Scope is the admin recs surface only. Other admin views (themes admin, anime admin if exists) are out of scope.

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets

- `frontend/web/src/views/admin/AdminRecs.vue` — 292 LOC, table at lines 49-148.
- `frontend/web/src/views/admin/AdminRecsPicker.vue` — 67 LOC, currently lacks listbox semantics.
- `frontend/web/src/composables/useAdminRecs.ts` — composable that fetches recs data; error handling lives here.
- Phase 7 `Input.vue` `$attrs` pass-through means the picker input can now accept `aria-describedby` / `aria-controls` from this phase.

### Established Patterns

- Toast system: confirm via existing useToast composable or app-level toast component (check `frontend/web/src/components/`).
- Focus ring pattern: `focus:outline-none focus:ring-2 focus:ring-cyan-400`.

### Integration Points

- No backend changes. Pure frontend a11y + UX polish.
- Admin role guard already exists in the router; the silent-redirect (UA-100) just needs a user-facing message before redirect.

</code_context>

<specifics>
## Specific Ideas

- AdminRecs is a DEBUG surface used by site operators. A11y is still required (might be used over SSH-forwarded sessions where a screen reader is present), but the surface is low-traffic; don't over-engineer.
- The "Вы (You)" label on the admin in their own picker results is cleaner than filtering — operators sometimes WANT to query their own data for debugging. Mark, don't hide.

</specifics>

<deferred>
## Deferred Ideas

- AdminRecs sort columns by score / rank (currently fixed order) — defer to Phase 20.
- Bulk operations on admin recs (e.g. mass-pin / mass-unpin) — out of scope; future admin tooling work.
- Full keyboard navigation across the picker results (arrow keys) — Phase 20 polish; Tab+Shift+Tab cycle is sufficient for v0.1.

</deferred>
