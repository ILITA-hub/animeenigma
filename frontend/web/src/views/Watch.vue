<template>
  <div class="watch-page">
    <div class="video-container">
      <VideoPlayer v-if="episode" :episode="episode" />
      <div v-else-if="loading" class="loading">Loading episode...</div>
      <div v-else-if="error" class="error">{{ error }}</div>
    </div>

    <div class="watch-info">
      <div class="episode-details">
        <h1>{{ episode?.title || 'Loading...' }}</h1>
        <p v-if="anime">{{ anime.title }} - Episode {{ episode?.episodeNumber }}</p>
      </div>

      <div class="controls">
        <button @click="previousEpisode" :disabled="!hasPrevious" class="nav-btn">
          ← Previous
        </button>
        <button @click="nextEpisode" :disabled="!hasNext" class="nav-btn">
          Next →
        </button>
      </div>
    </div>

    <div class="episodes-list">
      <h2>Episodes</h2>
      <div class="episodes-scroll">
        <button
          v-for="ep in episodes"
          :key="ep.id"
          @click="selectEpisode(ep.id)"
          :class="['episode-item', { active: ep.id === episodeId }]"
        >
          <span class="ep-number">{{ ep.episodeNumber }}</span>
          <span class="ep-title">{{ ep.title }}</span>
        </button>
      </div>
    </div>

    <div class="anime-info" v-if="anime">
      <h2>About This Anime</h2>
      <p>{{ anime.description }}</p>
      <div class="genres">
        <span v-for="genre in anime.genres" :key="genre" class="genre-tag">
          {{ genre }}
        </span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAnime } from '@/composables/useAnime'
import { usePlayerStore } from '@/stores/player'
import VideoPlayer from '@/components/player/VideoPlayer.vue'

const route = useRoute()
const router = useRouter()
const { anime, episodes, loading, error, fetchAnime, fetchEpisodes, fetchEpisode } = useAnime()
const playerStore = usePlayerStore()

const animeId = ref(route.params.animeId as string)
const episodeId = ref(route.params.episodeId as string)
const episode = ref<any>(null)

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
    episode.value = await fetchEpisode(episodeId.value)
    playerStore.setEpisode(episode.value)
  } catch (err) {
    console.error('Failed to load episode:', err)
  }
}

watch(() => route.params.episodeId, async (newId) => {
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

<style scoped>
.watch-page {
  background: #000;
  min-height: 100vh;
}

.video-container {
  width: 100%;
  background: #000;
}

.watch-info {
  padding: 1.5rem 2rem;
  background: #1a1a1a;
  display: flex;
  justify-content: space-between;
  align-items: center;
  flex-wrap: wrap;
  gap: 1rem;
}

.episode-details h1 {
  font-size: 1.5rem;
  margin-bottom: 0.5rem;
  color: #fff;
}

.episode-details p {
  color: #999;
}

.controls {
  display: flex;
  gap: 1rem;
}

.nav-btn {
  padding: 0.75rem 1.5rem;
  background: #333;
  color: white;
  border: none;
  border-radius: 8px;
  cursor: pointer;
  transition: background 0.3s;
}

.nav-btn:hover:not(:disabled) {
  background: #444;
}

.nav-btn:disabled {
  opacity: 0.3;
  cursor: not-allowed;
}

.episodes-list {
  padding: 2rem;
  background: #0f0f0f;
}

.episodes-list h2 {
  font-size: 1.5rem;
  margin-bottom: 1rem;
  color: #fff;
}

.episodes-scroll {
  display: flex;
  gap: 1rem;
  overflow-x: auto;
  padding-bottom: 1rem;
}

.episode-item {
  min-width: 200px;
  padding: 1rem;
  background: #1a1a1a;
  border: 2px solid transparent;
  border-radius: 8px;
  cursor: pointer;
  transition: all 0.3s;
  text-align: left;
  color: white;
}

.episode-item:hover {
  background: #222;
}

.episode-item.active {
  border-color: #ff6b6b;
  background: #222;
}

.ep-number {
  display: block;
  font-weight: bold;
  margin-bottom: 0.5rem;
  color: #ff6b6b;
}

.ep-title {
  display: block;
  font-size: 0.9rem;
  color: #ccc;
}

.anime-info {
  padding: 2rem;
  background: #0f0f0f;
}

.anime-info h2 {
  font-size: 1.5rem;
  margin-bottom: 1rem;
  color: #fff;
}

.anime-info p {
  color: #ccc;
  line-height: 1.6;
  margin-bottom: 1rem;
}

.genres {
  display: flex;
  gap: 0.5rem;
  flex-wrap: wrap;
}

.genre-tag {
  padding: 0.25rem 0.75rem;
  background: #333;
  border-radius: 4px;
  font-size: 0.85rem;
}

.loading,
.error {
  text-align: center;
  padding: 3rem;
  color: #999;
}

.error {
  color: #ff6b6b;
}
</style>
