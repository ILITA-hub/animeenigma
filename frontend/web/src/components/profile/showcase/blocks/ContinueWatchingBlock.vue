<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { publicApi } from '@/api/client'
import { getLocalizedTitle } from '@/utils/title'
import { getImageUrl } from '@/composables/useImageProxy'

const props = defineProps<{ userId: string; config?: unknown; isOwner?: boolean }>()

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
    const raw = res.data as { data?: WatchingAnime[] } & { items?: WatchingAnime[] } & WatchingAnime[]
    const list: WatchingAnime[] = Array.isArray(raw)
      ? raw
      : ('data' in raw && Array.isArray(raw.data))
        ? raw.data as WatchingAnime[]
        : ('items' in raw && Array.isArray(raw.items))
          ? raw.items as WatchingAnime[]
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
        <img
          :src="getImageUrl(entry.anime?.poster_url) || ''"
          :alt="getLocalizedTitle(entry.anime?.name, entry.anime?.name_ru, entry.anime?.name_jp) || ''"
          class="h-14 w-10 shrink-0 rounded-md object-cover"
        />
        <span class="min-w-0 truncate text-sm font-medium text-foreground" data-testid="cw-title">
          {{ getLocalizedTitle(entry.anime?.name, entry.anime?.name_ru, entry.anime?.name_jp) || entry.anime?.name }}
        </span>
      </div>
    </div>
  </div>
</template>
