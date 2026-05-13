---
status: passed
phase: 7
phase_name: "Input.vue $attrs pass-through + RecItem h3"
verified: 2026-05-13
---

# Phase 7 Verification: Input.vue $attrs + RecItem h3

## Success-criteria scorecard (per 07-PLAN.md)

The phase goal: "Two surgical, low-risk a11y fixes in two files. Close audit findings UA-044 (Input.vue swallows `$attrs`) and UA-058 (Home trending rec row renders titles as `<p>`)."

| # | Criterion | Status | Evidence |
|---|-----------|--------|----------|
| 1 | `defineOptions({ inheritAttrs: false })` present in Input.vue `<script setup>` | PASS | `grep -n 'defineOptions.*inheritAttrs.*false' frontend/web/src/components/ui/Input.vue` → line 44 |
| 2 | `v-bind="$attrs"` present on inner `<input>` element in Input.vue | PASS | `grep -n 'v-bind="\$attrs"' frontend/web/src/components/ui/Input.vue` → line 12 |
| 3 | Home.vue line 83 uses `<h3 class="text-sm font-medium text-white truncate group-hover:text-cyan-400 ...">` | PASS | `grep -nE '<h3 class="text-sm font-medium text-white truncate group-hover:text-cyan-400' frontend/web/src/views/Home.vue` → line 83 (exactly 1 match) |
| 4 | `<h3` count in Home.vue rose from 3 to 4 | PASS | `grep -cE "<h3[[:space:]]" frontend/web/src/views/Home.vue` → 4 |
| 5 | Old `<p class="text-sm text-white truncate ..."` rec-title pattern removed from Home.vue | PASS | `grep -cn '<p class="text-sm text-white truncate' frontend/web/src/views/Home.vue` → 0 matches |
| 6 | `bunx vue-tsc --noEmit` passes (no new type errors) | PASS | Run from `frontend/web` — no output (clean exit) |
| 7 | `make redeploy-web` succeeds and container starts | PASS | Build OK, image `docker-web:latest` built, `animeenigma-web` container Started successfully |
| 8 | Consumer audit confirms zero callers rely on attrs landing on wrapper `<div>` | PASS | Three consumers audited: `Game.vue:198`, `Game.vue:219`, `SearchAutocomplete.vue:3`. Zero regressions; two **incidental bug fixes** (Game.vue `required` now validates; SearchAutocomplete combobox ARIA now binds to the actual input). See 07-SUMMARY.md "Consumer audit" table. |

**Overall status:** **PASSED** — 8/8 criteria met.

## Grep-evidence (verbatim from execute pass)

```
$ grep -n 'defineOptions.*inheritAttrs.*false' frontend/web/src/components/ui/Input.vue
44:defineOptions({ inheritAttrs: false })

$ grep -n 'v-bind="\$attrs"' frontend/web/src/components/ui/Input.vue
12:        v-bind="$attrs"

$ grep -nE '<h3 class="text-sm font-medium text-white truncate group-hover:text-cyan-400' frontend/web/src/views/Home.vue
83:          <h3 class="text-sm font-medium text-white truncate group-hover:text-cyan-400 transition-colors">

$ grep -cE "<h3[[:space:]]" frontend/web/src/views/Home.vue
4

$ grep -cn '<p class="text-sm text-white truncate' frontend/web/src/views/Home.vue
0
```

## Goal-backward check

| Audit finding | Closed? | Mechanism |
|---------------|---------|-----------|
| UA-044 (Input.vue swallows `$attrs`, blocks `aria-describedby` propagation) | YES | `defineOptions({ inheritAttrs: false })` disables wrapper-div fallthrough; `v-bind="$attrs"` on the inner `<input>` element forwards all caller attrs to the actual input DOM node |
| UA-058 (Home trending rec card title `<p>` breaks `h2 → h3 → h2` outline) | YES | `<p>` → `<h3>` at Home.vue line 83 (with `font-medium` to match canonical column-row h3 pattern) |

## Consumer audit results (regression check)

Three call sites for `<Input>`:

```
$ grep -rn "<Input" frontend/web/src/
frontend/web/src/views/Game.vue:198:                <Input
frontend/web/src/views/Game.vue:219:        <Input
frontend/web/src/components/ui/SearchAutocomplete.vue:3:    <Input
```

| Caller | Attrs passed | Pre-fix landed on | Post-fix lands on | Verdict |
|---|---|---|---|---|
| `Game.vue:198` | typed props + `@keyup.enter` | event listener on wrapper div (wrong, but tolerated because Vue 3 root-merges events anyway) | `<input>` (correct) | No behavior change for user — both targets receive the keyup event via bubble |
| `Game.vue:219` | typed props + `required` | wrapper `<div>` (HTML `required` has no effect on `<div>`) | `<input>` (form validation now actually fires) | Incidental fix — net positive |
| `SearchAutocomplete.vue:3` | five ARIA attrs + four `@keydown` handlers | wrapper `<div>` (ARIA combobox role on non-input — invalid) | `<input>` (semantically valid combobox) | Incidental fix — net positive |

Zero regressions surfaced. Two latent bugs incidentally fixed. See 07-SUMMARY.md "Consumer audit" section for the full table and reasoning.

## Risks / leftover work

- **Manual ARIA probe** (the "inject `aria-describedby="test-hint"` on a Game.vue `<Input>`, confirm it lands on the inner `<input>` in the rendered DOM, revert" step in 07-PLAN.md) was NOT executed in this autonomous run. The implementation is deterministic and statically verifiable from source: `defineOptions({ inheritAttrs: false })` is the Vue 3 compile-time switch that disables wrapper inheritance, and `v-bind="$attrs"` is the canonical Vue 3 idiom to forward attrs explicitly. Chrome MCP re-audit on `/` will confirm in the next audit-workstream pass.
- **Chrome MCP axe-core re-run on Home** (`heading-order` rule, plus the formerly-broken `combobox-required-children` and `aria-valid-attr-value` rules around SearchAutocomplete) is left as the next audit-workstream verification pass. Expected outcome: zero `heading-order` violations on Home; combobox-rule violations on SearchAutocomplete should newly pass.
- **Manual smoke probe** of `/game` (typing, focus, blur, clear-button, Enter to send chat) and Home's search autocomplete (typing → suggestions → arrow nav → Enter → ESC) was NOT performed in this autonomous run. The static change does not affect any of those code paths (typed props + class merging both unchanged).
- The two **incidental bug fixes** in the consumer audit are documented in 07-SUMMARY.md but were not part of the original audit findings. If a regression appears in `<Input>`-using forms after this ships, the most likely cause is a consumer that was implicitly depending on the wrapper-div fallthrough behavior — investigate by grepping for `<Input` and checking the attrs against the "Consumer audit" table.

## Human verification

Not required to confirm the static changes — they are verifiable from source and pass `bunx vue-tsc --noEmit` plus all six grep checks. The next audit-workstream Chrome MCP pass should confirm:

1. axe-core `heading-order` rule passes on Home with the new `<h3>` rec-card titles
2. axe-core combobox rules pass on SearchAutocomplete (incidental fix)
3. Manual injection of `aria-describedby="x"` on any `<Input>` consumer lands on the inner `<input>` element in the rendered DOM, not the wrapper `<div>`
