<template>
  <div class="w-full">
    <!-- Trigger -->
    <div class="flex justify-end">
      <Button
        variant="ghost"
        size="sm"
        class="gap-1.5 text-white/70 hover:text-white"
        :aria-expanded="open"
        @click="open = !open"
      >
        <SlidersHorizontal class="size-4" />
        <span>{{ $t('profile.filters.button') }}</span>
        <Badge v-if="count > 0" variant="primary" size="sm">{{ count }}</Badge>
        <ChevronDown class="size-4 transition-transform duration-200" :class="open ? 'rotate-180' : ''" />
      </Button>
    </div>

    <!-- Inline filter block (separate panel below the controls row) -->
    <Transition
      enter-active-class="transition duration-150 ease-out"
      enter-from-class="opacity-0 -translate-y-1"
      enter-to-class="opacity-100 translate-y-0"
      leave-active-class="transition duration-100 ease-in"
      leave-from-class="opacity-100 translate-y-0"
      leave-to-class="opacity-0 -translate-y-1"
    >
      <div v-if="open" class="mt-2 rounded-xl border border-white/10 bg-white/5 p-4 md:p-6">
        <div class="grid grid-cols-1 md:grid-cols-3 gap-6">
          <!-- Genres (AND) -->
          <section v-if="facets.genres.length">
            <header class="flex items-center justify-between mb-2">
              <span class="text-sm font-semibold text-white">{{ $t('profile.filters.genres') }}</span>
              <span class="text-xs text-muted-foreground">{{ $t('profile.filters.genresHint') }}</span>
            </header>
            <Input
              v-if="facets.genres.length > 8"
              v-model="genreSearch"
              size="sm"
              :placeholder="$t('profile.filters.searchGenres')"
              class="mb-2"
            />
            <ul class="space-y-0.5 max-h-64 overflow-y-auto pr-1">
              <li v-for="g in filteredGenres" :key="g.id">
                <label class="flex items-center gap-2 px-1 py-1 rounded-md hover:bg-white/5 cursor-pointer">
                  <Checkbox :model-value="genreIds.includes(g.id)" @update:model-value="() => toggleGenre(g.id)" />
                  <span class="text-sm text-white/90 flex-1 truncate">{{ localizedGenre(g) }}</span>
                  <span class="text-xs text-muted-foreground tabular-nums">{{ g.count }}</span>
                </label>
              </li>
            </ul>
          </section>

          <!-- Types (OR) -->
          <section v-if="facets.kinds.length">
            <header class="flex items-center justify-between mb-2">
              <span class="text-sm font-semibold text-white">{{ $t('profile.filters.types') }}</span>
              <span class="text-xs text-muted-foreground">{{ $t('profile.filters.typesHint') }}</span>
            </header>
            <ul class="space-y-0.5 max-h-64 overflow-y-auto pr-1">
              <li v-for="k in facets.kinds" :key="k.kind">
                <label class="flex items-center gap-2 px-1 py-1 rounded-md hover:bg-white/5 cursor-pointer">
                  <Checkbox :model-value="kinds.includes(k.kind)" @update:model-value="() => toggleKind(k.kind)" />
                  <span class="text-sm text-white/90 flex-1">{{ $t('profile.filters.kind.' + k.kind) }}</span>
                  <span class="text-xs text-muted-foreground tabular-nums">{{ k.count }}</span>
                </label>
              </li>
            </ul>
          </section>

          <!-- Year range -->
          <section v-if="facets.years.min !== null && facets.years.max !== null">
            <header class="mb-2">
              <span class="text-sm font-semibold text-white">{{ $t('profile.filters.year') }}</span>
            </header>
            <div class="flex items-center gap-2">
              <Select :model-value="yearMinStr" :options="yearMinOptions" size="sm" class="flex-1"
                @update:model-value="(v) => emitYear('min', v as string)" />
              <span class="text-white/40">—</span>
              <Select :model-value="yearMaxStr" :options="yearMaxOptions" size="sm" class="flex-1"
                @update:model-value="(v) => emitYear('max', v as string)" />
            </div>
          </section>
        </div>

        <!-- Clear all -->
        <div class="mt-4 pt-3 border-t border-white/10 flex justify-end">
          <Button variant="ghost" size="sm" class="text-muted-foreground" :disabled="count === 0" @click="clearAll">
            {{ $t('profile.filters.clear') }}
          </Button>
        </div>
      </div>
    </Transition>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { SlidersHorizontal, ChevronDown } from 'lucide-vue-next'
import Button from '@/components/ui/Button.vue'
import Badge from '@/components/ui/Badge.vue'
import Checkbox from '@/components/ui/Checkbox.vue'
import Input from '@/components/ui/Input.vue'
import Select from '@/components/ui/Select.vue'
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

const { locale } = useI18n()
const open = ref(false)
const genreSearch = ref('')

const count = computed(() =>
  activeFilterCount({ genreIds: props.genreIds, kinds: props.kinds, yearMin: props.yearMin, yearMax: props.yearMax }),
)

function localizedGenre(g: FacetGenre): string {
  const loc = locale.value || ''
  return loc.startsWith('ru') && g.name_ru ? g.name_ru : g.name
}

const filteredGenres = computed(() => {
  const q = genreSearch.value.trim().toLowerCase()
  if (!q) return props.facets.genres
  return props.facets.genres.filter((g) => localizedGenre(g).toLowerCase().includes(q))
})

function toggleGenre(id: string) {
  const next = props.genreIds.includes(id)
    ? props.genreIds.filter((x) => x !== id)
    : [...props.genreIds, id]
  emit('update:genreIds', next)
}

function toggleKind(kind: string) {
  const next = props.kinds.includes(kind)
    ? props.kinds.filter((x) => x !== kind)
    : [...props.kinds, kind]
  emit('update:kinds', next)
}

const years = computed(() => {
  const lo = props.facets.years.min
  const hi = props.facets.years.max
  if (lo === null || hi === null) return []
  const out: number[] = []
  for (let y = hi; y >= lo; y--) out.push(y)
  return out
})

// reka-ui's SelectItem forbids an empty-string value (it's reserved for
// clearing the selection), so the "any year" option uses a non-empty sentinel.
const ANY_YEAR = 'any'

const yearMinStr = computed(() => (props.yearMin === null ? ANY_YEAR : String(props.yearMin)))
const yearMaxStr = computed(() => (props.yearMax === null ? ANY_YEAR : String(props.yearMax)))

const yearMinOptions = computed(() => [
  { value: ANY_YEAR, label: '—' },
  ...years.value.map((y) => ({ value: String(y), label: String(y) })),
])
const yearMaxOptions = computed(() => [
  { value: ANY_YEAR, label: '—' },
  ...years.value.map((y) => ({ value: String(y), label: String(y) })),
])

function emitYear(which: 'min' | 'max', v: string) {
  const n = v === ANY_YEAR || v === '' ? null : Number(v)
  if (which === 'min') emit('update:yearMin', n)
  else emit('update:yearMax', n)
}

function clearAll() {
  emit('update:genreIds', [])
  emit('update:kinds', [])
  emit('update:yearMin', null)
  emit('update:yearMax', null)
}
</script>
