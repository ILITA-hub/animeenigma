# Shared Episode Selector (Phase A) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extract one shared `EpisodeSelector.vue` + one `useWatchedEpisodes` composable, and wire them into all 6 player SFCs so every player shows a consistent episode grid with watched-episode highlighting in its own accent color.

**Architecture:** A presentational `EpisodeSelector.vue` renders a normalized `EpisodeOption[]`, defining its own `.accent-*` classes against the **inherited** `--player-accent` / `--player-accent-rgb` CSS vars each player declares. A `useWatchedEpisodes(animeId)` composable is the single source of the watched-episode count (from `GET /users/watchlist/{id}`). Each player maps its native episode model → `EpisodeOption[]` and delegates rendering.

**Tech Stack:** Vue 3 `<script setup>` + TypeScript, Vitest + `@vue/test-utils`, Tailwind v4, `bun`/`bunx`.

**Scope:** This is Phase A of the spec `docs/superpowers/specs/2026-06-03-unified-watch-interface-design.md`. Phases B (`WatchSurface`) and C (presence layer) get their own plans.

---

## File Structure

- Create: `frontend/web/src/components/player/EpisodeSelector.types.ts` — `EpisodeOption` interface (separate `.ts` to avoid the vue-tsc `.vue` type-import false-pass gotcha).
- Create: `frontend/web/src/components/player/EpisodeSelector.vue` — shared presentational grid.
- Create: `frontend/web/src/components/player/__tests__/EpisodeSelector.spec.ts` — component tests.
- Create: `frontend/web/src/composables/useWatchedEpisodes.ts` — watched-count source.
- Create: `frontend/web/src/composables/__tests__/useWatchedEpisodes.spec.ts` — composable tests.
- Modify: `frontend/web/src/components/player/KodikPlayer.vue` (grid `:95-120`, watched `:800-818`, `--player-accent` `:1077`).
- Modify: `frontend/web/src/components/player/AnimeLibPlayer.vue` (grid `:112-137`, watched `:807-809,:854-863`, accent `:869`).
- Modify: `frontend/web/src/components/player/OurEnglishPlayer.vue` (grid `:157-170`, accent hardcoded `:574` — add `--player-accent` vars).
- Modify: `frontend/web/src/components/player/HanimePlayer.vue` (grid `:83-96`, accent `:479`).
- Modify: `frontend/web/src/components/player/RawPlayer.vue` (grid `:122-136`, accent hardcoded `:447` — add `--player-accent` vars).
- Modify: `frontend/web/src/components/player/Anime18Player.vue` (grid + accent `:443-450`).

**Run all commands from `frontend/web/`** unless noted.

---

### Task 1: `EpisodeOption` type + `EpisodeSelector.vue` + tests

**Files:**
- Create: `frontend/web/src/components/player/EpisodeSelector.types.ts`
- Create: `frontend/web/src/components/player/EpisodeSelector.vue`
- Test: `frontend/web/src/components/player/__tests__/EpisodeSelector.spec.ts`

- [ ] **Step 1: Create the type file**

`frontend/web/src/components/player/EpisodeSelector.types.ts`:
```ts
/** One episode entry rendered by EpisodeSelector.vue. Each player normalizes
 *  its native episode model into this shape. */
export interface EpisodeOption {
  /** Unique key — v-for key, selection match, and Phase-C data-wt-id. */
  key: string | number
  /** Text shown on the button (episode number / ordinal). */
  label: string | number
  /** Ordinal for watched comparison: watched when number <= watchedUpTo. */
  number: number
  /** Optional filler episode — dimmed. */
  isFiller?: boolean
}
```

- [ ] **Step 2: Write the failing test**

`frontend/web/src/components/player/__tests__/EpisodeSelector.spec.ts`:
```ts
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import EpisodeSelector from '../EpisodeSelector.vue'
import type { EpisodeOption } from '../EpisodeSelector.types'

const eps: EpisodeOption[] = [
  { key: 1, label: 1, number: 1 },
  { key: 2, label: 2, number: 2 },
  { key: 3, label: 3, number: 3 },
]

describe('EpisodeSelector', () => {
  it('renders one button per episode', () => {
    const w = mount(EpisodeSelector, { props: { episodes: eps, selectedKey: null } })
    expect(w.findAll('button')).toHaveLength(3)
  })

  it('marks the selected episode with accent-bg', () => {
    const w = mount(EpisodeSelector, { props: { episodes: eps, selectedKey: 2 } })
    const btns = w.findAll('button')
    expect(btns[1].classes()).toContain('accent-bg')
    expect(btns[0].classes()).not.toContain('accent-bg')
  })

  it('shows watched styling + badge for episodes <= watchedUpTo', () => {
    const w = mount(EpisodeSelector, { props: { episodes: eps, selectedKey: null, watchedUpTo: 2 } })
    const btns = w.findAll('button')
    expect(btns[0].classes()).toContain('accent-bg-muted')
    expect(btns[0].find('svg').exists()).toBe(true)
  })

  it('does not mark episodes beyond watchedUpTo as watched', () => {
    const w = mount(EpisodeSelector, { props: { episodes: eps, selectedKey: null, watchedUpTo: 2 } })
    const btns = w.findAll('button')
    expect(btns[2].classes()).not.toContain('accent-bg-muted')
    expect(btns[2].find('svg').exists()).toBe(false)
  })

  it('does not show the watched badge on the selected episode', () => {
    const w = mount(EpisodeSelector, { props: { episodes: eps, selectedKey: 1, watchedUpTo: 3 } })
    const first = w.findAll('button')[0]
    expect(first.classes()).toContain('accent-bg')
    expect(first.find('svg').exists()).toBe(false)
  })

  it('emits select with the episode key on click', async () => {
    const w = mount(EpisodeSelector, { props: { episodes: eps, selectedKey: null } })
    await w.findAll('button')[2].trigger('click')
    expect(w.emitted('select')?.[0]).toEqual([3])
  })

  it('exposes data-wt-id for the presence layer', () => {
    const w = mount(EpisodeSelector, { props: { episodes: eps, selectedKey: null } })
    expect(w.findAll('button')[0].attributes('data-wt-id')).toBe('episode:1')
  })
})
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `bunx vitest run src/components/player/__tests__/EpisodeSelector.spec.ts`
Expected: FAIL — `Failed to resolve import "../EpisodeSelector.vue"`.

- [ ] **Step 4: Create the component**

`frontend/web/src/components/player/EpisodeSelector.vue`:
```vue
<script setup lang="ts">
/**
 * Shared episode grid for every video player (Kodik, AnimeLib, OurEnglish,
 * Hanime, Raw, Anime18) in BOTH solo (Anime.vue) and Watch Together.
 * Presentational only — the host player owns episode + watched state and
 * passes a normalized list down.
 *
 * Watched episodes render in the HOST PLAYER'S accent: this component
 * redefines its own .accent-* classes against the inherited --player-accent
 * / --player-accent-rgb vars each player declares on its root. (Scoped class
 * definitions don't cross component boundaries, but CSS custom properties
 * inherit — so we redefine the classes here against the inherited vars.)
 */
import type { EpisodeOption } from './EpisodeSelector.types'

const props = withDefaults(defineProps<{
  episodes: EpisodeOption[]
  selectedKey: string | number | null
  watchedUpTo?: number
}>(), {
  watchedUpTo: 0,
})

const emit = defineEmits<{ (e: 'select', key: string | number): void }>()

function isWatched(ep: EpisodeOption): boolean {
  return ep.number <= props.watchedUpTo
}
</script>

<template>
  <div class="flex flex-wrap gap-2 max-h-32 overflow-y-auto custom-scrollbar p-1">
    <button
      v-for="ep in episodes"
      :key="ep.key"
      :data-wt-id="`episode:${ep.key}`"
      @click="emit('select', ep.key)"
      class="relative w-12 h-10 rounded-lg text-sm font-medium transition-all"
      :class="[
        selectedKey === ep.key
          ? 'accent-bg text-white'
          : isWatched(ep)
            ? 'accent-bg-muted accent-text border accent-border hover:brightness-125'
            : 'bg-white/10 text-white hover:bg-white/20',
        ep.isFiller ? 'opacity-60' : '',
      ]"
    >
      {{ ep.label }}
      <span
        v-if="isWatched(ep) && selectedKey !== ep.key"
        class="absolute -top-1 -right-1 w-3 h-3 accent-bg rounded-full flex items-center justify-center"
      >
        <svg class="w-2 h-2 text-black" fill="currentColor" viewBox="0 0 20 20">
          <path fill-rule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clip-rule="evenodd" />
        </svg>
      </span>
    </button>
  </div>
</template>

<style scoped>
/* Redefine accent utilities against the INHERITED player vars so watched
   episodes glow in the host player's accent. Each player declares
   --player-accent + --player-accent-rgb on its root. */
.accent-bg { background-color: var(--player-accent); }
.accent-text { color: color-mix(in srgb, var(--player-accent), white 40%); }
.accent-border { border-color: var(--player-accent); }
.accent-bg-muted { background-color: rgba(var(--player-accent-rgb), 0.28); }
</style>
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `bunx vitest run src/components/player/__tests__/EpisodeSelector.spec.ts`
Expected: PASS (7 tests).

- [ ] **Step 6: Type-check + design-system lint**

Run: `bunx vue-tsc --noEmit` → Expected: no errors.
Run: `bash scripts/design-system-lint.sh` → Expected: `ERRORS=0` (no hardcoded hex; `--player-accent` is not the deprecated `--accent`).

- [ ] **Step 7: Commit**

```bash
git add frontend/web/src/components/player/EpisodeSelector.types.ts \
        frontend/web/src/components/player/EpisodeSelector.vue \
        frontend/web/src/components/player/__tests__/EpisodeSelector.spec.ts
git commit -m "feat(player): shared EpisodeSelector.vue with per-player-accent watched state"
```

---

### Task 2: `useWatchedEpisodes` composable + tests

**Files:**
- Create: `frontend/web/src/composables/useWatchedEpisodes.ts`
- Test: `frontend/web/src/composables/__tests__/useWatchedEpisodes.spec.ts`

- [ ] **Step 1: Write the failing test**

`frontend/web/src/composables/__tests__/useWatchedEpisodes.spec.ts`:
```ts
import { describe, it, expect, vi, beforeEach } from 'vitest'

const getWatchlistEntry = vi.fn()
vi.mock('@/api/client', () => ({
  userApi: { getWatchlistEntry: (...a: unknown[]) => getWatchlistEntry(...a) },
}))

let isAuthenticated = true
vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({ get isAuthenticated() { return isAuthenticated } }),
}))

import { useWatchedEpisodes } from '../useWatchedEpisodes'

beforeEach(() => { getWatchlistEntry.mockReset(); isAuthenticated = true })

describe('useWatchedEpisodes', () => {
  it('reads entry.episodes (wrapped data.data) when authenticated', async () => {
    getWatchlistEntry.mockResolvedValue({ data: { data: { episodes: 7 } } })
    const { watchedUpTo, refresh } = useWatchedEpisodes('a1')
    await refresh()
    expect(watchedUpTo.value).toBe(7)
    expect(getWatchlistEntry).toHaveBeenCalledWith('a1')
  })

  it('handles the unwrapped data shape (data.episodes)', async () => {
    getWatchlistEntry.mockResolvedValue({ data: { episodes: 3 } })
    const { watchedUpTo, refresh } = useWatchedEpisodes('a1')
    await refresh()
    expect(watchedUpTo.value).toBe(3)
  })

  it('stays 0 and never calls the API when unauthenticated', async () => {
    isAuthenticated = false
    const { watchedUpTo, refresh } = useWatchedEpisodes('a1')
    await refresh()
    expect(watchedUpTo.value).toBe(0)
    expect(getWatchlistEntry).not.toHaveBeenCalled()
  })

  it('falls back to 0 on API error', async () => {
    getWatchlistEntry.mockRejectedValue(new Error('404'))
    const { watchedUpTo, refresh } = useWatchedEpisodes('a1')
    await refresh()
    expect(watchedUpTo.value).toBe(0)
  })
})
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `bunx vitest run src/composables/__tests__/useWatchedEpisodes.spec.ts`
Expected: FAIL — cannot resolve `../useWatchedEpisodes`.

- [ ] **Step 3: Create the composable**

`frontend/web/src/composables/useWatchedEpisodes.ts`:
```ts
import { ref, toValue, type MaybeRefOrGetter, type Ref } from 'vue'
import { userApi } from '@/api/client'
import { useAuthStore } from '@/stores/auth'

/**
 * Single source of the user's watched-episode count for an anime. Returns a
 * reactive `watchedUpTo` (episodes with number <= this are "watched") and a
 * `refresh()` the player calls on mount, on episode change, and after a
 * mark-watched. Per-user; never synced across Watch Together members.
 */
export function useWatchedEpisodes(animeId: MaybeRefOrGetter<string>): {
  watchedUpTo: Ref<number>
  refresh: () => Promise<void>
} {
  const watchedUpTo = ref(0)
  const auth = useAuthStore()

  async function refresh(): Promise<void> {
    if (!auth.isAuthenticated) {
      watchedUpTo.value = 0
      return
    }
    const id = toValue(animeId)
    if (!id) {
      watchedUpTo.value = 0
      return
    }
    try {
      const res = await userApi.getWatchlistEntry(id)
      const entry = res.data?.data ?? res.data
      watchedUpTo.value = Number(entry?.episodes) || 0
    } catch {
      // No entry / not in list / network — treat as none watched.
      watchedUpTo.value = 0
    }
  }

  return { watchedUpTo, refresh }
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `bunx vitest run src/composables/__tests__/useWatchedEpisodes.spec.ts`
Expected: PASS (4 tests).

- [ ] **Step 5: Commit**

```bash
git add frontend/web/src/composables/useWatchedEpisodes.ts \
        frontend/web/src/composables/__tests__/useWatchedEpisodes.spec.ts
git commit -m "feat(player): useWatchedEpisodes composable — single watched-count source"
```

---

### Task 3: Wire `KodikPlayer.vue`

KodikPlayer already has the canonical watched look; this swaps its inline grid for the shared component and the composable. Episode model = plain number.

**Files:** Modify `frontend/web/src/components/player/KodikPlayer.vue`

- [ ] **Step 1: Add imports** (with the other component/composable imports near the top of `<script setup>`):
```ts
import EpisodeSelector from './EpisodeSelector.vue'
import type { EpisodeOption } from './EpisodeSelector.types'
import { useWatchedEpisodes } from '@/composables/useWatchedEpisodes'
```

- [ ] **Step 2: Add the composable + normalized list.** Add near the other refs/computeds:
```ts
const { watchedUpTo, refresh: refreshWatched } = useWatchedEpisodes(() => props.animeId)

const episodeOptions = computed<EpisodeOption[]>(() =>
  episodeRange.value.map((n) => ({ key: n, label: n, number: n })),
)
```
(`episodeRange` is the existing computed at `KodikPlayer.vue:538`. Ensure `computed` is imported — it already is.)

- [ ] **Step 3: Replace the inline grid.** Delete the `<div class="flex flex-wrap gap-2 max-h-32 …">…</div>` block (`KodikPlayer.vue:95-120`) and replace with:
```vue
<EpisodeSelector
  :episodes="episodeOptions"
  :selected-key="selectedEpisode"
  :watched-up-to="watchedUpTo"
  @select="(k) => selectEpisode(Number(k))"
/>
```

- [ ] **Step 4: Replace the old watched logic.** Delete the `watchedEpisodes` ref (`:257`), the `isEpisodeWatched` function (`:816-818`), and the body of `fetchWatchedEpisodes` (`:800-813`). Wherever `fetchWatchedEpisodes()` was called (on mount and after `autoMarkEpisodeWatched` / manual mark / episode change), call `refreshWatched()` instead. Search the file for `fetchWatchedEpisodes` and `watchedEpisodes` and remove all remaining references.

- [ ] **Step 5: Confirm accent vars.** Verify the root style block declares both `--player-accent: #06b6d4;` and `--player-accent-rgb: 6, 182, 212;`. If `--player-accent-rgb` is missing, add it next to `--player-accent` (`:1077`).

- [ ] **Step 6: Type-check + existing specs + lint**

Run: `bunx vue-tsc --noEmit` → Expected: no errors (no dangling `watchedEpisodes`/`isEpisodeWatched`).
Run: `bunx vitest run src/components/player` → Expected: PASS.
Run: `bash scripts/design-system-lint.sh` → Expected: `ERRORS=0`.

- [ ] **Step 7: Commit**
```bash
git add frontend/web/src/components/player/KodikPlayer.vue
git commit -m "refactor(player): KodikPlayer uses shared EpisodeSelector + useWatchedEpisodes"
```

---

### Task 4: Wire `AnimeLibPlayer.vue`

Episode model = object `{ id:number; number:string; name:string }` (`AnimeLibEpisode[]`).

**Files:** Modify `frontend/web/src/components/player/AnimeLibPlayer.vue`

- [ ] **Step 1: Add imports** (in `<script setup>`, near the other imports):
```ts
import EpisodeSelector from './EpisodeSelector.vue'
import type { EpisodeOption } from './EpisodeSelector.types'
import { useWatchedEpisodes } from '@/composables/useWatchedEpisodes'
```
Ensure `computed` is imported from `vue`.

- [ ] **Step 2: Add composable + normalized list:**
```ts
const { watchedUpTo, refresh: refreshWatched } = useWatchedEpisodes(() => props.animeId)

const episodeOptions = computed<EpisodeOption[]>(() =>
  episodes.value.map((ep) => ({ key: ep.id, label: ep.number, number: Number(ep.number) })),
)
```
(`episodes` is the existing `AnimeLibEpisode[]` ref iterated at `AnimeLibPlayer.vue:114`.)

- [ ] **Step 3: Replace the inline grid** (`:112-137`) with:
```vue
<EpisodeSelector
  :episodes="episodeOptions"
  :selected-key="selectedEpisodeId"
  :watched-up-to="watchedUpTo"
  @select="onEpisodePicked"
/>
```
Use the player's existing selected-episode identifier in `:selected-key` (the `id` the buttons compared against — inspect the old grid's `:class` to confirm the exact ref name; it is the `id` bound in the old `v-for`). Add a thin handler if one doesn't already exist:
```ts
function onEpisodePicked(key: string | number) {
  const ep = episodes.value.find((e) => e.id === key)
  if (ep) selectEpisode(ep) // call the player's existing episode-select fn with its native arg
}
```
(Match `selectEpisode`'s existing signature — if it takes the episode object, pass `ep`; if it takes a number, pass `Number(ep.number)`.)

- [ ] **Step 4: Replace old watched logic.** Delete `watchedEpisodes` ref (`:420`), `isEpisodeWatched` (`:807-809`), and the watched-fetch in `onMounted` (`:854-863`); call `refreshWatched()` in its place and after mark-watched / episode change. Remove all remaining `watchedEpisodes`/`isEpisodeWatched` references.

- [ ] **Step 5: Confirm accent vars.** Verify `--player-accent: #f97316;` and add `--player-accent-rgb: 249, 115, 22;` if missing (`:869`).

- [ ] **Step 6: Verify**
Run: `bunx vue-tsc --noEmit` → no errors.
Run: `bunx vitest run src/components/player` → PASS.
Run: `bash scripts/design-system-lint.sh` → `ERRORS=0`.

- [ ] **Step 7: Commit**
```bash
git add frontend/web/src/components/player/AnimeLibPlayer.vue
git commit -m "refactor(player): AnimeLibPlayer uses shared EpisodeSelector + useWatchedEpisodes"
```

---

### Task 5: Wire `OurEnglishPlayer.vue` (adds watched-state — the reported fix)

Episode model = `{ id:string; number:number; title?:string; is_filler?:boolean }` (`ScraperEpisode[]`). Currently NO watched-state and a hardcoded cyan accent.

**Files:** Modify `frontend/web/src/components/player/OurEnglishPlayer.vue`

- [ ] **Step 1: Add imports** (in `<script setup>`, near the other imports):
```ts
import EpisodeSelector from './EpisodeSelector.vue'
import type { EpisodeOption } from './EpisodeSelector.types'
import { useWatchedEpisodes } from '@/composables/useWatchedEpisodes'
```

- [ ] **Step 2: Add composable + normalized list:**
```ts
const { watchedUpTo, refresh: refreshWatched } = useWatchedEpisodes(() => props.animeId)

const episodeOptions = computed<EpisodeOption[]>(() =>
  episodes.value.map((ep) => ({
    key: ep.id, label: ep.number, number: Number(ep.number), isFiller: ep.is_filler,
  })),
)
```

- [ ] **Step 3: Replace the inline grid** (`:157-170`) with:
```vue
<EpisodeSelector
  :episodes="episodeOptions"
  :selected-key="selectedEpisodeId"
  :watched-up-to="watchedUpTo"
  @select="onEpisodePicked"
/>
```
Use the player's existing selected-episode id ref for `:selected-key`. Add:
```ts
function onEpisodePicked(key: string | number) {
  const ep = episodes.value.find((e) => e.id === key)
  if (ep) selectEpisode(ep) // match the player's existing select signature
}
```

- [ ] **Step 4: Add watched refresh.** Call `refreshWatched()` in `onMounted` and after the player marks an episode watched / changes episode.

- [ ] **Step 5: Add the accent vars.** This player hardcodes `rgb(34, 211, 238)` at `:574`. In the player root's style, declare:
```css
--player-accent: #22d3ee;
--player-accent-rgb: 34, 211, 238;
```
on the same root element the grid lives under (so the inherited vars reach `EpisodeSelector`). Keep the existing cyan identity.

- [ ] **Step 6: Verify**
Run: `bunx vue-tsc --noEmit` → no errors.
Run: `bunx vitest run src/components/player` → PASS.
Run: `bash scripts/design-system-lint.sh` → `ERRORS=0` (`#22d3ee` must be a token/var or allowlisted — prefer the `--player-accent` var; if a raw hex is unavoidable add a justified line to `frontend/web/scripts/design-system-allowlist.txt`).

- [ ] **Step 7: Commit**
```bash
git add frontend/web/src/components/player/OurEnglishPlayer.vue
git commit -m "feat(player): OurEnglishPlayer gains watched-episode highlighting via shared selector"
```

---

### Task 6: Wire `HanimePlayer.vue`

Episode model = `{ name:string; slug:string }`, displayed as ordinal `idx+1`.

**Files:** Modify `frontend/web/src/components/player/HanimePlayer.vue`

- [ ] **Step 1: Add imports** (in `<script setup>`, near the other imports):
```ts
import EpisodeSelector from './EpisodeSelector.vue'
import type { EpisodeOption } from './EpisodeSelector.types'
import { useWatchedEpisodes } from '@/composables/useWatchedEpisodes'
```

- [ ] **Step 2: Add composable + normalized list:**
```ts
const { watchedUpTo, refresh: refreshWatched } = useWatchedEpisodes(() => props.animeId)

const episodeOptions = computed<EpisodeOption[]>(() =>
  episodes.value.map((ep, idx) => ({ key: ep.slug, label: idx + 1, number: idx + 1 })),
)
```

- [ ] **Step 3: Replace the inline grid** (`:83-96`) with:
```vue
<EpisodeSelector
  :episodes="episodeOptions"
  :selected-key="selectedSlug"
  :watched-up-to="watchedUpTo"
  @select="onEpisodePicked"
/>
```
Use the player's existing selected-slug ref for `:selected-key`. Add:
```ts
function onEpisodePicked(key: string | number) {
  const idx = episodes.value.findIndex((e) => e.slug === key)
  if (idx >= 0) selectEpisode(episodes.value[idx], idx) // existing signature: (ep, idx)
}
```

- [ ] **Step 4: Add watched refresh.** Call `refreshWatched()` in `onMounted` and after `markEpisodeWatched` (`:427-435`) / episode change.

- [ ] **Step 5: Confirm accent vars.** `--player-accent: #ec4899;` exists (`:479`); ensure `--player-accent-rgb: 236, 72, 153;` is present (it is, per `:480`).

- [ ] **Step 6: Verify**
Run: `bunx vue-tsc --noEmit` → no errors.
Run: `bunx vitest run src/components/player` → PASS.
Run: `bash scripts/design-system-lint.sh` → `ERRORS=0`.

- [ ] **Step 7: Commit**
```bash
git add frontend/web/src/components/player/HanimePlayer.vue
git commit -m "feat(player): HanimePlayer gains watched-episode highlighting via shared selector"
```

---

### Task 7: Wire `RawPlayer.vue`

Episode model = `{ id:string; number:number; … }` (`RawEpisode[]`). Hardcoded cyan accent; watch-tracking is a placeholder.

**Files:** Modify `frontend/web/src/components/player/RawPlayer.vue`

- [ ] **Step 1: Add imports** (in `<script setup>`, near the other imports):
```ts
import EpisodeSelector from './EpisodeSelector.vue'
import type { EpisodeOption } from './EpisodeSelector.types'
import { useWatchedEpisodes } from '@/composables/useWatchedEpisodes'
```

- [ ] **Step 2: Add composable + normalized list:**
```ts
const { watchedUpTo, refresh: refreshWatched } = useWatchedEpisodes(() => props.animeId)

const episodeOptions = computed<EpisodeOption[]>(() =>
  episodes.value.map((ep) => ({ key: ep.id, label: ep.number, number: Number(ep.number) })),
)
```

- [ ] **Step 3: Replace the inline grid** (`:122-136`) with:
```vue
<EpisodeSelector
  :episodes="episodeOptions"
  :selected-key="selectedEpisodeId"
  :watched-up-to="watchedUpTo"
  @select="onEpisodePicked"
/>
```
Use the existing selected-episode id ref. Add:
```ts
function onEpisodePicked(key: string | number) {
  const ep = episodes.value.find((e) => e.id === key)
  if (ep) selectEpisode(ep) // match existing signature
}
```

- [ ] **Step 4: Add watched refresh.** Call `refreshWatched()` in `onMounted` and on episode change.

- [ ] **Step 5: Add the accent vars.** This player hardcodes `rgb(34, 211, 238)` at `:447`. Declare on the player root:
```css
--player-accent: #22d3ee;
--player-accent-rgb: 34, 211, 238;
```

- [ ] **Step 6: Verify**
Run: `bunx vue-tsc --noEmit` → no errors.
Run: `bunx vitest run src/components/player` → PASS.
Run: `bash scripts/design-system-lint.sh` → `ERRORS=0` (allowlist the `#22d3ee` hex per Task 5 Step 6 if the lint flags it).

- [ ] **Step 7: Commit**
```bash
git add frontend/web/src/components/player/RawPlayer.vue
git commit -m "feat(player): RawPlayer gains watched-episode highlighting via shared selector"
```

---

### Task 8: Wire `Anime18Player.vue`

Episode model — inspect the file (mirrors Hanime or Raw). It already defines `--player-accent: #f43f5e;` + `--player-accent-rgb: 244, 63, 94;` (`:443-450`).

**Files:** Modify `frontend/web/src/components/player/Anime18Player.vue`

- [ ] **Step 1: Add imports** (in `<script setup>`, near the other imports):
```ts
import EpisodeSelector from './EpisodeSelector.vue'
import type { EpisodeOption } from './EpisodeSelector.types'
import { useWatchedEpisodes } from '@/composables/useWatchedEpisodes'
```

- [ ] **Step 2: Add composable + normalized list.** Inspect this player's episode iteration in its template; build `episodeOptions` the same way as the matching player above (slug-ordinal like Hanime, or `id`/`number` like Raw). Use:
```ts
const { watchedUpTo, refresh: refreshWatched } = useWatchedEpisodes(() => props.animeId)
```

- [ ] **Step 3: Replace the inline grid** with the `<EpisodeSelector … />` element (mirror the matching player's `:selected-key` + `@select` handler).

- [ ] **Step 4: Add watched refresh** in `onMounted` + on episode change.

- [ ] **Step 5: Verify**
Run: `bunx vue-tsc --noEmit` → no errors.
Run: `bunx vitest run src/components/player` → PASS.
Run: `bash scripts/design-system-lint.sh` → `ERRORS=0`.

- [ ] **Step 6: Commit**
```bash
git add frontend/web/src/components/player/Anime18Player.vue
git commit -m "feat(player): Anime18Player uses shared EpisodeSelector + watched state"
```

---

### Task 9: Full-suite gate + in-browser smoke + deploy

**Files:** none (verification + deploy).

- [ ] **Step 1: Full frontend gate**

Run: `bunx vue-tsc --noEmit` → Expected: no errors.
Run: `bunx vitest run` → Expected: all PASS.
Run: `bash scripts/design-system-lint.sh` → Expected: `ERRORS=0`.

- [ ] **Step 2: In-browser smoke (DS-NF-06 standing rule — jsdom cannot catch Tailwind v4 cascade bugs)**

Log in as `ui_audit_bot`, open an anime, and for EACH player (Kodik, AnimeLib if enabled, OurEnglish, Hanime, Raw) at **desktop + mobile** widths confirm:
- episode grid renders and selecting an episode works;
- watched episodes show the muted-accent fill + checkmark badge in **that player's accent color** (verify OurEnglish/Hanime/Raw now highlight — the original complaint);
- Kodik/AnimeLib look unchanged.

- [ ] **Step 3: Deploy**

Invoke `/animeenigma-after-update` ONCE for the whole Phase A batch (redeploys `web`, updates changelog in Russian Trump-mode, commits, pushes). Do not run per-task after-updates.

---

## Self-Review Notes

- **Spec coverage:** Phase A §5 fully covered — `EpisodeSelector` (Task 1), `useWatchedEpisodes` (Task 2), all 6 players wired with per-player accent + `--player-accent-rgb` ensured (Tasks 3–8), watched-state added to OurEnglish/Hanime/Raw (Tasks 5–7, the reported fix), `data-wt-id` present for Phase C, design-lint + browser smoke (Tasks 1/9). Phases B and C are explicitly out of this plan.
- **Type consistency:** `EpisodeOption { key, label, number, isFiller? }` defined once in `EpisodeSelector.types.ts` and imported everywhere; `useWatchedEpisodes` returns `{ watchedUpTo, refresh }` used uniformly; `@select` always emits `key`.
- **Known unknowns left to the executor (file-read required, not placeholders):** the exact name of each player's selected-episode ref and its `selectEpisode` signature — the plan instructs inspecting the old grid's binding and matching the native call. These differ per player and cannot be hardcoded without the executor reading the file; the mapping and handler shape are fully specified.
