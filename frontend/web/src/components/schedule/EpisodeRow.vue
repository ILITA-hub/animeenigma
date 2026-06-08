<!-- frontend/web/src/components/schedule/EpisodeRow.vue -->
<template>
  <div class="flex items-center gap-2 py-1.5 border-t border-white/5 first:border-t-0 first:pt-0">
    <img
      :src="occurrence.anime.poster_url || '/placeholder.svg'"
      :alt="title"
      class="w-7 h-10 rounded object-cover flex-none bg-muted"
      loading="lazy"
    />
    <div class="min-w-0 flex-1">
      <div class="text-[10px] leading-tight font-medium text-foreground line-clamp-2">{{ title }}</div>
      <div class="text-[9px] text-muted-foreground mt-0.5">
        <span class="text-primary font-semibold">{{ occurrence.episode }}</span>
        {{ $t('schedule.episodeShort') }} · {{ time }}
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { Occurrence } from '@/composables/schedule/types'
import { getLocalizedTitle } from '@/utils/title'
import { formatAirTime } from '@/composables/schedule/format'

const props = defineProps<{ occurrence: Occurrence }>()
const title = computed(() => getLocalizedTitle(props.occurrence.anime.name, props.occurrence.anime.name_ru, props.occurrence.anime.name_jp))
const time = computed(() => formatAirTime(props.occurrence.date))
</script>
