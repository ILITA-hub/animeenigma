# Subtitle Timing Settings Menu — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a gear-button subtitle-timing settings menu to the Raw (JP) and OurEnglish (EN) players so users can nudge subtitle cues earlier/later, persisted across sessions.

**Architecture:** A module-singleton composable (`useSubtitleTimingOffset`) owns a reactive offset value backed by `localStorage`. A shared `SubtitleSettingsMenu.vue` (gear + popover) reads/writes that value. Both players mount the menu in their existing toolbar and feed `offset` into their already-`offset`-aware `SubtitleOverlay`.

**Tech Stack:** Vue 3 `<script setup>` + TypeScript, `@vueuse/core` (`onClickOutside`, `onKeyStroke`), vue-i18n, Vitest + `@vue/test-utils`, Tailwind. Frontend-only — no backend changes.

**Reference spec:** `docs/superpowers/specs/2026-06-02-subtitle-timing-settings-menu-design.md`

---

## File Structure

- **Create** `frontend/web/src/composables/useSubtitleTimingOffset.ts` — singleton reactive offset + persistence (Task 1)
- **Create** `frontend/web/src/composables/__tests__/useSubtitleTimingOffset.spec.ts` — composable unit tests (Task 1)
  - (If `src/composables/__tests__/` doesn't exist, co-locate as `frontend/web/src/composables/useSubtitleTimingOffset.spec.ts` instead — check first.)
- **Create** `frontend/web/src/components/player/SubtitleSettingsMenu.vue` — gear + popover UI (Task 3)
- **Create** `frontend/web/src/components/player/SubtitleSettingsMenu.spec.ts` — component tests (Task 3)
- **Modify** `frontend/web/src/locales/en.json` + `ru.json` — `player.subtitleSettings.*` keys (Task 2)
- **Modify** `frontend/web/src/components/player/RawPlayer.vue` — mount menu + pass `:offset` (Task 4)
- **Modify** `frontend/web/src/components/player/OurEnglishPlayer.vue` — mount menu + pass `:offset` (Task 5)

> **Working directory for all commands:** `frontend/web`. Use `bun`/`bunx`, never npm.

---

### Task 1: Timing-offset composable

**Files:**
- Create: `frontend/web/src/composables/useSubtitleTimingOffset.ts`
- Test: `frontend/web/src/composables/__tests__/useSubtitleTimingOffset.spec.ts` (or co-located `.spec.ts` — see File Structure note)

- [ ] **Step 1: Write the failing test**

Create the test file:

```ts
import { describe, it, expect, beforeEach, vi } from 'vitest'

describe('useSubtitleTimingOffset', () => {
  beforeEach(() => {
    localStorage.clear()
    vi.resetModules() // fresh module-singleton per test so load() re-runs
  })

  it('defaults to 0 when nothing is stored', async () => {
    const { useSubtitleTimingOffset } = await import('../useSubtitleTimingOffset')
    expect(useSubtitleTimingOffset().offset.value).toBe(0)
  })

  it('loads an existing stored value on init', async () => {
    localStorage.setItem('subtitle_timing_offset', '2.5')
    const { useSubtitleTimingOffset } = await import('../useSubtitleTimingOffset')
    expect(useSubtitleTimingOffset().offset.value).toBe(2.5)
  })

  it('clamps an out-of-range stored value to MAX', async () => {
    localStorage.setItem('subtitle_timing_offset', '999')
    const { useSubtitleTimingOffset } = await import('../useSubtitleTimingOffset')
    expect(useSubtitleTimingOffset().offset.value).toBe(30)
  })

  it('falls back to 0 on a corrupt stored value', async () => {
    localStorage.setItem('subtitle_timing_offset', 'not-a-number')
    const { useSubtitleTimingOffset } = await import('../useSubtitleTimingOffset')
    expect(useSubtitleTimingOffset().offset.value).toBe(0)
  })

  it('nudge accumulates and rounds to 1 decimal', async () => {
    const { useSubtitleTimingOffset } = await import('../useSubtitleTimingOffset')
    const { offset, nudge } = useSubtitleTimingOffset()
    nudge(0.1); nudge(0.1); nudge(0.1)
    expect(offset.value).toBe(0.3) // not 0.30000000000000004
  })

  it('clamps nudge to MAX', async () => {
    const { useSubtitleTimingOffset } = await import('../useSubtitleTimingOffset')
    const { offset, nudge } = useSubtitleTimingOffset()
    nudge(100)
    expect(offset.value).toBe(30)
  })

  it('reset returns to 0', async () => {
    const { useSubtitleTimingOffset } = await import('../useSubtitleTimingOffset')
    const { offset, nudge, reset } = useSubtitleTimingOffset()
    nudge(1)
    reset()
    expect(offset.value).toBe(0)
  })

  it('writes through to localStorage synchronously on change', async () => {
    const { useSubtitleTimingOffset } = await import('../useSubtitleTimingOffset')
    useSubtitleTimingOffset().nudge(1)
    expect(localStorage.getItem('subtitle_timing_offset')).toBe('1')
  })
})
```

> If you co-locate the spec next to the source (no `__tests__` dir), change the import paths from `'../useSubtitleTimingOffset'` to `'./useSubtitleTimingOffset'`.

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd frontend/web && bunx vitest run src/composables` (adjust path if co-located)
Expected: FAIL — `Cannot find module '../useSubtitleTimingOffset'`.

- [ ] **Step 3: Write the composable**

Create `frontend/web/src/composables/useSubtitleTimingOffset.ts`:

```ts
import { ref, watch } from 'vue'

const STORAGE_KEY = 'subtitle_timing_offset'
const MIN = -30
const MAX = 30

function clamp(v: number): number {
  return Math.min(MAX, Math.max(MIN, v))
}

function round1(v: number): number {
  return Math.round(v * 10) / 10
}

function load(): number {
  try {
    const stored = localStorage.getItem(STORAGE_KEY)
    if (stored === null) return 0
    const raw = Number(stored)
    return Number.isFinite(raw) ? clamp(round1(raw)) : 0
  } catch {
    return 0
  }
}

// Module-level singleton: every caller of useSubtitleTimingOffset() shares this
// reactive ref, so the menu and the player overlays stay in sync.
const offset = ref<number>(load())

// flush: 'sync' makes persistence deterministic (tests + no lost writes on unmount).
watch(
  offset,
  (v) => {
    try {
      localStorage.setItem(STORAGE_KEY, String(v))
    } catch {
      /* ignore quota / storage-disabled */
    }
  },
  { flush: 'sync' },
)

export function useSubtitleTimingOffset() {
  function nudge(delta: number): void {
    offset.value = clamp(round1(offset.value + delta))
  }
  function reset(): void {
    offset.value = 0
  }
  return { offset, nudge, reset }
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd frontend/web && bunx vitest run src/composables`
Expected: PASS — all 8 tests green.

- [ ] **Step 5: Commit**

```bash
git add frontend/web/src/composables/useSubtitleTimingOffset.ts frontend/web/src/composables/**/useSubtitleTimingOffset.spec.ts
git commit -m "feat(player): subtitle timing offset composable (localStorage-backed)

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task 2: i18n keys (en + ru)

**Files:**
- Modify: `frontend/web/src/locales/en.json` (after the `subtitlePicker` block, ~line 243)
- Modify: `frontend/web/src/locales/ru.json` (after the `subtitlePicker` block, ~line 243)

- [ ] **Step 1: Add the English keys**

In `frontend/web/src/locales/en.json`, find:

```json
    "subtitlePicker": {
      "label": "Subtitles",
      "none": "Off"
    },
    "otherSubs": {
```

Replace it with (insert the new block between):

```json
    "subtitlePicker": {
      "label": "Subtitles",
      "none": "Off"
    },
    "subtitleSettings": {
      "label": "Subtitle timing",
      "title": "Subtitle timing",
      "offsetHint": "Shift subtitles earlier or later to match the audio.",
      "reset": "Reset"
    },
    "otherSubs": {
```

- [ ] **Step 2: Add the Russian keys**

In `frontend/web/src/locales/ru.json`, find:

```json
    "subtitlePicker": {
      "label": "Субтитры",
      "none": "Выключить"
    },
    "otherSubs": {
```

Replace it with:

```json
    "subtitlePicker": {
      "label": "Субтитры",
      "none": "Выключить"
    },
    "subtitleSettings": {
      "label": "Тайминг субтитров",
      "title": "Тайминг субтитров",
      "offsetHint": "Сдвиньте субтитры раньше или позже, чтобы попасть в звук.",
      "reset": "Сброс"
    },
    "otherSubs": {
```

- [ ] **Step 3: Verify both files parse and contain the keys**

Run:
```bash
cd frontend/web && node -e "for (const l of ['en','ru']) { const j = require('./src/locales/'+l+'.json'); const k = j.player.subtitleSettings; if (!k || !k.label || !k.title || !k.offsetHint || !k.reset) throw new Error(l+' missing subtitleSettings keys'); console.log(l, 'OK'); }"
```
Expected: `en OK` then `ru OK` (no thrown error). A parse error or missing key throws non-zero.

- [ ] **Step 4: Commit**

```bash
git add frontend/web/src/locales/en.json frontend/web/src/locales/ru.json
git commit -m "i18n(player): subtitle timing settings strings (en + ru)

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task 3: `SubtitleSettingsMenu.vue` component

**Files:**
- Create: `frontend/web/src/components/player/SubtitleSettingsMenu.vue`
- Test: `frontend/web/src/components/player/SubtitleSettingsMenu.spec.ts`

- [ ] **Step 1: Write the failing test**

Create `frontend/web/src/components/player/SubtitleSettingsMenu.spec.ts`:

```ts
import { describe, it, expect, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import SubtitleSettingsMenu from './SubtitleSettingsMenu.vue'
import { useSubtitleTimingOffset } from '@/composables/useSubtitleTimingOffset'

const mountMenu = (hasActiveSub = true) =>
  mount(SubtitleSettingsMenu, {
    props: { hasActiveSub },
    global: { mocks: { $t: (k: string) => k } },
  })

beforeEach(() => {
  localStorage.clear()
  // Normalize the shared singleton between tests.
  useSubtitleTimingOffset().reset()
})

describe('SubtitleSettingsMenu', () => {
  it('disables the gear when there is no active subtitle', () => {
    const w = mountMenu(false)
    expect(w.get('[data-test="sub-timing-gear"]').attributes('disabled')).toBeDefined()
  })

  it('opens the popover and nudges the offset later (+0.1s)', async () => {
    const w = mountMenu(true)
    await w.get('[data-test="sub-timing-gear"]').trigger('click')
    await w.get('[data-test="nudge-plus-01"]').trigger('click')
    expect(w.get('[data-test="readout"]').text()).toBe('+0.1s')
  })

  it('nudges the offset earlier (−1s) and shows a negative readout', async () => {
    const w = mountMenu(true)
    await w.get('[data-test="sub-timing-gear"]').trigger('click')
    await w.get('[data-test="nudge-minus-1"]').trigger('click')
    expect(w.get('[data-test="readout"]').text()).toBe('-1.0s')
  })

  it('reset returns the readout to 0.0s', async () => {
    const w = mountMenu(true)
    await w.get('[data-test="sub-timing-gear"]').trigger('click')
    await w.get('[data-test="nudge-plus-1"]').trigger('click')
    expect(w.get('[data-test="readout"]').text()).toBe('+1.0s')
    await w.get('[data-test="reset"]').trigger('click')
    expect(w.get('[data-test="readout"]').text()).toBe('0.0s')
  })

  it('persists the offset to localStorage', async () => {
    const w = mountMenu(true)
    await w.get('[data-test="sub-timing-gear"]').trigger('click')
    await w.get('[data-test="nudge-plus-01"]').trigger('click')
    expect(localStorage.getItem('subtitle_timing_offset')).toBe('0.1')
  })
})
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd frontend/web && bunx vitest run src/components/player/SubtitleSettingsMenu.spec.ts`
Expected: FAIL — `Failed to resolve import "./SubtitleSettingsMenu.vue"`.

- [ ] **Step 3: Write the component**

Create `frontend/web/src/components/player/SubtitleSettingsMenu.vue`:

```vue
<template>
  <div ref="rootRef" class="relative">
    <button
      type="button"
      data-test="sub-timing-gear"
      class="inline-flex items-center gap-2 px-4 py-2 rounded-md text-sm font-medium bg-white/10 hover:bg-white/15 text-white border border-white/10 transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
      :disabled="!hasActiveSub"
      :title="$t('player.subtitleSettings.label')"
      :aria-label="$t('player.subtitleSettings.label')"
      @click="open = !open"
    >
      <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z" />
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
      </svg>
      {{ $t('player.subtitleSettings.label') }}
    </button>

    <div
      v-if="open"
      data-test="sub-timing-popover"
      class="absolute right-0 z-30 mt-2 w-72 rounded-lg border border-white/10 bg-zinc-900/95 p-3 shadow-xl backdrop-blur"
    >
      <p class="text-white/80 text-sm font-medium mb-1">{{ $t('player.subtitleSettings.title') }}</p>
      <p class="text-white/40 text-xs mb-3">{{ $t('player.subtitleSettings.offsetHint') }}</p>
      <div class="flex items-center justify-between gap-2">
        <button type="button" data-test="nudge-minus-1" class="px-2 py-1 rounded bg-white/10 hover:bg-white/20 text-white text-sm" @click="nudge(-1)">−1s</button>
        <button type="button" data-test="nudge-minus-01" class="px-2 py-1 rounded bg-white/10 hover:bg-white/20 text-white text-sm" @click="nudge(-0.1)">−0.1s</button>
        <span data-test="readout" class="min-w-[3.5rem] text-center text-cyan-200 text-sm font-semibold tabular-nums">{{ readout }}</span>
        <button type="button" data-test="nudge-plus-01" class="px-2 py-1 rounded bg-white/10 hover:bg-white/20 text-white text-sm" @click="nudge(0.1)">+0.1s</button>
        <button type="button" data-test="nudge-plus-1" class="px-2 py-1 rounded bg-white/10 hover:bg-white/20 text-white text-sm" @click="nudge(1)">+1s</button>
      </div>
      <button type="button" data-test="reset" class="mt-3 text-xs text-white/50 hover:text-white/80 underline" @click="reset()">
        {{ $t('player.subtitleSettings.reset') }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { onClickOutside, onKeyStroke } from '@vueuse/core'
import { useSubtitleTimingOffset } from '@/composables/useSubtitleTimingOffset'

defineProps<{ hasActiveSub: boolean }>()

const { offset, nudge, reset } = useSubtitleTimingOffset()

const open = ref(false)
const rootRef = ref<HTMLElement | null>(null)

onClickOutside(rootRef, () => { open.value = false })
onKeyStroke('Escape', () => { if (open.value) open.value = false })

// Signed, 1-decimal readout: '+1.5s', '0.0s', '-0.5s'. toFixed already emits the
// minus for negatives, so only positives need an explicit '+'.
const readout = computed(() => `${offset.value > 0 ? '+' : ''}${offset.value.toFixed(1)}s`)
</script>
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd frontend/web && bunx vitest run src/components/player/SubtitleSettingsMenu.spec.ts`
Expected: PASS — all 5 tests green.

- [ ] **Step 5: Commit**

```bash
git add frontend/web/src/components/player/SubtitleSettingsMenu.vue frontend/web/src/components/player/SubtitleSettingsMenu.spec.ts
git commit -m "feat(player): SubtitleSettingsMenu (gear + timing-offset popover)

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task 4: Wire into RawPlayer.vue

**Files:**
- Modify: `frontend/web/src/components/player/RawPlayer.vue` (template toolbar ~line 95-104, `<SubtitleOverlay>` ~line 60-66, script imports ~line 149-159)

- [ ] **Step 1: Import the component and composable**

In the `<script setup>` block, after the existing line:

```ts
import OtherSubsPanel from './OtherSubsPanel.vue'
```

add:

```ts
import SubtitleSettingsMenu from './SubtitleSettingsMenu.vue'
import { useSubtitleTimingOffset } from '@/composables/useSubtitleTimingOffset'
```

- [ ] **Step 2: Instantiate the composable**

In `<script setup>`, after this existing line:

```ts
const playerContainer = ref<HTMLElement | null>(null)
```

add:

```ts
const { offset: subtitleOffset } = useSubtitleTimingOffset()
```

- [ ] **Step 3: Pass the offset into SubtitleOverlay**

Find the overlay (currently has no `offset` prop):

```vue
        <SubtitleOverlay
          :video-element="videoRef"
          :subtitle-url="activeSubUrl"
          :format="activeSubFormat"
          :visible="!!activeSubUrl"
          :fullscreen-container="playerContainer"
        />
```

Replace with (add the `:offset` line):

```vue
        <SubtitleOverlay
          :video-element="videoRef"
          :subtitle-url="activeSubUrl"
          :format="activeSubFormat"
          :visible="!!activeSubUrl"
          :fullscreen-container="playerContainer"
          :offset="subtitleOffset"
        />
```

- [ ] **Step 4: Mount the gear in the toolbar**

Find the Other Subs button block:

```vue
        <button
          type="button"
          class="inline-flex items-center gap-2 px-4 py-2 rounded-md text-sm font-medium bg-cyan-500/15 hover:bg-cyan-500/25 text-cyan-100 border border-cyan-400/30 transition-colors"
          @click="otherSubsOpen = true"
        >
          <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 10h16M4 14h10M4 18h6" />
          </svg>
          {{ $t('player.otherSubs.openButton') }}
        </button>
```

Replace with (wrap the existing button and the new menu in a flex row so they sit together):

```vue
        <div class="flex items-center gap-2">
          <SubtitleSettingsMenu :has-active-sub="!!activeSubUrl" />
          <button
            type="button"
            class="inline-flex items-center gap-2 px-4 py-2 rounded-md text-sm font-medium bg-cyan-500/15 hover:bg-cyan-500/25 text-cyan-100 border border-cyan-400/30 transition-colors"
            @click="otherSubsOpen = true"
          >
            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 10h16M4 14h10M4 18h6" />
            </svg>
            {{ $t('player.otherSubs.openButton') }}
          </button>
        </div>
```

- [ ] **Step 5: Type-check**

Run: `cd frontend/web && bunx tsc --noEmit`
Expected: no errors referencing `RawPlayer.vue` or `SubtitleSettingsMenu`.

- [ ] **Step 6: Commit**

```bash
git add frontend/web/src/components/player/RawPlayer.vue
git commit -m "feat(player/raw): mount subtitle timing menu + wire offset into overlay

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task 5: Wire into OurEnglishPlayer.vue

**Files:**
- Modify: `frontend/web/src/components/player/OurEnglishPlayer.vue` (`<SubtitleOverlay>` ~line 60-66, subtitle toolbar row ~line 104-122, script import ~line 163)

- [ ] **Step 1: Import the component and composable**

In `<script setup>`, after the existing line:

```ts
import SubtitleOverlay from './SubtitleOverlay.vue'
```

add:

```ts
import SubtitleSettingsMenu from './SubtitleSettingsMenu.vue'
import { useSubtitleTimingOffset } from '@/composables/useSubtitleTimingOffset'
```

- [ ] **Step 2: Instantiate the composable**

In `<script setup>`, add near the other top-level refs (e.g. directly after the `import` lines' first `const ... = ref(...)` declaration — anywhere in setup scope is fine):

```ts
const { offset: subtitleOffset } = useSubtitleTimingOffset()
```

> If a name collision with an existing `subtitleOffset` appears, the type-check in Step 5 will catch it — rename the local alias (e.g. `subOffset`) and update the `:offset` binding to match.

- [ ] **Step 3: Pass the offset into SubtitleOverlay**

Find:

```vue
        <SubtitleOverlay
          :video-element="videoRef"
          :subtitle-url="activeSubUrl"
          :format="activeSubFormat"
          :visible="!!activeSubUrl"
          :fullscreen-container="playerContainer"
        />
```

Replace with:

```vue
        <SubtitleOverlay
          :video-element="videoRef"
          :subtitle-url="activeSubUrl"
          :format="activeSubFormat"
          :visible="!!activeSubUrl"
          :fullscreen-container="playerContainer"
          :offset="subtitleOffset"
        />
```

- [ ] **Step 4: Mount the gear next to the subtitle picker**

Find the subtitle picker `<select>` and its closing tag inside the toolbar:

```vue
          <select
            v-model="selectedSubKey"
            class="bg-white/10 hover:bg-white/15 text-white text-sm rounded-md px-3 py-1.5 border border-white/10 focus:outline-none focus:ring-2 focus:ring-cyan-400/40"
            :disabled="availableSubChoices.length === 0"
          >
            <option value="">{{ $t('player.subtitlePicker.none') }}</option>
            <option v-for="choice in availableSubChoices" :key="choice.key" :value="choice.key">
              {{ choice.label }}
            </option>
          </select>
        </div>
```

Replace with (insert the menu after the `</select>`, still inside the toolbar's inner flex row):

```vue
          <select
            v-model="selectedSubKey"
            class="bg-white/10 hover:bg-white/15 text-white text-sm rounded-md px-3 py-1.5 border border-white/10 focus:outline-none focus:ring-2 focus:ring-cyan-400/40"
            :disabled="availableSubChoices.length === 0"
          >
            <option value="">{{ $t('player.subtitlePicker.none') }}</option>
            <option v-for="choice in availableSubChoices" :key="choice.key" :value="choice.key">
              {{ choice.label }}
            </option>
          </select>

          <SubtitleSettingsMenu :has-active-sub="!!activeSubUrl" />
        </div>
```

- [ ] **Step 5: Type-check**

Run: `cd frontend/web && bunx tsc --noEmit`
Expected: no errors referencing `OurEnglishPlayer.vue` or `SubtitleSettingsMenu`.

- [ ] **Step 6: Run the existing OurEnglishPlayer spec (regression)**

Run: `cd frontend/web && bunx vitest run src/components/player/OurEnglishPlayer.spec.ts`
Expected: PASS (the existing spec must still be green after the toolbar/overlay edits).

- [ ] **Step 7: Commit**

```bash
git add frontend/web/src/components/player/OurEnglishPlayer.vue
git commit -m "feat(player/ourenglish): mount subtitle timing menu + wire offset into overlay

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task 6: Full-suite verification

**Files:** none (verification only)

- [ ] **Step 1: Run the player + composable test suites**

Run:
```bash
cd frontend/web && bunx vitest run src/components/player src/composables
```
Expected: PASS — including the two new specs and the existing player specs.

- [ ] **Step 2: Type-check the whole frontend**

Run: `cd frontend/web && bunx tsc --noEmit`
Expected: exit 0, no errors.

- [ ] **Step 3: Lint the touched files**

Run:
```bash
cd frontend/web && bunx eslint src/composables/useSubtitleTimingOffset.ts src/components/player/SubtitleSettingsMenu.vue src/components/player/RawPlayer.vue src/components/player/OurEnglishPlayer.vue
```
Expected: no errors.

- [ ] **Step 4: Post-deploy smoke (manual, after redeploy)**

After `make redeploy-web`, load a Raw-player anime, pick a JP subtitle, open the gear, click `+1s`/`−0.1s`, and confirm cues shift and the readout matches. Reload the page and confirm the offset survived (localStorage). Repeat once on an OurEnglish title. (i18n key paths are string-typed — confirm the gear label/popover text render as words, not raw `player.subtitleSettings.*` keys.)

---

## Notes for the executor

- **Deploy + changelog:** This plan does NOT include `/animeenigma-after-update`. Run it once after all tasks land (single batched changelog entry, Russian Trump-mode), per project policy. Do not push mid-plan — commits only.
- **No backend, no new npm deps** — `@vueuse/core` is already a dependency.
- **Font size is intentionally out of scope** (see spec Non-goals). If the user later wants it, add a second control row to `SubtitleSettingsMenu.vue` and a `fontScale` prop path through `SubtitleOverlay.vue`'s `baseFontSize` — the composable/menu structure already supports adding it.
