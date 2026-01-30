<template>
  <div class="home">
    <section class="hero">
      <div class="hero-content">
        <h1>Welcome to AnimeEnigma</h1>
        <p>Stream your favorite anime series and movies</p>
        <router-link to="/browse" class="cta-button">Browse Anime</router-link>
      </div>
    </section>

    <section class="section">
      <h2>Trending Now</h2>
      <div v-if="loading" class="loading">Loading...</div>
      <div v-else-if="error" class="error">{{ error }}</div>
      <div v-else class="anime-grid">
        <AnimeCard
          v-for="anime in trendingAnime"
          :key="anime.id"
          :anime="anime"
        />
      </div>
    </section>

    <section class="section">
      <h2>Popular Anime</h2>
      <div v-if="loading" class="loading">Loading...</div>
      <div v-else class="anime-grid">
        <AnimeCard
          v-for="anime in popularAnime"
          :key="anime.id"
          :anime="anime"
        />
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useAnime } from '@/composables/useAnime'
import AnimeCard from '@/components/anime/AnimeCard.vue'

interface Anime {
  id: string
  title: string
  coverImage: string
  rating?: number
  releaseYear?: number
  status?: string
  genres?: string[]
}

const { loading, error, fetchTrending, fetchPopular } = useAnime()
const trendingAnime = ref<Anime[]>([])
const popularAnime = ref<Anime[]>([])

onMounted(async () => {
  try {
    trendingAnime.value = await fetchTrending()
    popularAnime.value = await fetchPopular()
  } catch (err) {
    console.error('Failed to load anime:', err)
  }
})
</script>

<style scoped>
.home {
  padding-bottom: 2rem;
}

.hero {
  height: 60vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  text-align: center;
  margin-bottom: 3rem;
}

.hero-content h1 {
  font-size: 3rem;
  margin-bottom: 1rem;
  color: white;
}

.hero-content p {
  font-size: 1.5rem;
  margin-bottom: 2rem;
  color: rgba(255, 255, 255, 0.9);
}

.cta-button {
  display: inline-block;
  padding: 1rem 2rem;
  background: #ff6b6b;
  color: white;
  text-decoration: none;
  border-radius: 8px;
  font-size: 1.1rem;
  transition: background 0.3s;
}

.cta-button:hover {
  background: #ff5252;
}

.section {
  padding: 2rem;
}

.section h2 {
  font-size: 2rem;
  margin-bottom: 1.5rem;
  color: #fff;
}

.anime-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
  gap: 1.5rem;
}

.loading,
.error {
  text-align: center;
  padding: 2rem;
  color: #999;
}

.error {
  color: #ff6b6b;
}
</style>
