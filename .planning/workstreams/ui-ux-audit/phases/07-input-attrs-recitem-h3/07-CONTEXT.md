# Phase 7: Input.vue $attrs + RecItem h3 - Context

**Gathered:** 2026-05-13
**Status:** Ready for planning
**Mode:** Auto-generated (autonomous run, two-touch surgical fix)

<domain>
## Phase Boundary

Two surgical changes blocked downstream a11y work:

1. **UA-044 — Input.vue `v-bind="$attrs"` pass-through.** The current `<Input>` wrapper swallows all attributes; downstream surfaces cannot bind `aria-describedby`, `aria-invalid`, `aria-required`, etc. to the underlying `<input>`. Add `v-bind="$attrs"` on the inner `<input>` element so consumer-supplied ARIA attributes propagate.

2. **UA-058 — RecItem title heading order.** The rec row in `Home.vue` (`v-for="item in trendingRecs"`) renders the title as a `<p>` while the row label above is `<h2>`. axe-confirmed heading-order violation: h2 → no h3 → next row's h2 again. Convert the title to `<h3>` to repair the document outline. Apply the same fix wherever rec card titles render — currently only the trending row uses `<p>`; the three column rows (Ongoing/Top/Announcements) already use `<h3>` at lines 159, 246, 327.

Scope is **two files** plus a downstream audit pass on Input.vue consumers to confirm no regression (none expected — props-only bindings keep working; `inheritAttrs` defaults to true on the root `<div>` which is wrong; we need to set `inheritAttrs: false` AND `v-bind="$attrs"` on the inner `<input>`).

</domain>

<decisions>
## Implementation Decisions

### Claude's Discretion (autonomous mode)

- **Input.vue strategy**: add `defineOptions({ inheritAttrs: false })` to disable default attr inheritance on the root `<div>` wrapper, then `v-bind="$attrs"` on the inner `<input>` element. Vue 3 idiom. No prop schema changes — existing typed props (`modelValue`, `placeholder`, etc.) continue to work via `defineProps`.
- **Class merging**: existing `:class="[inputClasses, $slots.prefix ? 'pl-10' : '', ...]"` keeps its computed bindings. Caller-supplied `class` attrs still merge correctly because Vue merges class/style separately from generic `$attrs`. Verified by reading the template at lines 9-21 of `frontend/web/src/components/ui/Input.vue`.
- **RecItem title**: change `<p class="text-sm text-white truncate ...">` to `<h3 class="text-sm font-medium text-white truncate ...">`. Keep all visual classes; add `font-medium` to match the column-rows' h3 styling (currently `text-sm font-medium text-white ... line-clamp-2`).
- **No i18n changes**: both are structural HTML / Vue changes. No new strings.
- **No new files**: in-place edits only.
- **Consumer audit**: grep all callers of `<Input` in the codebase. Confirm none rely on attrs landing on the wrapper `<div>` (which would be the `inheritAttrs: false` failure mode). Expected zero callers affected — the only attrs in practice are `placeholder`, `label`, `error`, etc. which are typed props.

### Locked from ROADMAP

- Closes UA-044 and UA-058 only. No other Input.vue or Home.vue findings touched.
- Phase 7 unblocks Phase 12 (AdminRecs SPA quality) which needs `aria-describedby` on filter inputs.

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets

- `frontend/web/src/components/ui/Input.vue` — only two changes needed: add `defineOptions({ inheritAttrs: false })` and `v-bind="$attrs"` on the `<input>` element.
- `frontend/web/src/views/Home.vue` — line 83 has the `<p>`-as-rec-title to swap; line 32-88 is the full rec row block. Lines 159, 246, 327 already use `<h3>` (Ongoing/Top/Announcements columns) — confirms `<h3>` is the canonical pattern.

### Established Patterns

- Vue 3 `defineOptions` (compile-time macro) is used elsewhere in the codebase for similar attr-inheritance overrides — to be verified during execution but the pattern is standard.
- Tailwind class composition: rec row titles use `text-sm font-medium text-white ... transition-colors`.

### Integration Points

- Input.vue is consumed across Profile, AdminRecs, Browse search, etc. Need to grep and audit consumers.
- Home.vue is the only consumer of the trending rec row pattern with `<p>`-as-title.

</code_context>

<specifics>
## Specific Ideas

- For Input.vue, set `inheritAttrs: false` is required — without it, attrs land on both the wrapper `<div>` (Vue's default) AND the `<input>` (via explicit `v-bind`), creating duplicate `aria-*` attributes. The pair must move together.
- The h3 swap is single-line; the only risk is that the `<h3>` element's default browser styling differs from `<p>` (margins, font-weight). The Tailwind classes `text-sm font-medium text-white truncate` explicitly set font-size and weight, so visually nothing changes. Reset margins are handled by Tailwind's preflight.

</specifics>

<deferred>
## Deferred Ideas

- Broader heading-order sweep across all views → Phase 4 already addressed Browse, Profile, etc. If new violations emerge in Phase 20 polish audit, address there.
- Generic ARIA prop layer on Input.vue (e.g. typed `ariaInvalid` prop) — premature abstraction; `$attrs` pass-through is the standard Vue 3 escape hatch.

</deferred>
