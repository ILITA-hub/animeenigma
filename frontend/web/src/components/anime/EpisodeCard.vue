<template>
  <button
    class="group relative aspect-video w-full rounded-lg overflow-hidden bg-surface border border-white/10 transition-all hover:border-cyan-500/50"
    :class="{ 'ring-2 ring-cyan-400': isActive }"
    @click="$emit('select')"
  >
    <!-- Thumbnail -->
    <img
      v-if="thumbnail"
      :src="thumbnail"
      :alt="`Episode ${episodeNumber}`"
      class="w-full h-full object-cover transition-transform duration-300 group-hover:scale-105"
      loading="lazy"
    />
    <div
      v-else
      class="w-full h-full bg-gradient-to-br from-white/5 to-white/10 flex items-center justify-center"
    >
      <span class="text-3xl font-semibold text-white/20">{{ episodeNumber }}</span>
    </div>

    <!-- Overlay -->
    <div class="absolute inset-0 bg-gradient-to-t from-black/80 via-black/20 to-transparent" />

    <!-- Watched Indicator -->
    <div
      v-if="watched"
      class="absolute top-2 right-2 w-5 h-5 rounded-full bg-success flex items-center justify-center"
    >
      <Check class="size-3 text-white" aria-hidden="true" />
    </div>

    <!-- Progress Bar (for partially watched) -->
    <div
      v-if="progress && progress > 0 && progress < 100"
      class="absolute bottom-0 left-0 right-0 h-1 bg-white/20"
    >
      <div
        class="h-full bg-cyan-400"
        :style="{ width: `${progress}%` }"
      />
    </div>

    <!-- Episode Info -->
    <div class="absolute bottom-0 left-0 right-0 p-3">
      <div class="flex items-center gap-2">
        <Badge variant="default" size="sm">
          {{ episodeNumber }}
        </Badge>
        <span v-if="duration" class="text-xs text-white/60">
          {{ formatDuration(duration) }}
        </span>
      </div>
      <h4 v-if="title" class="text-sm font-medium text-white line-clamp-1 mt-1">
        {{ title }}
      </h4>
    </div>

    <!-- Play Icon on Hover -->
    <div class="absolute inset-0 flex items-center justify-center opacity-0 group-hover:opacity-100 transition-opacity">
      <div class="w-12 h-12 rounded-full bg-cyan-500/90 flex items-center justify-center shadow-[0_0_20px_var(--cyan-a40)]">
        <Play class="size-5 text-white ml-0.5" fill="currentColor" aria-hidden="true" />
      </div>
    </div>
  </button>
</template>

<script setup lang="ts">
import { Check, Play } from 'lucide-vue-next'
import Badge from '@/components/ui/Badge.vue'

defineProps<{
  episodeNumber: number
  title?: string
  thumbnail?: string
  duration?: number // in seconds
  watched?: boolean
  progress?: number // 0-100
  isActive?: boolean
}>()

defineEmits<{
  select: []
}>()

const formatDuration = (seconds: number): string => {
  const mins = Math.floor(seconds / 60)
  return `${mins} min`
}
</script>
