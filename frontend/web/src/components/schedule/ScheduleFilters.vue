<!-- frontend/web/src/components/schedule/ScheduleFilters.vue -->
<template>
  <div>
    <div class="flex items-center gap-2 flex-wrap p-2.5 rounded-xl border border-white/[0.06] bg-white/[0.025] mb-2.5">
      <div class="flex-1 min-w-[160px] flex items-center gap-2 rounded-lg bg-white/[0.06] px-2.5 py-1.5">
        <span class="opacity-50">🔍</span>
        <input
          :value="filters.search"
          :placeholder="$t('schedule.searchPlaceholder')"
          class="bg-transparent border-0 outline-none text-foreground text-sm w-full placeholder:text-muted-foreground"
          @input="setSearch(($event.target as HTMLInputElement).value)"
        />
      </div>
      <Chip
        v-if="loggedIn"
        :active="filters.myList"
        size="sm"
        @click="toggleMyList()"
      >★ {{ $t('schedule.myList') }}</Chip>
      <FilterDropdown :label="$t('schedule.genre')" :options="genreOptions" :selected="filters.genres" searchable :search-placeholder="$t('schedule.searchPlaceholder')" :empty-text="$t('schedule.empty')" @toggle="toggleSet(filters.genres, $event)" />
    </div>

    <div class="flex items-center gap-2 flex-wrap mb-3 min-h-6">
      <template v-if="activeChips.length">
        <span class="text-[11px] text-muted-foreground">{{ $t('schedule.activeFilters') }}</span>
        <Chip
          v-for="chip in activeChips"
          :key="chip.key"
          active
          removable
          size="sm"
          :remove-label="$t('schedule.removeFilter')"
          @remove="chip.remove()"
        >{{ chip.label }}</Chip>
        <button type="button" class="text-xs text-muted-foreground underline cursor-pointer hover:text-foreground" @click="$emit('reset')">{{ $t('schedule.resetAll') }}</button>
      </template>
      <span v-else class="text-[11px] text-muted-foreground">{{ $t('schedule.noFilters') }}</span>
      <span class="text-[11px] text-white/35 ml-auto">{{ $t('schedule.countOf', { n: matchCount, total }) }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import FilterDropdown from './FilterDropdown.vue'
import { Chip } from '@/components/ui'
import type { ScheduleFilterState, ScheduleGenre } from '@/composables/schedule/types'

const props = defineProps<{
  filters: ScheduleFilterState
  genres: ScheduleGenre[]
  loggedIn: boolean
  matchCount: number
  total: number
}>()
defineEmits<{ reset: [] }>()
const { t, locale } = useI18n()

const genreOptions = computed(() => props.genres.filter(g => g.name).map(g => ({ value: g.name as string, label: (locale.value === 'ru' && g.name_ru) ? g.name_ru : (g.name as string) })))

// filters is a shared reactive object from the parent composable; direct
// mutation of its nested fields is intentional (same-reference shared state).
/* eslint-disable vue/no-mutating-props */
function setSearch(v: string) { props.filters.search = v }
function toggleMyList() { props.filters.myList = !props.filters.myList }
function toggleSet(set: Set<string>, v: string) { set.has(v) ? set.delete(v) : set.add(v) }

const activeChips = computed(() => {
  const chips: { key: string; label: string; remove: () => void }[] = []
  if (props.filters.search) chips.push({ key: 'q', label: `${t('schedule.searchChip')}: ${props.filters.search}`, remove: () => { props.filters.search = '' } })
  if (props.filters.myList) chips.push({ key: 'mine', label: `★ ${t('schedule.myList')}`, remove: () => { props.filters.myList = false } })
  props.filters.genres.forEach(g => chips.push({ key: 'g:' + g, label: genreOptions.value.find(o => o.value === g)?.label ?? g, remove: () => props.filters.genres.delete(g) }))
  return chips
})
/* eslint-enable vue/no-mutating-props */
</script>
