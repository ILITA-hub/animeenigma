<template>
  <div class="flex flex-col" data-test="episodes-panel">
    <!-- Sheet head: count, view toggle (100+), next-unwatched, jump (16+) -->
    <div class="px-3 pt-2.5 pb-2 flex items-center gap-2">
      <span class="text-[13px] font-semibold text-white">{{ $t('player.aePlayer.episodes') }}</span>
      <span class="text-[11px] text-[var(--muted-foreground)]">{{ episodes.length }}</span>

      <div v-if="showGridToggle" class="ep-vt" role="group" :aria-label="$t('player.aePlayer.episodeListView')">
        <button
          type="button"
          :class="{ 'ep-vt--on': view === 'strip' }"
          :aria-label="$t('player.aePlayer.stripView')"
          :title="$t('player.aePlayer.stripView')"
          data-test="view-strip"
          @click="setView('strip')"
        >
          <GalleryHorizontal :size="13" aria-hidden="true" />
        </button>
        <button
          type="button"
          :class="{ 'ep-vt--on': view === 'grid' }"
          :aria-label="$t('player.aePlayer.gridView')"
          :title="$t('player.aePlayer.gridView')"
          data-test="view-grid"
          @click="setView('grid')"
        >
          <LayoutGrid :size="13" aria-hidden="true" />
        </button>
      </div>

      <span class="flex-1" />

      <button
        v-if="downloadMode === 'ready'"
        type="button"
        class="ep-chip ep-chip-season"
        data-test="season-download"
        :title="seasonChipTitle"
        @click="emit('download-season')"
      >
        <Download :size="11" aria-hidden="true" />
        {{ seasonChipLabel }}
      </button>
      <button
        v-if="showJump && nextUnwatched"
        type="button"
        class="ep-chip"
        data-test="next-unwatched"
        @click="scrollToEp(nextUnwatched.number, true)"
      >
        → {{ $t('player.aePlayer.nextUnwatched', { n: nextUnwatched.number }) }}
      </button>
      <input
        v-if="showJump"
        v-model="jumpVal"
        class="ep-jump"
        type="text"
        inputmode="numeric"
        :placeholder="$t('player.aePlayer.jumpPlaceholder')"
        :aria-label="$t('player.aePlayer.jumpAria')"
        data-test="jump-input"
        @keydown.enter.prevent="onJump"
        @keydown.stop
      >
    </div>

    <div v-if="episodes.length === 0" class="px-3 pb-3 text-[13px] text-[var(--muted-foreground)]">
      {{ $t('player.aePlayer.noEpisodes') }}
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
          <span>{{ $t('player.aePlayer.epAbbrev') }} {{ ep.label }}</span>
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
          v-if="downloadMode === 'ready' && (downloadStates[ep.number] === 'done')"
          :data-test="`ep-downloaded-${ep.number}`"
          class="ep-dl text-success"
          :title="$t('player.aePlayer.offline.downloaded')"
        ><Check :size="14" /></span>
        <span
          v-else-if="downloadMode === 'ready' && (downloadStates[ep.number] === 'downloading' || downloadStates[ep.number] === 'queued')"
          class="ep-dl text-muted-foreground animate-spin"
          :title="$t('player.aePlayer.offline.downloading')"
        ><Loader2 :size="14" /></span>
        <span
          v-if="partialPct(ep) > 0"
          class="ep-progress"
          :style="{ width: partialPct(ep) + '%' }"
          aria-hidden="true"
          data-test="ep-progress"
        />
      </button>
      <div
        v-if="upcomingVisible"
        class="ep-card ep-card--upcoming"
        data-test="episode-upcoming"
        :title="upcomingText"
      >
        <span class="ep-card-n">
          <span>{{ $t('player.aePlayer.epAbbrev') }} {{ upcoming?.number }}</span>
          <Clock :size="11" class="ep-card-clock" aria-hidden="true" />
        </span>
        <span class="ep-card-t">{{ upcomingText }}</span>
      </div>
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
          ? 'background: var(--cyan-a20)'
          : 'background: var(--white-a8)'"
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
      <div
        v-if="upcomingVisible"
        class="ep-cell ep-cell--upcoming relative rounded-[var(--r-sm)] text-[12px] font-semibold"
        data-test="episode-grid-upcoming"
        :title="upcomingText"
      >
        {{ upcoming?.number }}
        <Clock class="ep-cell-clock" :size="10" aria-hidden="true" />
      </div>
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
        <span v-if="marked">{{ $t('player.aePlayer.epWatched', { n: selectedNumber }) }}</span>
        <span v-else-if="marking">{{ $t('player.aePlayer.marking') }}</span>
        <span v-else>{{ $t('player.aePlayer.markWatched', { n: selectedNumber }) }}</span>
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, nextTick, onMounted } from 'vue'
import { Check, LayoutGrid, GalleryHorizontal, Clock, Download, Loader2 } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import type { EpisodeOption } from '@/components/player/EpisodeSelector.types'
import type { DownloadState } from '@/offline/types'

export interface EpisodeUserProgress {
  /** 0..1 fraction of the episode watched */
  pct: number
  /** saved playhead position in seconds */
  sec?: number
  completed: boolean
}

const { t } = useI18n()

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
    /** Disabled placeholder for the not-yet-aired next episode (from the resume banner). */
    upcoming?: { number: number; etaLabel?: string } | null
    /** Season-download chip mode: 'ready' = real download (installed app),
     *  'off' = hidden (browser tab — downloads are app-only — or unavailable). */
    downloadMode?: 'off' | 'ready'
    /** Download state per episode number, for the per-episode status icons. */
    downloadStates?: Record<number, DownloadState>
  }>(),
  {
    watchedUpTo: 0,
    progress: () => ({}),
    canMark: false,
    marking: false,
    marked: false,
    upcoming: null,
    downloadMode: 'off',
    downloadStates: () => ({}),
  },
)

const emit = defineEmits<{
  (e: 'select', ep: EpisodeOption): void
  (e: 'mark-watched'): void
  (e: 'download-season'): void
}>()

// The season chip only renders in 'ready' mode (installed app).
const seasonChipLabel = computed(() => t('player.aePlayer.offline.season'))
const seasonChipTitle = computed(() => t('player.aePlayer.offline.scopeSeasonTitle'))

// ── V2b adaptive rules ───────────────────────────────────────────────────────
// ≤15 eps: strip only. 16+: jump input appears. 100+: grid toggle appears.

const view = ref<'strip' | 'grid'>('strip')
const showJump = computed(() => props.episodes.length > 15)
const showGridToggle = computed(() => props.episodes.length >= 100)

const stripRef = ref<HTMLElement | null>(null)
const gridRef = ref<HTMLElement | null>(null)
const jumpVal = ref('')

// The high-water mark (watchedUpTo = anime_list.episodes) means "watched up to
// N" — correct for MAL-imported lists that carry only a count and no per-episode
// rows. But when that mark is itself anchored by an OUT-OF-ORDER on-platform
// completion (a completed row sits at the mark while episodes below it have no
// row — e.g. an episode auto-opened by a stale deep-link, then marked watched),
// the contiguous fill would falsely paint never-opened episodes as watched. In
// that case we trust the per-episode rows alone. Watched-count bug, 2026-06-30.
const onPlatformGappy = computed(() => {
  if (!props.progress[props.watchedUpTo]?.completed) return false // mark is import-derived
  let rowsAtOrBelowMark = 0
  for (const key of Object.keys(props.progress)) {
    if (Number(key) <= props.watchedUpTo) rowsAtOrBelowMark++
  }
  return rowsAtOrBelowMark < props.watchedUpTo // a gap exists under an on-platform mark
})

function isWatched(ep: EpisodeOption): boolean {
  const row = props.progress[ep.number]
  if (row) return !!row.completed // real per-episode playback data wins
  if (ep.number > props.watchedUpTo) return false
  return !onPlatformGappy.value // contiguous fill, unless the mark jumped over gaps
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

// Show the disabled "next episode airs …" placeholder, unless that episode is
// already in the loaded list (then it's a real, selectable card).
const upcomingVisible = computed(
  () => !!props.upcoming && !props.episodes.some((e) => e.number === props.upcoming!.number),
)
const upcomingText = computed(() => {
  const u = props.upcoming
  if (!u) return ''
  return u.etaLabel
    ? t('player.aePlayer.episodeUpcoming', { when: u.etaLabel })
    : t('player.aePlayer.episodeUpcomingNoEta')
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
  background: var(--white-a4);
  color: var(--muted-foreground);
  cursor: pointer;
  transition: background 0.15s, color 0.15s;
}

.ep-vt button:hover {
  color: white;
}

.ep-vt .ep-vt--on {
  background: var(--accent-soft);
  color: var(--brand-cyan);
}

.ep-chip {
  height: 26px;
  padding: 0 10px;
  border-radius: var(--r-sm);
  border: 0;
  background: var(--accent-soft);
  color: var(--brand-cyan);
  font-size: 11.5px;
  font-weight: 600;
  cursor: pointer;
  white-space: nowrap;
  transition: background 0.15s;
}

.ep-chip:hover {
  background: var(--cyan-a20);
}

.ep-jump {
  width: 112px;
  height: 28px;
  border-radius: var(--r-sm);
  border: 1px solid var(--border);
  background: var(--white-a4);
  color: white;
  padding: 0 10px;
  font-size: 12px;
}

.ep-jump::placeholder {
  color: var(--muted-foreground);
}

.ep-jump:focus {
  outline: none;
  border-color: var(--cyan-a40);
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
  background: var(--white-a4);
  padding: 8px 11px 9px;
  cursor: pointer;
  position: relative;
  overflow: hidden;
  text-align: left;
  transition: background 0.15s, border-color 0.15s;
}

.ep-card:hover {
  background: var(--line-strong);
}

.ep-card--sel {
  border-color: var(--cyan-a60);
  background: var(--cyan-a08);
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

.ep-dl {
  display: inline-flex;
  align-items: center;
  margin-left: auto;
  padding: 2px;
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
    box-shadow: 0 0 14px var(--cyan-a40);
  }
  100% {
    box-shadow: none;
  }
}

/* upcoming (not-yet-aired) placeholder — disabled, non-interactive */
.ep-card--upcoming {
  opacity: 0.5;
  cursor: default;
  border-style: dashed;
}

.ep-card--upcoming:hover {
  background: var(--white-a4);
}

.ep-card-clock {
  color: var(--muted-foreground);
  flex-shrink: 0;
}

.ep-cell-clock {
  position: absolute;
  top: 3px;
  right: 3px;
  color: var(--muted-foreground);
}

.ep-cell--upcoming {
  opacity: 0.5;
  cursor: default;
  background: var(--white-a8);
  color: var(--muted-foreground);
}

/* mark-as-watched footer */
.ep-mark {
  background: var(--white-a8);
  color: var(--ink-2);
}

.ep-mark:not(:disabled):hover {
  background: var(--line-strong);
  color: white;
}

.ep-mark--done {
  color: var(--brand-cyan);
  background: var(--cyan-a08);
  cursor: default;
}

.ep-chip-season {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

/* Touch: keep the header chips comfortably tappable. */
@media (pointer: coarse) {
  .ep-chip {
    min-height: 36px;
  }
}
</style>
