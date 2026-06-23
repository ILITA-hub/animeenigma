<template>
  <aside class="bg-card/40 border border-white/10 rounded-xl p-4 space-y-1">
    <header class="flex items-center justify-between pb-2">
      <h2 class="text-lg font-semibold text-white">{{ $t('browse.filters.title') }}</h2>
    </header>

    <!-- Genres — searchable multi-select -->
    <FilterSection
      :label="$t('browse.filters.section.genres')"
      :count="filters.genres.value.length"
    >
      <FilterCheckboxList
        :items="genreItems"
        :selected="filters.genres.value"
        :search-placeholder="$t('browse.filters.searchGenres')"
        @update:selected="onGenresChange"
      />
    </FilterSection>

    <!-- Studios — searchable multi-select -->
    <FilterSection
      :label="$t('browse.filters.section.studios')"
      :count="filters.studios.value.length"
    >
      <FilterCheckboxList
        :items="studios"
        :selected="filters.studios.value"
        :search-placeholder="$t('browse.filters.searchStudios')"
        @update:selected="onStudiosChange"
      />
    </FilterSection>

    <!-- Format (kind) — single-select radio -->
    <FilterSection
      :label="$t('browse.filters.section.format')"
      :count="filters.kind.value ? 1 : 0"
    >
      <RadioGroup :model-value="filters.kind.value" :options="kindOptions" @update:model-value="(v) => onKindChange(v as Kind)" />
    </FilterSection>

    <!-- Status — single-select radio -->
    <FilterSection
      :label="$t('browse.filters.section.status')"
      :count="filters.status.value ? 1 : 0"
    >
      <RadioGroup :model-value="filters.status.value" :options="statusOptions" @update:model-value="onStatusChange" />
    </FilterSection>

    <!-- Season — single-select radio -->
    <FilterSection
      :label="$t('browse.filters.section.season')"
      :count="filters.season.value ? 1 : 0"
    >
      <RadioGroup :model-value="filters.season.value" :options="seasonOptions" @update:model-value="onSeasonChange" />
    </FilterSection>

    <!-- Year range -->
    <FilterSection
      :label="$t('browse.filters.section.year')"
      :count="filters.yearFrom.value || filters.yearTo.value ? 1 : 0"
    >
      <div class="flex items-center gap-2">
        <div class="w-1/2">
          <Input type="number" size="sm" :min="MIN_YEAR" :max="MAX_YEAR" :model-value="filters.yearFrom.value != null ? String(filters.yearFrom.value) : ''" :placeholder="$t('browse.filters.year.from')" :aria-label="$t('browse.filters.year.from')" @change="onYearChange('from', ($event.target as HTMLInputElement).valueAsNumber)" />
        </div>
        <span class="text-white/40">—</span>
        <div class="w-1/2">
          <Input type="number" size="sm" :min="MIN_YEAR" :max="MAX_YEAR" :model-value="filters.yearTo.value != null ? String(filters.yearTo.value) : ''" :placeholder="$t('browse.filters.year.to')" :aria-label="$t('browse.filters.year.to')" @change="onYearChange('to', ($event.target as HTMLInputElement).valueAsNumber)" />
        </div>
      </div>
    </FilterSection>

    <!-- Minimum score — range slider from 0-10, step 0.5 (AUTO-091) -->
    <FilterSection
      :label="$t('browse.filters.section.score')"
      :count="filters.scoreMin.value ? 1 : 0"
    >
      <div class="space-y-1.5">
        <div class="flex items-center justify-between text-xs text-white/50">
          <span>0</span>
          <span class="text-white/80 font-medium">
            {{ filters.scoreMin.value ? `>= ${filters.scoreMin.value}` : $t('browse.filters.score.any') }}
          </span>
          <span>10</span>
        </div>
        <!-- bespoke-keep: range slider; no slider primitive in the design system -->
        <input
          type="range"
          min="0"
          max="10"
          step="0.5"
          :value="filters.scoreMin.value ?? 0"
          :aria-label="$t('browse.filters.section.score')"
          class="w-full h-1.5 rounded-full appearance-none bg-white/10 accent-cyan-500 cursor-pointer focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cyan-500/50"
          @input="onScoreMinChange(($event.target as HTMLInputElement).valueAsNumber)"
        />
      </div>
    </FilterSection>

    <!-- Provider — checkbox list with per-provider accent colors -->
    <FilterSection
      :label="$t('browse.filters.section.provider')"
      :count="filters.providers.value.length"
    >
      <label
        v-for="opt in providerOptions"
        :key="opt.value"
        class="flex items-center gap-2 text-sm text-white/70 hover:text-white cursor-pointer py-0.5"
      >
        <!-- bespoke-keep: per-provider brand accent (opt.accent); cyan-only Checkbox primitive can't model it -->
        <input
          type="checkbox"
          :value="opt.value"
          :checked="filters.providers.value.includes(opt.value)"
          :class="['rounded border-white/20 bg-transparent focus:ring-2', opt.accent]"
          @change="onProviderToggle(opt.value, ($event.target as HTMLInputElement).checked)"
        />
        <span>{{ opt.label }}</span>
      </label>
    </FilterSection>

    <!-- Sort — radio set (Phase 11's 5-axis options reused at sidebar density) -->
    <FilterSection
      :label="$t('browse.filters.section.sort')"
      :count="filters.sort.value !== 'popularity' ? 1 : 0"
    >
      <RadioGroup :model-value="filters.sort.value" :options="sortOptions" @update:model-value="(v) => onSortChange(v as Sort)" />
    </FilterSection>

    <!-- Reset -->
    <div class="pt-3">
      <button
        type="button"
        class="w-full px-3 py-2 rounded-md bg-pink-500/10 border border-pink-400/20 text-pink-300 hover:text-pink-200 hover:bg-pink-500/20 text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cyan-500/50"
        @click="onReset"
      >
        {{ $t('browse.filters.reset') }}
      </button>
    </div>
  </aside>
</template>

<script setup lang="ts">
// The "filters" prop is a useBrowseFilters() instance shared with the
// parent view by design — the sidebar mutates the composable's
// reactive refs (e.g. filters.genres.value = ...) and the parent
// re-runs its apiParams watcher. This is the standard pattern for
// passing a composable down; eslint's `vue/no-mutating-props` mis-
// flags it because the rule can't tell the prop value from a deeply
// reactive object.
/* eslint-disable vue/no-mutating-props */
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  useBrowseFilters,
  type Provider,
  type Kind,
  type Sort,
} from '@/composables/useBrowseFilters'
import FilterSection from './FilterSection.vue'
import FilterCheckboxList from '@/components/filters/FilterCheckboxList.vue'
import { getLocalizedGenre } from '@/utils/title'
import { Input, RadioGroup } from '@/components/ui'

// Phase 15 (UX-31) — Browse.vue passes the genre list down (no
// duplicate fetch) and the parent's useBrowseFilters instance so the
// sidebar mutates the same reactive state.

interface Genre {
  id: string
  name: string
  name_ru?: string
}

// Phase 15 (UX-31) — sidebar consumes the parent's useBrowseFilters
// instance via prop so Browse.vue's loadAnime() watcher and the sidebar
// share a single source of truth. The sidebar does NOT instantiate the
// composable itself.
const props = defineProps<{
  genres: Genre[]
  studios: { id: string; label: string }[]
  filters: ReturnType<typeof useBrowseFilters>
}>()
const { t } = useI18n()

const MIN_YEAR = 1960
const MAX_YEAR = new Date().getFullYear() + 1

const kindOptions = computed<{ value: Kind; label: string }[]>(() => [
  { value: '', label: t('browse.filters.format.any') },
  { value: 'tv', label: t('browse.filters.format.tv') },
  { value: 'movie', label: t('browse.filters.format.movie') },
  { value: 'ova', label: t('browse.filters.format.ova') },
  { value: 'ona', label: t('browse.filters.format.ona') },
  { value: 'special', label: t('browse.filters.format.special') },
])

const statusOptions = computed(() => [
  { value: '', label: t('browse.filters.status.any') },
  { value: 'released', label: t('browse.filters.status.released') },
  { value: 'ongoing', label: t('browse.filters.status.ongoing') },
  { value: 'announced', label: t('browse.filters.status.anons') },
])

const seasonOptions = computed(() => [
  { value: '', label: t('browse.filters.season.any') },
  { value: 'winter', label: t('browse.filters.season.winter') },
  { value: 'spring', label: t('browse.filters.season.spring') },
  { value: 'summer', label: t('browse.filters.season.summer') },
  { value: 'fall', label: t('browse.filters.season.fall') },
])

function localizedGenre(g: Genre) {
  return getLocalizedGenre(g.name, g.name_ru)
}

const genreItems = computed(() => props.genres.map(g => ({ id: g.id, label: localizedGenre(g) })))

// Per-provider brand/identity accents (DS brand-exempt hues + a semantic token).
const providerOptions = computed<{ value: Provider; label: string; accent: string }[]>(() => [
  {
    value: 'kodik',
    label: t('browse.filters.provider.kodik'),
    accent: 'text-cyan-500 focus:ring-cyan-500',
  },
  {
    value: 'dub',
    // English-dub availability (has_dub) — semantic success token (EN surface).
    label: t('browse.filters.provider.dub'),
    accent: 'text-success focus:ring-success',
  },
  {
    value: 'raw',
    // Raw (JP) provider-identity hue (rose) — brand-exempt decorative accent.
    label: t('browse.filters.provider.raw'),
    accent: 'text-rose-500 focus:ring-rose-500',
  },
  {
    value: 'ae',
    // First-party AnimeEnigma (has_video) — indigo brand-exempt accent.
    label: t('browse.filters.provider.ae'),
    accent: 'text-indigo-400 focus:ring-indigo-400',
  },
])

const sortOptions = computed<{ value: Sort; label: string }[]>(() => [
  { value: 'popularity', label: t('browse.sort.popularity') },
  { value: 'rating', label: t('browse.sort.rating') },
  { value: 'year', label: t('browse.sort.year') },
  { value: 'updated', label: t('browse.sort.updated') },
  { value: 'title', label: t('browse.sort.title') },
])

function onGenresChange(ids: string[]) {
  props.filters.genres.value = ids
  props.filters.writeUrl()
}

function onStudiosChange(ids: string[]) {
  props.filters.studios.value = ids
  props.filters.writeUrl()
}

function onProviderToggle(p: Provider, checked: boolean) {
  const set = new Set(props.filters.providers.value)
  if (checked) set.add(p)
  else set.delete(p)
  props.filters.providers.value = [...set]
  props.filters.writeUrl()
}

function onKindChange(v: Kind) {
  props.filters.kind.value = v
  props.filters.writeUrl()
}

function onStatusChange(v: string) {
  // Composable's status ref accepts the same whitelisted string set.
  props.filters.status.value = v as typeof props.filters.status.value
  props.filters.writeUrl()
}

function onSeasonChange(v: string) {
  props.filters.season.value = v as typeof props.filters.season.value
  props.filters.writeUrl()
}

function onSortChange(v: Sort) {
  props.filters.sort.value = v
  props.filters.writeUrl()
}

function onYearChange(which: 'from' | 'to', n: number) {
  const v = Number.isFinite(n) && n >= MIN_YEAR && n <= MAX_YEAR ? n : null
  if (which === 'from') {
    props.filters.yearFrom.value = v
    // Client-side validation: from <= to (locked in CONTEXT.md specifics).
    if (v && props.filters.yearTo.value && v > props.filters.yearTo.value) {
      props.filters.yearTo.value = v
    }
  } else {
    props.filters.yearTo.value = v
    if (v && props.filters.yearFrom.value && v < props.filters.yearFrom.value) {
      props.filters.yearFrom.value = v
    }
  }
  props.filters.writeUrl()
}

function onScoreMinChange(n: number) {
  props.filters.scoreMin.value = n > 0 ? n : null
  props.filters.writeUrl()
}

function onReset() {
  props.filters.reset()
}
</script>
