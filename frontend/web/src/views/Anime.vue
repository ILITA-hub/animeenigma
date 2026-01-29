<template>
  <div class="anime-detail" v-if="anime">
    <div class="banner" :style="{ backgroundImage: `url(${anime.bannerImage || anime.coverImage})` }">
      <div class="banner-overlay"></div>
    </div>

    <div class="content">
      <div class="anime-info">
        <img :src="anime.coverImage" :alt="anime.title" class="cover-image" />
        <div class="info">
          <h1>{{ anime.title }}</h1>
          <div class="meta">
            <span class="badge">{{ anime.status }}</span>
            <span class="rating">★ {{ anime.rating }}/10</span>
            <span>{{ anime.releaseYear }}</span>
            <span>{{ anime.totalEpisodes }} Episodes</span>
          </div>
          <div class="genres">
            <span v-for="genre in anime.genres" :key="genre" class="genre-tag">
              {{ genre }}
            </span>
          </div>
          <p class="description">{{ anime.description }}</p>
          <div class="actions">
            <button @click="addToWatchlist" class="btn btn-primary">
              Add to Watchlist
            </button>
            <button @click="shareAnime" class="btn btn-secondary">
              Share
            </button>
          </div>
        </div>
      </div>

      <div class="episodes-section">
        <h2>Episodes</h2>
        <div v-if="loadingEpisodes" class="loading">Loading episodes...</div>
        <div v-else class="episodes-grid">
          <router-link
            v-for="episode in episodes"
            :key="episode.id"
            :to="`/watch/${anime.id}/${episode.id}`"
            class="episode-card"
          >
            <div class="episode-thumbnail" v-if="episode.thumbnail">
              <img :src="episode.thumbnail" :alt="episode.title" />
            </div>
            <div class="episode-info">
              <h3>Episode {{ episode.episodeNumber }}</h3>
              <p>{{ episode.title }}</p>
            </div>
          </router-link>
        </div>
      </div>
    </div>
  </div>
  <div v-else-if="loading" class="loading">Loading...</div>
  <div v-else-if="error" class="error">{{ error }}</div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { useAnime } from '@/composables/useAnime'

const route = useRoute()
const { anime, episodes, loading, error, fetchAnime, fetchEpisodes, addToWatchlist: addToWatchlistApi } = useAnime()
const loadingEpisodes = ref(false)

const addToWatchlist = async () => {
  if (anime.value) {
    const success = await addToWatchlistApi(anime.value.id)
    if (success) {
      alert('Added to watchlist!')
    }
  }
}

const shareAnime = () => {
  if (navigator.share) {
    navigator.share({
      title: anime.value?.title,
      text: anime.value?.description,
      url: window.location.href
    })
  } else {
    navigator.clipboard.writeText(window.location.href)
    alert('Link copied to clipboard!')
  }
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

<style scoped>
.anime-detail {
  min-height: 100vh;
}

.banner {
  height: 400px;
  background-size: cover;
  background-position: center;
  position: relative;
}

.banner-overlay {
  position: absolute;
  inset: 0;
  background: linear-gradient(to bottom, transparent, #0f0f0f);
}

.content {
  padding: 2rem;
  margin-top: -150px;
  position: relative;
}

.anime-info {
  display: flex;
  gap: 2rem;
  margin-bottom: 3rem;
}

.cover-image {
  width: 250px;
  height: 350px;
  object-fit: cover;
  border-radius: 12px;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.5);
}

.info {
  flex: 1;
}

.info h1 {
  font-size: 2.5rem;
  margin-bottom: 1rem;
  color: #fff;
}

.meta {
  display: flex;
  gap: 1rem;
  margin-bottom: 1rem;
  flex-wrap: wrap;
  align-items: center;
}

.badge {
  padding: 0.25rem 0.75rem;
  background: #ff6b6b;
  border-radius: 20px;
  font-size: 0.85rem;
  text-transform: uppercase;
}

.rating {
  color: #ffd700;
  font-weight: bold;
}

.genres {
  display: flex;
  gap: 0.5rem;
  margin-bottom: 1rem;
  flex-wrap: wrap;
}

.genre-tag {
  padding: 0.25rem 0.75rem;
  background: #333;
  border-radius: 4px;
  font-size: 0.85rem;
}

.description {
  color: #ccc;
  line-height: 1.6;
  margin-bottom: 1.5rem;
}

.actions {
  display: flex;
  gap: 1rem;
}

.btn {
  padding: 0.75rem 1.5rem;
  border: none;
  border-radius: 8px;
  font-size: 1rem;
  cursor: pointer;
  transition: transform 0.2s;
}

.btn:hover {
  transform: translateY(-2px);
}

.btn-primary {
  background: #ff6b6b;
  color: white;
}

.btn-secondary {
  background: #333;
  color: white;
}

.episodes-section {
  margin-top: 2rem;
}

.episodes-section h2 {
  font-size: 2rem;
  margin-bottom: 1.5rem;
  color: #fff;
}

.episodes-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(250px, 1fr));
  gap: 1rem;
}

.episode-card {
  background: #1a1a1a;
  border-radius: 8px;
  overflow: hidden;
  text-decoration: none;
  color: inherit;
  transition: transform 0.2s;
}

.episode-card:hover {
  transform: translateY(-4px);
}

.episode-thumbnail {
  width: 100%;
  height: 140px;
  overflow: hidden;
}

.episode-thumbnail img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.episode-info {
  padding: 1rem;
}

.episode-info h3 {
  font-size: 1.1rem;
  margin-bottom: 0.5rem;
  color: #fff;
}

.episode-info p {
  color: #999;
  font-size: 0.9rem;
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
