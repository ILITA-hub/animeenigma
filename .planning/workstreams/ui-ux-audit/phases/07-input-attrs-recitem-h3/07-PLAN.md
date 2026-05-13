# Phase 7 Plan: Input.vue $attrs pass-through + RecItem h3

**Status:** Active
**Plan #:** 1
**Created:** 2026-05-13

Two surgical, low-risk a11y fixes in two files. Closes audit findings UA-044
(Input.vue swallows `$attrs`, blocking `aria-describedby` / `aria-invalid` /
`aria-required` propagation) and UA-058 (Home trending rec row renders titles
as `<p>` while column rows above/below use `<h3>` — axe-confirmed
heading-order violation). Unblocks Phase 12 AdminRecs SPA quality work which
needs `aria-describedby` on filter inputs. No new visuals, no i18n
changes — pure structural HTML + Vue idiom.

## Tasks

### Input.vue — `$attrs` pass-through (UA-044)

- [ ] Add `defineOptions({ inheritAttrs: false })` to the top of the
  `<script setup>` block in `frontend/web/src/components/ui/Input.vue`,
  immediately after the existing `import { computed, ref } from 'vue'`
  line. This disables Vue 3's default behavior of falling attrs through
  to the root `<div>` wrapper.
- [ ] Add `v-bind="$attrs"` to the inner `<input>` element at line 10 of
  `Input.vue`. Place the binding on its own line between `:id="inputId"`
  and `v-model="model"` so the diff is minimal and the existing typed
  prop bindings (`type`, `placeholder`, `disabled`, `readonly`) remain
  visually grouped. Result: every caller-supplied `aria-*`, `data-*`,
  `name`, `autocomplete`, and event-handler attr now lands on the
  underlying `<input>` element, not the wrapper `<div>`.
- [ ] Do **not** touch the class merging — Vue 3 merges `class` and
  `style` separately from generic `$attrs`, so the existing
  `:class="[inputClasses, $slots.prefix ? 'pl-10' : '', ...]"`
  on the `<input>` continues to merge caller-supplied class strings
  correctly. Caller-supplied `class` props were always landing on the
  wrapper `<div>` anyway (Vue's default behavior with single-root
  components is to attribute-inherit class/style at the root); switching
  `inheritAttrs: false` does **not** affect class merging because class
  is bound explicitly on the `<input>` via `:class`. Verified by reading
  template lines 9-21 and `defineProps` schema at lines 43-55.

### Home.vue — rec card title `<p>` → `<h3>` (UA-058)

- [ ] In `frontend/web/src/views/Home.vue`, line 83, replace the rec card
  title element:
  ```vue
  <p class="text-sm text-white truncate group-hover:text-cyan-400 transition-colors">
  ```
  with:
  ```vue
  <h3 class="text-sm font-medium text-white truncate group-hover:text-cyan-400 transition-colors">
  ```
  Closing tag: `</p>` → `</h3>` on line 85. Preserve indentation, the
  inner `{{ getLocalizedTitle(...) }}` expression, and the `truncate`
  semantics. Add `font-medium` so the visual weight matches the
  canonical column-row h3 pattern at lines 159, 246, 327
  (`text-sm font-medium text-white ... line-clamp-2`).
- [ ] Verify the only `<p>` in the trending rec block (lines 32-88) that
  represented a card title is the one at line 83. The `<p>` at lines
  41-48 is the pin-reason line above the row (unrelated; remains
  `<p>`). No other rec rows on Home use `<p>` as title — confirmed by
  `grep -n "<h3" frontend/web/src/views/Home.vue` already showing the
  canonical h3 in the three column rows.

### Consumer audit — Input.vue regression check

- [ ] Grep all `<Input` consumers and confirm none rely on attrs landing
  on the wrapper `<div>` (the `inheritAttrs: false` failure mode):
  ```bash
  grep -rn "<Input" frontend/web/src/
  ```
  Expected consumers per current codebase: `Game.vue:198`, `Game.vue:219`,
  `SearchAutocomplete.vue:3`. For each: open the call site, list the
  attrs being passed. If any rely on attrs landing on the wrapper
  (e.g. wrapper-scoped CSS selectors, wrapper-targeted event listeners),
  flag for follow-up. **Expected outcome: zero callers affected** —
  practical attrs are typed props (`placeholder`, `label`, `error`,
  `clearable`, etc.) which continue to work via `defineProps`, and
  `class` continues to merge via the explicit `:class` binding on
  `<input>`.
- [ ] If the audit surfaces a real regression, add a follow-up note to
  `07-VERIFICATION.md`. Otherwise verification proceeds.

### Verification

- [ ] `bunx vue-tsc --noEmit` — passes (no new type errors; `defineOptions`
  is a compile-time macro and is fully typed by Vue 3 + vue-tsc).
- [ ] `make redeploy-web` — frontend rebuilt and shipped to the running
  nginx container.
- [ ] `grep -n 'defineOptions\|v-bind="\$attrs"' frontend/web/src/components/ui/Input.vue`
  — confirms **both** lines present (`defineOptions({ inheritAttrs: false })`
  in script and `v-bind="$attrs"` on the `<input>` element).
- [ ] `grep -nE '<h3 class="text-sm font-medium text-white truncate group-hover:text-cyan-400' frontend/web/src/views/Home.vue`
  — confirms the new h3 in the trending rec block. Exactly one match
  expected (line ~83).
- [ ] `grep -cE "<h3[[:space:]]" frontend/web/src/views/Home.vue` — count
  rises from 3 (Ongoing / Top / Announcements columns) to **4** (now
  includes the trending row title).
- [ ] `grep -n "<p class=\"text-sm text-white truncate" frontend/web/src/views/Home.vue`
  — returns **zero matches** (old `<p>` rec-title pattern fully removed).
- [ ] Chrome MCP axe-core re-run on `https://animeenigma.ru/` (logged in as
  `ui_audit_bot` so the trending row populates from seeded data):
  - `heading-order` rule: zero violations. The row sequence
    `<h1>` (sr-only) → `<h2>` (trending row label) → `<h3>` (rec card
    titles) → `<h2>` (next row label) → `<h3>` (column row titles) is
    valid per WCAG SC 1.3.1.
  - No new violations on any rule that was previously clean on Home.
- [ ] Manual smoke probe of the three `<Input>` consumers via
  `https://animeenigma.ru/`:
  - `/game` (Game.vue: two `<Input>` instances — room code + username):
    typing, focus, blur, clear-button all behave identically to
    pre-change behavior. No console errors.
  - SearchAutocomplete on `/` (Home): typing triggers suggestions,
    listbox-id wiring continues to work, no console errors.
- [ ] Manual ARIA probe (DevTools / `read_page`): inject
  `aria-describedby="test-hint"` on one of the Game.vue `<Input>` calls
  via temporary edit (revert immediately) and confirm it lands on the
  inner `<input>` element in the rendered DOM, not the wrapper `<div>`.
  This is the affirmative test that UA-044 is actually fixed.

## Files touched

```
frontend/web/src/components/ui/Input.vue            (defineOptions + v-bind="$attrs")
frontend/web/src/views/Home.vue                     (<p> → <h3> at line 83)
.planning/workstreams/ui-ux-audit/phases/07-input-attrs-recitem-h3/
  07-CONTEXT.md                                     (already exists)
  07-PLAN.md                                        (this file)
  07-SUMMARY.md                                     (written at execute-phase end)
  07-VERIFICATION.md                                (written at execute-phase end)
```

No new files. No locale changes. No backend touches.

## Closes

| Finding | Surface | Mechanism |
|---|---|---|
| UA-044 | `frontend/web/src/components/ui/Input.vue` | `defineOptions({ inheritAttrs: false })` + `v-bind="$attrs"` on inner `<input>` — caller-supplied `aria-*` / `data-*` / `name` / `autocomplete` attrs now propagate to the actual input element |
| UA-058 | `frontend/web/src/views/Home.vue` line 83 | `<p>` → `<h3>` on trending rec card titles — repairs `h2 → h3 → h2` document outline and matches the canonical column-row h3 pattern at lines 159/246/327 |
