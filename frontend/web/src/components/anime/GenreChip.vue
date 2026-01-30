<template>
  <router-link
    :to="`/browse?genre=${encodeURIComponent(genre)}`"
    class="inline-flex items-center gap-2 px-4 py-3 rounded-xl bg-white/5 border border-white/10 text-white/80 transition-all hover:border-cyan-500/50 hover:text-cyan-400 hover:bg-white/10 touch-target"
  >
    <!-- Icon -->
    <component v-if="icon" :is="icon" class="w-5 h-5" />
    <span v-else-if="emoji" class="text-lg">{{ emoji }}</span>

    <!-- Label -->
    <span class="font-medium">{{ label || genre }}</span>
  </router-link>
</template>

<script setup lang="ts">
import { computed, type Component } from 'vue'

const props = defineProps<{
  genre: string
  label?: string
  icon?: Component
  emoji?: string
}>()

// Default emoji mapping for common genres
const genreEmojis: Record<string, string> = {
  'Action': '⚔️',
  'Adventure': '🗺️',
  'Comedy': '😂',
  'Drama': '🎭',
  'Fantasy': '🧙',
  'Horror': '👻',
  'Mystery': '🔍',
  'Romance': '💕',
  'Sci-Fi': '🚀',
  'Slice of Life': '☕',
  'Sports': '⚽',
  'Supernatural': '✨',
  'Thriller': '😱',
  'Mecha': '🤖',
  'Music': '🎵',
  'Psychological': '🧠',
  'School': '🏫',
  'Shounen': '👊',
  'Shoujo': '🌸',
  'Isekai': '🌀',
}

const emoji = computed(() => {
  if (props.emoji) return props.emoji
  return genreEmojis[props.genre] || '🎬'
})
</script>
