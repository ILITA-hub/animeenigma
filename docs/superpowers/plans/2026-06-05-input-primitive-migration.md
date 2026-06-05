# Input-Primitive Migration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Migrate 38 hand-rolled native form controls to the shared shadcn-vue `Input` / `Checkbox` primitives and a new `RadioGroup` primitive, value-preserving (no visual/behavioral diff).

**Architecture:** Two small enhancements to `Input.vue` (`date` type + `defineExpose({ focus })`), one new reka-backed `RadioGroup.vue` primitive, then per-file swaps of native `<input>` for the primitives. Bindings: clean `v-model` → direct; controlled `:value`+`@blur` editors → `:model-value` + passthrough listener (NOT `:value`); `ref`-focus searches → primitive `focus()`. Four controls stay native as documented bespoke-keeps.

**Tech Stack:** Vue 3 `<script setup>`, TypeScript, reka-ui (RadioGroup/Checkbox), Tailwind v4, Vitest + @vue/test-utils, `vue-tsc`, custom `design-system-lint.sh` gate. Frontend uses **bun** / **bunx** (never npm/npx). All commands run from `frontend/web/`.

**Reference spec:** `docs/superpowers/specs/2026-06-05-input-primitive-migration-design.md`

---

## File Structure

**Primitives (modified / created):**
- `frontend/web/src/components/ui/Input.vue` — add `'date'` type, expose `focus()`.
- `frontend/web/src/components/ui/Input.spec.ts` — extend tests.
- `frontend/web/src/components/ui/RadioGroup.vue` — **new** primitive.
- `frontend/web/src/components/ui/RadioGroup.spec.ts` — **new** tests.
- `frontend/web/src/components/ui/index.ts` — export `RadioGroup`.

**Call sites (template swaps), grouped by task:**
- Clean v-model: `views/Anime.vue`, `views/ProfileSetup.vue`, `views/Game.vue`, `views/admin/RawLibrary.vue`, `views/admin/AdminCollectionEdit.vue` (text), `views/Profile.vue` (text).
- Ref-focus search: `components/ui/GenreFilterPopup.vue`, `views/admin/AdminRecsPicker.vue`, `components/layout/Navbar.vue`.
- Controlled inline-edit: `views/Profile.vue` (number/date), `views/admin/AdminCollectionEdit.vue` (sort_order), `components/browse/BrowseSidebar.vue` (year).
- Checkbox: `components/browse/BrowseSidebar.vue` (genre), `views/Profile.vue` (publicStatuses), `views/admin/AdminCollectionEdit.vue` (published).
- RadioGroup: `components/browse/BrowseSidebar.vue` (kind/status/sort).
- Bespoke-keep comments: `components/browse/BrowseSidebar.vue` (provider, range), `components/layout/FeedbackButton.vue` (radio), `views/Profile.vue` (file).

**Verification gates (run from `frontend/web/`):**
- `bunx vitest run` — unit tests
- `bunx vue-tsc --noEmit` — type-check
- `bash scripts/design-system-lint.sh` — design-system gate (or `make lint-design` from repo root)
- `bun run build` — production build

---

## Task 1: Input primitive — add `date` type + expose `focus()`

**Files:**
- Modify: `frontend/web/src/components/ui/Input.vue`
- Test: `frontend/web/src/components/ui/Input.spec.ts`

- [ ] **Step 1: Write the failing tests**

Append these `it(...)` blocks inside the existing `describe('Input.vue', () => { ... })` in `src/components/ui/Input.spec.ts` (before its closing `})`):

```ts
  it('accepts type="date" and renders a date input', () => {
    const w = mount(Input, { props: { type: 'date' } })
    expect(w.find('input').attributes('type')).toBe('date')
  })

  it('exposes focus() which focuses the inner input', () => {
    const w = mount(Input, { attachTo: document.body })
    const vm = w.vm as unknown as { focus: () => void }
    expect(typeof vm.focus).toBe('function')
    vm.focus()
    expect(document.activeElement).toBe(w.find('input').element)
    w.unmount()
  })

  it('controlled :model-value + passthrough @blur fires with the native input as target', async () => {
    const onBlur = vi.fn()
    const w = mount(Input, { props: { modelValue: '7' }, attrs: { onBlur } })
    await w.find('input').trigger('blur')
    expect(onBlur).toHaveBeenCalledTimes(1)
    expect(onBlur.mock.calls[0][0].target).toBe(w.find('input').element)
  })
```

Add `vi` to the import at the top of the file:

```ts
import { describe, it, expect, vi } from 'vitest'
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd frontend/web && bunx vitest run src/components/ui/Input.spec.ts`
Expected: FAIL — `type="date"` test fails (TS/runtime rejects `'date'`) and `focus()` test fails (`vm.focus` is not a function).

- [ ] **Step 3: Add `'date'` to the type union**

In `src/components/ui/Input.vue`, change the `type` line in the `Props` interface (currently line ~49):

```ts
  type?: 'text' | 'email' | 'password' | 'search' | 'number' | 'tel' | 'url' | 'date'
```

- [ ] **Step 4: Add an inner ref and expose `focus()`**

In `src/components/ui/Input.vue`:

(a) Add `ref="inputRef"` to the `<input>` element (line ~10, alongside the existing `:id`):

```html
      <input
        :id="inputId"
        ref="inputRef"
        v-bind="$attrs"
```

(b) In `<script setup>`, after the existing `const focused = ref(false)` line, add the ref + expose:

```ts
const inputRef = ref<HTMLInputElement | null>(null)
defineExpose({ focus: () => inputRef.value?.focus() })
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd frontend/web && bunx vitest run src/components/ui/Input.spec.ts`
Expected: PASS (all Input tests green).

- [ ] **Step 6: Type-check**

Run: `cd frontend/web && bunx vue-tsc --noEmit`
Expected: no errors.

- [ ] **Step 7: Commit**

```bash
git add frontend/web/src/components/ui/Input.vue frontend/web/src/components/ui/Input.spec.ts
git commit -m "feat(ui): Input — add date type + expose focus()"
```

---

## Task 2: New `RadioGroup` primitive

**Files:**
- Create: `frontend/web/src/components/ui/RadioGroup.vue`
- Create: `frontend/web/src/components/ui/RadioGroup.spec.ts`
- Modify: `frontend/web/src/components/ui/index.ts`

The primitive mirrors `Checkbox.vue` (reka-backed, cyan `data-[state=checked]` styling, inline SVG — no lucide). API: `v-model` (selected string value) + `options: { value, label }[]` + optional `disabled`. Layout per option matches BrowseSidebar's current look (`flex items-center gap-2`, `text-white/70 hover:text-white`).

- [ ] **Step 1: Write the failing tests**

Create `src/components/ui/RadioGroup.spec.ts`:

```ts
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import RadioGroup from './RadioGroup.vue'

const OPTIONS = [
  { value: '', label: 'Any' },
  { value: 'tv', label: 'TV' },
  { value: 'movie', label: 'Movie' },
]

describe('RadioGroup.vue (Reka RadioGroup, string v-model)', () => {
  it('renders a radiogroup with one radio per option', () => {
    const w = mount(RadioGroup, { props: { modelValue: '', options: OPTIONS } })
    expect(w.find('[role="radiogroup"]').exists()).toBe(true)
    expect(w.findAll('[role="radio"]')).toHaveLength(3)
  })

  it('renders each option label', () => {
    const w = mount(RadioGroup, { props: { modelValue: '', options: OPTIONS } })
    expect(w.text()).toContain('Any')
    expect(w.text()).toContain('TV')
    expect(w.text()).toContain('Movie')
  })

  it('supports an empty-string option value as selectable', () => {
    const w = mount(RadioGroup, { props: { modelValue: '', options: OPTIONS } })
    // The first radio (value="") is the checked one when modelValue is "".
    const first = w.findAll('[role="radio"]')[0]
    expect(first.attributes('aria-checked')).toBe('true')
  })

  it('selecting an option emits update:modelValue with its value', async () => {
    const w = mount(RadioGroup, { props: { modelValue: '', options: OPTIONS } })
    await w.findAll('[role="radio"]')[1].trigger('click')
    expect(w.emitted('update:modelValue')).toBeTruthy()
    expect(w.emitted('update:modelValue')!.at(-1)).toEqual(['tv'])
  })

  it('reflects the checked option from modelValue', () => {
    const w = mount(RadioGroup, { props: { modelValue: 'movie', options: OPTIONS } })
    const radios = w.findAll('[role="radio"]')
    expect(radios[2].attributes('aria-checked')).toBe('true')
  })

  it('disables all items when disabled', () => {
    const w = mount(RadioGroup, { props: { modelValue: '', options: OPTIONS, disabled: true } })
    for (const r of w.findAll('[role="radio"]')) {
      expect(r.attributes('disabled')).toBeDefined()
    }
  })
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd frontend/web && bunx vitest run src/components/ui/RadioGroup.spec.ts`
Expected: FAIL — cannot resolve `./RadioGroup.vue`.

- [ ] **Step 3: Create the primitive**

Create `src/components/ui/RadioGroup.vue`:

```vue
<template>
  <RadioGroupRoot
    v-model="model"
    :disabled="disabled"
    class="space-y-1"
  >
    <label
      v-for="opt in options"
      :key="opt.value || '__any__'"
      class="flex items-center gap-2 text-sm text-white/70 hover:text-white cursor-pointer py-0.5"
    >
      <RadioGroupItem
        :value="opt.value"
        :disabled="disabled"
        class="h-4 w-4 shrink-0 rounded-full border border-input data-[state=checked]:border-primary transition-colors disabled:opacity-50 disabled:cursor-not-allowed flex items-center justify-center"
      >
        <RadioGroupIndicator class="h-2 w-2 rounded-full bg-primary" />
      </RadioGroupItem>
      <span>{{ opt.label }}</span>
    </label>
  </RadioGroupRoot>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { RadioGroupRoot, RadioGroupItem, RadioGroupIndicator } from 'reka-ui'

interface Option {
  value: string
  label: string
}

interface Props {
  modelValue: string
  options: Option[]
  disabled?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  disabled: false,
})

const emit = defineEmits<{
  'update:modelValue': [value: string]
}>()

const model = computed<string>({
  get: () => props.modelValue,
  set: (v) => emit('update:modelValue', v),
})
</script>
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd frontend/web && bunx vitest run src/components/ui/RadioGroup.spec.ts`
Expected: PASS. If the empty-string-value test fails (reka rejects `value=""`), STOP and report — the fallback is to map `''` to a sentinel inside the primitive; do not proceed silently.

- [ ] **Step 5: Export from the barrel**

In `src/components/ui/index.ts`, add after the `Popover` export line:

```ts
export { default as RadioGroup } from './RadioGroup.vue'
```

- [ ] **Step 6: Type-check**

Run: `cd frontend/web && bunx vue-tsc --noEmit`
Expected: no errors.

- [ ] **Step 7: Commit**

```bash
git add frontend/web/src/components/ui/RadioGroup.vue frontend/web/src/components/ui/RadioGroup.spec.ts frontend/web/src/components/ui/index.ts
git commit -m "feat(ui): add RadioGroup primitive (reka-backed, options prop)"
```

---

## Task 3: Migrate clean `v-model` text inputs

These are direct swaps: replace `<input ... />` with `<Input ... />` and add an import. All passthrough attrs (`placeholder`, `:disabled`, `required`, `aria-*`, `@input`, `@keyup`, `class`) ride `$attrs` unchanged. `v-model.number` consumers keep `.number` at the call site (the primitive emits a string; `.number` coerces on the component v-model boundary).

> Import pattern for every file below: add `import { Input } from '@/components/ui'` to the file's `<script setup>` import block (or extend an existing `from '@/components/ui'` import).

### 3a. `views/Anime.vue`

**Files:** Modify `frontend/web/src/views/Anime.vue`

- [ ] **Step 1: Swap the input**

Before:
```html
<input v-model="editShikimoriId" type="text" class="flex-1 bg-white/5 border border-white/10 rounded-lg px-3 py-2 text-white text-sm focus:outline-none focus:border-cyan-500" :placeholder="$t('anime.examplePlaceholder')" />
```
After:
```html
<Input v-model="editShikimoriId" type="text" class="flex-1" :placeholder="$t('anime.examplePlaceholder')" />
```

Add `import { Input } from '@/components/ui'` to the script imports (check it is not already imported).

- [ ] **Step 2: Type-check + lint**

Run: `cd frontend/web && bunx vue-tsc --noEmit && bash scripts/design-system-lint.sh`
Expected: no errors; lint PASS.

### 3b. `views/ProfileSetup.vue`

- [ ] **Step 1: Swap**

Before:
```html
<input v-model="publicId" type="text" placeholder="your-username" class="flex-1 bg-transparent py-3 pr-3 text-white placeholder-white/40 focus:outline-none" :disabled="saving" @keyup.enter="save" />
```
After:
```html
<Input v-model="publicId" type="text" placeholder="your-username" class="flex-1 bg-transparent border-0" :disabled="saving" @keyup.enter="save" />
```
(`border-0`/`bg-transparent` preserve the borderless inline look of the original, which sat inside a styled shell.)

Add the `Input` import.

### 3c. `views/Game.vue`

`Game.vue` already imports `Input` (verify the import line; extend if it imports other names).

- [ ] **Step 1: Swap (`v-model.number` preserved)**

Before:
```html
<input v-model.number="newRoom.maxPlayers" type="number" min="2" max="20" class="w-full bg-white/5 border border-white/10 rounded-xl px-4 py-3 text-white focus:outline-none focus:border-cyan-500" required />
```
After:
```html
<Input v-model.number="newRoom.maxPlayers" type="number" min="2" max="20" required />
```

### 3d. `views/admin/RawLibrary.vue` (3 inputs)

- [ ] **Step 1: Swap all three**

Search input — before/after:
```html
<input v-model="searchQuery" type="text" :placeholder="$t('player.adminLibrary.search.placeholder')" :aria-label="$t('player.adminLibrary.search.placeholder')" class="w-full px-3 py-2 bg-white/5 border border-white/10 rounded text-white placeholder-white/40 focus:outline-none focus:ring-2 focus:ring-cyan-400" />
```
```html
<Input v-model="searchQuery" type="text" :placeholder="$t('player.adminLibrary.search.placeholder')" :aria-label="$t('player.adminLibrary.search.placeholder')" />
```

MAL ID — before/after:
```html
<input v-model.number="searchMalId" type="number" placeholder="MAL ID" aria-label="MAL ID" class="w-full px-3 py-2 bg-white/5 border border-white/10 rounded text-white placeholder-white/40 focus:outline-none focus:ring-2 focus:ring-cyan-400" />
```
```html
<Input v-model.number="searchMalId" type="number" placeholder="MAL ID" aria-label="MAL ID" />
```

Pending-link search — before/after (note: keeps `warning` focus ring via explicit class passthrough):
```html
<input v-model="pendingLinkSearchQueries[job.id]" type="text" :placeholder="$t('player.adminLibrary.jobs.pendingLink.searchPlaceholder')" :aria-label="$t('player.adminLibrary.jobs.pendingLink.searchPlaceholder')" class="w-full px-3 py-2 bg-white/5 border border-white/10 rounded text-white placeholder-white/40 text-sm focus:outline-none focus:ring-2 focus:ring-warning" @input="onPendingLinkInput(job.id)" />
```
```html
<Input v-model="pendingLinkSearchQueries[job.id]" type="text" size="sm" :placeholder="$t('player.adminLibrary.jobs.pendingLink.searchPlaceholder')" :aria-label="$t('player.adminLibrary.jobs.pendingLink.searchPlaceholder')" class="focus:ring-warning" @input="onPendingLinkInput(job.id)" />
```

Add the `Input` import.

### 3e. `views/admin/AdminCollectionEdit.vue` (6 text/url inputs — NOT sort_order, NOT published)

- [ ] **Step 1: Swap the 6 form fields**

Pattern (apply to `form.slug`, `form.title` [`required`], `form.title_ru`, `form.title_jp`):
```html
<input v-model="form.slug" type="text" class="w-full px-3 py-2 rounded bg-black/40 border border-white/10 text-white text-sm" :placeholder="$t('admin.collections.slugPlaceholder')" />
```
→
```html
<Input v-model="form.slug" type="text" class="bg-black/40" :placeholder="$t('admin.collections.slugPlaceholder')" />
```

`cover_image_url` (url):
```html
<Input v-model="form.cover_image_url" type="url" class="bg-black/40" placeholder="https://…" />
```

Item search:
```html
<Input v-model="searchQuery" type="text" class="bg-black/40" :placeholder="$t('admin.collections.itemSearchPlaceholder')" @input="onSearch" />
```

Keep `required` on the title field. Add the `Input` import. (The `form.published` checkbox and `sort_order` number are handled in Tasks 5 & 6.)

### 3f. `views/Profile.vue` — 4 clean text inputs

- [ ] **Step 1: Swap the 4 text inputs**

`searchQuery`:
```html
<Input v-model="searchQuery" type="text" :placeholder="$t('profile.watchlist.searchPlaceholder')" class="flex-shrink-0 w-48 rounded-full mr-auto" />
```
`malUsername`:
```html
<Input v-model="malUsername" type="text" :placeholder="$t('profile.import.malPlaceholder')" class="flex-1 bg-white/10" :disabled="malSync.importing" />
```
`shikimoriNickname`:
```html
<Input v-model="shikimoriNickname" type="text" :placeholder="$t('profile.import.shikimoriPlaceholder')" class="flex-1 bg-white/10" :disabled="shikimoriSync.importing" />
```
`publicId`:
```html
<Input v-model="publicId" type="text" placeholder="your-username" class="flex-1 bg-transparent border-0" :disabled="savingPublicId" />
```

Add `Input` to the existing `@/components/ui` import if present, else add the import.

- [ ] **Step 2: Type-check + lint + commit (Task 3 whole)**

Run: `cd frontend/web && bunx vue-tsc --noEmit && bash scripts/design-system-lint.sh`
Expected: no errors; lint PASS.

```bash
git add frontend/web/src/views/Anime.vue frontend/web/src/views/ProfileSetup.vue frontend/web/src/views/Game.vue frontend/web/src/views/admin/RawLibrary.vue frontend/web/src/views/admin/AdminCollectionEdit.vue frontend/web/src/views/Profile.vue
git commit -m "refactor(ui): migrate clean v-model text inputs to <Input>"
```

---

## Task 4: Migrate ref-focus search inputs

`Input` now exposes `focus()`. Three sites call `.focus()` on the input ref; after the swap the ref points at the `Input` component instance, whose `focus()` is exposed.

### 4a. `components/ui/GenreFilterPopup.vue` (no icon — plain swap)

**Files:** Modify `frontend/web/src/components/ui/GenreFilterPopup.vue`

- [ ] **Step 1: Swap the input**

Before:
```html
<input
  ref="searchRef"
  v-model="search"
  type="text"
  :placeholder="searchPlaceholder"
  class="w-full px-3 py-1.5 text-sm bg-white/5 border border-white/10 rounded-lg text-white placeholder-white/30 focus:outline-none focus:border-cyan-500/30"
  @keydown="handleSearchKeydown"
/>
```
After:
```html
<Input
  ref="searchRef"
  v-model="search"
  type="text"
  size="sm"
  :placeholder="searchPlaceholder"
  @keydown="handleSearchKeydown"
/>
```

- [ ] **Step 2: Update the ref type and the focus call site**

Find `const searchRef = ref<HTMLInputElement | null>(null)` and any `searchRef.value?.focus()` / `searchRef.value.focus()`. Change the type to the component-instance shape (it only needs `focus`):

```ts
import { Input } from '@/components/ui'
// ...
const searchRef = ref<{ focus: () => void } | null>(null)
```

The existing `searchRef.value?.focus()` call works unchanged (the exposed `focus()` is called). If the code reads other input DOM props off `searchRef` (e.g. `.value`, `.select()`), STOP and report — those are not exposed.

- [ ] **Step 3: Type-check**

Run: `cd frontend/web && bunx vue-tsc --noEmit`
Expected: no errors.

### 4b. `views/admin/AdminRecsPicker.vue` (plain swap)

- [ ] **Step 1: Swap**

Before:
```html
<input id="rec-user-id" ref="searchInputRef" v-model="input" type="text" :placeholder="$t('admin.recs.pickerPlaceholder')" autocomplete="off" spellcheck="false" :aria-busy="isSubmitting" class="w-full px-3 py-2 pr-9 rounded-lg bg-white/10 border border-white/20 text-white font-mono text-sm placeholder-white/30 focus:outline-none focus:border-cyan-400/60 focus-visible:ring-2 focus-visible:ring-cyan-400" required />
```
After:
```html
<Input id="rec-user-id" ref="searchInputRef" v-model="input" type="text" :placeholder="$t('admin.recs.pickerPlaceholder')" autocomplete="off" spellcheck="false" :aria-busy="isSubmitting" class="bg-white/10 font-mono pr-9" required />
```

- [ ] **Step 2: Update ref type**

Change `const searchInputRef = ref<HTMLInputElement | null>(null)` to `ref<{ focus: () => void } | null>(null)`. Verify the only usage is `.focus()`; if it reads `.value`/`.select()`, STOP and report. Add the `Input` import.

### 4c. `components/layout/Navbar.vue` (HIGHEST RISK — icon → prefix slot, dropdown reparent)

The native input shares a `<div class="relative">` with an absolutely-positioned autocomplete dropdown and a magnifier icon. The migration: give the wrapper a fixed `w-64`, move the icon into `Input`'s `prefix` slot, keep the dropdown as a sibling.

- [ ] **Step 1: Restructure the search block**

Before (the `<div class="relative">` … input … icon svg):
```html
<div class="relative">
  <input
    ref="searchInputRef"
    v-model="searchQuery"
    type="text"
    :placeholder="$t('search.placeholder')"
    class="w-64 px-3 py-1.5 pl-9 bg-white/10 border border-white/20 rounded-lg text-white text-sm placeholder-white/40 focus:outline-none focus:border-cyan-400/50 focus:bg-white/15 transition-all"
    @input="onSearchInput"
    @keydown.enter="goToSearch"
    @keydown.escape="closeSearch"
    @keydown.down.prevent="highlightNext"
    @keydown.up.prevent="highlightPrev"
  />
  <svg class="absolute left-2.5 top-1/2 -translate-y-1/2 w-4 h-4 text-white/60" fill="none" stroke="currentColor" viewBox="0 0 24 24">
    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
  </svg>
  <!-- Autocomplete Dropdown -->
  <Transition name="dropdown">
    ...
  </Transition>
</div>
```
After (icon → `#prefix`, wrapper `w-64`, dropdown unchanged):
```html
<div class="relative w-64">
  <Input
    ref="searchInputRef"
    v-model="searchQuery"
    type="text"
    size="sm"
    :placeholder="$t('search.placeholder')"
    class="bg-white/10 border-white/20 text-sm"
    @input="onSearchInput"
    @keydown.enter="goToSearch"
    @keydown.escape="closeSearch"
    @keydown.down.prevent="highlightNext"
    @keydown.up.prevent="highlightPrev"
  >
    <template #prefix>
      <svg class="w-4 h-4 text-white/60" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
      </svg>
    </template>
  </Input>
  <!-- Autocomplete Dropdown (unchanged) -->
  <Transition name="dropdown">
    ...
  </Transition>
</div>
```

Leave the `<Transition>` dropdown block exactly as-is. Add the `Input` import.

- [ ] **Step 2: Update the ref type + focus call**

`const searchInputRef = ref<HTMLInputElement | null>(null)` → `ref<{ focus: () => void } | null>(null)`. The existing `searchInputRef.value?.focus()` (line ~428) works via the exposed method.

- [ ] **Step 3: Type-check + lint**

Run: `cd frontend/web && bunx vue-tsc --noEmit && bash scripts/design-system-lint.sh`
Expected: no errors; lint PASS.

- [ ] **Step 4: Commit (Task 4 whole)**

```bash
git add frontend/web/src/components/ui/GenreFilterPopup.vue frontend/web/src/views/admin/AdminRecsPicker.vue frontend/web/src/components/layout/Navbar.vue
git commit -m "refactor(ui): migrate ref-focus search inputs to <Input> (focus() + prefix slot)"
```

> In-browser smoke for Navbar/GenreFilterPopup is in Task 8 — do not skip it; this is the riskiest visual change.

---

## Task 5: Migrate controlled inline-edit number/date inputs

These commit **on blur/change** via `event.target.valueAsNumber` / `.value`. Bind **`:model-value`** (string) — NOT `:value`, which the primitive's internal `v-model` overrides — and keep the `@blur`/`@change` listener (Vue merges it onto the inner input; `event.target` is the native input). Type/min/max pass through `$attrs` and stay in the DOM (Profile's focus `querySelector('input[type="number"][min="0"][max="10"]')` keeps matching).

### 5a. `views/Profile.vue` — score (×2), episodes, dates (×2), skipIntroSec

- [ ] **Step 1: Swap the inline score editors**

First score editor (table, `editingScore`) — before:
```html
<input
  v-if="editingScore === anime.anime_id"
  type="number"
  min="0" max="10"
  :value="anime.score || 0"
  @blur="(e) => { finishEditScore(anime.anime_id, (e.target as HTMLInputElement).value); }"
  ...
  @keydown.escape="editingScore = null"
  class="..."
/>
```
After (keep `v-if`, all bindings; `:value` → `:model-value` with String()):
```html
<Input
  v-if="editingScore === anime.anime_id"
  type="number"
  min="0" max="10"
  :model-value="String(anime.score || 0)"
  @blur="(e) => { finishEditScore(anime.anime_id, (e.target as HTMLInputElement).value); }"
  @keydown.escape="editingScore = null"
  class="w-20"
/>
```
(Preserve any other attrs/handlers present on the original element verbatim; only `:value`→`:model-value` and the tag name change.)

Grid score editor (`editingScoreGrid`) — same transform:
```html
<Input
  v-if="isOwnProfile && editingScoreGrid === anime.anime_id"
  type="number"
  min="0" max="10"
  :model-value="String(anime.score || 0)"
  @blur="(e) => { finishEditScore(anime.anime_id, (e.target as HTMLInputElement).value); editingScoreGrid = null; }"
  @keydown.escape="editingScoreGrid = null"
  class="w-20"
/>
```

- [ ] **Step 2: Swap the episodes editor**

Before `:value="anime.episodes || 0"` → after:
```html
<Input
  type="number"
  :model-value="String(anime.episodes || 0)"
  min="0" :max="animeTotalEpisodes(anime) || 9999"
  @blur="(e) => updateAnimeEpisodes(anime.anime_id, parseInt((e.target as HTMLInputElement).value) || 0)"
  class="w-20"
/>
```

- [ ] **Step 3: Swap the two date editors**

`started_at`:
```html
<Input
  v-if="isOwnProfile"
  type="date"
  :model-value="formatDateForInput(anime.started_at)"
  @change="(e) => updateAnimeDate(anime.anime_id, 'started_at', (e.target as HTMLInputElement).value)"
  size="sm"
  class="w-full text-xs"
/>
```
`completed_at`:
```html
<Input
  v-if="isOwnProfile"
  type="date"
  :model-value="formatDateForInput(anime.completed_at)"
  @change="(e) => updateAnimeDate(anime.anime_id, 'completed_at', (e.target as HTMLInputElement).value)"
  size="sm"
  class="w-full text-xs"
/>
```

- [ ] **Step 4: Swap the skip-intro seconds editor**

Before `:value="skipIntroSec"` → after:
```html
<Input
  id="skip-intro-dismiss-sec"
  type="number"
  :min="skipIntroMin" :max="skipIntroMax" step="1"
  :model-value="String(skipIntroSec)"
  @change="onSkipIntroSecChange"
  class="w-32 bg-white/10"
/>
```

- [ ] **Step 5: Verify the editingScore focus watcher still matches**

Confirm the `watch(editingScore, ...)` block still does `document.querySelector('input[type="number"][min="0"][max="10"]')`. Since `Input` renders `<input type="number" min="0" max="10">` (attrs passthrough), no change is needed. Do NOT alter the watcher.

### 5b. `views/admin/AdminCollectionEdit.vue` — sort_order

- [ ] **Step 1: Swap**

Before:
```html
<input :value="item.sort_order" type="number" min="0" class="w-16 px-2 py-1 rounded bg-black/40 border border-white/10 text-white text-sm text-right" @change="(e) => ...">
```
After (keep the exact `@change` handler body verbatim):
```html
<Input :model-value="String(item.sort_order)" type="number" min="0" size="sm" class="w-16 bg-black/40 text-right" @change="(e) => ...">
```

### 5c. `components/browse/BrowseSidebar.vue` — year from/to (side-by-side; needs wrapper divs)

`Input`'s wrapper is hardcoded `w-full`; two side-by-side inputs need `w-1/2` containers.

- [ ] **Step 1: Swap the year row**

Before:
```html
<div class="flex items-center gap-2">
  <input type="number" :min="MIN_YEAR" :max="MAX_YEAR" :value="filters.yearFrom.value ?? ''" :placeholder="$t('browse.filters.year.from')" :aria-label="$t('browse.filters.year.from')" class="w-1/2 px-2 py-1 text-sm bg-white/5 border border-white/10 rounded-md text-white focus:ring-2 focus:ring-cyan-500/40 focus:outline-none" @change="onYearChange('from', ($event.target as HTMLInputElement).valueAsNumber)" />
  <span class="text-white/40">—</span>
  <input type="number" :min="MIN_YEAR" :max="MAX_YEAR" :value="filters.yearTo.value ?? ''" :placeholder="$t('browse.filters.year.to')" :aria-label="$t('browse.filters.year.to')" class="w-1/2 px-2 py-1 text-sm bg-white/5 border border-white/10 rounded-md text-white focus:ring-2 focus:ring-cyan-500/40 focus:outline-none" @change="onYearChange('to', ($event.target as HTMLInputElement).valueAsNumber)" />
</div>
```
After:
```html
<div class="flex items-center gap-2">
  <div class="w-1/2">
    <Input type="number" size="sm" :min="MIN_YEAR" :max="MAX_YEAR" :model-value="filters.yearFrom.value != null ? String(filters.yearFrom.value) : ''" :placeholder="$t('browse.filters.year.from')" :aria-label="$t('browse.filters.year.from')" @change="onYearChange('from', ($event.target as HTMLInputElement).valueAsNumber)" />
  </div>
  <span class="text-white/40">—</span>
  <div class="w-1/2">
    <Input type="number" size="sm" :min="MIN_YEAR" :max="MAX_YEAR" :model-value="filters.yearTo.value != null ? String(filters.yearTo.value) : ''" :placeholder="$t('browse.filters.year.to')" :aria-label="$t('browse.filters.year.to')" @change="onYearChange('to', ($event.target as HTMLInputElement).valueAsNumber)" />
  </div>
</div>
```

Add `Input` to the imports in BrowseSidebar (the `@/components/ui` import will also gain `Checkbox` and `RadioGroup` in Tasks 6 & 7 — one combined import line is fine).

- [ ] **Step 2: Type-check + lint + commit (Task 5 whole)**

Run: `cd frontend/web && bunx vue-tsc --noEmit && bash scripts/design-system-lint.sh`
Expected: no errors; lint PASS.

```bash
git add frontend/web/src/views/Profile.vue frontend/web/src/views/admin/AdminCollectionEdit.vue frontend/web/src/components/browse/BrowseSidebar.vue
git commit -m "refactor(ui): migrate controlled inline-edit number/date inputs to <Input>"
```

---

## Task 6: Migrate plain checkboxes to `<Checkbox>`

`Checkbox` is icon-only (`boolean | 'indeterminate'` v-model, no label). Wrap in the existing `<label>`. Multi-select array checkboxes bridge via `:model-value="arr.includes(x)"` + `@update:model-value="toggle(...)"` (handlers already take a boolean; the checkboxes are never indeterminate).

### 6a. `components/browse/BrowseSidebar.vue` — genre checkbox

- [ ] **Step 1: Swap**

Before:
```html
<input
  type="checkbox"
  :value="g.id"
  :checked="filters.genres.value.includes(g.id)"
  class="rounded border-white/20 bg-transparent text-cyan-500 focus:ring-cyan-500"
  @change="onGenreToggle(g.id, ($event.target as HTMLInputElement).checked)"
/>
```
After:
```html
<Checkbox
  :model-value="filters.genres.value.includes(g.id)"
  @update:model-value="(v) => onGenreToggle(g.id, v === true)"
/>
```

### 6b. `views/Profile.vue` — publicStatuses checkbox

- [ ] **Step 1: Swap**

Before:
```html
<input type="checkbox" :checked="publicStatuses.includes(status.value)" @change="togglePublicStatus(status.value)" class="w-4 h-4 rounded border-white/20 bg-white/10 text-cyan-500 focus:ring-cyan-500 focus:ring-offset-0" />
```
After (`togglePublicStatus` ignores its arg — it toggles by membership):
```html
<Checkbox :model-value="publicStatuses.includes(status.value)" @update:model-value="() => togglePublicStatus(status.value)" />
```

Add `Checkbox` to the Profile `@/components/ui` import.

### 6c. `views/admin/AdminCollectionEdit.vue` — published (clean boolean)

- [ ] **Step 1: Swap**

Before:
```html
<input v-model="form.published" type="checkbox" class="mr-2" />
```
After:
```html
<Checkbox v-model="form.published" class="mr-2" />
```

Add `Checkbox` to the import.

- [ ] **Step 2: Type-check + lint + commit (Task 6 whole)**

Run: `cd frontend/web && bunx vue-tsc --noEmit && bash scripts/design-system-lint.sh`
Expected: no errors; lint PASS. (If `form.published` is typed as anything but `boolean`, the `Checkbox` v-model — `boolean | 'indeterminate'` — may widen it; if vue-tsc complains, coerce with `:model-value="!!form.published"` + `@update:model-value="v => form.published = v === true"`.)

```bash
git add frontend/web/src/components/browse/BrowseSidebar.vue frontend/web/src/views/Profile.vue frontend/web/src/views/admin/AdminCollectionEdit.vue
git commit -m "refactor(ui): migrate plain checkboxes to <Checkbox>"
```

---

## Task 7: Migrate radio sets to `<RadioGroup>`

Each set is a `FilterSection` wrapping a `v-for` of `<label><input type=radio></label>`. Collapse to one `<RadioGroup v-model :options>`. The `options` computeds (`kindOptions`, `statusOptions`, `sortOptions`) already have `{ value, label }` shape and stay as-is.

**Files:** Modify `frontend/web/src/components/browse/BrowseSidebar.vue`

- [ ] **Step 1: Replace the Format (kind) radio block**

Before (the `<label v-for="opt in kindOptions">…</label>` inside the Format `FilterSection`):
```html
<label v-for="opt in kindOptions" :key="opt.value || 'any-kind'" class="flex items-center gap-2 text-sm text-white/70 hover:text-white cursor-pointer py-0.5">
  <input type="radio" name="kind-filter" :value="opt.value" :checked="filters.kind.value === opt.value" class="border-white/20 bg-transparent text-cyan-500 focus:ring-cyan-500" @change="onKindChange(opt.value)" />
  <span>{{ opt.label }}</span>
</label>
```
After (single primitive — the model is bound one-way + explicit handler to preserve `writeUrl()` side-effect):
```html
<RadioGroup :model-value="filters.kind.value" :options="kindOptions" @update:model-value="(v) => onKindChange(v as Kind)" />
```

- [ ] **Step 2: Replace the Status radio block**

```html
<RadioGroup :model-value="filters.status.value" :options="statusOptions" @update:model-value="onStatusChange" />
```

- [ ] **Step 3: Replace the Sort radio block**

```html
<RadioGroup :model-value="filters.sort.value" :options="sortOptions" @update:model-value="(v) => onSortChange(v as Sort)" />
```

- [ ] **Step 4: Add the import**

Ensure BrowseSidebar's `@/components/ui` import includes `RadioGroup` (and `Input`, `Checkbox` from earlier tasks), e.g.:
```ts
import { Input, Checkbox, RadioGroup } from '@/components/ui'
```
`Kind` and `Sort` are already imported from `@/composables/useBrowseFilters`.

- [ ] **Step 5: Type-check + lint + commit**

Run: `cd frontend/web && bunx vue-tsc --noEmit && bash scripts/design-system-lint.sh`
Expected: no errors; lint PASS.

```bash
git add frontend/web/src/components/browse/BrowseSidebar.vue
git commit -m "refactor(ui): migrate BrowseSidebar radio sets to <RadioGroup>"
```

---

## Task 8: Document bespoke-keeps + full gates + in-browser smoke

- [ ] **Step 1: Annotate the four bespoke-keeps**

Add a one-line comment immediately above each kept-native control so future audits know it is intentional:

`components/browse/BrowseSidebar.vue` — above the provider checkbox `<input>`:
```html
<!-- bespoke-keep: per-provider brand accent (opt.accent); cyan-only Checkbox primitive can't model it -->
```
`components/browse/BrowseSidebar.vue` — above the score `<input type="range">`:
```html
<!-- bespoke-keep: range slider; no slider primitive in the design system -->
```
`components/layout/FeedbackButton.vue` — above the `<input type="radio" ... class="sr-only">`:
```html
<!-- bespoke-keep: sr-only radio inside a custom card selector (segmented-toggle pattern) -->
```
`views/Profile.vue` — above the `<input type="file">`:
```html
<!-- bespoke-keep: file upload; out of Input-primitive scope -->
```

- [ ] **Step 2: Run the full gate suite**

Run from `frontend/web/`:
```bash
bunx vitest run
bunx vue-tsc --noEmit
bash scripts/design-system-lint.sh
bun run build
```
Expected: vitest all pass (note any pre-existing failure such as `AnimeContextMenu.spec.ts` separately — it is unrelated); vue-tsc 0 errors; lint PASS (0/0/0); build succeeds.

- [ ] **Step 3: Prove the lint fail-path is intact**

Run: `cd frontend/web && bash scripts/design-system-lint.sh --selftest`
Expected: `SELFTEST PASS`, tree clean.

- [ ] **Step 4: Commit the annotations**

```bash
git add frontend/web/src/components/browse/BrowseSidebar.vue frontend/web/src/components/layout/FeedbackButton.vue frontend/web/src/views/Profile.vue
git commit -m "docs(ui): annotate bespoke-keep native controls (provider/range/radio-card/file)"
```

- [ ] **Step 5: In-browser smoke (DS-NF-06 — desktop + mobile; jsdom cannot catch Tailwind v4 cascade)**

Deploy locally if needed (`make redeploy-web`) and verify each surface at a desktop and a mobile viewport. Confirm NO visual or behavioral diff vs. before:

1. **Browse filters** (`BrowseSidebar`): genre checkboxes toggle + update results; kind/status/sort radios single-select; year from/to commit on change and clamp (from ≤ to); provider checkboxes still show per-provider accent; score range slider still works.
2. **Profile watchlist** (own profile): inline score edit commits on blur and on Enter→blur; episodes edit commits; started/completed date pickers commit on change; the score input auto-focuses when entering edit mode (querySelector watcher).
3. **Profile settings**: MAL/Shikimori import fields type + submit; public-id field; public-status checkboxes toggle and persist; avatar file picker opens.
4. **Admin**: AdminCollectionEdit form fields + published checkbox + sort_order; AdminRecsPicker focuses on open and submits; RawLibrary search + MAL-ID + pending-link search.
5. **Navbar search** (HIGH RISK): opens and auto-focuses; magnifier icon renders in the prefix slot; typing shows the autocomplete dropdown positioned correctly; ArrowUp/Down highlight, Enter navigates, Escape closes; fixed width unchanged.
6. **GenreFilterPopup**: opens, search input auto-focuses, keyboard nav works.
7. **Game**: create-room max-players number input.

- [ ] **Step 6: Run `/animeenigma-after-update`**

One batched run at the very end (per the batching rule — no changelog spam across tasks): lints/builds, redeploys `web`, health-checks, writes ONE Russian Trump-mode changelog entry covering the primitive migration, commits, and pushes.

---

## Self-Review notes (for the executor)

- **Highest-risk tasks:** Task 4c (Navbar — layout reparent + custom focus classes) and Task 5 (Profile inline-edit commit semantics + focus watcher). Smoke-test these explicitly.
- **Stop-and-report triggers:** a ref site reading input DOM props other than `focus()` (Task 4); reka rejecting `value=""` in RadioGroup (Task 2 Step 4); `form.published` typing widening under Checkbox v-model (Task 6c).
- **Never** use `:value` on `<Input>` for controlled inputs — always `:model-value`. The primitive's internal `v-model` overrides a passed `value`.
- **Bun only:** `bunx` / `bun run`, never npm/npx.
