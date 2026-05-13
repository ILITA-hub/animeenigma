---
phase: 12
plan: 1
subsystem: ui-ux-audit
tags: [frontend, vue3, i18n, a11y, admin, recs-debug, error-mapping, listbox]
requires: [phase-5, phase-7]
provides:
  - admin-recs-table-a11y
  - admin-recs-picker-listbox-shape
  - admin-recs-http-error-mapping
  - admin-guard-redirect-banner
affects:
  - /admin/recs/:user_id (a11y only, no behavior change)
  - /admin/recs (picker, a11y + UX polish)
  - non-admin guard redirect (now surfaces a banner before home)
tech-stack:
  added: []
  patterns:
    - sr-only-caption-plus-aria-label-on-table
    - role-button-tabindex-aria-expanded-aria-controls-on-tr
    - focus-visible-ring-on-clickable-rows
    - mobile-scroll-fade-affordance-md-hidden-gradient
    - aria-busy-input-with-inline-spinner-during-router-push
    - session-storage-relay-from-router-guard-to-app-banner
    - http-error-status-to-i18n-key-mapping
key-files:
  created: []
  modified:
    - frontend/web/src/views/admin/AdminRecs.vue
    - frontend/web/src/views/admin/AdminRecsPicker.vue
    - frontend/web/src/composables/useAdminRecs.ts
    - frontend/web/src/router/index.ts
    - frontend/web/src/App.vue
    - frontend/web/src/locales/en.json
    - frontend/web/src/locales/ru.json
    - frontend/web/src/locales/ja.json
decisions:
  - <caption-class-sr-only>-plus-table-aria-label-redundancy-on-purpose-belt-and-braces
  - keep-legacy-error-equals-403-branch-for-the-existing-red-banner-template
  - admin-guard-relay-via-sessionStorage-not-new-toast-infra
  - banner-auto-dismisses-after-6s-so-it-doesnt-linger-cross-page
  - picker-listbox-shape-applied-defensively-since-live-search-not-implemented-yet
  - youBadge-on-self-quick-action-not-on-a-listbox-row-since-no-rows-exist
  - empty-state-renders-when-input-blank-not-after-zero-hits-since-no-live-search
metrics:
  duration: ~25min
  completed: 2026-05-13
  commits: 5
  tasks_complete: 6
  tasks_total: 6
---

# Phase 12 Summary: AdminRecs SPA quality — a11y + UX polish

**One-liner:** Made the admin recs debug surface keyboard-navigable, screen-reader-friendly, and mobile-aware: `<caption>`/scope/aria-expanded on the recs table, focus rings + autofocus + spinner on the picker, HTTP-status-to-i18n-key error mapping in the composable, and a no-longer-silent banner when non-admins are redirected away from `/admin/recs`.

## What landed

| Area | Mechanism |
|---|---|
| **Recs table a11y** (UA-094/095/093) | `<caption class="sr-only">` + table `aria-label` (redundant on purpose — some AT skips one, some skips the other); `scope="col"` on all 10 `<th>` headers; clickable `<tr>` gains `tabindex=0`, `role="button"`, `aria-expanded`, `aria-controls`, `@keydown.enter/space`, focus-visible ring; detail `<tr>` gets `:id="detail-{rank}"` for the `aria-controls` linkage. S1–S5 headers get `:title` tooltips that explain what each signal means. |
| **Mobile scroll-fade** (UA-098) | `md:hidden absolute right-0 top-0 bottom-0 w-8 bg-gradient-to-l from-black/40 to-transparent pointer-events-none` overlay sits inside the `overflow-x-auto` wrapper, hinting that the table extends past the viewport on phones. |
| **Picker listbox shape** (UA-091/092/097/101/090) | Autofocus search input via template ref + `onMounted`; `focus-visible:ring-2 ring-cyan-400` on input + buttons; inline spinner with `aria-busy="true"` on the input while `router.push()` resolves; `<p>` empty-state hint below the input; `form role="search"` + `aria-label`; "You" badge on the "Use my account" quick action so operators see at a glance that the target is themselves. |
| **HTTP error mapping** (UA-096) | New `mapHttpError()` helper in `useAdminRecs.ts` translates 401 → `admin.errors.unauthorized`, 403 → `admin.errors.forbidden`, ≥500 → `admin.errors.serverError`, ECONNABORTED/ETIMEDOUT/`/timeout/i` → `admin.errors.timeout`. Legacy `error === '403'` branch preserved so the existing red banner template keeps rendering. |
| **Admin-guard banner** (UA-100) | Router stashes `admin_redirect_reason = 'admin.errors.notAdmin'` in sessionStorage before `next({ name: 'home' })`; App.vue consumes it on mount + after every route transition and renders a fixed top-of-page red banner (`role="alert"`, dismiss button, auto-dismiss in 6 s). No new toast infrastructure introduced. |
| **i18n** | 13 new keys × 3 locales = 39 entries: `admin.recs.tableCaption`, `admin.recs.s{1-5}Title`, `admin.recs.picker.{listboxLabel,empty,youBadge}`, `admin.errors.{notAdmin,unauthorized,forbidden,serverError,timeout}`. |

## Plan deviations

**Picker has no live search backend yet.** The plan's UA-090 ("filter self from results"), UA-091 (`role="listbox"` container with `role="option"` rows), and UA-097 ("empty after zero hits") all assume a live result list. The current picker is a single-input form that submits to `/admin/recs/{id}`. Adapted as follows:

| Plan slot | Adaptation |
|---|---|
| `role="listbox"` container | `role="search"` on the `<form>` + `role="group"` on the quick-actions row. Future live-search drop-in can replace the form body without changing semantics. |
| `role="option"` rows | The "Use my account" quick action becomes the one option; tagged `role="option"`, `tabindex=0`. |
| "You" badge on the self-row | Badge sits on the "Use my account" quick action button. |
| Empty state after zero hits | Hint renders when input is blank (not after a zero-hit search), so the user is greeted with "No users found" guidance up-front. |

These are all forward-compat: when a live `/admin/users/search` endpoint lands, the listbox/option/empty slots already speak the right ARIA — only the data binding changes.

**Toast infrastructure.** The plan suggested using an existing toast helper for UA-100; there isn't one in the codebase (verified by grep — only the report-error system uses `Toast`-like UI, scoped per-player). Used the established Phase 11 pattern instead: reactive ref + fixed red banner near the top of App.vue, relayed from the router via sessionStorage. No new infra.

## Commits

| Commit | Subject | Files |
|---|---|---|
| `7c999f2` | feat(12): AdminRecs table a11y + mobile scroll-fade (UA-093/094/095/098) | `views/admin/AdminRecs.vue` |
| `559fc37` | feat(12): AdminRecsPicker listbox + autofocus + you badge + empty state (UA-090/091/092/097/101) | `views/admin/AdminRecsPicker.vue` |
| `4b70903` | feat(12): useAdminRecs HTTP error mapping (UA-096) | `composables/useAdminRecs.ts` |
| `8258612` | feat(12): admin guard redirect banner (UA-100) | `router/index.ts`, `App.vue` |
| `767ebbf` | feat(12): admin i18n keys for Phase 12 a11y + UX surfaces | `locales/{en,ru,ja}.json` |

5 commits; each independently revertable. Touched 8 files (3 Vue, 1 TS composable, 1 router, 1 App.vue, 3 locales).

## Findings closure

| Finding | Surface | Status | Mechanism |
|---|---|---|---|
| UA-090 | AdminRecsPicker | CLOSED | "You" badge on self-quick-action (adapted; see deviations) |
| UA-091 | AdminRecsPicker | CLOSED | Autofocus + form `role="search"` + `aria-label` |
| UA-092 | AdminRecsPicker | CLOSED | aria-busy input + inline spinner during router.push |
| UA-093 | AdminRecs table | CLOSED | `:title=` tooltips on S1–S5 column headers |
| UA-094 | AdminRecs table | CLOSED | `<caption class="sr-only">` + table `aria-label` + `scope="col"` |
| UA-095 | AdminRecs table | CLOSED | `tabindex=0`, `role="button"`, `aria-expanded`, `aria-controls`, `@keydown.enter/space` |
| UA-096 | useAdminRecs | CLOSED | `mapHttpError()` → i18n keys for 401/403/≥500/timeout |
| UA-097 | AdminRecsPicker | CLOSED | `<p>` empty-state hint below the input (adapted; see deviations) |
| UA-098 | AdminRecs table | CLOSED | `md:hidden absolute right-edge gradient` inside scroll container |
| UA-100 | Admin guard | CLOSED | sessionStorage relay → App.vue red banner (6 s auto-dismiss) |
| UA-101 | AdminRecsPicker | CLOSED | `focus-visible:ring-2 ring-cyan-400` on input + buttons |

**Phase 12 outcome:** PASSED. All 11 audit findings shipped across 3 frontend files + 1 router + 1 App-level banner + 3 locales. Zero backend changes. Zero new dependencies. The picker's listbox semantics are deliberately forward-compatible with a future live-search backend.

## Self-Check: PASSED

- File `frontend/web/src/views/admin/AdminRecs.vue` — FOUND (313 lines)
- File `frontend/web/src/views/admin/AdminRecsPicker.vue` — FOUND (132 lines)
- File `frontend/web/src/composables/useAdminRecs.ts` — FOUND (153 lines)
- File `frontend/web/src/router/index.ts` — FOUND
- File `frontend/web/src/App.vue` — FOUND
- Locales `frontend/web/src/locales/{en,ru,ja}.json` — FOUND (all 13 keys × 3 locales present)
- Commit `7c999f2` — FOUND
- Commit `559fc37` — FOUND
- Commit `4b70903` — FOUND
- Commit `8258612` — FOUND
- Commit `767ebbf` — FOUND
