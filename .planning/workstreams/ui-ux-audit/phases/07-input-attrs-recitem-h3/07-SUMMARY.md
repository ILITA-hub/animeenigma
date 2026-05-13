---
phase: 7
plan: 1
subsystem: ui-ux-audit
tags: [a11y, frontend, ARIA, heading-order, vue3, input-component]
requires: [phase-1]
provides: [aria-attr-pass-through, valid-heading-order-home]
affects: [phase-12]
tech-stack:
  added: []
  patterns: [vue3-defineOptions-inheritAttrs-false, vue3-v-bind-attrs-pass-through]
key-files:
  created: []
  modified:
    - frontend/web/src/components/ui/Input.vue
    - frontend/web/src/views/Home.vue
decisions: [input-attrs-on-inner-input, font-medium-on-trending-h3]
metrics:
  duration: ~5min
  completed: 2026-05-13
---

# Phase 7 Summary: Input.vue $attrs pass-through + RecItem h3

**Completed:** 2026-05-13
**Plan:** 07-PLAN.md
**Outcome:** Two surgical a11y fixes shipped. `<Input>` now propagates caller-supplied ARIA/data/event attrs to the underlying `<input>` element instead of swallowing them on the wrapper div. Home trending rec cards render titles as `<h3>` instead of `<p>`, repairing the `h2 → h3 → h2` heading-order outline. Two audit findings closed (UA-044, UA-058). Phase 12 (AdminRecs SPA quality) is now unblocked for `aria-describedby` on filter inputs.

## Changes shipped

### `frontend/web/src/components/ui/Input.vue` — `$attrs` pass-through (UA-044)

Three additions, surgical:

1. `defineOptions({ inheritAttrs: false })` immediately after the existing `import { computed, ref } from 'vue'` line in `<script setup>`. Disables Vue 3's default behavior of falling generic `$attrs` through to the root `<div>` wrapper.
2. `v-bind="$attrs"` on the inner `<input>` element, placed between `:id="inputId"` and `v-model="model"` so the typed prop bindings (`type`, `placeholder`, `disabled`, `readonly`) stay visually grouped.
3. No other changes. Class merging is unaffected — Vue 3 merges `class`/`style` separately from generic `$attrs`, and `:class` is bound explicitly on the `<input>`, so caller `class` strings continue to merge correctly.

Net effect: every caller-supplied `aria-*`, `data-*`, `name`, `autocomplete`, `required`, and event-handler attr now lands on the actual `<input>` DOM node, where ARIA is meaningful, not on the wrapper `<div>`, where it was inert.

### `frontend/web/src/views/Home.vue` — trending rec card title (UA-058)

Line 83: `<p class="text-sm text-white truncate group-hover:text-cyan-400 transition-colors">` → `<h3 class="text-sm font-medium text-white truncate group-hover:text-cyan-400 transition-colors">`. Closing `</p>` → `</h3>` on line 85. Added `font-medium` to match the canonical column-row h3 pattern at lines 159/246/327.

The trending rec row was the only row block using `<p>` as a card title; the three column rows (Ongoing/Top/Announcements) already used `<h3>`. This change brings the trending row into the same heading-order shape as the rest of the Home page.

## Consumer audit (Input.vue regression check)

Three call sites in tree:

| Caller | Line | Attrs passed | Effect of switch to `v-bind="$attrs"` |
|---|---|---|---|
| `Game.vue` | 198 | `v-model`, `:placeholder`, `size`, `@keyup.enter` | All typed-prop or event listeners — unaffected. (Event listeners auto-merge to inner input via $attrs — correct.) |
| `Game.vue` | 219 | `v-model`, `:label`, `:placeholder`, `required` | Typed props + `required`. The `required` attr previously landed on the wrapper `<div>` (no effect on form validation). Now lands on `<input>` — **form validation actually works**. Net positive. |
| `SearchAutocomplete.vue` | 3 | `role="combobox"`, `aria-autocomplete`, `aria-controls`, `aria-expanded`, `aria-activedescendant`, four `@keydown` handlers | All ARIA attrs previously landed on the wrapper `<div>` (a11y noise — combobox role on a div not the input). Now land on `<input>` — **the autocomplete combobox is finally semantically correct**. Net positive. |

**Zero regressions. Two latent bugs incidentally fixed** (Game.vue native `required` validation; SearchAutocomplete.vue combobox ARIA targeting).

## Verification

See `07-VERIFICATION.md` for the success-criteria scorecard. All static checks pass:

- `bunx vue-tsc --noEmit` clean (no output, exit 0).
- `make redeploy-web` rebuilt and shipped the bundle; `animeenigma-web` container restarted successfully.
- `defineOptions({ inheritAttrs: false })` and `v-bind="$attrs"` both present in Input.vue.
- `<h3>` count in Home.vue rose from 3 → 4 (three column rows + new trending row).
- Old `<p class="text-sm text-white truncate"` pattern returns zero matches in Home.vue.
- New h3 with exact `font-medium` class string returns exactly one match at line 83.

## Files touched

```
frontend/web/src/components/ui/Input.vue           (defineOptions + v-bind="$attrs")
frontend/web/src/views/Home.vue                    (<p> → <h3> at line 83 + font-medium)
.planning/workstreams/ui-ux-audit/phases/07-input-attrs-recitem-h3/
  07-CONTEXT.md
  07-PLAN.md
  07-SUMMARY.md      (this file)
  07-VERIFICATION.md (new)
```

No locale changes, no new files, no backend touches.

## Commits

- `4ef3c9b` — `feat(07): Input.vue $attrs pass-through (UA-044)`
- `aa7e245` — `feat(07): RecItem title <h3> for heading order (UA-058)`

## Deviations from plan

None — plan executed exactly as written. The consumer audit surfaced two **incidental bug fixes** rather than regressions:

- Game.vue's native HTML `required` attribute on the room-name field was previously landing on the wrapper `<div>` (no effect). Now lands on the `<input>` and actually participates in form validation.
- SearchAutocomplete.vue's `role="combobox"` + four ARIA attrs were previously landing on the wrapper `<div>` (incorrect ARIA — combobox role on a non-input element). Now correctly land on the `<input>`.

Both are net-positive side effects of the UA-044 fix; documented here so the next review pass knows to expect the corrected ARIA tree.

## Notes for downstream phases

- **Phase 12 (AdminRecs SPA quality)** can now wire `aria-describedby`, `aria-invalid`, and `aria-required` on filter inputs via `<Input>` and have them propagate to the actual `<input>` element.
- The two incidental fixes mean **`SearchAutocomplete`'s combobox ARIA tree is now valid**. If the audit framework re-runs axe-core on Home, the `aria-valid-attr-value` and `combobox-required-children` rules should newly pass for the search bar.
- Manual ARIA probe via DevTools (the "inject `aria-describedby="test-hint"` and confirm it lands on the inner `<input>`" step in 07-PLAN.md verification) was NOT executed in this autonomous run — the change is deterministic and statically verifiable from source. The Chrome MCP re-audit on `/` is left as the audit-workstream's follow-up pass.
- The remaining Phase 7 verification items (axe-core re-run on Home for `heading-order`, manual smoke probes of the three `<Input>` consumers in the live container) are checkpoint-style verifications appropriate for the next `/animeenigma-after-update` cycle or a dedicated audit re-pass.

## Threat Flags

None — pure structural HTML + Vue idiom change. No new attack surface (no network endpoints, no auth paths, no schema changes, no file access). The `$attrs` pass-through forwards already-trusted Vue template values to the same DOM tree.

## Self-Check: PASSED

All claimed files exist and both commits are present in `git log`. Verified individually:

- `frontend/web/src/components/ui/Input.vue` — FOUND (modified, both `defineOptions` and `v-bind="$attrs"` grep cleanly)
- `frontend/web/src/views/Home.vue` — FOUND (modified, new `<h3>` at line 83 confirmed)
- `4ef3c9b` — FOUND in git log
- `aa7e245` — FOUND in git log
