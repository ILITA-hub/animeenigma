<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { publicApi } from '@/api/client'

const props = defineProps<{ userId: string }>()

interface WatchlistStats {
  total_entries?: number
  avg_score?: number
  total_episodes?: number
  completed?: number
}

const stats = ref<WatchlistStats | null>(null)

onMounted(async () => {
  try {
    const res = await publicApi.getPublicWatchlistStats(props.userId)
    const data = (res.data as { data?: WatchlistStats } & WatchlistStats)
    stats.value = ('data' in data && data.data ? data.data : data) as WatchlistStats
  } catch {
    stats.value = null
  }
})
</script>

<template>
  <div class="rounded-xl border border-border bg-card p-4 md:p-6">
    <h3 class="mb-3 text-lg font-semibold text-foreground">{{ $t('showcase.block.stats') }}</h3>
    <div class="grid grid-cols-2 gap-3 sm:grid-cols-4">
      <div class="text-center">
        <div class="text-xl font-semibold text-foreground">{{ stats?.total_entries ?? 0 }}</div>
        <div class="text-xs text-muted-foreground">{{ $t('profile.stats.totalAnime') }}</div>
      </div>
      <div class="text-center">
        <div class="text-xl font-semibold text-foreground">
          {{ stats?.avg_score && stats.avg_score > 0 ? stats.avg_score.toFixed(1) : '-' }}
        </div>
        <div class="text-xs text-muted-foreground">{{ $t('profile.stats.avgScore') }}</div>
      </div>
      <div class="text-center">
        <div class="text-xl font-semibold text-foreground">{{ stats?.total_episodes ?? 0 }}</div>
        <div class="text-xs text-muted-foreground">{{ $t('profile.stats.episodesWatched') }}</div>
      </div>
      <div class="text-center">
        <div class="text-xl font-semibold text-foreground">{{ stats?.completed ?? 0 }}</div>
        <div class="text-xs text-muted-foreground">{{ $t('profile.stats.completed') }}</div>
      </div>
    </div>
  </div>
</template>
