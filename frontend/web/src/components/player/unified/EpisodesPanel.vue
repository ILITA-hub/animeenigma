<template>
  <div class="flex flex-col" data-test="episodes-panel">
    <!-- Sheet head: count, view toggle (100+), next-unwatched, jump (16+) -->
    <div class="px-3 pt-2.5 pb-2 flex items-center gap-2">
      <span class="text-[13px] font-semibold text-white">Episodes</span>
      <span class="text-[11px] text-[var(--muted-foreground)]">{{ episodes.length }}</span>

      <div v-if="showGridToggle" class="ep-vt" role="group" aria-label="Episode list view">
        <button
          type="button"
          :class="{ 'ep-vt--on': view === 'strip' }"
          aria-label="Strip view"
          title="Strip"
          data-test="view-strip"
          @click="setView('strip')"
        >
          <GalleryHorizontal :size="13" aria-hidden="true" />
        </button>
        <button
          type="button"
          :class="{ 'ep-vt--on': view === 'grid' }"
          aria-label="Grid view"
          title="Grid"
          data-test="view-grid"
          @click="setView('grid')"
        >
          <LayoutGrid :size="13" aria-hidden="true" />
        </button>
      </div>

      <span class="flex-1" />

      <button
        v-if="showJump && nextUnwatched"
        type="button"
        class="ep-chip"
        data-test="next-unwatched"
        @click="scrollToEp(nextUnwatched.number, true)"
      >
        → next unwatched: {{ nextUnwatched.number }}
      </button>
      <input
        v-if="showJump"
        v-model="jumpVal"
        class="ep-jump"
        type="text"
        inputmode="numeric"
        placeholder="Jump to ep… ⏎"
        aria-label="Jump to episode"
        data-test="jump-input"
        @keydown.enter.prevent="onJump"
        @keydown.stop
      >
    </div>

    <div v-if="episodes.length === 0" class="px-3 pb-3 text-[13px] text-[var(--muted-foreground)]">
      No episodes from this source
    </div>

    <!-- Strip view: horizontal episode cards (titles + user data visible) -->
    <div
      v-else-if="view === 'strip'"
      ref="stripRef"
      class="ep-strip"
      data-test="ep-strip"
    >
      <button
        v-for="ep in episodes"
        :key="ep.key"
        type="button"
        :class="[
          'ep-card',
          ep.number === selectedNumber ? 'ep-card--sel text-[var(--brand-cyan)]' : '',
          ep.isFiller ? 'opacity-50' : '',
        ]"
        :title="ep.title ? `${ep.number}. ${ep.title}` : undefined"
        :data-ep="ep.number"
        :data-test="`episode-${ep.number}`"
        @click="emit('select', ep)"
      >
        <span class="ep-card-n">
          <span>EP {{ ep.label }}</span>
          <Check
            v-if="isWatched(ep)"
            :size="10"
            :stroke-width="3"
            class="ep-card-check"
            aria-hidden="true"
            data-test="ep-watched"
          />
        </span>
        <span class="ep-card-t">{{ ep.title || '\u00a0' }}</span>
        <span
          v-if="partialPct(ep) > 0"
          class="ep-progress"
          :style="{ width: partialPct(ep) + '%' }"
          aria-hidden="true"
          data-test="ep-progress"
        />
      </button>
    </div>

    <!-- Grid view (100+): dense archive navigation, vertical scroll -->
    <div v-else ref="gridRef" class="ep-sheetgrid" data-test="ep-grid">
      <button
        v-for="ep in episodes"
        :key="ep.key"
        type="button"
        :class="[
          'ep-cell relative rounded-[var(--r-sm)] border-0 text-[12px] font-semibold transition-colors cursor-pointer overflow-hidden',
          ep.number === selectedNumber
            ? 'text-[var(--brand-cyan)]'
            : isWatched(ep)
              ? 'text-[var(--muted-foreground)] hover:bg-white/[0.14] hover:text-white'
              : 'text-[var(--ink-2)] hover:bg-white/[0.14] hover:text-white',
          ep.isFiller ? 'opacity-50' : '',
        ]"
        :style="ep.number === selectedNumber
          ? 'background: rgba(0,212,255,0.18)'
          : 'background: rgba(255,255,255,0.07)'"
        :title="ep.title ? `${ep.number}. ${ep.title}` : undefined"
        :data-ep="ep.number"
        :data-test="`episode-grid-${ep.number}`"
        @click="emit('select', ep)"
      >
        {{ ep.label }}
        <Check
          v-if="isWatched(ep)"
          class="ep-check"
          :size="10"
          :stroke-width="3"
          aria-hidden="true"
        />
        <span
          v-if="partialPct(ep) > 0"
          class="ep-progress"
          :style="{ width: partialPct(ep) + '%' }"
          aria-hidden="true"
        />
      </button>
    </div>

    <!-- Manual mark-as-watched for the CURRENT episode (Kodik parity) -->
    <div
      v-if="canMark && selectedNumber !== null"
      class="px-3 pb-3 pt-1 border-t border-[var(--border)]"
    >
      <button
        class="ep-mark w-full h-8 rounded-[var(--r-sm)] border-0 inline-flex items-center justify-center gap-2 text-[12px] font-semibold transition-colors"
        :class="marked ? 'ep-mark--done' : 'cursor-pointer'"
        :disabled="marked || marking"
        data-test="mark-watched"
        @click="emit('mark-watched')"
      >
        <Check :size="12" :stroke-width="3" aria-hidden="true" />
        <span v-if="marked">Ep. {{ selectedNumber }} watched</span>
        <span v-else-if="marking">Marking…</span>
        <span v-else>Mark ep. {{ selectedNumber }} as watched</span>
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, nextTick, onMounted } from 'vue'
import { Check, LayoutGrid, GalleryHorizontal } from 'lucide-vue-next'
import type { EpisodeOption } from '@/components/player/EpisodeSelector.types'

export interface EpisodeUserProgress {
  /** 0..1 fraction of the episode watched */
  pct: number
  /** saved playhead position in seconds */
  sec?: number
  completed: boolean
}

const props = withDefaults(
  defineProps<{
    episodes: EpisodeOption[]
    selectedNumber: number | null
    /** episodes with number <= this are watched (anime-list user data) */
    watchedUpTo?: number
    /** per-episode watch progress keyed by episode number (user data) */
    progress?: Record<number, EpisodeUserProgress>
    /** logged-in gate for the manual mark-as-watched action */
    canMark?: boolean
    /** mark request in flight */
    marking?: boolean
    /** current episode already watched — disables the action */
    marked?: boolean
  }>(),
  { watchedUpTo: 0, progress: () => ({}), canMark: false, marking: false, marked: false },
)

const emit = defineEmits<{
  (e: 'select', ep: EpisodeOption): void
  (e: 'mark-watched'): void
}>()

// ── V2b adaptive rules ───────────────────────────────────────────────────────
// ≤15 eps: strip only. 16+: jump input appears. 100+: grid toggle appears.

const view = ref<'strip' | 'grid'>('strip')
const showJump = computed(() => props.episodes.length > 15)
const showGridToggle = computed(() => props.episodes.length >= 100)

const stripRef = ref<HTMLElement | null>(null)
const gridRef = ref<HTMLElement | null>(null)
const jumpVal = ref('')

function isWatched(ep: EpisodeOption): boolean {
  return ep.number <= props.watchedUpTo || !!props.progress[ep.number]?.completed
}

/** Partial progress fraction (0..100); 0 for watched/untouched episodes. */
function partialPct(ep: EpisodeOption): number {
  if (isWatched(ep)) return 0
  const p = props.progress[ep.number]
  if (!p || p.pct <= 0) return 0
  return Math.min(100, Math.round(p.pct * 100))
}

const nextUnwatched = computed(() => {
  const ep = props.episodes.find((e) => !isWatched(e))
  if (!ep || ep.number === props.selectedNumber) return null
  return ep
})

function setView(v: 'strip' | 'grid') {
  if (view.value === v) return
  view.value = v
  // Re-center the current episode when coming back to the strip.
  if (v === 'strip') void nextTick(() => scrollToEp(props.selectedNumber ?? 0, false))
}

/** Scroll the strip/grid to episode n. Jump never SELECTS — navigation only. */
function scrollToEp(n: number, flash: boolean) {
  const host = view.value === 'strip' ? stripRef.value : gridRef.value
  const el = host?.querySelector<HTMLElement>(`[data-ep="${n}"]`)
  if (!el) return
  el.scrollIntoView?.({
    inline: 'center',
    block: 'nearest',
    behavior: flash ? 'smooth' : 'auto',
  })
  if (flash) {
    el.classList.remove('ep-flash')
    void el.offsetWidth // restart the animation
    el.classList.add('ep-flash')
  }
}

function onJump() {
  const n = parseInt(jumpVal.value, 10)
  if (!Number.isFinite(n)) return
  const nums = props.episodes.map((e) => e.number)
  if (nums.length === 0) return
  const clamped = Math.max(Math.min(...nums), Math.min(Math.max(...nums), n))
  scrollToEp(clamped, true)
}

// The sheet mounts fresh on every open (parent v-if) — center the current
// episode so neighbours are immediately visible.
onMounted(() => {
  void nextTick(() => {
    if (props.selectedNumber !== null) scrollToEp(props.selectedNumber, false)
  })
})
</script>

<style scoped>
/* ── head controls ── */
.ep-vt {
  display: inline-flex;
  border-radius: var(--r-sm);
  overflow: hidden;
  border: 1px solid var(--border);
  margin-left: 4px;
}

.ep-vt button {
  width: 28px;
  height: 24px;
  display: grid;
  place-items: center;
  border: 0;
  background: rgba(255, 255, 255, 0.04);
  color: var(--muted-foreground);
  cursor: pointer;
  transition: background 0.15s, color 0.15s;
}

.ep-vt button:hover {
  color: white;
}

.ep-vt .ep-vt--on {
  background: rgba(0, 212, 255, 0.16);
  color: var(--brand-cyan);
}

.ep-chip {
  height: 26px;
  padding: 0 10px;
  border-radius: var(--r-sm);
  border: 0;
  background: rgba(0, 212, 255, 0.12);
  color: var(--brand-cyan);
  font-size: 11.5px;
  font-weight: 600;
  cursor: pointer;
  white-space: nowrap;
  transition: background 0.15s;
}

.ep-chip:hover {
  background: rgba(0, 212, 255, 0.2);
}

.ep-jump {
  width: 112px;
  height: 28px;
  border-radius: var(--r-sm);
  border: 1px solid var(--border);
  background: rgba(255, 255, 255, 0.05);
  color: white;
  padding: 0 10px;
  font-size: 12px;
}

.ep-jump::placeholder {
  color: var(--muted-foreground);
}

.ep-jump:focus {
  outline: none;
  border-color: rgba(0, 212, 255, 0.5);
}

/* ── strip view ── */
.ep-strip {
  display: flex;
  gap: 8px;
  overflow-x: auto;
  padding: 2px 12px 12px;
  scroll-snap-type: x proximity;
  scrollbar-width: thin;
}

.ep-card {
  scroll-snap-align: center;
  flex: 0 0 158px;
  border-radius: 10px;
  border: 1px solid var(--border);
  background: rgba(255, 255, 255, 0.05);
  padding: 8px 11px 9px;
  cursor: pointer;
  position: relative;
  overflow: hidden;
  text-align: left;
  transition: background 0.15s, border-color 0.15s;
}

.ep-card:hover {
  background: rgba(255, 255, 255, 0.11);
}

.ep-card--sel {
  border-color: rgba(0, 212, 255, 0.6);
  background: rgba(0, 212, 255, 0.1);
}

.ep-card-n {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 13.5px;
  font-weight: 600;
  color: white;
}

.ep-card--sel .ep-card-n {
  color: var(--brand-cyan);
}

.ep-card-check {
  color: var(--brand-cyan);
  opacity: 0.85;
  flex-shrink: 0;
}

.ep-card-t {
  display: block;
  font-size: 11px;
  color: var(--muted-foreground);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  margin-top: 1px;
}

/* ── grid view (archive) ── */
.ep-sheetgrid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(46px, 1fr));
  gap: 5px;
  padding: 2px 12px 12px;
  max-height: 168px;
  overflow-y: auto;
  scrollbar-width: thin;
}

.ep-sheetgrid .ep-cell {
  height: 32px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
}

/* shared user-data primitives */
.ep-check {
  position: absolute;
  top: 3px;
  right: 3px;
  color: var(--brand-cyan);
  opacity: 0.85;
}

.ep-progress {
  position: absolute;
  left: 0;
  bottom: 0;
  height: 2px;
  background: var(--brand-cyan);
  box-shadow: 0 0 4px var(--brand-cyan);
}

/* jump-target highlight */
.ep-flash {
  animation: ep-flash 1.2s ease 1;
}

@keyframes ep-flash {
  0%,
  60% {
    border-color: var(--brand-cyan);
    box-shadow: 0 0 14px rgba(0, 212, 255, 0.45);
  }
  100% {
    box-shadow: none;
  }
}

/* mark-as-watched footer */
.ep-mark {
  background: rgba(255, 255, 255, 0.07);
  color: var(--ink-2);
}

.ep-mark:not(:disabled):hover {
  background: rgba(255, 255, 255, 0.14);
  color: white;
}

.ep-mark--done {
  color: var(--brand-cyan);
  background: rgba(0, 212, 255, 0.1);
  cursor: default;
}
</style>
