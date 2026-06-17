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
            :class="[
              'relative z-10 py-[9px] px-1.5 border-0 bg-transparent text-[13px] font-semibold transition-colors duration-[180ms] text-center',
              'focus-visible:outline-none',
              audio === opt.value ? 'text-white' : 'text-[var(--muted-foreground)]',
            ]"
            @click="emit('update:audio', opt.value)"
          >
            {{ opt.label }}
          </button>
        </div>
      </div>

      <!-- Language slider -->
      <div>
        <span class="block text-[11px] font-semibold uppercase tracking-[0.06em] text-[var(--muted-foreground)] mb-1.5">
          Language
        </span>
        <div
          class="relative rounded-full p-1"
          style="background: var(--white-a8); display: grid; grid-template-columns: repeat(3, 1fr);"
          :data-on="langIndex"
        >
          <!-- Sliding thumb (3 cols) -->
          <span
            class="absolute top-1 bottom-1 left-1 rounded-full pointer-events-none transition-transform duration-[220ms] ease-[cubic-bezier(0.4,0,0.2,1)]"
            :style="{
              width: 'calc((100% - 8px) / 3)',
              background: 'linear-gradient(135deg, var(--brand-cyan), var(--brand-pink))',
              transform: `translateX(${langIndex * 100}%)`,
            }"
            aria-hidden="true"
          />
          <button
            v-for="opt in langOptions"
            :key="opt.value"
            :data-test="'lang-' + opt.value"
            :class="[
              'relative z-10 py-[9px] px-1.5 border-0 bg-transparent text-[13px] font-semibold transition-colors duration-[180ms] text-center',
              'focus-visible:outline-none',
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
          :key="r.def.id"
          :row="r"
          :cap="capMap.get(r.def.id)"
          :best="!hackerMode && !expanded && r.def.id === topRow?.def.id"
          :selected="r.def.id === provider"
          :hacker-mode="hackerMode"
          @select="emit('select-provider', r.def.id)"
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
import type { AudioKind, TrackLang, ProviderRow } from '@/types/unifiedPlayer'
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
    rankedIds?: string[]
    hackerMode?: boolean
    playbackError?: boolean
  }>(),
  {
    capMap: () => new Map<string, ProviderCap>(),
    rankedIds: () => [],
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

const audioOptions: { value: AudioKind; label: string }[] = [
  { value: 'sub', label: 'SUB' },
  { value: 'dub', label: 'DUB' },
]

const langOptions: { value: TrackLang; label: string }[] = [
  { value: 'en', label: 'English' },
  { value: 'ru', label: 'Русский' },
  { value: 'ja', label: '日本語' },
]

const audioIndex = computed(() =>
  audioOptions.findIndex(o => o.value === props.audio),
)

const langIndex = computed(() =>
  langOptions.findIndex(o => o.value === props.lang),
)

// Rows sorted by the merged capability ranking (registry order for unranked).
const sortedRows = computed(() => {
  const pos = (id: string) => {
    const i = props.rankedIds.indexOf(id)
    return i === -1 ? Number.MAX_SAFE_INTEGER : i
  }
  return [...props.rows].sort((a, b) => pos(a.def.id) - pos(b.def.id))
})
const activeRows = computed(() => sortedRows.value.filter(r => r.state === 'active'))
const activeCount = computed(() => activeRows.value.length)

// Collapse: by default show only the best playable source — the selected
// provider when set (the smart default already picked the top-ranked one),
// else the first active row. Hacker mode → full ranked list; error-expanded
// → all active rows.
const topRow = computed(
  () => activeRows.value.find(r => r.def.id === props.provider) ?? activeRows.value[0] ?? null,
)
const expanded = ref(false)
watch(() => props.provider, () => { expanded.value = false })

const visibleRows = computed(() => {
  if (props.hackerMode) return sortedRows.value
  if (expanded.value) return activeRows.value
  return topRow.value ? [topRow.value] : []
})
const hiddenCount = computed(() => Math.max(0, activeCount.value - 1))

// Team → category tag from the selected provider's capability variants.
function teamCategory(name: string): 'sub' | 'dub' | null {
  const cap = props.capMap.get(props.provider)
  if (!cap) return null
  const v = cap.variants.find(x => x.team?.name === name)
  if (!v) return null
  return v.category === 'dub' ? 'dub' : v.category === 'sub' ? 'sub' : null
}
</script>
