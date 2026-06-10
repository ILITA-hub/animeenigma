# Unified Player Fix Pack Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship the approved spec `docs/superpowers/specs/2026-06-10-unified-player-fix-pack-design.md` — buffering spinner, hacker-mode debug HUD, episodes drawer, ±5s glyph fix, 60s buffer window, real quality ladder, AniSkip intro/outro wiring — all in the unified player.

**Architecture:** All frontend, inside `frontend/web/src/components/player/unified/` + `frontend/web/src/composables/unifiedPlayer/`. The AniSkip backend (`/api/skip-times/{malId}/{episode}`), API client fn `animeApi.getSkipTimes`, and the `useSkipTimes` composable **already exist and are consumer-less** — Task 7 only wires them in. No Go changes.

**Tech Stack:** Vue 3 `<script setup>` + TS, hls.js ~1.5.20 (pinned), Vitest + @vue/test-utils, bun/bunx.

**Git strategy (IMPORTANT):** The shared tree is on `feat/ds-primitives-lucide`, which carries today's parallel fleet work *including the lucide player migration these files depend on* — main does NOT have it, so cherry-picks to main would conflict. Commit path-scoped to the current branch and push after every commit: `git push origin HEAD:feat/ds-primitives-lucide`. Do NOT `git add -A` (parallel agents' dirty files). Do NOT checkout main. The final report must flag that the branch needs a merge to main.

**Verification commands:**
- Unit: `cd /data/animeenigma/frontend/web && bunx vitest run src/components/player/unified/ src/composables/unifiedPlayer/`
- Types: `cd /data/animeenigma/frontend/web && bunx tsc --noEmit`
- DS lint: `bash /data/animeenigma/frontend/web/scripts/design-system-lint.sh`

---

### Task 1: ±5s skip-button glyph fix

The digit "5" drifts because the forward button mirrors the whole SVG path with `scaleX(-1)` while the `<text>` is not mirrored, and the two buttons use different x offsets (12.5 vs 11.5).

**Files:**
- Modify: `frontend/web/src/components/player/unified/PlayerControlBar.vue:30-60`
- Test: `frontend/web/src/components/player/unified/PlayerControlBar.spec.ts`

- [ ] **Step 1: Write the failing test** — append inside the existing `describe('PlayerControlBar')`:

```ts
  it('centers the digit 5 identically in both skip buttons (no drift)', () => {
    const w = mount(PlayerControlBar, { props: baseProps })
    const back = w.find('[data-test="seek-back"] text')
    const fwd = w.find('[data-test="seek-fwd"] text')
    expect(back.exists()).toBe(true)
    expect(fwd.exists()).toBe(true)
    expect(back.attributes('x')).toBe('12')
    expect(fwd.attributes('x')).toBe('12')
    expect(back.attributes('y')).toBe(fwd.attributes('y'))
    expect(back.attributes('text-anchor')).toBe('middle')
    expect(fwd.attributes('text-anchor')).toBe('middle')
  })
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /data/animeenigma/frontend/web && bunx vitest run src/components/player/unified/PlayerControlBar.spec.ts`
Expected: FAIL — `back.attributes('x')` is `'12.5'`.

- [ ] **Step 3: Fix the SVGs.** Replace the two skip-button SVG bodies (the digit stays an inline SVG composite — lucide keep-list). Mirror ONLY the path via a `<g>`; both digits at the same centered coordinates:

```html
      <!-- −5s (hidden on mobile via CSS) — circular replay arrow w/ "5" inside -->
      <button
        class="pl-icon pl-skip-back"
        aria-label="Back 5 seconds"
        data-test="seek-back"
        @click="emit('seek-rel', -5)"
      >
        <svg width="22" height="22" viewBox="0 0 24 24" fill="none"
          stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"
          aria-hidden="true"
        >
          <path d="M4 4v6h6M4 10a8 8 0 11-1 4" />
          <text x="12" y="16" font-size="8" font-weight="700" font-family="var(--font-mono,monospace)" fill="currentColor" stroke="none" text-anchor="middle">5</text>
        </svg>
      </button>

      <!-- +5s (hidden on mobile via CSS) — circular forward arrow w/ "5" inside -->
      <button
        class="pl-icon pl-skip-fwd"
        aria-label="Forward 5 seconds"
        data-test="seek-fwd"
        @click="emit('seek-rel', 5)"
      >
        <svg width="22" height="22" viewBox="0 0 24 24" fill="none"
          stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"
          aria-hidden="true"
        >
          <g style="transform: scaleX(-1); transform-origin: center">
            <path d="M4 4v6h6M4 10a8 8 0 11-1 4" />
          </g>
          <text x="12" y="16" font-size="8" font-weight="700" font-family="var(--font-mono,monospace)" fill="currentColor" stroke="none" text-anchor="middle">5</text>
        </svg>
      </button>
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /data/animeenigma/frontend/web && bunx vitest run src/components/player/unified/PlayerControlBar.spec.ts`
Expected: PASS (all tests).

- [ ] **Step 5: Commit + push**

```bash
cd /data/animeenigma
git add frontend/web/src/components/player/unified/PlayerControlBar.vue frontend/web/src/components/player/unified/PlayerControlBar.spec.ts
git commit -m "fix(unified-player): center the 5 digit in both skip buttons (mirror path only)"
git push origin HEAD:feat/ds-primitives-lucide
```

(Use the standard co-author trailer block from MEMORY.md on every commit in this plan.)

---

### Task 2: BufferingOverlay component

**Files:**
- Create: `frontend/web/src/components/player/unified/overlays/BufferingOverlay.vue`
- Test: `frontend/web/src/components/player/unified/overlays/BufferingOverlay.spec.ts`

- [ ] **Step 1: Write the failing test**

```ts
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import BufferingOverlay from './BufferingOverlay.vue'

describe('BufferingOverlay', () => {
  it('renders the spinner when visible', () => {
    const w = mount(BufferingOverlay, { props: { visible: true } })
    expect(w.find('[data-test="buffering-overlay"]').exists()).toBe(true)
    expect(w.find('.pl-buffering-ring').exists()).toBe(true)
  })

  it('renders nothing when not visible', () => {
    const w = mount(BufferingOverlay, { props: { visible: false } })
    expect(w.find('[data-test="buffering-overlay"]').exists()).toBe(false)
  })

  it('is announced as status for screen readers', () => {
    const w = mount(BufferingOverlay, { props: { visible: true } })
    expect(w.find('[data-test="buffering-overlay"]').attributes('role')).toBe('status')
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /data/animeenigma/frontend/web && bunx vitest run src/components/player/unified/overlays/BufferingOverlay.spec.ts`
Expected: FAIL — cannot resolve `./BufferingOverlay.vue`.

- [ ] **Step 3: Create the component**

```vue
<template>
  <div
    v-if="visible"
    class="pl-buffering"
    data-test="buffering-overlay"
    role="status"
    aria-label="Buffering"
  >
    <span class="pl-buffering-ring" aria-hidden="true" />
  </div>
</template>

<script setup lang="ts">
defineProps<{ visible: boolean }>()
</script>

<style scoped>
.pl-buffering {
  position: absolute;
  inset: 0;
  z-index: 4;
  display: grid;
  place-items: center;
  pointer-events: none;
}

/* Ring spinner — DS keep-list style (inline, brand-cyan top arc). */
.pl-buffering-ring {
  width: 54px;
  height: 54px;
  border-radius: 50%;
  border: 3px solid rgba(255, 255, 255, 0.2);
  border-top-color: var(--brand-cyan);
  filter: drop-shadow(0 0 8px rgba(0, 212, 255, 0.45));
  animation: pl-buffer-spin 0.8s linear infinite;
}

@keyframes pl-buffer-spin {
  to {
    transform: rotate(360deg);
  }
}
</style>
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /data/animeenigma/frontend/web && bunx vitest run src/components/player/unified/overlays/BufferingOverlay.spec.ts`
Expected: PASS.

- [ ] **Step 5: Commit + push**

```bash
cd /data/animeenigma
git add frontend/web/src/components/player/unified/overlays/BufferingOverlay.vue frontend/web/src/components/player/unified/overlays/BufferingOverlay.spec.ts
git commit -m "feat(unified-player): BufferingOverlay ring spinner component"
git push origin HEAD:feat/ds-primitives-lucide
```

---

### Task 3: Wire buffering state into UnifiedPlayer

**Files:**
- Modify: `frontend/web/src/components/player/unified/UnifiedPlayer.vue` (video element events at :25-35, BigPlayButton at :110-113, script)

- [ ] **Step 1: Add buffering state to the script section** (place after the "rAF progress loop" section, before "Next episode logic"):

```ts
// ─── Buffering indicator ──────────────────────────────────────────────────────
// waiting/stalled/seeking → on; playing/canplay → off. A 150ms grace window
// keeps instant in-buffer seeks from flashing the ring. `seeked` only clears
// when the element actually has decodable data (readyState ≥ 3) — otherwise
// the following `waiting` keeps the ring up.

const isBuffering = ref(false)
const showBuffering = ref(false)
let bufferingTimer: ReturnType<typeof setTimeout> | null = null

function setBuffering(on: boolean) {
  if (on === isBuffering.value) return
  isBuffering.value = on
  if (on) {
    bufferingTimer = setTimeout(() => {
      showBuffering.value = true
    }, 150)
  } else {
    if (bufferingTimer) {
      clearTimeout(bufferingTimer)
      bufferingTimer = null
    }
    showBuffering.value = false
  }
}

function onBufferStart() {
  setBuffering(true)
}

function onBufferEnd() {
  setBuffering(false)
}

function onSeeked() {
  const v = videoRef.value
  if (v && v.readyState >= 3) setBuffering(false)
}
```

- [ ] **Step 2: Bind the events on the `<video>` element** — extend the existing element:

```html
    <video
      ref="videoRef"
      class="absolute inset-0 w-full h-full object-contain z-[1]"
      playsinline
      preload="auto"
      @play="onVideoPlay"
      @pause="onVideoPause"
      @ended="onEnded"
      @click="togglePlay"
      @volumechange="onVolumeChange"
      @waiting="onBufferStart"
      @stalled="onBufferStart"
      @seeking="onBufferStart"
      @canplay="onBufferEnd"
      @playing="onBufferEnd"
      @seeked="onSeeked"
    />
```

- [ ] **Step 3: Mount the overlay + suppress the big play button while buffering.** Import `BufferingOverlay` next to the other overlay imports:

```ts
import BufferingOverlay from './overlays/BufferingOverlay.vue'
```

Replace the BigPlayButton block and add the overlay after it:

```html
    <BigPlayButton
      :visible="!state.playing.value && !sourceError && !showBuffering && !isResolving"
      @play="togglePlay"
    />

    <BufferingOverlay :visible="(showBuffering || isResolving) && !sourceError" />
```

- [ ] **Step 4: Clear the timer on unmount** — extend the existing `onUnmounted`:

```ts
onUnmounted(() => {
  stopRaf()
  clearNextEpTimer()
  if (bufferingTimer) clearTimeout(bufferingTimer)
  window.removeEventListener('keydown', onKeydown)
})
```

- [ ] **Step 5: Verify types + existing tests still pass**

Run: `cd /data/animeenigma/frontend/web && bunx tsc --noEmit && bunx vitest run src/components/player/unified/`
Expected: PASS, no type errors.

- [ ] **Step 6: Commit + push**

```bash
cd /data/animeenigma
git add frontend/web/src/components/player/unified/UnifiedPlayer.vue
git commit -m "feat(unified-player): buffering spinner on seek/stall (150ms grace)"
git push origin HEAD:feat/ds-primitives-lucide
```

---

### Task 4: Episodes drawer (fixes the dead hamburger)

**Files:**
- Create: `frontend/web/src/components/player/unified/EpisodesPanel.vue`
- Test: `frontend/web/src/components/player/unified/EpisodesPanel.spec.ts`
- Modify: `frontend/web/src/components/player/unified/UnifiedPlayer.vue` (top-bar buttons, MenuKind, floating panel, emits)
- Modify: `frontend/web/src/components/player/unified/PlayerControlBar.vue:185` (openMenu prop type)
- Modify: `frontend/web/src/views/Anime.vue:649` (remove no-op handler)

- [ ] **Step 1: Write the failing test**

```ts
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import EpisodesPanel from './EpisodesPanel.vue'
import type { EpisodeOption } from '@/components/player/EpisodeSelector.types'

const eps: EpisodeOption[] = [
  { key: 1, label: 1, number: 1 },
  { key: 2, label: 2, number: 2 },
  { key: 3, label: 3, number: 3, isFiller: true },
]

describe('EpisodesPanel', () => {
  it('renders one button per episode + a count', () => {
    const w = mount(EpisodesPanel, { props: { episodes: eps, selectedNumber: 1 } })
    expect(w.findAll('[data-test^="episode-"]').length).toBe(3)
    expect(w.text()).toContain('3')
  })

  it('highlights the selected episode', () => {
    const w = mount(EpisodesPanel, { props: { episodes: eps, selectedNumber: 2 } })
    expect(w.find('[data-test="episode-2"]').classes().join(' ')).toContain('brand-cyan')
    expect(w.find('[data-test="episode-1"]').classes().join(' ')).not.toContain('brand-cyan')
  })

  it('emits select with the episode option on click', async () => {
    const w = mount(EpisodesPanel, { props: { episodes: eps, selectedNumber: 1 } })
    await w.find('[data-test="episode-2"]').trigger('click')
    expect(w.emitted('select')?.[0]).toEqual([eps[1]])
  })

  it('dims filler episodes', () => {
    const w = mount(EpisodesPanel, { props: { episodes: eps, selectedNumber: 1 } })
    expect(w.find('[data-test="episode-3"]').classes()).toContain('opacity-50')
  })

  it('shows an empty state when there are no episodes', () => {
    const w = mount(EpisodesPanel, { props: { episodes: [], selectedNumber: null } })
    expect(w.text()).toContain('No episodes from this source')
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /data/animeenigma/frontend/web && bunx vitest run src/components/player/unified/EpisodesPanel.spec.ts`
Expected: FAIL — cannot resolve `./EpisodesPanel.vue`.

- [ ] **Step 3: Create the component**

```vue
<template>
  <div class="flex flex-col" data-test="episodes-panel">
    <div class="px-3 pt-3 pb-2 flex items-baseline justify-between">
      <span class="text-[13px] font-semibold text-white">Episodes</span>
      <span class="text-[11px] text-[var(--muted-foreground)]">{{ episodes.length }}</span>
    </div>

    <div v-if="episodes.length === 0" class="px-3 pb-3 text-[13px] text-[var(--muted-foreground)]">
      No episodes from this source
    </div>

    <div v-else class="grid grid-cols-5 gap-[6px] p-3 pt-1">
      <button
        v-for="ep in episodes"
        :key="ep.key"
        :class="[
          'h-9 rounded-[var(--r-sm)] border-0 text-[13px] font-semibold transition-colors cursor-pointer',
          ep.number === selectedNumber
            ? 'text-[var(--brand-cyan)]'
            : 'text-[var(--ink-2)] hover:bg-white/[0.14] hover:text-white',
          ep.isFiller ? 'opacity-50' : '',
        ]"
        :style="ep.number === selectedNumber
          ? 'background: rgba(0,212,255,0.18)'
          : 'background: rgba(255,255,255,0.07)'"
        :data-test="`episode-${ep.number}`"
        @click="emit('select', ep)"
      >
        {{ ep.label }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { EpisodeOption } from '@/components/player/EpisodeSelector.types'

defineProps<{
  episodes: EpisodeOption[]
  selectedNumber: number | null
}>()

const emit = defineEmits<{
  (e: 'select', ep: EpisodeOption): void
}>()
</script>
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /data/animeenigma/frontend/web && bunx vitest run src/components/player/unified/EpisodesPanel.spec.ts`
Expected: PASS.

- [ ] **Step 5: Wire into UnifiedPlayer.** All edits in `UnifiedPlayer.vue`:

(a) Import:

```ts
import EpisodesPanel from './EpisodesPanel.vue'
```

(b) Widen the menu union (script, "Menu state" section):

```ts
type MenuKind = 'source' | 'settings' | 'subs' | 'episodes' | null
```

(c) Both top-bar buttons open the drawer — replace `@click="$emit('open-episodes')"` on BOTH the back button and the episode-list button with `@click="toggleMenu('episodes')"` (they're inside `.pl-top` which already stops propagation).

(d) Drop the dead emit — the emits block becomes:

```ts
defineEmits<{
  (e: 'toggle-theater'): void
}>()
```

(e) Add the floating panel right after the Source panel block:

```html
    <!-- Episodes drawer (floating, top-right — reuses source-panel geometry) -->
    <div v-if="openMenu === 'episodes'" class="pl-floating pl-floating--source" @click.stop>
      <EpisodesPanel
        :episodes="episodes"
        :selected-number="selectedEpisode?.number ?? null"
        @select="onSelectEpisode"
      />
    </div>
```

(f) Selection handler (place near `onSelectProvider`):

```ts
function onSelectEpisode(ep: EpisodeOption) {
  openMenu.value = null
  if (selectedEpisode.value?.number === ep.number) return
  // The audio/lang/server/episode watcher re-resolves the stream.
  selectedEpisode.value = ep
}
```

- [ ] **Step 6: Widen PlayerControlBar's openMenu prop type** (`PlayerControlBar.vue`, props interface) so passing `'episodes'` type-checks (no is-open highlight needed for it):

```ts
    /** which floating menu is open, for trigger-button is-open highlight */
    openMenu?: 'source' | 'settings' | 'subs' | 'episodes' | null
```

- [ ] **Step 7: Remove the no-op in Anime.vue** — delete line 649 (`@open-episodes="() => {}"`), leaving:

```html
        <UnifiedPlayer
          v-if="unifiedSelected && unifiedPlayerEnabled"
          :anime-id="anime.id"
          :anime="{ title: anime.title, ep: (anime.episodesAired || 1), eps: (anime.totalEpisodes || anime.episodesAired || 1), still: anime.coverImage }"
          :theater="theaterMode"
          :is-hentai="isHentai"
          :initial-episode="resumeStartEpisode"
          @toggle-theater="setTheater(!theaterMode)"
        />
```

- [ ] **Step 8: Verify**

Run: `cd /data/animeenigma/frontend/web && bunx tsc --noEmit && bunx vitest run src/components/player/unified/`
Expected: PASS, no type errors.

- [ ] **Step 9: Commit + push**

```bash
cd /data/animeenigma
git add frontend/web/src/components/player/unified/EpisodesPanel.vue frontend/web/src/components/player/unified/EpisodesPanel.spec.ts frontend/web/src/components/player/unified/UnifiedPlayer.vue frontend/web/src/components/player/unified/PlayerControlBar.vue frontend/web/src/views/Anime.vue
git commit -m "feat(unified-player): in-player episodes drawer (fixes dead hamburger button)"
git push origin HEAD:feat/ds-primitives-lucide
```

---

### Task 5: Buffer window — 60s ahead

**Files:**
- Modify: `frontend/web/src/composables/unifiedPlayer/useVideoEngine.ts:65`

- [ ] **Step 1: Extend the hls.js config** (config-only; covered by the in-browser smoke in Task 11):

```ts
    hls = new Hls({
      enableWorker: true,
      backBufferLength: 90,
      // Seek-ahead window (spec 2026-06-10): keep ~1 min buffered ahead so
      // ±5s arrow-key seeks land inside the buffer and resolve instantly.
      // backBufferLength 90 already covers the "10s behind" requirement.
      maxBufferLength: 60,
      maxMaxBufferLength: 120,
    })
```

- [ ] **Step 2: Verify types + tests**

Run: `cd /data/animeenigma/frontend/web && bunx tsc --noEmit && bunx vitest run src/composables/unifiedPlayer/useVideoEngine.spec.ts`
Expected: PASS.

- [ ] **Step 3: Commit + push**

```bash
cd /data/animeenigma
git add frontend/web/src/composables/unifiedPlayer/useVideoEngine.ts
git commit -m "perf(unified-player): 60s forward buffer window for instant arrow-key seeks"
git push origin HEAD:feat/ds-primitives-lucide
```

---

### Task 6: Real quality ladder

**Files:**
- Modify: `frontend/web/src/composables/unifiedPlayer/useVideoEngine.ts`
- Modify: `frontend/web/src/composables/unifiedPlayer/useVideoEngine.spec.ts`
- Modify: `frontend/web/src/components/player/unified/UnifiedPlayer.vue` (settings menu wiring, mp4 swap)
- Modify: `frontend/web/src/components/player/unified/PlaybackSettingsMenu.vue` (qualityDisplay + HD badge)

- [ ] **Step 1: Write the failing tests** — append to `useVideoEngine.spec.ts`:

```ts
import { buildLevelLabels } from './useVideoEngine'

describe('buildLevelLabels', () => {
  it('labels levels by height, sorted high to low, keeping hls indexes', () => {
    expect(
      buildLevelLabels([{ height: 480 }, { height: 1080 }, { height: 720 }]),
    ).toEqual([
      { label: '1080p', index: 1 },
      { label: '720p', index: 2 },
      { label: '480p', index: 0 },
    ])
  })

  it('dedupes same-height levels keeping the first index', () => {
    expect(
      buildLevelLabels([{ height: 720, bitrate: 2_000_000 }, { height: 720, bitrate: 1_000_000 }]),
    ).toEqual([{ label: '720p', index: 0 }])
  })

  it('falls back to bitrate labels when height is missing', () => {
    expect(buildLevelLabels([{ bitrate: 1_500_000 }])).toEqual([{ label: '1500k', index: 0 }])
  })

  it('returns empty for empty/unlabelable input', () => {
    expect(buildLevelLabels([])).toEqual([])
    expect(buildLevelLabels([{}])).toEqual([])
  })
})
```

- [ ] **Step 2: Run to verify failure**

Run: `cd /data/animeenigma/frontend/web && bunx vitest run src/composables/unifiedPlayer/useVideoEngine.spec.ts`
Expected: FAIL — `buildLevelLabels` is not exported.

- [ ] **Step 3: Implement in `useVideoEngine.ts`.**

(a) Add after `chooseLoadStrategy`:

```ts
export interface QualityLevel {
  label: string
  index: number
}

/**
 * Build the quality menu from hls.js levels: label by height (`720p`),
 * dedupe by label keeping the FIRST hls index for it, sort high→low.
 * Height-less levels fall back to a bitrate label (`1500k`); unlabelable
 * levels are dropped (no fake entries — design rule D-05).
 */
export function buildLevelLabels(
  levels: { height?: number; bitrate?: number }[],
): QualityLevel[] {
  const byLabel = new Map<string, number>()
  levels.forEach((l, index) => {
    const label = l.height
      ? `${l.height}p`
      : l.bitrate
        ? `${Math.round(l.bitrate / 1000)}k`
        : ''
    if (!label || byLabel.has(label)) return
    byLabel.set(label, index)
  })
  return [...byLabel.entries()]
    .map(([label, index]) => ({ label, index }))
    .sort((a, b) => parseInt(b.label) - parseInt(a.label))
}
```

(b) Inside `useVideoEngine()`, add state next to `fatal`:

```ts
  const levels = ref<QualityLevel[]>([])
  const currentLevelLabel = ref('')
```

(c) In `load()`, reset both right after `fatal.value = null`:

```ts
    fatal.value = null
    levels.value = []
    currentLevelLabel.value = ''
```

(d) Replace the `MANIFEST_PARSED` handler and add `LEVEL_SWITCHED` after it:

```ts
    hls.on(Hls.Events.MANIFEST_PARSED, (_e: unknown, data: any) => {
      levels.value = buildLevelLabels(data?.levels ?? [])
      hls?.startLoad(-1)
    })
    hls.on(Hls.Events.LEVEL_SWITCHED, (_e: unknown, data: any) => {
      const lvl = levels.value.find((l) => l.index === data?.level)
      if (lvl) currentLevelLabel.value = lvl.label
    })
```

(e) Add `setLevel` before `destroy()` and export everything:

```ts
  function setLevel(label: string) {
    if (!hls) return
    if (label === 'Auto') {
      hls.currentLevel = -1
      return
    }
    const lvl = levels.value.find((l) => l.label === label)
    if (lvl) hls.currentLevel = lvl.index
  }
```

```ts
  return { fatal, load, destroy, levels, currentLevelLabel, setLevel }
```

- [ ] **Step 4: Run engine tests**

Run: `cd /data/animeenigma/frontend/web && bunx vitest run src/composables/unifiedPlayer/useVideoEngine.spec.ts`
Expected: PASS.

- [ ] **Step 5: Wire UnifiedPlayer.** All in `UnifiedPlayer.vue`:

(a) Track the current stream — add near `resolvedServers`:

```ts
const currentStream = ref<StreamResult | null>(null)
```

with the type import added to the existing import block:

```ts
import type { StreamResult } from '@/types/unifiedPlayer'
```

In BOTH `loadEpisodesAndStream()` and `resolveStreamForEpisode()`, right after `await engine.load(stream)` add:

```ts
    currentStream.value = stream
```

(b) Quality plumbing (new section after "Active provider display info"):

```ts
// ─── Quality ladder ──────────────────────────────────────────────────────────
// HLS: data-driven from hls.js levels. MP4: only when the provider returned
// multiple URL-valued qualities. Single-variant streams stay Auto-only (D-05).

const mp4Qualities = computed(() => {
  const s = currentStream.value
  if (!s || s.type !== 'mp4' || !s.qualities) return []
  return s.qualities.filter(
    (q) => typeof q.value === 'string' && /^(https?:|\/)/.test(q.value as string),
  )
})

const qualities = computed(() => {
  if (mp4Qualities.value.length > 1) return ['Auto', ...mp4Qualities.value.map((q) => q.label)]
  return ['Auto', ...engine.levels.value.map((l) => l.label)]
})

const qualityDisplay = computed(() =>
  state.quality.value === 'Auto' && engine.currentLevelLabel.value
    ? `Auto · ${engine.currentLevelLabel.value}`
    : state.quality.value,
)

// New stream may not offer the previously-chosen quality — snap back to Auto.
watch(qualities, (qs) => {
  if (!qs.includes(state.quality.value)) state.quality.value = 'Auto'
})

function swapMp4Source(url: string) {
  const v = videoRef.value
  if (!v) return
  const t = v.currentTime
  const wasPlaying = !v.paused
  v.addEventListener(
    'loadedmetadata',
    () => {
      v.currentTime = t
      if (wasPlaying) void v.play()
    },
    { once: true },
  )
  v.src = url
}

function onSetQuality(q: string) {
  state.quality.value = q
  const mq = mp4Qualities.value.find((x) => x.label === q)
  if (mq) {
    swapMp4Source(mq.value as string)
    return
  }
  if (q === 'Auto' && currentStream.value?.type === 'mp4') {
    swapMp4Source(currentStream.value.url)
    return
  }
  engine.setLevel(q)
}
```

(c) Update the settings-menu binding:

```html
      <PlaybackSettingsMenu
        :quality="state.quality.value"
        :qualities="qualities"
        :quality-display="qualityDisplay"
        :speed="state.speed.value"
        :speeds="[0.75, 1, 1.25, 1.5, 2]"
        :auto-next="state.autoNext.value"
        :auto-skip="state.autoSkip.value"
        @update:quality="onSetQuality"
        @update:speed="onSetSpeed"
        @update:auto-next="v => { state.autoNext.value = v }"
        @update:auto-skip="v => { state.autoSkip.value = v }"
      />
```

- [ ] **Step 6: PlaybackSettingsMenu — display label + HD badge on the highest real level.**

(a) Props gain:

```ts
defineProps<{
  quality: string
  qualities: string[]
  /** e.g. "Auto · 720p" while auto-switching; falls back to `quality` */
  qualityDisplay?: string
  speed: number
  speeds: number[]
  autoNext: boolean
  autoSkip: boolean
}>()
```

(b) Quality row value span shows `{{ qualityDisplay ?? quality }}` instead of `{{ quality }}`.

(c) HD badge condition (quality sub-view) — replace `v-if="q === qualities[0]"` with:

```html
          <span v-if="q !== 'Auto' && q === (qualities.find(x => x !== 'Auto') ?? '')" class="ml-auto text-[10px] font-semibold uppercase text-white" style="background: var(--brand-cyan); padding: 1px 5px; border-radius: 4px; color: var(--color-base);">HD</span>
```

- [ ] **Step 7: Verify**

Run: `cd /data/animeenigma/frontend/web && bunx tsc --noEmit && bunx vitest run src/components/player/unified/ src/composables/unifiedPlayer/`
Expected: PASS.

- [ ] **Step 8: Commit + push**

```bash
cd /data/animeenigma
git add frontend/web/src/composables/unifiedPlayer/useVideoEngine.ts frontend/web/src/composables/unifiedPlayer/useVideoEngine.spec.ts frontend/web/src/components/player/unified/UnifiedPlayer.vue frontend/web/src/components/player/unified/PlaybackSettingsMenu.vue
git commit -m "feat(unified-player): real quality ladder from hls.js levels + mp4 multi-quality (closes D-05)"
git push origin HEAD:feat/ds-primitives-lucide
```

---

### Task 7: Intro/outro skip via the existing AniSkip stack

Backend route, `animeApi.getSkipTimes`, and `useSkipTimes` already exist (Phase 18, currently consumer-less). This wires them into the unified player.

**Files:**
- Create: `frontend/web/src/composables/unifiedPlayer/skipSegments.ts`
- Test: `frontend/web/src/composables/unifiedPlayer/skipSegments.spec.ts`
- Modify: `frontend/web/src/components/player/unified/overlays/SkipIntroChip.vue` (label prop)
- Modify: `frontend/web/src/components/player/unified/UnifiedPlayer.vue` (malId prop, useSkipTimes, chapters, chip, auto-skip)
- Modify: `frontend/web/src/views/Anime.vue` (pass `:mal-id`)

- [ ] **Step 1: Write the failing tests**

```ts
import { describe, it, expect } from 'vitest'
import { segmentsToChapters, activeSkipSegment } from './skipSegments'

describe('segmentsToChapters', () => {
  it('maps op+ed segments to pct-based chapters', () => {
    expect(
      segmentsToChapters({ start: 60, end: 150 }, { start: 1300, end: 1390 }, 1400),
    ).toEqual([
      { kind: 'intro', startPct: (60 / 1400) * 100, widthPct: (90 / 1400) * 100 },
      { kind: 'outro', startPct: (1300 / 1400) * 100, widthPct: (90 / 1400) * 100 },
    ])
  })

  it('returns [] until the duration is known (no fake markers — F6)', () => {
    expect(segmentsToChapters({ start: 60, end: 150 }, null, 0)).toEqual([])
  })

  it('clamps segments to the duration and drops degenerate ones', () => {
    expect(segmentsToChapters({ start: 1395, end: 1500 }, null, 1400)).toEqual([])
    const [c] = segmentsToChapters({ start: 1300, end: 1500 }, null, 1400)
    expect(c.startPct + c.widthPct).toBeCloseTo(100)
  })

  it('handles null segments', () => {
    expect(segmentsToChapters(null, null, 1400)).toEqual([])
  })
})

describe('activeSkipSegment', () => {
  const op = { start: 60, end: 150 }
  const ed = { start: 1300, end: 1390 }

  it('offers the intro inside the OP window', () => {
    expect(activeSkipSegment(100, op, ed)).toEqual({ kind: 'intro', end: 150 })
  })

  it('offers the outro inside the ED window', () => {
    expect(activeSkipSegment(1320, op, ed)).toEqual({ kind: 'outro', end: 1390 })
  })

  it('offers nothing outside both windows', () => {
    expect(activeSkipSegment(500, op, ed)).toBeNull()
  })

  it('stops offering 1s before the segment end (no ~no-op seeks)', () => {
    expect(activeSkipSegment(149.5, op, ed)).toBeNull()
  })

  it('handles null segments', () => {
    expect(activeSkipSegment(100, null, null)).toBeNull()
  })
})
```

- [ ] **Step 2: Run to verify failure**

Run: `cd /data/animeenigma/frontend/web && bunx vitest run src/composables/unifiedPlayer/skipSegments.spec.ts`
Expected: FAIL — cannot resolve `./skipSegments`.

- [ ] **Step 3: Create `skipSegments.ts`**

```ts
// Pure mapping helpers between AniSkip segments (useSkipTimes) and the
// unified player's scrub-bar chapters / skip-chip state. Kept pure for
// direct unit testing — no Vue reactivity in here.

import type { SkipSegment } from '@/composables/useSkipTimes'

export interface Chapter {
  kind: 'intro' | 'outro'
  startPct: number
  widthPct: number
}

/** Map AniSkip segments to scrub-bar chapter markers. Returns [] until the
 *  real duration is known (no fake markers — design rule F6). */
export function segmentsToChapters(
  opening: SkipSegment | null,
  ending: SkipSegment | null,
  durationSec: number,
): Chapter[] {
  if (!durationSec || durationSec <= 0) return []
  const out: Chapter[] = []
  const push = (kind: Chapter['kind'], seg: SkipSegment) => {
    const start = Math.max(0, Math.min(seg.start, durationSec))
    const end = Math.max(start, Math.min(seg.end, durationSec))
    if (end - start < 1) return
    out.push({
      kind,
      startPct: (start / durationSec) * 100,
      widthPct: ((end - start) / durationSec) * 100,
    })
  }
  if (opening) push('intro', opening)
  if (ending) push('outro', ending)
  return out
}

/** The segment the skip chip should offer at time t, if any. Stops offering
 *  1s before the segment end so the chip never seeks ~nowhere. */
export function activeSkipSegment(
  t: number,
  opening: SkipSegment | null,
  ending: SkipSegment | null,
): { kind: 'intro' | 'outro'; end: number } | null {
  if (opening && t >= opening.start && t < opening.end - 1) {
    return { kind: 'intro', end: opening.end }
  }
  if (ending && t >= ending.start && t < ending.end - 1) {
    return { kind: 'outro', end: ending.end }
  }
  return null
}
```

- [ ] **Step 4: Run to verify pass**

Run: `cd /data/animeenigma/frontend/web && bunx vitest run src/composables/unifiedPlayer/skipSegments.spec.ts`
Expected: PASS.

- [ ] **Step 5: SkipIntroChip gains a label** — replace props + template text:

```vue
<template>
  <button
    v-if="visible"
    class="pl-skip"
    data-test="skip-intro"
    @click="emit('skip')"
  >
    <!-- Forward-skip icon -->
    <FastForward class="size-4" aria-hidden="true" />
    {{ label }}
  </button>
</template>

<script setup lang="ts">
import { FastForward } from 'lucide-vue-next'

withDefaults(
  defineProps<{
    visible: boolean
    label?: string
  }>(),
  { label: 'Skip Intro' },
)

const emit = defineEmits<{
  (e: 'skip'): void
}>()
</script>
```

(scoped style block unchanged)

- [ ] **Step 6: Wire UnifiedPlayer.**

(a) Imports:

```ts
import { useSkipTimes } from '@/composables/useSkipTimes'
import { segmentsToChapters, activeSkipSegment } from '@/composables/unifiedPlayer/skipSegments'
```

(b) Props gain malId:

```ts
const props = defineProps<{
  animeId: string
  anime: { title: string; ep: number; eps: number; still?: string }
  theater: boolean
  isHentai?: boolean
  initialEpisode?: number
  /** Shikimori id (= MAL id) for AniSkip skip-times. Optional — absent ⇒ no skip UI. */
  malId?: string | number
}>()
```

(c) Skip-times section (after the "Next episode logic" section):

```ts
// ─── Intro/outro skip (AniSkip via catalog proxy) ────────────────────────────

const epNumber = computed(() => selectedEpisode.value?.number ?? null)
const malIdRef = computed(() => props.malId ?? null)
const { opening, ending } = useSkipTimes(malIdRef, epNumber)

const chapters = computed(() =>
  segmentsToChapters(opening.value, ending.value, duration.value),
)

const skipTarget = computed(() =>
  activeSkipSegment(currentTime.value, opening.value, ending.value),
)

function onSkipSegment() {
  const v = videoRef.value
  const target = skipTarget.value
  if (!v || !target) return
  v.currentTime = target.end
  writeProgress()
}

// Auto-skip intro (settings toggle) — once per episode view so a manual
// seek back into the OP isn't fought.
let autoSkippedEp: number | null = null
watch(epNumber, () => {
  autoSkippedEp = null
})
watch(currentTime, (t) => {
  if (!state.autoSkip.value) return
  const op = opening.value
  if (!op) return
  const ep = epNumber.value
  if (ep === null || autoSkippedEp === ep) return
  if (t >= op.start && t < op.end - 1) {
    autoSkippedEp = ep
    const v = videoRef.value
    if (v) {
      v.currentTime = op.end
      writeProgress()
    }
  }
})
```

(d) Replace the hardcoded-off chip (delete the `TODO: real skip-timings backend` line):

```html
    <SkipIntroChip
      :visible="!!skipTarget"
      :label="skipTarget?.kind === 'outro' ? 'Skip Outro' : 'Skip Intro'"
      @skip="onSkipSegment"
    />
```

(e) Feed real chapters to the control bar — replace `:chapters="[]"` with:

```html
      :chapters="chapters"
```

- [ ] **Step 7: Pass malId from Anime.vue** — add to the UnifiedPlayer binding:

```html
          :mal-id="anime.shikimoriId"
```

- [ ] **Step 8: Verify**

Run: `cd /data/animeenigma/frontend/web && bunx tsc --noEmit && bunx vitest run src/components/player/unified/ src/composables/unifiedPlayer/`
Expected: PASS.

- [ ] **Step 9: Commit + push**

```bash
cd /data/animeenigma
git add frontend/web/src/composables/unifiedPlayer/skipSegments.ts frontend/web/src/composables/unifiedPlayer/skipSegments.spec.ts frontend/web/src/components/player/unified/overlays/SkipIntroChip.vue frontend/web/src/components/player/unified/UnifiedPlayer.vue frontend/web/src/views/Anime.vue
git commit -m "feat(unified-player): intro/outro skip via existing AniSkip stack (chip + chapters + auto-skip)"
git push origin HEAD:feat/ds-primitives-lucide
```

---

### Task 8: Hacker mode — persisted setting + engine fragment telemetry

**Files:**
- Modify: `frontend/web/src/composables/unifiedPlayer/usePlayerState.ts`
- Modify: `frontend/web/src/composables/unifiedPlayer/usePlayerState.spec.ts`
- Modify: `frontend/web/src/composables/unifiedPlayer/useVideoEngine.ts`

- [ ] **Step 1: Write the failing tests** — append to `usePlayerState.spec.ts`:

```ts
describe('hackerMode', () => {
  beforeEach(() => {
    localStorage.removeItem('pl_hacker_mode')
  })

  it('defaults to off', () => {
    const s = usePlayerState()
    expect(s.hackerMode.value).toBe(false)
  })

  it('persists to localStorage and restores', async () => {
    const s = usePlayerState()
    s.hackerMode.value = true
    await nextTick()
    expect(localStorage.getItem('pl_hacker_mode')).toBe('1')
    const s2 = usePlayerState()
    expect(s2.hackerMode.value).toBe(true)
  })

  it('clears the key when switched off', async () => {
    localStorage.setItem('pl_hacker_mode', '1')
    const s = usePlayerState()
    s.hackerMode.value = false
    await nextTick()
    expect(localStorage.getItem('pl_hacker_mode')).toBeNull()
  })
})
```

Add `import { nextTick } from 'vue'` and `beforeEach` to the spec's imports if missing.

- [ ] **Step 2: Run to verify failure**

Run: `cd /data/animeenigma/frontend/web && bunx vitest run src/composables/unifiedPlayer/usePlayerState.spec.ts`
Expected: FAIL — `hackerMode` undefined.

- [ ] **Step 3: Implement in `usePlayerState.ts`** — change the vue import to `import { ref, watch } from 'vue'`, add before the `return`:

```ts
  // Hacker mode (debug HUD) — persisted, default off.
  const HACKER_KEY = 'pl_hacker_mode'
  const hackerMode = ref(
    typeof localStorage !== 'undefined' && localStorage.getItem(HACKER_KEY) === '1',
  )
  watch(hackerMode, (on) => {
    if (typeof localStorage === 'undefined') return
    if (on) localStorage.setItem(HACKER_KEY, '1')
    else localStorage.removeItem(HACKER_KEY)
  })
```

and add `hackerMode` to the returned object.

- [ ] **Step 4: Run to verify pass**

Run: `cd /data/animeenigma/frontend/web && bunx vitest run src/composables/unifiedPlayer/usePlayerState.spec.ts`
Expected: PASS.

- [ ] **Step 5: Engine fragment telemetry** (`useVideoEngine.ts` — no clean unit seam through the dynamic hls.js import; covered by the in-browser smoke):

(a) Export the stat shape near `QualityLevel`:

```ts
export interface FragStat {
  /** media start position of the fragment (sec) */
  start: number
  /** fragment duration (sec) */
  duration: number
  /** payload size in bytes */
  size: number
  /** wall-clock load time in ms */
  loadMs: number
}
```

(b) State next to `levels`:

```ts
  const fragStats = ref<FragStat[]>([])
  const bandwidthEstimate = ref(0)
```

(c) Reset in `load()` next to the levels reset:

```ts
    fragStats.value = []
    bandwidthEstimate.value = 0
```

(d) Listener after the `LEVEL_SWITCHED` handler:

```ts
    hls.on(Hls.Events.FRAG_LOADED, (_e: unknown, data: any) => {
      const f = data?.frag
      const st = f?.stats
      if (!f || !st) return
      const loadMs = Math.max(0, (st.loading?.end ?? 0) - (st.loading?.start ?? 0))
      // Rolling window of the last 30 fragments — enough for the HUD +
      // scrub-bar heatmap without unbounded growth on long episodes.
      fragStats.value = [
        ...fragStats.value.slice(-29),
        { start: f.start ?? 0, duration: f.duration ?? 0, size: st.total ?? 0, loadMs },
      ]
      bandwidthEstimate.value = hls?.bandwidthEstimate ?? 0
    })
```

(e) Extend the return:

```ts
  return { fatal, load, destroy, levels, currentLevelLabel, setLevel, fragStats, bandwidthEstimate }
```

- [ ] **Step 6: Verify**

Run: `cd /data/animeenigma/frontend/web && bunx tsc --noEmit && bunx vitest run src/composables/unifiedPlayer/`
Expected: PASS.

- [ ] **Step 7: Commit + push**

```bash
cd /data/animeenigma
git add frontend/web/src/composables/unifiedPlayer/usePlayerState.ts frontend/web/src/composables/unifiedPlayer/usePlayerState.spec.ts frontend/web/src/composables/unifiedPlayer/useVideoEngine.ts
git commit -m "feat(unified-player): persisted hacker-mode setting + hls fragment telemetry"
git push origin HEAD:feat/ds-primitives-lucide
```

---

### Task 9: usePlaybackStats composable

**Files:**
- Create: `frontend/web/src/composables/unifiedPlayer/usePlaybackStats.ts`
- Test: `frontend/web/src/composables/unifiedPlayer/usePlaybackStats.spec.ts`

- [ ] **Step 1: Write the failing test**

```ts
import { describe, it, expect } from 'vitest'
import { ref } from 'vue'
import { usePlaybackStats } from './usePlaybackStats'

// Minimal fake <video> — only what sample() reads.
function fakeVideo(over: Record<string, unknown> = {}) {
  return {
    currentTime: 50,
    readyState: 4,
    videoWidth: 1280,
    videoHeight: 720,
    buffered: { length: 1, start: () => 40, end: () => 95 },
    getVideoPlaybackQuality: () => ({ droppedVideoFrames: 3, totalVideoFrames: 1200 }),
    ...over,
  } as unknown as HTMLVideoElement
}

describe('usePlaybackStats', () => {
  it('samples buffer ahead/behind around currentTime', () => {
    const { stats, sample } = usePlaybackStats(ref(fakeVideo()), ref(false))
    sample()
    expect(stats.value.bufferAheadSec).toBe(45)
    expect(stats.value.bufferBehindSec).toBe(10)
    expect(stats.value.readyState).toBe(4)
    expect(stats.value.resolution).toBe('1280×720')
    expect(stats.value.droppedFrames).toBe(3)
    expect(stats.value.totalFrames).toBe(1200)
  })

  it('reports zeros when currentTime is outside every buffered range', () => {
    const v = fakeVideo({ currentTime: 200 })
    const { stats, sample } = usePlaybackStats(ref(v), ref(false))
    sample()
    expect(stats.value.bufferAheadSec).toBe(0)
    expect(stats.value.bufferBehindSec).toBe(0)
  })

  it('degrades when getVideoPlaybackQuality is unavailable', () => {
    const v = fakeVideo({ getVideoPlaybackQuality: undefined })
    const { stats, sample } = usePlaybackStats(ref(v), ref(false))
    sample()
    expect(stats.value.droppedFrames).toBe(0)
  })

  it('reports empty stats with no video element', () => {
    const { stats, sample } = usePlaybackStats(ref(null), ref(false))
    sample()
    expect(stats.value.readyState).toBe(0)
    expect(stats.value.resolution).toBe('')
  })
})
```

- [ ] **Step 2: Run to verify failure**

Run: `cd /data/animeenigma/frontend/web && bunx vitest run src/composables/unifiedPlayer/usePlaybackStats.spec.ts`
Expected: FAIL — cannot resolve `./usePlaybackStats`.

- [ ] **Step 3: Create the composable**

```ts
// Samples <video>-element debug stats (buffer window, readyState, dropped
// frames, resolution) on a 500ms interval while `enabled` — feeds the
// hacker-mode DebugHud and the settings-menu mini-stats.

import { ref, watch, onUnmounted, type Ref } from 'vue'

export interface PlaybackStats {
  readyState: number
  bufferAheadSec: number
  bufferBehindSec: number
  droppedFrames: number
  totalFrames: number
  /** e.g. "1920×1080"; empty until the first frame is decoded */
  resolution: string
}

const EMPTY: PlaybackStats = {
  readyState: 0,
  bufferAheadSec: 0,
  bufferBehindSec: 0,
  droppedFrames: 0,
  totalFrames: 0,
  resolution: '',
}

export function usePlaybackStats(
  videoEl: Ref<HTMLVideoElement | null>,
  enabled: Ref<boolean>,
) {
  const stats = ref<PlaybackStats>({ ...EMPTY })
  let timer: ReturnType<typeof setInterval> | null = null

  function sample() {
    const v = videoEl.value
    if (!v) {
      stats.value = { ...EMPTY }
      return
    }
    const t = v.currentTime
    let ahead = 0
    let behind = 0
    for (let i = 0; i < v.buffered.length; i++) {
      const s = v.buffered.start(i)
      const e = v.buffered.end(i)
      if (t >= s && t <= e) {
        ahead = e - t
        behind = t - s
        break
      }
    }
    const q =
      typeof v.getVideoPlaybackQuality === 'function' ? v.getVideoPlaybackQuality() : null
    stats.value = {
      readyState: v.readyState,
      bufferAheadSec: ahead,
      bufferBehindSec: behind,
      droppedFrames: q?.droppedVideoFrames ?? 0,
      totalFrames: q?.totalVideoFrames ?? 0,
      resolution: v.videoWidth ? `${v.videoWidth}×${v.videoHeight}` : '',
    }
  }

  watch(
    enabled,
    (on) => {
      if (on && timer === null) {
        sample()
        timer = setInterval(sample, 500)
      }
      if (!on && timer !== null) {
        clearInterval(timer)
        timer = null
      }
    },
    { immediate: true },
  )

  onUnmounted(() => {
    if (timer !== null) clearInterval(timer)
  })

  return { stats, sample }
}
```

- [ ] **Step 4: Run to verify pass**

Run: `cd /data/animeenigma/frontend/web && bunx vitest run src/composables/unifiedPlayer/usePlaybackStats.spec.ts`
Expected: PASS.

- [ ] **Step 5: Commit + push**

```bash
cd /data/animeenigma
git add frontend/web/src/composables/unifiedPlayer/usePlaybackStats.ts frontend/web/src/composables/unifiedPlayer/usePlaybackStats.spec.ts
git commit -m "feat(unified-player): usePlaybackStats sampler (buffer window, frames, resolution)"
git push origin HEAD:feat/ds-primitives-lucide
```

---

### Task 10: DebugHud overlay + scrub-bar fragment heatmap

**Files:**
- Create: `frontend/web/src/components/player/unified/overlays/DebugHud.vue`
- Test: `frontend/web/src/components/player/unified/overlays/DebugHud.spec.ts`
- Modify: `frontend/web/src/components/player/unified/PlayerScrubBar.vue` (fragments layer)
- Modify: `frontend/web/src/components/player/unified/PlayerScrubBar.spec.ts`
- Modify: `frontend/web/src/components/player/unified/PlayerControlBar.vue` (fragments passthrough)

- [ ] **Step 1: Write the failing DebugHud test**

```ts
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import DebugHud from './DebugHud.vue'

const baseProps = {
  stats: {
    readyState: 4,
    bufferAheadSec: 42.31,
    bufferBehindSec: 12.18,
    droppedFrames: 3,
    totalFrames: 28741,
    resolution: '1920×1080',
  },
  frags: [
    { start: 0, duration: 6, size: 512 * 1024, loadMs: 230 },
    { start: 6, duration: 6, size: 2 * 1024 * 1024, loadMs: 800 },
  ],
  bandwidth: 4_200_000,
  provider: 'Gogoanime',
  streamType: 'hls',
  levelLabel: '1080p',
}

describe('DebugHud', () => {
  it('renders provider, type and level in the header row', () => {
    const w = mount(DebugHud, { props: baseProps })
    const head = w.find('.pl-hud-head').text()
    expect(head).toContain('Gogoanime')
    expect(head).toContain('hls')
    expect(head).toContain('1080p')
  })

  it('formats bandwidth in Mbit/s and the buffer window', () => {
    const w = mount(DebugHud, { props: baseProps })
    expect(w.text()).toContain('4.2 Mbit/s')
    expect(w.text()).toContain('+42.3s')
    expect(w.text()).toContain('12.2s')
  })

  it('lists recent fragments with sizes (KB/MB) and load times', () => {
    const w = mount(DebugHud, { props: baseProps })
    expect(w.text()).toContain('512KB')
    expect(w.text()).toContain('2.0MB')
    expect(w.text()).toContain('230ms')
  })

  it('shows a placeholder instead of fragments for mp4', () => {
    const w = mount(DebugHud, { props: { ...baseProps, frags: [], streamType: 'mp4' } })
    expect(w.text()).toContain('no fragments')
  })
})
```

- [ ] **Step 2: Run to verify failure**

Run: `cd /data/animeenigma/frontend/web && bunx vitest run src/components/player/unified/overlays/DebugHud.spec.ts`
Expected: FAIL — cannot resolve `./DebugHud.vue`.

- [ ] **Step 3: Create DebugHud.vue**

```vue
<template>
  <div class="pl-hud" data-test="debug-hud" aria-hidden="true">
    <div class="pl-hud-row pl-hud-head">
      {{ provider }} · {{ streamType }}<template v-if="levelLabel"> · {{ levelLabel }}</template>
    </div>
    <div class="pl-hud-row">BW   {{ bw }}</div>
    <div class="pl-hud-row">
      BUF  +{{ stats.bufferAheadSec.toFixed(1) }}s / −{{ stats.bufferBehindSec.toFixed(1) }}s · rs={{ stats.readyState }}
    </div>
    <div class="pl-hud-row">
      RES  {{ stats.resolution || '—' }} · drop {{ stats.droppedFrames }}/{{ stats.totalFrames }}
    </div>
    <template v-if="frags.length">
      <div v-for="(f, i) in lastFrags" :key="i" class="pl-hud-row">
        FRAG {{ fmtSize(f.size) }} · {{ Math.round(f.loadMs) }}ms · {{ f.duration.toFixed(1) }}s @{{ Math.round(f.start) }}s
      </div>
    </template>
    <div v-else class="pl-hud-row pl-hud-dim">no fragments (mp4 / not loaded)</div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { PlaybackStats } from '@/composables/unifiedPlayer/usePlaybackStats'
import type { FragStat } from '@/composables/unifiedPlayer/useVideoEngine'

const props = defineProps<{
  stats: PlaybackStats
  frags: FragStat[]
  /** hls.js bandwidthEstimate, bits/s; 0 = unknown */
  bandwidth: number
  provider: string
  streamType: string
  levelLabel: string
}>()

const lastFrags = computed(() => props.frags.slice(-5).reverse())

const bw = computed(() =>
  props.bandwidth > 0 ? `${(props.bandwidth / 1_000_000).toFixed(1)} Mbit/s` : '—',
)

function fmtSize(bytes: number): string {
  if (bytes >= 1_048_576) return `${(bytes / 1_048_576).toFixed(1)}MB`
  return `${Math.round(bytes / 1024)}KB`
}
</script>

<style scoped>
.pl-hud {
  position: absolute;
  top: 76px;
  left: 14px;
  z-index: 5;
  padding: 10px 12px;
  border-radius: var(--r-md, 8px);
  background: rgba(0, 0, 0, 0.78);
  border: 1px solid var(--border);
  font-family: var(--font-mono, monospace);
  font-size: 11px;
  line-height: 1.7;
  color: var(--success);
  pointer-events: none;
  white-space: pre;
  max-width: min(420px, calc(100% - 28px));
  overflow: hidden;
}

.pl-hud-head {
  color: var(--brand-cyan);
}

.pl-hud-dim {
  opacity: 0.6;
}
</style>
```

- [ ] **Step 4: Run to verify pass**

Run: `cd /data/animeenigma/frontend/web && bunx vitest run src/components/player/unified/overlays/DebugHud.spec.ts`
Expected: PASS.

- [ ] **Step 5: Scrub-bar fragment heatmap.** Write the failing test — append to `PlayerScrubBar.spec.ts`:

```ts
  it('renders hacker-mode fragment segments with tones and labels', () => {
    const w = mount(PlayerScrubBar, {
      props: {
        progress: 10,
        buffered: 30,
        durationSec: 1400,
        chapters: [],
        fragments: [
          { startPct: 0, widthPct: 2, tone: 'ok' as const, label: '300 KB · 120 ms' },
          { startPct: 2, widthPct: 2, tone: 'bad' as const, label: '2048 KB · 900 ms' },
        ],
      },
    })
    const frags = w.findAll('[data-test="frag"]')
    expect(frags.length).toBe(2)
    expect(frags[0].attributes('data-tone')).toBe('ok')
    expect(frags[1].attributes('data-tone')).toBe('bad')
    expect(frags[0].attributes('title')).toBe('300 KB · 120 ms')
  })

  it('renders no fragment layer by default', () => {
    const w = mount(PlayerScrubBar, {
      props: { progress: 10, buffered: 30, durationSec: 1400, chapters: [] },
    })
    expect(w.findAll('[data-test="frag"]').length).toBe(0)
  })
```

Run: `cd /data/animeenigma/frontend/web && bunx vitest run src/components/player/unified/PlayerScrubBar.spec.ts`
Expected: FAIL (0 frag elements found in the first test).

- [ ] **Step 6: Implement in PlayerScrubBar.vue.**

(a) Add the exported type + prop. Replace the props block:

```ts
export interface FragSegment {
  startPct: number
  widthPct: number
  tone: 'ok' | 'warn' | 'bad'
  label: string
}
```

(note: `export interface` inside `<script setup>` is hoisted by vue/tsc to the component module — declare it in a normal `<script lang="ts">` block above `<script setup>` if vue-tsc complains; the props line itself:)

```ts
const props = defineProps<{
  progress: number
  buffered: number
  durationSec: number
  chapters: Chapter[]
  stillUrl?: string
  /** hacker-mode per-fragment heatmap segments (empty/omitted = off) */
  fragments?: FragSegment[]
}>()
```

(b) Template — add AFTER the playback-fill div (so the heatmap reads on top in hacker mode), before the chapter markers:

```html
    <!-- Hacker-mode fragment heatmap -->
    <span
      v-for="(f, i) in fragments ?? []"
      :key="'frag' + i"
      data-test="frag"
      class="pl-frag"
      :data-tone="f.tone"
      :style="{ left: f.startPct + '%', width: f.widthPct + '%' }"
      :title="f.label"
    />
```

(c) Scoped CSS (semantic tokens only — DS lint clean):

```css
.pl-frag {
  position: absolute;
  height: 4px;
  opacity: 0.85;
  border-right: 1px solid rgba(0, 0, 0, 0.6);
}

.pl-track:hover .pl-frag {
  height: 6px;
}

.pl-frag[data-tone='ok'] {
  background: var(--success);
}

.pl-frag[data-tone='warn'] {
  background: var(--warning);
}

.pl-frag[data-tone='bad'] {
  background: var(--destructive);
}
```

(Clicks still seek: the track's click handler reads `event.currentTarget`, so child spans don't break `getPct`.)

- [ ] **Step 7: PlayerControlBar passthrough.** Add to its props interface:

```ts
    /** hacker-mode fragment heatmap, forwarded to the scrub bar */
    fragments?: { startPct: number; widthPct: number; tone: 'ok' | 'warn' | 'bad'; label: string }[]
```

and forward on the `<PlayerScrubBar>` element:

```html
      <PlayerScrubBar
        :progress="progress"
        :buffered="buffered"
        :duration-sec="duration"
        :chapters="chapters"
        :still-url="stillUrl"
        :fragments="fragments"
        @seek="emit('seek', $event)"
      />
```

- [ ] **Step 8: Verify**

Run: `cd /data/animeenigma/frontend/web && bunx tsc --noEmit && bunx vitest run src/components/player/unified/`
Expected: PASS.

- [ ] **Step 9: Commit + push**

```bash
cd /data/animeenigma
git add frontend/web/src/components/player/unified/overlays/DebugHud.vue frontend/web/src/components/player/unified/overlays/DebugHud.spec.ts frontend/web/src/components/player/unified/PlayerScrubBar.vue frontend/web/src/components/player/unified/PlayerScrubBar.spec.ts frontend/web/src/components/player/unified/PlayerControlBar.vue
git commit -m "feat(unified-player): DebugHud overlay + scrub-bar fragment heatmap (hacker mode)"
git push origin HEAD:feat/ds-primitives-lucide
```

---

### Task 11: Hacker-mode glue — settings toggle, menu mini-stats, UnifiedPlayer wiring

**Files:**
- Modify: `frontend/web/src/components/player/unified/PlaybackSettingsMenu.vue`
- Modify: `frontend/web/src/components/player/unified/UnifiedPlayer.vue`

- [ ] **Step 1: PlaybackSettingsMenu — toggle + mini-stats.**

(a) Extend imports: add `Terminal` to the lucide import list.

(b) Props gain (after `autoSkip: boolean`):

```ts
  hackerMode: boolean
  /** compact live debug numbers; null hides the section */
  debugStats?: { bw: string; buffer: string; level: string; frag: string } | null
```

(c) Emits gain:

```ts
  (e: 'update:hackerMode', value: boolean): void
```

(d) Root menu — append after the Auto-skip row (still inside the root `<template v-else>`):

```html
      <!-- Hacker mode toggle -->
      <div class="flex items-center gap-[10px] px-[10px] py-[9px]">
        <Terminal class="size-4 flex-shrink-0 text-[var(--ink-2)]" aria-hidden="true" />
        <span class="flex-1 text-[14px] text-[var(--ink-2)]">Hacker mode</span>
        <Switch
          :model-value="hackerMode"
          @update:model-value="emit('update:hackerMode', $event)"
        />
      </div>

      <!-- Live debug mini-stats (hacker mode only) -->
      <template v-if="hackerMode && debugStats">
        <div class="h-px mx-1 my-[6px]" style="background: var(--border);"/>
        <div class="px-[10px] pb-[8px] font-mono text-[11px] leading-[1.8] text-[var(--success)]" data-test="debug-stats">
          <div>BW   {{ debugStats.bw }}</div>
          <div>BUF  {{ debugStats.buffer }}</div>
          <div>LVL  {{ debugStats.level }}</div>
          <div>FRAG {{ debugStats.frag }}</div>
        </div>
      </template>
```

- [ ] **Step 2: UnifiedPlayer glue.**

(a) Imports:

```ts
import DebugHud from './overlays/DebugHud.vue'
import { usePlaybackStats } from '@/composables/unifiedPlayer/usePlaybackStats'
```

(b) Hacker-mode section (after the buffering section from Task 3):

```ts
// ─── Hacker mode (debug HUD) ──────────────────────────────────────────────────

const statsEnabled = computed(() => state.hackerMode.value)
const { stats: playbackStats } = usePlaybackStats(videoRef, statsEnabled)

// HUD shows while paused or while actively buffering/seeking.
const hudVisible = computed(
  () => state.hackerMode.value && (!state.playing.value || isBuffering.value),
)

// Scrub-bar heatmap segments — size-tinted (green <300KB, amber <1MB, red ≥1MB).
const fragOverlay = computed(() => {
  if (!state.hackerMode.value) return []
  const dur = duration.value
  if (!dur) return []
  return engine.fragStats.value.map((f) => ({
    startPct: (f.start / dur) * 100,
    widthPct: (f.duration / dur) * 100,
    tone: (f.size < 300_000 ? 'ok' : f.size < 1_000_000 ? 'warn' : 'bad') as 'ok' | 'warn' | 'bad',
    label: `${Math.round(f.size / 1024)} KB · ${Math.round(f.loadMs)} ms`,
  }))
})

// Compact line set for the settings-menu mini-stats section.
const debugStats = computed(() => {
  if (!state.hackerMode.value) return null
  const bwv = engine.bandwidthEstimate.value
  const frs = engine.fragStats.value
  const last = frs[frs.length - 1]
  return {
    bw: bwv > 0 ? `${(bwv / 1_000_000).toFixed(1)} Mbit/s` : '—',
    buffer: `+${playbackStats.value.bufferAheadSec.toFixed(1)}s / −${playbackStats.value.bufferBehindSec.toFixed(1)}s`,
    level:
      engine.currentLevelLabel.value ||
      (currentStream.value?.type === 'mp4' ? 'mp4' : '—'),
    frag: last ? `${Math.round(last.size / 1024)} KB · ${Math.round(last.loadMs)} ms` : '—',
  }
})
```

(c) Mount the HUD after `<BufferingOverlay …/>`:

```html
    <DebugHud
      v-if="hudVisible"
      :stats="playbackStats"
      :frags="engine.fragStats.value"
      :bandwidth="engine.bandwidthEstimate.value"
      :provider="activeProviderName"
      :stream-type="currentStream?.type ?? '—'"
      :level-label="engine.currentLevelLabel.value"
    />
```

(d) Forward the heatmap through the control bar — add to the `<PlayerControlBar>` binding:

```html
      :fragments="fragOverlay"
```

(e) Extend the settings-menu binding with:

```html
        :hacker-mode="state.hackerMode.value"
        :debug-stats="debugStats"
        @update:hacker-mode="v => { state.hackerMode.value = v }"
```

- [ ] **Step 3: Verify**

Run: `cd /data/animeenigma/frontend/web && bunx tsc --noEmit && bunx vitest run src/components/player/unified/ src/composables/unifiedPlayer/`
Expected: PASS.

- [ ] **Step 4: Commit + push**

```bash
cd /data/animeenigma
git add frontend/web/src/components/player/unified/PlaybackSettingsMenu.vue frontend/web/src/components/player/unified/UnifiedPlayer.vue
git commit -m "feat(unified-player): hacker mode — settings toggle, debug HUD, menu mini-stats"
git push origin HEAD:feat/ds-primitives-lucide
```

---

### Task 12: Full verification gates + in-browser smoke + ship

- [ ] **Step 1: Full frontend gates**

```bash
cd /data/animeenigma/frontend/web
bunx vitest run src/components/player/ src/composables/unifiedPlayer/
bunx tsc --noEmit
bunx eslint src/components/player/unified/ src/composables/unifiedPlayer/
bash scripts/design-system-lint.sh
```
Expected: all green; DS lint `ERRORS=0`.

- [ ] **Step 2: In-browser smoke (DS-NF-06 — MANDATORY).** Deploy first (`make redeploy-web`), then on `https://animeenigma.ru/anime/f0b40660-6627-4a59-8dcf-7ec8596b3623` (Frieren) with the `AnimeEnigma · Бета` player active, desktop + ≤680px:
  1. Arrow-key seek → spinner appears only when buffering (no flash on in-buffer seeks).
  2. ±5 buttons: digit optically centered in both.
  3. Hamburger (top-right) opens the episodes drawer; selecting an episode re-resolves; works in fullscreen.
  4. Settings → Hacker mode ON → HUD on pause, heatmap on scrub bar, mini-stats in gear menu; survives reload (localStorage).
  5. Settings → Quality lists real levels on a multi-variant HLS stream ("Auto · NNNp" while auto).
  6. Skip chip appears during Frieren's OP (AniSkip has data); chapter markers visible; Auto-skip toggle jumps the OP once.
  7. Known caveat: providers affected by D-07 (fragment stall) will show an endless spinner — that's the pre-existing bug becoming visible, not a regression.

- [ ] **Step 3: Run `/animeenigma-after-update`** (skill): lint/build, `make redeploy-web`, health check, Russian Trump-mode changelog entry in `frontend/web/public/changelog.json`, commit + push.

- [ ] **Step 4: Final report must flag:** all work landed on `feat/ds-primitives-lucide` (with today's fleet work); the branch needs a merge to `main`.
