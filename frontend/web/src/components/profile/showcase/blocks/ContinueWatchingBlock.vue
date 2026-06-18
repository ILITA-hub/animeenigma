<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { publicApi } from '@/api/client'
import { getLocalizedTitle } from '@/utils/title'
import PosterImage from '@/components/anime/PosterImage.vue'

const props = defineProps<{ userId: string; config?: unknown; isOwner?: boolean; variant?: string }>()

interface WatchingAnime {
  anime_id: string
  anime?: {
    name: string
    name_ru?: string
    name_jp?: string
    poster_url?: string
  }
}

const items = ref<WatchingAnime[]>([])

onMounted(async () => {
  try {
    const res = await publicApi.getPublicWatchlist(props.userId, { status: 'watching', per_page: 12 })
    type WatchlistResponse = { data?: WatchingAnime[] } & { items?: WatchingAnime[] } & WatchingAnime[]
    const raw = res.data as unknown as WatchlistResponse
    const list: WatchingAnime[] = Array.isArray(raw)
      ? raw
      : ('data' in raw && Array.isArray((raw as { data?: WatchingAnime[] }).data))
        ? (raw as { data: WatchingAnime[] }).data
        : ('items' in raw && Array.isArray((raw as { items?: WatchingAnime[] }).items))
          ? (raw as { items: WatchingAnime[] }).items
          : []
    items.value = list
  } catch {
    items.value = []
  }
})
</script>

<template>
  <div v-if="items.length" class="rounded-xl border border-border bg-card p-4 md:p-6">
    <h3 class="mb-3 text-lg font-semibold text-foreground">{{ $t('showcase.block.continue_watching') }}</h3>
    <div class="cw flex flex-col gap-2">
      <div
        v-for="entry in items"
        :key="entry.anime_id"
        class="cwc flex items-center gap-3 rounded-lg border border-border bg-white/[0.02] px-3 py-2"
        data-testid="cw-card"
      >
        <PosterImage
          :src="entry.anime?.poster_url || '/placeholder.svg'"
          :alt="getLocalizedTitle(entry.anime?.name, entry.anime?.name_ru, entry.anime?.name_jp) || ''"
          ratio="2/3"
          rounded="md"
          :proxy-width="128"
          class="w-10 shrink-0"
        />
        <span class="min-w-0 truncate text-sm font-medium text-foreground" data-testid="cw-title">
          {{ getLocalizedTitle(entry.anime?.name, entry.anime?.name_ru, entry.anime?.name_jp) || entry.anime?.name }}
        </span>
      </div>
    </div>
  </div>
</template>
