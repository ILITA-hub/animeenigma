<template>
  <div v-if="anime" class="min-h-screen pb-20 md:pb-0">
    <!-- Hero Banner with Blurred Background -->
    <div class="relative h-[50vh] md:h-[60vh] overflow-hidden">
      <!-- Background Image -->
      <div
        class="absolute inset-0 bg-cover bg-center scale-110 blur-sm"
        :style="{ backgroundImage: `url(${anime.bannerImage || anime.coverImage})` }"
      />
      <!-- Gradient Overlays -->
      <div class="absolute inset-0 bg-gradient-to-t from-base via-base/70 to-transparent" />
      <div class="absolute inset-0 bg-gradient-to-r from-base/80 to-transparent" />
    </div>

    <!-- Main Content -->
    <div class="relative z-10 max-w-7xl mx-auto px-4 lg:px-8 -mt-64 md:-mt-72">
      <div class="flex flex-col md:flex-row gap-6 md:gap-8">
        <!-- Poster -->
        <div class="flex-shrink-0">
          <div class="w-40 md:w-56 aspect-[2/3] rounded-xl overflow-hidden shadow-2xl ring-1 ring-white/10">
            <img
              :src="anime.coverImage"
              :alt="anime.title"
              class="w-full h-full object-cover"
            />
          </div>
        </div>

        <!-- Info -->
        <div class="flex-1 pt-2">
          <!-- Title -->
          <h1 class="text-2xl md:text-4xl font-bold text-white mb-2">
            {{ anime.title }}
          </h1>
          <p v-if="(anime as AnimeWithExtras).japaneseTitle" class="text-lg text-white/50 mb-4">
            {{ (anime as AnimeWithExtras).japaneseTitle }}
          </p>

          <!-- Meta Info -->
          <div class="flex flex-wrap items-center gap-3 mb-4">
            <Badge :variant="statusVariant" size="md">
              {{ $t(`anime.status.${anime.status?.toLowerCase() || 'ongoing'}`) }}
            </Badge>
            <span class="text-white/60">{{ anime.releaseYear }}</span>
            <span class="text-white/30">•</span>
            <span class="text-white/60">{{ (anime as AnimeWithExtras).type || 'TV' }}</span>
            <span class="text-white/30">•</span>
            <span class="text-white/60">{{ anime.totalEpisodes }} {{ $t('anime.episodes') }}</span>
          </div>

          <!-- Rating -->
          <div v-if="anime.rating" class="flex items-center gap-2 mb-4">
            <div class="flex items-center gap-1 text-amber-400">
              <svg class="w-5 h-5" fill="currentColor" viewBox="0 0 20 20">
                <path d="M9.049 2.927c.3-.921 1.603-.921 1.902 0l1.07 3.292a1 1 0 00.95.69h3.462c.969 0 1.371 1.24.588 1.81l-2.8 2.034a1 1 0 00-.364 1.118l1.07 3.292c.3.921-.755 1.688-1.54 1.118l-2.8-2.034a1 1 0 00-1.175 0l-2.8 2.034c-.784.57-1.838-.197-1.539-1.118l1.07-3.292a1 1 0 00-.364-1.118L2.98 8.72c-.783-.57-.38-1.81.588-1.81h3.461a1 1 0 00.951-.69l1.07-3.292z" />
              </svg>
              <span class="font-bold text-lg">{{ anime.rating.toFixed(1) }}</span>
            </div>
            <span class="text-white/40">/10</span>
          </div>

          <!-- Actions -->
          <div class="flex flex-wrap gap-3 mb-6">
            <Button
              v-if="episodes.length > 0"
              size="lg"
              @click="watchFirstEpisode"
            >
              <template #icon>
                <svg class="w-5 h-5" fill="currentColor" viewBox="0 0 24 24">
                  <path d="M8 5v14l11-7z" />
                </svg>
              </template>
              {{ $t('anime.watch') }} EP1
            </Button>
            <Button
              size="lg"
              :variant="isInWatchlist ? 'secondary' : 'outline'"
              @click="toggleWatchlist"
            >
              <template #icon>
                <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path v-if="isInWatchlist" stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
                  <path v-else stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
                </svg>
              </template>
              {{ isInWatchlist ? $t('anime.removeFromList') : $t('anime.addToList') }}
            </Button>
          </div>

          <!-- Genres -->
          <div class="flex flex-wrap gap-2">
            <GenreChip
              v-for="genre in anime.genres"
              :key="genre"
              :genre="genre"
            />
          </div>
        </div>
      </div>

      <!-- Synopsis -->
      <section class="mt-8">
        <h2 class="text-xl font-semibold text-white mb-3">{{ $t('anime.synopsis') }}</h2>
        <div class="glass-card p-4">
          <p
            class="text-white/70 leading-relaxed"
            :class="{ 'line-clamp-4': !synopsisExpanded }"
          >
            {{ anime.description }}
          </p>
          <button
            v-if="anime.description && anime.description.length > 300"
            class="mt-2 text-cyan-400 hover:text-cyan-300 transition-colors text-sm"
            @click="synopsisExpanded = !synopsisExpanded"
          >
            {{ synopsisExpanded ? 'Show less' : 'Show more' }}
          </button>
        </div>
      </section>

      <!-- Episodes -->
      <section class="mt-8">
        <div class="flex items-center justify-between mb-4">
          <h2 class="text-xl font-semibold text-white">{{ $t('anime.episodes') }}</h2>
          <div class="flex gap-2">
            <button
              class="p-2 rounded-lg transition-colors"
              :class="episodeView === 'grid' ? 'bg-white/10 text-white' : 'text-white/50 hover:text-white'"
              @click="episodeView = 'grid'"
            >
              <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2H6a2 2 0 01-2-2V6zm10 0a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2h-2a2 2 0 01-2-2V6zM4 16a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2H6a2 2 0 01-2-2v-2zm10 0a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2h-2a2 2 0 01-2-2v-2z" />
              </svg>
            </button>
            <button
              class="p-2 rounded-lg transition-colors"
              :class="episodeView === 'list' ? 'bg-white/10 text-white' : 'text-white/50 hover:text-white'"
              @click="episodeView = 'list'"
            >
              <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 12h16M4 18h16" />
              </svg>
            </button>
          </div>
        </div>

        <div v-if="loadingEpisodes" class="flex justify-center py-12">
          <div class="w-8 h-8 border-2 border-cyan-400 border-t-transparent rounded-full animate-spin" />
        </div>

        <!-- Grid View -->
        <div
          v-else-if="episodeView === 'grid'"
          class="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 gap-4"
        >
          <EpisodeCard
            v-for="episode in episodes"
            :key="episode.id"
            :episode-number="episode.episodeNumber"
            :title="episode.title"
            :thumbnail="episode.thumbnail"
            :duration="episode.duration"
            :watched="(episode as EpisodeWithProgress).watched"
            :progress="(episode as EpisodeWithProgress).progress"
            @select="goToEpisode(episode.id)"
          />
        </div>

        <!-- List View -->
        <div v-else class="space-y-2">
          <button
            v-for="episode in episodes"
            :key="episode.id"
            class="w-full flex items-center gap-4 p-3 rounded-xl bg-white/5 border border-white/10 hover:bg-white/10 hover:border-cyan-500/30 transition-all text-left"
            @click="goToEpisode(episode.id)"
          >
            <Badge variant="default" size="md">{{ episode.episodeNumber }}</Badge>
            <div class="flex-1 min-w-0">
              <h4 class="text-white font-medium truncate">{{ episode.title || `Episode ${episode.episodeNumber}` }}</h4>
              <p v-if="episode.duration" class="text-white/50 text-sm">{{ formatDuration(episode.duration) }}</p>
            </div>
            <div v-if="(episode as EpisodeWithProgress).watched" class="w-6 h-6 rounded-full bg-emerald-500 flex items-center justify-center">
              <svg class="w-4 h-4 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
              </svg>
            </div>
            <svg class="w-5 h-5 text-white/30" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
            </svg>
          </button>
        </div>
      </section>

      <!-- Related Anime -->
      <section v-if="relatedAnime.length > 0" class="mt-8">
        <Carousel
          :items="relatedAnime"
          :title="$t('anime.related')"
          item-key="id"
        >
          <template #default="{ item }">
            <AnimeCardNew :anime="(item as RelatedAnime)" />
          </template>
        </Carousel>
      </section>
    </div>
  </div>

  <!-- Loading State -->
  <div v-else-if="loading" class="min-h-screen flex items-center justify-center">
    <div class="text-center">
      <div class="w-12 h-12 border-2 border-cyan-400 border-t-transparent rounded-full animate-spin mx-auto mb-4" />
      <p class="text-white/60">{{ $t('common.loading') }}</p>
    </div>
  </div>

  <!-- Error State -->
  <div v-else-if="error" class="min-h-screen flex items-center justify-center">
    <div class="text-center">
      <p class="text-pink-400 mb-4">{{ error }}</p>
      <Button variant="outline" @click="retry">{{ $t('common.retry') }}</Button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAnime } from '@/composables/useAnime'
import { Badge, Button } from '@/components/ui'
import { EpisodeCard, GenreChip, AnimeCardNew } from '@/components/anime'
import { Carousel } from '@/components/carousel'

interface AnimeWithExtras {
  japaneseTitle?: string
  type?: string
}

interface EpisodeWithProgress {
  watched?: boolean
  progress?: number
}

interface RelatedAnime {
  id: string
  title: string
  coverImage: string
  rating?: number
  releaseYear?: number
  episodes?: number
  genres?: string[]
}

const route = useRoute()
const router = useRouter()
const { anime, episodes, loading, error, fetchAnime, fetchEpisodes, addToWatchlist: addToWatchlistApi } = useAnime()

const loadingEpisodes = ref(false)
const synopsisExpanded = ref(false)
const episodeView = ref<'grid' | 'list'>('grid')
const isInWatchlist = ref(false)
const relatedAnime = ref<RelatedAnime[]>([])

const statusVariant = computed(() => {
  const status = anime.value?.status?.toLowerCase()
  if (status === 'completed') return 'success'
  if (status === 'upcoming') return 'warning'
  return 'primary' // ongoing
})

const watchFirstEpisode = () => {
  if (anime.value && episodes.value.length > 0) {
    router.push(`/watch/${anime.value.id}/${episodes.value[0].id}`)
  }
}

const goToEpisode = (episodeId: string) => {
  if (anime.value) {
    router.push(`/watch/${anime.value.id}/${episodeId}`)
  }
}

const toggleWatchlist = async () => {
  if (anime.value) {
    if (isInWatchlist.value) {
      // TODO: Remove from watchlist
      isInWatchlist.value = false
    } else {
      const success = await addToWatchlistApi(anime.value.id)
      if (success) {
        isInWatchlist.value = true
      }
    }
  }
}

const formatDuration = (seconds: number): string => {
  const mins = Math.floor(seconds / 60)
  return `${mins} min`
}

const retry = () => {
  const animeId = route.params.id as string
  fetchAnime(animeId)
}

onMounted(async () => {
  const animeId = route.params.id as string
  await fetchAnime(animeId)
  loadingEpisodes.value = true
  try {
    await fetchEpisodes(animeId)
  } finally {
    loadingEpisodes.value = false
  }
})
</script>
