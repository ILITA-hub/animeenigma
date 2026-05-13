<template>
  <!-- Phase 8 (UX-15 / UA-061). Hidden when no items so logged-in users
       with zero in-progress rows see no degraded affordance — the
       trending row below remains the top-of-home anchor in that case.
       Anonymous users get `items.length === 0` from the composable's
       early-return path. -->
  <div v-if="items.length > 0" class="px-4 lg:px-8 max-w-7xl mx-auto mb-8">
    <div class="flex items-center justify-between mb-4">
      <h2 class="text-xl md:text-2xl font-bold text-white">
        {{ $t('home.continueWatching') }}
      </h2>
    </div>
    <div class="flex gap-3 overflow-x-auto scrollbar-hide pb-2 -mx-1 px-1">
      <router-link
        v-for="item in items"
        :key="item.anime.id + ':' + item.episode_number"
        :to="`/anime/${item.anime.id}?episode=${item.episode_number}`"
        class="flex-shrink-0 w-32 md:w-40 lg:w-48 group"
      >
        <div class="relative rounded-xl overflow-hidden bg-white/5 aspect-[2/3] mb-2">
          <img
            :src="item.anime.poster_url || '/placeholder.svg'"
            alt=""
            class="w-full h-full object-cover group-hover:scale-105 transition-transform duration-300"
            loading="lazy"
          />
          <!-- Episode badge -->
          <div class="absolute top-2 right-2 px-2 py-1 rounded-md bg-black/70 backdrop-blur-sm text-xs font-semibold text-white">
            {{ $t('home.continueWatchingEpisode', { n: item.episode_number }) }}
          </div>
          <!-- Thin progress bar at the bottom of the poster -->
          <div class="absolute bottom-0 left-0 right-0 h-[2px] bg-white/10">
            <div
              class="h-full bg-cyan-400 transition-all"
              :style="{ width: progressPct(item) + '%' }"
            />
          </div>
        </div>
        <h3 class="text-sm font-medium text-white truncate group-hover:text-cyan-400 transition-colors">
          {{ getLocalizedTitle(item.anime.name, item.anime.name_ru, item.anime.name_jp) }}
        </h3>
      </router-link>
    </div>
  </div>
  <!-- Loading skeleton — matches the trending-row loading skeleton at
       Home.vue lines 89-97 for visual consistency. -->
  <div v-else-if="isLoading" class="px-4 lg:px-8 max-w-7xl mx-auto mb-8">
    <div class="h-8 w-48 bg-white/10 rounded animate-pulse mb-4" />
    <div class="flex gap-3 overflow-hidden">
      <div
        v-for="i in 6"
        :key="i"
        class="flex-shrink-0 w-32 md:w-40 lg:w-48 aspect-[2/3] bg-white/10 rounded-xl animate-pulse"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { getLocalizedTitle } from '@/utils/title'
import { useContinueWatching, type ContinueWatchingItem } from '@/composables/useContinueWatching'

const { items, isLoading } = useContinueWatching(10)

function progressPct(item: ContinueWatchingItem): number {
  if (!item.duration || item.duration <= 0) return 0
  const pct = (item.progress / item.duration) * 100
  // Cap at 100 in case progress > duration (clock skew between heartbeats).
  return Math.max(0, Math.min(100, pct))
}
</script>
