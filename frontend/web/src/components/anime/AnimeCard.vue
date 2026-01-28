<template>
  <router-link :to="`/anime/${anime.id}`" class="anime-card">
    <div class="card-image">
      <img :src="anime.coverImage" :alt="anime.title" />
      <div class="overlay">
        <button class="play-btn">▶</button>
      </div>
      <div class="rating" v-if="anime.rating">
        ★ {{ anime.rating }}
      </div>
    </div>
    <div class="card-content">
      <h3 class="title">{{ anime.title }}</h3>
      <div class="meta">
        <span v-if="anime.releaseYear" class="year">{{ anime.releaseYear }}</span>
        <span v-if="anime.status" class="status">{{ anime.status }}</span>
      </div>
      <div class="genres" v-if="anime.genres && anime.genres.length > 0">
        <span v-for="genre in anime.genres.slice(0, 3)" :key="genre" class="genre">
          {{ genre }}
        </span>
      </div>
    </div>
  </router-link>
</template>

<script setup lang="ts">
import { defineProps } from 'vue'

interface Anime {
  id: string
  title: string
  coverImage: string
  rating?: number
  releaseYear?: number
  status?: string
  genres?: string[]
}

defineProps<{
  anime: Anime
}>()
</script>

<style scoped>
.anime-card {
  display: block;
  background: #1a1a1a;
  border-radius: 12px;
  overflow: hidden;
  text-decoration: none;
  color: inherit;
  transition: transform 0.3s, box-shadow 0.3s;
  cursor: pointer;
}

.anime-card:hover {
  transform: translateY(-8px);
  box-shadow: 0 8px 24px rgba(255, 107, 107, 0.3);
}

.card-image {
  position: relative;
  width: 100%;
  padding-top: 140%;
  overflow: hidden;
  background: #111;
}

.card-image img {
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  object-fit: cover;
  transition: transform 0.3s;
}

.anime-card:hover .card-image img {
  transform: scale(1.1);
}

.overlay {
  position: absolute;
  inset: 0;
  background: rgba(0, 0, 0, 0.6);
  display: flex;
  align-items: center;
  justify-content: center;
  opacity: 0;
  transition: opacity 0.3s;
}

.anime-card:hover .overlay {
  opacity: 1;
}

.play-btn {
  width: 60px;
  height: 60px;
  border-radius: 50%;
  background: #ff6b6b;
  border: none;
  color: white;
  font-size: 1.5rem;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: transform 0.2s;
}

.play-btn:hover {
  transform: scale(1.1);
}

.rating {
  position: absolute;
  top: 0.5rem;
  right: 0.5rem;
  background: rgba(0, 0, 0, 0.8);
  padding: 0.25rem 0.5rem;
  border-radius: 4px;
  color: #ffd700;
  font-size: 0.9rem;
  font-weight: bold;
}

.card-content {
  padding: 1rem;
}

.title {
  font-size: 1.1rem;
  font-weight: 600;
  margin-bottom: 0.5rem;
  color: #fff;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
  line-height: 1.3;
}

.meta {
  display: flex;
  gap: 0.5rem;
  margin-bottom: 0.5rem;
  font-size: 0.85rem;
}

.year {
  color: #999;
}

.status {
  padding: 0.15rem 0.5rem;
  background: #ff6b6b;
  border-radius: 12px;
  font-size: 0.75rem;
  text-transform: uppercase;
}

.genres {
  display: flex;
  gap: 0.25rem;
  flex-wrap: wrap;
}

.genre {
  padding: 0.15rem 0.5rem;
  background: #333;
  border-radius: 4px;
  font-size: 0.75rem;
  color: #aaa;
}
</style>
