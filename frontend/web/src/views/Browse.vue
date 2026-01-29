<template>
  <div class="browse">
    <div class="browse-header">
      <h1>Browse Anime</h1>
      <div class="search-filters">
        <input
          v-model="searchQuery"
          type="text"
          placeholder="Search anime..."
          @input="handleSearch"
          class="search-input"
        />
        <select v-model="selectedGenre" @change="handleFilter" class="filter-select">
          <option value="">All Genres</option>
          <option value="action">Action</option>
          <option value="adventure">Adventure</option>
          <option value="comedy">Comedy</option>
          <option value="drama">Drama</option>
          <option value="fantasy">Fantasy</option>
          <option value="romance">Romance</option>
          <option value="sci-fi">Sci-Fi</option>
        </select>
        <select v-model="sortBy" @change="handleFilter" class="filter-select">
          <option value="title">Title</option>
          <option value="rating">Rating</option>
          <option value="year">Year</option>
          <option value="popularity">Popularity</option>
        </select>
      </div>
    </div>

    <div v-if="loading" class="loading">Loading...</div>
    <div v-else-if="error" class="error">{{ error }}</div>
    <div v-else-if="animeList.length === 0" class="empty">
      No anime found. Try adjusting your filters.
    </div>
    <div v-else class="anime-grid">
      <AnimeCard
        v-for="anime in animeList"
        :key="anime.id"
        :anime="anime"
      />
    </div>

    <div v-if="hasMore" class="load-more">
      <button @click="loadMore" :disabled="loading">
        {{ loading ? 'Loading...' : 'Load More' }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { useAnime } from '@/composables/useAnime'
import AnimeCard from '@/components/anime/AnimeCard.vue'

const route = useRoute()
const { animeList, loading, error, fetchAnimeList, searchAnime } = useAnime()

const searchQuery = ref('')
const selectedGenre = ref('')
const sortBy = ref('popularity')
const currentPage = ref(1)
const hasMore = ref(true)

const handleSearch = async () => {
  if (searchQuery.value.trim()) {
    await searchAnime(searchQuery.value)
  } else {
    await loadAnime()
  }
}

const handleFilter = async () => {
  currentPage.value = 1
  await loadAnime()
}

const loadAnime = async () => {
  const params = {
    page: currentPage.value,
    genre: selectedGenre.value,
    sort: sortBy.value
  }
  const results = await fetchAnimeList(params)
  hasMore.value = results.length >= 20
}

const loadMore = async () => {
  currentPage.value++
  await loadAnime()
}

onMounted(async () => {
  if (route.query.q) {
    searchQuery.value = route.query.q as string
    await searchAnime(searchQuery.value)
  } else {
    await loadAnime()
  }
})
</script>

<style scoped>
.browse {
  padding: 2rem;
  min-height: 80vh;
}

.browse-header {
  margin-bottom: 2rem;
}

.browse-header h1 {
  font-size: 2.5rem;
  margin-bottom: 1.5rem;
  color: #fff;
}

.search-filters {
  display: flex;
  gap: 1rem;
  flex-wrap: wrap;
}

.search-input {
  flex: 1;
  min-width: 250px;
  padding: 0.75rem;
  background: #1a1a1a;
  border: 2px solid #333;
  border-radius: 8px;
  color: white;
  font-size: 1rem;
}

.search-input:focus {
  outline: none;
  border-color: #ff6b6b;
}

.filter-select {
  padding: 0.75rem;
  background: #1a1a1a;
  border: 2px solid #333;
  border-radius: 8px;
  color: white;
  font-size: 1rem;
  cursor: pointer;
}

.filter-select:focus {
  outline: none;
  border-color: #ff6b6b;
}

.anime-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
  gap: 1.5rem;
  margin-bottom: 2rem;
}

.loading,
.error,
.empty {
  text-align: center;
  padding: 3rem;
  color: #999;
}

.error {
  color: #ff6b6b;
}

.load-more {
  text-align: center;
  margin-top: 2rem;
}

.load-more button {
  padding: 1rem 2rem;
  background: #ff6b6b;
  color: white;
  border: none;
  border-radius: 8px;
  font-size: 1rem;
  cursor: pointer;
  transition: background 0.3s;
}

.load-more button:hover:not(:disabled) {
  background: #ff5252;
}

.load-more button:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
</style>
