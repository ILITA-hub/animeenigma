<!-- frontend/web/src/components/schedule/EpisodeRow.vue -->
<template>
  <div
    class="flex items-center border-t border-white/5 first:border-t-0 first:pt-0"
    :class="size === 'lg' ? 'gap-3 py-2' : 'gap-2 py-1.5'"
  >
    <img
      :src="occurrence.anime.poster_url || '/placeholder.svg'"
      :alt="title"
      class="rounded object-cover flex-none bg-muted"
      :class="size === 'lg' ? 'w-12 h-16' : 'w-7 h-10'"
      loading="lazy"
    />
    <div class="min-w-0 flex-1">
      <div
        class="font-medium text-foreground line-clamp-2"
        :class="size === 'lg' ? 'text-xs leading-snug' : 'text-[10px] leading-tight'"
      >{{ title }}</div>
      <div class="text-muted-foreground" :class="size === 'lg' ? 'text-[11px] mt-1' : 'text-[9px] mt-0.5'">
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

const props = withDefaults(defineProps<{ occurrence: Occurrence; size?: 'sm' | 'lg' }>(), { size: 'sm' })
const title = computed(() => getLocalizedTitle(props.occurrence.anime.name, props.occurrence.anime.name_ru, props.occurrence.anime.name_jp))
const time = computed(() => formatAirTime(props.occurrence.date))
</script>
