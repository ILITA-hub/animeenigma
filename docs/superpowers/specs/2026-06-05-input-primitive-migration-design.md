# Input-Primitive Migration — Design

**Date:** 2026-06-05
**Status:** Approved (design), pending implementation plan
**Scope:** Migrate hand-rolled native form controls to the shared shadcn-vue
`Input` / `Checkbox` primitives (and a new `RadioGroup` primitive). Part of the
"Neon Tokyo" design-system v2 reuse effort (follows v2.0 Phase 1 Button reuse).

---

## Problem

The frontend ships a fully-featured `Input` primitive (`components/ui/Input.vue`)
but it has **1 importer** (`Game.vue`, which *still* hand-rolls one `number` input
beside it). A sweep found native `<input>` tags in 12 files; excluding the
primitive itself and one `:deep(input)` `<style>` selector in `Home.vue` (which
already consumes `SearchAutocomplete`), **38 real native inputs remain across 11
template files**. The
Home already consumes `SearchAutocomplete`). The
`Checkbox` primitive (boolean / indeterminate, reka-backed) has **0 adoption**.
There is **no radio primitive** at all.

This is the single largest primitive-adoption gap in the design system. Closing it
makes form controls consistent (focus ring, error/hint, sizing tokens) and removes
~35 bespoke control implementations.

## Goals

- Migrate every text-like native input to `<Input>`.
- Migrate plain checkboxes to `<Checkbox>`.
- Systematize radios behind a new reka-backed `<RadioGroup>` primitive.
- **Value-preserving**: no intended visual or behavioral diff. Inline-edit commit
  semantics (commit-on-blur) and search keyboard nav must be preserved exactly.
- Keep the design-system lint gate green; no new off-palette / hex.

## Non-goals

- No redesign of any form. No new fields, no layout changes.
- No migration of the `Switch` primitive (separate, out of scope).
- No change to provider brand-accent styling (kept as bespoke).

---

## The 40 native inputs — disposition

| Disposition | Count | Notes |
|---|---|---|
| → `<Input>` | 28 | text / number / url / **date** (2) |
| → `<Checkbox>` | 3 | plain cyan checkboxes |
| → `<RadioGroup>` (new) | 3 sets | BrowseSidebar kind / status / sort |
| Documented bespoke-keep (native) | 4 | provider checkboxes, FeedbackButton radio, range slider, file input |
| Sub-total accounted | — | (radio "sets" expand to the raw `radio` tags) |

### Binding patterns (drive the migration mechanics)

1. **Clean `v-model`** (text/number/url) — direct `<Input v-model>` swap. Pass-through
   `$attrs` carries `placeholder`, `required`, `aria-*`, `@input`, `@keydown`, etc.
   - Caveat: `v-model.number` (RawLibrary `searchMalId`, Game `maxPlayers`) — the
     primitive does not implement `modelModifiers`, so it emits a **string**. The
     consumer must coerce (`Number(...)`) or keep `.number` handling in the parent.
2. **Controlled `:value` + `@blur`/`@change`** (Profile inline editors,
   AdminCollectionEdit `sort_order`, BrowseSidebar year) — commit-on-blur via
   `event.target.valueAsNumber` / `.value`. Migrate by binding **`:model-value`**
   (NOT `:value` — the primitive's internal `v-model` overrides a passed `value`)
   and keeping the `@blur`/`@change` listener. Vue merges the passthrough listener
   (`$attrs.onBlur`) with the primitive's internal `@blur`, so both fire and
   `event.target` is the real inner `<input>`. No per-keystroke commit is
   introduced (parent ignores `update:modelValue`).
3. **`ref`-based focus** (Navbar, GenreFilterPopup, AdminRecsPicker) — parent calls
   `inputRef.value.focus()`. The component `ref` is the instance, not the element,
   so this breaks unless the primitive exposes a focus method.

---

## Primitive changes

### 1. `Input.vue`

- Add `'date'` to the `type` union (`'text' | 'email' | 'password' | 'search' |
  'number' | 'tel' | 'url' | 'date'`). One line; covers the 2 Profile date fields.
- `defineExpose({ focus })`, where `focus()` calls `.focus()` on a template ref
  bound to the inner `<input>`. Lets Navbar / GenreFilterPopup / AdminRecsPicker
  keep programmatic focus. (Add an `inputRef` template ref; expose only `focus`
  to keep the surface minimal.)
- No styling change. Existing cyan/pink classes are brand-exempt — gate stays green.

### 2. New `RadioGroup.vue`

- reka `RadioGroupRoot` + `RadioGroupItem` + `RadioGroupIndicator`, mirroring
  `Checkbox.vue`'s structure and token usage.
- API: `v-model` (selected value, string), `:disabled`, plus an `options` prop OR a
  default slot. Recommendation: **`options: { value, label }[]` prop** so the 3
  call sites stay terse (`<RadioGroup v-model="x" :options="kindOptions" />`),
  matching how `Select` is consumed. Label text + per-item layout
  (`flex items-center gap-2`, `text-white/70 hover:text-white`) baked in to match
  the current BrowseSidebar look.
- Styling: cyan `data-[state=checked]` dot, `border-input`, matches Checkbox.
- Export from `components/ui/index.ts`.

### 3. `Checkbox.vue`

- **No change.** Consumed as `<label class="…"><Checkbox v-model/ :model-value+@update/><span>…</span></label>`.
- Multi-select array checkboxes (genre) bridge via
  `:model-value="arr.includes(id)"` + `@update:model-value="toggle(id, $event)"`.
  The handler already takes a boolean; `Checkbox` emits `boolean | 'indeterminate'`
  — these are never indeterminate, so boolean coercion is safe.

---

## Migration map (per file)

**→ `<Input>` (28):**

- `views/Profile.vue` (10): `searchQuery`, `editingScore` (number, blur),
  `episodes` (number, blur), `started_at` (date, change), `completed_at`
  (date, change), `score` (number, blur), `malUsername`, `shikimoriNickname`,
  `publicId`, `skipIntroSec` (number, change).
- `views/admin/AdminCollectionEdit.vue` (7): `slug`, `title`, `title_ru`,
  `title_jp`, `cover_image_url` (url), item `searchQuery`, `sort_order`
  (number, change).
- `views/admin/RawLibrary.vue` (3): `searchQuery`, `searchMalId` (number),
  `pendingLinkSearchQueries[job.id]`.
- `components/browse/BrowseSidebar.vue` (2): `yearFrom`, `yearTo` (number, change).
- `components/layout/Navbar.vue` (1): search — `prefix` slot for the magnifier
  icon; `focus()` for the existing focus behavior; keep all `@keydown.*` handlers.
- `components/ui/GenreFilterPopup.vue` (1): search — `focus()`; keep
  `@keydown` handler.
- `views/admin/AdminRecsPicker.vue` (1): `input` — `focus()`; keep `required`,
  `autocomplete`, `spellcheck`, `aria-busy`.
- `views/Anime.vue` (1): `editShikimoriId`.
- `views/ProfileSetup.vue` (1): `publicId` (keep `@keyup.enter`).
- `views/Game.vue` (1): `maxPlayers` — replace the lone native input; the file
  already imports `Input`.

**→ `<Checkbox>` (3):**

- `components/browse/BrowseSidebar.vue`: genre checkbox (multi, array bridge).
- `views/Profile.vue`: `publicStatuses` checkbox (multi, array bridge).
- `views/admin/AdminCollectionEdit.vue`: `form.published` (clean boolean `v-model`).

**→ `<RadioGroup>` (3 sets):**

- `components/browse/BrowseSidebar.vue`: `kind`, `status`, `sort`. Each currently a
  `v-for` of `<label><input type=radio :checked @change></label>`; collapses to one
  `<RadioGroup v-model :options>` per set.

**Bespoke-keep (native, add `<!-- bespoke-keep: reason -->`):**

- `BrowseSidebar` provider checkboxes — per-provider brand accent (`opt.accent`),
  the cyan-only primitive can't model it without a visible diff.
- `FeedbackButton` radio — `sr-only` input inside a custom card selector
  (segmented-toggle pattern; the primitive would replace the card UI).
- `BrowseSidebar` score **range** slider — no slider primitive.
- `Profile` avatar **file** input — file upload, out of primitive scope.

---

## Verification

- **Unit (vitest):**
  - Extend `Input.spec.ts`: `type="date"` renders; `focus()` exposed and focuses
    the inner input; `:model-value` + `@blur` passthrough fires with the native
    input as `event.target`.
  - New `RadioGroup.spec.ts`: renders options, `v-model` updates on select,
    checked state reflects model, disabled.
  - Existing `Checkbox.spec.ts` covers the primitive; add a render assertion at one
    array-bridge call site if practical.
- **Gates (all must be green):** `bunx vitest run`, `bunx vue-tsc --noEmit`,
  `make lint-design` (+ `--selftest` unchanged), `bun run build`.
- **In-browser smoke (DS-NF-06, desktop + mobile)** — jsdom can't catch Tailwind v4
  cascade: Browse filters (checkboxes/radios/year/range), Profile watchlist inline
  edits + settings, admin collection/recs/raw forms, Navbar search + autocomplete
  keyboard nav, Game create-room.

## Risks

- **Inline-edit commit semantics** (Profile) — the highest-risk migration. Must
  verify commit-on-blur still fires and no per-keystroke API calls are introduced.
- **`v-model.number`** coercion — string emission could surface `"12"` where a
  number is expected. Coerce at the call site.
- **Navbar autocomplete** — heavy keyboard handling + ref focus; smoke-test the
  arrow/enter/escape flow explicitly.
- **`RadioGroup` `options` vs slot** — if a call site needs custom per-item markup
  later, the `options` prop is limiting; acceptable for the 3 current plain sets.

## Rollout

Single workstream, sequenced: (1) primitive changes + their tests, (2) low-risk
clean `v-model` swaps, (3) controlled inline-edit swaps, (4) checkbox/radio cluster,
(5) ref-focus search inputs, (6) gates + in-browser smoke. One
`/animeenigma-after-update` at the end (batch; no changelog spam).

## Metrics (project convention — no time units)

- **UXΔ = +1 (Better)** — consistent focus/error/hint and control sizing across all
  forms; near-zero user-visible change by design.
- **CDI = 0.04 * 21** — spread across 12 files + 2 primitives + 1 new primitive
  (moderate spread), low shift (value-preserving), Effort_Fib 21.
- **MVQ = Griffin 85%/85%** — disciplined, gated, value-preserving migration;
  slop-resistant via unit tests + lint gate + in-browser smoke.
