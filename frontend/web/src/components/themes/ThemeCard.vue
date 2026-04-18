<template>
  <div
    class="glass-card rounded-xl overflow-hidden transition-all duration-200 hover:ring-1 hover:ring-cyan-500/30"
  >
    <!-- Card Header -->
    <div class="flex gap-3 p-4 cursor-pointer" @click="expanded = !expanded">
      <!-- Poster -->
      <div class="w-16 h-22 flex-shrink-0 rounded-lg overflow-hidden bg-white/5">
        <img
          v-if="theme.poster_url"
          :src="theme.poster_url"
          :alt="theme.anime_name"
          class="w-full h-full object-cover"
          loading="lazy"
        />
        <div v-else class="w-full h-full flex items-center justify-center text-white/20">
          <svg class="w-8 h-8" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M15.91 11.672a.375.375 0 010 .656l-5.603 3.113a.375.375 0 01-.557-.328V8.887c0-.286.307-.466.557-.327l5.603 3.112z" />
          </svg>
        </div>
      </div>

      <!-- Info -->
      <div class="flex-1 min-w-0">
        <div class="flex items-start justify-between gap-2">
          <div class="min-w-0">
            <router-link
              v-if="theme.anime_id"
              :to="`/anime/${theme.anime_id}`"
              class="inline-flex items-center gap-1 text-cyan-400 font-medium text-sm leading-tight hover:text-cyan-300 transition-colors max-w-full"
              @click.stop
            >
              <span class="truncate">{{ theme.anime_name }}</span>
              <svg class="w-3.5 h-3.5 flex-shrink-0 opacity-60" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14" />
              </svg>
            </router-link>
            <h3 v-else class="text-white/90 font-medium text-sm leading-tight truncate">{{ theme.anime_name }}</h3>
            <div class="flex items-center gap-2 mt-1">
              <span
                class="inline-flex items-center px-2 py-0.5 rounded text-xs font-bold"
                :class="theme.theme_type === 'OP'
                  ? 'bg-cyan-500/20 text-cyan-400'
                  : 'bg-purple-500/20 text-purple-400'"
              >
                {{ theme.slug || theme.theme_type }}
              </span>
            </div>
            <p v-if="theme.song_title" class="text-white/80 text-xs mt-1 truncate">
              {{ theme.song_title }}
              <span v-if="theme.artist_name" class="text-white/70"> - {{ theme.artist_name }}</span>
            </p>
          </div>

          <!-- Rating display -->
          <div class="flex flex-col items-end flex-shrink-0">
            <div class="flex items-center gap-1">
              <svg class="w-4 h-4 text-yellow-400" fill="currentColor" viewBox="0 0 20 20">
                <path d="M9.049 2.927c.3-.921 1.603-.921 1.902 0l1.07 3.292a1 1 0 00.95.69h3.462c.969 0 1.371 1.24.588 1.81l-2.8 2.034a1 1 0 00-.364 1.118l1.07 3.292c.3.921-.755 1.688-1.54 1.118l-2.8-2.034a1 1 0 00-1.175 0l-2.8 2.034c-.784.57-1.838-.197-1.539-1.118l1.07-3.292a1 1 0 00-.364-1.118L2.98 8.72c-.783-.57-.38-1.81.588-1.81h3.461a1 1 0 00.951-.69l1.07-3.292z"/>
              </svg>
              <span class="text-white font-medium text-sm">
                {{ theme.avg_score > 0 ? theme.avg_score.toFixed(1) : '-' }}
              </span>
            </div>
            <span class="text-white/80 text-xs">{{ theme.vote_count }} votes</span>
          </div>
        </div>
      </div>

      <!-- Expand arrow -->
      <div class="flex items-center">
        <svg
          class="w-5 h-5 text-white/60 transition-transform duration-200"
          :class="{ 'rotate-180': expanded }"
          fill="none"
          stroke="currentColor"
          viewBox="0 0 24 24"
        >
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
        </svg>
      </div>
    </div>

    <!-- Expanded Content -->
    <Transition name="expand">
      <div v-if="expanded" class="px-4 pb-4 space-y-3">
        <!-- Video Player -->
        <div v-if="theme.video_basename" class="rounded-lg overflow-hidden bg-black">
          <video
            ref="videoEl"
            class="w-full max-h-64"
            controls
            preload="none"
            :poster="theme.poster_url"
          >
            <source :src="`/api/themes/video/${theme.video_basename}`" type="video/webm" />
          </video>
        </div>
        <p v-else class="text-white/60 text-sm text-center py-4">No video available</p>

        <!-- User Rating -->
        <div v-if="isAuthenticated" class="pt-1">
          <p class="text-white/70 text-xs mb-2">Your rating:</p>
          <RatingStars
            :model-value="theme.user_score"
            @update:model-value="$emit('rate', $event)"
            @remove="$emit('unrate')"
          />
        </div>
      </div>
    </Transition>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useAuthStore } from '@/stores/auth'
import RatingStars from './RatingStars.vue'

const authStore = useAuthStore()
const isAuthenticated = authStore.isAuthenticated

defineProps<{
  theme: {
    id: string
    anime_name: string
    anime_slug: string
    anime_id?: string
    poster_url: string
    theme_type: string
    slug: string
    song_title: string
    artist_name: string
    video_basename: string
    avg_score: number
    vote_count: number
    user_score?: number | null
  }
}>()

defineEmits<{
  (e: 'rate', score: number): void
  (e: 'unrate'): void
}>()

const expanded = ref(false)
const videoEl = ref<HTMLVideoElement | null>(null)
</script>

<style scoped>
.expand-enter-active,
.expand-leave-active {
  transition: all 0.2s ease;
  overflow: hidden;
}

.expand-enter-from,
.expand-leave-to {
  opacity: 0;
  max-height: 0;
}

.expand-enter-to,
.expand-leave-from {
  opacity: 1;
  max-height: 500px;
}
</style>
