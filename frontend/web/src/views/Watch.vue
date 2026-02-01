<template>
  <div class="min-h-screen bg-black">
    <!-- Video Player Section -->
    <div class="relative aspect-video max-h-[70vh] bg-black">
      <VideoPlayer v-if="episode" :episode="episode" />
      <div v-else-if="loading" class="absolute inset-0 flex items-center justify-center">
        <div class="w-12 h-12 border-2 border-cyan-400 border-t-transparent rounded-full animate-spin" />
      </div>
      <div v-else-if="error" class="absolute inset-0 flex items-center justify-center">
        <div class="text-center">
          <p class="text-pink-400 mb-4">{{ error }}</p>
          <Button variant="outline" @click="loadEpisode">{{ $t('common.retry') }}</Button>
        </div>
      </div>
    </div>

    <!-- Episode Info Bar -->
    <div class="bg-surface border-b border-white/10">
      <div class="max-w-7xl mx-auto px-4 py-4">
        <div class="flex flex-col md:flex-row md:items-center md:justify-between gap-4">
          <!-- Episode Details -->
          <div class="flex-1 min-w-0">
            <h1 class="text-lg md:text-xl font-semibold text-white truncate">
              {{ episode?.title || `Episode ${episode?.episodeNumber}` }}
            </h1>
            <router-link
              v-if="anime"
              :to="`/anime/${animeId}`"
              class="text-white/60 hover:text-cyan-400 transition-colors inline-flex items-center gap-2"
            >
              <span class="truncate">{{ anime.title }}</span>
              <span class="text-white/30">•</span>
              <span>{{ $t('anime.episode') }} {{ episode?.episodeNumber }} {{ $t('anime.of') }} {{ anime.totalEpisodes }}</span>
            </router-link>
          </div>

          <!-- Navigation Controls -->
          <div class="flex items-center gap-3">
            <Button
              size="sm"
              variant="ghost"
              :disabled="!hasPrevious"
              @click="previousEpisode"
            >
              <template #icon>
                <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 19l-7-7 7-7" />
                </svg>
              </template>
              {{ $t('player.previous') }}
            </Button>
            <Button
              size="sm"
              variant="ghost"
              :disabled="!hasNext"
              @click="nextEpisode"
            >
              {{ $t('player.next') }}
              <svg class="w-4 h-4 ml-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
              </svg>
            </Button>
          </div>
        </div>
      </div>
    </div>

    <!-- Content Area -->
    <div class="max-w-7xl mx-auto px-4 py-6 space-y-8">
      <!-- Episodes Section -->
      <section>
        <h2 class="text-xl font-semibold text-white mb-4">{{ $t('anime.episodes') }}</h2>
        <div class="flex gap-3 overflow-x-auto scrollbar-hide pb-2 snap-x-mandatory">
          <button
            v-for="ep in episodes"
            :key="ep.id"
            class="flex-shrink-0 w-28 p-3 rounded-xl text-left transition-all snap-start"
            :class="[
              ep.id === episodeId
                ? 'bg-cyan-500/20 border-2 border-cyan-500/50 text-cyan-400'
                : 'bg-white/5 border border-white/10 text-white/70 hover:bg-white/10 hover:text-white'
            ]"
            @click="selectEpisode(ep.id)"
          >
            <div class="text-2xl font-bold mb-1">{{ ep.episodeNumber }}</div>
            <div class="text-xs truncate opacity-70">{{ ep.title || `EP ${ep.episodeNumber}` }}</div>
          </button>
        </div>
      </section>

      <!-- Anime Info Section -->
      <section v-if="anime" class="glass-card p-6">
        <div class="flex gap-4">
          <router-link
            :to="`/anime/${animeId}`"
            class="flex-shrink-0 w-20 aspect-[2/3] rounded-lg overflow-hidden"
          >
            <img
              :src="anime.coverImage"
              :alt="anime.title"
              class="w-full h-full object-cover"
            />
          </router-link>
          <div class="flex-1 min-w-0">
            <router-link
              :to="`/anime/${animeId}`"
              class="text-lg font-semibold text-white hover:text-cyan-400 transition-colors"
            >
              {{ anime.title }}
            </router-link>
            <div class="flex flex-wrap gap-2 mt-2">
              <Badge v-if="anime.rating" variant="rating" size="sm">
                <svg class="w-3 h-3 mr-1" fill="currentColor" viewBox="0 0 20 20">
                  <path d="M9.049 2.927c.3-.921 1.603-.921 1.902 0l1.07 3.292a1 1 0 00.95.69h3.462c.969 0 1.371 1.24.588 1.81l-2.8 2.034a1 1 0 00-.364 1.118l1.07 3.292c.3.921-.755 1.688-1.54 1.118l-2.8-2.034a1 1 0 00-1.175 0l-2.8 2.034c-.784.57-1.838-.197-1.539-1.118l1.07-3.292a1 1 0 00-.364-1.118L2.98 8.72c-.783-.57-.38-1.81.588-1.81h3.461a1 1 0 00.951-.69l1.07-3.292z" />
                </svg>
                {{ anime.rating.toFixed(1) }}
              </Badge>
              <Badge variant="default" size="sm">{{ anime.releaseYear }}</Badge>
              <Badge variant="default" size="sm">{{ anime.totalEpisodes }} eps</Badge>
            </div>
            <p class="text-white/60 text-sm mt-3 line-clamp-2">
              {{ anime.description }}
            </p>
            <div class="flex flex-wrap gap-2 mt-3">
              <span
                v-for="genre in anime.genres?.slice(0, 3)"
                :key="genre"
                class="text-xs px-2 py-1 rounded-md bg-white/5 text-white/50"
              >
                {{ genre }}
              </span>
            </div>
          </div>
        </div>
      </section>

      <!-- Player Settings -->
      <section class="glass-card p-4">
        <div class="flex flex-wrap items-center gap-4">
          <div class="flex items-center gap-2">
            <span class="text-white/60 text-sm">{{ $t('player.autoplay') }}</span>
            <button
              class="w-10 h-6 rounded-full transition-colors relative"
              :class="autoplay ? 'bg-cyan-500' : 'bg-white/20'"
              @click="autoplay = !autoplay"
            >
              <span
                class="absolute top-1 w-4 h-4 rounded-full bg-white transition-transform"
                :class="autoplay ? 'left-5' : 'left-1'"
              />
            </button>
          </div>
          <div class="flex items-center gap-2">
            <span class="text-white/60 text-sm">{{ $t('player.quality') }}</span>
            <div class="w-24">
              <Select
                v-model="quality"
                :options="qualityOptions"
                size="xs"
              />
            </div>
          </div>
        </div>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch as vueWatch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAnime } from '@/composables/useAnime'
import { usePlayerStore, type Episode as PlayerEpisode } from '@/stores/player'
import VideoPlayer from '@/components/player/VideoPlayer.vue'
import { Button, Badge, Select } from '@/components/ui'

const route = useRoute()
const router = useRouter()
const { anime, episodes, loading, error, fetchAnime, fetchEpisodes, fetchEpisode } = useAnime()
const playerStore = usePlayerStore()

const animeId = ref(route.params.animeId as string)
const episodeId = ref(route.params.episodeId as string)
const episode = ref<PlayerEpisode | null>(null)
const autoplay = ref(false)
const quality = ref('auto')

const qualityOptions = [
  { value: 'auto', label: 'Auto' },
  { value: '1080p', label: '1080p' },
  { value: '720p', label: '720p' },
  { value: '480p', label: '480p' },
]

const currentEpisodeIndex = computed(() => {
  return episodes.value.findIndex(ep => ep.id === episodeId.value)
})

const hasPrevious = computed(() => currentEpisodeIndex.value > 0)
const hasNext = computed(() => currentEpisodeIndex.value < episodes.value.length - 1)

const previousEpisode = () => {
  if (hasPrevious.value) {
    const prevEpisode = episodes.value[currentEpisodeIndex.value - 1]
    router.push(`/watch/${animeId.value}/${prevEpisode.id}`)
  }
}

const nextEpisode = () => {
  if (hasNext.value) {
    const nextEp = episodes.value[currentEpisodeIndex.value + 1]
    router.push(`/watch/${animeId.value}/${nextEp.id}`)
  }
}

const selectEpisode = (epId: string) => {
  router.push(`/watch/${animeId.value}/${epId}`)
}

const loadEpisode = async () => {
  try {
    const fetchedEpisode = await fetchEpisode(episodeId.value)
    if (fetchedEpisode) {
      episode.value = fetchedEpisode as PlayerEpisode
      playerStore.setEpisode(episode.value)
    }
  } catch (err) {
    console.error('Failed to load episode:', err)
  }
}

vueWatch(() => route.params.episodeId, async (newId) => {
  if (newId) {
    episodeId.value = newId as string
    await loadEpisode()
  }
})

onMounted(async () => {
  await fetchAnime(animeId.value)
  await fetchEpisodes(animeId.value)
  await loadEpisode()
})
</script>
