<template>
  <router-link
    :to="`/browse?genre=${encodeURIComponent(genre)}`"
    class="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-full bg-white/5 border border-white/10 text-sm text-white/80 transition-all hover:border-cyan-500/50 hover:text-cyan-400 hover:bg-white/10 min-h-[44px] md:min-h-0"
  >
    <!-- Icon -->
    <component v-if="icon" :is="icon" class="w-4 h-4" />
    <span v-else-if="emoji" class="text-base">{{ emoji }}</span>

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
