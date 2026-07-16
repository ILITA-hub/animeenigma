<template>
  <div class="flex flex-col gap-3 p-3">
    <!-- Header: active count -->
    <div class="flex items-center justify-between">
      <span class="text-[11px] font-semibold uppercase tracking-[0.06em] text-[var(--muted-foreground)]">
        Sources
      </span>
      <span class="text-[11px] font-semibold text-[var(--muted-foreground)]">
        {{ activeCount }} available
      </span>
    </div>

    <!-- Big Filters: Audio (Sub/Dub) + Language (EN/RU/JA) -->
    <div class="flex flex-col gap-3">
      <!-- Audio slider -->
      <div>
        <span class="block text-[11px] font-semibold uppercase tracking-[0.06em] text-[var(--muted-foreground)] mb-1.5">
          Audio
        </span>
        <div
          class="relative grid grid-cols-2 rounded-full p-1"
          style="background: var(--white-a8);"
          :data-on="audioIndex"
          role="radiogroup"
          :aria-label="$t('player.aePlayer.audio')"
        >
          <!-- Sliding thumb -->
          <span
            class="absolute top-1 bottom-1 left-1 rounded-full pointer-events-none transition-transform duration-[220ms] ease-[cubic-bezier(0.4,0,0.2,1)]"
            :style="{
              width: 'calc((100% - 8px) / 2)',
              background: 'linear-gradient(135deg, var(--brand-cyan), var(--brand-pink))',
              transform: `translateX(${audioIndex * 100}%)`,
            }"
            aria-hidden="true"
          />
          <button
            v-for="opt in audioOptions"
            :key="opt.value"
            :data-test="'audio-' + opt.value"
            role="radio"
            :aria-checked="audio === opt.value"
            :class="[
              'relative z-10 py-[9px] px-1.5 border-0 bg-transparent text-[13px] font-semibold transition-colors duration-[180ms] text-center rounded-full',
              'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--brand-cyan)]',
              audio === opt.value ? 'text-white' : 'text-[var(--muted-foreground)]',
            ]"
            @click="emit('update:audio', opt.value)"
          >
            {{ $t(opt.labelKey) }}
          </button>
        </div>
      </div>

      <!-- Language slider (DUB only — RAW is original voices, no language facet) -->
      <div v-if="audio === 'dub'">
        <span class="block text-[11px] font-semibold uppercase tracking-[0.06em] text-[var(--muted-foreground)] mb-1.5">
          Language
        </span>
        <div
          class="relative rounded-full p-1"
          style="background: var(--white-a8); display: grid; grid-template-columns: repeat(2, 1fr);"
          :data-on="langIndex"
          role="radiogroup"
          :aria-label="$t('player.aePlayer.language')"
        >
          <!-- Sliding thumb (2 cols) -->
          <span
            class="absolute top-1 bottom-1 left-1 rounded-full pointer-events-none transition-transform duration-[220ms] ease-[cubic-bezier(0.4,0,0.2,1)]"
            :style="{
              width: 'calc((100% - 8px) / 2)',
              background: 'linear-gradient(135deg, var(--brand-cyan), var(--brand-pink))',
              transform: `translateX(${langIndex * 100}%)`,
            }"
            aria-hidden="true"
          />
          <button
            v-for="opt in langOptions"
            :key="opt.value"
            :data-test="'lang-' + opt.value"
            role="radio"
            :aria-checked="lang === opt.value"
            :class="[
              'relative z-10 py-[9px] px-1.5 border-0 bg-transparent text-[13px] font-semibold transition-colors duration-[180ms] text-center rounded-full',
              'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--brand-cyan)]',
              lang === opt.value ? 'text-white' : 'text-[var(--muted-foreground)]',
            ]"
            @click="emit('update:lang', opt.value)"
          >
            {{ opt.label }}
          </button>
        </div>
      </div>
    </div>

    <!-- Team chips (only shown when teams.length > 0) -->
    <div v-if="teams.length > 0" class="flex flex-col gap-2">
      <span class="text-[11px] font-semibold uppercase tracking-[0.06em] text-[var(--muted-foreground)]">
        Team
      </span>
      <div class="flex flex-wrap gap-1.5">
        <button
          v-for="t in teams"
          :key="t"
          :class="[
            'px-3 py-1.5 rounded-full text-[12px] font-semibold border transition-all duration-150',
            'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--brand-cyan)]',
            team === t
              ? 'bg-[var(--cyan-a20)] border-[var(--accent-line)] text-[var(--brand-cyan)]'
              : 'border-transparent text-[var(--ink-2)] hover:text-white',
          ]"
          style="background: var(--white-a8);"
          @click="emit('update:team', t)"
        >
          <span>{{ t }}</span>
          <span
            v-if="teamCategory(t)"
            class="ml-[5px] text-[9px] font-semibold font-mono uppercase tracking-wide opacity-80"
          >{{ teamCategory(t) === 'dub' ? $t('player.dub') : $t('player.sub') }}</span>
        </button>
      </div>
    </div>

    <!-- Provider list (collapsed to the best/selected source unless hacker mode / error-expanded) -->
    <div class="flex flex-col gap-1">
      <span class="text-[11px] font-semibold uppercase tracking-[0.06em] text-[var(--muted-foreground)] mb-0.5">
        Provider
      </span>
      <div class="flex flex-col gap-1">
        <ProviderChip
          v-for="r in visibleRows"
          :key="r.id"
          :row="r"
          :cap="capMap.get(r.id)"
          :verify="r.verify ?? null"
          :best="!hackerMode && !expanded && r.id === topRow?.id"
          :selected="r.id === provider"
          :hacker-mode="hackerMode"
          :forced="forcedSelectableIds.has(r.id)"
          @select="emit('select-provider', r.id)"
        />
      </div>
      <!-- Error escape hatch: reveal the rest without full hacker mode -->
      <button
        v-if="playbackError && !hackerMode && !expanded && hiddenCount > 0"
        data-test="try-another"
        class="mt-1 self-start text-[12px] font-semibold text-[var(--brand-cyan)] hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--brand-cyan)] rounded px-1"
        @click="expanded = true"
      >
        {{ $t('player.sources.tryAnother') }} ({{ hiddenCount }})
      </button>
    </div>

    <!-- Server list -->
    <div v-if="servers.length > 0" class="flex flex-col gap-2">
      <span class="text-[11px] font-semibold uppercase tracking-[0.06em] text-[var(--muted-foreground)]">
        Server
      </span>
      <div class="flex flex-col gap-1">
        <button
          v-for="s in servers"
          :key="s.id"
          :class="[
            'flex items-center gap-2.5 px-2.5 py-[9px] rounded-[var(--r-md)] border text-sm text-left transition-all duration-150 w-full',
            'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--brand-cyan)]',
            server === s.id
              ? 'bg-[var(--cyan-a08)] border-[var(--accent-line)] text-white'
              : 'bg-white/[0.04] border-transparent text-[var(--ink-2)] hover:bg-white/[0.09] hover:text-white',
          ]"
          @click="emit('select-server', s.id)"
        >
          <span class="flex-1 font-semibold truncate">{{ s.label }}</span>
          <!-- 1st-party badge for SVO servers -->
          <span
            v-if="s.label.startsWith('SVO')"
            class="text-[10px] font-semibold font-mono uppercase tracking-wide px-[5px] py-px rounded"
            style="background: var(--brand-cyan); color: var(--color-base);"
          >1st</span>
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { AudioKind, TrackLang, ProviderRow, ChipState } from '@/types/aePlayer'
import type { ProviderCap } from '@/types/capabilities'
import ProviderChip from './ProviderChip.vue'

const props = withDefaults(
  defineProps<{
    rows: ProviderRow[]
    audio: AudioKind
    lang: TrackLang
    team: string | null
    provider: string
    server: string
    servers: { id: string; label: string }[]
    teams: string[]
    capMap?: Map<string, ProviderCap>
    hackerMode?: boolean
    playbackError?: boolean
  }>(),
  {
    capMap: () => new Map<string, ProviderCap>(),
    hackerMode: false,
    playbackError: false,
  },
)

const emit = defineEmits<{
  (e: 'update:audio', v: AudioKind): void
  (e: 'update:lang', v: TrackLang): void
  (e: 'update:team', v: string | null): void
  (e: 'select-provider', id: string): void
  (e: 'select-server', id: string): void
}>()

// RAW = original voices (today's sub + raw-JP); DUB = dubbed. Labels are
// translatable: RAW → player.aePlayer.audioRaw (Original/Оригинал/オリジナル),
// DUB reuses the existing player.dub key.
const audioOptions: { value: AudioKind; labelKey: string }[] = [
  { value: 'sub', labelKey: 'player.aePlayer.audioRaw' },
  { value: 'dub', labelKey: 'player.dub' },
]

// Language is a DUB-only facet (RU/EN). Under RAW the slider is hidden — the
// subtitle language is chosen in the Subtitles menu.
const langOptions: { value: TrackLang; label: string }[] = [
  { value: 'en', label: 'English' },
  { value: 'ru', label: 'Русский' },
]

const audioIndex = computed(() =>
  audioOptions.findIndex(o => o.value === props.audio),
)

const langIndex = computed(() =>
  langOptions.findIndex(o => o.value === props.lang),
)

// State bucket → available sources float to the top of the full list. Backend
// `order` (desc) is the within-bucket tiebreak for every bucket EXCEPT
// 'degraded', which tiebreaks by `playability_index` (desc) then `order`
// (Phase B). 'degraded'/'recovering' are selectable-but-not-auto (AUTO-484) so
// they rank below 'active' but above 'no_content'. Array.prototype.sort is
// stable, so equal keys keep input order (rows already arrive `order`-sorted
// from rowsFromReport).
const STATE_RANK: Record<ChipState, number> = {
  active: 0,
  recovering: 1,
  degraded: 2,
  no_content: 3,
}
const sortedRows = computed(() =>
  [...props.rows].sort((a, b) => {
    const stateDiff = STATE_RANK[a.state] - STATE_RANK[b.state]
    if (stateDiff) return stateDiff
    // Within the degraded bucket, rank by real playability; order is the final
    // tiebreak. Other buckets keep backend `order` (desc). (Promoted providers
    // arrive as `active` and sort with the active bucket.)
    if (a.state === 'degraded') {
      return (b.playability_index ?? 0) - (a.playability_index ?? 0) || b.order - a.order
    }
    return b.order - a.order
  }),
)
const activeRows = computed(() => sortedRows.value.filter(r => r.state === 'active'))
const activeCount = computed(() => activeRows.value.length)

// The single best playable source — the selected provider when set (the smart
// default already picked the top-ranked one), else the first active row. Only
// this one carries the BEST badge.
const topRow = computed(
  () => activeRows.value.find(r => r.id === props.provider) ?? activeRows.value[0] ?? null,
)

// Default collapse shows the top N selectable sources (not just the single
// best) so users can switch between the strongest options without opening
// hacker mode. The selected provider is always pinned into the visible set
// even if it ranks below the top N. Hacker mode → full ranked list;
// error-expanded → all active rows.
const TOP_N = 3
const expanded = ref(false)
watch(() => props.provider, () => { expanded.value = false })

const collapsedRows = computed(() => {
  const top = activeRows.value.slice(0, TOP_N)
  // Pad with the next best non-active sources so the user always has up to
  // TOP_N selectable providers, even when nothing is active (fully-degraded
  // fleet) — otherwise the panel is a dead end without hacker mode.
  if (top.length < TOP_N) {
    for (const r of sortedRows.value) {
      if (top.length >= TOP_N) break
      if (r.state === 'active' || r.state === 'no_content') continue
      if (!top.some(x => x.id === r.id)) top.push(r)
    }
  }
  const selected = sortedRows.value.find(r => r.id === props.provider)
  if (selected && !top.some(r => r.id === selected.id)) top.push(selected)
  return top
})

// The padded (non-active, non-no_content) rows are made selectable WITHOUT
// hacker mode so the user can always pick a source.
const forcedSelectableIds = computed(() => {
  const ids = new Set<string>()
  for (const r of collapsedRows.value) {
    if (r.state !== 'active' && r.state !== 'no_content') ids.add(r.id)
  }
  return ids
})

const visibleRows = computed(() => {
  if (props.hackerMode) return sortedRows.value
  if (expanded.value) return activeRows.value
  return collapsedRows.value
})
const hiddenCount = computed(() => Math.max(0, activeCount.value - collapsedRows.value.length))

// Team → category tag from the selected provider's capability variants.
function teamCategory(name: string): 'sub' | 'dub' | null {
  const cap = props.capMap.get(props.provider)
  if (!cap) return null
  const v = cap.variants.find(x => x.team?.name === name)
  if (!v) return null
  return v.category === 'dub' ? 'dub' : v.category === 'sub' ? 'sub' : null
}
</script>
