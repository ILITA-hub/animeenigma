<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { publicApi } from '@/api/client'
import type { FacetGenre, WatchlistFacets } from '@/types/watchlist-facets'
import { useI18n } from 'vue-i18n'

const props = defineProps<{ userId: string; config?: unknown; isOwner?: boolean }>()

const { locale } = useI18n()

const genres = ref<FacetGenre[]>([])

onMounted(async () => {
  try {
    const res = await publicApi.getPublicWatchlistFacets(props.userId)
    const raw = res.data as { data?: WatchlistFacets } & WatchlistFacets
    const facets: WatchlistFacets = ('data' in raw && raw.data ? raw.data : raw) as WatchlistFacets
    const sorted = [...(facets.genres ?? [])].sort((a, b) => b.count - a.count).slice(0, 6)
    genres.value = sorted
  } catch {
    genres.value = []
  }
})

const maxCount = computed(() => Math.max(...genres.value.map((g) => g.count), 1))

function genreLabel(g: FacetGenre): string {
  return locale.value === 'ru' && g.name_ru ? g.name_ru : g.name
}
</script>

<template>
  <div v-if="genres.length" class="rounded-xl border border-border bg-card p-4 md:p-6">
    <h3 class="mb-3 text-lg font-semibold text-foreground">{{ $t('showcase.block.anime_dna') }}</h3>
    <div class="dna flex flex-col gap-2">
      <div
        v-for="g in genres"
        :key="g.id"
        class="drow"
        data-testid="dna-bar"
      >
        <div class="mb-1 flex items-center justify-between gap-2">
          <span class="text-xs font-medium text-foreground" data-testid="dna-genre-name">{{ genreLabel(g) }}</span>
          <span class="text-xs font-medium text-muted-foreground">{{ g.count }}</span>
        </div>
        <div class="h-[6px] w-full overflow-hidden rounded-full bg-white/[0.06]">
          <div
            class="h-full rounded-full"
            :style="{
              width: `${Math.round((g.count / maxCount) * 100)}%`,
              background: 'linear-gradient(90deg, var(--brand-cyan), var(--brand-violet))',
            }"
          />
        </div>
      </div>
    </div>
  </div>
</template>
