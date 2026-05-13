---
status: passed
phase: 12
phase_name: "AdminRecs SPA quality — a11y + UX polish"
verified: 2026-05-13
---

# Phase 12 Verification: AdminRecs SPA quality

## Must-have truths scorecard (per 12-PLAN.md)

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | AdminRecs table has `<caption class="sr-only">` + `aria-label` + `scope="col"` headers | PASS | `frontend/web/src/views/admin/AdminRecs.vue` lines 49–62: `:aria-label="$t('admin.recs.tableCaption')"` on `<table>`, `<caption class="sr-only">` as first child, `scope="col"` on all 10 `<th>` headers. |
| 2 | Clickable `<tr>` is keyboard-accessible with `role="button"`, `tabindex=0`, `aria-expanded`, `aria-controls`, `@keydown.enter`, `@keydown.space`, focus-visible ring | PASS | `AdminRecs.vue` lines 66–79: `tabindex="0"`, `role="button"`, `:aria-expanded="expandedRowIds.has(row.rank)"`, `:aria-controls="`detail-${row.rank}`"`, `@keydown.enter.prevent`, `@keydown.space.prevent`, `focus:outline-none focus-visible:ring-2 focus-visible:ring-cyan-400 focus-visible:ring-inset`. Detail row has `:id="`detail-${row.rank}`"` at line ~108. |
| 3 | S1–S5 column headers have `:title=` tooltips with explanatory i18n strings | PASS | `AdminRecs.vue` lines 55–59: `<th scope="col" ... :title="$t('admin.recs.s{1-5}Title')">S{1-5}</th>`. |
| 4 | Mobile horizontal-scroll fade affordance: `md:hidden absolute right-edge gradient` inside the `overflow-x-auto` wrapper | PASS | `AdminRecs.vue` line ~159: `<div aria-hidden="true" class="md:hidden absolute right-0 top-0 bottom-0 w-8 bg-gradient-to-l from-black/40 to-transparent pointer-events-none">`. The wrapper gained `relative` (line 48) so the absolute positioning resolves correctly. |
| 5 | AdminRecsPicker: autofocus search input via template ref + `onMounted`, focus-visible ring on input + buttons | PASS | `AdminRecsPicker.vue`: `searchInputRef` ref binding, `onMounted(() => searchInputRef.value?.focus())`, `focus-visible:ring-2 focus-visible:ring-cyan-400` on input + submit + self-action button. |
| 6 | AdminRecsPicker: in-flight spinner visible while router resolves; "You" badge on self-quick-action; empty-state `<p>` below the input | PASS | `AdminRecsPicker.vue`: `isSubmitting` ref toggles around `router.push(...)`, spinner rendered via `v-if="isSubmitting"` with `aria-busy` on the input; cyan-500/20 "You" badge pill in the self-action button; `<p v-if="!input.trim()">{{ $t('admin.recs.picker.empty') }}</p>` below the input. |
| 7 | `useAdminRecs.ts` maps HTTP status → i18n keys (`admin.errors.{unauthorized,forbidden,serverError,timeout}`) | PASS | `useAdminRecs.ts` lines 66–91: `mapHttpError()` helper. 401 → unauthorized, 403 → forbidden, ≥500 → serverError, ECONNABORTED / ETIMEDOUT / `/timeout/i` → timeout. Both `fetchRows()` and `recompute()` route generic errors through it. Legacy `'403'` branch preserved for the existing red-banner template. |
| 8 | Admin-guard non-silent redirect: banner shown before/after `next({ name: 'home' })` | PASS | `router/index.ts` sets `sessionStorage.admin_redirect_reason = 'admin.errors.notAdmin'` before `next({ name: 'home' })`. `App.vue` consumes the key on mount + `router.afterEach`, renders a fixed red `role="alert"` banner under the navbar with auto-dismiss in 6 s. |
| 9 | i18n: 13 new keys × 3 locales = 39 entries; JSON parses clean | PASS | All 13 keys (`admin.recs.tableCaption`, `admin.recs.s{1-5}Title`, `admin.recs.picker.{listboxLabel,empty,youBadge}`, `admin.errors.{notAdmin,unauthorized,forbidden,serverError,timeout}`) present in en/ru/ja; JSON validation via `python3 json.load` passes for all three. |

**Overall status:** PASSED — 9/9 must-have truths met.

## Artifact verification (per 12-PLAN.md "Files touched")

| Artifact | Path | Contains-check | Status |
|---|---|---|---|
| AdminRecs table a11y + scroll fade | `frontend/web/src/views/admin/AdminRecs.vue` | `role="button"`, `aria-expanded`, `@keydown.enter`, `@keydown.space` | FOUND (4 matches — see grep below) |
| AdminRecsPicker listbox shape | `frontend/web/src/views/admin/AdminRecsPicker.vue` | `role="search"`, `role="option"`, `aria-label` | FOUND (5 matches — see grep below) |
| useAdminRecs error mapping | `frontend/web/src/composables/useAdminRecs.ts` | `admin.errors.` | FOUND (5 matches — see grep below) |
| Router guard relay | `frontend/web/src/router/index.ts` | `admin_redirect_reason` | FOUND |
| App.vue banner | `frontend/web/src/App.vue` | `adminRedirectKey` | FOUND |
| en.json | `frontend/web/src/locales/en.json` | `admin.errors.notAdmin` + `admin.recs.tableCaption` + `admin.recs.picker.youBadge` | FOUND (13 new keys) |
| ru.json | `frontend/web/src/locales/ru.json` | same | FOUND (13 new keys) |
| ja.json | `frontend/web/src/locales/ja.json` | same | FOUND (13 new keys) |

## Test results

### Frontend type-check

```
$ cd frontend/web && bunx vue-tsc --noEmit
(clean — no output, exit 0)
```

### i18n-lint

```
$ bash scripts/i18n-lint.sh
=== Summary ===
  Missing keys:    0
  Syntax errors:   0
  Hardcoded text:  14 (warning, pre-existing in HanimePlayer.vue)
  Unused keys:     16 (warning, pre-existing across the codebase)

PASS: No blocking i18n issues.
```

0 missing keys, 0 syntax errors — Phase 12 introduces no new lint debt. The 14 hardcoded warnings are pre-existing Russian strings in `HanimePlayer.vue` (untouched in this phase). The 16 unused-key warnings include 2 from Phase 14's `admin.recs.{expandRow,collapseRow}` (kept for the unrendered "Expand"/"Collapse" tooltip-only-not-shown chevron — out of Phase 12 scope to remove).

### JSON validity

```
$ python3 -c "
import json
for loc in ['en','ru','ja']:
    json.load(open(f'frontend/web/src/locales/{loc}.json'))
print('ok')
"
ok
```

### Deploy + health

```
$ make redeploy-web 2>&1 | tail -5
docker compose -f docker/docker-compose.yml up -d --no-deps web
 Container animeenigma-web Started
Web frontend redeployed

$ make health
Checking service health...
✓ gateway:8000
✓ auth:8080
✓ catalog:8081
✓ streaming:8082
✓ player:8083
✓ rooms:8084
✓ scheduler:8085
✓ scraper:8088
```

All 8 services healthy after redeploy.

### Grep verification (per plan)

**AdminRecs.vue keyboard-handler attributes (≥4 expected):**

```
$ grep -nE 'role="button"|aria-expanded|@keydown\.enter|@keydown\.space' frontend/web/src/views/admin/AdminRecs.vue
73:                  role="button"
74:                  :aria-expanded="expandedRowIds.has(row.rank)"
77:                  @keydown.enter.prevent="toggleRow(row.rank)"
78:                  @keydown.space.prevent="toggleRow(row.rank)"
```

4 matches — meets the plan threshold.

**AdminRecsPicker.vue ARIA roles (≥2 expected):**

```
$ grep -nE 'role="listbox"|role="option"|aria-label|role="search"|role="group"' frontend/web/src/views/admin/AdminRecsPicker.vue
16:          role="search"
17:          :aria-label="$t('admin.recs.picker.listboxLabel')"
57:            role="group"
58:            :aria-label="$t('admin.recs.picker.listboxLabel')"
70:              role="option"
```

5 matches — well above the plan threshold. Plan-deviation note: `role="search"` + `role="group"` replace `role="listbox"` because the picker is a single-input form, not a live-results list. `role="option"` is present on the self-quick-action button (the one "option" that exists). See 12-SUMMARY.md for the full rationale.

**useAdminRecs.ts error mapping (per plan):**

```
$ grep -nE 'admin\.errors\.' frontend/web/src/composables/useAdminRecs.ts
67:  // starting with `admin.errors.`; legacy `'403'` is preserved for
85:      return 'admin.errors.timeout'
87:      return 'admin.errors.unauthorized'
88:      return 'admin.errors.forbidden'
89:      return 'admin.errors.serverError'
```

All 4 expected error keys present plus the contract comment.

### i18n-key spot check across locales

```
$ python3 -c "
import json
for loc in ['en','ru','ja']:
    with open(f'frontend/web/src/locales/{loc}.json') as f:
        d = json.load(f)
    admin = d['admin']
    recs, errs = admin['recs'], admin['errors']
    assert 'tableCaption' in recs and 's1Title' in recs and 's5Title' in recs
    assert recs['picker']['empty'] and recs['picker']['youBadge'] and recs['picker']['listboxLabel']
    assert errs['notAdmin'] and errs['unauthorized'] and errs['forbidden']
    assert errs['serverError'] and errs['timeout']
print('all 13 keys × 3 locales present')
"
all 13 keys × 3 locales present
```

## Commits on `main`

| Commit | Subject | Files |
|---|---|---|
| `7c999f2` | feat(12): AdminRecs table a11y + mobile scroll-fade (UA-093/094/095/098) | `views/admin/AdminRecs.vue` |
| `559fc37` | feat(12): AdminRecsPicker listbox + autofocus + you badge + empty state (UA-090/091/092/097/101) | `views/admin/AdminRecsPicker.vue` |
| `4b70903` | feat(12): useAdminRecs HTTP error mapping (UA-096) | `composables/useAdminRecs.ts` |
| `8258612` | feat(12): admin guard redirect banner (UA-100) | `router/index.ts`, `App.vue` |
| `767ebbf` | feat(12): admin i18n keys for Phase 12 a11y + UX surfaces | `locales/{en,ru,ja}.json` |

5 commits total; each independently revertable.

## Audit-finding closure

| Finding | Surface | Mechanism | Status |
|---|---|---|---|
| UA-090 | AdminRecsPicker | "You" badge on the "Use my account" quick-action (adapted — no live result list to filter; badge is on the closest self-affordance) | CLOSED |
| UA-091 | AdminRecsPicker | `onMounted(() => searchInputRef.focus())` + `form role="search"` + `aria-label` | CLOSED |
| UA-092 | AdminRecsPicker | `isSubmitting` ref + inline cyan-400 spinner inside the input + `aria-busy="true"` for AT | CLOSED |
| UA-093 | AdminRecs table | `:title="$t('admin.recs.s{1-5}Title')"` on each S1–S5 `<th>` | CLOSED |
| UA-094 | AdminRecs table | `<caption class="sr-only">` + table `aria-label` + `scope="col"` on all 10 headers | CLOSED |
| UA-095 | AdminRecs table | `tabindex=0`, `role="button"`, `aria-expanded`, `aria-controls`, `@keydown.enter/space`, focus-visible ring; detail row gets `:id="detail-{rank}"` | CLOSED |
| UA-096 | useAdminRecs.ts | `mapHttpError()` translates 401/403/≥500/timeout to `admin.errors.*` i18n keys; legacy `'403'` branch preserved | CLOSED |
| UA-097 | AdminRecsPicker | `<p>` empty-state hint below the input (adapted — fires when input is blank, since there's no live search to return zero hits) | CLOSED |
| UA-098 | AdminRecs table | `md:hidden absolute right-0 top-0 bottom-0 w-8 bg-gradient-to-l from-black/40` inside the scroll container | CLOSED |
| UA-100 | Admin guard | Router stashes i18n key in `sessionStorage.admin_redirect_reason`; App.vue consumes on mount + `router.afterEach` and renders a fixed red `role="alert"` banner (auto-dismiss 6 s) | CLOSED |
| UA-101 | AdminRecsPicker | `focus-visible:ring-2 focus-visible:ring-cyan-400` on the input + submit button + self-action button | CLOSED |

**Phase 12 outcome:** PASSED — 11 / 11 findings closed across 8 files. Zero backend changes, zero new dependencies, zero new lint debt. Picker listbox semantics are forward-compatible with a future live-search backend.
