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
import { getGenreEmoji } from '@/utils/genre-emoji'

const props = defineProps<{
  genre: string
  label?: string
  icon?: Component
  emoji?: string
}>()

const emoji = computed(() => {
  if (props.emoji) return props.emoji
  return getGenreEmoji(props.genre)
})
</script>
