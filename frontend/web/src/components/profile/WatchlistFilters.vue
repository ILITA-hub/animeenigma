<template>
  <!-- Panel-only: the trigger + open state live in Profile.vue; this renders
       the separate full-width filter block below the controls row. -->
  <div class="mt-2 rounded-xl border border-white/10 bg-white/5 p-4 md:p-6">
    <div class="grid grid-cols-1 md:grid-cols-3 gap-6">
      <!-- Genres (AND) -->
      <section v-if="facets.genres.length">
        <header class="flex items-center justify-between mb-2">
          <span class="text-sm font-semibold text-white">{{ $t('profile.filters.genres') }}</span>
          <span class="text-xs text-muted-foreground">{{ $t('profile.filters.genresHint') }}</span>
        </header>
        <FilterCheckboxList
          :items="genreItems"
          :selected="genreIds"
          :searchable="facets.genres.length > 8"
          :search-placeholder="$t('profile.filters.searchGenres')"
          max-height-class="max-h-64"
          @update:selected="(v) => emit('update:genreIds', v)"
        />
      </section>

      <!-- Types (OR) -->
      <section v-if="facets.kinds.length">
        <header class="flex items-center justify-between mb-2">
          <span class="text-sm font-semibold text-white">{{ $t('common.filters.type') }}</span>
          <span class="text-xs text-muted-foreground">{{ $t('profile.filters.typesHint') }}</span>
        </header>
        <FilterCheckboxList
          :items="kindItems"
          :selected="kinds"
          :searchable="false"
          @update:selected="(v) => emit('update:kinds', v)"
        />
      </section>

      <!-- Year range -->
      <section v-if="facets.years.min !== null && facets.years.max !== null">
        <header class="mb-2">
          <span class="text-sm font-semibold text-white">{{ $t('profile.filters.year') }}</span>
        </header>
        <FilterYearRange
          :min="yearMin"
          :max="yearMax"
          :floor-year="facets.years.min as number"
          :ceil-year="facets.years.max as number"
          @update:min="(v) => emit('update:yearMin', v)"
          @update:max="(v) => emit('update:yearMax', v)"
        />
      </section>
    </div>

    <!-- Clear all -->
    <div class="mt-4 pt-3 border-t border-white/10 flex justify-end">
      <Button variant="ghost" size="sm" class="text-muted-foreground" :disabled="count === 0" @click="clearAll">
        {{ $t('profile.filters.clear') }}
      </Button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Button from '@/components/ui/Button.vue'
import FilterCheckboxList from '@/components/filters/FilterCheckboxList.vue'
import FilterYearRange from '@/components/filters/FilterYearRange.vue'
import type { WatchlistFacets, FacetGenre } from '@/types/watchlist-facets'
import { activeFilterCount } from '@/types/watchlist-facets'

const props = defineProps<{
  facets: WatchlistFacets
  genreIds: string[]
  kinds: string[]
  yearMin: number | null
  yearMax: number | null
}>()

const emit = defineEmits<{
  'update:genreIds': [string[]]
  'update:kinds': [string[]]
  'update:yearMin': [number | null]
  'update:yearMax': [number | null]
}>()

const { locale, t } = useI18n()

const count = computed(() =>
  activeFilterCount({ genreIds: props.genreIds, kinds: props.kinds, yearMin: props.yearMin, yearMax: props.yearMax }),
)

function localizedGenre(g: FacetGenre): string {
  const loc = locale.value || ''
  return loc.startsWith('ru') && g.name_ru ? g.name_ru : g.name
}

const genreItems = computed(() =>
  props.facets.genres.map((g) => ({ id: g.id, label: localizedGenre(g), count: g.count })),
)

const kindItems = computed(() =>
  props.facets.kinds.map((k) => ({ id: k.kind, label: t('common.filters.kind.' + k.kind), count: k.count })),
)

function clearAll() {
  emit('update:genreIds', [])
  emit('update:kinds', [])
  emit('update:yearMin', null)
  emit('update:yearMax', null)
}
</script>
