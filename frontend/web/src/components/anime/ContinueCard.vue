<template>
  <router-link
    :to="`/watch/${anime.id}/${currentEpisode}`"
    class="group block"
  >
    <div class="card-hover rounded-xl overflow-hidden bg-white/5 border border-white/10">
      <!-- Poster Container -->
      <div class="relative aspect-[2/3] overflow-hidden bg-surface">
        <!-- Image -->
        <img
          :src="anime.coverImage"
          :alt="anime.title"
          class="w-full h-full object-cover transition-transform duration-300 group-hover:scale-110"
          loading="lazy"
        />

        <!-- Overlay Gradient -->
        <div class="absolute inset-0 bg-gradient-to-t from-black/80 via-transparent to-transparent" />

        <!-- Episode Badge -->
        <div class="absolute top-2 left-2">
          <Badge variant="primary" size="sm">
            EP {{ currentEpisode }}
          </Badge>
        </div>

        <!-- Play Button Overlay -->
        <div class="absolute inset-0 flex items-center justify-center opacity-0 group-hover:opacity-100 transition-opacity">
          <div class="w-14 h-14 rounded-full bg-cyan-500/90 flex items-center justify-center shadow-[0_0_20px_rgba(0,212,255,0.5)]">
            <svg class="w-6 h-6 text-white ml-1" fill="currentColor" viewBox="0 0 24 24">
              <path d="M8 5v14l11-7z" />
            </svg>
          </div>
        </div>

        <!-- Progress Bar -->
        <div class="absolute bottom-0 left-0 right-0 h-1 bg-white/20">
          <div
            class="h-full bg-cyan-400 transition-all"
            :style="{ width: `${progress}%` }"
          />
        </div>
      </div>

      <!-- Card Content -->
      <div class="p-3">
        <!-- Title -->
        <h3 class="font-medium text-white line-clamp-1 mb-1 group-hover:text-cyan-400 transition-colors">
          {{ anime.title }}
        </h3>

        <!-- Episode Info -->
        <p class="text-xs text-white/50">
          {{ $t('anime.episode') }} {{ currentEpisode }} {{ $t('anime.of') }} {{ totalEpisodes }}
        </p>
      </div>
    </div>
  </router-link>
</template>

<script setup lang="ts">
import Badge from '@/components/ui/Badge.vue'

interface Anime {
  id: string | number
  title: string
  coverImage: string
}

defineProps<{
  anime: Anime
  currentEpisode: number
  totalEpisodes: number
  progress: number // 0-100
}>()
</script>
