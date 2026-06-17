<script setup lang="ts">
import { ref, onMounted } from 'vue'
import type { FavoriteAnimeConfig } from '@/types/showcase'
import { animeApi } from '@/api/client'
import { fromHomeAnime } from '@/utils/toCardModel'
import PosterCard from '@/components/anime/PosterCard.vue'
import type { AnimeCardModel } from '@/types/card'

const props = defineProps<{ config: FavoriteAnimeConfig }>()

// Raw API anime shape from getById
interface ApiAnime {
  id: string
  name?: string
  name_ru?: string
  name_jp?: string
  poster_url?: string
  score?: number
  status?: string
  episodes_count?: number
  episodes_aired?: number
  year?: number
  next_episode_at?: string
}

const items = ref<AnimeCardModel[]>([])

onMounted(async () => {
  const ids = props.config.anime_ids ?? []
  if (!ids.length) return
  try {
    const results = await Promise.all(
      ids.map((id) =>
        animeApi
          .getById(id)
          .then((r) => {
            const raw = r.data as { data?: ApiAnime } & ApiAnime
            return ('data' in raw && raw.data ? raw.data : raw) as ApiAnime
          })
          .catch(() => null),
      ),
    )
    items.value = results
      .filter((a): a is ApiAnime => a !== null)
      .map((a) => fromHomeAnime(a))
  } catch {
    items.value = []
  }
})
</script>

<template>
  <div class="rounded-xl border border-border bg-card p-4 md:p-6">
    <h3 class="mb-3 text-lg font-semibold text-foreground">{{ $t('showcase.block.favorite_anime') }}</h3>
    <div v-if="items.length" class="grid grid-cols-3 gap-3 sm:grid-cols-4 md:grid-cols-6">
      <PosterCard v-for="a in items" :key="a.id" :model="a" />
    </div>
    <p v-else class="text-sm text-muted-foreground">{{ $t('showcase.empty') }}</p>
  </div>
</template>
