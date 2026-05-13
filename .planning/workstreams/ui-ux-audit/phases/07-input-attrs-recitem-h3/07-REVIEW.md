---
phase: 07-input-attrs-recitem-h3
reviewed: 2026-05-13T00:00:00Z
depth: quick
files_reviewed: 2
files_reviewed_list:
  - frontend/web/src/components/ui/Input.vue
  - frontend/web/src/views/Home.vue
findings:
  critical: 0
  warning: 0
  info: 2
  total: 2
status: clean
---

# Phase 7: Code Review Report

**Reviewed:** 2026-05-13
**Depth:** quick
**Files Reviewed:** 2
**Status:** clean (2 info-level observations, no blockers, no warnings)

## Summary

Phase 7 ships two surgical a11y fixes:

1. `frontend/web/src/components/ui/Input.vue` — adds
   `defineOptions({ inheritAttrs: false })` (line 44) and `v-bind="$attrs"`
   on the inner `<input>` (line 12).
2. `frontend/web/src/views/Home.vue` — line 83 swaps the trending-rec
   card title from `<p>` to `<h3>` with `font-medium` added to match the
   canonical column-row h3 pattern at lines 159/246/327.

Both changes are correct, idiomatic Vue 3, and produce no regressions in
the three consumers of `<Input>` (`SearchAutocomplete.vue:3`, `Game.vue:198`,
`Game.vue:219`). The two "incidental fixes" the executor flagged
(`required` on Game.vue:223 now reaching the actual input, and
SearchAutocomplete combobox ARIA attrs now landing on the `<input>` rather
than the wrapper `<div>`) are real net-positive corrections, not subtle
breakage. No new XSS / injection / secret-handling surface area.

No Critical findings. No Warnings. Two Info-level observations follow for
forward awareness — neither blocks merge.

## Info

### IN-01: `v-bind="$attrs"` ordering vs. explicit `:id` binding

**File:** `frontend/web/src/components/ui/Input.vue:10-21`
**Issue:** `v-bind="$attrs"` is placed at line 12, BEFORE the explicit
`:id="inputId"` (line 11 actually comes first; `v-bind` follows). In Vue
3, when an explicit attribute precedes `v-bind`, the spread wins; when it
follows, the explicit binding wins. Current ordering is:
```vue
:id="inputId"
v-bind="$attrs"
v-model="model"
:type="type"
:placeholder="placeholder"
:disabled="disabled"
:readonly="readonly"
```
This means a caller passing `id="something"` would silently override the
auto-generated `inputId` that the `<label :for="inputId">` (line 3) is
wired to, breaking the label-to-input association.

This is not a real regression for current consumers — none of
`SearchAutocomplete.vue:3`, `Game.vue:198`, or `Game.vue:219` pass an
`id`. But it's a latent footgun: anyone calling `<Input id="foo" label="X">`
in the future would silently break the label association.

**Fix (low priority, follow-up):** Move `v-bind="$attrs"` to BEFORE
`:id="inputId"` so explicit `:id` always wins over any caller-supplied
`id`, OR extract `id` from `$attrs` via `useAttrs()` and strip it before
binding. Example minimal fix:
```vue
<input
  v-bind="$attrs"
  :id="inputId"
  v-model="model"
  ...
/>
```
This preserves the label-for wiring as the authoritative `id`. Leaving
as-is is acceptable for Phase 7 scope; flag for Phase 12 (AdminRecs
SPA quality) to revisit if filter inputs ever need a caller-supplied id.

### IN-02: Dead `focused` ref in `Input.vue`

**File:** `frontend/web/src/components/ui/Input.vue:19-20, 79`
**Issue:** The `focused` ref at line 79 is written by the inline
`@focus`/`@blur` handlers at lines 19-20 but never read by any computed,
template binding, or emit. It's dead state. Pre-existing — not introduced
by Phase 7 — but worth flagging because adding `v-bind="$attrs"` now means
caller-supplied `@focus`/`@blur` handlers will fire alongside the
internal ones (Vue 3 auto-merges listeners on the same element). That
merge behavior is correct, but it surfaces the fact that the internal
listener is doing nothing useful.

**Fix (cleanup, follow-up):** Either remove the unused `focused` ref
and its `@focus`/`@blur` handlers, or wire `focused` into
`inputClasses` if a focus-styled state was the original intent. Out of
Phase 7 scope; leave for a future Input.vue polish pass.

## Confirmed safe

- **`defineOptions` + `inheritAttrs: false` pairing:** correct. Without
  `inheritAttrs: false`, `$attrs` would land on BOTH the wrapper `<div>`
  (Vue's default) and the inner `<input>` (via explicit `v-bind`),
  producing duplicate ARIA attributes. Both lines are present
  (Input.vue:44 and Input.vue:12).
- **Event listener flow through `$attrs`:** Vue 3 includes event
  listeners in `$attrs` as `onClick`, `onKeydown`, etc. Consumer-passed
  `@keydown.enter`, `@keyup.enter`, `@keydown.down`, etc. correctly land
  on the inner `<input>`. The internal `@focus`/`@blur` handlers
  auto-merge with any caller-supplied handlers — no conflict.
- **Class merging:** `:class="[inputClasses, ...]"` is bound explicitly
  on the `<input>` (line 18), so caller-supplied `class` strings continue
  to merge via Vue's class/style auto-merge. The wrapper `<div>`'s
  `:class="wrapperClasses"` is unaffected.
- **Typed prop shadowing:** `type`, `placeholder`, `disabled`, `readonly`
  are extracted by `defineProps` and never appear in `$attrs`. No
  conflict between the spread and the explicit `:type`/`:placeholder`/etc.
  bindings.
- **`<h3>` inside `<router-link>` (renders as `<a><h3>...</h3></a>`):**
  Valid HTML5. The `<a>` element has "transparent" content model and
  accepts flow content including headings. Vue Router does not warn about
  this. The interactive headline pattern is common and accessible.
- **`font-medium` added to new h3:** Matches the existing column-row h3
  pattern at Home.vue:159, 246, 327. Visual parity confirmed.
- **Heading-order outline on Home:** `<h1 class="sr-only">` (line 4) →
  `<h2>` (trending row label, line 34) → `<h3>` (rec card titles, line
  83, new) → `<h2>` (Ongoing column header, line 114) → `<h3>` (anime
  titles, line 159) → repeats for Top + Announcements columns. Valid
  per WCAG SC 1.3.1.
- **Consumer audit (Game.vue:198, Game.vue:219, SearchAutocomplete.vue:3):**
  All three call sites verified. The two "incidental fixes" the
  executor noted (`required` reaching the `<input>` on Game.vue:219;
  `role="combobox"` + ARIA attrs reaching the `<input>` on
  SearchAutocomplete.vue:3) are correct improvements, not subtle breakage.
  - **Game.vue:198**: passes `v-model`, `:placeholder`, `size`,
    `@keyup.enter`. All typed props or event listeners — `@keyup.enter`
    now lands on the actual `<input>` via `$attrs` rather than the
    wrapper `<div>` (where it never worked). Net positive.
  - **Game.vue:219**: passes `v-model`, `:label`, `:placeholder`,
    `required`. The native `required` now participates in the
    `<form @submit.prevent="createRoom">` validation at line 218. Net
    positive — and consistent with the manual `<input ... required>`
    at line 232-238 of the same form.
  - **SearchAutocomplete.vue:3**: passes `role="combobox"`,
    `aria-autocomplete`, `aria-controls`, `aria-expanded`,
    `aria-activedescendant`, four `@keydown` handlers. All ARIA attrs
    now land on the `<input>`, making the combobox semantically valid
    (WAI-ARIA 1.2 allows `role="combobox"` on `<input type="search">`).
    Keydown handlers now fire on the input rather than the wrapper
    `<div>`. Net positive — repairs the combobox ARIA tree.
- **No XSS / injection surfaces introduced:** `$attrs` forwards
  Vue-template-trusted values only; no string concatenation into
  innerHTML, no `v-html` introduced, no eval, no dynamic component
  resolution. The h3 swap is structural only — same `{{ ... }}` text
  interpolation as the prior `<p>`.

---

_Reviewed: 2026-05-13_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: quick_
